package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"

	"github.com/linlay/cli-imagine/internal/schema"
)

type ModelEndpointConfig struct {
	BaseURL  string `toml:"base_url" yaml:"baseUrl" json:"base_url"`
	ProxyURL string `toml:"proxy_url" yaml:"proxyUrl" json:"proxy_url"`
}

type ModelAuthConfig struct {
	APIKey     string `toml:"api_key" yaml:"apiKey" json:"api_key,omitempty"`
	APIKeyEnv  string `toml:"api_key_env" json:"api_key_env,omitempty"`
	APIKeyFile string `toml:"api_key_file" json:"api_key_file,omitempty"`
}

type ModelResponseConfig struct {
	DefaultFormat  string            `toml:"default_format" yaml:"defaultFormat" json:"default_format"`
	AllowedFormats []string          `toml:"allowed_formats" yaml:"allowedFormats" json:"allowed_formats,omitempty"`
	ParserByFormat map[string]string `toml:"parser_by_format" yaml:"parserByFormat" json:"parser_by_format"`
}

type ModelRequestConfig struct {
	Kind             string         `toml:"kind" yaml:"kind" json:"kind"`
	SizeMode         string         `toml:"size_mode" yaml:"sizeMode" json:"size_mode"`
	AspectField      string         `toml:"aspect_field" yaml:"aspectField" json:"aspect_field,omitempty"`
	ImageSizeField   string         `toml:"image_size_field" yaml:"imageSizeField" json:"image_size_field,omitempty"`
	DefaultArguments map[string]any `toml:"default_arguments" yaml:"defaultArguments" json:"default_arguments,omitempty"`
}

type ModelCapabilityConfig struct {
	InputSchema     map[string]any      `toml:"-" yaml:"inputSchema" json:"-"`
	InputSchemaPath string              `toml:"input_schema" json:"input_schema"`
	Request         ModelRequestConfig  `toml:"request" yaml:"request" json:"request"`
	Response        ModelResponseConfig `toml:"response" yaml:"response" json:"response"`
}

type ModelCapabilitiesConfig struct {
	Generate *ModelCapabilityConfig `toml:"generate" yaml:"generate" json:"generate,omitempty"`
	Edit     *ModelCapabilityConfig `toml:"edit" yaml:"edit" json:"edit,omitempty"`
}

type ModelConfig struct {
	Provider     string                  `toml:"-" json:"provider"`
	Name         string                  `toml:"name" yaml:"name" json:"name"`
	Endpoint     ModelEndpointConfig     `toml:"-" json:"endpoint"`
	Auth         ModelAuthConfig         `toml:"-" json:"auth"`
	Capabilities ModelCapabilitiesConfig `toml:"capabilities" yaml:"capabilities" json:"capabilities"`
}

type ProviderConfig struct {
	Name        string                `toml:"name" yaml:"name" json:"name"`
	Description string                `toml:"description" json:"description,omitempty"`
	Path        string                `toml:"-" json:"path"`
	Endpoint    *ModelEndpointConfig  `toml:"endpoint" yaml:"endpoint" json:"endpoint,omitempty"`
	Auth        *ModelAuthConfig      `toml:"auth" yaml:"auth" json:"auth,omitempty"`
	Models      []ProviderModelConfig `toml:"models" yaml:"models" json:"models"`
}

type ProviderModelConfig struct {
	Name         string                  `toml:"name" yaml:"name" json:"name"`
	Endpoint     *ModelEndpointConfig    `toml:"endpoint" yaml:"endpoint" json:"endpoint,omitempty"`
	Auth         *ModelAuthConfig        `toml:"auth" yaml:"auth" json:"auth,omitempty"`
	Capabilities ModelCapabilitiesConfig `toml:"capabilities" yaml:"capabilities" json:"capabilities"`
}

type Config struct {
	ConfigDir  string           `json:"config_dir"`
	Providers  []ProviderConfig `json:"providers"`
	Models     []ModelConfig    `json:"models"`
	ModelCount int              `json:"model_count"`
}

