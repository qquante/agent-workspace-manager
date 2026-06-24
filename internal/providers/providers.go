package providers

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultRepoManifestPath  = "manifests/providers/default.yaml"
	DefaultImageManifestPath = "/opt/stola/manifests/providers/default.yaml"

	WorkspaceManifestPath      = ".stola/providers/default.yaml"
	WorkspaceCatalogPath       = ".stola/providers/catalog.json"
	WorkspaceEnvExamplePath    = ".stola/providers/env.example"
	WorkspaceLiteLLMConfigPath = ".stola/providers/litellm.config.yaml"
	WorkspaceOpenAICompatPath  = ".stola/providers/openai-compatible.json"

	ContainerLiteLLMConfigPath = "/home/stola/.config/litellm/config.yaml"
)

type Manifest struct {
	Version         int        `yaml:"version" json:"version"`
	Name            string     `yaml:"name" json:"name"`
	Description     string     `yaml:"description" json:"description,omitempty"`
	DefaultProvider string     `yaml:"default_provider" json:"default_provider,omitempty"`
	Providers       []Provider `yaml:"providers" json:"providers"`
}

type Provider struct {
	ID            string   `yaml:"id" json:"id"`
	Title         string   `yaml:"title" json:"title"`
	Kind          string   `yaml:"kind" json:"kind"`
	BaseURL       string   `yaml:"base_url" json:"base_url,omitempty"`
	APIKeyEnv     string   `yaml:"api_key_env" json:"api_key_env,omitempty"`
	ExtraEnv      []string `yaml:"extra_env" json:"extra_env,omitempty"`
	LiteLLMModel  string   `yaml:"litellm_model" json:"litellm_model,omitempty"`
	ModelExamples []string `yaml:"model_examples" json:"model_examples,omitempty"`
	Enabled       bool     `yaml:"enabled" json:"enabled"`
	Notes         string   `yaml:"notes" json:"notes,omitempty"`
}

type Bundle struct {
	Manifest Manifest
	Files    map[string][]byte
}

var (
	providerIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)
	envNamePattern    = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
)

func ResolveManifestPath(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	if envPath := strings.TrimSpace(os.Getenv("STOLA_PROVIDERS_MANIFEST")); envPath != "" {
		return envPath
	}
	if _, err := os.Stat(DefaultRepoManifestPath); err == nil {
		return DefaultRepoManifestPath
	}
	return DefaultImageManifestPath
}

func LoadFile(filePath string) (Manifest, []byte, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return Manifest{}, nil, err
	}
	manifest, err := Parse(raw)
	if err != nil {
		return Manifest{}, nil, err
	}
	return manifest, raw, nil
}

