package structureaware

import (
	"context"
	"fmt"

	chunking "github.com/wo4zhuzi/eino-document-chunking"
)

// DefaultMaxRunes is the default maximum size of one structure-aware chunk.
const DefaultMaxRunes = 1000

// HeadingContextMode controls whether semantic structure paths are included in chunk content.
type HeadingContextMode string

const (
	// HeadingContextPrepend writes the semantic structure path into chunk content.
	HeadingContextPrepend HeadingContextMode = "prepend"
	// HeadingContextMetadataOnly keeps the semantic structure path only in metadata.
	HeadingContextMetadataOnly HeadingContextMode = "metadata_only"
)

// OversizeSplitter splits one atomic structured block without changing its meaning.
type OversizeSplitter interface {
	Split(ctx context.Context, block chunking.Block, maxRunes int) ([]string, error)
}

// OversizeSplitterFunc adapts a function to OversizeSplitter.
type OversizeSplitterFunc func(context.Context, chunking.Block, int) ([]string, error)

// Split implements OversizeSplitter.
func (splitter OversizeSplitterFunc) Split(
	ctx context.Context,
	block chunking.Block,
	maxRunes int,
) ([]string, error) {
	if splitter == nil {
		return nil, fmt.Errorf("%w: oversize splitter function is nil", chunking.ErrInvalidConfig)
	}
	return splitter(ctx, block, maxRunes)
}

// StructureAwareConfig configures deterministic structure-aware assembly.
type StructureAwareConfig struct {
	MaxRunes         int
	MinRunes         int
	HeadingContext   HeadingContextMode
	OversizeSplitter OversizeSplitter
}

// StructureAwareStrategy assembles flat chunks without crossing structural boundaries.
type StructureAwareStrategy struct {
	maxRunes         int
	minRunes         int
	headingContext   HeadingContextMode
	oversizeSplitter OversizeSplitter
}

// NewStructureAwareStrategy creates a structure-aware strategy with bounded defaults.
func NewStructureAwareStrategy(config StructureAwareConfig) (*StructureAwareStrategy, error) {
	maxRunes := config.MaxRunes
	if maxRunes == 0 {
		maxRunes = DefaultMaxRunes
	}
	if maxRunes < 1 {
		return nil, fmt.Errorf("%w: structure-aware MaxRunes must be positive", chunking.ErrInvalidConfig)
	}
	minRunes := config.MinRunes
	if minRunes == 0 {
		minRunes = max(1, maxRunes/2)
	}
	if minRunes < 1 || minRunes > maxRunes {
		return nil, fmt.Errorf("%w: structure-aware MinRunes must be within [1, MaxRunes]", chunking.ErrInvalidConfig)
	}
	headingContext := config.HeadingContext
	if headingContext == "" {
		headingContext = HeadingContextPrepend
	}
	switch headingContext {
	case HeadingContextPrepend, HeadingContextMetadataOnly:
	default:
		return nil, fmt.Errorf("%w: unsupported heading context mode %q", chunking.ErrInvalidConfig, headingContext)
	}
	if splitter, ok := config.OversizeSplitter.(OversizeSplitterFunc); ok && splitter == nil {
		return nil, fmt.Errorf("%w: oversize splitter is nil", chunking.ErrInvalidConfig)
	}
	return &StructureAwareStrategy{
		maxRunes:         maxRunes,
		minRunes:         minRunes,
		headingContext:   headingContext,
		oversizeSplitter: config.OversizeSplitter,
	}, nil
}
