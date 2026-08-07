package chunking

import (
	"context"
	"fmt"
	"strings"
)

const DefaultMaxParentRunes = 2000

// ParentDraft is a bounded parent candidate produced before IDs are assigned.
type ParentDraft struct {
	DocumentID    string
	Content       string
	SourceUnitIDs []string
	Metadata      map[string]any
}

// ParentBuilder constructs bounded parent candidates from logical blocks.
type ParentBuilder interface {
	Build(ctx context.Context, blocks []Block) ([]ParentDraft, error)
}

// BoundedParentBuilderConfig configures the default parent builder.
type BoundedParentBuilderConfig struct {
	MaxRunes int
}

// BoundedParentBuilder keeps every parent below a deterministic rune limit.
type BoundedParentBuilder struct {
	maxRunes int
}

// NewBoundedParentBuilder creates the default parent construction policy.
func NewBoundedParentBuilder(config BoundedParentBuilderConfig) (*BoundedParentBuilder, error) {
	maxRunes := config.MaxRunes
	if maxRunes == 0 {
		maxRunes = DefaultMaxParentRunes
	}
	if maxRunes < 1 {
		return nil, fmt.Errorf("%w: parent MaxRunes must be positive", ErrInvalidConfig)
	}
	return &BoundedParentBuilder{maxRunes: maxRunes}, nil
}

// Build implements ParentBuilder.
func (builder *BoundedParentBuilder) Build(ctx context.Context, blocks []Block) ([]ParentDraft, error) {
	if builder == nil || builder.maxRunes < 1 {
		return nil, fmt.Errorf("%w: parent builder is unavailable", ErrInvalidConfig)
	}
	if err := contextError(ctx, "build parents"); err != nil {
		return nil, err
	}
	parents := make([]ParentDraft, 0, len(blocks))
	for _, block := range blocks {
		if err := contextError(ctx, "build parents"); err != nil {
			return nil, err
		}
		for _, content := range splitBoundedText(block.Content, builder.maxRunes) {
			if strings.TrimSpace(content) == "" {
				continue
			}
			parents = append(parents, ParentDraft{
				DocumentID:    block.DocumentID,
				Content:       content,
				SourceUnitIDs: append([]string(nil), block.SourceUnitIDs...),
				Metadata:      cloneMetadata(block.Metadata),
			})
		}
	}
	return parents, nil
}

var _ ParentBuilder = (*BoundedParentBuilder)(nil)
