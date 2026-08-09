package adapter

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/cloudwego/eino/schema"
	chunking "github.com/wo4zhuzi/eino-document-chunking"
)

func TestStructuredDocumentAdapter(t *testing.T) {
	resolverPath := []string{"Guide", "Install"}
	resolverSemanticPath := []string{"指南", "安装"}
	adapter, err := NewStructuredDocumentAdapter(StructuredDocumentAdapterConfig{
		Resolver: StructureResolverFunc(func(_ context.Context, document *schema.Document) (*chunking.BlockStructure, error) {
			document.Content = "mutated"
			document.MetaData["nested"].(map[string]any)["owner"] = "mutated"
			return &chunking.BlockStructure{
				Kind:         chunking.BlockKindParagraph,
				Depth:        2,
				ParentID:     "heading-install",
				Path:         resolverPath,
				SemanticPath: resolverSemanticPath,
				Boundary:     chunking.BlockBoundarySoft,
			}, nil
		}),
	})
	if err != nil {
		t.Fatalf("NewStructuredDocumentAdapter() error = %v", err)
	}

	nested := map[string]any{"owner": "caller"}
	document := &schema.Document{
		ID:      "paragraph-1",
		Content: "Install the service.",
		MetaData: map[string]any{
			"document_id": "guide",
			"nested":      nested,
		},
	}
	blocks, err := adapter.Adapt(context.Background(), []*schema.Document{nil, {Content: " "}, document})
	if err != nil {
		t.Fatalf("Adapt() error = %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	block := blocks[0]
	if block.ID != "paragraph-1" || block.DocumentID != "guide" || block.Content != "Install the service." {
		t.Fatalf("block = %#v", block)
	}
	wantStructure := &chunking.BlockStructure{
		Kind:         chunking.BlockKindParagraph,
		Depth:        2,
		ParentID:     "heading-install",
		Path:         []string{"Guide", "Install"},
		SemanticPath: []string{"指南", "安装"},
		Boundary:     chunking.BlockBoundarySoft,
	}
	if !reflect.DeepEqual(block.Structure, wantStructure) {
		t.Fatalf("structure = %#v, want %#v", block.Structure, wantStructure)
	}
	if document.Content != "Install the service." || nested["owner"] != "caller" {
		t.Fatalf("resolver mutated caller document: %#v", document)
	}
	resolverPath[0] = "changed"
	resolverSemanticPath[0] = "changed"
	if block.Structure.Path[0] != "Guide" || block.Structure.SemanticPath[0] != "指南" {
		t.Fatalf("structure paths alias resolver output: %#v", block.Structure)
	}
}

func TestStructuredDocumentAdapterErrors(t *testing.T) {
	if _, err := NewStructuredDocumentAdapter(StructuredDocumentAdapterConfig{}); !errors.Is(err, chunking.ErrInvalidConfig) {
		t.Fatalf("NewStructuredDocumentAdapter() error = %v", err)
	}
	var nilResolver StructureResolverFunc
	if _, err := NewStructuredDocumentAdapter(StructuredDocumentAdapterConfig{Resolver: nilResolver}); !errors.Is(err, chunking.ErrInvalidConfig) {
		t.Fatalf("NewStructuredDocumentAdapter(nil function) error = %v", err)
	}

	var nilAdapter *StructuredDocumentAdapter
	if _, err := nilAdapter.Adapt(context.Background(), nil); !errors.Is(err, chunking.ErrInvalidConfig) {
		t.Fatalf("nil adapter error = %v", err)
	}

	missing, err := NewStructuredDocumentAdapter(StructuredDocumentAdapterConfig{
		Resolver: StructureResolverFunc(func(context.Context, *schema.Document) (*chunking.BlockStructure, error) {
			return nil, nil
		}),
	})
	if err != nil {
		t.Fatalf("NewStructuredDocumentAdapter() error = %v", err)
	}
	if _, err := missing.Adapt(context.Background(), []*schema.Document{{Content: "content"}}); !errors.Is(err, chunking.ErrStructureRequired) {
		t.Fatalf("missing structure error = %v", err)
	}

	sentinel := errors.New("resolver failed")
	failing, err := NewStructuredDocumentAdapter(StructuredDocumentAdapterConfig{
		Resolver: StructureResolverFunc(func(context.Context, *schema.Document) (*chunking.BlockStructure, error) {
			return nil, sentinel
		}),
	})
	if err != nil {
		t.Fatalf("NewStructuredDocumentAdapter() error = %v", err)
	}
	if _, err := failing.Adapt(context.Background(), []*schema.Document{{Content: "content"}}); !errors.Is(err, chunking.ErrStructureResolution) || !errors.Is(err, sentinel) {
		t.Fatalf("resolver error = %v", err)
	}
	if _, err := failing.Adapt(nil, nil); !errors.Is(err, chunking.ErrNilContext) {
		t.Fatalf("nil context error = %v", err)
	}
}
