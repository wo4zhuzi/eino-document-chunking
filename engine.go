package chunking

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
	"github.com/wo4zhuzi/eino-document-chunking/internal/metadatautil"
)

// EngineConfig configures the format adapter, strategy, profile, and ID generator.
type EngineConfig struct {
	Profile     Profile
	Adapter     FormatAdapter
	Strategy    Strategy
	IDGenerator IDGenerator
}

// Engine is the concurrency-safe, stateless chunking entry point.
type Engine struct {
	profile     Profile
	adapter     FormatAdapter
	strategy    Strategy
	idGenerator IDGenerator
}

// NewEngine validates and freezes an engine configuration.
func NewEngine(config EngineConfig) (*Engine, error) {
	profile := Profile{
		Name:    strings.TrimSpace(config.Profile.Name),
		Version: strings.TrimSpace(config.Profile.Version),
	}
	if profile.Name == "" || profile.Version == "" {
		return nil, fmt.Errorf("%w: name and version are required", ErrInvalidProfile)
	}
	if config.Adapter == nil {
		return nil, fmt.Errorf("%w: adapter is required", ErrInvalidConfig)
	}
	if config.Strategy == nil {
		return nil, fmt.Errorf("%w: strategy is required", ErrInvalidConfig)
	}
	if strings.TrimSpace(config.Adapter.Name()) == "" {
		return nil, fmt.Errorf("%w: adapter name is required", ErrInvalidConfig)
	}
	if strings.TrimSpace(config.Strategy.Name()) == "" {
		return nil, fmt.Errorf("%w: strategy name is required", ErrInvalidConfig)
	}
	idGenerator := config.IDGenerator
	if idGenerator == nil {
		idGenerator = NewSHA256IDGenerator()
	}
	return &Engine{
		profile:     profile,
		adapter:     config.Adapter,
		strategy:    config.Strategy,
		idGenerator: idGenerator,
	}, nil
}

// Chunk converts documents into a complete validated Result.
func (engine *Engine) Chunk(ctx context.Context, documents []*schema.Document) (*Result, error) {
	if engine == nil || engine.adapter == nil || engine.strategy == nil || engine.idGenerator == nil {
		return nil, ErrEngineUnavailable
	}
	if err := contextError(ctx, "chunk documents"); err != nil {
		return nil, err
	}
	inputDocuments := cloneDocuments(documents)
	blocks, err := engine.adapter.Adapt(ctx, inputDocuments)
	if err != nil {
		return nil, fmt.Errorf("%w: adapter=%q: %w", ErrAdapterFailed, engine.adapter.Name(), err)
	}
	blocks, err = normalizeBlocks(blocks)
	if err != nil {
		return nil, err
	}
	if err := contextError(ctx, "run chunk strategy"); err != nil {
		return nil, err
	}
	output, err := engine.strategy.Chunk(ctx, StrategyInput{
		Profile:     engine.profile,
		Blocks:      cloneBlocks(blocks),
		IDGenerator: engine.idGenerator,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: strategy=%q: %w", ErrStrategyFailed, engine.strategy.Name(), err)
	}
	if output == nil || len(output.Chunks) == 0 {
		return nil, ErrNoValidChunks
	}
	chunks := cloneChunks(output.Chunks)
	for i := range chunks {
		chunks[i].CharacterCount = utf8.RuneCountInString(chunks[i].Content)
		if err := decorateChunkMetadata(
			&chunks[i],
			engine.profile,
			engine.adapter.Name(),
			engine.strategy.Name(),
		); err != nil {
			return nil, err
		}
	}
	relations := append([]Relation(nil), output.Relations...)
	if err := validateOutput(chunks, relations, blocks); err != nil {
		return nil, err
	}
	return &Result{
		Profile:      engine.profile,
		AdapterName:  engine.adapter.Name(),
		StrategyName: engine.strategy.Name(),
		Chunks:       chunks,
		Relations:    relations,
		Statistics:   buildStatistics(len(documents), len(blocks), chunks),
	}, nil
}

func normalizeBlocks(blocks []Block) ([]Block, error) {
	normalized := make([]Block, 0, len(blocks))
	seenIDs := make(map[string]struct{}, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block.Content) == "" {
			continue
		}
		block.ID = strings.TrimSpace(block.ID)
		block.DocumentID = strings.TrimSpace(block.DocumentID)
		if block.ID == "" || block.DocumentID == "" {
			return nil, fmt.Errorf("%w: block id and document id are required", ErrInvalidBlock)
		}
		if _, exists := seenIDs[block.ID]; exists {
			return nil, fmt.Errorf("%w: block id %q", ErrDuplicateID, block.ID)
		}
		seenIDs[block.ID] = struct{}{}
		if err := ensureNoReservedMetadata(block.Metadata); err != nil {
			return nil, fmt.Errorf("block id %q: %w", block.ID, err)
		}
		block.Metadata = metadatautil.Clone(block.Metadata)
		block.Sequence = len(normalized) + 1
		if len(block.SourceUnitIDs) == 0 {
			block.SourceUnitIDs = []string{block.ID}
		} else {
			block.SourceUnitIDs = append([]string(nil), block.SourceUnitIDs...)
		}
		for _, sourceUnitID := range block.SourceUnitIDs {
			if strings.TrimSpace(sourceUnitID) == "" {
				return nil, fmt.Errorf("%w: block %q contains an empty source unit id", ErrInvalidBlock, block.ID)
			}
		}
		normalized = append(normalized, block)
	}
	if len(normalized) == 0 {
		return nil, ErrNoValidBlocks
	}
	return normalized, nil
}

