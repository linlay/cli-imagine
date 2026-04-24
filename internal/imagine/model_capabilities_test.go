package imagine

import (
	"strings"
	"testing"

	"github.com/linlay/cli-imagine/internal/config"
)

func TestParseGenerateInputShouldUseModelDefaultResponseFormat(t *testing.T) {
	input, err := parseGenerateInput(map[string]any{
		"model":  "gemini-2.5-flash-image",
		"prompt": "otter",
		"size":   "1024x1024",
	}, testModelCatalog())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if input.ResponseFormat != "b64_json" {
		t.Fatalf("expected default response format b64_json, got %s", input.ResponseFormat)
	}
	if input.ParserKind != parserKindDataB64JSON {
		t.Fatalf("unexpected parser kind: %s", input.ParserKind)
	}
	if input.ProxyURL != "" {
		t.Fatalf("expected empty proxy url, got %s", input.ProxyURL)
	}
}

func TestParseGenerateInputShouldPropagateProxyURL(t *testing.T) {
	input, err := parseGenerateInput(map[string]any{
		"model":  "GPT-Image-1",
		"prompt": "otter",
		"size":   "1:1",
	}, testModelCatalog())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if input.ProxyURL != "http://127.0.0.1:8001" {
		t.Fatalf("expected proxy url to propagate, got %s", input.ProxyURL)
	}
	if input.TimeoutMs != 120000 {
		t.Fatalf("expected timeout to propagate, got %d", input.TimeoutMs)
	}
}

func TestParseGenerateInputShouldRejectUnsupportedModel(t *testing.T) {
	_, err := parseGenerateInput(map[string]any{
		"model":  "missing-model",
		"prompt": "otter",
		"size":   "1024x1024",
	}, testModelCatalog())
	if err == nil || !strings.Contains(err.Error(), "unsupported model") {
		t.Fatalf("expected unsupported model error, got %v", err)
	}
}

func TestParseGenerateInputShouldRejectDisallowedResponseFormat(t *testing.T) {
	_, err := parseGenerateInput(map[string]any{
		"model":          "GPT-Image-1",
		"prompt":         "otter",
		"size":           "1:1",
		"responseFormat": "b64_json",
	}, testModelCatalog())
	if err == nil || !strings.Contains(err.Error(), "responseFormat") {
		t.Fatalf("expected responseFormat error, got %v", err)
	}
}

func TestParseGenerateInputShouldUseSizeAliasForRatioModel(t *testing.T) {
	input, err := parseGenerateInput(map[string]any{
		"model":     "gemini-3.1-flash-image-preview",
		"prompt":    "otter",
		"size":      "9:16",
		"imageSize": "2K",
	}, testModelCatalog())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if input.AspectRatio != "9:16" {
		t.Fatalf("expected aspect ratio alias, got %s", input.AspectRatio)
	}
	if input.Size != "" {
		t.Fatalf("expected normalized size to be empty, got %s", input.Size)
	}
}

func TestParseGenerateInputShouldRejectConflictingSizeAndAspectRatio(t *testing.T) {
	_, err := parseGenerateInput(map[string]any{
		"model":       "gemini-3.1-flash-image-preview",
		"prompt":      "otter",
		"size":        "9:16",
		"aspectRatio": "1:1",
		"imageSize":   "2K",
	}, testModelCatalog())
	if err == nil || !strings.Contains(err.Error(), "must match") {
		t.Fatalf("expected aspect conflict error, got %v", err)
	}
}

func TestParseGenerateInputShouldRequireImageSizeForPreviewModel(t *testing.T) {
	_, err := parseGenerateInput(map[string]any{
		"model":  "gemini-3-pro-image-preview",
		"prompt": "otter",
		"size":   "9:16",
	}, testModelCatalog())
	if err == nil || !strings.Contains(err.Error(), "imageSize") {
		t.Fatalf("expected imageSize required error, got %v", err)
	}
}

func TestParseGenerateInputShouldRejectUnsupportedModelSpecificFlags(t *testing.T) {
	_, err := parseGenerateInput(map[string]any{
		"model":   "grok-imagine-image",
		"prompt":  "otter",
		"size":    "1:1",
		"useMask": true,
	}, testModelCatalog())
	if err == nil || !strings.Contains(err.Error(), "is not allowed") {
		t.Fatalf("expected unsupported flag schema error, got %v", err)
	}
}

func TestParseGenerateInputShouldRejectUnsupportedImageSize(t *testing.T) {
	_, err := parseGenerateInput(map[string]any{
		"model":     "gemini-2.5-flash-image",
		"prompt":    "otter",
		"size":      "1024x1024",
		"imageSize": "2K",
	}, testModelCatalog())
	if err == nil || !strings.Contains(err.Error(), "is not allowed") {
		t.Fatalf("expected unsupported imageSize error, got %v", err)
	}
}

