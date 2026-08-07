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
	path := []string{"Guide", "Install"}
	document := &schema.Document{
		ID:      "install-body",
		Content: "Install the service.",
		MetaData: map[string]any{
			"_source":                           "guide.md",
			ingestion.MetadataStructureKind:     "paragraph",
			ingestion.MetadataStructureDepth:    int64(2),
			ingestion.MetadataStructureParentID: "install",
			ingestion.MetadataStructurePath:     path,
			ingestion.MetadataStructureBoundary: "soft",
		},
	}
	blocks, err := formatAdapter.Adapt(context.Background(), []*schema.Document{document})
	if err != nil {
		t.Fatalf("Adapt() error = %v", err)
	}
	want := &chunking.BlockStructure{
		Kind:     chunking.BlockKindParagraph,
		Depth:    2,
		ParentID: "install",
		Path:     []string{"Guide", "Install"},
		Boundary: chunking.BlockBoundarySoft,
	}
	if len(blocks) != 1 || !reflect.DeepEqual(blocks[0].Structure, want) {
		t.Fatalf("structure = %#v, want %#v", blocks[0].Structure, want)
	}
	path[0] = "changed"
	if blocks[0].Structure.Path[0] != "Guide" {
		t.Fatalf("structure path aliases input: %#v", blocks[0].Structure.Path)
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
