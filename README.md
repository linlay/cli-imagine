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
- `config import-yaml` 把仓库维护的 YAML 源配置转换成正式运行配置

它不再提供 HTTP MCP 服务；运行入口就是本地二进制 `imagine`。

## 2. 配置目录约定

当前仓库里有两套职责明确的配置目录：

- `configs/`
  - 仓库内维护的 YAML 源配置
  - 方便直接录入 provider API 细节和内嵌 schema
  - 当前仓库里的 `apiKey` 明文保持现状，这一轮没有改成 env/file
- `examples/`
  - 正式运行配置目录
  - 供 `--config ./examples` 直接加载
  - 内容是 `TOML + 外部 schema JSON`

运行时不会直接读取 `configs/*.yml`。

要从源 YAML 重新生成正式配置，执行：

```bash
go run ./cmd/imagine config import-yaml --from ./configs --to ./examples
```

`configs/provider.example.yml` 是新增 provider 时可参考的 YAML 模板。

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

### 生成正式配置并做 discovery

```bash
./imagine config import-yaml --from ./configs --to ./examples
./imagine --config ./examples config validate
./imagine --config ./examples providers
./imagine --config ./examples models --provider babelark
./imagine --config ./examples model gemini-2.5-flash-image
```

### 常见执行方式

直接走具名命令：

```bash
./imagine generate \
  --config ./examples \
  --model gemini-2.5-flash-image \
  --prompt "A red panda reading in a lantern-lit forest" \
  --arg size='"1024x1024"'

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
  --args '{"model":"gemini-2.5-flash-image","prompt":"otter","size":"1024x1024"}'

./imagine inspect image.generate \
  --config ./examples \
  --args '{"model":"gemini-2.5-flash-image","prompt":"otter","size":"1024x1024"}'
```

### 留在项目里的样图

仓库约定的样图生成命令是：

```bash
./imagine generate \
  --config ./examples \
  --model gemini-2.5-flash-image \
  --prompt "A clean technical cover image for a CLI image generation tool, terminal + config files + image workflow, flat modern illustration, square composition" \
  --arg size='"1024x1024"' \
  --output-dir ./examples/output \
  --output-name sample-babelark-gemini-2-5-flash-image.png
```

成功后会写出：

```text
examples/output/sample-babelark-gemini-2-5-flash-image.png
```

这个样图只作为项目内示例输出保留，不作为文档主视觉资产。

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

当前正式示例见：

- [examples/babelark.toml](./examples/babelark.toml)
- [examples/poe.toml](./examples/poe.toml)
- [examples/schemas/babelark/gemini-2-5-flash-image.generate.json](./examples/schemas/babelark/gemini-2-5-flash-image.generate.json)
- [examples/schemas/poe/GPT-Image-1.generate.json](./examples/schemas/poe/GPT-Image-1.generate.json)

### YAML 源配置与迁移

`configs/*.yml` 允许直接内嵌 `inputSchema`，更适合维护 provider API 细节。

`config import-yaml` 会把它们转换成：

- `examples/<provider>.toml`
- `examples/schemas/<provider>/*.json`

当前 `configs/` 中涉及的 request kind / parser 都已被运行时支持，包括：

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

当前仓库维护的 YAML 源配置仍保留明文 `apiKey`，这次没有切到 env/file；CLI 本身仍然支持三种鉴权写法。

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

先把 YAML 源配置生成成正式运行配置：

```bash
./imagine config import-yaml --from ./configs --to ./examples
```

再校验配置是否能被加载：

```bash
./imagine --config ./examples config validate
```

然后检查某个 tool 的最终请求：

```bash
./imagine --config ./examples inspect image.generate --args '{"model":"gemini-2.5-flash-image","prompt":"otter","size":"1024x1024"}'
```

最后再做真实请求验证：

```bash
./imagine --config ./examples verify --output-dir ./tmp
```

### 常见排查

- 如果提示没有配置文件，先确认 `--config` 是否指向包含 `*.toml` 的目录，而不是 `configs/`
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
