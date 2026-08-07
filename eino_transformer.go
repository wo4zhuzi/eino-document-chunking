package chunking

import (
	"context"
	"fmt"

	einodocument "github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

// TransformerOutput explicitly selects which chunks an Eino Transformer returns.
type TransformerOutput string

const (
	TransformerOutputParents  TransformerOutput = "parents"
	TransformerOutputChildren TransformerOutput = "children"
	TransformerOutputAll      TransformerOutput = "all"
)

// EinoTransformerConfig configures the explicit output projection.
type EinoTransformerConfig struct {
	Output TransformerOutput
}

// EinoTransformer adapts Engine to Eino's document.Transformer interface.
type EinoTransformer struct {
	engine *Engine
	output TransformerOutput
}

// NewEinoTransformer creates an adapter with an explicit output projection.
func NewEinoTransformer(engine *Engine, config EinoTransformerConfig) (*EinoTransformer, error) {
	if engine == nil {
		return nil, fmt.Errorf("%w: engine is required", ErrInvalidConfig)
	}
	switch config.Output {
	case TransformerOutputParents, TransformerOutputChildren, TransformerOutputAll:
	default:
		return nil, fmt.Errorf("%w: transformer output must be parents, children, or all", ErrInvalidConfig)
	}
	return &EinoTransformer{engine: engine, output: config.Output}, nil
}

// Transform implements Eino document.Transformer.
func (transformer *EinoTransformer) Transform(
	ctx context.Context,
	documents []*schema.Document,
	_ ...einodocument.TransformerOption,
) ([]*schema.Document, error) {
	if transformer == nil || transformer.engine == nil {
		return nil, ErrEngineUnavailable
	}
	result, err := transformer.engine.Chunk(ctx, documents)
	if err != nil {
		return nil, fmt.Errorf("transform documents with chunking engine: %w", err)
	}
	output := make([]*schema.Document, 0, len(result.Chunks))
	for _, chunk := range result.Chunks {
		if !transformer.selects(chunk.Kind) {
			continue
		}
		output = append(output, &schema.Document{
			ID:       chunk.ID,
			Content:  chunk.Content,
			MetaData: cloneMetadata(chunk.Metadata),
		})
	}
	return output, nil
}

func (transformer *EinoTransformer) selects(kind ChunkKind) bool {
	switch transformer.output {
	case TransformerOutputParents:
		return kind == ChunkKindParent
	case TransformerOutputChildren:
		return kind == ChunkKindChild
	case TransformerOutputAll:
		return true
	default:
		return false
	}
}

var _ einodocument.Transformer = (*EinoTransformer)(nil)