func LoadDir(configDir string) (Config, error) {
	files, err := findConfigFiles(configDir)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{ConfigDir: configDir}
	models := make([]ModelConfig, 0)
	providers := make([]ProviderConfig, 0, len(files))
	seenProviders := map[string]struct{}{}
	seenModels := map[string]struct{}{}

	for _, path := range files {
		provider, err := decodeProviderTOML(path)
		if err != nil {
			return Config{}, err
		}
		if err := validateProviderFile(provider); err != nil {
			return Config{}, fmt.Errorf("validate provider %q: %w", path, err)
		}

		provider.Path = path
		key := normalizeKey(provider.Name)
		if _, exists := seenProviders[key]; exists {
			return Config{}, fmt.Errorf("duplicate provider %q", provider.Name)
		}
		seenProviders[key] = struct{}{}

		resolvedModels, err := resolveProviderModels(path, provider)
		if err != nil {
			return Config{}, err
		}
		for _, model := range resolvedModels {
			key := normalizeKey(model.Name)
			if _, exists := seenModels[key]; exists {
				return Config{}, fmt.Errorf("duplicate model %q", model.Name)
			}
			seenModels[key] = struct{}{}
			models = append(models, model)
		}
		providers = append(providers, provider)
	}

	cfg.Providers = providers
	cfg.Models = models
	cfg.ModelCount = len(models)
	return cfg, nil
}

func findConfigFiles(configDir string) ([]string, error) {
	info, err := os.Stat(configDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config directory %q does not exist", configDir)
		}
		return nil, fmt.Errorf("stat config dir %q: %w", configDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("config path %q must be a directory", configDir)
	}
	entries, err := os.ReadDir(configDir)
	if err != nil {
		return nil, fmt.Errorf("read config dir %q: %w", configDir, err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".toml" {
			continue
		}
		files = append(files, filepath.Join(configDir, name))
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no provider config files found in %s", configDir)
	}
	return files, nil
}

func decodeProviderTOML(path string) (ProviderConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ProviderConfig{}, fmt.Errorf("read provider config %q: %w", path, err)
	}
	var provider ProviderConfig
	decoder := toml.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&provider); err != nil {
		return ProviderConfig{}, fmt.Errorf("decode provider config %q: %w", path, err)
	}
	return provider, nil
}

