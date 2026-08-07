package parentchild

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
	chunking "github.com/wo4zhuzi/eino-document-chunking"
	"github.com/wo4zhuzi/eino-document-chunking/internal/metadatautil"
)

const ParentChildStrategyName = "parent_child"

// ParentChildConfig configures the parent-child strategy.
type ParentChildConfig struct {
	ParentBuilder    ParentBuilder
	ChildSplitter    ChildSplitter
	ChildTransformer document.Transformer
}

// ParentChildStrategy builds bounded parent chunks and searchable child chunks.
type ParentChildStrategy struct {
	parentBuilder ParentBuilder
	childSplitter ChildSplitter
}

// NewParentChildStrategy creates a parent-child strategy with safe defaults.
func NewParentChildStrategy(config ParentChildConfig) (*ParentChildStrategy, error) {
	if config.ChildSplitter != nil && config.ChildTransformer != nil {
		return nil, fmt.Errorf("%w: configure either ChildSplitter or ChildTransformer", chunking.ErrInvalidConfig)
	}
	parentBuilder := config.ParentBuilder
	if parentBuilder == nil {
		var err error
		parentBuilder, err = NewBoundedParentBuilder(BoundedParentBuilderConfig{})
		if err != nil {
			return nil, err
		}
	}
	childSplitter := config.ChildSplitter
	if config.ChildTransformer != nil {
		var err error
		childSplitter, err = NewTransformerChildSplitter(config.ChildTransformer)
		if err != nil {
			return nil, err
		}
	}
	if childSplitter == nil {
		var err error
		childSplitter, err = NewBoundedTextSplitter(BoundedTextSplitterConfig{})
		if err != nil {
			return nil, err
		}
	}
	return &ParentChildStrategy{
		parentBuilder: parentBuilder,
		childSplitter: childSplitter,
	}, nil
}

// Name implements chunking.Strategy.
func (*ParentChildStrategy) Name() string {
	return ParentChildStrategyName
}

// Chunk implements chunking.Strategy.
func (strategy *ParentChildStrategy) Chunk(ctx context.Context, input chunking.StrategyInput) (*chunking.StrategyOutput, error) {
	if strategy == nil || strategy.parentBuilder == nil || strategy.childSplitter == nil {
		return nil, fmt.Errorf("%w: parent-child strategy is unavailable", chunking.ErrInvalidConfig)
	}
	if input.IDGenerator == nil {
		return nil, fmt.Errorf("%w: id generator is required", chunking.ErrInvalidConfig)
	}
	if err := contextError(ctx, "run parent-child strategy"); err != nil {
		return nil, err
	}
	parents, err := strategy.parentBuilder.Build(ctx, cloneBlocks(input.Blocks))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", chunking.ErrParentBuilderFailed, err)
	}
	if len(parents) == 0 {
		return nil, chunking.ErrNoValidChunks
	}

	chunks := make([]chunking.Chunk, 0, len(parents)*2)
	seenIDs := make(map[string]struct{}, len(parents)*2)
	lastParentIndexByDocument := make(map[string]int)
	for parentIndex, parent := range parents {
		if err := validateParentDraft(parentIndex, parent); err != nil {
			return nil, err
		}
		if err := contextError(ctx, "build parent-child chunks"); err != nil {
			return nil, err
		}
		parentSequence := len(chunks) + 1
		parentID, err := generateChunkID(ctx, input, chunking.IDInput{
			Profile:       input.Profile,
			StrategyName:  strategy.Name(),
			Kind:          chunking.ChunkKindParent,
			Level:         0,
			DocumentID:    parent.DocumentID,
			Sequence:      parentSequence,
			Content:       parent.Content,
			SourceUnitIDs: parent.SourceUnitIDs,
		})
		if err != nil {
			return nil, err
		}
		if err := recordGeneratedID(seenIDs, parentID); err != nil {
			return nil, err
		}
		parentChunk := chunking.Chunk{
			ID:            parentID,
			Kind:          chunking.ChunkKindParent,
			Content:       parent.Content,
			DocumentID:    parent.DocumentID,
			Level:         0,
			SourceUnitIDs: append([]string(nil), parent.SourceUnitIDs...),
			Sequence:      parentSequence,
			Metadata:      metadatautil.Clone(parent.Metadata),
		}
		if previousIndex, exists := lastParentIndexByDocument[parent.DocumentID]; exists {
			parentChunk.PreviousID = chunks[previousIndex].ID
			chunks[previousIndex].NextID = parentChunk.ID
		}
		parentChunkIndex := len(chunks)
		chunks = append(chunks, parentChunk)
		lastParentIndexByDocument[parent.DocumentID] = parentChunkIndex

		childDocuments, err := strategy.childSplitter.Split(ctx, &schema.Document{
			ID:       parentID,
			Content:  parent.Content,
			MetaData: metadatautil.Clone(parent.Metadata),
		})
		if err != nil {
			return nil, fmt.Errorf("%w: parent=%q: %w", chunking.ErrSplitterFailed, parentID, err)
		}
		previousChildIndex := -1
		childCount := 0
		for childIndex, childDocument := range childDocuments {
			if err := contextError(ctx, "build child chunks"); err != nil {
				return nil, err
			}
			if childDocument == nil || strings.TrimSpace(childDocument.Content) == "" {
				continue
			}
			childSequence := len(chunks) + 1
			childID, err := generateChunkID(ctx, input, chunking.IDInput{
				Profile:       input.Profile,
				StrategyName:  strategy.Name(),
				Kind:          chunking.ChunkKindChild,
				Level:         1,
				DocumentID:    parent.DocumentID,
				ParentID:      parentID,
				Sequence:      childSequence,
				Content:       childDocument.Content,
				SourceUnitIDs: parent.SourceUnitIDs,
			})
			if err != nil {
				return nil, fmt.Errorf("child index %d: %w", childIndex, err)
			}
			if err := recordGeneratedID(seenIDs, childID); err != nil {
				return nil, err
			}
			childChunk := chunking.Chunk{
				ID:            childID,
				Kind:          chunking.ChunkKindChild,
				Content:       childDocument.Content,
				DocumentID:    parent.DocumentID,
				Level:         1,
				ParentID:      parentID,
				SourceUnitIDs: append([]string(nil), parent.SourceUnitIDs...),
				Sequence:      childSequence,
				Metadata:      metadatautil.MergePreserving(parent.Metadata, childDocument.MetaData),
			}
			if previousChildIndex >= 0 {
				childChunk.PreviousID = chunks[previousChildIndex].ID
				chunks[previousChildIndex].NextID = childChunk.ID
			}
			previousChildIndex = len(chunks)
			chunks = append(chunks, childChunk)
			childCount++
		}
		if childCount == 0 {
			return nil, fmt.Errorf("%w: parent %q produced no child chunks", chunking.ErrNoValidChunks, parentID)
		}
	}
	return &chunking.StrategyOutput{
		Chunks:    chunks,
		Relations: buildRelations(chunks),
	}, nil
}

