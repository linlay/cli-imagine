package imagine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrimaryModeShouldHandleURLResponse(t *testing.T) {
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes())
	}))
	defer imageServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"url": imageServer.URL + "/image.png"}},
		})
	}))
	defer apiServer.Close()

	client := NewImageProviderClient(apiServer.Client(), 1024*1024, 4*1024*1024, 30000)
	result, err := client.Generate(context.Background(), GenerateRequest{
		Model:          "gemini-2.5-flash-image",
		BaseURL:        apiServer.URL,
		Prompt:         "otter",
		Size:           "1024x1024",
		RequestKind:    requestKindImagesGenerate,
		ResponseFormat: "url",
		ParserKind:     parserKindDataURL,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.MimeType != "image/png" {
		t.Fatalf("unexpected mime type: %s", result.MimeType)
	}
}

func TestGenerateContentModeShouldBuildRequest(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-3.1-flash-image-preview:generateContent" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		generationConfig := body["generationConfig"].(map[string]any)
		imageConfig := generationConfig["imageConfig"].(map[string]any)
		if imageConfig["aspectRatio"] != "9:16" || imageConfig["imageSize"] != "2K" {
			t.Fatalf("unexpected image config: %#v", imageConfig)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"parts": []map[string]any{{
						"inlineData": map[string]any{
							"mimeType": "image/png",
							"data":     base64.StdEncoding.EncodeToString(pngBytes()),
						},
					}},
				},
			}},
		})
	}))
	defer apiServer.Close()

	client := NewImageProviderClient(apiServer.Client(), 1024*1024, 4*1024*1024, 30000)
	result, err := client.Generate(context.Background(), GenerateRequest{
		Model:          "gemini-3.1-flash-image-preview",
		BaseURL:        apiServer.URL,
		Prompt:         "otter",
		AspectRatio:    "9:16",
		ImageSize:      "2K",
		RequestKind:    requestKindGenerateContent,
		AspectField:    "aspectRatio",
		ImageSizeField: "imageSize",
		ResponseFormat: "b64_json",
		ParserKind:     parserKindCandidatesInline,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.MimeType != "image/png" {
		t.Fatalf("unexpected mime type: %s", result.MimeType)
	}
}

func TestEditModeShouldHandleB64JSONResponse(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString(pngBytes())}},
		})
	}))
	defer apiServer.Close()

	client := NewImageProviderClient(apiServer.Client(), 1024*1024, 4*1024*1024, 30000)
	result, err := client.Edit(context.Background(), EditRequest{
		Model:          "gemini-2.5-flash-image-edit",
		BaseURL:        apiServer.URL,
		Prompt:         "poster",
		Images:         []string{"data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes())},
		Size:           "1024x1024",
		RequestKind:    requestKindImagesEdit,
		ResponseFormat: "b64_json",
		ParserKind:     parserKindDataB64JSON,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.MimeType != "image/png" {
		t.Fatalf("unexpected mime type: %s", result.MimeType)
	}
}

func TestChatCompletionsModeShouldBuildExtraBody(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		extraBody := body["extra_body"].(map[string]any)
		if extraBody["aspect_ratio"] != "4:3" || extraBody["image_size"] != "2K" {
			t.Fatalf("unexpected extra body: %#v", extraBody)
		}
		if extraBody["image_only"] != true || extraBody["web_search"] != true {
			t.Fatalf("unexpected flags: %#v", extraBody)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": []map[string]any{{
						"type": "file",
						"file": map[string]any{
							"file_data": "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes()),
						},
					}},
				},
			}},
		})
	}))
	defer apiServer.Close()

	client := NewImageProviderClient(apiServer.Client(), 1024*1024, 4*1024*1024, 30000)
	result, err := client.Generate(context.Background(), GenerateRequest{
		Model:          "nano-banana-pro",
		BaseURL:        apiServer.URL + "/v1",
		Prompt:         "otter",
		AspectRatio:    "4:3",
		ImageSize:      "2K",
		RequestKind:    requestKindChatCompletions,
		AspectField:    "aspect_ratio",
		ImageSizeField: "image_size",
		ImageOnly:      boolPtr(true),
		WebSearch:      boolPtr(true),
		ParserKind:     parserKindMessageContentImage,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.MimeType != "image/png" {
		t.Fatalf("unexpected mime type: %s", result.MimeType)
	}
}

func TestChatCompletionsModeShouldFallbackToURLInText(t *testing.T) {
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes())
	}))
	defer imageServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": "Here is your image: " + imageServer.URL + "/poster.png",
				},
			}},
		})
	}))
	defer apiServer.Close()

	client := NewImageProviderClient(apiServer.Client(), 1024*1024, 4*1024*1024, 30000)
	result, err := client.Generate(context.Background(), GenerateRequest{
		Model:       "GPT-Image-1",
		BaseURL:     apiServer.URL + "/v1",
		Prompt:      "otter",
		AspectRatio: "3:2",
		RequestKind: requestKindChatCompletions,
		AspectField: "aspect",
		ParserKind:  parserKindMessageContentImage,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result.Bytes) == 0 {
		t.Fatal("expected image bytes")
	}
}

