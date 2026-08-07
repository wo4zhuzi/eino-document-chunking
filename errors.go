package chunking

import "errors"

var (
	ErrNilContext          = errors.New("context is nil")
	ErrInvalidConfig       = errors.New("invalid chunking config")
	ErrEngineUnavailable   = errors.New("chunking engine is unavailable")
	ErrInvalidProfile      = errors.New("invalid chunking profile")
	ErrInvalidBlock        = errors.New("invalid logical block")
	ErrNoValidBlocks       = errors.New("no valid logical blocks")
	ErrNoValidChunks       = errors.New("no valid chunks")
	ErrDuplicateID         = errors.New("duplicate id")
	ErrInvalidRelation     = errors.New("invalid chunk relation")
	ErrMetadataConflict    = errors.New("chunking metadata key conflicts with input metadata")
	ErrAdapterFailed       = errors.New("format adapter failed")
	ErrStrategyFailed      = errors.New("chunk strategy failed")
	ErrParentBuilderFailed = errors.New("parent builder failed")
	ErrSplitterFailed      = errors.New("child splitter failed")
	ErrIDGenerationFailed  = errors.New("chunk id generation failed")
)
