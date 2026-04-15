package imagine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type ProviderImage struct {
	Bytes    []byte
	MimeType string
}

type ProviderRequestPreview struct {
	Method     string         `json:"method"`
	Endpoint   string         `json:"endpoint"`
	Body       map[string]any `json:"body"`
	RequestKind string        `json:"request_kind"`
}

type GenerateRequest struct {
	Model          string
	BaseURL        string
	ProxyURL       string
	APIKey         string
	Prompt         string
	Size           string
	AspectRatio    string
	ImageSize      string
	Quality        string
	RequestKind    string
	AspectField    string
	ImageSizeField string
	UseMask        *bool
	ImageOnly      *bool
	WebSearch      *bool
	ResponseFormat string
	ParserKind     string
}

type EditRequest struct {
	Model          string
	BaseURL        string
	ProxyURL       string
	APIKey         string
	Prompt         string
	Images         []string
	Size           string
	AspectRatio    string
	ImageSize      string
	Quality        string
	RequestKind    string
	AspectField    string
	ImageSizeField string
	UseMask        *bool
	ImageOnly      *bool
	WebSearch      *bool
	ResponseFormat string
	ParserKind     string
}

type ImageProviderClient struct {
	maxFileBytes     int64
	maxResponseBytes int64
	timeoutMs        int
	mu               sync.Mutex
	clients          map[string]*http.Client
}

func NewImageProviderClient(client *http.Client, maxFileBytes int64, maxResponseBytes int64, timeoutMs int) *ImageProviderClient {
	clients := map[string]*http.Client{}
	if client == nil {
		client = newHTTPClient(timeoutMs, "")
	}
	clients[""] = client
	return &ImageProviderClient{
		maxFileBytes:     maxFileBytes,
		maxResponseBytes: maxResponseBytes,
		timeoutMs:        timeoutMs,
		clients:          clients,
	}
}

func (c *ImageProviderClient) DirectClient() *http.Client {
	client, err := c.clientForProxy("")
	if err != nil {
		return nil
	}
	return client
}

func (c *ImageProviderClient) Generate(ctx context.Context, req GenerateRequest) (ProviderImage, error) {
	client, err := c.clientForProxy(req.ProxyURL)
	if err != nil {
		return ProviderImage{}, err
	}
	preview, err := PreviewGenerateRequest(req)
	if err != nil {
		return ProviderImage{}, fmt.Errorf("unsupported generate request kind: %s", req.RequestKind)
	}
	return c.requestImage(ctx, client, req.Model, req.RequestKind, req.BaseURL, req.APIKey, preview.Endpoint, preview.Body, req.ParserKind)
}

func (c *ImageProviderClient) Edit(ctx context.Context, req EditRequest) (ProviderImage, error) {
	client, err := c.clientForProxy(req.ProxyURL)
	if err != nil {
		return ProviderImage{}, err
	}
	preview, err := PreviewEditRequest(req)
	if err != nil {
		return ProviderImage{}, fmt.Errorf("unsupported edit request kind: %s", req.RequestKind)
	}
	return c.requestImage(ctx, client, req.Model, req.RequestKind, req.BaseURL, req.APIKey, preview.Endpoint, preview.Body, req.ParserKind)
}

func (c *ImageProviderClient) requestImage(ctx context.Context, client HTTPDoer, model, requestKind, baseURL, apiKey, path string, body map[string]any, parserKind string) (ProviderImage, error) {
	endpoint := joinURL(baseURL, path)
	respBytes, err := doJSONRequest(ctx, client, http.MethodPost, endpoint, body, apiKey, c.maxResponseBytes)
	if err != nil {
		return ProviderImage{}, fmt.Errorf("provider request failed for model %q requestKind %q endpoint %q timeoutMs %d: %w", model, requestKind, endpoint, c.timeoutMs, err)
	}
	return parseProviderImage(ctx, client, respBytes, parserKind, apiKey, c.maxFileBytes)
}

