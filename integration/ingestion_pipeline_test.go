package integration_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/document/parser"
	"github.com/cloudwego/eino/schema"
	chunking "github.com/wo4zhuzi/eino-document-chunking"
	"github.com/wo4zhuzi/eino-document-chunking/adapter"
	"github.com/wo4zhuzi/eino-document-chunking/strategy/parentchild"
	"github.com/wo4zhuzi/eino-document-chunking/strategy/structureaware"
	ingestion "github.com/wo4zhuzi/eino-document-ingestion"
)

func TestIngestionLoaderParserWithChunkStrategies(t *testing.T) {
	ctx := context.Background()
	sourceURI := writeMarkdownFixture(t)
	ingestor := newMarkdownIngestor(t, ctx)

	ingested, err := ingestor.Ingest(ctx, sourceURI)
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if ingested.Parser.Name != "test_structured_markdown" || ingested.Parser.Version != "v1" {
		t.Fatalf("parser = %#v, want test_structured_markdown@v1", ingested.Parser)
	}
	if len(ingested.Documents) != 4 {
		t.Fatalf("ingested document count = %d, want 4", len(ingested.Documents))
	}
	for index, document := range ingested.Documents {
		if document.MetaData["_source"] != sourceURI {
			t.Fatalf("document %d source = %#v, want %q", index, document.MetaData["_source"], sourceURI)
		}
	}

	parentChildResult, err := newParentChildEngine(t, ingested.Parser).Chunk(ctx, ingested.Documents)
	if err != nil {
		t.Fatalf("parent-child Chunk() error = %v", err)
	}
	assertParentChildResult(t, parentChildResult, sourceURI)

	structureResult, err := newStructureAwareEngine(t, ingested.Parser).Chunk(ctx, ingested.Documents)
	if err != nil {
		t.Fatalf("structure-aware Chunk() error = %v", err)
	}
	assertStructureAwareResult(t, structureResult, sourceURI)
}