func decorateChunkMetadata(
	chunk *Chunk,
	profile Profile,
	adapterName string,
	strategyName string,
) error {
	if err := ensureNoReservedMetadata(chunk.Metadata); err != nil {
		return fmt.Errorf("chunk id %q: %w", chunk.ID, err)
	}
	metadata := metadatautil.Clone(chunk.Metadata)
	metadata[MetadataChunkID] = chunk.ID
	metadata[MetadataChunkKind] = string(chunk.Kind)
	metadata[MetadataChunkLevel] = chunk.Level
	metadata[MetadataDocumentID] = chunk.DocumentID
	if chunk.ParentID != "" {
		metadata[MetadataParentID] = chunk.ParentID
	}
	if chunk.PreviousID != "" {
		metadata[MetadataPreviousID] = chunk.PreviousID
	}
	if chunk.NextID != "" {
		metadata[MetadataNextID] = chunk.NextID
	}
	metadata[MetadataSourceUnitIDs] = append([]string(nil), chunk.SourceUnitIDs...)
	metadata[MetadataSequence] = chunk.Sequence
	metadata[MetadataCharacterCount] = chunk.CharacterCount
	metadata[MetadataTokenCount] = chunk.TokenCount
	metadata[MetadataProfileName] = profile.Name
	metadata[MetadataProfileVersion] = profile.Version
	metadata[MetadataStrategyName] = strategyName
	metadata[MetadataAdapterName] = adapterName
	chunk.Metadata = metadata
	return nil
}

func ensureNoReservedMetadata(metadata map[string]any) error {
	for _, key := range reservedMetadataKeys {
		if _, exists := metadata[key]; exists {
			return fmt.Errorf("%w: key %q is reserved", ErrMetadataConflict, key)
		}
	}
	return nil
}

func cloneChunks(chunks []Chunk) []Chunk {
	cloned := make([]Chunk, len(chunks))
	for i := range chunks {
		cloned[i] = chunks[i]
		cloned[i].SourceUnitIDs = append([]string(nil), chunks[i].SourceUnitIDs...)
		cloned[i].Metadata = metadatautil.Clone(chunks[i].Metadata)
	}
	return cloned
}

func buildStatistics(inputDocumentCount, blockCount int, chunks []Chunk) Statistics {
	statistics := Statistics{
		InputDocumentCount: inputDocumentCount,
		BlockCount:         blockCount,
		ChunkCount:         len(chunks),
	}
	for _, chunk := range chunks {
		statistics.CharacterCount += chunk.CharacterCount
		statistics.TokenCount += chunk.TokenCount
		switch chunk.Kind {
		case ChunkKindParent:
			statistics.ParentCount++
		case ChunkKindChild:
			statistics.ChildCount++
		}
	}
	return statistics
}

func contextError(ctx context.Context, operation string) error {
	if ctx == nil {
		return ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}