func validateProviderFile(provider ProviderConfig) error {
	if strings.TrimSpace(provider.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if len(provider.Models) == 0 {
		return fmt.Errorf("models must not be empty")
	}
	if provider.Endpoint == nil || strings.TrimSpace(provider.Endpoint.BaseURL) == "" {
		return fmt.Errorf("endpoint.base_url is required")
	}
	if err := validateProxyURL("provider", provider.Endpoint.ProxyURL); err != nil {
		return err
	}
	if err := validateAuth("provider auth", provider.Auth); err != nil {
		return err
	}
	return nil
}

func resolveProviderModels(providerPath string, provider ProviderConfig) ([]ModelConfig, error) {
	results := make([]ModelConfig, 0, len(provider.Models))
	for _, model := range provider.Models {
		if strings.TrimSpace(model.Name) == "" {
			return nil, fmt.Errorf("provider %q contains unnamed model", provider.Name)
		}
		endpoint, err := mergeEndpoint(provider.Endpoint, model.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("provider %q model %q: %w", provider.Name, model.Name, err)
		}
		auth, err := resolveAuth(provider.Auth, model.Auth)
		if err != nil {
			return nil, fmt.Errorf("provider %q model %q: %w", provider.Name, model.Name, err)
		}
		resolved := ModelConfig{
			Provider: provider.Name,
			Name:     strings.TrimSpace(model.Name),
			Endpoint: endpoint,
			Auth:     auth,
			Capabilities: ModelCapabilitiesConfig{
				Generate: cloneCapability(model.Capabilities.Generate),
				Edit:     cloneCapability(model.Capabilities.Edit),
			},
		}
		if err := loadAndValidateCapabilities(providerPath, &resolved); err != nil {
			return nil, err
		}
		results = append(results, resolved)
	}
	return results, nil
}

func mergeEndpoint(provider, model *ModelEndpointConfig) (ModelEndpointConfig, error) {
	if provider == nil {
		return ModelEndpointConfig{}, fmt.Errorf("provider endpoint is required")
	}
	endpoint := *provider
	if model != nil {
		if strings.TrimSpace(model.BaseURL) != "" {
			endpoint.BaseURL = strings.TrimSpace(model.BaseURL)
		}
		if strings.TrimSpace(model.ProxyURL) != "" {
			endpoint.ProxyURL = strings.TrimSpace(model.ProxyURL)
		}
	}
	if strings.TrimSpace(endpoint.BaseURL) == "" {
		return ModelEndpointConfig{}, fmt.Errorf("endpoint.base_url is required")
	}
	if err := validateProxyURL("endpoint", endpoint.ProxyURL); err != nil {
		return ModelEndpointConfig{}, err
	}
	return endpoint, nil
}

func resolveAuth(provider, model *ModelAuthConfig) (ModelAuthConfig, error) {
	auth := ModelAuthConfig{}
	if provider != nil {
		auth = *provider
	}
	if model != nil {
		if strings.TrimSpace(model.APIKey) != "" || strings.TrimSpace(model.APIKeyEnv) != "" || strings.TrimSpace(model.APIKeyFile) != "" {
			auth = *model
		}
	}
	if err := validateAuth("auth", &auth); err != nil {
		return ModelAuthConfig{}, err
	}
	auth.APIKey = strings.TrimSpace(auth.APIKey)
	if auth.APIKey == "" {
		switch {
		case strings.TrimSpace(auth.APIKeyEnv) != "":
			auth.APIKey = strings.TrimSpace(os.Getenv(strings.TrimSpace(auth.APIKeyEnv)))
		case strings.TrimSpace(auth.APIKeyFile) != "":
			raw, err := os.ReadFile(strings.TrimSpace(auth.APIKeyFile))
			if err != nil {
				return ModelAuthConfig{}, fmt.Errorf("read api_key_file %q: %w", auth.APIKeyFile, err)
			}
			auth.APIKey = strings.TrimSpace(string(raw))
		}
	}
	if strings.TrimSpace(auth.APIKey) == "" {
		return ModelAuthConfig{}, fmt.Errorf("resolved api key is empty")
	}
	return auth, nil
}

func validateAuth(scope string, auth *ModelAuthConfig) error {
	if auth == nil {
		return fmt.Errorf("%s is required", scope)
	}
	count := 0
	if strings.TrimSpace(auth.APIKey) != "" {
		count++
	}
	if strings.TrimSpace(auth.APIKeyEnv) != "" {
		count++
	}
	if strings.TrimSpace(auth.APIKeyFile) != "" {
		count++
	}
	if count != 1 {
		return fmt.Errorf("%s must set exactly one of api_key, api_key_env, api_key_file", scope)
	}
	return nil
}

func loadAndValidateCapabilities(providerPath string, model *ModelConfig) error {
	if model.Capabilities.Generate == nil && model.Capabilities.Edit == nil {
		return fmt.Errorf("model %q must include at least one capability", model.Name)
	}
	if model.Capabilities.Generate != nil {
		if err := loadCapabilitySchema(providerPath, model.Name, "generate", model.Capabilities.Generate); err != nil {
			return err
		}
	}
	if model.Capabilities.Edit != nil {
		if err := loadCapabilitySchema(providerPath, model.Name, "edit", model.Capabilities.Edit); err != nil {
			return err
		}
	}
	return nil
}

func loadCapabilitySchema(providerPath, modelName, operation string, capability *ModelCapabilityConfig) error {
	if strings.TrimSpace(capability.InputSchemaPath) == "" {
		return fmt.Errorf("model %q capability %s missing input_schema", modelName, operation)
	}
	schemaPath := capability.InputSchemaPath
	if !filepath.IsAbs(schemaPath) {
		schemaPath = filepath.Join(filepath.Dir(providerPath), schemaPath)
	}
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read input schema %q: %w", schemaPath, err)
	}
	var schemaMap map[string]any
	if err := json.Unmarshal(raw, &schemaMap); err != nil {
		return fmt.Errorf("decode input schema %q: %w", schemaPath, err)
	}
	capability.InputSchema = schemaMap
	if err := validateCapability(modelName, operation, *capability); err != nil {
		return err
	}
	return nil
}

