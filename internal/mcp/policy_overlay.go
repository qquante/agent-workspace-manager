package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type PolicyOverlay struct {
	Version         int                   `yaml:"version" json:"version"`
	ID              string                `yaml:"id" json:"id,omitempty"`
	ExecutionPolicy ExecutionPolicy       `yaml:"execution_policy" json:"execution_policy,omitempty"`
	Servers         []PolicyOverlayServer `yaml:"servers" json:"servers,omitempty"`
}

type PolicyOverlayServer struct {
	ID             string         `yaml:"id" json:"id"`
	ToolPolicy     ToolPolicy     `yaml:"tool_policy" json:"tool_policy,omitempty"`
	ArgumentPolicy ArgumentPolicy `yaml:"argument_policy" json:"argument_policy,omitempty"`
}

func ResolvePolicyPath(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	return strings.TrimSpace(os.Getenv("STOLA_MCP_POLICY"))
}

func LoadPolicyOverlayFile(filePath string) (PolicyOverlay, []byte, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return PolicyOverlay{}, nil, err
	}
	overlay, err := ParsePolicyOverlay(raw)
	if err != nil {
		return PolicyOverlay{}, nil, err
	}
	return overlay, raw, nil
}

func ParsePolicyOverlay(raw []byte) (PolicyOverlay, error) {
	var overlay PolicyOverlay
	if err := yaml.Unmarshal(raw, &overlay); err != nil {
		return PolicyOverlay{}, err
	}
	if err := overlay.Validate(); err != nil {
		return PolicyOverlay{}, err
	}
	return overlay, nil
}

func (o PolicyOverlay) Validate() error {
	var problems []string
	if o.Version <= 0 {
		problems = append(problems, "version must be positive")
	}
	problems = append(problems, validateExecutionPolicy("execution_policy", o.ExecutionPolicy)...)
	seen := map[string]bool{}
	for index, server := range o.Servers {
		location := fmt.Sprintf("servers[%d]", index)
		id := strings.TrimSpace(server.ID)
		if id == "" {
			problems = append(problems, location+".id is required")
			continue
		}
		if !serverIDPattern.MatchString(id) {
			problems = append(problems, location+".id must match ^[a-z][a-z0-9_.-]*$")
		}
		if seen[id] {
			problems = append(problems, fmt.Sprintf("duplicate server id %q", id))
		}
		seen[id] = true
		problems = append(problems, validateToolPolicy(location+".tool_policy", server.ToolPolicy)...)
		problems = append(problems, validateArgumentPolicy(location+".argument_policy", server.ArgumentPolicy)...)
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func ApplyPolicyOverlay(manifest Manifest, overlay PolicyOverlay) (Manifest, error) {
	if err := overlay.Validate(); err != nil {
		return Manifest{}, err
	}
	if !executionPolicyEmpty(overlay.ExecutionPolicy) {
		manifest.ExecutionPolicy = mergeExecutionPolicy(manifest.ExecutionPolicy, overlay.ExecutionPolicy)
	}
	for _, override := range overlay.Servers {
		if !applyPolicyOverlayToServers(manifest.Servers, override) {
			if !applyPolicyOverlayToServers(manifest.OptionalServers, override) {
				return Manifest{}, fmt.Errorf("policy overlay references unknown server %q", override.ID)
			}
		}
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func LoadEffectiveFile(manifestPath string, policyFile string) (Manifest, []byte, string, error) {
	manifest, manifestRaw, err := LoadFile(manifestPath)
	if err != nil {
		return Manifest{}, nil, "", err
	}
	resolvedPolicyPath := ResolvePolicyPath(policyFile)
	if resolvedPolicyPath == "" {
		return manifest, manifestRaw, PolicyVersion(manifestRaw, nil), nil
	}
	overlay, policyRaw, err := LoadPolicyOverlayFile(resolvedPolicyPath)
	if err != nil {
		return Manifest{}, nil, "", err
	}
	manifest, err = ApplyPolicyOverlay(manifest, overlay)
	if err != nil {
		return Manifest{}, nil, "", err
	}
	effectiveRaw, err := yaml.Marshal(manifest)
	if err != nil {
		return Manifest{}, nil, "", err
	}
	return manifest, effectiveRaw, PolicyVersion(manifestRaw, policyRaw), nil
}

func LoadEffectiveAndRender(manifestPath string, policyFile string) (Bundle, string, error) {
	manifest, raw, version, err := LoadEffectiveFile(manifestPath, policyFile)
	if err != nil {
		return Bundle{}, "", err
	}
	bundle, err := RenderBundle(manifest, raw)
	if err != nil {
		return Bundle{}, "", err
	}
	return bundle, version, nil
}

func PolicyVersion(manifestRaw []byte, policyRaw []byte) string {
	h := sha256.New()
	_, _ = h.Write(manifestRaw)
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(policyRaw)
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:8])
}

func applyPolicyOverlayToServers(servers []Server, override PolicyOverlayServer) bool {
	for index := range servers {
		if servers[index].ID != override.ID {
			continue
		}
		servers[index].ToolPolicy = mergeToolPolicy(servers[index].ToolPolicy, override.ToolPolicy)
		servers[index].ArgumentPolicy = mergeArgumentPolicy(servers[index].ArgumentPolicy, override.ArgumentPolicy)
		return true
	}
	return false
}

func mergeToolPolicy(base ToolPolicy, override ToolPolicy) ToolPolicy {
	out := base
	if override.DefaultAction != "" {
		out.DefaultAction = override.DefaultAction
	}
	if len(override.Rules) > 0 {
		out.Rules = append([]ToolRule(nil), override.Rules...)
	}
	return out
}

func executionPolicyEmpty(policy ExecutionPolicy) bool {
	return policy.Enabled == nil &&
		policy.DefaultAction == "" &&
		len(policy.Risk.RequireApproval) == 0 &&
		len(policy.Risk.Deny) == 0 &&
		len(policy.Network.Allowed) == 0 &&
		len(policy.Network.Denied) == 0 &&
		len(policy.Commands.AllowedBins) == 0 &&
		len(policy.Commands.DeniedBins) == 0 &&
		policy.RateLimit.MaxCalls == 0 &&
		policy.RateLimit.Window == "" &&
		len(policy.Rules) == 0
}
