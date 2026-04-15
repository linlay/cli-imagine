package imagine

import (
	"strings"
)

type GenerateInput struct {
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
	OutputName     string
}

func parseGenerateInput(args map[string]any, catalog ModelCatalog) (GenerateInput, error) {
	useMask, err := readOptionalBool(args, "useMask")
	if err != nil {
		return GenerateInput{}, err
	}
	imageOnly, err := readOptionalBool(args, "imageOnly")
	if err != nil {
		return GenerateInput{}, err
	}
	webSearch, err := readOptionalBool(args, "webSearch")
	if err != nil {
		return GenerateInput{}, err
	}
	input := GenerateInput{
		Model:          readText(args, "model"),
		Prompt:         readText(args, "prompt"),
		Size:           readText(args, "size"),
		AspectRatio:    readText(args, "aspectRatio"),
		ImageSize:      readText(args, "imageSize"),
		Quality:        strings.ToLower(readText(args, "quality")),
		ResponseFormat: normalizeResponseFormat(readText(args, "responseFormat")),
		UseMask:        useMask,
		ImageOnly:      imageOnly,
		WebSearch:      webSearch,
		OutputName:     readText(args, "outputName"),
	}
	if err := validateModel(input.Model); err != nil {
		return GenerateInput{}, err
	}
	if strings.TrimSpace(input.Prompt) == "" {
		return GenerateInput{}, invalidParams("prompt is required")
	}
	if err := catalog.ValidateArguments(input.Model, operationGenerate, args); err != nil {
		return GenerateInput{}, err
	}
	resolved, err := catalog.Resolve(input.Model, operationGenerate, input.ResponseFormat)
	if err != nil {
		return GenerateInput{}, err
	}
	options, err := validateCapabilityInputs(resolved.Config, input.Model, input.Size, input.AspectRatio, input.ImageSize, input.Quality, input.UseMask, input.ImageOnly, input.WebSearch)
	if err != nil {
		return GenerateInput{}, err
	}
	input.Size = options.Size
	input.AspectRatio = options.AspectRatio
	input.ImageSize = options.ImageSize
	input.Quality = options.Quality
	input.UseMask = options.UseMask
	input.ImageOnly = options.ImageOnly
	input.WebSearch = options.WebSearch
	input.BaseURL = resolved.EndpointBaseURL
	input.ProxyURL = resolved.ProxyURL
	input.APIKey = resolved.APIKey
	input.RequestKind = resolved.Config.Request.Kind
	input.AspectField = resolved.Config.Request.AspectField
	input.ImageSizeField = resolved.Config.Request.ImageSizeField
	input.ResponseFormat = resolved.ResponseFormat
	input.ParserKind = resolved.ParserKind
	return input, nil
}

func assetToMap(record AssetRecord) map[string]any {
	return map[string]any{
		"assetId":      record.AssetID,
		"kind":         record.Kind,
		"sourceMode":   record.SourceMode,
		"model":        record.Model,
		"relativePath": record.RelativePath,
		"mimeType":     record.MimeType,
		"sizeBytes":    record.SizeBytes,
		"sha256":       record.SHA256,
		"prompt":       record.Prompt,
		"sourceImages": record.SourceImages,
		"createdAt":    record.CreatedAt,
	}
}
