package structureaware

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	chunking "github.com/wo4zhuzi/eino-document-chunking"
	"github.com/wo4zhuzi/eino-document-chunking/internal/metadatautil"
	"github.com/wo4zhuzi/eino-document-chunking/internal/textutil"
)

const (
	// StructureAwareStrategyName is the stable strategy identifier.
	StructureAwareStrategyName = "structure_aware"
	// ChunkKindStructure identifies flat structure-aware chunks.
	ChunkKindStructure = chunking.ChunkKind("structure")
)

type structuredUnit struct {
	block   chunking.Block
	content string
}

type chunkDraft struct {
	documentID    string
	path          []string
	depth         int
	units         []structuredUnit
	sourceUnitIDs []string
	metadata      map[string]any
	kinds         []chunking.BlockKind
	hasHeading    bool
}

// Name implements chunking.Strategy.
func (*StructureAwareStrategy) Name() string {
	return StructureAwareStrategyName
}

// Chunk implements chunking.Strategy.
func (strategy *StructureAwareStrategy) Chunk(
	ctx context.Context,
	input chunking.StrategyInput,
) (*chunking.StrategyOutput, error) {
	if strategy == nil || strategy.maxRunes < 1 || strategy.minRunes < 1 {
		return nil, fmt.Errorf("%w: structure-aware strategy is unavailable", chunking.ErrInvalidConfig)
	}
	if input.IDGenerator == nil {
		return nil, fmt.Errorf("%w: id generator is required", chunking.ErrInvalidConfig)
	}
	if err := contextError(ctx, "run structure-aware strategy"); err != nil {
		return nil, err
	}
	blocks := cloneBlocks(input.Blocks)
	if err := validateStructuredBlocks(blocks); err != nil {
		return nil, err
	}

	units := make([]structuredUnit, 0, len(blocks))
	for _, block := range blocks {
		prepared, err := strategy.prepareBlock(ctx, block)
		if err != nil {
			return nil, err
		}
		units = append(units, prepared...)
	}
	if len(units) == 0 {
		return nil, chunking.ErrNoValidChunks
	}

	chunks := make([]chunking.Chunk, 0, len(units))
	seenIDs := make(map[string]struct{}, len(units))
	var current *chunkDraft
	for _, unit := range units {
		if err := contextError(ctx, "assemble structure-aware chunks"); err != nil {
			return nil, err
		}
		if current != nil && strategy.mustStartNew(current, unit) {
			var err error
			chunks, err = strategy.appendChunk(ctx, input, chunks, current, seenIDs)
			if err != nil {
				return nil, err
			}
			current = nil
		}
		if current == nil {
			current = newChunkDraft(unit)
			continue
		}
		current.add(unit)
	}
	if current != nil {
		var err error
		chunks, err = strategy.appendChunk(ctx, input, chunks, current, seenIDs)
		if err != nil {
			return nil, err
		}
	}
	if len(chunks) == 0 {
		return nil, chunking.ErrNoValidChunks
	}
	linkAdjacentChunks(chunks)
	return &chunking.StrategyOutput{
		Chunks:    chunks,
		Relations: buildRelations(chunks),
	}, nil
}

func (strategy *StructureAwareStrategy) prepareBlock(
	ctx context.Context,
	block chunking.Block,
) ([]structuredUnit, error) {
	if err := contextError(ctx, "prepare structured block"); err != nil {
		return nil, err
	}
	available := strategy.availableBodyRunes(block.Structure.Path, block.Structure.Kind == chunking.BlockKindHeading)
	if available < 1 {
		return nil, fmt.Errorf("%w: block %q structure path consumes MaxRunes", chunking.ErrOversizeBlock, block.ID)
	}
	if utf8.RuneCountInString(strings.TrimSpace(block.Content)) <= available {
		return []structuredUnit{{block: block, content: strings.TrimSpace(block.Content)}}, nil
	}

	var parts []string
	if isAtomicKind(block.Structure.Kind) {
		if strategy.oversizeSplitter == nil {
			return nil, fmt.Errorf("%w: atomic block %q kind=%q", chunking.ErrOversizeBlock, block.ID, block.Structure.Kind)
		}
		var err error
		parts, err = strategy.oversizeSplitter.Split(ctx, cloneBlock(block), available)
		if err != nil {
			return nil, fmt.Errorf("%w: split atomic block %q: %w", chunking.ErrOversizeBlock, block.ID, err)
		}
	} else {
		parts = textutil.SplitBounded(block.Content, available)
	}
	units := make([]structuredUnit, 0, len(parts))
	for partIndex, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if utf8.RuneCountInString(part) > available {
			return nil, fmt.Errorf("%w: block %q part %d exceeds %d runes", chunking.ErrOversizeBlock, block.ID, partIndex, available)
		}
		units = append(units, structuredUnit{block: cloneBlock(block), content: part})
	}
	if len(units) == 0 {
		return nil, fmt.Errorf("%w: block %q splitter produced no content", chunking.ErrOversizeBlock, block.ID)
	}
	return units, nil
}

