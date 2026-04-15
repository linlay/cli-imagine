package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/linlay/cli-imagine/internal/config"
	"github.com/linlay/cli-imagine/internal/imagine"
)

const (
	defaultPrompt = "A cinematic watercolor illustration of a red panda reading beside a glowing lantern in a quiet forest, highly detailed, warm colors."
	editPrompt    = "Transform the source image into a polished poster illustration with sunrise lighting, clean composition, and vivid warm tones."
)

type Options struct {
	Config    config.Config
	ConfigDir string
	OutputDir string
}

type Summary struct {
	OutputDir   string     `json:"output_dir"`
	RunDir      string     `json:"run_dir"`
	SummaryPath string     `json:"summary_path"`
	Results     []CaseInfo `json:"results"`
}

type CaseInfo struct {
	Model         string         `json:"model"`
	Provider      string         `json:"provider"`
	Operation     string         `json:"operation"`
	Status        string         `json:"status"`
	Reason        string         `json:"reason,omitempty"`
	Arguments     map[string]any `json:"arguments,omitempty"`
	ResultPath    string         `json:"result_path,omitempty"`
	InspectBody   map[string]any `json:"inspect_body,omitempty"`
	InspectURL    string         `json:"inspect_url,omitempty"`
	InspectMethod string         `json:"inspect_method,omitempty"`
}

func Run(ctx context.Context, opts Options) (Summary, error) {
	root := strings.TrimSpace(opts.OutputDir)
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Summary{}, fmt.Errorf("resolve output dir: %w", err)
	}
	runDir := filepath.Join(absRoot, "verification", time.Now().UTC().Format("20060102T150405Z"))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return Summary{}, fmt.Errorf("create verify dir: %w", err)
	}

	rt := imagine.NewRuntime(opts.Config, imagine.RunContext{OutputDir: runDir})
	seedPath, err := writeSeedAsset(rt.Storage)
	if err != nil {
		return Summary{}, err
	}

	models := rt.ListModels("", "", "")
	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider == models[j].Provider {
			return models[i].Name < models[j].Name
		}
		return models[i].Provider < models[j].Provider
	})

	results := make([]CaseInfo, 0)
	for _, model := range models {
		sort.Slice(model.Operations, func(i, j int) bool {
			return model.Operations[i].Name < model.Operations[j].Name
		})
		for _, operation := range model.Operations {
			arguments, skipReason := inferArguments(model.Name, operation, seedPath)
			if skipReason != "" {
				results = append(results, CaseInfo{
					Model:     model.Name,
					Provider:  model.Provider,
					Operation: operation.Name,
					Status:    "skipped",
					Reason:    skipReason,
				})
				continue
			}

			inspection, err := rt.Inspect(operation.ToolName, arguments)
			if err != nil {
				results = append(results, CaseInfo{
					Model:     model.Name,
					Provider:  model.Provider,
					Operation: operation.Name,
					Status:    "failed",
					Reason:    err.Error(),
					Arguments: arguments,
				})
				continue
			}

			result, err := rt.Execute(ctx, operation.ToolName, arguments)
			if err != nil {
				results = append(results, CaseInfo{
					Model:         model.Name,
					Provider:      model.Provider,
					Operation:     operation.Name,
					Status:        "failed",
					Reason:        err.Error(),
					Arguments:     arguments,
					InspectMethod: readString(inspection.Request, "method"),
					InspectURL:    readString(inspection.Request, "endpoint"),
					InspectBody:   readMap(inspection.Request, "body"),
				})
				continue
			}

			results = append(results, CaseInfo{
				Model:         model.Name,
				Provider:      model.Provider,
				Operation:     operation.Name,
				Status:        "ok",
				Arguments:     arguments,
				ResultPath:    resolveResultPath(result),
				InspectMethod: readString(inspection.Request, "method"),
				InspectURL:    readString(inspection.Request, "endpoint"),
				InspectBody:   readMap(inspection.Request, "body"),
			})
		}
	}

	summaryPath := filepath.Join(runDir, "summary.md")
	if err := writeSummary(summaryPath, results); err != nil {
		return Summary{}, err
	}
	return Summary{
		OutputDir:   absRoot,
		RunDir:      runDir,
		SummaryPath: summaryPath,
		Results:     results,
	}, nil
}

