# eino-document-chunking 实施计划

## 目标

在当前仓库实现一个面向 CloudWeGo Eino 的可扩展文档 Chunking 框架，当前完整支持父子 Chunk，并完成离线测试、示例和中文文档。

## 阶段

| 阶段 | 状态 | 内容 |
|---|---|---|
| 1. 基线与契约调研 | 已完成 | 阅读当前仓库、只读参考仓库、上游 ingestion 契约和本地 Eino 源码，确认真实版本与 API |
| 2. API 与架构定稿 | 已完成 | 确定 Engine、Adapter、Strategy、Profile、Block、Chunk、Relation、Result、IDGenerator 和 Eino 适配边界 |
| 3. 核心实现 | 已完成 | 实现引擎、稳定 ID、校验、默认适配器和父子策略 |
| 4. Eino 适配与示例 | 已完成 | 实现 Transformer 适配器和完全离线示例 |
| 5. 测试与文档 | 已完成 | 完成需求覆盖测试和中文 README |
| 6. 全量验证 | 已完成 | 执行 gofmt、go test、race、vet 并修复问题 |
| 7. 包结构重构 | 已完成 | 根包只保留稳定契约和 Engine，具体 Adapter、父子 Strategy、Eino 适配与内部工具下沉到独立目录 |
| 8. 重构回归验证 | 已完成 | 迁移测试、示例和 README，重新执行 gofmt、test、race、vet |
| 9. 结构感知契约 | 已完成 | Block 可选类型安全结构信息与 StructuredDocumentAdapter 已完成 |
| 10. 结构感知策略 | 已完成 | 扁平 Structure-aware Strategy、稳定关系、错误处理和离线测试已完成 |
| 11. 文档与全量验证 | 已完成 | README、离线示例、gofmt、test、race、vet 全部完成 |

## 关键约束

- 只修改当前仓库，不修改 `/Users/david/Documents/code/my_github/eino-lab`。
- 先从实际文件确认 Go、Eino 和依赖 API，不猜测。
- 当前新增 Structure-aware Chunk，但不扩展为语义、多层、代码或表格专用切分。
- 格式适配与 Chunk 策略正交，避免格式和策略耦合类型。
- 不修改调用方输入对象、Metadata 或切片。
- 单元测试完全离线。

## 遇到的错误

| 错误 | 尝试次数 | 解决方案 |
|---|---:|---|
| `go mod tidy` 无法写入 `~/Library/Caches/go-build` | 1 | 改用 `/tmp` 下任务专用 `GOCACHE/GOMODCACHE`，并使用本机模块下载缓存的 `file://` 离线代理 |
| 离线 `go mod tidy` 仍尝试访问 `sum.golang.org` | 1 | 增加 `GOSUMDB=off`，模块内容继续从本机只读下载缓存获取 |
| Structure Chunk 使用结构深度作为 Level 导致跨深度相邻关系校验失败 | 1 | 扁平策略统一使用 `Chunk.Level=0`，结构深度保存在专用 Metadata |
| `go doc` 同时传入两个完整符号路径导致参数被解析为包名 | 1 | 分别对 Adapter 和 Strategy 的公开符号执行 `go doc` |
