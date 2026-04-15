package imagine

import (
	"encoding/base64"
	"fmt"
	"strings"
)

type EditInput struct {
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
	OutputName     string
}

func parseEditInput(args map[string]any, catalog ModelCatalog) (EditInput, error) {
	images, err := readStringList(args, "images")
	if err != nil {
		return EditInput{}, err
	}
	useMask, err := readOptionalBool(args, "useMask")
	if err != nil {
		return EditInput{}, err
	}
	imageOnly, err := readOptionalBool(args, "imageOnly")
	if err != nil {
		return EditInput{}, err
	}
	webSearch, err := readOptionalBool(args, "webSearch")
	if err != nil {
		return EditInput{}, err
	}
	input := EditInput{
		Model:       readText(args, "model"),
		Prompt:      readText(args, "prompt"),
		Images:      images,
		Size:        readText(args, "size"),
		AspectRatio: readText(args, "aspectRatio"),
		ImageSize:   readText(args, "imageSize"),
		Quality:     strings.ToLower(readText(args, "quality")),
		UseMask:     useMask,
		ImageOnly:   imageOnly,
		WebSearch:   webSearch,
		OutputName:  readText(args, "outputName"),
	}
	if err := validateModel(input.Model); err != nil {
		return EditInput{}, err
	}
	if len(input.Images) == 0 {
		return EditInput{}, invalidParams("images must not be empty")
	}
	if strings.TrimSpace(input.Prompt) == "" {
		return EditInput{}, invalidParams("prompt is required")
	}
	if err := catalog.ValidateArguments(input.Model, operationEdit, args); err != nil {
		return EditInput{}, err
	}
	resolved, err := catalog.Resolve(input.Model, operationEdit, "")
	if err != nil {
		return EditInput{}, err
	}
	options, err := validateCapabilityInputs(resolved.Config, input.Model, input.Size, input.AspectRatio, input.ImageSize, input.Quality, input.UseMask, input.ImageOnly, input.WebSearch)
	if err != nil {
		return EditInput{}, err
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

func normalizeImageRefs(storage *Storage, refs []string) ([]string, error) {
	normalized := make([]string, 0, len(refs))
	for _, imageRef := range refs {
		switch {
		case strings.HasPrefix(imageRef, "http://"), strings.HasPrefix(imageRef, "https://"), strings.HasPrefix(imageRef, "data:"):
			normalized = append(normalized, imageRef)
		default:
			bytes, mimeType, err := storage.ReadFile(imageRef)
			if err != nil {
				return nil, err
			}
			normalized = append(normalized, fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(bytes)))
		}
	}
	return normalized, nil
}
