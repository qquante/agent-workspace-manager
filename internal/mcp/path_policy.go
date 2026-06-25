package mcp

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
)

type ArgumentPolicyError struct {
	Reason    string
	Rule      string
	Fields    []string
	PathCount int
}

func (e *ArgumentPolicyError) Error() string {
	if e == nil {
		return ""
	}
	if e.Reason == "" {
		return "argument path policy denied"
	}
	return e.Reason
}

func (e *ArgumentPolicyError) AuditDetails() map[string]any {
	if e == nil {
		return map[string]any{}
	}
	fields := append([]string(nil), e.Fields...)
	sort.Strings(fields)
	return map[string]any{
		"argument_policy_reason": e.Reason,
		"argument_policy_rule":   e.Rule,
		"argument_path_fields":   fields,
		"argument_path_count":    e.PathCount,
	}
}

type filesystemPathSpec struct {
	Field    string
	Array    bool
	Required bool
}

type ArgumentPolicy struct {
	Enabled                   *bool                          `yaml:"enabled" json:"enabled,omitempty"`
	DenyUnknownTools          *bool                          `yaml:"deny_unknown_tools" json:"deny_unknown_tools,omitempty"`
	Tools                     map[string][]ArgumentPathField `yaml:"tools" json:"tools,omitempty"`
	ProtectedSegments         []string                       `yaml:"protected_segments" json:"protected_segments,omitempty"`
	ProtectedBasenames        []string                       `yaml:"protected_basenames" json:"protected_basenames,omitempty"`
	ProtectedSuffixes         []string                       `yaml:"protected_suffixes" json:"protected_suffixes,omitempty"`
	ProtectedNameMarkers      []string                       `yaml:"protected_name_markers" json:"protected_name_markers,omitempty"`
	AllowProtectedSegments    []string                       `yaml:"allow_protected_segments" json:"allow_protected_segments,omitempty"`
	AllowProtectedBasenames   []string                       `yaml:"allow_protected_basenames" json:"allow_protected_basenames,omitempty"`
	AllowProtectedSuffixes    []string                       `yaml:"allow_protected_suffixes" json:"allow_protected_suffixes,omitempty"`
	AllowProtectedNameMarkers []string                       `yaml:"allow_protected_name_markers" json:"allow_protected_name_markers,omitempty"`
	Overrides                 []ArgumentPolicyOverride       `yaml:"overrides" json:"overrides,omitempty"`
}

type ArgumentPathField struct {
	Field    string `yaml:"field" json:"field"`
	Type     string `yaml:"type,omitempty" json:"type,omitempty"`
	Required *bool  `yaml:"required,omitempty" json:"required,omitempty"`
}

type ArgumentPolicyOverride struct {
	Scope  ArgumentPolicyScope `yaml:"scope" json:"scope"`
	Policy ArgumentPolicy      `yaml:"policy" json:"policy"`
}

type ArgumentPolicyScope struct {
	VPSIDs  []string `yaml:"vps_ids" json:"vps_ids,omitempty"`
	Plans   []string `yaml:"plans" json:"plans,omitempty"`
	UserIDs []string `yaml:"user_ids" json:"user_ids,omitempty"`
}

type ArgumentPolicyContext struct {
	VPSID  string
	Plan   string
	UserID string
}

func DefaultArgumentPolicy() ArgumentPolicy {
	return ArgumentPolicy{
		Enabled:          boolPtr(true),
		DenyUnknownTools: boolPtr(true),
		Tools: map[string][]ArgumentPathField{
			"read_file":                {{Field: "path", Type: "string", Required: boolPtr(true)}},
			"read_text_file":           {{Field: "path", Type: "string", Required: boolPtr(true)}},
			"read_media_file":          {{Field: "path", Type: "string", Required: boolPtr(true)}},
			"read_multiple_files":      {{Field: "paths", Type: "string_array", Required: boolPtr(true)}},
			"write_file":               {{Field: "path", Type: "string", Required: boolPtr(true)}},
			"edit_file":                {{Field: "path", Type: "string", Required: boolPtr(true)}},
			"create_directory":         {{Field: "path", Type: "string", Required: boolPtr(true)}},
			"list_directory":           {{Field: "path", Type: "string", Required: boolPtr(true)}},
			"directory_tree":           {{Field: "path", Type: "string", Required: boolPtr(true)}},
			"move_file":                {{Field: "source", Type: "string", Required: boolPtr(true)}, {Field: "destination", Type: "string", Required: boolPtr(true)}},
			"search_files":             {{Field: "path", Type: "string", Required: boolPtr(true)}},
			"get_file_info":            {{Field: "path", Type: "string", Required: boolPtr(true)}},
			"list_allowed_directories": nil,
		},
		ProtectedSegments: []string{
			".stola",
			".git",
			".ssh",
			".gnupg",
			".aws",
			".azure",
			".gcloud",
			".kube",
			".docker",
			".config",
		},
		ProtectedBasenames: []string{
			".env",
			".env.local",
			".env.production",
			".git-credentials",
			".gitconfig",
			".netrc",
			".npmrc",
			".pypirc",
			"authorized_keys",
			"id_dsa",
			"id_ecdsa",
			"id_ed25519",
			"id_rsa",
			"known_hosts",
			"opencode.json",
		},
		ProtectedSuffixes: []string{
			".key",
			".pem",
			".p12",
			".pfx",
			".kubeconfig",
		},
		ProtectedNameMarkers: []string{
			"secret",
			"token",
			"credential",
			"private_key",
		},
	}
}

