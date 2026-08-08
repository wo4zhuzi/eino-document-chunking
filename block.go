package chunking

// Block is the format-neutral logical unit consumed by chunk strategies.
type Block struct {
	ID            string          `json:"id"`
	DocumentID    string          `json:"document_id"`
	Content       string          `json:"content"`
	Sequence      int             `json:"sequence"`
	SourceUnitIDs []string        `json:"source_unit_ids"`
	Metadata      map[string]any  `json:"metadata"`
	Structure     *BlockStructure `json:"structure,omitempty"`
}

// BlockKind identifies the format-neutral role of a logical block.
type BlockKind string

const (
	BlockKindText      BlockKind = "text"
	BlockKindHeading   BlockKind = "heading"
	BlockKindParagraph BlockKind = "paragraph"
	BlockKindListItem  BlockKind = "list_item"
	BlockKindCode      BlockKind = "code"
	BlockKindCodeBlock BlockKind = "code_block"
	BlockKindTable     BlockKind = "table"
	BlockKindQuote     BlockKind = "quote"
)

// BlockBoundary describes whether a block starts a structural boundary.
type BlockBoundary string

const (
	BlockBoundaryNone BlockBoundary = ""
	BlockBoundarySoft BlockBoundary = "soft"
	BlockBoundaryHard BlockBoundary = "hard"
)

// BlockStructure contains optional format-neutral structure information.
type BlockStructure struct {
	Kind     BlockKind     `json:"kind"`
	Depth    int           `json:"depth"`
	ParentID string        `json:"parent_id,omitempty"`
	Path     []string      `json:"path,omitempty"`
	Boundary BlockBoundary `json:"boundary,omitempty"`
}
