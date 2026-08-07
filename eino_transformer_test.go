package chunking

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

func TestEinoTransformerRequiresExplicitOutputAndProjectsChunks(t *testing.T) {
	engine := newTestEngine(t, 100, 5)
	if _, err := NewEinoTransformer(nil, EinoTransformerConfig{Output: TransformerOutputAll}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("NewEinoTransformer(nil) error = %v", err)
	}
	if _, err := NewEinoTransformer(engine, EinoTransformerConfig{}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("NewEinoTransformer(implicit output) error = %v", err)
	}

	tests := []struct {
		name       string
		output     TransformerOutput
		wantCount  int
		wantKind   ChunkKind
		checkKinds bool
	}{
		{name: "parents", output: TransformerOutputParents, wantCount: 1, wantKind: ChunkKindParent, checkKinds: true},
		{name: "children", output: TransformerOutputChildren, wantCount: 2, wantKind: ChunkKindChild, checkKinds: true},
		{name: "all", output: TransformerOutputAll, wantCount: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transformer, err := NewEinoTransformer(engine, EinoTransformerConfig{Output: test.output})
			if err != nil {
				t.Fatalf("NewEinoTransformer() error = %v", err)
			}
			documents, err := transformer.Transform(context.Background(), []*schema.Document{{
				ID:       "unit",
				Content:  "abcdefghij",
				MetaData: map[string]any{"origin": "test"},
			}})
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			if len(documents) != test.wantCount {
				t.Fatalf("len(documents) = %d, want %d", len(documents), test.wantCount)
			}
			for _, transformed := range documents {
				if transformed.ID == "" || transformed.Content == "" || transformed.MetaData["origin"] != "test" {
					t.Fatalf("transformed = %#v", transformed)
				}
				if test.checkKinds && transformed.MetaData[MetadataChunkKind] != string(test.wantKind) {
					t.Fatalf("kind metadata = %#v", transformed.MetaData[MetadataChunkKind])
				}
				if test.wantKind == ChunkKindChild && transformed.MetaData[MetadataParentID] == "" {
					t.Fatalf("child parent metadata = %#v", transformed.MetaData)
				}
			}
		})
	}
}

func TestParentChildAcceptsEinoChildTransformer(t *testing.T) {
	strategy, err := NewParentChildStrategy(ParentChildConfig{
		ChildTransformer: &addingTransformer{},
	})
	if err != nil {
		t.Fatalf("NewParentChildStrategy() error = %v", err)
	}
	engine := newEngineForTest(t, NewDocumentAdapter(), strategy, nil)
	result, err := engine.Chunk(context.Background(), []*schema.Document{{
		ID:       "doc",
		Content:  "content",
		MetaData: map[string]any{"original": true},
	}})
	if err != nil {
		t.Fatalf("Chunk() error = %v", err)
	}
	children := 0
	for _, chunk := range result.Chunks {
		if chunk.Kind != ChunkKindChild {
			continue
		}
		children++
		if chunk.Metadata["original"] != true || chunk.Metadata["transformer"] != "added" {
			t.Fatalf("child metadata = %#v", chunk.Metadata)
		}
	}
	if children != 1 {
		t.Fatalf("children = %d, want 1", children)
	}

	sentinel := errors.New("transformer failed")
	strategy, err = NewParentChildStrategy(ParentChildConfig{
		ChildTransformer: &failingTransformer{err: sentinel},
	})
	if err != nil {
		t.Fatalf("NewParentChildStrategy() error = %v", err)
	}
	engine = newEngineForTest(t, NewDocumentAdapter(), strategy, nil)
	if _, err := engine.Chunk(context.Background(), []*schema.Document{{ID: "doc", Content: "content"}}); !errors.Is(err, ErrSplitterFailed) || !errors.Is(err, sentinel) {
		t.Fatalf("transformer error = %v", err)
	}
}

type addingTransformer struct{}

func (*addingTransformer) Transform(
	_ context.Context,
	documents []*schema.Document,
	_ ...document.TransformerOption,
) ([]*schema.Document, error) {
	metadata := cloneMetadata(documents[0].MetaData)
	metadata["transformer"] = "added"
	return []*schema.Document{{
		ID:       "ignored-by-chunking",
		Content:  documents[0].Content,
		MetaData: metadata,
	}}, nil
}

type failingTransformer struct {
	err error
}

func (transformer *failingTransformer) Transform(
	context.Context,
	[]*schema.Document,
	...document.TransformerOption,
) ([]*schema.Document, error) {
	return nil, transformer.err
}
