package mcp

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ExecutionPolicy struct {
	Enabled       *bool           `yaml:"enabled" json:"enabled,omitempty"`
	DefaultAction string          `yaml:"default_action" json:"default_action,omitempty"`
	Risk          RiskPolicy      `yaml:"risk" json:"risk,omitempty"`
	Network       NetworkPolicy   `yaml:"network" json:"network,omitempty"`
	Commands      CommandPolicy   `yaml:"commands" json:"commands,omitempty"`
	RateLimit     RateLimitPolicy `yaml:"rate_limit" json:"rate_limit,omitempty"`
	Rules         []ExecutionRule `yaml:"rules" json:"rules,omitempty"`
}

type RiskPolicy struct {
	RequireApproval []string `yaml:"require_approval" json:"require_approval,omitempty"`
	Deny            []string `yaml:"deny" json:"deny,omitempty"`
}

type NetworkPolicy struct {
	Allowed []string `yaml:"allowed" json:"allowed,omitempty"`
	Denied  []string `yaml:"denied" json:"denied,omitempty"`
}

type CommandPolicy struct {
	AllowedBins []string `yaml:"allowed_bins" json:"allowed_bins,omitempty"`
	DeniedBins  []string `yaml:"denied_bins" json:"denied_bins,omitempty"`
}

type RateLimitPolicy struct {
	MaxCalls int    `yaml:"max_calls" json:"max_calls,omitempty"`
	Window   string `yaml:"window" json:"window,omitempty"`
}

type ExecutionRule struct {
	ID        string             `yaml:"id" json:"id"`
	Action    string             `yaml:"action" json:"action"`
	Reason    string             `yaml:"reason" json:"reason,omitempty"`
	Match     ExecutionRuleMatch `yaml:"match" json:"match,omitempty"`
	RateLimit RateLimitPolicy    `yaml:"rate_limit" json:"rate_limit,omitempty"`
}

type ExecutionRuleMatch struct {
	Servers     []string `yaml:"servers" json:"servers,omitempty"`
	Tools       []string `yaml:"tools" json:"tools,omitempty"`
	Risks       []string `yaml:"risks" json:"risks,omitempty"`
	Networks    []string `yaml:"networks" json:"networks,omitempty"`
	CommandBins []string `yaml:"command_bins" json:"command_bins,omitempty"`
	VPSIDs      []string `yaml:"vps_ids" json:"vps_ids,omitempty"`
	Plans       []string `yaml:"plans" json:"plans,omitempty"`
	UserIDs     []string `yaml:"user_ids" json:"user_ids,omitempty"`
}

type ExecutionPolicyContext struct {
	VPSID  string
	Plan   string
	UserID string
}

type ExecutionDecision struct {
	Action       string
	Reason       string
	Rule         string
	MatchedRules []string
	Risk         string
	Network      string
	CommandBin   string
	RateLimit    *ExecutionRateLimit
}

type ExecutionRateLimit struct {
	Key      string
	Rule     string
	MaxCalls int
	Window   time.Duration
}

func DefaultExecutionPolicy() ExecutionPolicy {
	return ExecutionPolicy{
		Enabled:       boolPtr(true),
		DefaultAction: ToolActionAllow,
		Risk: RiskPolicy{
			RequireApproval: []string{"high"},
		},
		Network: NetworkPolicy{
			Allowed: []string{"none", "https_only"},
			Denied:  []string{"raw", "raw_network", "unrestricted", "all"},
		},
		Commands: CommandPolicy{
			AllowedBins: []string{"npx", "uvx"},
			DeniedBins:  []string{"docker", "podman", "kubectl", "pveum", "ssh", "powershell", "pwsh", "bash", "sh"},
		},
		RateLimit: RateLimitPolicy{
			MaxCalls: 120,
			Window:   "1m",
		},
	}
}

