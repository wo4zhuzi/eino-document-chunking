package adapter

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/cloudwego/eino/schema"
	chunking "github.com/wo4zhuzi/eino-document-chunking"
	ingestion "github.com/wo4zhuzi/eino-document-ingestion"
)

func TestIngestionAdapterUnstructuredOutput(t *testing.T) {
	formatAdapter, err := NewIngestionAdapter(ingestion.ParserInfo{
		Output: ingestion.ParserOutput{Granularity: ingestion.GranularityPage},
	})
	if err != nil {
		t.Fatalf("NewIngestionAdapter() error = %v", err)
	}
	document := &schema.Document{
		Content: "page content",
		MetaData: map[string]any{
			"_source":                       "guide.pdf",
			"page_number":                   1,
			ingestion.MetadataStructureKind: "ignored",
		},
	}
	blocks, err := formatAdapter.Adapt(context.Background(), []*schema.Document{document})
	if err != nil {
		t.Fatalf("Adapt() error = %v", err)
	}
	if len(blocks) != 1 || blocks[0].DocumentID != "guide.pdf" || blocks[0].Structure != nil {
		t.Fatalf("blocks = %#v", blocks)
	}
	if blocks[0].Metadata["page_number"] != 1 {
		t.Fatalf("page_number = %#v", blocks[0].Metadata["page_number"])
	}
}

func TestIngestionAdapterStructuredOutput(t *testing.T) {
	formatAdapter, err := NewIngestionAdapter(ingestion.ParserInfo{
		Output: ingestion.ParserOutput{
			Granularity: ingestion.GranularityBlock,
			Structured:  true,
		},
	})
	if err != nil {
		t.Fatalf("NewIngestionAdapter() error = %v", err)
	}
	headingPath := []string{"guide", "install"}
	paragraphPath := []string{"guide", "install", "install-body"}
	documents := []*schema.Document{
		{
			ID:      "install",
			Content: "Install",
			MetaData: map[string]any{
				"_source":                           "guide.md",
				ingestion.MetadataStructureKind:     "heading",
				ingestion.MetadataStructureDepth:    int64(1),
				ingestion.MetadataStructureParentID: "guide",
				ingestion.MetadataStructurePath:     headingPath,
				ingestion.MetadataStructureBoundary: "hard",
				ingestion.MetadataStructureLabel:    " Install ",
			},
		},
		{
			ID:      "install-body",
			Content: "Install the service.",
			MetaData: map[string]any{
				"_source":                           "guide.md",
				ingestion.MetadataStructureKind:     "paragraph",
				ingestion.MetadataStructureDepth:    int64(2),
				ingestion.MetadataStructureParentID: "install",
				ingestion.MetadataStructurePath:     paragraphPath,
				ingestion.MetadataStructureBoundary: "soft",
			},
		},
	}
	blocks, err := formatAdapter.Adapt(context.Background(), documents)
	if err != nil {
		t.Fatalf("Adapt() error = %v", err)
	}
	want := []*chunking.BlockStructure{
		{
			Kind:         chunking.BlockKindHeading,
			Depth:        1,
			ParentID:     "guide",
			Path:         []string{"guide", "install"},
			SemanticPath: []string{"Install"},
			Boundary:     chunking.BlockBoundaryHard,
		},
		{
			Kind:         chunking.BlockKindParagraph,
			Depth:        2,
			ParentID:     "install",
			Path:         []string{"guide", "install"},
			SemanticPath: []string{"Install"},
			Boundary:     chunking.BlockBoundarySoft,
		},
	}
	if len(blocks) != len(want) {
		t.Fatalf("blocks = %#v", blocks)
	}
	for index := range want {
		if !reflect.DeepEqual(blocks[index].Structure, want[index]) {
			t.Fatalf("structure[%d] = %#v, want %#v", index, blocks[index].Structure, want[index])
		}
	}
	if !reflect.DeepEqual(blocks[1].Metadata[ingestion.MetadataStructurePath], paragraphPath) {
		t.Fatalf("original node path was not preserved: %#v", blocks[1].Metadata)
	}
	headingPath[0] = "changed"
	paragraphPath[0] = "changed"
	if blocks[0].Structure.Path[0] != "guide" || blocks[1].Structure.Path[0] != "guide" {
		t.Fatalf("structure path aliases input: %#v %#v", blocks[0].Structure.Path, blocks[1].Structure.Path)
	}
}

func TestIngestionAdapterErrors(t *testing.T) {
	tests := []ingestion.ParserOutput{
		{Granularity: "unknown"},
		{Granularity: ingestion.GranularityPage, Structured: true},
	}
	for _, output := range tests {
		if _, err := NewIngestionAdapter(ingestion.ParserInfo{Output: output}); !errors.Is(err, chunking.ErrInvalidConfig) {
			t.Fatalf("NewIngestionAdapter(%#v) error = %v", output, err)
		}
	}

	var nilAdapter *IngestionAdapter
	if _, err := nilAdapter.Adapt(context.Background(), nil); !errors.Is(err, chunking.ErrInvalidConfig) {
		t.Fatalf("nil adapter error = %v", err)
	}

	formatAdapter, err := NewIngestionAdapter(ingestion.ParserInfo{
		Output: ingestion.ParserOutput{Granularity: ingestion.GranularityBlock, Structured: true},
	})
	if err != nil {
		t.Fatalf("NewIngestionAdapter() error = %v", err)
	}
	_, err = formatAdapter.Adapt(context.Background(), []*schema.Document{{
		ID:       "missing-structure",
		Content:  "content",
		MetaData: map[string]any{"_source": "guide.md"},
	}})
	if !errors.Is(err, chunking.ErrStructureResolution) {
		t.Fatalf("missing structure error = %v", err)
	}
}