func validateCapability(modelName, operation string, capability ModelCapabilityConfig) error {
	scope := fmt.Sprintf("models[%s].capabilities.%s", modelName, operation)
	if capability.InputSchema == nil {
		return fmt.Errorf("%s.input_schema is required", scope)
	}
	if _, err := schema.Compile(scope, capability.InputSchema); err != nil {
		return fmt.Errorf("%s.input_schema is invalid: %w", scope, err)
	}
	if !isSupportedRequestKind(capability.Request.Kind) {
		return fmt.Errorf("%s.request.kind contains unsupported value %q", scope, capability.Request.Kind)
	}
	if !isSupportedSizeMode(capability.Request.SizeMode) {
		return fmt.Errorf("%s.request.size_mode contains unsupported value %q", scope, capability.Request.SizeMode)
	}
	if !isSupportedAspectField(capability.Request.AspectField) {
		return fmt.Errorf("%s.request.aspect_field contains unsupported value %q", scope, capability.Request.AspectField)
	}
	if !isSupportedImageSizeField(capability.Request.ImageSizeField) {
		return fmt.Errorf("%s.request.image_size_field contains unsupported value %q", scope, capability.Request.ImageSizeField)
	}
	for key, value := range capability.Request.DefaultArguments {
		switch key {
		case "useMask", "imageOnly", "webSearch":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("%s.request.default_arguments.%s must be a boolean", scope, key)
			}
		}
	}
	defaultFormat := strings.ToLower(strings.TrimSpace(capability.Response.DefaultFormat))
	if !isSupportedResponseFormat(defaultFormat) {
		return fmt.Errorf("%s.response.default_format contains unsupported value %q", scope, capability.Response.DefaultFormat)
	}
	if parser := strings.TrimSpace(capability.Response.ParserByFormat[defaultFormat]); parser == "" {
		return fmt.Errorf("%s.response.parser_by_format must contain default_format %q", scope, defaultFormat)
	}
	for _, value := range capability.Response.AllowedFormats {
		if !isSupportedResponseFormat(value) {
			return fmt.Errorf("%s.response.allowed_formats contains unsupported value %q", scope, value)
		}
	}
	for format, parser := range capability.Response.ParserByFormat {
		if !isSupportedResponseFormat(format) {
			return fmt.Errorf("%s.response.parser_by_format contains unsupported format %q", scope, format)
		}
		if !isSupportedParserKind(parser) {
			return fmt.Errorf("%s.response.parser_by_format[%s] contains unsupported value %q", scope, format, parser)
		}
	}
	return nil
}

func isSupportedRequestKind(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "images_generate", "images_edit", "generate_content", "chat_completions":
		return true
	default:
		return false
	}
}

func isSupportedSizeMode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "standard", "ratio", "pixel_x", "pixel_star", "ratio_or_pixel":
		return true
	default:
		return false
	}
}

func isSupportedAspectField(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "aspect", "aspect_ratio", "aspectratio":
		return true
	default:
		return false
	}
}

func isSupportedImageSizeField(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "imagesize", "image_size":
		return true
	default:
		return false
	}
}

func isSupportedResponseFormat(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "url", "b64_json":
		return true
	default:
		return false
	}
}

func isSupportedParserKind(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "data_b64_json", "data_url", "candidates_inline_data", "message_content_image":
		return true
	default:
		return false
	}
}

func validateProxyURL(scope, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if !(strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")) {
		return fmt.Errorf("%s.proxy_url must use http or https", scope)
	}
	return nil
}

func normalizeKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func cloneCapability(in *ModelCapabilityConfig) *ModelCapabilityConfig {
	if in == nil {
		return nil
	}
	out := *in
	out.InputSchema = cloneJSONMap(in.InputSchema)
	out.Request.DefaultArguments = cloneJSONMap(in.Request.DefaultArguments)
	out.Response.AllowedFormats = append([]string{}, in.Response.AllowedFormats...)
	out.Response.ParserByFormat = cloneStringMap(in.Response.ParserByFormat)
	return &out
}

func cloneJSONMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
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

type yamlProviderConfig struct {
	Name     string                `yaml:"name"`
	Endpoint *ModelEndpointConfig  `yaml:"endpoint"`
	Auth     *ModelAuthConfig      `yaml:"auth"`
	Models   []ProviderModelConfig `yaml:"models"`
}

type importProviderTOML struct {
	Name        string                `toml:"name"`
	Description string                `toml:"description,omitempty"`
	Endpoint    *ModelEndpointConfig  `toml:"endpoint"`
	Auth        *ModelAuthConfig      `toml:"auth"`
	Models      []importProviderModel `toml:"models"`
}

type importProviderModel struct {
	Name         string                     `toml:"name"`
	Endpoint     *ModelEndpointConfig       `toml:"endpoint,omitempty"`
	Auth         *ModelAuthConfig           `toml:"auth,omitempty"`
	Capabilities importModelCapabilitiesRef `toml:"capabilities"`
}

type importModelCapabilitiesRef struct {
	Generate *ModelCapabilityConfig `toml:"generate,omitempty"`
	Edit     *ModelCapabilityConfig `toml:"edit,omitempty"`
}