func EvaluateExecutionPolicy(manifest Manifest, server Server, toolName string, ctx ExecutionPolicyContext) (ExecutionDecision, error) {
	policy := mergeExecutionPolicy(DefaultExecutionPolicy(), manifest.ExecutionPolicy)
	decision := ExecutionDecision{
		Action:     executionPolicyAction(policy.DefaultAction, ToolActionAllow),
		Reason:     "execution policy default",
		Rule:       "execution_policy:default",
		Risk:       normalizeRisk(server.Risk),
		Network:    serverNetwork(server),
		CommandBin: commandBin(server.Command),
	}
	if !argumentPolicyBool(policy.Enabled, true) {
		decision.Action = ToolActionAllow
		decision.Reason = "execution policy disabled"
		decision.Rule = "execution_policy:disabled"
		return decision, nil
	}
	if err := applyRateLimitPolicy(&decision, policy.RateLimit, "execution_policy:rate_limit"); err != nil {
		return ExecutionDecision{}, err
	}

	if stringListContainsNormalized(policy.Risk.Deny, decision.Risk) {
		decision.apply(ToolActionDeny, "server risk is denied by execution policy", "execution_policy:risk_deny")
	}
	if stringListContainsNormalized(policy.Risk.RequireApproval, decision.Risk) {
		decision.apply(ToolActionRequireApproval, "server risk requires approval", "execution_policy:risk_approval")
	}
	if stringListContainsNormalized(policy.Network.Denied, decision.Network) {
		decision.apply(ToolActionDeny, "server network access is denied by execution policy", "execution_policy:network_deny")
	}
	if len(policy.Network.Allowed) > 0 && !stringListContainsNormalized(policy.Network.Allowed, decision.Network) {
		decision.apply(ToolActionDeny, "server network access is not allowed by execution policy", "execution_policy:network_allowed")
	}
	if stringListContainsNormalized(policy.Commands.DeniedBins, decision.CommandBin) {
		decision.apply(ToolActionDeny, "server command is denied by execution policy", "execution_policy:command_deny")
	}
	if len(policy.Commands.AllowedBins) > 0 && !stringListContainsNormalized(policy.Commands.AllowedBins, decision.CommandBin) {
		decision.apply(ToolActionDeny, "server command is not allowed by execution policy", "execution_policy:command_allowed")
	}

	toolDecision := server.ToolDecision(toolName)
	if toolDecision.Action != "" && !(toolDecision.Action == ToolActionAllow && toolDecision.Rule == "default") {
		decision.apply(toolDecision.Action, toolDecision.Reason, "tool_policy:"+toolDecision.Rule)
	}

	for _, rule := range policy.Rules {
		if !executionRuleMatches(rule.Match, server, toolName, decision, ctx) {
			continue
		}
		ruleID := strings.TrimSpace(rule.ID)
		if ruleID == "" {
			ruleID = "unnamed"
		}
		decision.MatchedRules = append(decision.MatchedRules, ruleID)
		if err := applyRateLimitPolicy(&decision, rule.RateLimit, "execution_policy:"+ruleID+":rate_limit"); err != nil {
			return ExecutionDecision{}, err
		}
		reason := strings.TrimSpace(rule.Reason)
		if reason == "" {
			reason = "matched execution policy rule"
		}
		decision.apply(rule.Action, reason, "execution_policy:"+ruleID)
	}
	if decision.Action == "" {
		decision.Action = ToolActionAllow
	}
	sort.Strings(decision.MatchedRules)
	if decision.RateLimit != nil {
		decision.RateLimit.Key = RateLimitKey(ctx, server.ID, toolName, decision.RateLimit.Rule)
	}
	return decision, nil
}

func (d *ExecutionDecision) apply(action string, reason string, rule string) {
	action = executionPolicyAction(action, "")
	if action == "" {
		return
	}
	if d.Action == ToolActionDeny {
		return
	}
	if action == ToolActionAllow && d.Action == ToolActionRequireApproval {
		return
	}
	if action == ToolActionDeny || action == ToolActionRequireApproval || d.Action == "" {
		d.Action = action
		if strings.TrimSpace(reason) != "" {
			d.Reason = reason
		}
		if strings.TrimSpace(rule) != "" {
			d.Rule = rule
		}
	}
}

func validateExecutionPolicy(location string, policy ExecutionPolicy) []string {
	var problems []string
	if policy.DefaultAction != "" && !validToolAction(policy.DefaultAction) {
		problems = append(problems, location+".default_action must be allow, require_approval, or deny")
	}
	problems = append(problems, validateRateLimitPolicy(location+".rate_limit", policy.RateLimit)...)
	seen := map[string]bool{}
	for index, rule := range policy.Rules {
		ruleLocation := fmt.Sprintf("%s.rules[%d]", location, index)
		id := strings.TrimSpace(rule.ID)
		if id == "" {
			problems = append(problems, ruleLocation+".id is required")
		}
		if seen[id] {
			problems = append(problems, fmt.Sprintf("%s duplicate rule id %q", location, id))
		}
		seen[id] = true
		if !validToolAction(rule.Action) {
			problems = append(problems, ruleLocation+".action must be allow, require_approval, or deny")
		}
		if executionRuleMatchEmpty(rule.Match) {
			problems = append(problems, ruleLocation+".match must not be empty")
		}
		problems = append(problems, validateRateLimitPolicy(ruleLocation+".rate_limit", rule.RateLimit)...)
	}
	return problems
}

func validateRateLimitPolicy(location string, policy RateLimitPolicy) []string {
	var problems []string
	if policy.MaxCalls < 0 {
		problems = append(problems, location+".max_calls must be non-negative")
	}
	if policy.MaxCalls > 0 {
		if strings.TrimSpace(policy.Window) == "" {
			problems = append(problems, location+".window is required when max_calls is set")
		} else if _, err := time.ParseDuration(policy.Window); err != nil {
			problems = append(problems, location+".window must be a Go duration")
		}
	}
	return problems
}

