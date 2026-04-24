package imagine

import (
	"fmt"
	"sort"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"

	"github.com/linlay/cli-imagine/internal/config"
	"github.com/linlay/cli-imagine/internal/schema"
)

const (
	ToolImageGenerate = "image.generate"
	ToolImageEdit     = "image.edit"
	ToolImageImport   = "image.import"

	operationGenerate = "generate"
	operationEdit     = "edit"

	responseFormatURL     = "url"
	responseFormatB64JSON = "b64_json"

	parserKindDataB64JSON         = "data_b64_json"
	parserKindDataURL             = "data_url"
	parserKindCandidatesInline    = "candidates_inline_data"
	parserKindMessageContentImage = "message_content_image"

	requestKindImagesGenerate  = "images_generate"
	requestKindGenerateContent = "generate_content"
	requestKindImagesEdit      = "images_edit"
	requestKindChatCompletions = "chat_completions"

	sizeModeStandard     = "standard"
	sizeModeRatio        = "ratio"
	sizeModePixelX       = "pixel_x"
	sizeModePixelStar    = "pixel_star"
	sizeModeRatioOrPixel = "ratio_or_pixel"
)

type ResolvedCapability struct {
	ModelName       string
	ToolName        string
	EndpointBaseURL string
	ProxyURL        string
	TimeoutMs       int
	APIKey          string
	Config          config.ModelCapabilityConfig
	ResponseFormat  string
	ParserKind      string
	CompiledSchema  *jsonschema.Schema
}

type ResolvedRequestOptions struct {
	Size        string
	AspectRatio string
	ImageSize   string
	Quality     string
	UseMask     *bool
	ImageOnly   *bool
	WebSearch   *bool
}

type CatalogOperation struct {
	ModelName       string
	Name            string
	ToolName        string
	EndpointBaseURL string
	ProxyURL        string
	TimeoutMs       int
	APIKey          string
	Capability      config.ModelCapabilityConfig
	CompiledSchema  *jsonschema.Schema
}

type CatalogModel struct {
	Provider   string
	Name       string
	Operations map[string]CatalogOperation
}

type DiscoveryOperation struct {
	Name             string
	ToolName         string
	InputSchema      map[string]any
	InputSchemaPath  string
	RequestKind      string
	DefaultArguments map[string]any
	Response         config.ModelResponseConfig
}

type DiscoveryModel struct {
	Provider   string
	Name       string
	Operations []DiscoveryOperation
}

type ModelCatalog struct {
	models map[string]CatalogModel
}

func NewModelCatalog(models []config.ModelConfig) ModelCatalog {
	index := make(map[string]CatalogModel, len(models))
	for _, model := range models {
		normalized := normalizeModelConfig(model)
		catalogModel := CatalogModel{
			Provider:   normalized.Provider,
			Name:       normalized.Name,
			Operations: map[string]CatalogOperation{},
		}
		if normalized.Capabilities.Generate != nil {
			catalogModel.Operations[operationGenerate] = mustBuildCatalogOperation(normalized, operationGenerate, ToolImageGenerate, *normalized.Capabilities.Generate)
		}
		if normalized.Capabilities.Edit != nil {
			catalogModel.Operations[operationEdit] = mustBuildCatalogOperation(normalized, operationEdit, ToolImageEdit, *normalized.Capabilities.Edit)
		}
		index[normalizeModelKey(normalized.Name)] = catalogModel
	}
	return ModelCatalog{models: index}
}

func mustBuildCatalogOperation(model config.ModelConfig, operation, toolName string, capability config.ModelCapabilityConfig) CatalogOperation {
	compiled, err := schema.Compile(model.Name+"."+operation+".inputSchema", capability.InputSchema)
	if err != nil {
		panic(fmt.Sprintf("compile %s capability schema: %v", model.Name, err))
	}
	return CatalogOperation{
		ModelName:       model.Name,
		Name:            operation,
		ToolName:        toolName,
		EndpointBaseURL: model.Endpoint.BaseURL,
		ProxyURL:        model.Endpoint.ProxyURL,
		TimeoutMs:       model.Endpoint.TimeoutMs,
		APIKey:          model.Auth.APIKey,
		Capability:      capability,
		CompiledSchema:  compiled,
	}
}

