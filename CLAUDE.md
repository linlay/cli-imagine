# CLAUDE.md

`cli-imagine` 的用户安装、配置、验证和日常排障说明见 [README.md](./README.md)。

这个文件面向维护者、协作工程师和智能体，记录项目事实、分层边界、关键数据结构和开发约定。

## 1. 项目概览

`cli-imagine` 是一个配置驱动的本地图像 CLI。

它的目标是：

- 把 provider 差异收敛到本地 TOML + JSON schema 配置
- 把图像生成、图像编辑、外部图片导入统一成稳定的 CLI 调用方式
- 让调用方可以先 discovery，再 inspect，再真正执行
- 让维护者可以基于配置目录快速接入新模型或迁移旧 YAML 配置

它不打算解决的问题：

- 不再提供 HTTP MCP 服务
- 不尝试覆盖所有图像 provider 协议，只支持当前实现的 request kind / parser 组合
- 不把输出目录中的 manifest 承诺为稳定的对外 API

## 2. 技术栈

- 语言：Go 1.26
- CLI 框架：`spf13/cobra`
- 配置解码：`github.com/pelletier/go-toml/v2`
- schema 编译：`github.com/santhosh-tekuri/jsonschema/v5`
- YAML 迁移：`gopkg.in/yaml.v3`
- 网络访问：标准库 `net/http`
- 本地第三方镜像：`third_party/cobra`、`third_party/jsonschema`、`third_party/gocheck`

当前仓库是单二进制 CLI 项目，没有数据库、消息队列或前端运行时。

## 3. 架构设计

主执行链路可以理解为：

`cobra command -> config.LoadDir -> imagine.Runtime -> provider request / storage -> render output`

分层职责如下：

- `cmd/imagine`：程序入口，只负责把进程参数交给 `internal/app`
- `internal/app`：CLI 命令装配、flag 解析、输出格式选择、退出码映射
- `internal/config`：扫描配置目录、加载 provider TOML、解析 auth、编译 capability schema、导入旧 YAML
- `internal/imagine`：模型目录、参数校验、请求构造、provider 调用、图片导入、资产落盘
- `internal/verify`：基于当前配置自动推导最小合法参数，执行 inspect + real run，并写摘要
- `internal/schema`：schema 编译与校验辅助

设计边界：

- Cobra 类型不进入 `internal/imagine` 运行时逻辑
- 运行时只接收标准化后的 `tool + args` 输入
- 配置加载在执行前完成，运行时不直接关心 TOML 文件扫描
- 资产持久化统一由 `Storage` 管理，输出目录是其安全边界

## 4. 目录结构

```text
cli-imagine/
  cmd/imagine/                 # CLI 入口
  internal/app/                # Cobra 命令、flag 解析、输出渲染、退出码
  internal/buildinfo/          # 版本与构建信息
  internal/config/             # TOML/YAML 配置加载、schema 路径解析、迁移逻辑
  internal/imagine/            # tool 运行时、模型目录、provider 请求、存储
  internal/jsonutil/           # JSON 辅助
  internal/schema/             # JSON schema 编译/校验
  internal/verify/             # 真实验证与 summary 生成
  examples/                    # 示例 provider TOML 与 schema
  third_party/                 # 本地替换的第三方依赖
  README.md                    # 用户文档
  CLAUDE.md                    # 项目事实文档
```

当前根目录没有 `Dockerfile`、`docker-compose.yml`、`Makefile`、`.env.example`。这些不属于现有代码路径的一部分，如果未来引入，需要明确其职责边界，避免与当前配置目录模型冲突。

## 5. 数据结构

核心配置结构：

- `config.ProviderConfig`
  - provider 名称、描述、默认 `endpoint`、默认 `auth`、模型列表
- `config.ModelConfig`
  - 已合并 provider 默认值后的模型视图
- `config.ModelCapabilityConfig`
  - `input_schema`、`request`、`response`

核心运行时结构：

- `imagine.ModelCatalog`
  - 运行时使用的模型索引，按模型名查 capability
- `imagine.DiscoveryModel` / `imagine.DiscoveryOperation`
  - 用于 `models`、`model`、`inspect`、`verify` 的发现视图