func inferArguments(modelName string, operation imagine.DiscoveryOperation, seedPath string) (map[string]any, string) {
	args := map[string]any{
		"model":  modelName,
		"prompt": defaultPrompt,
	}
	if operation.Name == "edit" {
		args["prompt"] = editPrompt
		args["images"] = []any{seedPath}
	}

	properties := readMap(operation.InputSchema, "properties")
	required := readStringSlice(operation.InputSchema, "required")
	requiredSet := make(map[string]struct{}, len(required))
	for _, item := range required {
		requiredSet[item] = struct{}{}
	}

	for key, propertyAny := range properties {
		property, ok := propertyAny.(map[string]any)
		if !ok {
			continue
		}
		if _, exists := args[key]; exists {
			continue
		}

		if value, ok := inferPropertyValue(key, property, operation); ok {
			args[key] = value
			continue
		}

		if _, needed := requiredSet[key]; needed {
			return nil, fmt.Sprintf("cannot infer required field %q", key)
		}
	}
	return args, ""
}

func inferPropertyValue(key string, property map[string]any, operation imagine.DiscoveryOperation) (any, bool) {
	switch key {
	case "size", "aspectRatio", "imageSize", "quality", "responseFormat":
		if values := readEnum(property); len(values) > 0 {
			return values[0], true
		}
	case "useMask", "imageOnly", "webSearch":
		if value, ok := operation.DefaultArguments[key]; ok {
			return value, true
		}
		if typeName := strings.TrimSpace(readString(property, "type")); typeName == "boolean" {
			return false, true
		}
	case "outputName":
		return safeOutputName(operation.Name), true
	}

	switch strings.TrimSpace(readString(property, "type")) {
	case "string":
		if values := readEnum(property); len(values) > 0 {
			return values[0], true
		}
	case "boolean":
		return false, true
	}
	return nil, false
}

func safeOutputName(operation string) string {
	if operation == "edit" {
		return "verify-edit.png"
	}
	return "verify-generate.png"
}

func writeSeedAsset(storage *imagine.Storage) (string, error) {
	record, err := storage.SaveAsset(imagine.SaveRequest{
		Kind:          "imported",
		SourceMode:    "verify_seed",
		Prompt:        "",
		OutputName:    "verify-seed.png",
		DefaultPrefix: "seed_",
		Bytes:         pngBytes(),
		MimeType:      "image/png",
	})
	if err != nil {
		return "", fmt.Errorf("write verify seed image: %w", err)
	}
	return record.RelativePath, nil
}

func resolveResultPath(result map[string]any) string {
	if asset, ok := result["asset"].(map[string]any); ok {
		return readString(asset, "relativePath")
	}
	return ""
}

func writeSummary(path string, results []CaseInfo) error {
	var builder strings.Builder
	builder.WriteString("# cli-imagine verify summary\n\n")
	for _, item := range results {
		builder.WriteString(fmt.Sprintf("## %s / %s / %s\n\n", item.Provider, item.Model, item.Operation))
		builder.WriteString(fmt.Sprintf("- status: %s\n", item.Status))
		if item.Reason != "" {
			builder.WriteString(fmt.Sprintf("- reason: %s\n", item.Reason))
		}
		if item.ResultPath != "" {
			builder.WriteString(fmt.Sprintf("- result: %s\n", item.ResultPath))
		}
		if len(item.Arguments) > 0 {
			payload, _ := json.MarshalIndent(item.Arguments, "", "  ")
			builder.WriteString("- arguments:\n\n```json\n")
			builder.Write(payload)
			builder.WriteString("\n```\n")
		}
		if item.InspectURL != "" || len(item.InspectBody) > 0 {
			builder.WriteString(fmt.Sprintf("- request: `%s %s`\n\n", item.InspectMethod, item.InspectURL))
			payload, _ := json.MarshalIndent(item.InspectBody, "", "  ")
			builder.WriteString("```json\n")
			builder.Write(payload)
			builder.WriteString("\n```\n")
		}
		builder.WriteString("\n")
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func readMap(source map[string]any, key string) map[string]any {
	if source == nil {
		return map[string]any{}
	}
	value, ok := source[key].(map[string]any)
	if !ok || value == nil {
		return map[string]any{}
	}
	return value
}

func readString(source map[string]any, key string) string {
	if source == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(source[key]))
}

func readStringSlice(source map[string]any, key string) []string {
	if source == nil {
		return nil
	}
	raw, ok := source[key].([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		result = append(result, strings.TrimSpace(fmt.Sprint(item)))
	}
	return result
}

func readEnum(property map[string]any) []string {
	raw, ok := property["enum"].([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		result = append(result, strings.TrimSpace(fmt.Sprint(item)))
	}
	return result
}

func pngBytes() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41,
		0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x03, 0x01, 0x01, 0x00, 0xc9, 0xfe, 0x92,
		0xef, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82,
	}
}
