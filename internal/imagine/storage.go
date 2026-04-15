package imagine

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const manifestFileName = ".imagine-assets.json"

type AssetRecord struct {
	AssetID      string   `json:"assetId"`
	Kind         string   `json:"kind"`
	SourceMode   string   `json:"sourceMode"`
	Model        string   `json:"model"`
	RelativePath string   `json:"relativePath"`
	MimeType     string   `json:"mimeType"`
	SizeBytes    int64    `json:"sizeBytes"`
	SHA256       string   `json:"sha256"`
	Prompt       string   `json:"prompt"`
	SourceImages []string `json:"sourceImages"`
	CreatedAt    string   `json:"createdAt"`
}

type SaveRequest struct {
	Kind          string
	SourceMode    string
	Model         string
	Prompt        string
	SourceImages  []string
	OutputName    string
	DefaultPrefix string
	Bytes         []byte
	MimeType      string
}

type Storage struct {
	root         string
	maxFileBytes int64
	mu           sync.Mutex
}

func NewStorage(outputDir string, maxFileBytes int64) *Storage {
	if strings.TrimSpace(outputDir) == "" {
		outputDir = "."
	}
	if maxFileBytes <= 0 {
		maxFileBytes = 20 * 1024 * 1024
	}
	return &Storage{
		root:         outputDir,
		maxFileBytes: maxFileBytes,
	}
}

func (s *Storage) Root() (string, error) {
	root, err := filepath.Abs(s.root)
	if err != nil {
		return "", fmt.Errorf("resolve output dir: %w", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}
	return root, nil
}

func (s *Storage) ManifestPath() (string, error) {
	root, err := s.Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, manifestFileName), nil
}

func (s *Storage) SaveAsset(req SaveRequest) (AssetRecord, error) {
	if int64(len(req.Bytes)) > s.maxFileBytes {
		return AssetRecord{}, fmt.Errorf("file exceeds maxFileBytes limit")
	}
	root, err := s.Root()
	if err != nil {
		return AssetRecord{}, err
	}
	mimeType := detectMimeType(req.Bytes, req.MimeType)
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return AssetRecord{}, fmt.Errorf("payload is not an image")
	}
	filePath, relativePath, err := s.nextOutputPath(root, req.DefaultPrefix, req.OutputName, extensionForMime(mimeType))
	if err != nil {
		return AssetRecord{}, err
	}
	if err := os.WriteFile(filePath, req.Bytes, 0o644); err != nil {
		return AssetRecord{}, fmt.Errorf("write asset: %w", err)
	}
	record := AssetRecord{
		AssetID:      newAssetID(),
		Kind:         req.Kind,
		SourceMode:   req.SourceMode,
		Model:        req.Model,
		RelativePath: relativePath,
		MimeType:     mimeType,
		SizeBytes:    int64(len(req.Bytes)),
		SHA256:       sha256Hex(req.Bytes),
		Prompt:       req.Prompt,
		SourceImages: append([]string{}, req.SourceImages...),
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if err := s.AppendManifest([]AssetRecord{record}); err != nil {
		_ = os.Remove(filePath)
		return AssetRecord{}, err
	}
	return record, nil
}

func (s *Storage) AppendManifest(entries []AssetRecord) error {
	if len(entries) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.readManifestUnlocked()
	if err != nil {
		return err
	}
	current = append(current, entries...)
	return s.writeManifestUnlocked(current)
}

func (s *Storage) ReadManifest() ([]AssetRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readManifestUnlocked()
}

func (s *Storage) RemoveManifestEntries(assetIDs []string) error {
	if len(assetIDs) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.readManifestUnlocked()
	if err != nil {
		return err
	}
	removeSet := make(map[string]struct{}, len(assetIDs))
	for _, id := range assetIDs {
		removeSet[id] = struct{}{}
	}
	filtered := make([]AssetRecord, 0, len(current))
	for _, item := range current {
		if _, ok := removeSet[item.AssetID]; ok {
			continue
		}
		filtered = append(filtered, item)
	}
	return s.writeManifestUnlocked(filtered)
}