func TestParseGenerateInputShouldRejectUnsupportedAspectRatioForStandardModel(t *testing.T) {
	_, err := parseGenerateInput(map[string]any{
		"model":       "gemini-2.5-flash-image",
		"prompt":      "otter",
		"size":        "1024x1024",
		"aspectRatio": "1:1",
	}, testModelCatalog())
	if err == nil || !strings.Contains(err.Error(), "is not allowed") {
		t.Fatalf("expected unsupported aspectRatio error, got %v", err)
	}
}

func TestParseGenerateInputShouldApplyConfiguredDefaultFlags(t *testing.T) {
	input, err := parseGenerateInput(map[string]any{
		"model":  "nano-banana-pro",
		"prompt": "otter",
		"size":   "4:3",
	}, testModelCatalog())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if input.ImageOnly == nil || !*input.ImageOnly {
		t.Fatal("expected imageOnly default to be true")
	}
	if input.WebSearch == nil || !*input.WebSearch {
		t.Fatal("expected webSearch default to be true")
	}
}

func TestParseEditInputShouldRejectModelWithoutEditOperation(t *testing.T) {
	_, err := parseEditInput(map[string]any{
		"model":  "gemini-2.5-flash-image",
		"prompt": "poster",
		"images": []any{"input.png"},
		"size":   "1024x1024",
	}, testModelCatalog())
	if err == nil || !strings.Contains(err.Error(), "does not support edit") {
		t.Fatalf("expected edit operation error, got %v", err)
	}
}