func (strategy *StructureAwareStrategy) availableBodyRunes(path []string, hasHeading bool) int {
	if strategy.headingContext != HeadingContextPrepend || len(path) == 0 || hasHeading {
		return strategy.maxRunes
	}
	prefix := strings.Join(path, " > ") + "\n\n"
	return strategy.maxRunes - utf8.RuneCountInString(prefix)
}

func (strategy *StructureAwareStrategy) mustStartNew(current *chunkDraft, unit structuredUnit) bool {
	structure := unit.block.Structure
	if current.documentID != unit.block.DocumentID ||
		structure.Boundary == chunking.BlockBoundaryHard ||
		structure.Kind == chunking.BlockKindHeading ||
		!slices.Equal(current.path, structure.Path) {
		return true
	}
	if structure.Boundary == chunking.BlockBoundarySoft && strategy.renderedRunes(current) >= strategy.minRunes {
		return true
	}
	candidate := current.clone()
	candidate.add(unit)
	return strategy.renderedRunes(candidate) > strategy.maxRunes
}

func (strategy *StructureAwareStrategy) appendChunk(
	ctx context.Context,
	input chunking.StrategyInput,
	chunks []chunking.Chunk,
	draft *chunkDraft,
	seenIDs map[string]struct{},
) ([]chunking.Chunk, error) {
	content := strategy.render(draft)
	if strings.TrimSpace(content) == "" {
		return nil, chunking.ErrNoValidChunks
	}
	if utf8.RuneCountInString(content) > strategy.maxRunes {
		return nil, fmt.Errorf("%w: assembled chunk exceeds %d runes", chunking.ErrOversizeBlock, strategy.maxRunes)
	}
	sequence := len(chunks) + 1
	id, err := input.IDGenerator.Generate(ctx, chunking.IDInput{
		Profile:       input.Profile,
		StrategyName:  strategy.Name(),
		Kind:          ChunkKindStructure,
		Level:         0,
		DocumentID:    draft.documentID,
		Sequence:      sequence,
		Content:       content,
		SourceUnitIDs: draft.sourceUnitIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", chunking.ErrIDGenerationFailed, err)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("%w: generator returned an empty id", chunking.ErrIDGenerationFailed)
	}
	if _, exists := seenIDs[id]; exists {
		return nil, fmt.Errorf("%w: chunk id %q", chunking.ErrDuplicateID, id)
	}
	seenIDs[id] = struct{}{}
	metadata, err := decorateStructureMetadata(draft.metadata, draft.depth, draft.path, draft.kinds)
	if err != nil {
		return nil, err
	}
	return append(chunks, chunking.Chunk{
		ID:            id,
		Kind:          ChunkKindStructure,
		Content:       content,
		DocumentID:    draft.documentID,
		Level:         0,
		SourceUnitIDs: append([]string(nil), draft.sourceUnitIDs...),
		Sequence:      sequence,
		Metadata:      metadata,
	}), nil
}

func (strategy *StructureAwareStrategy) renderedRunes(draft *chunkDraft) int {
	return utf8.RuneCountInString(strategy.render(draft))
}

func (strategy *StructureAwareStrategy) render(draft *chunkDraft) string {
	parts := make([]string, len(draft.units))
	for index, unit := range draft.units {
		parts[index] = unit.content
	}
	body := strings.Join(parts, "\n\n")
	if strategy.headingContext != HeadingContextPrepend || len(draft.path) == 0 || draft.hasHeading {
		return body
	}
	return strings.Join(draft.path, " > ") + "\n\n" + body
}

func newChunkDraft(unit structuredUnit) *chunkDraft {
	structure := unit.block.Structure
	draft := &chunkDraft{
		documentID: unit.block.DocumentID,
		path:       append([]string(nil), structure.Path...),
		depth:      structure.Depth,
		metadata:   metadatautil.Clone(unit.block.Metadata),
	}
	draft.add(unit)
	return draft
}

func (draft *chunkDraft) add(unit structuredUnit) {
	draft.units = append(draft.units, unit)
	if unit.block.Structure.Depth < draft.depth {
		draft.depth = unit.block.Structure.Depth
	}
	draft.hasHeading = draft.hasHeading || unit.block.Structure.Kind == chunking.BlockKindHeading
	for _, sourceUnitID := range unit.block.SourceUnitIDs {
		if !slices.Contains(draft.sourceUnitIDs, sourceUnitID) {
			draft.sourceUnitIDs = append(draft.sourceUnitIDs, sourceUnitID)
		}
	}
	if !slices.Contains(draft.kinds, unit.block.Structure.Kind) {
		draft.kinds = append(draft.kinds, unit.block.Structure.Kind)
	}
	draft.metadata = metadatautil.MergePreserving(draft.metadata, unit.block.Metadata)
}

func (draft *chunkDraft) clone() *chunkDraft {
	cloned := *draft
	cloned.path = append([]string(nil), draft.path...)
	cloned.units = append([]structuredUnit(nil), draft.units...)
	cloned.sourceUnitIDs = append([]string(nil), draft.sourceUnitIDs...)
	cloned.metadata = metadatautil.Clone(draft.metadata)
	cloned.kinds = append([]chunking.BlockKind(nil), draft.kinds...)
	return &cloned
}

func validateStructuredBlocks(blocks []chunking.Block) error {
	blockByID := make(map[string]chunking.Block, len(blocks))
	blockIndexByID := make(map[string]int, len(blocks))
	for index, block := range blocks {
		if strings.TrimSpace(block.ID) == "" || strings.TrimSpace(block.DocumentID) == "" || strings.TrimSpace(block.Content) == "" {
			return fmt.Errorf("%w: block at index %d has empty id, document id, or content", chunking.ErrInvalidBlock, index)
		}
		if block.Structure == nil {
			return fmt.Errorf("%w: block %q", chunking.ErrStructureRequired, block.ID)
		}
		if block.Structure.Kind == "" || block.Structure.Depth < 0 {
			return fmt.Errorf("%w: block %q has invalid kind or depth", chunking.ErrInvalidStructure, block.ID)
		}
		switch block.Structure.Boundary {
		case chunking.BlockBoundaryNone, chunking.BlockBoundarySoft, chunking.BlockBoundaryHard:
		default:
			return fmt.Errorf("%w: block %q has unsupported boundary %q", chunking.ErrInvalidStructure, block.ID, block.Structure.Boundary)
		}
		if _, exists := blockByID[block.ID]; exists {
			return fmt.Errorf("%w: block id %q", chunking.ErrDuplicateID, block.ID)
		}
		blockByID[block.ID] = block
		blockIndexByID[block.ID] = index
	}
	for blockIndex, block := range blocks {
		parentID := strings.TrimSpace(block.Structure.ParentID)
		if parentID == "" {
			continue
		}
		parent, exists := blockByID[parentID]
		if !exists || parent.Structure == nil {
			return fmt.Errorf("%w: block %q references missing structural parent %q", chunking.ErrInvalidStructure, block.ID, parentID)
		}
		if blockIndexByID[parentID] >= blockIndex ||
			parent.DocumentID != block.DocumentID ||
			parent.Structure.Depth >= block.Structure.Depth ||
			!isPathPrefix(parent.Structure.Path, block.Structure.Path) {
			return fmt.Errorf("%w: block %q has invalid structural parent %q", chunking.ErrInvalidStructure, block.ID, parentID)
		}
	}
	return nil
}

func isPathPrefix(parent, child []string) bool {
	return len(parent) <= len(child) && slices.Equal(parent, child[:len(parent)])
}

func cloneBlocks(blocks []chunking.Block) []chunking.Block {
	cloned := make([]chunking.Block, len(blocks))
	for index, block := range blocks {
		cloned[index] = cloneBlock(block)
	}
	return cloned
}

func cloneBlock(block chunking.Block) chunking.Block {
	cloned := block
	cloned.SourceUnitIDs = append([]string(nil), block.SourceUnitIDs...)
	cloned.Metadata = metadatautil.Clone(block.Metadata)
	if block.Structure != nil {
		structure := *block.Structure
		structure.Path = append([]string(nil), block.Structure.Path...)
		cloned.Structure = &structure
	}
	return cloned
}

func isAtomicKind(kind chunking.BlockKind) bool {
	return kind == chunking.BlockKindCode ||
		kind == chunking.BlockKindCodeBlock ||
		kind == chunking.BlockKindTable
}

func linkAdjacentChunks(chunks []chunking.Chunk) {
	lastIndexByDocument := make(map[string]int)
	for index := range chunks {
		if previousIndex, exists := lastIndexByDocument[chunks[index].DocumentID]; exists {
			chunks[index].PreviousID = chunks[previousIndex].ID
			chunks[previousIndex].NextID = chunks[index].ID
		}
		lastIndexByDocument[chunks[index].DocumentID] = index
	}
}

func buildRelations(chunks []chunking.Chunk) []chunking.Relation {
	relations := make([]chunking.Relation, 0, len(chunks)*2)
	chunkByID := make(map[string]chunking.Chunk, len(chunks))
	for _, chunk := range chunks {
		chunkByID[chunk.ID] = chunk
	}
	for _, chunk := range chunks {
		if chunk.NextID != "" {
			next := chunkByID[chunk.NextID]
			relations = append(relations, chunking.Relation{
				Type:      chunking.RelationTypePreviousNext,
				FromID:    chunk.ID,
				ToID:      next.ID,
				FromLevel: chunk.Level,
				ToLevel:   next.Level,
			})
		}
		for _, sourceUnitID := range chunk.SourceUnitIDs {
			relations = append(relations, chunking.Relation{
				Type:      chunking.RelationTypeSource,
				FromID:    chunk.ID,
				ToID:      sourceUnitID,
				FromLevel: chunk.Level,
				ToLevel:   -1,
			})
		}
	}
	return relations
}

var _ chunking.Strategy = (*StructureAwareStrategy)(nil)