func PreviewGenerateRequest(req GenerateRequest) (ProviderRequestPreview, error) {
	switch req.RequestKind {
	case requestKindGenerateContent:
		imageConfig := map[string]any{}
		if req.AspectRatio != "" {
			key := req.AspectField
			if key == "" {
				key = "aspectRatio"
			}
			imageConfig[key] = req.AspectRatio
		}
		if req.ImageSize != "" {
			key := req.ImageSizeField
			if key == "" {
				key = "imageSize"
			}
			imageConfig[key] = req.ImageSize
		}
		return ProviderRequestPreview{
			Method:      http.MethodPost,
			Endpoint:    "/v1beta/models/" + url.PathEscape(req.Model) + ":generateContent",
			RequestKind: req.RequestKind,
			Body: map[string]any{
				"contents": []map[string]any{{
					"parts": []map[string]any{{"text": req.Prompt}},
				}},
				"generationConfig": map[string]any{
					"responseModalities": []string{"IMAGE"},
					"imageConfig":        imageConfig,
				},
			},
		}, nil
	case "", requestKindImagesGenerate:
		body := map[string]any{
			"model":  req.Model,
			"prompt": req.Prompt,
			"size":   req.Size,
		}
		if req.Quality != "" {
			body["quality"] = req.Quality
		}
		if req.ResponseFormat != "" {
			body["response_format"] = req.ResponseFormat
		}
		return ProviderRequestPreview{
			Method:      http.MethodPost,
			Endpoint:    "/v1/images/generations",
			RequestKind: req.RequestKind,
			Body:        body,
		}, nil
	case requestKindChatCompletions:
		extraBody := map[string]any{}
		if req.Size != "" {
			extraBody["size"] = req.Size
		}
		if req.AspectField != "" && req.AspectRatio != "" {
			extraBody[req.AspectField] = req.AspectRatio
		}
		if req.ImageSizeField != "" && req.ImageSize != "" {
			extraBody[req.ImageSizeField] = req.ImageSize
		}
		if req.Quality != "" {
			extraBody["quality"] = req.Quality
		}
		if req.UseMask != nil {
			extraBody["use_mask"] = *req.UseMask
		}
		if req.ImageOnly != nil {
			extraBody["image_only"] = *req.ImageOnly
		}
		if req.WebSearch != nil {
			extraBody["web_search"] = *req.WebSearch
		}
		return ProviderRequestPreview{
			Method:      http.MethodPost,
			Endpoint:    "chat/completions",
			RequestKind: req.RequestKind,
			Body: map[string]any{
				"model": req.Model,
				"messages": []map[string]any{
					{"role": "user", "content": req.Prompt},
				},
				"stream":     false,
				"extra_body": extraBody,
			},
		}, nil
	default:
		return ProviderRequestPreview{}, fmt.Errorf("unsupported generate request kind: %s", req.RequestKind)
	}
}

func PreviewEditRequest(req EditRequest) (ProviderRequestPreview, error) {
	switch req.RequestKind {
	case "", requestKindImagesEdit:
		body := map[string]any{
			"model":  req.Model,
			"prompt": req.Prompt,
			"image":  req.Images,
			"size":   req.Size,
		}
		if req.Quality != "" {
			body["quality"] = req.Quality
		}
		if req.ResponseFormat != "" {
			body["response_format"] = req.ResponseFormat
		}
		return ProviderRequestPreview{
			Method:      http.MethodPost,
			Endpoint:    "/v1/images/edits",
			RequestKind: req.RequestKind,
			Body:        body,
		}, nil
	default:
		return ProviderRequestPreview{}, fmt.Errorf("unsupported edit request kind: %s", req.RequestKind)
	}
}

func (c *ImageProviderClient) clientForProxy(proxyURL string) (*http.Client, error) {
	key := strings.TrimSpace(proxyURL)
	c.mu.Lock()
	defer c.mu.Unlock()
	if client, ok := c.clients[key]; ok {
		return client, nil
	}
	client, err := buildProxyAwareHTTPClient(c.timeoutMs, key)
	if err != nil {
		return nil, err
	}
	c.clients[key] = client
	return client, nil
}

func buildProxyAwareHTTPClient(timeoutMs int, proxyURL string) (*http.Client, error) {
	timeout := time.Duration(timeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: newHTTPTransport(proxyURL),
	}, nil
}

func newHTTPClient(timeoutMs int, proxyURL string) *http.Client {
	client, err := buildProxyAwareHTTPClient(timeoutMs, proxyURL)
	if err != nil {
		panic(fmt.Sprintf("build http client: %v", err))
	}
	return client
}

func newHTTPTransport(proxyURL string) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return transport
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		panic(fmt.Sprintf("parse proxy URL %q: %v", proxyURL, err))
	}
	transport.Proxy = http.ProxyURL(parsed)
	return transport
}

