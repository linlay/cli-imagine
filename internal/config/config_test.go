package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDirShouldResolveProviderDefaultsAndOverrides(t *testing.T) {
	dir := t.TempDir()
	writeSchema(t, filepath.Join(dir, "schemas/provider/model-a.generate.json"), map[string]any{
		"type": "object",
		"properties": map[string]any{
			"model":  map[string]any{"type": "string"},
			"prompt": map[string]any{"type": "string"},
			"size":   map[string]any{"type": "string", "enum": []string{"1:1"}},
		},
		"required":             []string{"model", "prompt", "size"},
		"additionalProperties": false,
	})
	writeSchema(t, filepath.Join(dir, "schemas/provider/model-b.edit.json"), map[string]any{
		"type": "object",
		"properties": map[string]any{
			"model":  map[string]any{"type": "string"},
			"prompt": map[string]any{"type": "string"},
			"images": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1},
			"size":   map[string]any{"type": "string", "enum": []string{"1024x1024"}},
		},
		"required":             []string{"model", "prompt", "images", "size"},
		"additionalProperties": false,
	})
	writeFile(t, filepath.Join(dir, "provider.toml"), `
name = "provider"
[endpoint]
base_url = "https://api.example.com/v1"
proxy_url = "http://127.0.0.1:8001"
timeout_ms = 45000
[auth]
api_key = "provider-key"

[[models]]
name = "model-a"
[models.capabilities.generate]
input_schema = "schemas/provider/model-a.generate.json"
[models.capabilities.generate.request]
kind = "chat_completions"
size_mode = "ratio"
aspect_field = "aspect"
[models.capabilities.generate.response]
default_format = "url"
parser_by_format = { url = "message_content_image" }

[[models]]
name = "model-b"
[models.endpoint]
base_url = "https://override.example.com/v1"
timeout_ms = 90000
[models.auth]
api_key = "model-key"
[models.capabilities.edit]
input_schema = "schemas/provider/model-b.edit.json"
[models.capabilities.edit.request]
kind = "images_edit"
size_mode = "standard"
[models.capabilities.edit.response]
default_format = "b64_json"
parser_by_format = { b64_json = "data_b64_json" }
`)

	cfg, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(cfg.Providers) != 1 || len(cfg.Models) != 2 {
		t.Fatalf("unexpected config counts: %#v", cfg)
	}
	if cfg.Models[0].Endpoint.BaseURL != "https://api.example.com/v1" {
		t.Fatalf("expected provider endpoint inheritance, got %s", cfg.Models[0].Endpoint.BaseURL)
	}
	if cfg.Models[1].Endpoint.BaseURL != "https://override.example.com/v1" {
		t.Fatalf("expected model endpoint override, got %s", cfg.Models[1].Endpoint.BaseURL)
	}
	if cfg.Models[0].Endpoint.TimeoutMs != 45000 {
		t.Fatalf("expected provider timeout inheritance, got %d", cfg.Models[0].Endpoint.TimeoutMs)
	}
	if cfg.Models[1].Endpoint.TimeoutMs != 90000 {
		t.Fatalf("expected model timeout override, got %d", cfg.Models[1].Endpoint.TimeoutMs)
	}
	if cfg.Models[1].Auth.APIKey != "model-key" {
		t.Fatalf("expected model auth override, got %s", cfg.Models[1].Auth.APIKey)
	}
}

func TestLoadDirShouldResolveEnvAndFileAuth(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.json")
	writeSchema(t, schemaPath, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"model":  map[string]any{"type": "string"},
			"prompt": map[string]any{"type": "string"},
			"size":   map[string]any{"type": "string", "enum": []string{"1:1"}},
		},
		"required":             []string{"model", "prompt", "size"},
		"additionalProperties": false,
	})
	secretFile := filepath.Join(dir, "secret.txt")
	writeFile(t, secretFile, " file-key \n")
	t.Setenv("IMAGINE_KEY", "env-key")

	writeFile(t, filepath.Join(dir, "env.toml"), `
name = "env-provider"
[endpoint]
base_url = "https://api.example.com"
[auth]
api_key_env = "IMAGINE_KEY"
[[models]]
name = "env-model"
[models.capabilities.generate]
input_schema = "schema.json"
[models.capabilities.generate.request]
kind = "images_generate"
size_mode = "standard"
[models.capabilities.generate.response]
default_format = "b64_json"
parser_by_format = { b64_json = "data_b64_json" }
`)
	writeFile(t, filepath.Join(dir, "file.toml"), `
name = "file-provider"
[endpoint]
base_url = "https://api.example.com"
[auth]
api_key_file = "`+secretFile+`"
[[models]]
name = "file-model"
[models.capabilities.generate]
input_schema = "schema.json"
[models.capabilities.generate.request]
kind = "images_generate"
size_mode = "standard"
[models.capabilities.generate.response]
default_format = "b64_json"
parser_by_format = { b64_json = "data_b64_json" }
`)

	cfg, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	foundEnv := false
	foundFile := false
	for _, model := range cfg.Models {
		switch model.Name {
		case "env-model":
			foundEnv = true
			if model.Auth.APIKey != "env-key" {
				t.Fatalf("expected env api key, got %s", model.Auth.APIKey)
			}
		case "file-model":
			foundFile = true
			if model.Auth.APIKey != "file-key" {
				t.Fatalf("expected file api key, got %s", model.Auth.APIKey)
			}
		}
	}
	if !foundEnv || !foundFile {
		t.Fatalf("expected both models to load, got %#v", cfg.Models)
	}
}

