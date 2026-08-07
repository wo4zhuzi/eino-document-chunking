# eino-document-chunking

`eino-document-chunking` 是面向 CloudWeGo Eino 的可扩展文档 Chunking 框架。它接收 Loader、Parser 或 [`eino-document-ingestion`](https://github.com/wo4zhuzi/eino-document-ingestion) 产生的标准 `[]*schema.Document`，输出可供 Embedding、Indexer 和 Retriever 使用的 Chunk、关系、统计与可追溯元数据。

当前版本只实现父子 Chunk。固定长度、递归、语义、多层、代码和表格专用 Chunk 等能力仅是未来方向，尚未实现。

## 责任边界

本项目负责：

- `schema.Document` 到 Chunk 的确定性转换。
- 格式适配与 Chunk 策略的解耦和选择。
- 有界父 Chunk、子 Chunk、稳定 ID、层级和相邻关系。
- 输入 Metadata 保留、来源单元追踪、Profile 名称和版本记录。
- 完整 Chunk Result 与 Eino `document.Transformer` 适配。

本项目不负责：

- 文件下载、MIME 检测、Loader、Parser、OCR。
- Embedding、Indexer、Retriever、Reranker。
- 数据库、向量存储、索引事务和知识库版本生命周期。
- Prompt、ChatModel 和答案生成。

父级持久化、子级向量化、父级查询聚合均由调用方完成。

## 版本要求

- Go：`1.26.x`，模块声明为 `go 1.26.0`。
- CloudWeGo Eino：`v0.9.12`。
- 参考摄取组件：`github.com/wo4zhuzi/eino-document-ingestion@v0.0.0-20260806102959-f0ac8222e281`。

核心包只依赖 Eino，不强制依赖摄取组件。

## 架构

```text
[]*schema.Document
        |
        v
FormatAdapter       不同 Parser 输出 -> 统一 Block
        |
        v
[]Block
        |
        v
Strategy            Block -> Chunk + Relation
        |
        v
Result              Profile + Chunk + Relation + Statistics
```

格式和策略是两个独立扩展维度。新增 PDF、Markdown、表格等格式适配时实现 `FormatAdapter`；新增 Chunk 策略时实现 `Strategy`。不需要创建 `PDFParentChildChunker`、`MarkdownParentChildChunker` 这类组合类型。

仓库按职责组织，而不是按格式和策略做组合：

```text
.
├── engine.go                    # 稳定执行入口与结果装饰
├── types.go                     # Block、Chunk、Relation、Strategy 等核心契约
├── profile.go                   # 可复现 Profile
├── id.go                        # 稳定 ID 契约与默认实现
├── metadata.go                  # 集中的 Metadata Key
├── adapter/                     # Document -> Block 的格式适配
├── strategy/
│   └── parentchild/             # 当前唯一内置父子 Chunk 策略
├── einoadapter/                 # Eino document.Transformer 投影
├── internal/                    # 非公开 Metadata 与文本工具
└── examples/                    # 完全离线示例
```

根包只保留稳定契约和 Engine，不依赖任何具体 Adapter 或 Strategy。具体实现通过构造函数注入，因此未来新增格式或策略时不会修改 Engine，也不会形成“格式数量 × 策略数量”的包结构。

核心公开契约包括：

- `Engine`：统一执行、输入克隆、错误包装、Metadata 装饰和结果校验。
- `FormatAdapter`：将标准 Document 转为统一 `Block`。
- `Strategy`：将 Block 组织为 Chunk 和 Relation。
- `Profile`：可复现配置的名称和版本。
- `IDGenerator`：可注入稳定 ID 生成器，默认使用 SHA-256。
- `Chunk`：包含 ID、Kind、Level、Parent/Previous/Next ID、来源单元、顺序、字符/Token 统计和 Metadata。
- `Result`：包含 Chunk、关系、统计、Profile、Adapter 和 Strategy 信息。

## 父子 Chunk 默认行为

未注入父级构造器时，`ParentChildStrategy` 使用 `BoundedParentBuilder`：

- 每个逻辑 Block 独立构造父 Chunk。
- 父 Chunk 默认最多 `2000` 个 Unicode 字符。
- 超长 Block 优先在空段、换行或空白边界切分，找不到边界时才硬切。
- 因此默认父 Chunk 不会是无限大的完整文件。

未注入子级 Splitter 或 Transformer 时，策略使用 `BoundedTextSplitter`：

- 子 Chunk 默认最多 `500` 个 Unicode 字符。
- 每个有效父 Chunk 至少生成一个子 Chunk。
- 每个子 Chunk 都有 `ParentID`，并生成父子 Relation。

输出采用稳定的层次顺序：父 Chunk 后紧跟其子 Chunk，`Sequence` 从 `1` 开始。父 Chunk 在同一 `DocumentID` 内维护 Previous/Next；子 Chunk 在同一父 Chunk 内维护 Previous/Next。

默认 `DocumentAdapter` 每个非空 Document 生成一个 Block。逻辑 `DocumentID` 按以下顺序解析：

1. Metadata `document_id`。
2. Eino FileLoader Metadata `_source`。
3. Document 自身 `ID`。
4. 基于输入顺序和内容生成的稳定来源 ID。

nil 和空白 Document 会被忽略；全部为空时返回 `ErrNoValidBlocks`。

## 快速开始

```go
package main

import (
    "context"
    "fmt"

    "github.com/cloudwego/eino/schema"
    chunking "github.com/wo4zhuzi/eino-document-chunking"
    "github.com/wo4zhuzi/eino-document-chunking/adapter"
    "github.com/wo4zhuzi/eino-document-chunking/strategy/parentchild"
)

func main() {
    strategy, err := parentchild.NewParentChildStrategy(parentchild.ParentChildConfig{})
    if err != nil {
        panic(err)
    }
    engine, err := chunking.NewEngine(chunking.EngineConfig{
        Profile:  chunking.Profile{Name: "knowledge-base", Version: "v1"},
        Adapter:  adapter.NewDocumentAdapter(),
        Strategy: strategy,
    })
    if err != nil {
        panic(err)
    }

    result, err := engine.Chunk(context.Background(), []*schema.Document{{
        ID:      "section-1",
        Content: "需要切分的正文",
        MetaData: map[string]any{
            "_source": "memory://guide.md",
        },
    }})
    if err != nil {
        panic(err)
    }
    fmt.Printf("chunks=%d relations=%d\n", len(result.Chunks), len(result.Relations))
}
```

完整离线示例：

```bash
go run ./examples/parent-child
```

预期输出为 JSON，包含 `profile`、`adapter_name`、`strategy_name`、`chunks`、`relations` 和 `statistics`；不需要网络、模型、数据库或 API Key。

## 注入父级构造器和子级 Transformer

```go
parentBuilder, err := parentchild.NewBoundedParentBuilder(parentchild.BoundedParentBuilderConfig{
    MaxRunes: 1200,
})
if err != nil {
    return err
}

strategy, err := parentchild.NewParentChildStrategy(parentchild.ParentChildConfig{
    ParentBuilder:    parentBuilder,
    ChildTransformer: yourEinoTransformer,
})
```

也可以通过 `ChildSplitter` 注入更小的最小接口。`ChildSplitter` 和 `ChildTransformer` 不能同时配置。策略会逐父 Chunk 调用 Transformer，因此不依赖 Transformer 是否保留输入 Document ID；最终父子 ID 始终由本项目统一生成。

## 与 eino-document-ingestion 衔接

摄取组件负责数据源、格式校验和 Parser，本项目直接消费其 Documents：

```go
ingested, err := ingestor.Ingest(ctx, sourceURI)
if err != nil {
    return err
}

chunkResult, err := engine.Chunk(ctx, ingested.Documents)
if err != nil {
    return err
}
```

本项目不会读取 `SourceInfo` 或重新判断格式。PDF 页码、DOCX Section、XLSX Sheet/Row、FileLoader `_source` 等原有 Metadata 会保留到父子 Chunk。

## Eino Transformer 适配

核心 API `Engine.Chunk` 始终返回完整 Result。接入 Eino Graph 时，通过构造参数显式选择输出：

```go
transformer, err := einoadapter.NewEinoTransformer(engine, einoadapter.EinoTransformerConfig{
    Output: einoadapter.TransformerOutputChildren,
})
```

可选值：

- `TransformerOutputParents`：只返回父 Chunk。
- `TransformerOutputChildren`：只返回子 Chunk，适合交给 Embedding/向量 Indexer。
- `TransformerOutputAll`：按 Result 顺序返回全部 Chunk。

未指定输出模式会返回 `ErrInvalidConfig`，不存在隐式默认行为。

## Metadata 契约

输入 Metadata 会递归克隆常见 map/slice 类型，并原样保留；本项目不会修改调用方传入的 Document、Metadata 或切片。

本项目集中定义以下命名空间字段：

| 常量 | Key |
|---|---|
| `MetadataChunkID` | `eino_chunking.chunk_id` |
| `MetadataChunkKind` | `eino_chunking.chunk_kind` |
| `MetadataChunkLevel` | `eino_chunking.chunk_level` |
| `MetadataDocumentID` | `eino_chunking.document_id` |
| `MetadataParentID` | `eino_chunking.parent_id`，仅子 Chunk 写入 |
| `MetadataPreviousID` / `MetadataNextID` | 相邻 Chunk ID |
| `MetadataSourceUnitIDs` | 原始解析单元 ID 列表 |
| `MetadataSequence` | 从 1 开始的稳定输出顺序 |
| `MetadataCharacterCount` / `MetadataTokenCount` | 长度统计 |
| `MetadataProfileName` / `MetadataProfileVersion` | Profile 标识 |
| `MetadataStrategyName` / `MetadataAdapterName` | 执行组件标识 |

这些 Key 属于保留命名空间。输入或自定义策略提前写入同名 Key 时返回 `ErrMetadataConflict`，不会覆盖调用方字段。

Eino Parent Indexer 和 Parent Retriever 可将 `ParentIDKey` 配置为 `chunking.MetadataParentID`。本项目只生成兼容元数据，不调用底层 Indexer 或父文档存储。

## 稳定 ID 和 Profile

默认 `SHA256IDGenerator` 使用以下稳定字段生成 Chunk ID：

- Profile 名称和版本。
- Strategy 名称、Chunk Kind 和 Level。
- DocumentID、ParentID、Sequence。
- Chunk Content 和 SourceUnitIDs。

相同输入、相同 Profile、相同策略配置和相同 IDGenerator 会产生相同 ID 与输出顺序。修改 Profile 版本会有意生成新 ID，便于配置升级和重建索引。

生产环境可以实现 `IDGenerator` 接口接入现有 ID 规范。生成器返回空 ID、重复 ID 或错误时，Engine 会拒绝结果。

## 结果校验

Engine 在返回前验证：

- Block ID 和 Chunk ID 不重复。
- Chunk 内容非空、Sequence 连续、来源单元存在。
- 子 Chunk 必须引用存在且层级更低的同一逻辑文档父 Chunk。
- Previous/Next 必须双向一致，且 Kind、Level、DocumentID、ParentID 相同。
- 父子、相邻、来源 Relation 与 Chunk 字段完全一致。
- 至少存在一个有效 Chunk。

错误增加上下文时均使用 `%w`，调用方可以通过 `errors.Is` 判断哨兵错误、Context 取消和依赖错误。

## 下游使用边界

推荐调用方按以下方式组装：

```text
Loader / Parser / eino-document-ingestion
                |
                v
        chunking.Engine
          |          |
          |          +--> 父 Chunk -> 文档存储，由调用方管理
          |
          +--> 子 Chunk -> Embedding -> Indexer
                                      |
                                      v
                                Retriever 命中子 Chunk
                                      |
                                      v
                         根据 MetadataParentID 回查父 Chunk
```

本项目不决定父 Chunk 和子 Chunk 分别存入哪个存储，也不实现召回后的父级聚合。

## 测试和验证

默认测试完全离线：

```bash
gofmt -w *.go adapter/*.go strategy/parentchild/*.go einoadapter/*.go internal/*/*.go examples/parent-child/*.go
go test ./...
go test -race ./...
go vet ./...
```

预期结果：所有包测试通过，race detector 无数据竞争，`go vet` 无诊断。

测试覆盖单/多文档、父子与相邻关系、稳定 ID、Metadata 和输入不可变、空输入、非法配置、依赖错误、重复 ID、非法关系、Context 取消/超时、并发调用、Eino Transformer，以及无需修改 Engine 的自定义 Strategy。

## 已知限制


- 默认切分按 Unicode 字符计数，不提供模型 Tokenizer；`TokenCount` 默认是 `0`，`CharacterCount` 始终可用。
- 默认文本边界切分不理解 Markdown AST、代码语法、表格结构或语义相似度；这些能力应通过 FormatAdapter、ParentBuilder 或 ChildSplitter 扩展。
- Metadata 深拷贝覆盖常见 JSON/Eino map 和 slice 类型；未知自定义引用类型按原值保留，调用方不应在 Chunking 期间并发修改此类对象。
- 本项目不提供父 Chunk 存储、向量数据库、索引事务、Retriever 聚合或版本发布。

## 未来规划

以下能力尚未实现：固定长度 Chunk、递归 Chunk、语义 Chunk、多层 Chunk、代码和表格等领域专用 Chunk、其他自定义内置策略。未来新增能力应继续复用现有 `FormatAdapter -> Block -> Strategy -> Result` 契约。
