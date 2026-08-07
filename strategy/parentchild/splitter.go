package parentchild

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
	chunking "github.com/wo4zhuzi/eino-document-chunking"
	"github.com/wo4zhuzi/eino-document-chunking/internal/metadatautil"
	"github.com/wo4zhuzi/eino-document-chunking/internal/textutil"
)

const DefaultMaxChildRunes = 500

// ChildSplitter splits one parent document into child documents.
type ChildSplitter interface {
	Split(ctx context.Context, parent *schema.Document) ([]*schema.Document, error)
}

// BoundedTextSplitterConfig configures the offline default child splitter.
type BoundedTextSplitterConfig struct {
	MaxRunes int
}

// BoundedTextSplitter splits text at stable natural boundaries when possible.
type BoundedTextSplitter struct {
	maxRunes int
}

// NewBoundedTextSplitter creates the default child splitter.
func NewBoundedTextSplitter(config BoundedTextSplitterConfig) (*BoundedTextSplitter, error) {
	maxRunes := config.MaxRunes
	if maxRunes == 0 {
		maxRunes = DefaultMaxChildRunes
	}
	if maxRunes < 1 {
		return nil, fmt.Errorf("%w: child MaxRunes must be positive", chunking.ErrInvalidConfig)
	}
	return &BoundedTextSplitter{maxRunes: maxRunes}, nil
}

// Split implements ChildSplitter.
func (splitter *BoundedTextSplitter) Split(ctx context.Context, parent *schema.Document) ([]*schema.Document, error) {
	if splitter == nil || splitter.maxRunes < 1 {
		return nil, fmt.Errorf("%w: child splitter is unavailable", chunking.ErrInvalidConfig)
	}
	if err := contextError(ctx, "split parent"); err != nil {
		return nil, err
	}
	if parent == nil || strings.TrimSpace(parent.Content) == "" {
		return nil, nil
	}
	parts := textutil.SplitBounded(parent.Content, splitter.maxRunes)
	documents := make([]*schema.Document, 0, len(parts))
	for _, part := range parts {
		if err := contextError(ctx, "split parent"); err != nil {
			return nil, err
		}
		if strings.TrimSpace(part) == "" {
			continue
		}
		documents = append(documents, &schema.Document{
			ID:       parent.ID,
			Content:  part,
			MetaData: metadatautil.Clone(parent.MetaData),
		})
	}
	return documents, nil
}

// TransformerChildSplitter adapts an Eino Transformer to ChildSplitter.
type TransformerChildSplitter struct {
	transformer document.Transformer
}

// NewTransformerChildSplitter wraps an Eino document.Transformer.
func NewTransformerChildSplitter(transformer document.Transformer) (*TransformerChildSplitter, error) {
	if transformer == nil {
		return nil, fmt.Errorf("%w: child transformer is required", chunking.ErrInvalidConfig)
	}
	return &TransformerChildSplitter{transformer: transformer}, nil
}

// Split implements ChildSplitter.
func (splitter *TransformerChildSplitter) Split(ctx context.Context, parent *schema.Document) ([]*schema.Document, error) {
	if splitter == nil || splitter.transformer == nil {
		return nil, fmt.Errorf("%w: child transformer is unavailable", chunking.ErrInvalidConfig)
	}
	if err := contextError(ctx, "transform parent into children"); err != nil {
		return nil, err
	}
	documents, err := splitter.transformer.Transform(ctx, []*schema.Document{cloneDocument(parent)})
	if err != nil {
		return nil, fmt.Errorf("transform parent: %w", err)
	}
	return cloneDocuments(documents), nil
}

var (
	_ ChildSplitter = (*BoundedTextSplitter)(nil)
	_ ChildSplitter = (*TransformerChildSplitter)(nil)
)

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

func cloneDocuments(documents []*schema.Document) []*schema.Document {
	cloned := make([]*schema.Document, len(documents))
	for i, document := range documents {
		cloned[i] = cloneDocument(document)
	}
	return cloned
}
