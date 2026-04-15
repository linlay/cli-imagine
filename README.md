# cli-imagine

`cli-imagine` 是一个 Go 1.26 的本地图像 CLI，把原先 `mcp-server-imagine` 的核心能力收敛成可直接执行的命令行工具。

用户安装、配置、验证和排障看这个 README；项目设计、目录边界和内部语义见 [CLAUDE.md](./CLAUDE.md)。

## 1. 项目简介

`cli-imagine` 解决的是“把不同图像 provider 的模型能力配置化，然后用统一 CLI 执行生图、改图、导入和检查”这个问题。

当前仓库的核心能力包括：

- 读取本地 provider `*.toml` 配置和外部 JSON schema
- 发现 provider / model / operation
- 执行 `image.generate`、`image.edit`、`image.import`
- 用 `inspect` 预览最终请求，而不真正发请求
- 用 `verify` 对当前配置里的模型做真实连通性验证

它不再暴露 HTTP MCP 服务；运行入口就是本地二进制 `imagine`。

## 2. 快速开始

### 前置要求

- Go 1.26
- 可访问的图像 provider endpoint
- 至少一个 provider TOML 配置文件及其引用的 schema JSON
- 对输出目录有写权限

### 本地构建

```bash
go build -o ./imagine ./cmd/imagine
./imagine version
./imagine --help
```

### 用仓库示例配置做 discovery

```bash
./imagine --config ./examples providers
./imagine --config ./examples models
./imagine --config ./examples model GPT-Image-1
```

### 常见执行方式

直接走具名命令：

```bash
./imagine generate \
  --config ./examples \
  --model GPT-Image-1 \
  --prompt "A red panda reading in a lantern-lit forest" \
  --arg size='"1:1"'

./imagine edit \
  --config ./examples \
  --model gemini-2.5-flash-image-edit \
  --prompt "Turn this into a poster" \
  --image ./seed.png \
  --arg size='"1024x1024"'

./imagine import \
  --output-dir ./tmp \
  --item '{"type":"url","value":"https://example.com/image.png"}'
```

或者直接调用 tool 名：

```bash
./imagine run image.generate \
  --config ./examples \
  --args '{"model":"GPT-Image-1","prompt":"otter","size":"1:1"}'

./imagine inspect image.generate \
  --config ./examples \
  --args '{"model":"GPT-Image-1","prompt":"otter","size":"1:1"}'
```

### 配置校验与真实验证

```bash
./imagine --config ./examples config validate
./imagine --config ./examples verify --output-dir ./tmp
```

`verify` 会在 `<output-dir>/verification/<timestamp>/summary.md` 写出验证摘要。

## 3. 配置说明

### 配置目录

默认配置目录为：

```text
$XDG_CONFIG_HOME/imagine
```

如果未设置 `XDG_CONFIG_HOME`，则回落到：

```text
~/.config/imagine
```

也可以通过 `--config <dir>` 显式指定。

推荐目录形态：

```text
<config-dir>/
  poe.toml
  schemas/
    poe/
      GPT-Image-1.generate.json
      gemini-2.5-flash-image-edit.edit.json
```

### provider TOML 规则

当前配置模型是“每个 provider 一个 TOML 文件”：

- 根目录只扫描当前层级的 `*.toml`
- provider 名不能重复
- model 名不能重复
- 每个 provider 必须声明 `endpoint.base_url`
- 每个 model 至少包含一个 capability：`generate` 或 `edit`

仓库示例见：

- [examples/poe.toml](./examples/poe.toml)
- [examples/schemas/poe/GPT-Image-1.generate.json](./examples/schemas/poe/GPT-Image-1.generate.json)
- [examples/schemas/poe/gemini-2.5-flash-image-edit.edit.json](./examples/schemas/poe/gemini-2.5-flash-image-edit.edit.json)

### schema 与 capability

每个 capability 的 `input_schema` 都指向外部 JSON 文件，路径既可以写绝对路径，也可以写相对 provider TOML 的相对路径。

运行时会对 schema 做两件事：

- 加载并编译 JSON schema
- 根据 capability 的 `request.kind`、`size_mode`、`response.parser_by_format` 推导请求构造与响应解析方式

当前内置的 tool 名是：

- `image.generate`
- `image.edit`
- `image.import`

### 鉴权与 endpoint 覆盖