func normalizeModelConfig(model config.ModelConfig) config.ModelConfig {
	model.Name = strings.TrimSpace(model.Name)
	model.Endpoint.BaseURL = strings.TrimSpace(model.Endpoint.BaseURL)
	model.Endpoint.ProxyURL = strings.TrimSpace(model.Endpoint.ProxyURL)
	model.Auth.APIKey = strings.TrimSpace(model.Auth.APIKey)
	if model.Capabilities.Generate != nil {
		normalized := normalizeCapability(*model.Capabilities.Generate)
		model.Capabilities.Generate = &normalized
	}
	if model.Capabilities.Edit != nil {
		normalized := normalizeCapability(*model.Capabilities.Edit)
		model.Capabilities.Edit = &normalized
	}
	return model
}

func normalizeCapability(capability config.ModelCapabilityConfig) config.ModelCapabilityConfig {
	capability.Request.Kind = strings.ToLower(strings.TrimSpace(capability.Request.Kind))
	capability.Request.SizeMode = strings.ToLower(strings.TrimSpace(capability.Request.SizeMode))
	capability.Request.AspectField = normalizeAspectField(capability.Request.AspectField)
	capability.Request.ImageSizeField = normalizeImageSizeField(capability.Request.ImageSizeField)
	capability.Request.DefaultArguments = cloneMap(capability.Request.DefaultArguments)
	capability.Response.DefaultFormat = normalizeResponseFormat(capability.Response.DefaultFormat)
	capability.Response.AllowedFormats = normalizeStringSlice(capability.Response.AllowedFormats)
	if len(capability.Response.ParserByFormat) > 0 {
		normalized := make(map[string]string, len(capability.Response.ParserByFormat))
		for format, parser := range capability.Response.ParserByFormat {
			normalized[normalizeResponseFormat(format)] = strings.ToLower(strings.TrimSpace(parser))
		}
		capability.Response.ParserByFormat = normalized
	}
	return capability
}

func normalizeAspectField(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "aspect":
		return "aspect"
	case "aspect_ratio":
		return "aspect_ratio"
	case "aspectratio":
		return "aspectRatio"
	default:
		return ""
	}
}

func normalizeImageSizeField(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "imagesize":
		return "imageSize"
	case "image_size":
		return "image_size"
	default:
		return ""
	}
}

func normalizeStringSlice(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		normalized = append(normalized, strings.ToLower(strings.TrimSpace(value)))
	}
	return normalized
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func normalizeModelKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (c ModelCatalog) Resolve(model, operation, requestedFormat string) (ResolvedCapability, error) {
	catalogModel, ok := c.models[normalizeModelKey(model)]
	if !ok {
		return ResolvedCapability{}, invalidParams("unsupported model %q", strings.TrimSpace(model))
	}
	capability, ok := catalogModel.Operations[strings.ToLower(strings.TrimSpace(operation))]
	if !ok {
		return ResolvedCapability{}, invalidParams("model %q does not support %s", catalogModel.Name, operation)
	}

	finalFormat := capability.Capability.Response.DefaultFormat
	requestedFormat = normalizeResponseFormat(requestedFormat)
	if requestedFormat != "" {
		if requestedFormat != finalFormat && !containsString(capability.Capability.Response.AllowedFormats, requestedFormat) {
			return ResolvedCapability{}, invalidParams("responseFormat %q is not supported for model %q", requestedFormat, capability.ModelName)
		}
		finalFormat = requestedFormat
	}

	parserKind := capability.Capability.Response.ParserByFormat[finalFormat]
	if strings.TrimSpace(parserKind) == "" {
		return ResolvedCapability{}, fmt.Errorf("model %q is missing parser for responseFormat %q", capability.ModelName, finalFormat)
	}
	return ResolvedCapability{
		ModelName:       capability.ModelName,
		ToolName:        capability.ToolName,
		EndpointBaseURL: capability.EndpointBaseURL,
		ProxyURL:        capability.ProxyURL,
		TimeoutMs:       capability.TimeoutMs,
		APIKey:          capability.APIKey,
		Config:          capability.Capability,
		ResponseFormat:  finalFormat,
		ParserKind:      parserKind,
		CompiledSchema:  capability.CompiledSchema,
	}, nil
}

func (c ModelCatalog) ValidateArguments(model, operation string, args map[string]any) error {
	resolved, err := c.Resolve(model, operation, normalizeResponseFormat(readText(args, "responseFormat")))
	if err != nil {
		return err
	}
	if err := schema.Validate(resolved.CompiledSchema, args); err != nil {
		return invalidParams("%s", err.Error())
	}
	return nil
}

