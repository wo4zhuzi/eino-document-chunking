package adapter

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
	chunking "github.com/wo4zhuzi/eino-document-chunking"
	"github.com/wo4zhuzi/eino-document-chunking/internal/metadatautil"
)

const structuredDocumentAdapterName = "structured_document"

// StructureResolver extracts format-neutral structure from one parsed document unit.
type StructureResolver interface {
	Resolve(ctx context.Context, document *schema.Document) (*chunking.BlockStructure, error)
}

// StructureResolverFunc adapts a function to StructureResolver.
type StructureResolverFunc func(context.Context, *schema.Document) (*chunking.BlockStructure, error)

// Resolve implements StructureResolver.
func (resolver StructureResolverFunc) Resolve(
	ctx context.Context,
	document *schema.Document,
) (*chunking.BlockStructure, error) {
	if resolver == nil {
		return nil, fmt.Errorf("%w: structure resolver function is nil", chunking.ErrInvalidConfig)
	}
	return resolver(ctx, document)
}

// StructuredDocumentAdapterConfig configures structure extraction.
type StructuredDocumentAdapterConfig struct {
	Resolver StructureResolver
}

// StructuredDocumentAdapter converts parsed document units into structured blocks.
type StructuredDocumentAdapter struct {
	resolver StructureResolver
}

// NewStructuredDocumentAdapter creates an adapter with an explicit resolver.
func NewStructuredDocumentAdapter(config StructuredDocumentAdapterConfig) (*StructuredDocumentAdapter, error) {
	if config.Resolver == nil {
		return nil, fmt.Errorf("%w: structure resolver is required", chunking.ErrInvalidConfig)
	}
	if resolver, ok := config.Resolver.(StructureResolverFunc); ok && resolver == nil {
		return nil, fmt.Errorf("%w: structure resolver is required", chunking.ErrInvalidConfig)
	}
	return &StructuredDocumentAdapter{resolver: config.Resolver}, nil
}

// Name implements chunking.FormatAdapter.
func (*StructuredDocumentAdapter) Name() string {
	return structuredDocumentAdapterName
}

// Adapt implements chunking.FormatAdapter without modifying supplied documents.
func (adapter *StructuredDocumentAdapter) Adapt(
	ctx context.Context,
	documents []*schema.Document,
) ([]chunking.Block, error) {
	if adapter == nil || adapter.resolver == nil {
		return nil, fmt.Errorf("%w: structured document adapter is unavailable", chunking.ErrInvalidConfig)
	}
	if err := contextError(ctx, "adapt structured documents"); err != nil {
		return nil, err
	}
	blocks := make([]chunking.Block, 0, len(documents))
	for inputIndex, document := range documents {
		if err := contextError(ctx, "adapt structured documents"); err != nil {
			return nil, err
		}
		if document == nil || strings.TrimSpace(document.Content) == "" {
			continue
		}

		metadata := metadatautil.Clone(document.MetaData)
		documentID := resolveDocumentID(inputIndex, document, metadata)
		unitID := strings.TrimSpace(document.ID)
		if unitID == "" {
			unitID = stableUnitID(documentID, inputIndex, document.Content)
		}
		structure, err := adapter.resolver.Resolve(ctx, cloneDocument(document))
		if err != nil {
			return nil, fmt.Errorf(
				"%w: document index %d: %w",
				chunking.ErrStructureResolution,
				inputIndex,
				err,
			)
		}
		if structure == nil {
			return nil, fmt.Errorf("%w: document index %d", chunking.ErrStructureRequired, inputIndex)
		}

		blocks = append(blocks, chunking.Block{
			ID:            unitID,
			DocumentID:    documentID,
			Content:       document.Content,
			Sequence:      len(blocks) + 1,
			SourceUnitIDs: []string{unitID},
			Metadata:      metadata,
			Structure:     cloneStructure(structure),
		})
	}
	return blocks, nil
}

func cloneDocument(document *schema.Document) *schema.Document {
	if document == nil {
		return nil
	}
	return &schema.Document{
		ID:       document.ID,
		Content:  document.Content,
		MetaData: metadatautil.Clone(document.MetaData),
	}
}

func cloneStructure(structure *chunking.BlockStructure) *chunking.BlockStructure {
	if structure == nil {
		return nil
	}
	cloned := *structure
	cloned.Path = append([]string(nil), structure.Path...)
	return &cloned
}

var _ chunking.FormatAdapter = (*StructuredDocumentAdapter)(nil)
