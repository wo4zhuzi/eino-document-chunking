package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	chunking "github.com/wo4zhuzi/eino-document-chunking"
	"github.com/wo4zhuzi/eino-document-chunking/adapter"
	"github.com/wo4zhuzi/eino-document-chunking/strategy/structureaware"
	ingestion "github.com/wo4zhuzi/eino-document-ingestion"
	"github.com/wo4zhuzi/eino-document-parser-structured/markdown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("用法: go run ./examples/structure-aware <本地 .md 文件或 HTTP/HTTPS URL>")
	}
	ctx := context.Background()
	registry, err := ingestion.NewDefaultRegistry(ctx)
	if err != nil {
		return fmt.Errorf("创建默认 Parser 注册表: %w", err)
	}
	if err := registry.ReplaceParser(
		ingestion.ExtensionMarkdown,
		markdown.ParserInfo(),
		markdown.New(),
	); err != nil {
		return fmt.Errorf("替换结构化 Markdown Parser: %w", err)
	}
	ingestor, err := ingestion.New(ctx, ingestion.Config{Registry: registry})
	if err != nil {
		return fmt.Errorf("创建文档摄取器: %w", err)
	}
	ingested, err := ingestor.Ingest(ctx, os.Args[1])
	if err != nil {
		return fmt.Errorf("摄取文档: %w", err)
	}
	if !ingested.Parser.Output.Structured || ingested.Parser.Output.Granularity != ingestion.GranularityBlock {
		return fmt.Errorf(
			"Parser %q 输出不支持 Structure-aware: granularity=%q structured=%t",
			ingested.Parser.Name,
			ingested.Parser.Output.Granularity,
			ingested.Parser.Output.Structured,
		)
	}

	formatAdapter, err := adapter.NewIngestionAdapter(ingested.Parser)
	if err != nil {
		return fmt.Errorf("创建 ingestion adapter: %w", err)
	}
	strategy, err := structureaware.NewStructureAwareStrategy(structureaware.StructureAwareConfig{
		MaxRunes:       3000,
		HeadingContext: structureaware.HeadingContextMetadataOnly,
	})
	if err != nil {
		return fmt.Errorf("创建 Structure-aware 策略: %w", err)
	}
	engine, err := chunking.NewEngine(chunking.EngineConfig{
		Profile:  chunking.Profile{Name: "ingestion-structure-aware", Version: "v1"},
		Adapter:  formatAdapter,
		Strategy: strategy,
	})
	if err != nil {
		return fmt.Errorf("创建 Chunk Engine: %w", err)
	}

	result, err := engine.Chunk(ctx, ingested.Documents)
	if err != nil {
		return fmt.Errorf("切分文档: %w", err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		return fmt.Errorf("输出结果: %w", err)
	}
	return nil
}
