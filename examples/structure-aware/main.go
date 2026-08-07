package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cloudwego/eino/schema"
	chunking "github.com/wo4zhuzi/eino-document-chunking"
	"github.com/wo4zhuzi/eino-document-chunking/adapter"
	"github.com/wo4zhuzi/eino-document-chunking/strategy/structureaware"
)

func main() {
	formatAdapter, err := adapter.NewStructuredDocumentAdapter(adapter.StructuredDocumentAdapterConfig{
		Resolver: adapter.StructureResolverFunc(resolveStructure),
	})
	if err != nil {
		panic(fmt.Errorf("create structured adapter: %w", err))
	}
	strategy, err := structureaware.NewStructureAwareStrategy(structureaware.StructureAwareConfig{
		MaxRunes: 120,
		MinRunes: 60,
	})
	if err != nil {
		panic(fmt.Errorf("create structure-aware strategy: %w", err))
	}
	engine, err := chunking.NewEngine(chunking.EngineConfig{
		Profile:  chunking.Profile{Name: "structure-aware-demo", Version: "v1"},
		Adapter:  formatAdapter,
		Strategy: strategy,
	})
	if err != nil {
		panic(fmt.Errorf("create chunking engine: %w", err))
	}

	documents := []*schema.Document{
		structuredDocument("install", "安装", chunking.BlockKindHeading, 0, "", []string{"安装"}, chunking.BlockBoundaryHard),
		structuredDocument("install-body", "下载发布包并初始化配置文件。", chunking.BlockKindParagraph, 1, "install", []string{"安装"}, chunking.BlockBoundaryNone),
		structuredDocument("run", "运行", chunking.BlockKindHeading, 0, "", []string{"运行"}, chunking.BlockBoundaryHard),
		structuredDocument("run-body", "启动服务后检查健康检查接口和日志输出。", chunking.BlockKindParagraph, 1, "run", []string{"运行"}, chunking.BlockBoundaryNone),
	}
	result, err := engine.Chunk(context.Background(), documents)
	if err != nil {
		panic(fmt.Errorf("chunk structured documents: %w", err))
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		panic(fmt.Errorf("encode result: %w", err))
	}
}

func resolveStructure(
	_ context.Context,
	document *schema.Document,
) (*chunking.BlockStructure, error) {
	metadata := document.MetaData
	kind, ok := metadata["structure_kind"].(string)
	if !ok || kind == "" {
		return nil, fmt.Errorf("document %q has no structure kind", document.ID)
	}
	depth, ok := metadata["structure_depth"].(int)
	if !ok {
		return nil, fmt.Errorf("document %q has no structure depth", document.ID)
	}
	parentID, _ := metadata["structure_parent_id"].(string)
	path, _ := metadata["structure_path"].([]string)
	boundary, _ := metadata["structure_boundary"].(string)
	return &chunking.BlockStructure{
		Kind:     chunking.BlockKind(kind),
		Depth:    depth,
		ParentID: parentID,
		Path:     append([]string(nil), path...),
		Boundary: chunking.BlockBoundary(boundary),
	}, nil
}

func structuredDocument(
	id string,
	content string,
	kind chunking.BlockKind,
	depth int,
	parentID string,
	path []string,
	boundary chunking.BlockBoundary,
) *schema.Document {
	return &schema.Document{
		ID:      id,
		Content: content,
		MetaData: map[string]any{
			"document_id":         "memory://guide.md",
			"structure_kind":      string(kind),
			"structure_depth":     depth,
			"structure_parent_id": parentID,
			"structure_path":      append([]string(nil), path...),
			"structure_boundary":  string(boundary),
			"source":              "offline-example",
		},
	}
}
