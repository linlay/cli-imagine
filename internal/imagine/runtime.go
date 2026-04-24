package imagine

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/linlay/cli-imagine/internal/config"
)

const (
	DefaultProviderTimeoutMs       = 30000
	DefaultMaxResponseBytes        = 32 * 1024 * 1024
	DefaultMaxFileBytes      int64 = 20 * 1024 * 1024
)

type RunContext struct {
	OutputDir string
}

type Runtime struct {
	Context       RunContext
	Storage       *Storage
	ImageProvider *ImageProviderClient
	ModelCatalog  ModelCatalog
}

type Inspection struct {
	Tool      string         `json:"tool"`
	OutputDir string         `json:"output_dir"`
	Arguments map[string]any `json:"arguments"`
	Model     map[string]any `json:"model,omitempty"`
	Request   map[string]any `json:"request,omitempty"`
}

func NewRuntime(cfg config.Config, ctx RunContext) *Runtime {
	storage := NewStorage(ctx.OutputDir, DefaultMaxFileBytes)
	client := NewImageProviderClient(nil, DefaultMaxFileBytes, DefaultMaxResponseBytes, DefaultProviderTimeoutMs)
	return &Runtime{
		Context:       ctx,
		Storage:       storage,
		ImageProvider: client,
		ModelCatalog:  NewModelCatalog(cfg.Models),
	}
}

func (rt *Runtime) Execute(ctx context.Context, tool string, args map[string]any) (map[string]any, error) {
	switch strings.TrimSpace(tool) {
	case ToolImageGenerate:
		return rt.executeGenerate(ctx, args)
	case ToolImageEdit:
		return rt.executeEdit(ctx, args)
	case ToolImageImport:
		return rt.executeImport(ctx, args)
	default:
		return nil, fmt.Errorf("unsupported tool %q", tool)
	}
}

func (rt *Runtime) Inspect(tool string, args map[string]any) (Inspection, error) {
	inspection := Inspection{
		Tool:      tool,
		OutputDir: rt.Context.OutputDir,
		Arguments: cloneMap(args),
	}
	switch strings.TrimSpace(tool) {
	case ToolImageGenerate:
		input, err := parseGenerateInput(args, rt.ModelCatalog)
		if err != nil {
			return Inspection{}, err
		}
		preview, err := PreviewGenerateRequest(input.toRequest())
		if err != nil {
			return Inspection{}, err
		}
		inspection.Model = map[string]any{
			"model":           input.Model,
			"operation":       operationGenerate,
			"responseFormat":  input.ResponseFormat,
			"parserKind":      input.ParserKind,
			"baseUrl":         input.BaseURL,
			"proxyUrl":        input.ProxyURL,
			"timeoutMs":       input.TimeoutMs,
			"requestKind":     input.RequestKind,
			"aspectField":     input.AspectField,
			"imageSizeField":  input.ImageSizeField,
			"normalizedInput": input.toMap(),
		}
		inspection.Request = map[string]any{
			"method":   preview.Method,
			"endpoint": joinURL(input.BaseURL, preview.Endpoint),
			"body":     preview.Body,
		}
	case ToolImageEdit:
		input, err := parseEditInput(args, rt.ModelCatalog)
		if err != nil {
			return Inspection{}, err
		}
		normalizedImages, err := normalizeImageRefs(rt.Storage, input.Images)
		if err != nil {
			return Inspection{}, err
		}
		request := input.toRequest(normalizedImages)
		preview, err := PreviewEditRequest(request)
		if err != nil {
			return Inspection{}, err
		}
		inspection.Model = map[string]any{
			"model":           input.Model,
			"operation":       operationEdit,
			"responseFormat":  input.ResponseFormat,
			"parserKind":      input.ParserKind,
			"baseUrl":         input.BaseURL,
			"proxyUrl":        input.ProxyURL,
			"timeoutMs":       input.TimeoutMs,
			"requestKind":     input.RequestKind,
			"aspectField":     input.AspectField,
			"imageSizeField":  input.ImageSizeField,
			"normalizedInput": input.toMap(),
		}
		inspection.Request = map[string]any{
			"method":   preview.Method,
			"endpoint": joinURL(input.BaseURL, preview.Endpoint),
			"body":     preview.Body,
		}
	case ToolImageImport:
		items, err := parseImportItems(args)
		if err != nil {
			return Inspection{}, err
		}
		normalized := make([]map[string]any, 0, len(items))
		for _, item := range items {
			normalized = append(normalized, map[string]any{
				"type":     item.Type,
				"value":    item.Value,
				"name":     item.Name,
				"mimeType": item.MimeType,
			})
		}
		inspection.Request = map[string]any{"items": normalized}
	default:
		return Inspection{}, fmt.Errorf("unsupported tool %q", tool)
	}
	return inspection, nil
}