func ImportYAML(fromPath, toDir string) ([]string, error) {
	paths, err := resolveYAMLInputs(fromPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(toDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir %q: %w", toDir, err)
	}

	written := make([]string, 0)
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read yaml config %q: %w", path, err)
		}
		var provider yamlProviderConfig
		if err := yaml.Unmarshal(raw, &provider); err != nil {
			return nil, fmt.Errorf("decode yaml config %q: %w", path, err)
		}
		if strings.TrimSpace(provider.Name) == "" {
			return nil, fmt.Errorf("yaml config %q missing provider name", path)
		}

		providerDir := filepath.Join(toDir, "schemas", safeName(provider.Name))
		if err := os.MkdirAll(providerDir, 0o755); err != nil {
			return nil, fmt.Errorf("create schema dir %q: %w", providerDir, err)
		}

		out := importProviderTOML{
			Name:     provider.Name,
			Endpoint: provider.Endpoint,
			Auth:     provider.Auth,
			Models:   make([]importProviderModel, 0, len(provider.Models)),
		}
		for _, model := range provider.Models {
			entry := importProviderModel{
				Name:     model.Name,
				Endpoint: model.Endpoint,
				Auth:     model.Auth,
			}
			if model.Capabilities.Generate != nil {
				capability, schemaFile, err := exportCapabilitySchema(provider.Name, model.Name, "generate", *model.Capabilities.Generate, toDir)
				if err != nil {
					return nil, err
				}
				capability.InputSchema = nil
				capability.InputSchemaPath = schemaFile
				entry.Capabilities.Generate = &capability
				written = append(written, filepath.Join(toDir, schemaFile))
			}
			if model.Capabilities.Edit != nil {
				capability, schemaFile, err := exportCapabilitySchema(provider.Name, model.Name, "edit", *model.Capabilities.Edit, toDir)
				if err != nil {
					return nil, err
				}
				capability.InputSchema = nil
				capability.InputSchemaPath = schemaFile
				entry.Capabilities.Edit = &capability
				written = append(written, filepath.Join(toDir, schemaFile))
			}
			out.Models = append(out.Models, entry)
		}

		payload, err := toml.Marshal(out)
		if err != nil {
			return nil, fmt.Errorf("encode provider %q as toml: %w", provider.Name, err)
		}
		providerFile := filepath.Join(toDir, safeName(provider.Name)+".toml")
		if err := os.WriteFile(providerFile, payload, 0o644); err != nil {
			return nil, fmt.Errorf("write provider file %q: %w", providerFile, err)
		}
		written = append(written, providerFile)
	}

	sort.Strings(written)
	return written, nil
}

func resolveYAMLInputs(fromPath string) ([]string, error) {
	info, err := os.Stat(fromPath)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", fromPath, err)
	}
	if !info.IsDir() {
		return []string{fromPath}, nil
	}
	matches, err := filepath.Glob(filepath.Join(fromPath, "*.yml"))
	if err != nil {
		return nil, fmt.Errorf("glob yaml files in %q: %w", fromPath, err)
	}
	files := make([]string, 0, len(matches))
	for _, path := range matches {
		if strings.HasSuffix(strings.ToLower(path), ".example.yml") {
			continue
		}
		files = append(files, path)
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no yaml config files found in %s", fromPath)
	}
	return files, nil
}

func exportCapabilitySchema(provider, model, operation string, capability ModelCapabilityConfig, toDir string) (ModelCapabilityConfig, string, error) {
	relPath := filepath.Join("schemas", safeName(provider), safeName(model)+"."+operation+".json")
	fullPath := filepath.Join(toDir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return ModelCapabilityConfig{}, "", fmt.Errorf("create schema parent for %q: %w", fullPath, err)
	}
	payload, err := json.MarshalIndent(capability.InputSchema, "", "  ")
	if err != nil {
		return ModelCapabilityConfig{}, "", fmt.Errorf("encode schema for %s/%s/%s: %w", provider, model, operation, err)
	}
	if err := os.WriteFile(fullPath, payload, 0o644); err != nil {
		return ModelCapabilityConfig{}, "", fmt.Errorf("write schema %q: %w", fullPath, err)
	}
	return capability, filepath.ToSlash(relPath), nil
}

func safeName(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unnamed"
	}
	var b strings.Builder
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	collapsed := strings.Trim(b.String(), "-")
	collapsed = strings.ReplaceAll(collapsed, "--", "-")
	if collapsed == "" {
		return "unnamed"
	}
	return collapsed
}

func ProviderByName(cfg Config) map[string]ProviderConfig {
	result := make(map[string]ProviderConfig, len(cfg.Providers))
	for _, provider := range cfg.Providers {
		result[provider.Name] = provider
	}
	return result
}

func LookupModel(cfg Config, name string) (ModelConfig, bool) {
	key := normalizeKey(name)
	for _, model := range cfg.Models {
		if normalizeKey(model.Name) == key {
			return model, true
		}
	}
	return ModelConfig{}, false
}

func ParseScalar(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := strconv.ParseBool(value); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseFloat(value, 64); err == nil {
		return parsed
	}
	return value
}
