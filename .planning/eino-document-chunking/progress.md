# 实施进度

## 2026-08-07

- 已读取 `planning-with-files-zh` 和 `using-coze-cli` 技能规范。
- 已确认当前任务应在本地 Go 仓库完成，参考仓库保持只读，不创建超出授权范围的外部 Coze 项目。
- 已创建任务计划、调研发现和进度文件。
- 已完成当前仓库与参考仓库根基线读取：确认当前仓库尚未初始化 Go 模块，参考版本为 Go 1.26.0、Eino v0.9.12。
- 已读完参考示例全部源码与测试，确认摄取/Chunk/索引职责边界和输入不可变约定。
- 已定位本机 ingestion 与 Eino v0.9.12 模块缓存源码。
- 已读取 ingestion README/公开类型及 Eino Parent Indexer、Parent Retriever 实现，确认父子 Metadata 与职责边界。
- 已精读 Eino Transformer、TransformerOption 和 schema.Document 的实际定义。
- 已完成公开 API 与父子策略架构定稿，准备进入实现阶段。
- 已初始化 Go 模块并新增公开数据模型、Metadata Key、默认 SHA-256 IDGenerator、Document Adapter、Engine 和统一校验。
- 已实现有界父级构造、默认子级文本 Splitter、Eino Transformer 子级包装、父子策略和显式输出的 Eino Transformer 适配器。
- 首次 `go mod tidy` 因沙箱禁止写用户级 Go 构建缓存而失败，已定位为环境问题，准备切换任务专用临时缓存。
- 使用本机 `file://` 模块代理后仍触发 sumdb 网络校验，已确认需在离线验证命令中设置 `GOSUMDB=off`。
- 使用任务专用临时缓存和本机离线模块代理完成 `go mod tidy`，核心包首次编译通过。
- 已补齐父子行为、错误路径、Context、并发、自定义 Strategy 和 Eino Transformer 测试，`go test ./...` 通过。
- 已新增完全离线父子示例并运行成功，README 已扩展为完整中文项目文档。
- 自查后调整父 Chunk Metadata：不写入空 `parent_id`，避免 Eino Parent Retriever 将空值当作父 ID。
- 最终执行 `gofmt -w`，完成且无剩余格式差异。
- 最终执行 `go test ./...`：通过，根包测试成功，离线示例包编译成功。
- 最终执行 `go test -race ./...`：通过，无数据竞争。
- 最终执行 `go vet ./...`：通过，无诊断。
- 最终执行 `go run ./examples/parent-child`：成功输出 2 个父 Chunk、5 个子 Chunk、关系和统计 JSON。
- 最终执行 `git diff --check`：通过，无空白错误。
- 用户指出根目录平铺不利于未来扩展，已确认根因并启动包结构重构。
- 新结构按职责拆分，不机械照抄示例文件名，也不引入未使用抽象。
- 已完成包结构重构及回归验证，用户确认继续实现 Structure-aware Chunking。
- 已确定最小实现范围：Block 可选结构契约、Structured Adapter、扁平 Structure-aware Strategy、离线测试与文档；不修改 Engine 调度和现有父子行为。
- 已为 Block 增加可选 BlockStructure、开放 BlockKind、软硬边界和结构错误契约；补齐 Engine 与父子策略的防御性复制。
- 核心变更后执行 `go test ./...` 通过，现有父子策略和 Eino 适配无回归。
- 已实现 StructuredDocumentAdapter、StructureResolver 与函数适配器，覆盖输入复制、稳定 Block ID、结构必填和错误包装。
- Adapter 单测及全仓测试通过。
- 已实现 StructureAwareStrategy：结构路径和硬边界分组、软边界阈值、标题上下文、普通文本稳定切分、原子块注入 Splitter、扁平相邻与来源关系。
- 首次策略测试发现结构深度与相邻关系同层约束冲突，已修正为扁平 Chunk Level=0、结构深度写入专用 Metadata。
- 已补齐结构父节点、Metadata 冲突、原子块 Splitter 错误/空输出/超限输出、重复 ID、并发、Context 和 Eino OutputAll 测试。
- 已新增 `examples/structure-aware` 并运行成功，输出 2 个结构 Chunk 和完整关系。
- README 已更新当前能力、项目结构、Structure-aware 契约、边界、示例与限制。
- 最终执行 `gofmt -w`：完成。
- 最终执行 `go test ./...`：全部通过。
- 最终执行 `go test -race ./...`：全部通过，无数据竞争。
- 最终执行 `go vet ./...`：通过，无诊断。
- `go run ./examples/parent-child` 和 `go run ./examples/structure-aware` 均运行成功。
- Structure-aware 离线示例输出 2 个扁平结构 Chunk、相邻关系和来源关系。

## 2026-08-08

- 用户提供独立结构化 Parser 仓库，要求据此编写结构感知用例。
- 已确认新模块最新版本为 `v0.0.0-20260808024546-02602d613c64`，并读取 README、Markdown Parser、结构构造和示例源码。
- 已定位现有根因：`examples/structure-aware` 与 integration 测试仍内嵌手写 Parser，没有覆盖真实依赖；真实 Parser 的 ID path 与 `code_block` 类型还暴露出现有示例配置和原子块识别差异。
- 已确定最小改动方案：真实 Parser 替换、真实集成测试、`code_block` 兼容、README 更新和全量回归验证。
- 已删除 `examples/structure-aware/outline_parser.go` 和 `.outline` 输入，新增真实 Markdown 示例并接入 `markdown.ParserInfo()/markdown.New()`。
- 已将 integration 测试改为独立 Parser 全链路测试，不再维护手写测试 Parser。
- 已增加 `BlockKindCodeBlock` 及原子块回归测试，超长 `code_block` 无 Splitter 时返回 `ErrOversizeBlock`。
- 已升级 ingestion 依赖并新增 structured parser 与 Goldmark 依赖，`go mod tidy` 成功。
- `go test ./... -count=1`、`go test -race ./... -count=1`、`go vet ./...`、结构感知示例运行和 `git diff --check` 全部通过。