provider 和 model 都可以声明 `endpoint` 与 `auth`。合并规则是：

- model 级 `endpoint` 会覆盖 provider 级默认值
- model 级 `auth` 只要显式填写，就整体覆盖 provider 级 `auth`
- `auth` 必须且只能设置 `api_key`、`api_key_env`、`api_key_file` 三者之一
- 如果使用 `api_key_env` 或 `api_key_file`，最终解析出的 key 不能为空

推荐优先使用 `api_key_env` 或 `api_key_file`，不要把真实 key 提交进仓库。

### 命令参数合并规则

`generate`、`edit`、`run`、`inspect` 共用一套参数合并顺序：

```text
--args-file -> --args -> --arg key=json -> 显式 flags
```

其中：

- `--args-file` 读取一个 JSON object 文件
- `--args` 直接传 JSON object
- `--arg key=json` 用于逐项覆盖
- `generate` / `edit` 上的 `--model`、`--prompt`、`--response-format`、`--output-name`、`--image` 最后覆盖

### 输出目录与产物

`--output-dir` 未指定时默认写入当前目录。执行 `generate`、`edit`、`import`、`verify` 时会在输出目录维护：

```text
.imagine-assets.json
```

这个 manifest 会记录每个产物的：

- `assetId`
- `kind`
- `sourceMode`
- `model`
- `relativePath`
- `mimeType`
- `sizeBytes`
- `sha256`
- `prompt`
- `sourceImages`
- `createdAt`

`import` 当前支持的输入类型为：

- `url`
- `data_url`
- `base64`
- `data_path`

## 4. 部署

`cli-imagine` 是本地 CLI，不是常驻服务。当前仓库也没有 Dockerfile 或 docker-compose 编排，因此这里的“部署”主要指二进制分发与运行时目录准备。

### 二进制产物

推荐直接从源码构建：

```bash
go build -o ./imagine ./cmd/imagine
```

构建完成后，可以把 `imagine` 放到任意可执行目录，或在当前项目目录直接运行。

### 运行时准备

上线到某台开发机、跳板机或自动化环境时，至少需要准备：

- 一个可执行的 `imagine` 二进制
- 一个 provider 配置目录
- schema JSON 文件
- 通过环境变量或本地 secret 文件提供的 API key
- 一个可写的输出目录

如果目标环境需要代理访问 provider，可以在 provider 或 model 的 `endpoint.proxy_url` 中配置代理地址；model 级代理会覆盖 provider 级默认值。

## 5. 运维

### 常规检查命令

先校验配置是否能被加载：

```bash
./imagine --config <dir> config validate
```

再检查某个 tool 的最终请求：

```bash
./imagine --config <dir> inspect image.generate --args '{"model":"...","prompt":"..."}'
```

最后再做真实请求验证：

```bash
./imagine --config <dir> verify --output-dir ./tmp
```

### 常见排查

- 如果提示没有配置文件，先确认 `--config` 是否指向包含 `*.toml` 的目录
- 如果提示 schema 错误，先检查 `input_schema` 路径是否存在，且 JSON schema 能被正确编译
- 如果提示鉴权错误，检查 `api_key_env` 对应环境变量是否已注入，或 `api_key_file` 是否可读且内容非空
- 如果 `inspect` 正常但 `run`/`verify` 失败，优先排查 provider 网络连通性、代理配置和上游接口返回
- 如果产物没有落盘，确认 `--output-dir` 对当前用户可写

### 真实验证的注意事项

`verify` 不只是 schema dry-run，它会：

- 为 edit 场景写入一个本地 seed 图片
- 对每个已配置 capability 调用一次真实 provider 请求
- 在 `verification/<timestamp>/` 下输出产物和 `summary.md`

这意味着它可能消耗真实 provider 配额，也会受到网络状态影响。

### 退出码

- `0`：成功
- `2`：配置错误
- `3`：执行错误

## 常用命令

```bash
./imagine providers
./imagine models
./imagine model <model>
./imagine generate ...
./imagine edit ...
./imagine import ...
./imagine run <tool>
./imagine inspect <tool>
./imagine config validate
./imagine config import-yaml --from <src> --to <dst>
./imagine verify
./imagine version
```

`config import-yaml` 用于把旧版 YAML provider 配置迁移成当前的 TOML + schema JSON 目录结构。