func newMarkdownIngestor(t *testing.T, ctx context.Context) *ingestion.Ingestor {
	t.Helper()
	registry := ingestion.NewRegistry()
	err := registry.Register(ingestion.Format{
		Extension:         ingestion.ExtensionMarkdown,
		MIMEType:          "text/markdown",
		DetectedMIMETypes: []string{"text/plain"},
		ParserInfo: ingestion.ParserInfo{
			Name:    "test_structured_markdown",
			Version: "v1",
			Output: ingestion.ParserOutput{
				Granularity: ingestion.GranularityBlock,
				Structured:  true,
			},
		},
		Parser: headingMarkdownParser{},
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	ingestor, err := ingestion.New(ctx, ingestion.Config{
		MaxFileBytes: 1 << 20,
		Registry:     registry,
	})
	if err != nil {
		t.Fatalf("ingestion.New() error = %v", err)
	}
	return ingestor
}

func newParentChildEngine(t *testing.T, parserInfo ingestion.ParserInfo) *chunking.Engine {
	t.Helper()
	formatAdapter, err := adapter.NewIngestionAdapter(parserInfo)
	if err != nil {
		t.Fatalf("NewIngestionAdapter() error = %v", err)
	}
	parentBuilder, err := parentchild.NewBoundedParentBuilder(parentchild.BoundedParentBuilderConfig{
		MaxRunes: 200,
	})
	if err != nil {
		t.Fatalf("NewBoundedParentBuilder() error = %v", err)
	}
	childSplitter, err := parentchild.NewBoundedTextSplitter(parentchild.BoundedTextSplitterConfig{
		MaxRunes: 40,
	})
	if err != nil {
		t.Fatalf("NewBoundedTextSplitter() error = %v", err)
	}
	strategy, err := parentchild.NewParentChildStrategy(parentchild.ParentChildConfig{
		ParentBuilder: parentBuilder,
		ChildSplitter: childSplitter,
	})
	if err != nil {
		t.Fatalf("NewParentChildStrategy() error = %v", err)
	}
	engine, err := chunking.NewEngine(chunking.EngineConfig{
		Profile:  chunking.Profile{Name: "ingestion-parent-child", Version: "v1"},
		Adapter:  formatAdapter,
		Strategy: strategy,
	})
	if err != nil {
		t.Fatalf("chunking.NewEngine(parent-child) error = %v", err)
	}
	return engine
}

func newStructureAwareEngine(t *testing.T, parserInfo ingestion.ParserInfo) *chunking.Engine {
	t.Helper()
	formatAdapter, err := adapter.NewIngestionAdapter(parserInfo)
	if err != nil {
		t.Fatalf("NewIngestionAdapter() error = %v", err)
	}
	strategy, err := structureaware.NewStructureAwareStrategy(structureaware.StructureAwareConfig{
		MaxRunes:       200,
		MinRunes:       20,
		HeadingContext: structureaware.HeadingContextPrepend,
	})
	if err != nil {
		t.Fatalf("NewStructureAwareStrategy() error = %v", err)
	}
	engine, err := chunking.NewEngine(chunking.EngineConfig{
		Profile:  chunking.Profile{Name: "ingestion-structure-aware", Version: "v1"},
		Adapter:  formatAdapter,
		Strategy: strategy,
	})
	if err != nil {
		t.Fatalf("chunking.NewEngine(structure-aware) error = %v", err)
	}
	return engine
}

func assertParentChildResult(t *testing.T, result *chunking.Result, sourceURI string) {
	t.Helper()
	if result.StrategyName != parentchild.ParentChildStrategyName {
		t.Fatalf("strategy = %q, want %q", result.StrategyName, parentchild.ParentChildStrategyName)
	}
	if result.Statistics.ParentCount != 4 || result.Statistics.ChildCount != 4 {
		t.Fatalf("statistics = %#v, want 4 parents and 4 children", result.Statistics)
	}
	parents := make(map[string]chunking.Chunk, result.Statistics.ParentCount)
	parentRelations := 0
	for _, relation := range result.Relations {
		if relation.Type == chunking.RelationTypeParentChild {
			parentRelations++
		}
	}
	for _, chunk := range result.Chunks {
		if chunk.Metadata["_source"] != sourceURI {
			t.Fatalf("chunk %q source = %#v, want %q", chunk.ID, chunk.Metadata["_source"], sourceURI)
		}
		if chunk.Kind == chunking.ChunkKindParent {
			parents[chunk.ID] = chunk
		}
	}
	for _, chunk := range result.Chunks {
		if chunk.Kind != chunking.ChunkKindChild {
			continue
		}
		parent, exists := parents[chunk.ParentID]
		if !exists || parent.DocumentID != chunk.DocumentID || chunk.Level != parent.Level+1 {
			t.Fatalf("child %q has invalid parent %q", chunk.ID, chunk.ParentID)
		}
	}
	if parentRelations != result.Statistics.ChildCount {
		t.Fatalf("parent-child relation count = %d, want %d", parentRelations, result.Statistics.ChildCount)
	}
}

func assertStructureAwareResult(t *testing.T, result *chunking.Result, sourceURI string) {
	t.Helper()
	if result.StrategyName != structureaware.StructureAwareStrategyName {
		t.Fatalf("strategy = %q, want %q", result.StrategyName, structureaware.StructureAwareStrategyName)
	}
	if len(result.Chunks) != 2 {
		t.Fatalf("structure chunk count = %d, want 2", len(result.Chunks))
	}
	wantPaths := [][]string{{"安装"}, {"运行"}}
	for index, chunk := range result.Chunks {
		if chunk.Kind != structureaware.ChunkKindStructure || chunk.Level != 0 {
			t.Fatalf("chunk %d kind/level = %q/%d, want structure/0", index, chunk.Kind, chunk.Level)
		}
		if chunk.Metadata["_source"] != sourceURI {
			t.Fatalf("chunk %d source = %#v, want %q", index, chunk.Metadata["_source"], sourceURI)
		}
		path, ok := chunk.Metadata[structureaware.MetadataStructurePath].([]string)
		if !ok || !reflect.DeepEqual(path, wantPaths[index]) {
			t.Fatalf("chunk %d structure path = %#v, want %#v", index, path, wantPaths[index])
		}
		if len(chunk.SourceUnitIDs) != 2 {
			t.Fatalf("chunk %d source unit count = %d, want 2", index, len(chunk.SourceUnitIDs))
		}
	}
	if result.Chunks[0].NextID != result.Chunks[1].ID || result.Chunks[1].PreviousID != result.Chunks[0].ID {
		t.Fatalf("structure chunk adjacency is not reciprocal")
	}
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
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		unitID := fmt.Sprintf("unit-%d", len(documents)+1)
		metadata := cloneMetadata(options.ExtraMeta)
		if strings.HasPrefix(line, "# ") {
			title := strings.TrimSpace(strings.TrimPrefix(line, "# "))
			if title == "" {
				return nil, fmt.Errorf("empty markdown heading")
			}
			headingID = unitID
			path = []string{title}
			metadata[ingestion.MetadataStructureKind] = string(chunking.BlockKindHeading)
			metadata[ingestion.MetadataStructureDepth] = 0
			metadata[ingestion.MetadataStructurePath] = append([]string(nil), path...)
			metadata[ingestion.MetadataStructureBoundary] = string(chunking.BlockBoundaryHard)
			documents = append(documents, &schema.Document{ID: unitID, Content: title, MetaData: metadata})
			continue
		}
		if headingID == "" {
			return nil, fmt.Errorf("paragraph appears before the first heading")
		}
		metadata[ingestion.MetadataStructureKind] = string(chunking.BlockKindParagraph)
		metadata[ingestion.MetadataStructureDepth] = 1
		metadata[ingestion.MetadataStructureParentID] = headingID
		metadata[ingestion.MetadataStructurePath] = append([]string(nil), path...)
		metadata[ingestion.MetadataStructureBoundary] = string(chunking.BlockBoundaryNone)
		documents = append(documents, &schema.Document{ID: unitID, Content: line, MetaData: metadata})
	}
	if len(documents) == 0 {
		return nil, fmt.Errorf("markdown contains no logical units")
	}
	return documents, nil
}

func cloneMetadata(metadata map[string]any) map[string]any {
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func writeMarkdownFixture(t *testing.T) string {
	t.Helper()
	sourceURI := filepath.Join(t.TempDir(), "knowledge.md")
	content := "# 安装\n下载发布包并初始化配置文件。\n\n# 运行\n启动服务后检查健康检查接口和日志输出。\n"
	if err := os.WriteFile(sourceURI, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return sourceURI
}

var _ parser.Parser = headingMarkdownParser{}