- `imagine.GenerateRequest` / `imagine.EditRequest`
  - 标准化后的 provider 请求输入
- `imagine.Inspection`
  - `inspect` 输出，包含 tool、标准化参数、模型信息和最终请求体

核心持久化结构：

- `imagine.AssetRecord`
  - 记录输出图片的 `assetId`、相对路径、hash、来源模型、来源图片、创建时间等
- `.imagine-assets.json`
  - 输出目录下的资产 manifest
- `verify.Summary` / `verify.CaseInfo`
  - `verify` 执行结果与每个 capability 的检查明细

## 6. API 定义

这是一个 CLI API 项目，稳定接口以命令和参数为主。

顶层子命令：

- `providers`
- `models`
- `model <model>`
- `generate`
- `edit`
- `import`
- `run <tool>`
- `inspect <tool>`
- `config validate`
- `config import-yaml`
- `verify`
- `version`

运行时 tool 名：

- `image.generate`
- `image.edit`
- `image.import`

输出约定：

- `providers`、`models`、`model` 默认输出 text，可切到 `--format json`
- `inspect` 默认输出 json
- `generate`、`edit`、`import` 默认输出生成文件的相对路径
- `verify` 默认输出 `summary.md` 路径

错误与退出码：

- `0`：成功
- `2`：配置阶段失败
- `3`：执行阶段失败
- `4`：保留给断言类错误

## 7. 开发要点

配置加载规则：

- 只扫描配置目录当前层的 `*.toml`
- provider 名、model 名都必须唯一
- `input_schema` 相对路径相对于 provider TOML 所在目录解析
- capability schema 会在加载期编译，错误尽量前置暴露

鉴权规则：

- `auth` 必须且只能配置 `api_key`、`api_key_env`、`api_key_file` 之一
- model 一旦声明 `auth`，即整体覆盖 provider `auth`
- `api_key_env` / `api_key_file` 会在加载期解析成真实 key

请求构造规则：

- generate/edit 先把 CLI 参数标准化，再根据 capability 的 `request.kind` 与 `size_mode` 组装请求
- 当前支持的 request kind 包括 `images_generate`、`generate_content`、`images_edit`、`chat_completions`
- 响应解析通过 `response.default_format` 和 `parser_by_format` 决定

存储规则：

- 输出目录默认是当前目录
- 所有输出文件都必须落在输出目录内部
- `data_path` 导入只能读取输出目录内的相对路径，不能越界访问

测试与验证建议：

- 改配置加载逻辑时先跑 `go test ./...`
- 接入新模型时先跑 `config validate`
- 真正联调前先跑 `inspect`
- 只有在确认会访问真实 provider 时才跑 `verify`

## 8. 开发流程

推荐的日常变更流程：

1. 明确变更是在 CLI 层、配置层还是运行时层
2. 如果涉及新模型或新 provider，先补 `examples/` 中的最小示例
3. 运行 `go test ./...`
4. 用 `imagine --config <dir> config validate` 验证配置可加载
5. 用 `imagine --config <dir> inspect <tool> ...` 检查最终请求是否符合预期
6. 必要时再用 `verify` 做真实请求验证
7. 如果变更影响用户使用方式，同步更新 `README.md`
8. 如果变更影响分层、语义或目录边界，同步更新 `CLAUDE.md`

对旧配置兼容迁移时，优先保持 `config import-yaml --from <src> --to <dst>` 这条迁移链路可用，不要手动散落多个迁移脚本。

## 9. 已知约束与注意事项

- 当前项目是纯 CLI，不支持服务端常驻模式
- `verify` 会发真实请求，可能消耗 provider 配额
- `verify` 的成功率受网络、代理、上游 provider 行为和本地密钥注入影响
- request kind 和 parser 只覆盖当前代码显式实现的分支；新增 provider 时通常需要先扩展运行时
- `data_path` 导入受输出目录约束，不能直接读取任意绝对路径
- 运行时默认限制单文件大小为 20 MiB、响应体大小为 32 MiB、provider 超时为 30000 ms
- manifest 文件属于运行产物，不应作为稳定的跨版本协议依赖
