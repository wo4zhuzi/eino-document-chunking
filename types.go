package chunking

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

// ChunkKind describes the semantic role of a chunk.
type ChunkKind string

const (
	ChunkKindParent ChunkKind = "parent"
	ChunkKindChild  ChunkKind = "child"
)

// Chunk is the strategy-neutral output model.
type Chunk struct {
	ID             string         `json:"id"`
	Kind           ChunkKind      `json:"kind"`
	Content        string         `json:"content"`
	DocumentID     string         `json:"document_id"`
	Level          int            `json:"level"`
	ParentID       string         `json:"parent_id,omitempty"`
	PreviousID     string         `json:"previous_id,omitempty"`
	NextID         string         `json:"next_id,omitempty"`
	SourceUnitIDs  []string       `json:"source_unit_ids"`
	Sequence       int            `json:"sequence"`
	CharacterCount int            `json:"character_count"`
	TokenCount     int            `json:"token_count,omitempty"`
	Metadata       map[string]any `json:"metadata"`
}

// RelationType identifies a relation represented in Result.Relations.
type RelationType string

const (
	RelationTypeParentChild  RelationType = "parent_child"
	RelationTypePreviousNext RelationType = "previous_next"
	RelationTypeSource       RelationType = "source"
)

// Relation describes hierarchy, adjacency, or provenance between IDs.
type Relation struct {
	Type      RelationType `json:"type"`
	FromID    string       `json:"from_id"`
	ToID      string       `json:"to_id"`
	FromLevel int          `json:"from_level"`
	ToLevel   int          `json:"to_level"`
}

// Statistics summarizes a completed chunking operation.
type Statistics struct {
	InputDocumentCount int `json:"input_document_count"`
	BlockCount         int `json:"block_count"`
	ChunkCount         int `json:"chunk_count"`
	ParentCount        int `json:"parent_count"`
	ChildCount         int `json:"child_count"`
	CharacterCount     int `json:"character_count"`
	TokenCount         int `json:"token_count"`
}

// Result is the complete deterministic output of Engine.Chunk.
type Result struct {
	Profile      Profile    `json:"profile"`
	AdapterName  string     `json:"adapter_name"`
	StrategyName string     `json:"strategy_name"`
	Chunks       []Chunk    `json:"chunks"`
	Relations    []Relation `json:"relations"`
	Statistics   Statistics `json:"statistics"`
}

// FormatAdapter converts Eino documents into format-neutral logical blocks.
type FormatAdapter interface {
	Name() string
	Adapt(ctx context.Context, documents []*schema.Document) ([]Block, error)
}

// StrategyInput contains immutable data supplied by Engine to a Strategy.
type StrategyInput struct {
	Profile     Profile
	Blocks      []Block
	IDGenerator IDGenerator
}

// StrategyOutput is the unvalidated output returned by a Strategy.
type StrategyOutput struct {
	Chunks    []Chunk
	Relations []Relation
}

// Strategy organizes logical blocks into chunks.
type Strategy interface {
	Name() string
	Chunk(ctx context.Context, input StrategyInput) (*StrategyOutput, error)
}
