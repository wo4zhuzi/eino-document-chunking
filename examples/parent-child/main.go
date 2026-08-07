package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	chunking "github.com/wo4zhuzi/eino-document-chunking"
	"github.com/wo4zhuzi/eino-document-chunking/adapter"
	"github.com/wo4zhuzi/eino-document-chunking/strategy/parentchild"
	ingestion "github.com/wo4zhuzi/eino-document-ingestion"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("用法: go run ./examples/parent-child <本地文件或 HTTP/HTTPS URL>")
	}
	ctx := context.Background()
	ingestor, err := ingestion.New(ctx, ingestion.Config{})
	if err != nil {
		return fmt.Errorf("创建文档摄取器: %w", err)
	}
	ingested, err := ingestor.Ingest(ctx, os.Args[1])
	if err != nil {
		return fmt.Errorf("摄取文档: %w", err)
	}

	formatAdapter, err := adapter.NewIngestionAdapter(ingested.Parser)
	if err != nil {
		return fmt.Errorf("创建 ingestion adapter: %w", err)
	}
	strategy, err := parentchild.NewParentChildStrategy(parentchild.ParentChildConfig{})
	if err != nil {
		return fmt.Errorf("创建 Parent-child 策略: %w", err)
	}
	engine, err := chunking.NewEngine(chunking.EngineConfig{
		Profile:  chunking.Profile{Name: "ingestion-parent-child", Version: "v1"},
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
