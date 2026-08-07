package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"strings"

	"github.com/cloudwego/eino/components/document/parser"
	"github.com/cloudwego/eino/schema"
	chunking "github.com/wo4zhuzi/eino-document-chunking"
	"github.com/wo4zhuzi/eino-document-chunking/adapter"
	"github.com/wo4zhuzi/eino-document-chunking/strategy/structureaware"
)

const (
	metadataStructureKind     = "example.structure.kind"
	metadataStructureDepth    = "example.structure.depth"
	metadataStructureParentID = "example.structure.parent_id"
	metadataStructurePath     = "example.structure.path"
)

func main() {
	ctx := context.Background()
	documents, err := (headingMarkdownParser{}).Parse(
		ctx,
		strings.NewReader("# 安装\n下载发布包并初始化配置文件。\n\n# 运行\n启动服务后检查健康检查接口和日志输出。\n"),
		parser.WithURI("memory://guide.md"),
	)
	if err != nil {
		panic(fmt.Errorf("parse markdown: %w", err))
	}

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

	result, err := engine.Chunk(ctx, documents)
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
	kind, ok := metadata[metadataStructureKind].(string)
	if !ok || kind == "" {
		return nil, fmt.Errorf("document %q has no structure kind", document.ID)
	}
	depth, ok := metadata[metadataStructureDepth].(int)
	if !ok {
		return nil, fmt.Errorf("document %q has no structure depth", document.ID)
	}
	parentID, _ := metadata[metadataStructureParentID].(string)
	path, ok := metadata[metadataStructurePath].([]string)
	if !ok || len(path) == 0 {
		return nil, fmt.Errorf("document %q has no structure path", document.ID)
	}
	return &chunking.BlockStructure{
		Kind:     chunking.BlockKind(kind),
		Depth:    depth,
		ParentID: parentID,
		Path:     append([]string(nil), path...),
	}, nil
}

type headingMarkdownParser struct{}

func (headingMarkdownParser) Parse(
	ctx context.Context,
	reader io.Reader,
	opts ...parser.Option,
) ([]*schema.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("parse markdown: %w", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read markdown: %w", err)
	}
	options := parser.GetCommonOptions(&parser.Options{}, opts...)

	var documents []*schema.Document
	var headingID string
	var path []string
	for _, line := range strings.Split(string(data), "\n") {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("parse markdown: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		unitID := fmt.Sprintf("unit-%d", len(documents)+1)
		metadata := maps.Clone(options.ExtraMeta)
		if metadata == nil {
			metadata = make(map[string]any)
		}
		if strings.TrimSpace(options.URI) != "" {
			metadata["_source"] = strings.TrimSpace(options.URI)
		}

		if strings.HasPrefix(line, "# ") {
			title := strings.TrimSpace(strings.TrimPrefix(line, "# "))
			if title == "" {
				return nil, fmt.Errorf("heading %q has no title", unitID)
			}
			headingID = unitID
			path = []string{title}
			metadata[metadataStructureKind] = string(chunking.BlockKindHeading)
			metadata[metadataStructureDepth] = 0
			metadata[metadataStructurePath] = append([]string(nil), path...)
			documents = append(documents, &schema.Document{
				ID:       unitID,
				Content:  title,
				MetaData: metadata,
			})
			continue
		}

		if headingID == "" {
			return nil, fmt.Errorf("paragraph %q appears before the first heading", unitID)
		}
		metadata[metadataStructureKind] = string(chunking.BlockKindParagraph)
		metadata[metadataStructureDepth] = 1
		metadata[metadataStructureParentID] = headingID
		metadata[metadataStructurePath] = append([]string(nil), path...)
		documents = append(documents, &schema.Document{
			ID:       unitID,
			Content:  line,
			MetaData: metadata,
		})
	}
	if len(documents) == 0 {
		return nil, fmt.Errorf("markdown contains no structured content")
	}
	return documents, nil
}

var _ parser.Parser = headingMarkdownParser{}