func doJSONRequest(ctx context.Context, client HTTPDoer, method, endpoint string, body map[string]any, apiKey string, maxResponseBytes int64) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal provider request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build provider request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider request failed: %w", err)
	}
	defer resp.Body.Close()
	if maxResponseBytes <= 0 {
		maxResponseBytes = 32 * 1024 * 1024
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read provider response: %w", err)
	}
	if int64(len(bodyBytes)) > maxResponseBytes {
		return nil, fmt.Errorf("provider response exceeds maxResponseBytes limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("provider request failed: http %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	return bodyBytes, nil
}

func parseProviderImage(ctx context.Context, client HTTPDoer, respBytes []byte, parserKind, apiKey string, maxFileBytes int64) (ProviderImage, error) {
	switch strings.ToLower(strings.TrimSpace(parserKind)) {
	case parserKindDataB64JSON:
		item, err := parseBabelarkDataItem(respBytes)
		if err != nil {
			return ProviderImage{}, err
		}
		if strings.TrimSpace(item.B64JSON) == "" {
			return ProviderImage{}, fmt.Errorf("provider returned no usable image payload")
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(item.B64JSON))
		if err != nil {
			return ProviderImage{}, fmt.Errorf("decode provider b64_json: %w", err)
		}
		if int64(len(decoded)) > maxFileBytes {
			return ProviderImage{}, fmt.Errorf("file exceeds maxFileBytes limit")
		}
		return ProviderImage{Bytes: decoded, MimeType: detectMimeType(decoded, "")}, nil
	case parserKindDataURL:
		item, err := parseBabelarkDataItem(respBytes)
		if err != nil {
			return ProviderImage{}, err
		}
		if strings.TrimSpace(item.URL) == "" {
			return ProviderImage{}, fmt.Errorf("provider returned no usable image payload")
		}
		return fetchRemoteImage(ctx, client, item.URL, apiKey, maxFileBytes)
	case parserKindCandidatesInline:
		mimeType, payload, err := parseBabelarkCandidatesInlineData(respBytes)
		if err != nil {
			return ProviderImage{}, err
		}
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return ProviderImage{}, fmt.Errorf("decode provider inlineData: %w", err)
		}
		if int64(len(decoded)) > maxFileBytes {
			return ProviderImage{}, fmt.Errorf("file exceeds maxFileBytes limit")
		}
		return ProviderImage{Bytes: decoded, MimeType: detectMimeType(decoded, mimeType)}, nil
	case parserKindMessageContentImage:
		return parsePoeImage(ctx, client, respBytes, apiKey, maxFileBytes)
	default:
		return ProviderImage{}, fmt.Errorf("unsupported parser kind: %s", parserKind)
	}
}

func parseBabelarkCandidatesInlineData(respBytes []byte) (string, string, error) {
	var payload map[string]any
	if err := json.Unmarshal(respBytes, &payload); err != nil {
		return "", "", fmt.Errorf("decode provider response: %w", err)
	}
	candidates, ok := payload["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		return "", "", fmt.Errorf("provider returned no candidates")
	}
	for _, candidate := range candidates {
		candidateMap, ok := candidate.(map[string]any)
		if !ok {
			continue
		}
		content, ok := candidateMap["content"].(map[string]any)
		if !ok {
			continue
		}
		parts, ok := content["parts"].([]any)
		if !ok {
			continue
		}
		for _, part := range parts {
			partMap, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if mimeType, data := extractInlineData(partMap, "inlineData", "mimeType"); data != "" {
				return mimeType, data, nil
			}
			if mimeType, data := extractInlineData(partMap, "inline_data", "mime_type"); data != "" {
				return mimeType, data, nil
			}
		}
	}
	return "", "", fmt.Errorf("provider returned no inline image data")
}

func extractInlineData(part map[string]any, key, mimeKey string) (string, string) {
	inlineNode, ok := part[key].(map[string]any)
	if !ok {
		return "", ""
	}
	data := strings.TrimSpace(fmt.Sprint(inlineNode["data"]))
	if data == "" {
		return "", ""
	}
	mimeType := strings.TrimSpace(fmt.Sprint(inlineNode[mimeKey]))
	return mimeType, data
}

func parseBabelarkDataItem(respBytes []byte) (struct {
	URL     string `json:"url"`
	B64JSON string `json:"b64_json"`
}, error) {
	var response struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &response); err != nil {
		return struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		}{}, fmt.Errorf("decode provider response: %w", err)
	}
	if len(response.Data) == 0 {
		return struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		}{}, fmt.Errorf("provider returned no images")
	}
	return response.Data[0], nil
}

