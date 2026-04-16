package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteShouldAllowModelsCommandLocalFlags(t *testing.T) {
	configDir := writeProviderConfig(t, newGenerateProviderConfig("provider", "model-a", "https://api.example.com"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Execute([]string{"models", "--config", configDir, "--provider", "provider"}, strings.NewReader(""), &stdout, &stderr)
	if code != ExitSuccess {
		t.Fatalf("expected success, got code %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "model-a") {
		t.Fatalf("expected model listing, got %q", stdout.String())
	}
}

func TestExecuteShouldAllowImportYAMLFlags(t *testing.T) {
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
              enum: ["1024x1024"]
          required: [model, prompt, size]
          additionalProperties: false
        request:
          kind: images_generate
          sizeMode: standard
        response:
          defaultFormat: b64_json
          parserByFormat:
            b64_json: data_b64_json
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Execute([]string{"config", "import-yaml", "--from", src, "--to", dst}, strings.NewReader(""), &stdout, &stderr)
	if code != ExitSuccess {
		t.Fatalf("expected success, got code %d stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dst, "provider.toml")); err != nil {
		t.Fatalf("expected provider.toml output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "schemas", "provider", "model-a.generate.json")); err != nil {
		t.Fatalf("expected schema output: %v", err)
	}
}

func TestExecuteShouldAllowGenerateCommandLocalFlags(t *testing.T) {
	configDir := writeProviderConfig(t, newGenerateProviderConfig("babelark", "gemini-2.5-flash-image", "https://api.example.com"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Execute([]string{
		"generate",
		"--config", configDir,
		"--model", "gemini-2.5-flash-image",
		"--prompt", "technical cover",
		"--arg", `size="1024x1024"`,
		"--help",
	}, strings.NewReader(""), &stdout, &stderr)
	if code != ExitSuccess {
		t.Fatalf("expected success, got code %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "--model Model name") {
		t.Fatalf("expected generate help output, got %q", stdout.String())
	}
}

func newGenerateProviderConfig(providerName, modelName, baseURL string) string {
	return `
name = "` + providerName + `"
[endpoint]
base_url = "` + baseURL + `"
[auth]
api_key = "test-key"

[[models]]
name = "` + modelName + `"
[models.capabilities.generate]
input_schema = "schemas/` + providerName + `/` + modelName + `.generate.json"
[models.capabilities.generate.request]
kind = "images_generate"
size_mode = "standard"
[models.capabilities.generate.response]
default_format = "b64_json"
parser_by_format = { b64_json = "data_b64_json" }
`
}

func writeProviderConfig(t *testing.T, providerTOML string) string {
	t.Helper()
	dir := t.TempDir()
	providerName, modelName := extractProviderAndModelNames(providerTOML)
	writeFile(t, filepath.Join(dir, "provider.toml"), providerTOML)
	writeFile(t, filepath.Join(dir, "schemas", providerName, modelName+".generate.json"), `{
  "type": "object",
  "properties": {
    "model": { "type": "string" },
    "prompt": { "type": "string" },
    "size": { "type": "string", "enum": ["1024x1024"] },
    "outputName": { "type": "string" }
  },
  "required": ["model", "prompt", "size"],
  "additionalProperties": false
}`)
	return dir
}

func extractProviderAndModelNames(providerTOML string) (string, string) {
	var providerName string
	var modelName string
	for _, line := range strings.Split(providerTOML, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "name = ") {
			value := strings.Trim(strings.TrimPrefix(trimmed, "name = "), `"`)
			if providerName == "" {
				providerName = value
				continue
			}
			if modelName == "" {
				modelName = value
				break
			}
		}
	}
	return providerName, modelName
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}
