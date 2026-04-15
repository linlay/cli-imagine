package imagine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStorageShouldSaveAssetAndAppendManifest(t *testing.T) {
	root := t.TempDir()
	storage := NewStorage(root, 1024*1024)
	record, err := storage.SaveAsset(SaveRequest{
		Kind:          "generated",
		SourceMode:    "babelark_images_generate",
		Model:         "gemini-2.5-flash-image",
		Prompt:        "otter",
		DefaultPrefix: "img_",
		OutputName:    " poster.final.jpeg ",
		Bytes:         pngBytes(),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.HasSuffix(record.RelativePath, ".png") {
		t.Fatalf("expected extension corrected to png, got %s", record.RelativePath)
	}
	if record.MimeType != "image/png" {
		t.Fatalf("unexpected mime type: %s", record.MimeType)
	}
	if _, err := os.Stat(filepath.Join(root, record.RelativePath)); err != nil {
		t.Fatalf("expected saved file: %v", err)
	}
	entries, err := storage.ReadManifest()
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if len(entries) != 1 || entries[0].AssetID != record.AssetID {
		t.Fatalf("unexpected manifest entries: %#v", entries)
	}
}

func TestStorageShouldRejectEscapingDataPath(t *testing.T) {
	storage := NewStorage(t.TempDir(), 1024)
	_, _, err := storage.ReadFile("../secret.png")
	if err == nil || !strings.Contains(err.Error(), "must stay within output directory") {
		t.Fatalf("expected path traversal error, got %v", err)
	}
}

func TestStorageShouldAvoidNameCollisions(t *testing.T) {
	storage := NewStorage(t.TempDir(), 1024*1024)
	first, err := storage.SaveAsset(SaveRequest{
		Kind:          "imported",
		SourceMode:    "import",
		DefaultPrefix: "import_",
		OutputName:    "same.png",
		Bytes:         pngBytes(),
	})
	if err != nil {
		t.Fatalf("save first asset: %v", err)
	}
	second, err := storage.SaveAsset(SaveRequest{
		Kind:          "imported",
		SourceMode:    "import",
		DefaultPrefix: "import_",
		OutputName:    "same.png",
		Bytes:         pngBytes(),
	})
	if err != nil {
		t.Fatalf("save second asset: %v", err)
	}
	if first.RelativePath == second.RelativePath {
		t.Fatalf("expected collision avoidance, got same path %s", first.RelativePath)
	}
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
