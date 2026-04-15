package imagine

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
)

func readText(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	value, exists := args[key]
	if !exists || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func readStringList(args map[string]any, key string) ([]string, error) {
	if args == nil {
		return []string{}, nil
	}
	raw, exists := args[key]
	if !exists || raw == nil {
		return []string{}, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, invalidParams("%s must be an array", key)
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(fmt.Sprint(item))
		if value == "" {
			return nil, invalidParams("%s items must be non-empty strings", key)
		}
		result = append(result, value)
	}
	return result, nil
}

func readOptionalBool(args map[string]any, key string) (*bool, error) {
	if args == nil {
		return nil, nil
	}
	raw, exists := args[key]
	if !exists || raw == nil {
		return nil, nil
	}
	value, ok := raw.(bool)
	if !ok {
		return nil, invalidParams("%s must be a boolean", key)
	}
	return &value, nil
}

func isRatioSize(value string) bool {
	matched, _ := regexp.MatchString(`^\d+:\d+$`, strings.TrimSpace(value))
	return matched
}

func isStandardSize(value string) bool {
	switch strings.TrimSpace(value) {
	case "256x256", "512x512", "1024x1024", "1792x1024", "1024x1792":
		return true
	default:
		return false
	}
}

func isPixelStarSize(value string) bool {
	matched, _ := regexp.MatchString(`^\d+\*\d+$`, strings.TrimSpace(value))
	return matched
}

func isPixelXSize(value string) bool {
	matched, _ := regexp.MatchString(`^\d+x\d+$`, strings.TrimSpace(value))
	return matched
}

func validateModel(value string) error {
	if strings.TrimSpace(value) == "" {
		return invalidParams("model is required")
	}
	return nil
}

func validateQuality(value string) error {
	if value == "" {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "medium", "high":
		return nil
	default:
		return invalidParams("quality must be one of low, medium, high")
	}
}

func normalizeResponseFormat(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func detectMimeType(data []byte, fallback string) string {
	if len(data) == 0 {
		return fallback
	}
	detected := http.DetectContentType(data)
	if detected == "application/octet-stream" && fallback != "" {
		return fallback
	}
	return detected
}

func parseDataURL(raw string) (string, []byte, error) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "data:") {
		return "", nil, fmt.Errorf("invalid data URL")
	}
	comma := strings.Index(trimmed, ",")
	if comma < 0 {
		return "", nil, fmt.Errorf("invalid data URL")
	}
	header := trimmed[5:comma]
	payload := trimmed[comma+1:]
	parts := strings.Split(header, ";")
	mimeType := "application/octet-stream"
	if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
		mimeType = strings.TrimSpace(parts[0])
	}
	isBase64 := false
	for _, part := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(part), "base64") {
			isBase64 = true
			break
		}
	}
	if !isBase64 {
		return "", nil, fmt.Errorf("data URL must be base64 encoded")
	}
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", nil, fmt.Errorf("decode data URL: %w", err)
	}
	return mimeType, decoded, nil
}

func extensionForMime(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/bmp":
		return ".bmp"
	case "image/tiff":
		return ".tiff"
	case "image/svg+xml":
		return ".svg"
	default:
		return ".img"
	}
}

func basenameOnly(value string) string {
	base := filepath.Base(strings.TrimSpace(value))
	base = strings.TrimSpace(base)
	if base == "." || base == string(filepath.Separator) {
		return ""
	}
	return base
}
