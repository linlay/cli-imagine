package imagine

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
)

func rollbackImport(storage *Storage, saved []AssetRecord) {
	assetIDs := make([]string, 0, len(saved))
	for _, record := range saved {
		_ = storage.RemoveRelative(record.RelativePath)
		assetIDs = append(assetIDs, record.AssetID)
	}
	_ = storage.RemoveManifestEntries(assetIDs)
}

type ImportItem struct {
	Type     string
	Value    string
	Name     string
	MimeType string
}

func parseImportItems(args map[string]any) ([]ImportItem, error) {
	raw, ok := args["items"].([]any)
	if !ok || len(raw) == 0 {
		return nil, invalidParams("items must be a non-empty array")
	}
	items := make([]ImportItem, 0, len(raw))
	for _, item := range raw {
		node, ok := item.(map[string]any)
		if !ok {
			return nil, invalidParams("items must contain objects")
		}
		parsed := ImportItem{
			Type:     strings.ToLower(readText(node, "type")),
			Value:    readText(node, "value"),
			Name:     readText(node, "name"),
			MimeType: readText(node, "mimeType"),
		}
		switch parsed.Type {
		case "url", "data_url", "base64", "data_path":
		default:
			return nil, invalidParams("item.type must be one of url, data_url, base64, data_path")
		}
		if parsed.Value == "" {
			return nil, invalidParams("item.value is required")
		}
		items = append(items, parsed)
	}
	return items, nil
}

func resolveImportItem(ctx context.Context, storage *Storage, httpClient HTTPDoer, item ImportItem) (ProviderImage, string, error) {
	switch item.Type {
	case "url":
		image, err := fetchRemoteImage(ctx, httpClient, item.Value, "", storage.maxFileBytes)
		return image, item.Name, err
	case "data_url":
		mimeType, data, err := parseDataURL(item.Value)
		if err != nil {
			return ProviderImage{}, "", err
		}
		return ProviderImage{Bytes: data, MimeType: mimeType}, item.Name, nil
	case "base64":
		data, err := base64.StdEncoding.DecodeString(item.Value)
		if err != nil {
			return ProviderImage{}, "", fmt.Errorf("decode base64: %w", err)
		}
		mimeType := item.MimeType
		if mimeType == "" {
			mimeType = detectMimeType(data, "")
		}
		return ProviderImage{Bytes: data, MimeType: mimeType}, item.Name, nil
	case "data_path":
		data, mimeType, err := storage.ReadFile(item.Value)
		if err != nil {
			return ProviderImage{}, "", err
		}
		return ProviderImage{Bytes: data, MimeType: mimeType}, firstNonBlank(item.Name, basenameOnly(item.Value)), nil
	default:
		return ProviderImage{}, "", fmt.Errorf("unsupported import type")
	}
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