func parsePoeImage(ctx context.Context, client HTTPDoer, respBytes []byte, apiKey string, maxFileBytes int64) (ProviderImage, error) {
	var payload map[string]any
	if err := json.Unmarshal(respBytes, &payload); err != nil {
		return ProviderImage{}, fmt.Errorf("decode provider response: %w", err)
	}
	choices, ok := payload["choices"].([]any)
	if !ok || len(choices) == 0 {
		return ProviderImage{}, fmt.Errorf("provider returned no choices")
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return ProviderImage{}, fmt.Errorf("provider returned invalid choice")
	}
	message, ok := choice["message"].(map[string]any)
	if !ok {
		return ProviderImage{}, fmt.Errorf("provider returned no message")
	}

	if structured, ok := extractStructuredImagePayload(message["content"]); ok {
		return decodeOrFetchImage(ctx, client, structured, apiKey, maxFileBytes)
	}

	text := strings.TrimSpace(fmt.Sprint(message["content"]))
	if text == "" {
		return ProviderImage{}, fmt.Errorf("provider response did not include an image")
	}
	urlValue := firstURL(text)
	if urlValue == "" {
		return ProviderImage{}, fmt.Errorf("provider response did not include an image")
	}
	return fetchRemoteImage(ctx, client, urlValue, apiKey, maxFileBytes)
}

func fetchRemoteImage(ctx context.Context, client HTTPDoer, rawURL, apiKey string, maxFileBytes int64) (ProviderImage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return ProviderImage{}, fmt.Errorf("build image request: %w", err)
	}
	if strings.TrimSpace(apiKey) != "" && sameHost(rawURL, apiKey) {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}
	resp, err := client.Do(req)
	if err != nil {
		return ProviderImage{}, fmt.Errorf("download image failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return ProviderImage{}, fmt.Errorf("download image failed: http %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	limited := io.LimitReader(resp.Body, maxFileBytes+1)
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		return ProviderImage{}, fmt.Errorf("read image response: %w", err)
	}
	if int64(len(bodyBytes)) > maxFileBytes {
		return ProviderImage{}, fmt.Errorf("image exceeds maxFileBytes limit")
	}
	mimeType := detectMimeType(bodyBytes, resp.Header.Get("Content-Type"))
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return ProviderImage{}, fmt.Errorf("downloaded payload is not an image")
	}
	return ProviderImage{Bytes: bodyBytes, MimeType: mimeType}, nil
}

func decodeOrFetchImage(ctx context.Context, client HTTPDoer, payload string, apiKey string, maxFileBytes int64) (ProviderImage, error) {
	trimmed := strings.TrimSpace(payload)
	switch {
	case strings.HasPrefix(trimmed, "data:"):
		mimeType, data, err := parseDataURL(trimmed)
		if err != nil {
			return ProviderImage{}, err
		}
		if int64(len(data)) > maxFileBytes {
			return ProviderImage{}, fmt.Errorf("file exceeds maxFileBytes limit")
		}
		return ProviderImage{Bytes: data, MimeType: mimeType}, nil
	case strings.HasPrefix(trimmed, "http://"), strings.HasPrefix(trimmed, "https://"):
		return fetchRemoteImage(ctx, client, trimmed, apiKey, maxFileBytes)
	default:
		return ProviderImage{}, fmt.Errorf("unsupported image payload")
	}
}

func extractStructuredImagePayload(content any) (string, bool) {
	switch typed := content.(type) {
	case []any:
		for _, item := range typed {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(fmt.Sprint(block["type"]))) {
			case "image_url":
				if imageURL, ok := block["image_url"].(map[string]any); ok {
					if urlValue := strings.TrimSpace(fmt.Sprint(imageURL["url"])); urlValue != "" {
						return urlValue, true
					}
				}
			case "file":
				if fileNode, ok := block["file"].(map[string]any); ok {
					if dataValue := strings.TrimSpace(fmt.Sprint(fileNode["file_data"])); dataValue != "" {
						return dataValue, true
					}
					if urlValue := strings.TrimSpace(fmt.Sprint(fileNode["url"])); urlValue != "" {
						return urlValue, true
					}
				}
			}
		}
	case string:
		if urlValue := firstURL(typed); urlValue != "" {
			return urlValue, true
		}
	}
	return "", false
}

func firstURL(value string) string {
	pattern := regexp.MustCompile(`https?://[^\s\)>"']+`)
	match := pattern.FindString(strings.TrimSpace(value))
	return strings.TrimSuffix(match, ".")
}

func joinURL(baseURL, path string) string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
	}
	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	ref, err := url.Parse(strings.TrimLeft(path, "/"))
	if err != nil {
		return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
	}
	return base.ResolveReference(ref).String()
}

func sameHost(rawURL string, _ string) bool {
	return false
}