func ValidateArgumentPathPolicy(workspaceRoot string, server Server, toolName string, raw json.RawMessage, contexts ...ArgumentPolicyContext) error {
	if server.ID != "filesystem" {
		return nil
	}
	ctx := ArgumentPolicyContext{}
	if len(contexts) > 0 {
		ctx = contexts[0]
	}
	policy := EffectiveArgumentPolicy(server.ArgumentPolicy, ctx)
	if !argumentPolicyBool(policy.Enabled, true) {
		return nil
	}
	toolName = strings.TrimSpace(toolName)
	specs, ok := argumentPolicyToolSpecs(policy, toolName)
	if !ok {
		if !argumentPolicyBool(policy.DenyUnknownTools, true) {
			return nil
		}
		return &ArgumentPolicyError{
			Reason: "filesystem tool is not covered by Stola argument policy",
			Rule:   "unsupported_filesystem_tool",
		}
	}
	if len(specs) == 0 {
		return nil
	}
	var arguments map[string]json.RawMessage
	if err := json.Unmarshal(compactJSON(raw), &arguments); err != nil {
		return &ArgumentPolicyError{
			Reason: "tool arguments must be a JSON object for path policy",
			Rule:   "invalid_arguments",
		}
	}
	fields := []string{}
	pathCount := 0
	for _, spec := range specs {
		value, exists := arguments[spec.Field]
		if !exists {
			if spec.Required {
				return &ArgumentPolicyError{
					Reason:    "required path argument is missing",
					Rule:      "missing_path_argument",
					Fields:    append(fields, spec.Field),
					PathCount: pathCount,
				}
			}
			continue
		}
		fields = append(fields, spec.Field)
		if spec.Array {
			var paths []string
			if err := json.Unmarshal(value, &paths); err != nil {
				return &ArgumentPolicyError{
					Reason:    "path argument must be an array of strings",
					Rule:      "invalid_path_argument",
					Fields:    fields,
					PathCount: pathCount,
				}
			}
			for _, item := range paths {
				pathCount++
				if err := validateWorkspaceArgumentPath(workspaceRoot, item, policy); err != nil {
					err.Fields = fields
					err.PathCount = pathCount
					return err
				}
			}
			continue
		}
		var pathValue string
		if err := json.Unmarshal(value, &pathValue); err != nil {
			return &ArgumentPolicyError{
				Reason:    "path argument must be a string",
				Rule:      "invalid_path_argument",
				Fields:    fields,
				PathCount: pathCount,
			}
		}
		pathCount++
		if err := validateWorkspaceArgumentPath(workspaceRoot, pathValue, policy); err != nil {
			err.Fields = fields
			err.PathCount = pathCount
			return err
		}
	}
	return nil
}

func EffectiveArgumentPolicy(config ArgumentPolicy, ctx ArgumentPolicyContext) ArgumentPolicy {
	policy := mergeArgumentPolicy(DefaultArgumentPolicy(), config)
	for _, override := range config.Overrides {
		if !argumentPolicyScopeMatches(override.Scope, ctx) {
			continue
		}
		policy = mergeArgumentPolicy(policy, override.Policy)
	}
	return normalizeArgumentPolicy(policy)
}