func (rt *Runtime) ListModels(providerFilter, modelFilter, operationFilter string) []DiscoveryModel {
	models := rt.ModelCatalog.List(modelFilter, operationFilter)
	if strings.TrimSpace(providerFilter) == "" {
		return models
	}
	filtered := make([]DiscoveryModel, 0, len(models))
	for _, model := range models {
		if strings.EqualFold(strings.TrimSpace(providerFilter), strings.TrimSpace(model.Provider)) {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func (rt *Runtime) executeGenerate(ctx context.Context, args map[string]any) (map[string]any, error) {
	input, err := parseGenerateInput(args, rt.ModelCatalog)
	if err != nil {
		return nil, err
	}
	result, err := rt.ImageProvider.Generate(ctx, input.toRequest())
	if err != nil {
		return nil, err
	}
	record, err := rt.Storage.SaveAsset(SaveRequest{
		Kind:          "generated",
		SourceMode:    input.RequestKind,
		Model:         input.Model,
		Prompt:        input.Prompt,
		SourceImages:  []string{},
		OutputName:    input.OutputName,
		DefaultPrefix: "img_",
		Bytes:         result.Bytes,
		MimeType:      result.MimeType,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"asset": assetToMap(record)}, nil
}

func (rt *Runtime) executeEdit(ctx context.Context, args map[string]any) (map[string]any, error) {
	input, err := parseEditInput(args, rt.ModelCatalog)
	if err != nil {
		return nil, err
	}
	normalizedImages, err := normalizeImageRefs(rt.Storage, input.Images)
	if err != nil {
		return nil, err
	}
	result, err := rt.ImageProvider.Edit(ctx, input.toRequest(normalizedImages))
	if err != nil {
		return nil, err
	}
	record, err := rt.Storage.SaveAsset(SaveRequest{
		Kind:          "edited",
		SourceMode:    input.RequestKind,
		Model:         input.Model,
		Prompt:        input.Prompt,
		SourceImages:  append([]string{}, input.Images...),
		OutputName:    input.OutputName,
		DefaultPrefix: "edit_",
		Bytes:         result.Bytes,
		MimeType:      result.MimeType,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"asset": assetToMap(record)}, nil
}

func (rt *Runtime) executeImport(ctx context.Context, args map[string]any) (map[string]any, error) {
	items, err := parseImportItems(args)
	if err != nil {
		return nil, err
	}
	saved := make([]AssetRecord, 0, len(items))
	for _, item := range items {
		image, outputName, err := resolveImportItem(ctx, rt.Storage, rt.httpClient(), item)
		if err != nil {
			rollbackImport(rt.Storage, saved)
			return nil, err
		}
		record, err := rt.Storage.SaveAsset(SaveRequest{
			Kind:          "imported",
			SourceMode:    "import",
			SourceImages:  []string{item.Value},
			OutputName:    outputName,
			DefaultPrefix: "import_",
			Bytes:         image.Bytes,
			MimeType:      image.MimeType,
		})
		if err != nil {
			rollbackImport(rt.Storage, saved)
			return nil, err
		}
		saved = append(saved, record)
	}
	assets := make([]map[string]any, 0, len(saved))
	for _, record := range saved {
		assets = append(assets, assetToMap(record))
	}
	return map[string]any{"assets": assets}, nil
}

func (rt *Runtime) httpClient() HTTPDoer {
	client := rt.ImageProvider.DirectClient()
	if client != nil {
		return client
	}
	return &http.Client{}
}

func (in GenerateInput) toRequest() GenerateRequest {
	return GenerateRequest{
		Model:          in.Model,
		BaseURL:        in.BaseURL,
		ProxyURL:       in.ProxyURL,
		TimeoutMs:      in.TimeoutMs,
		APIKey:         in.APIKey,
		Prompt:         in.Prompt,
		Size:           in.Size,
		AspectRatio:    in.AspectRatio,
		ImageSize:      in.ImageSize,
		Quality:        in.Quality,
		RequestKind:    in.RequestKind,
		AspectField:    in.AspectField,
		ImageSizeField: in.ImageSizeField,
		UseMask:        in.UseMask,
		ImageOnly:      in.ImageOnly,
		WebSearch:      in.WebSearch,
		ResponseFormat: in.ResponseFormat,
		ParserKind:     in.ParserKind,
	}
}

func (in GenerateInput) toMap() map[string]any {
	return map[string]any{
		"model":          in.Model,
		"prompt":         in.Prompt,
		"size":           in.Size,
		"aspectRatio":    in.AspectRatio,
		"imageSize":      in.ImageSize,
		"quality":        in.Quality,
		"responseFormat": in.ResponseFormat,
		"outputName":     in.OutputName,
		"useMask":        boolPointerValue(in.UseMask),
		"imageOnly":      boolPointerValue(in.ImageOnly),
		"webSearch":      boolPointerValue(in.WebSearch),
	}
}

func (in EditInput) toRequest(images []string) EditRequest {
	return EditRequest{
		Model:          in.Model,
		BaseURL:        in.BaseURL,
		ProxyURL:       in.ProxyURL,
		TimeoutMs:      in.TimeoutMs,
		APIKey:         in.APIKey,
		Prompt:         in.Prompt,
		Images:         images,
		Size:           in.Size,
		AspectRatio:    in.AspectRatio,
		ImageSize:      in.ImageSize,
		Quality:        in.Quality,
		RequestKind:    in.RequestKind,
		AspectField:    in.AspectField,
		ImageSizeField: in.ImageSizeField,
		UseMask:        in.UseMask,
		ImageOnly:      in.ImageOnly,
		WebSearch:      in.WebSearch,
		ResponseFormat: in.ResponseFormat,
		ParserKind:     in.ParserKind,
	}
}

func (in EditInput) toMap() map[string]any {
	return map[string]any{
		"model":       in.Model,
		"prompt":      in.Prompt,
		"images":      append([]string{}, in.Images...),
		"size":        in.Size,
		"aspectRatio": in.AspectRatio,
		"imageSize":   in.ImageSize,
		"quality":     in.Quality,
		"outputName":  in.OutputName,
		"useMask":     boolPointerValue(in.UseMask),
		"imageOnly":   boolPointerValue(in.ImageOnly),
		"webSearch":   boolPointerValue(in.WebSearch),
	}
}

func boolPointerValue(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}