func mergeExecutionPolicy(base ExecutionPolicy, override ExecutionPolicy) ExecutionPolicy {
	out := base
	if override.Enabled != nil {
		out.Enabled = override.Enabled
	}
	if override.DefaultAction != "" {
		out.DefaultAction = override.DefaultAction
	}
	if len(override.Risk.RequireApproval) > 0 {
		out.Risk.RequireApproval = append([]string(nil), override.Risk.RequireApproval...)
	}
	if len(override.Risk.Deny) > 0 {
		out.Risk.Deny = append([]string(nil), override.Risk.Deny...)
	}
	if len(override.Network.Allowed) > 0 {
		out.Network.Allowed = append([]string(nil), override.Network.Allowed...)
	}
	if len(override.Network.Denied) > 0 {
		out.Network.Denied = append([]string(nil), override.Network.Denied...)
	}
	if len(override.Commands.AllowedBins) > 0 {
		out.Commands.AllowedBins = append([]string(nil), override.Commands.AllowedBins...)
	}
	if len(override.Commands.DeniedBins) > 0 {
		out.Commands.DeniedBins = append([]string(nil), override.Commands.DeniedBins...)
	}
	if override.RateLimit.MaxCalls > 0 || override.RateLimit.Window != "" {
		out.RateLimit = override.RateLimit
	}
	if len(override.Rules) > 0 {
		out.Rules = append([]ExecutionRule(nil), override.Rules...)
	}
	return out
}

func applyRateLimitPolicy(decision *ExecutionDecision, policy RateLimitPolicy, rule string) error {
	if policy.MaxCalls <= 0 {
		return nil
	}
	window, err := time.ParseDuration(policy.Window)
	if err != nil {
		return err
	}
	decision.RateLimit = &ExecutionRateLimit{
		Rule:     rule,
		MaxCalls: policy.MaxCalls,
		Window:   window,
	}
	return nil
}

func executionRuleMatches(match ExecutionRuleMatch, server Server, toolName string, decision ExecutionDecision, ctx ExecutionPolicyContext) bool {
	if len(match.Servers) > 0 && !stringListContains(match.Servers, server.ID) {
		return false
	}
	if len(match.Tools) > 0 && !stringListContains(match.Tools, toolName) {
		return false
	}
	if len(match.Risks) > 0 && !stringListContainsNormalized(match.Risks, decision.Risk) {
		return false
	}
	if len(match.Networks) > 0 && !stringListContainsNormalized(match.Networks, decision.Network) {
		return false
	}
	if len(match.CommandBins) > 0 && !stringListContainsNormalized(match.CommandBins, decision.CommandBin) {
		return false
	}
	if len(match.VPSIDs) > 0 && !stringListContains(match.VPSIDs, ctx.VPSID) {
		return false
	}
	if len(match.Plans) > 0 && !stringListContains(match.Plans, ctx.Plan) {
		return false
	}
	if len(match.UserIDs) > 0 && !stringListContains(match.UserIDs, ctx.UserID) {
		return false
	}
	return true
}

func executionRuleMatchEmpty(match ExecutionRuleMatch) bool {
	return len(match.Servers) == 0 &&
		len(match.Tools) == 0 &&
		len(match.Risks) == 0 &&
		len(match.Networks) == 0 &&
		len(match.CommandBins) == 0 &&
		len(match.VPSIDs) == 0 &&
		len(match.Plans) == 0 &&
		len(match.UserIDs) == 0
}

func executionPolicyAction(action string, fallback string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		return fallback
	}
	return action
}

func normalizeRisk(risk string) string {
	risk = strings.ToLower(strings.TrimSpace(risk))
	if risk == "" {
		return "low"
	}
	return risk
}

func serverNetwork(server Server) string {
	raw, ok := server.Permissions["network"]
	if !ok || raw == nil {
		return "none"
	}
	switch value := raw.(type) {
	case string:
		if strings.TrimSpace(value) != "" {
			return strings.ToLower(strings.TrimSpace(value))
		}
	case fmt.Stringer:
		return strings.ToLower(strings.TrimSpace(value.String()))
	}
	return "none"
}

func commandBin(command []string) string {
	if len(command) == 0 {
		return ""
	}
	first := strings.TrimSpace(command[0])
	if first == "" {
		return ""
	}
	base := filepath.Base(strings.ReplaceAll(first, "\\", "/"))
	base = strings.TrimSuffix(base, ".exe")
	return strings.ToLower(base)
}

func stringListContainsNormalized(values []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == want {
			return true
		}
	}
	return false
}

func RateLimitKey(ctx ExecutionPolicyContext, serverID string, toolName string, rule string) string {
	parts := []string{
		"mcp",
		"vps=" + strings.TrimSpace(ctx.VPSID),
		"plan=" + strings.TrimSpace(ctx.Plan),
		"user=" + strings.TrimSpace(ctx.UserID),
		"server=" + strings.TrimSpace(serverID),
		"tool=" + strings.TrimSpace(toolName),
		"rule=" + strings.TrimSpace(rule),
	}
	return strings.Join(parts, "|")
}