func Parse(raw []byte) (Manifest, error) {
	var manifest Manifest
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (m Manifest) Validate() error {
	var problems []string
	if m.Version <= 0 {
		problems = append(problems, "version must be positive")
	}
	if strings.TrimSpace(m.Name) == "" {
		problems = append(problems, "name is required")
	}
	if len(m.Providers) == 0 {
		problems = append(problems, "providers must not be empty")
	}
	seen := map[string]bool{}
	hasDefault := strings.TrimSpace(m.DefaultProvider) == ""
	for index, provider := range m.Providers {
		location := fmt.Sprintf("providers[%d]", index)
		id := strings.TrimSpace(provider.ID)
		if id == "" {
			problems = append(problems, location+".id is required")
		} else if !providerIDPattern.MatchString(id) {
			problems = append(problems, location+".id must match ^[a-z][a-z0-9_.-]*$")
		}
		if seen[id] {
			problems = append(problems, fmt.Sprintf("duplicate provider id %q", id))
		}
		seen[id] = true
		if id == m.DefaultProvider {
			hasDefault = true
		}
		if strings.TrimSpace(provider.Title) == "" {
			problems = append(problems, location+".title is required")
		}
		switch provider.Kind {
		case "openai_compatible", "anthropic", "google_gemini", "azure_openai", "aws_bedrock", "local", "litellm_proxy", "custom":
		default:
			problems = append(problems, location+".kind is invalid")
		}
		if provider.APIKeyEnv != "" && !envNamePattern.MatchString(provider.APIKeyEnv) {
			problems = append(problems, location+".api_key_env must be an uppercase environment variable name")
		}
		for extraIndex, envName := range provider.ExtraEnv {
			if !envNamePattern.MatchString(envName) {
				problems = append(problems, fmt.Sprintf("%s.extra_env[%d] must be an uppercase environment variable name", location, extraIndex))
			}
		}
	}
	if !hasDefault {
		problems = append(problems, "default_provider must match a provider id")
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func RenderBundle(manifest Manifest, rawManifest []byte) (Bundle, error) {
	if err := manifest.Validate(); err != nil {
		return Bundle{}, err
	}
	if len(rawManifest) == 0 {
		rendered, err := yaml.Marshal(manifest)
		if err != nil {
			return Bundle{}, err
		}
		rawManifest = rendered
	}
	catalog, err := renderCatalog(manifest)
	if err != nil {
		return Bundle{}, err
	}
	envExample := renderEnvExample(manifest)
	litellm, err := renderLiteLLMConfig(manifest)
	if err != nil {
		return Bundle{}, err
	}
	openaiCompat, err := renderOpenAICompatible(manifest)
	if err != nil {
		return Bundle{}, err
	}
	return Bundle{
		Manifest: manifest,
		Files: map[string][]byte{
			WorkspaceManifestPath:      rawManifest,
			WorkspaceCatalogPath:       catalog,
			WorkspaceEnvExamplePath:    envExample,
			WorkspaceLiteLLMConfigPath: litellm,
			WorkspaceOpenAICompatPath:  openaiCompat,
		},
	}, nil
}

func LoadAndRender(filePath string) (Bundle, error) {
	manifest, raw, err := LoadFile(filePath)
	if err != nil {
		return Bundle{}, err
	}
	return RenderBundle(manifest, raw)
}

func WriteBundleToDir(bundle Bundle, workspaceDir string) error {
	for filePath, content := range bundle.Files {
		if err := validateRelativeWorkspacePath(filePath); err != nil {
			return err
		}
		target := path.Clean(filePath)
		hostTarget := strings.ReplaceAll(target, "/", string(os.PathSeparator))
		fullPath := workspaceDir + string(os.PathSeparator) + hostTarget
		if err := os.MkdirAll(parentDir(fullPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(fullPath, content, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func EnabledProviders(manifest Manifest) []Provider {
	var out []Provider
	for _, provider := range manifest.Providers {
		if provider.Enabled {
			out = append(out, provider)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func FindProvider(manifest Manifest, id string) (Provider, bool) {
	id = strings.TrimSpace(id)
	for _, provider := range manifest.Providers {
		if provider.ID == id {
			return provider, true
		}
	}
	return Provider{}, false
}

func EnvNames(manifest Manifest) []string {
	return providerEnvNames(manifest)
}

func renderCatalog(manifest Manifest) ([]byte, error) {
	type providerView struct {
		ID            string   `json:"id"`
		Title         string   `json:"title"`
		Kind          string   `json:"kind"`
		BaseURL       string   `json:"base_url,omitempty"`
		APIKeyEnv     string   `json:"api_key_env,omitempty"`
		ExtraEnv      []string `json:"extra_env,omitempty"`
		LiteLLMModel  string   `json:"litellm_model,omitempty"`
		ModelExamples []string `json:"model_examples,omitempty"`
		Enabled       bool     `json:"enabled"`
		Notes         string   `json:"notes,omitempty"`
	}
	doc := struct {
		Version         int            `json:"version"`
		Name            string         `json:"name"`
		Description     string         `json:"description,omitempty"`
		DefaultProvider string         `json:"default_provider,omitempty"`
		Providers       []providerView `json:"providers"`
	}{
		Version:         manifest.Version,
		Name:            manifest.Name,
		Description:     manifest.Description,
		DefaultProvider: manifest.DefaultProvider,
	}
	for _, provider := range manifest.Providers {
		doc.Providers = append(doc.Providers, providerView{
			ID:            provider.ID,
			Title:         provider.Title,
			Kind:          provider.Kind,
			BaseURL:       provider.BaseURL,
			APIKeyEnv:     provider.APIKeyEnv,
			ExtraEnv:      append([]string(nil), provider.ExtraEnv...),
			LiteLLMModel:  provider.LiteLLMModel,
			ModelExamples: append([]string(nil), provider.ModelExamples...),
			Enabled:       provider.Enabled,
			Notes:         provider.Notes,
		})
	}
	return json.MarshalIndent(doc, "", "  ")
}

func renderEnvExample(manifest Manifest) []byte {
	lines := []string{
		"# Stola provider environment example.",
		"# Copy values into your secret store or runtime env; do not commit real keys.",
		"",
	}
	envs := providerEnvNames(manifest)
	for _, envName := range envs {
		lines = append(lines, fmt.Sprintf("export %s=\"\"", envName))
	}
	lines = append(lines, "")
	return []byte(strings.Join(lines, "\n"))
}

func renderLiteLLMConfig(manifest Manifest) ([]byte, error) {
	type params struct {
		Model   string `yaml:"model"`
		APIKey  string `yaml:"api_key,omitempty"`
		BaseURL string `yaml:"api_base,omitempty"`
	}
	type modelEntry struct {
		ModelName     string `yaml:"model_name"`
		LiteLLMParams params `yaml:"litellm_params"`
	}
	doc := struct {
		ModelList []modelEntry `yaml:"model_list"`
	}{}
	for _, provider := range EnabledProviders(manifest) {
		if strings.TrimSpace(provider.LiteLLMModel) == "" {
			continue
		}
		entry := modelEntry{
			ModelName: provider.ID,
			LiteLLMParams: params{
				Model: provider.LiteLLMModel,
			},
		}
		if provider.APIKeyEnv != "" {
			entry.LiteLLMParams.APIKey = "os.environ/" + provider.APIKeyEnv
		}
		if provider.BaseURL != "" && (provider.Kind == "openai_compatible" || provider.Kind == "local" || provider.Kind == "litellm_proxy") {
			entry.LiteLLMParams.BaseURL = provider.BaseURL
		}
		doc.ModelList = append(doc.ModelList, entry)
	}
	sort.SliceStable(doc.ModelList, func(i, j int) bool { return doc.ModelList[i].ModelName < doc.ModelList[j].ModelName })
	return yaml.Marshal(doc)
}

func renderOpenAICompatible(manifest Manifest) ([]byte, error) {
	doc := map[string]map[string]string{}
	for _, provider := range EnabledProviders(manifest) {
		if provider.Kind != "openai_compatible" && provider.Kind != "litellm_proxy" && provider.Kind != "local" {
			continue
		}
		item := map[string]string{}
		if provider.BaseURL != "" {
			item["base_url"] = provider.BaseURL
		}
		if provider.APIKeyEnv != "" {
			item["api_key_env"] = provider.APIKeyEnv
		}
		if provider.LiteLLMModel != "" {
			item["litellm_model"] = provider.LiteLLMModel
		}
		doc[provider.ID] = item
	}
	return json.MarshalIndent(doc, "", "  ")
}

func providerEnvNames(manifest Manifest) []string {
	seen := map[string]bool{}
	for _, provider := range manifest.Providers {
		if provider.APIKeyEnv != "" {
			seen[provider.APIKeyEnv] = true
		}
		for _, envName := range provider.ExtraEnv {
			if envName != "" {
				seen[envName] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for envName := range seen {
		out = append(out, envName)
	}
	sort.Strings(out)
	return out
}

func validateRelativeWorkspacePath(filePath string) error {
	clean := path.Clean(strings.ReplaceAll(filePath, "\\", "/"))
	if clean == "." || strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, "../") || clean == ".." || strings.Contains(clean, "\x00") {
		return fmt.Errorf("invalid workspace path: %q", filePath)
	}
	return nil
}

func parentDir(filePath string) string {
	idx := strings.LastIndex(filePath, string(os.PathSeparator))
	if idx == -1 {
		return "."
	}
	return filePath[:idx]
}