func validateWorkspaceArgumentPath(workspaceRoot string, value string, policy ArgumentPolicy) *ArgumentPolicyError {
	workspaceRoot = cleanWorkspaceRoot(workspaceRoot)
	raw := strings.TrimSpace(value)
	if raw == "" {
		return &ArgumentPolicyError{Reason: "path argument is empty", Rule: "empty_path"}
	}
	if strings.Contains(raw, "\x00") {
		return &ArgumentPolicyError{Reason: "path argument contains a NUL byte", Rule: "nul_byte_path"}
	}
	if strings.Contains(raw, ":") {
		return &ArgumentPolicyError{Reason: "path argument must be a container workspace path", Rule: "host_path_syntax"}
	}
	normalized := strings.ReplaceAll(raw, "\\", "/")
	var clean string
	if strings.HasPrefix(normalized, "/") {
		clean = path.Clean(normalized)
	} else {
		clean = path.Join(workspaceRoot, normalized)
	}
	if clean != workspaceRoot && !strings.HasPrefix(clean, workspaceRoot+"/") {
		return &ArgumentPolicyError{Reason: "path argument escapes the workspace root", Rule: "outside_workspace"}
	}
	if rule, ok := protectedWorkspacePath(workspaceRoot, clean, policy); ok {
		return &ArgumentPolicyError{Reason: "path argument targets protected workspace data", Rule: rule}
	}
	return nil
}

func cleanWorkspaceRoot(root string) string {
	root = strings.TrimSpace(strings.ReplaceAll(root, "\\", "/"))
	if root == "" {
		root = "/workspace"
	}
	if !strings.HasPrefix(root, "/") {
		root = "/" + root
	}
	return path.Clean(root)
}

func protectedWorkspacePath(workspaceRoot string, clean string, policy ArgumentPolicy) (string, bool) {
	rel := strings.TrimPrefix(clean, workspaceRoot)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return "", false
	}
	parts := strings.Split(rel, "/")
	segments := stringSet(policy.ProtectedSegments)
	for _, part := range parts {
		lower := strings.ToLower(part)
		if _, ok := segments[lower]; ok {
			return protectedPathRule("segment", lower), true
		}
	}
	base := strings.ToLower(path.Base(clean))
	basenames := stringSet(policy.ProtectedBasenames)
	if _, ok := basenames[base]; ok {
		return protectedPathRule("basename", base), true
	}
	if strings.HasPrefix(base, ".env.") {
		return "protected_env_file", true
	}
	for _, suffix := range normalizeStrings(policy.ProtectedSuffixes) {
		if strings.HasSuffix(base, suffix) {
			return protectedSuffixRule(suffix), true
		}
	}
	for _, marker := range normalizeStrings(policy.ProtectedNameMarkers) {
		if strings.Contains(base, marker) {
			return fmt.Sprintf("protected_%s_file", marker), true
		}
	}
	return "", false
}

func validateArgumentPolicy(location string, policy ArgumentPolicy) []string {
	var problems []string
	for tool, specs := range policy.Tools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			problems = append(problems, location+".tools contains an empty tool name")
		}
		for index, spec := range specs {
			specLocation := fmt.Sprintf("%s.tools.%s[%d]", location, tool, index)
			if strings.TrimSpace(spec.Field) == "" {
				problems = append(problems, specLocation+".field is required")
			}
			switch specType(spec) {
			case "string", "string_array":
			default:
				problems = append(problems, specLocation+".type must be string or string_array")
			}
		}
	}
	for index, override := range policy.Overrides {
		overrideLocation := fmt.Sprintf("%s.overrides[%d]", location, index)
		if len(override.Scope.VPSIDs) == 0 && len(override.Scope.Plans) == 0 && len(override.Scope.UserIDs) == 0 {
			problems = append(problems, overrideLocation+".scope must include vps_ids, plans, or user_ids")
		}
		problems = append(problems, validateArgumentPolicy(overrideLocation+".policy", override.Policy)...)
	}
	return problems
}

func mergeArgumentPolicy(base ArgumentPolicy, override ArgumentPolicy) ArgumentPolicy {
	out := base
	if override.Enabled != nil {
		out.Enabled = override.Enabled
	}
	if override.DenyUnknownTools != nil {
		out.DenyUnknownTools = override.DenyUnknownTools
	}
	if out.Tools == nil {
		out.Tools = map[string][]ArgumentPathField{}
	} else {
		out.Tools = cloneArgumentTools(out.Tools)
	}
	for tool, specs := range override.Tools {
		out.Tools[strings.TrimSpace(tool)] = append([]ArgumentPathField(nil), specs...)
	}
	out.ProtectedSegments = append(out.ProtectedSegments, override.ProtectedSegments...)
	out.ProtectedBasenames = append(out.ProtectedBasenames, override.ProtectedBasenames...)
	out.ProtectedSuffixes = append(out.ProtectedSuffixes, override.ProtectedSuffixes...)
	out.ProtectedNameMarkers = append(out.ProtectedNameMarkers, override.ProtectedNameMarkers...)
	out.AllowProtectedSegments = append(out.AllowProtectedSegments, override.AllowProtectedSegments...)
	out.AllowProtectedBasenames = append(out.AllowProtectedBasenames, override.AllowProtectedBasenames...)
	out.AllowProtectedSuffixes = append(out.AllowProtectedSuffixes, override.AllowProtectedSuffixes...)
	out.AllowProtectedNameMarkers = append(out.AllowProtectedNameMarkers, override.AllowProtectedNameMarkers...)
	out.Overrides = append([]ArgumentPolicyOverride(nil), override.Overrides...)
	return out
}

