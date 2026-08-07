package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/document/parser"
	"github.com/cloudwego/eino/schema"
	chunking "github.com/wo4zhuzi/eino-document-chunking"
	ingestion "github.com/wo4zhuzi/eino-document-ingestion"
)

const outlineExtension = ".outline"

type outlineParser struct{}

type outlineHeading struct {
	id    string
	title string
}

func (outlineParser) Parse(
	ctx context.Context,
	reader io.Reader,
	opts ...parser.Option,
) ([]*schema.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("解析大纲文档: %w", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("读取大纲文档: %w", err)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("大纲文档不是有效 UTF-8")
	}

	options := parser.GetCommonOptions(&parser.Options{}, opts...)
	documents := make([]*schema.Document, 0)
	headings := make([]outlineHeading, 0, 6)
	for lineNumber, rawLine := range strings.Split(string(data), "\n") {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("解析大纲文档: %w", err)
		}
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		id := fmt.Sprintf("block-%04d", len(documents)+1)
		level, title, isHeading := parseOutlineHeading(line)
		if isHeading {
			if level > len(headings)+1 {
				return nil, fmt.Errorf("第 %d 行标题层级从 %d 跳到 %d", lineNumber+1, len(headings), level)
			}
			depth := level - 1
			headings = headings[:depth]
			path := headingPath(headings, title)
			metadata := cloneMetadata(options.ExtraMeta)
			metadata[ingestion.MetadataStructureKind] = string(chunking.BlockKindHeading)
			metadata[ingestion.MetadataStructureDepth] = depth
			metadata[ingestion.MetadataStructurePath] = path
			metadata[ingestion.MetadataStructureBoundary] = string(chunking.BlockBoundaryHard)
			if depth > 0 {
				metadata[ingestion.MetadataStructureParentID] = headings[depth-1].id
			}
			documents = append(documents, &schema.Document{
				ID:       id,
				Content:  title,
				MetaData: metadata,
			})
			headings = append(headings, outlineHeading{id: id, title: title})
			continue
		}

		if len(headings) == 0 {
			return nil, fmt.Errorf("第 %d 行正文出现在第一个标题之前", lineNumber+1)
		}
		metadata := cloneMetadata(options.ExtraMeta)
		metadata[ingestion.MetadataStructureKind] = string(chunking.BlockKindParagraph)
		metadata[ingestion.MetadataStructureDepth] = len(headings)
		metadata[ingestion.MetadataStructurePath] = headingPath(headings, "")
		metadata[ingestion.MetadataStructureParentID] = headings[len(headings)-1].id
		metadata[ingestion.MetadataStructureBoundary] = string(chunking.BlockBoundaryNone)
		documents = append(documents, &schema.Document{
			ID:       id,
			Content:  line,
			MetaData: metadata,
		})
	}
	if len(documents) == 0 {
		return nil, fmt.Errorf("大纲文档没有可解析内容")
	}
	return documents, nil
}

func parseOutlineHeading(line string) (level int, title string, ok bool) {
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level >= len(line) || line[level] != ' ' {
		return 0, "", false
	}
	title = strings.TrimSpace(line[level+1:])
	if title == "" {
		return 0, "", false
	}
	return level, title, true
}

func headingPath(headings []outlineHeading, leaf string) []string {
	path := make([]string, 0, len(headings)+1)
	for _, heading := range headings {
		path = append(path, heading.title)
	}
	if leaf != "" {
		path = append(path, leaf)
	}
	return path
}

func cloneMetadata(metadata map[string]any) map[string]any {
	cloned := make(map[string]any, len(metadata)+5)
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

var _ parser.Parser = outlineParser{}
