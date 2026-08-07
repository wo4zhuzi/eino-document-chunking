package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cloudwego/eino/schema"
	chunking "github.com/wo4zhuzi/eino-document-chunking"
	"github.com/wo4zhuzi/eino-document-chunking/adapter"
	"github.com/wo4zhuzi/eino-document-chunking/strategy/parentchild"
)

func main() {
	parentBuilder, err := parentchild.NewBoundedParentBuilder(parentchild.BoundedParentBuilderConfig{
		MaxRunes: 160,
	})
	if err != nil {
		panic(fmt.Errorf("create parent builder: %w", err))
	}
	childSplitter, err := parentchild.NewBoundedTextSplitter(parentchild.BoundedTextSplitterConfig{
		MaxRunes: 64,
	})
	if err != nil {
		panic(fmt.Errorf("create child splitter: %w", err))
	}
	strategy, err := parentchild.NewParentChildStrategy(parentchild.ParentChildConfig{
		ParentBuilder: parentBuilder,
		ChildSplitter: childSplitter,
	})
	if err != nil {
		panic(fmt.Errorf("create parent-child strategy: %w", err))
	}
	engine, err := chunking.NewEngine(chunking.EngineConfig{
		Profile:  chunking.Profile{Name: "offline-demo", Version: "v1"},
		Adapter:  adapter.NewDocumentAdapter(),
		Strategy: strategy,
	})
	if err != nil {
		panic(fmt.Errorf("create chunking engine: %w", err))
	}

	documents := []*schema.Document{
		{
			ID:      "section-1",
			Content: "Eino uses schema.Document as the shared document model. Chunking runs after loading and parsing, and before embedding and indexing.",
			MetaData: map[string]any{
				"_source": "memory://eino-guide.md",
				"section": "overview",
			},
		},
		{
			ID:      "section-2",
			Content: "Parent chunks retain reading context. Child chunks are smaller retrieval units and always point to one parent chunk.",
			MetaData: map[string]any{
				"_source": "memory://eino-guide.md",
				"section": "parent-child",
			},
		},
	}

	result, err := engine.Chunk(context.Background(), documents)
	if err != nil {
		panic(fmt.Errorf("chunk documents: %w", err))
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		panic(fmt.Errorf("encode result: %w", err))
	}
}