func TestLoadDirShouldFailOnBadSchemaJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "broken.json"), "{not-json")
	writeFile(t, filepath.Join(dir, "provider.toml"), `
name = "provider"
[endpoint]
base_url = "https://api.example.com"
[auth]
api_key = "test"
[[models]]
name = "broken-model"
[models.capabilities.generate]
input_schema = "broken.json"
[models.capabilities.generate.request]
kind = "images_generate"
size_mode = "standard"
[models.capabilities.generate.response]
default_format = "b64_json"
parser_by_format = { b64_json = "data_b64_json" }
`)
	_, err := LoadDir(dir)
	if err == nil || !strings.Contains(err.Error(), "decode input schema") {
		t.Fatalf("expected schema decode failure, got %v", err)
	}
}

func TestImportYAMLShouldWriteTOMLAndSchemas(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "provider.yml"), `
name: provider
endpoint:
  baseUrl: https://api.example.com/v1
auth:
  apiKey: provider-key
models:
  - name: model-a
    capabilities:
      generate:
        inputSchema:
          type: object
          properties:
            model:
              type: string
            prompt:
              type: string
            size:
              type: string
              enum: ["1:1"]
          required: [model, prompt, size]
          additionalProperties: false
        request:
          kind: chat_completions
          sizeMode: ratio
          aspectField: aspect
        response:
          defaultFormat: url
          parserByFormat:
            url: message_content_image
`)

	written, err := ImportYAML(src, dst)
	if err != nil {
		t.Fatalf("ImportYAML: %v", err)
	}
	if len(written) != 2 {
		t.Fatalf("expected toml and schema outputs, got %v", written)
	}
	if _, err := os.Stat(filepath.Join(dst, "provider.toml")); err != nil {
		t.Fatalf("expected provider.toml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "schemas/provider/model-a.generate.json")); err != nil {
		t.Fatalf("expected schema file: %v", err)
	}
	cfg, err := LoadDir(dst)
	if err != nil {
		t.Fatalf("LoadDir on imported config: %v", err)
	}
	if len(cfg.Models) != 1 || cfg.Models[0].Name != "model-a" {
		t.Fatalf("unexpected imported config: %#v", cfg.Models)
	}
}

func TestRepositoryConfigTemplatesShouldLoadWhenCopiedToLocalTOML(t *testing.T) {
	repoConfigs := filepath.Join("..", "..", "configs")
	dir := t.TempDir()
	copyFile(t, filepath.Join(repoConfigs, "babelark.toml.example"), filepath.Join(dir, "babelark.toml"))
	copyFile(t, filepath.Join(repoConfigs, "poe.toml.example"), filepath.Join(dir, "poe.toml"))
	copyDir(t, filepath.Join(repoConfigs, "schemas"), filepath.Join(dir, "schemas"))
	t.Setenv("BABELARK_API_KEY", "test-babelark-key")
	t.Setenv("POE_API_KEY", "test-poe-key")

	cfg, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir on copied repository config templates: %v", err)
	}
	if len(cfg.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(cfg.Providers))
	}
	if len(cfg.Models) != 10 {
		t.Fatalf("expected 10 models, got %d", len(cfg.Models))
	}
	model, ok := LookupModel(cfg, "gemini-2.5-flash-image")
	if !ok {
		t.Fatal("expected gemini-2.5-flash-image to be imported")
	}
	if model.Capabilities.Generate == nil || model.Capabilities.Generate.Request.Kind != "images_generate" {
		t.Fatalf("unexpected generate capability: %#v", model.Capabilities.Generate)
	}
}

func writeSchema(t *testing.T, path string, schema map[string]any) {
	t.Helper()
	payload, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	writeFile(t, path, string(payload))
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read file %s: %v", src, err)
	}
	writeFile(t, dst, string(raw))
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	if err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o644)
	}); err != nil {
		t.Fatalf("copy dir %s to %s: %v", src, dst, err)
	}
}