func testModelCatalog() ModelCatalog {
	return NewModelCatalog([]config.ModelConfig{
		{
			Name:     "gemini-2.5-flash-image",
			Endpoint: config.ModelEndpointConfig{BaseURL: "https://image.primary/v1"},
			Auth:     config.ModelAuthConfig{APIKey: "test-primary-key"},
			Capabilities: config.ModelCapabilitiesConfig{
				Generate: capabilityConfig(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"model":          map[string]any{"type": "string", "minLength": 1},
						"prompt":         map[string]any{"type": "string", "minLength": 1},
						"size":           map[string]any{"type": "string", "enum": []string{"1024x1024"}},
						"responseFormat": map[string]any{"type": "string", "enum": []string{"b64_json", "url"}},
					},
					"required":             []string{"model", "prompt", "size"},
					"additionalProperties": false,
				}, config.ModelRequestConfig{
					Kind:     requestKindImagesGenerate,
					SizeMode: sizeModeStandard,
				}, config.ModelResponseConfig{
					DefaultFormat:  "b64_json",
					AllowedFormats: []string{"b64_json", "url"},
					ParserByFormat: map[string]string{
						"b64_json": parserKindDataB64JSON,
						"url":      parserKindDataURL,
					},
				}),
			},
		},
		{
			Name:     "gemini-2.5-flash-image-edit",
			Endpoint: config.ModelEndpointConfig{BaseURL: "https://image.primary/v1"},
			Auth:     config.ModelAuthConfig{APIKey: "test-primary-key"},
			Capabilities: config.ModelCapabilitiesConfig{
				Edit: capabilityConfig(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"model":  map[string]any{"type": "string", "minLength": 1},
						"prompt": map[string]any{"type": "string", "minLength": 1},
						"images": map[string]any{
							"type":     "array",
							"minItems": 1,
							"items":    map[string]any{"type": "string", "minLength": 1},
						},
						"size": map[string]any{"type": "string", "enum": []string{"1024x1024"}},
					},
					"required":             []string{"model", "prompt", "images", "size"},
					"additionalProperties": false,
				}, config.ModelRequestConfig{
					Kind:     requestKindImagesEdit,
					SizeMode: sizeModeStandard,
				}, config.ModelResponseConfig{
					DefaultFormat: "b64_json",
					ParserByFormat: map[string]string{
						"b64_json": parserKindDataB64JSON,
					},
				}),
			},
		},
		{
			Name:     "gemini-3.1-flash-image-preview",
			Endpoint: config.ModelEndpointConfig{BaseURL: "https://image.primary/v1"},
			Auth:     config.ModelAuthConfig{APIKey: "test-primary-key"},
			Capabilities: config.ModelCapabilitiesConfig{
				Generate: capabilityConfig(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"model":       map[string]any{"type": "string", "minLength": 1},
						"prompt":      map[string]any{"type": "string", "minLength": 1},
						"size":        map[string]any{"type": "string", "enum": []string{"1:1", "9:16"}},
						"aspectRatio": map[string]any{"type": "string", "enum": []string{"1:1", "9:16"}},
						"imageSize":   map[string]any{"type": "string", "enum": []string{"1024", "2K"}},
					},
					"required": []string{"model", "prompt", "imageSize"},
					"anyOf": []map[string]any{
						{"required": []string{"size"}},
						{"required": []string{"aspectRatio"}},
					},
					"additionalProperties": false,
				}, config.ModelRequestConfig{
					Kind:           requestKindGenerateContent,
					SizeMode:       sizeModeRatio,
					AspectField:    "aspectRatio",
					ImageSizeField: "imageSize",
				}, config.ModelResponseConfig{
					DefaultFormat: "b64_json",
					ParserByFormat: map[string]string{
						"b64_json": parserKindCandidatesInline,
					},
				}),
			},
		},
		{
			Name:     "gemini-3-pro-image-preview",
			Endpoint: config.ModelEndpointConfig{BaseURL: "https://image.primary/v1"},
			Auth:     config.ModelAuthConfig{APIKey: "test-primary-key"},
			Capabilities: config.ModelCapabilitiesConfig{
				Generate: capabilityConfig(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"model":       map[string]any{"type": "string", "minLength": 1},
						"prompt":      map[string]any{"type": "string", "minLength": 1},
						"size":        map[string]any{"type": "string", "enum": []string{"1:1", "9:16"}},
						"aspectRatio": map[string]any{"type": "string", "enum": []string{"1:1", "9:16"}},
						"imageSize":   map[string]any{"type": "string", "enum": []string{"1024", "2K"}},
					},
					"required": []string{"model", "prompt", "imageSize"},
					"anyOf": []map[string]any{
						{"required": []string{"size"}},
						{"required": []string{"aspectRatio"}},
					},
					"additionalProperties": false,
				}, config.ModelRequestConfig{
					Kind:           requestKindGenerateContent,
					SizeMode:       sizeModeRatio,
					AspectField:    "aspectRatio",
					ImageSizeField: "imageSize",
				}, config.ModelResponseConfig{
					DefaultFormat: "b64_json",
					ParserByFormat: map[string]string{
						"b64_json": parserKindCandidatesInline,
					},
				}),
			},
		},
		{
			Name:     "grok-imagine-image",
			Endpoint: config.ModelEndpointConfig{BaseURL: "https://image.primary/v1"},
			Auth:     config.ModelAuthConfig{APIKey: "test-primary-key"},
			Capabilities: config.ModelCapabilitiesConfig{
				Generate: capabilityConfig(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"model":  map[string]any{"type": "string", "minLength": 1},
						"prompt": map[string]any{"type": "string", "minLength": 1},
						"size":   map[string]any{"type": "string", "enum": []string{"1:1", "4:3"}},
					},
					"required":             []string{"model", "prompt", "size"},
					"additionalProperties": false,
				}, config.ModelRequestConfig{
					Kind:        requestKindChatCompletions,
					SizeMode:    sizeModeRatio,
					AspectField: "aspect",
				}, config.ModelResponseConfig{
					DefaultFormat: "url",
					ParserByFormat: map[string]string{
						"url": parserKindMessageContentImage,
					},
				}),
			},
		},
		{
			Name:     "nano-banana-pro",
			Endpoint: config.ModelEndpointConfig{BaseURL: "https://image.primary/v1"},
			Auth:     config.ModelAuthConfig{APIKey: "test-primary-key"},
			Capabilities: config.ModelCapabilitiesConfig{
				Generate: capabilityConfig(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"model":  map[string]any{"type": "string", "minLength": 1},
						"prompt": map[string]any{"type": "string", "minLength": 1},
						"size":   map[string]any{"type": "string", "enum": []string{"4:3"}},
					},
					"required":             []string{"model", "prompt", "size"},
					"additionalProperties": false,
				}, config.ModelRequestConfig{
					Kind:           requestKindChatCompletions,
					SizeMode:       sizeModeRatio,
					AspectField:    "aspect_ratio",
					ImageSizeField: "image_size",
					DefaultArguments: map[string]any{
						"imageOnly": true,
						"webSearch": true,
					},
				}, config.ModelResponseConfig{
					DefaultFormat: "url",
					ParserByFormat: map[string]string{
						"url": parserKindMessageContentImage,
					},
				}),
			},
		},
		{
			Name:     "GPT-Image-1",
			Endpoint: config.ModelEndpointConfig{BaseURL: "https://image.secondary/v1", ProxyURL: "http://127.0.0.1:8001", TimeoutMs: 120000},
			Auth:     config.ModelAuthConfig{APIKey: "test-secondary-key"},
			Capabilities: config.ModelCapabilitiesConfig{
				Generate: capabilityConfig(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"model":   map[string]any{"type": "string", "minLength": 1},
						"prompt":  map[string]any{"type": "string", "minLength": 1},
						"size":    map[string]any{"type": "string", "enum": []string{"1:1", "1024x1024"}},
						"quality": map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
					},
					"required":             []string{"model", "prompt", "size"},
					"additionalProperties": false,
				}, config.ModelRequestConfig{
					Kind:        requestKindChatCompletions,
					SizeMode:    sizeModeRatioOrPixel,
					AspectField: "aspect",
				}, config.ModelResponseConfig{
					DefaultFormat: "url",
					ParserByFormat: map[string]string{
						"url": parserKindMessageContentImage,
					},
				}),
			},
		},
	})
}

func capabilityConfig(inputSchema map[string]any, request config.ModelRequestConfig, response config.ModelResponseConfig) *config.ModelCapabilityConfig {
	return &config.ModelCapabilityConfig{
		InputSchema: inputSchema,
		Request:     request,
		Response:    response,
	}
}