func (c ModelCatalog) List(modelFilter, operationFilter string) []DiscoveryModel {
	modelFilter = normalizeModelKey(modelFilter)
	operationFilter = strings.ToLower(strings.TrimSpace(operationFilter))

	keys := make([]string, 0, len(c.models))
	for key := range c.models {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	models := make([]DiscoveryModel, 0, len(keys))
	for _, key := range keys {
		model := c.models[key]
		if modelFilter != "" && key != modelFilter {
			continue
		}
		operations := make([]DiscoveryOperation, 0, len(model.Operations))
		for _, name := range []string{operationGenerate, operationEdit} {
			op, ok := model.Operations[name]
			if !ok {
				continue
			}
			if operationFilter != "" && name != operationFilter {
				continue
			}
			operations = append(operations, DiscoveryOperation{
				Name:             name,
				ToolName:         op.ToolName,
				InputSchema:      op.Capability.InputSchema,
				InputSchemaPath:  op.Capability.InputSchemaPath,
				RequestKind:      op.Capability.Request.Kind,
				DefaultArguments: cloneMap(op.Capability.Request.DefaultArguments),
				Response: config.ModelResponseConfig{
					DefaultFormat:  op.Capability.Response.DefaultFormat,
					AllowedFormats: append([]string{}, op.Capability.Response.AllowedFormats...),
					ParserByFormat: cloneStringMap(op.Capability.Response.ParserByFormat),
				},
			})
		}
		if len(operations) == 0 {
			continue
		}
		models = append(models, DiscoveryModel{Provider: model.Provider, Name: model.Name, Operations: operations})
	}
	return models
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func validateCapabilityInputs(capability config.ModelCapabilityConfig, model, size, aspectRatio, imageSize, quality string, useMask, imageOnly, webSearch *bool) (ResolvedRequestOptions, error) {
	if strings.TrimSpace(model) == "" {
		return ResolvedRequestOptions{}, invalidParams("model is required")
	}

	options := ResolvedRequestOptions{
		Size:      strings.TrimSpace(size),
		ImageSize: strings.TrimSpace(imageSize),
		Quality:   strings.TrimSpace(quality),
		UseMask:   resolveOptionalBoolFromDefaults(useMask, capability.Request.DefaultArguments, "useMask"),
		ImageOnly: resolveOptionalBoolFromDefaults(imageOnly, capability.Request.DefaultArguments, "imageOnly"),
		WebSearch: resolveOptionalBoolFromDefaults(webSearch, capability.Request.DefaultArguments, "webSearch"),
	}

	aspectRatio = strings.TrimSpace(aspectRatio)
	if aspectRatio != "" && !isRatioSize(aspectRatio) {
		return ResolvedRequestOptions{}, invalidParams("aspectRatio must be ratio format")
	}

	switch capability.Request.SizeMode {
	case sizeModeStandard, sizeModePixelX, sizeModePixelStar:
		options.AspectRatio = aspectRatio
	case sizeModeRatio:
		ratioValue := aspectRatio
		if options.Size != "" {
			if !isRatioSize(options.Size) {
				return ResolvedRequestOptions{}, invalidParams("size must be ratio format")
			}
			if ratioValue != "" && ratioValue != options.Size {
				return ResolvedRequestOptions{}, invalidParams("size and aspectRatio must match when both are provided")
			}
			ratioValue = options.Size
		}
		options.AspectRatio = ratioValue
		options.Size = ""
	case sizeModeRatioOrPixel:
		if options.Size != "" {
			switch {
			case isRatioSize(options.Size):
				if aspectRatio != "" && aspectRatio != options.Size {
					return ResolvedRequestOptions{}, invalidParams("size and aspectRatio must match when both are provided")
				}
				options.AspectRatio = options.Size
				options.Size = ""
			default:
				options.AspectRatio = aspectRatio
			}
		} else {
			options.AspectRatio = aspectRatio
		}
	default:
		return ResolvedRequestOptions{}, fmt.Errorf("unsupported sizeMode %q for model %q", capability.Request.SizeMode, model)
	}
	return options, nil
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func resolveOptionalBoolFromDefaults(input *bool, defaults map[string]any, key string) *bool {
	if input != nil {
		value := *input
		return &value
	}
	raw, ok := defaults[key]
	if !ok {
		return nil
	}
	value, ok := raw.(bool)
	if !ok {
		return nil
	}
	return &value
}