func TestProviderShouldRouteConfiguredProxyRequests(t *testing.T) {
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "provider.test" {
			t.Fatalf("expected proxied host provider.test, got %s", r.Host)
		}
		if r.URL.String() != "http://provider.test/v1/images/generations" {
			t.Fatalf("unexpected proxied url: %s", r.URL.String())
		}
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read proxied request body: %v", err)
		}
		if !strings.Contains(string(bodyBytes), "\"prompt\":\"otter\"") {
			t.Fatalf("unexpected request body: %s", string(bodyBytes))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString(pngBytes())}},
		})
	}))
	defer proxyServer.Close()

	client := NewImageProviderClient(nil, 1024*1024, 4*1024*1024, 30000)
	result, err := client.Generate(context.Background(), GenerateRequest{
		Model:       "gemini-2.5-flash-image",
		BaseURL:     "http://provider.test",
		ProxyURL:    proxyServer.URL,
		Prompt:      "otter",
		Size:        "1024x1024",
		RequestKind: requestKindImagesGenerate,
		ParserKind:  parserKindDataB64JSON,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.MimeType != "image/png" {
		t.Fatalf("unexpected mime type: %s", result.MimeType)
	}
}

func TestProviderShouldCacheClientsByProxyURL(t *testing.T) {
	client := NewImageProviderClient(nil, 1024*1024, 4*1024*1024, 30000)
	directA, err := client.clientForRequest("", 30000)
	if err != nil {
		t.Fatalf("clientForRequest directA: %v", err)
	}
	directB, err := client.clientForRequest("", 30000)
	if err != nil {
		t.Fatalf("clientForRequest directB: %v", err)
	}
	proxyA, err := client.clientForRequest("http://127.0.0.1:8001", 30000)
	if err != nil {
		t.Fatalf("clientForRequest proxyA: %v", err)
	}
	proxyB, err := client.clientForRequest("http://127.0.0.1:8001", 30000)
	if err != nil {
		t.Fatalf("clientForRequest proxyB: %v", err)
	}
	if directA != directB {
		t.Fatal("expected direct client to be cached")
	}
	if proxyA != proxyB {
		t.Fatal("expected proxy client to be cached")
	}
	if directA == proxyA {
		t.Fatal("expected direct and proxy clients to differ")
	}
}

func TestProviderShouldCacheClientsByTimeoutAndProxyURL(t *testing.T) {
	client := NewImageProviderClient(nil, 1024*1024, 4*1024*1024, 30000)
	proxyDefault, err := client.clientForRequest("http://127.0.0.1:8001", 30000)
	if err != nil {
		t.Fatalf("clientForRequest proxyDefault: %v", err)
	}
	proxyLong, err := client.clientForRequest("http://127.0.0.1:8001", 120000)
	if err != nil {
		t.Fatalf("clientForRequest proxyLong: %v", err)
	}
	if proxyDefault == proxyLong {
		t.Fatal("expected clients with different timeouts to differ")
	}
}

func TestProviderShouldMapHTTPErrors(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer apiServer.Close()

	client := NewImageProviderClient(apiServer.Client(), 1024*1024, 4*1024*1024, 30000)
	_, err := client.Generate(context.Background(), GenerateRequest{
		Model:       "gemini-2.5-flash-image",
		BaseURL:     apiServer.URL,
		Prompt:      "otter",
		Size:        "1024x1024",
		RequestKind: requestKindImagesGenerate,
		ParserKind:  parserKindDataB64JSON,
	})
	if err == nil || !strings.Contains(err.Error(), "http 429") {
		t.Fatalf("expected http 429 error, got %v", err)
	}
}

func TestProviderShouldEnforceMaxResponseBytes(t *testing.T) {
	largeImage := append([]byte{}, pngBytes()...)
	largeImage = append(largeImage, make([]byte, 900*1024)...)
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString(largeImage)}},
		})
	}))
	defer apiServer.Close()

	client := NewImageProviderClient(apiServer.Client(), 2*1024*1024, 512*1024, 30000)
	_, err := client.Generate(context.Background(), GenerateRequest{
		Model:       "gemini-2.5-flash-image",
		BaseURL:     apiServer.URL,
		Prompt:      "otter",
		Size:        "1024x1024",
		RequestKind: requestKindImagesGenerate,
		ParserKind:  parserKindDataB64JSON,
	})
	if err == nil || !strings.Contains(err.Error(), "maxResponseBytes") {
		t.Fatalf("expected maxResponseBytes error, got %v", err)
	}
}

func boolPtr(value bool) *bool {
	return &value
}