func (s *Storage) ReadFile(relativePath string) ([]byte, string, error) {
	root, err := s.Root()
	if err != nil {
		return nil, "", err
	}
	absPath, err := secureJoin(root, relativePath)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, "", fmt.Errorf("read data_path: %w", err)
	}
	if int64(len(data)) > s.maxFileBytes {
		return nil, "", fmt.Errorf("data_path exceeds maxFileBytes limit")
	}
	return data, detectMimeType(data, ""), nil
}

func (s *Storage) RemoveRelative(relativePath string) error {
	root, err := s.Root()
	if err != nil {
		return err
	}
	absPath, err := secureJoin(root, relativePath)
	if err != nil {
		return err
	}
	if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Storage) readManifestUnlocked() ([]AssetRecord, error) {
	manifestPath, err := s.ManifestPath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(manifestPath)
	if os.IsNotExist(err) {
		return []AssetRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return []AssetRecord{}, nil
	}
	var entries []AssetRecord
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	return entries, nil
}

func (s *Storage) writeManifestUnlocked(entries []AssetRecord) error {
	manifestPath, err := s.ManifestPath()
	if err != nil {
		return err
	}
	payload, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	tempPath := manifestPath + ".tmp"
	if err := os.WriteFile(tempPath, payload, 0o644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	if err := os.Rename(tempPath, manifestPath); err != nil {
		return fmt.Errorf("replace manifest: %w", err)
	}
	return nil
}

func (s *Storage) nextOutputPath(root, prefix, outputName, actualExt string) (string, string, error) {
	filename := buildOutputName(prefix, outputName, actualExt)
	target := filepath.Join(root, filename)
	for i := 1; ; i++ {
		if err := ensureWithinRoot(root, target); err != nil {
			return "", "", err
		}
		if _, err := os.Stat(target); os.IsNotExist(err) {
			relative, err := filepath.Rel(root, target)
			if err != nil {
				return "", "", fmt.Errorf("build relative path: %w", err)
			}
			return target, filepath.ToSlash(relative), nil
		}
		base := strings.TrimSuffix(filename, filepath.Ext(filename))
		filename = fmt.Sprintf("%s_%d%s", base, i, actualExt)
		target = filepath.Join(root, filename)
	}
}

func secureJoin(root, relativePath string) (string, error) {
	if filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("data_path must be relative")
	}
	cleaned := filepath.Clean(strings.TrimSpace(relativePath))
	if cleaned == "." || strings.HasPrefix(cleaned, "..") {
		return "", fmt.Errorf("data_path must stay within output directory")
	}
	absPath := filepath.Join(root, cleaned)
	if err := ensureWithinRoot(root, absPath); err != nil {
		return "", err
	}
	return absPath, nil
}

func buildOutputName(prefix, outputName, actualExt string) string {
	if actualExt == "" {
		actualExt = ".img"
	}
	base := basenameOnly(outputName)
	if base == "" {
		return fmt.Sprintf("%s%s_%s%s", prefix, time.Now().UTC().Format("20060102T150405"), randomShortID(), actualExt)
	}
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	name = sanitizeFileName(name)
	if name == "" {
		name = prefix + randomShortID()
	}
	return name + actualExt
}

func sanitizeFileName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "_")
	value = invalidFilenameChars.ReplaceAllString(value, "_")
	value = strings.Trim(value, "._")
	return value
}

var invalidFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func ensureWithinRoot(root, target string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	relative, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return fmt.Errorf("check relative path: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path must stay within output directory")
	}
	return nil
}

func newAssetID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return randomShortID()
	}
	return hex.EncodeToString(buf)
}

func randomShortID() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "rand"
	}
	return hex.EncodeToString(buf)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
