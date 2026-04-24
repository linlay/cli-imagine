# cli-imagine

`cli-imagine` 是一个 Go 1.26 的本地图像 CLI，把不同 provider 的图像模型能力收敛成统一命令行调用。

用户安装、配置、验证和排障看这个 README；项目分层、内部语义和维护约定见 [CLAUDE.md](./CLAUDE.md)。

## 1. 项目简介

`cli-imagine` 解决的是“把不同图像 provider 的模型能力配置化，然后用统一 CLI 执行生图、改图、导入和检查”这个问题。

当前仓库提供：

- provider / model / operation discovery
- `image.generate`、`image.edit`、`image.import`
- `inspect` 预览最终请求而不真正发请求
- `verify` 对当前配置执行真实连通性验证
- `config import-yaml` 把外部 YAML 源配置转换成正式运行配置

它不再提供 HTTP MCP 服务；运行入口就是本地二进制 `imagine`。

## 2. 配置目录约定

当前仓库的配置与输出目录约定：

- `configs/`
  - 可提交的 provider 配置模板与 schema
  - `*.toml.example` 只放结构和环境变量名，不放真实账号
  - 本地运行时复制成 `*.toml`，这些真实账号文件会被 `.gitignore` 过滤
- `examples/`
  - 本地实验和输出目录
  - 整个目录会被 `.gitignore` 过滤
- `output/`
  - 推荐的本地图片输出目录
  - 整个目录会被 `.gitignore` 过滤

运行时只扫描配置目录当前层级的 `*.toml`，不会读取 `*.toml.example`。

首次本地运行前，按需从模板复制真实配置：

```bash
cp ./configs/babelark.toml.example ./configs/babelark.toml
cp ./configs/poe.toml.example ./configs/poe.toml
export BABELARK_API_KEY=...
export POE_API_KEY=...
```

`configs/*.toml` 属于本机账号配置，不进入 git。

## 3. 快速开始

### 前置要求

- Go 1.26
- 可访问的图像 provider endpoint
- 对输出目录有写权限

### 本地构建

```bash
go build -o ./imagine ./cmd/imagine
./imagine version
./imagine --help
```

### 准备本地配置并做 discovery

```bash
cp ./configs/babelark.toml.example ./configs/babelark.toml
export BABELARK_API_KEY=...
./imagine --config ./configs config validate
./imagine --config ./configs providers
./imagine --config ./configs models --provider babelark
./imagine --config ./configs model gemini-2.5-flash-image
```

### 常见执行方式

直接走具名命令：

```bash
./imagine generate \
  --config ./configs \
  --model gemini-2.5-flash-image \
  --prompt "A red panda reading in a lantern-lit forest" \
  --arg size='"1024x1024"'

./imagine edit \
  --config ./configs \
  --model gemini-2.5-flash-image-edit \
  --prompt "Turn this into a poster" \
  --image ./seed.png \
  --arg size='"1024x1024"'

./imagine import \
  --output-dir ./output \
  --item '{"type":"url","value":"https://example.com/image.png"}'
```

或者直接调用 tool 名：

```bash
./imagine run image.generate \
  --config ./configs \
  --args '{"model":"gemini-2.5-flash-image","prompt":"otter","size":"1024x1024"}'

./imagine inspect image.generate \
  --config ./configs \
  --args '{"model":"gemini-2.5-flash-image","prompt":"otter","size":"1024x1024"}'
```

### 本地样图输出

本地生成样图时可以使用：

```bash
./imagine generate \
  --config ./configs \
  --model gemini-2.5-flash-image \
  --prompt "A clean technical cover image for a CLI image generation tool, terminal + config files + image workflow, flat modern illustration, square composition" \
  --arg size='"1024x1024"' \
  --output-dir ./output \
  --output-name sample-babelark-gemini-2-5-flash-image.png
```

成功后会写出：

```text
output/sample-babelark-gemini-2-5-flash-image.png
```

`output/` 会被 `.gitignore` 过滤，不作为文档主视觉资产。

## 4. 配置说明

### 默认配置目录

默认配置目录为：

```text
$XDG_CONFIG_HOME/imagine
```

如果未设置 `XDG_CONFIG_HOME`，则回落到：

```text
~/.config/imagine
```

也可以通过 `--config <dir>` 显式指定。

### 正式运行配置规则

运行时正式读取的是 `TOML + 外部 schema JSON`：

- 根目录只扫描当前层级的 `*.toml`
- provider 名不能重复
- model 名不能重复
- 每个 provider 必须声明 `endpoint.base_url`
- 每个 model 至少包含一个 capability：`generate` 或 `edit`
- `input_schema` 相对路径相对于 provider TOML 所在目录解析

当前可提交模板见；真实账号配置使用 `configs/babelark.toml` / `configs/poe.toml`，这两个文件会被 `.gitignore` 过滤：

- [configs/babelark.toml.example](./configs/babelark.toml.example)
- [configs/poe.toml.example](./configs/poe.toml.example)
- [configs/schemas/babelark/gemini-2-5-flash-image.generate.json](./configs/schemas/babelark/gemini-2-5-flash-image.generate.json)
- [configs/schemas/poe/GPT-Image-1.generate.json](./configs/schemas/poe/GPT-Image-1.generate.json)

### YAML 源配置与迁移

`config import-yaml` 仍支持从外部 YAML 源配置生成 `TOML + 外部 schema JSON`。YAML 允许直接内嵌 `inputSchema`，更适合维护 provider API 细节。

`config import-yaml` 会把它们转换成：

- `<target>/<provider>.toml`
- `<target>/schemas/<provider>/*.json`

当前 `configs/` 模板中涉及的 request kind / parser 都已被运行时支持，包括：

- `images_generate`
- `images_edit`
- `generate_content`
- `chat_completions`
- `data_b64_json`
- `data_url`
- `candidates_inline_data`
- `message_content_image`

### 鉴权与 endpoint 覆盖

provider 和 model 都可以声明 `endpoint` 与 `auth`。合并规则是：

- model 级 `endpoint` 会覆盖 provider 级默认值
- model 级 `auth` 只要显式填写，就整体覆盖 provider 级 `auth`
- `auth` 必须且只能设置 `api_key`、`api_key_env`、`api_key_file` 三者之一
- 如果使用 `api_key_env` 或 `api_key_file`，最终解析出的 key 不能为空

当前仓库提交的模板使用 `api_key_env`，CLI 本身仍然支持三种鉴权写法。

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

manifest 会记录每个产物的：

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

## 5. 运维

### 常规检查命令

先准备本地账号配置：

```bash
cp ./configs/babelark.toml.example ./configs/babelark.toml
export BABELARK_API_KEY=...
```

再校验配置是否能被加载：

```bash
./imagine --config ./configs config validate
```

然后检查某个 tool 的最终请求：

```bash
./imagine --config ./configs inspect image.generate --args '{"model":"gemini-2.5-flash-image","prompt":"otter","size":"1024x1024"}'
```

最后再做真实请求验证：

```bash
./imagine --config ./configs verify --output-dir ./output
```

### 常见排查

- 如果提示没有配置文件，先确认 `--config` 是否指向包含本地 `*.toml` 的目录，而不是只有 `*.toml.example`
- 如果提示 schema 错误，先检查 `input_schema` 路径是否存在，且 JSON schema 能被正确编译
- 如果提示鉴权错误，检查当前 TOML 中使用的是 `api_key`、`api_key_env` 还是 `api_key_file`
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

## 6. 常用命令

```bash
./imagine providers
./imagine models --provider <provider>
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
