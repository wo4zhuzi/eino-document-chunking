package chunking

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
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
		return nil, fmt.Errorf("%w: child MaxRunes must be positive", ErrInvalidConfig)
	}
	return &BoundedTextSplitter{maxRunes: maxRunes}, nil
}

// Split implements ChildSplitter.
func (splitter *BoundedTextSplitter) Split(ctx context.Context, parent *schema.Document) ([]*schema.Document, error) {
	if splitter == nil || splitter.maxRunes < 1 {
		return nil, fmt.Errorf("%w: child splitter is unavailable", ErrInvalidConfig)
	}
	if err := contextError(ctx, "split parent"); err != nil {
		return nil, err
	}
	if parent == nil || strings.TrimSpace(parent.Content) == "" {
		return nil, nil
	}
	parts := splitBoundedText(parent.Content, splitter.maxRunes)
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
			MetaData: cloneMetadata(parent.MetaData),
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
		return nil, fmt.Errorf("%w: child transformer is required", ErrInvalidConfig)
	}
	return &TransformerChildSplitter{transformer: transformer}, nil
}

// Split implements ChildSplitter.
func (splitter *TransformerChildSplitter) Split(ctx context.Context, parent *schema.Document) ([]*schema.Document, error) {
	if splitter == nil || splitter.transformer == nil {
		return nil, fmt.Errorf("%w: child transformer is unavailable", ErrInvalidConfig)
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

func splitBoundedText(content string, maxRunes int) []string {
	runes := []rune(strings.TrimSpace(content))
	if len(runes) == 0 || maxRunes < 1 {
		return nil
	}
	parts := make([]string, 0, (len(runes)+maxRunes-1)/maxRunes)
	for start := 0; start < len(runes); {
		end := start + maxRunes
		if end >= len(runes) {
			end = len(runes)
		} else {
			end = preferredBoundary(runes, start, end)
		}
		if end <= start {
			end = start + maxRunes
			if end > len(runes) {
				end = len(runes)
			}
		}
		part := strings.TrimSpace(string(runes[start:end]))
		if part != "" {
			parts = append(parts, part)
		}
		start = end
		for start < len(runes) && unicode.IsSpace(runes[start]) {
			start++
		}
	}
	return parts
}

func preferredBoundary(runes []rune, start, end int) int {
	for index := end - 1; index > start; index-- {
		if runes[index] == '\n' && runes[index-1] == '\n' {
			return index + 1
		}
	}
	for index := end - 1; index > start; index-- {
		if runes[index] == '\n' {
			return index + 1
		}
	}
	for index := end - 1; index > start; index-- {
		if unicode.IsSpace(runes[index]) {
			return index + 1
		}
	}
	return end
}

var (
	_ ChildSplitter = (*BoundedTextSplitter)(nil)
	_ ChildSplitter = (*TransformerChildSplitter)(nil)
)