func normalizeArgumentPolicy(policy ArgumentPolicy) ArgumentPolicy {
	policy.ProtectedSegments = removeAllowedStrings(policy.ProtectedSegments, policy.AllowProtectedSegments)
	policy.ProtectedBasenames = removeAllowedStrings(policy.ProtectedBasenames, policy.AllowProtectedBasenames)
	policy.ProtectedSuffixes = removeAllowedStrings(policy.ProtectedSuffixes, policy.AllowProtectedSuffixes)
	policy.ProtectedNameMarkers = removeAllowedStrings(policy.ProtectedNameMarkers, policy.AllowProtectedNameMarkers)
	if policy.Tools == nil {
		policy.Tools = map[string][]ArgumentPathField{}
	}
	return policy
}

func argumentPolicyToolSpecs(policy ArgumentPolicy, toolName string) ([]filesystemPathSpec, bool) {
	fields, ok := policy.Tools[toolName]
	if !ok {
		return nil, false
	}
	specs := make([]filesystemPathSpec, 0, len(fields))
	for _, field := range fields {
		specs = append(specs, filesystemPathSpec{
			Field:    strings.TrimSpace(field.Field),
			Array:    specType(field) == "string_array",
			Required: argumentPathFieldRequired(field),
		})
	}
	return specs, true
}

func argumentPolicyBool(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func argumentPathFieldRequired(field ArgumentPathField) bool {
	if field.Required == nil {
		return true
	}
	return *field.Required
}

func specType(field ArgumentPathField) string {
	if strings.TrimSpace(field.Type) == "" {
		return "string"
	}
	return strings.TrimSpace(field.Type)
}

func argumentPolicyScopeMatches(scope ArgumentPolicyScope, ctx ArgumentPolicyContext) bool {
	if len(scope.VPSIDs) > 0 && !stringListContains(scope.VPSIDs, ctx.VPSID) {
		return false
	}
	if len(scope.Plans) > 0 && !stringListContains(scope.Plans, ctx.Plan) {
		return false
	}
	if len(scope.UserIDs) > 0 && !stringListContains(scope.UserIDs, ctx.UserID) {
		return false
	}
	return true
}

func cloneArgumentTools(values map[string][]ArgumentPathField) map[string][]ArgumentPathField {
	out := make(map[string][]ArgumentPathField, len(values))
	for key, specs := range values {
		out[key] = append([]ArgumentPathField(nil), specs...)
	}
	return out
}

func removeAllowedStrings(values []string, allowed []string) []string {
	allowedSet := stringSet(allowed)
	out := []string{}
	for _, value := range normalizeStrings(values) {
		if _, ok := allowedSet[value]; ok {
			continue
		}
		out = append(out, value)
	}
	return out
}

func stringSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range normalizeStrings(values) {
		out[value] = struct{}{}
	}
	return out
}

func normalizeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func stringListContains(values []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func protectedPathRule(kind string, value string) string {
	switch value {
	case ".stola":
		return "protected_stola_metadata"
	case ".git":
		return "protected_vcs_metadata"
	case ".config":
		return "protected_config_directory"
	case "opencode.json":
		return "protected_agent_config"
	case ".env", ".env.local", ".env.production":
		return "protected_env_file"
	case ".git-credentials":
		return "protected_credential_file"
	case ".gitconfig":
		return "protected_vcs_config"
	case ".netrc":
		return "protected_credential_file"
	case ".npmrc", ".pypirc":
		return "protected_package_manager_secret"
	case "authorized_keys", "id_dsa", "id_ecdsa", "id_ed25519", "id_rsa":
		return "protected_key_file"
	case "known_hosts":
		return "protected_ssh_file"
	default:
		if kind == "segment" && strings.HasPrefix(value, ".") {
			return "protected_secret_directory"
		}
		return "protected_workspace_" + kind
	}
}

func protectedSuffixRule(suffix string) string {
	switch suffix {
	case ".key", ".pem", ".p12", ".pfx":
		return "protected_key_file"
	case ".kubeconfig":
		return "protected_kube_config"
	default:
		return "protected_suffix_file"
	}
}

func boolPtr(value bool) *bool {
	return &value
}