func validateParentDraft(index int, parent ParentDraft) error {
	if strings.TrimSpace(parent.DocumentID) == "" || strings.TrimSpace(parent.Content) == "" {
		return fmt.Errorf("%w: parent draft at index %d has empty document id or content", chunking.ErrInvalidBlock, index)
	}
	if len(parent.SourceUnitIDs) == 0 {
		return fmt.Errorf("%w: parent draft at index %d has no source units", chunking.ErrInvalidBlock, index)
	}
	for _, sourceUnitID := range parent.SourceUnitIDs {
		if strings.TrimSpace(sourceUnitID) == "" {
			return fmt.Errorf("%w: parent draft at index %d has an empty source unit id", chunking.ErrInvalidBlock, index)
		}
	}
	return nil
}

func generateChunkID(ctx context.Context, input chunking.StrategyInput, idInput chunking.IDInput) (string, error) {
	id, err := input.IDGenerator.Generate(ctx, idInput)
	if err != nil {
		return "", fmt.Errorf("%w: %w", chunking.ErrIDGenerationFailed, err)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("%w: generator returned an empty id", chunking.ErrIDGenerationFailed)
	}
	return id, nil
}

func recordGeneratedID(seen map[string]struct{}, id string) error {
	if _, exists := seen[id]; exists {
		return fmt.Errorf("%w: chunk id %q", chunking.ErrDuplicateID, id)
	}
	seen[id] = struct{}{}
	return nil
}

func buildRelations(chunks []chunking.Chunk) []chunking.Relation {
	relations := make([]chunking.Relation, 0, len(chunks)*2)
	chunkByID := make(map[string]chunking.Chunk, len(chunks))
	for _, chunk := range chunks {
		chunkByID[chunk.ID] = chunk
	}
	for _, chunk := range chunks {
		if chunk.ParentID != "" {
			parent := chunkByID[chunk.ParentID]
			relations = append(relations, chunking.Relation{
				Type:      chunking.RelationTypeParentChild,
				FromID:    parent.ID,
				ToID:      chunk.ID,
				FromLevel: parent.Level,
				ToLevel:   chunk.Level,
			})
		}
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

func cloneBlocks(blocks []chunking.Block) []chunking.Block {
	cloned := make([]chunking.Block, len(blocks))
	for i := range blocks {
		cloned[i] = blocks[i]
		cloned[i].SourceUnitIDs = append([]string(nil), blocks[i].SourceUnitIDs...)
		cloned[i].Metadata = metadatautil.Clone(blocks[i].Metadata)
	}
	return cloned
}

var _ chunking.Strategy = (*ParentChildStrategy)(nil)
