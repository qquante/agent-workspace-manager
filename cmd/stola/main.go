package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"stola-os/internal/audit"
	"stola-os/internal/config"
	"stola-os/internal/controlplane"
	dockerbackend "stola-os/internal/docker"
	"stola-os/internal/llmproxy"
	"stola-os/internal/mcp"
	"stola-os/internal/providers"
	"stola-os/internal/provisioner"
	"stola-os/internal/proxmox"
	"stola-os/internal/secrets"
	"stola-os/internal/state"
	"stola-os/internal/version"
)

func main() {
	code := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	configPath, args, err := extractConfigPath(args, os.Getenv("STOLA_CONFIG"))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}

	switch args[0] {
	case "doctor":
		return cmdDoctor(configPath, stdout, stderr)
	case "init":
		return cmdInit(configPath, args[1:], stdout, stderr)
	case "config":
		return cmdConfig(configPath, args[1:], stdout, stderr)
	case "node":
		return cmdNode(configPath, args[1:], stdout, stderr)
	case "provisioner":
		return cmdProvisioner(ctx, configPath, args[1:], stdout, stderr)
	case "proxmox":
		return cmdProxmox(ctx, configPath, args[1:], stdout, stderr)
	case "docker":
		return cmdDocker(ctx, configPath, args[1:], stdout, stderr)
	case "mcp":
		return cmdMCP(ctx, configPath, args[1:], stdout, stderr)
	case "providers":
		return cmdProviders(ctx, args[1:], stdout, stderr)
	case "secrets":
		return cmdSecrets(args[1:], stdout, stderr)
	case "approvals":
		return cmdApprovals(ctx, configPath, args[1:], stdout, stderr)
	case "vps":
		return cmdVPS(ctx, configPath, args[1:], stdout, stderr, "vps")
	case "workspace":
		return cmdVPS(ctx, configPath, args[1:], stdout, stderr, "workspace")
	case "service":
		return cmdService(args[1:], stdout, stderr)
	case "daemon":
		return cmdDaemon(ctx, configPath, args[1:], stdout, stderr)
	case "version":
		fmt.Fprintln(stdout, version.Version)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func extractConfigPath(args []string, envValue string) (string, []string, error) {
	configPath := envValue
	if configPath == "" {
		configPath = config.DefaultConfigPath
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--config" {
			if i+1 >= len(args) {
				return "", nil, errors.New("--config requires a path")
			}
			configPath = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "--config=") {
			configPath = strings.TrimPrefix(arg, "--config=")
			continue
		}
		out = append(out, arg)
	}
	return configPath, out, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `stola Docker-only agent workspace core

Usage:
  stola [--config PATH] doctor
  stola [--config PATH] init [--force] [--node-id ID] [--state PATH] [--audit-log PATH]
  stola [--config PATH] config validate
  stola [--config PATH] node status
  stola [--config PATH] provisioner mock-create [--name NAME] [--plan PLAN]
  stola [--config PATH] docker check
  stola [--config PATH] mcp list|validate|render|tools|call [--manifest PATH] [--policy-file PATH]
  stola providers list|validate|render|check [--manifest PATH]
  stola secrets validate|list --file PATH [--providers-manifest PATH]
  stola [--config PATH] approvals list|approve [--api URL]
  stola [--config PATH] proxmox check|nodes|storage [--node NODE]
  stola [--config PATH] workspace create --name NAME [--image IMAGE] [--memory 1024] [--cores 1] [--mcp-manifest PATH] [--providers-manifest PATH] [--llm-proxy-url URL]
  stola [--config PATH] workspace list
  stola [--config PATH] workspace status|logs|stop|start|delete --id ID
  stola [--config PATH] workspace exec --id ID -- COMMAND [ARG...]
  stola [--config PATH] vps ...   legacy alias for workspace
  stola service status
  stola [--config PATH] daemon api|llm-proxy|provisioner|auditd [--once]
  stola version`)
}

func cmdProviders(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: stola providers list|validate|render|check [--manifest PATH]")
		return 2
	}
	switch args[0] {
	case "validate":
		fs := flag.NewFlagSet("providers validate", flag.ContinueOnError)
		fs.SetOutput(stderr)
		manifestPath := fs.String("manifest", "", "provider manifest path")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		path := providers.ResolveManifestPath(*manifestPath)
		manifest, _, err := providers.LoadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "provider manifest invalid: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "provider manifest valid: %s name=%s providers=%d default=%s\n", path, manifest.Name, len(manifest.Providers), manifest.DefaultProvider)
		return 0
	case "list":
		fs := flag.NewFlagSet("providers list", flag.ContinueOnError)
		fs.SetOutput(stderr)
		manifestPath := fs.String("manifest", "", "provider manifest path")
		includeDisabled := fs.Bool("include-disabled", true, "include disabled providers")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		path := providers.ResolveManifestPath(*manifestPath)
		manifest, _, err := providers.LoadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "provider manifest invalid: %v\n", err)
			return 1
		}
		items := append([]providers.Provider(nil), manifest.Providers...)
		sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		for _, provider := range items {
			if !provider.Enabled && !*includeDisabled {
				continue
			}
			fmt.Fprintf(stdout, "id=%s enabled=%t kind=%s", provider.ID, provider.Enabled, provider.Kind)
			if provider.APIKeyEnv != "" {
				fmt.Fprintf(stdout, " api_key_env=%s", provider.APIKeyEnv)
			}
			if len(provider.ExtraEnv) > 0 {
				extra := append([]string(nil), provider.ExtraEnv...)
				sort.Strings(extra)
				fmt.Fprintf(stdout, " extra_env=%s", strings.Join(extra, ","))
			}
			if provider.BaseURL != "" {
				fmt.Fprintf(stdout, " base_url=%s", provider.BaseURL)
			}
			if provider.LiteLLMModel != "" {
				fmt.Fprintf(stdout, " litellm_model=%s", provider.LiteLLMModel)
			}
			fmt.Fprintln(stdout)
		}
		return 0
	case "render":
		fs := flag.NewFlagSet("providers render", flag.ContinueOnError)
		fs.SetOutput(stderr)
		manifestPath := fs.String("manifest", "", "provider manifest path")
		workspace := fs.String("workspace", "", "optional host workspace directory to write generated config")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		path := providers.ResolveManifestPath(*manifestPath)
		bundle, err := providers.LoadAndRender(path)
		if err != nil {
			fmt.Fprintf(stderr, "provider render failed: %v\n", err)
			return 1
		}
		if *workspace != "" {
			if err := providers.WriteBundleToDir(bundle, *workspace); err != nil {
				fmt.Fprintf(stderr, "provider render write failed: %v\n", err)
				return 1
			}
			fmt.Fprintf(stdout, "provider config written: %s\n", *workspace)
		}
		names := make([]string, 0, len(bundle.Files))
		for name := range bundle.Files {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(stdout, "%s bytes=%d\n", name, len(bundle.Files[name]))
		}
		return 0
	case "check":
		fs := flag.NewFlagSet("providers check", flag.ContinueOnError)
		fs.SetOutput(stderr)
		manifestPath := fs.String("manifest", "", "provider manifest path")
		providerID := fs.String("provider", "", "provider ID to check")
		secretsFile := fs.String("secrets-file", "", "provider secret env file path")
		timeoutValue := fs.Duration("timeout", providers.DefaultCheckTimeout, "HTTP timeout")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if strings.TrimSpace(*providerID) == "" {
			fmt.Fprintln(stderr, "--provider is required")
			return 2
		}
		if strings.TrimSpace(*secretsFile) == "" {
			fmt.Fprintln(stderr, "--secrets-file is required")
			return 2
		}
		path := providers.ResolveManifestPath(*manifestPath)
		manifest, _, err := providers.LoadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "provider manifest invalid: %v\n", err)
			return 1
		}
		secretBundle, err := secrets.LoadProviderEnvFile(*secretsFile, providers.EnvNames(manifest))
		if err != nil {
			fmt.Fprintf(stderr, "secret file invalid: %v\n", err)
			return 1
		}
		result, err := providers.CheckProvider(ctx, manifest, *providerID, secretBundle, providers.CheckOptions{Timeout: *timeoutValue})
		if err != nil {
			if result.ProviderID != "" {
				fmt.Fprintf(stderr, "provider check failed: id=%s status=%d endpoint=%s duration_ms=%d error=%v\n", result.ProviderID, result.StatusCode, result.Endpoint, result.Duration.Milliseconds(), err)
			} else {
				fmt.Fprintf(stderr, "provider check failed: %v\n", err)
			}
			return 1
		}
		fmt.Fprintf(stdout, "provider check ok: id=%s kind=%s status=%d models=%d endpoint=%s duration_ms=%d secrets=%s\n", result.ProviderID, result.Kind, result.StatusCode, result.ModelCount, result.Endpoint, result.Duration.Milliseconds(), strings.Join(result.UsedSecretNames, ","))
		return 0
	default:
		fmt.Fprintf(stderr, "unknown providers command: %s\n", args[0])
		return 2
	}
}

func cmdSecrets(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: stola secrets validate|list --file PATH [--providers-manifest PATH]")
		return 2
	}
	switch args[0] {
	case "validate", "list":
		fs := flag.NewFlagSet("secrets "+args[0], flag.ContinueOnError)
		fs.SetOutput(stderr)
		filePath := fs.String("file", "", "provider secret env file path")
		providersManifest := fs.String("providers-manifest", "", "provider manifest path used to allow env names")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if strings.TrimSpace(*filePath) == "" {
			fmt.Fprintln(stderr, "--file is required")
			return 2
		}
		bundle, manifestPath, err := loadProviderSecrets(*filePath, *providersManifest)
		if err != nil {
			fmt.Fprintf(stderr, "secret file invalid: %v\n", err)
			return 1
		}
		names := secrets.Names(bundle.Secrets)
		if args[0] == "validate" {
			fmt.Fprintf(stdout, "secret file valid: %s names=%d providers_manifest=%s\n", *filePath, len(names), manifestPath)
			return 0
		}
		for _, name := range names {
			fmt.Fprintf(stdout, "name=%s\n", name)
		}
		return 0
	default:
		fmt.Fprintf(stderr, "unknown secrets command: %s\n", args[0])
		return 2
	}
}

func loadProviderSecrets(secretsFile string, providersManifest string) (secrets.Bundle, string, error) {
	manifestPath := providers.ResolveManifestPath(providersManifest)
	manifest, _, err := providers.LoadFile(manifestPath)
	if err != nil {
		return secrets.Bundle{}, manifestPath, err
	}
	bundle, err := secrets.LoadProviderEnvFile(secretsFile, providers.EnvNames(manifest))
	if err != nil {
		return secrets.Bundle{}, manifestPath, err
	}
	return bundle, manifestPath, nil
}

func cmdMCP(ctx context.Context, configPath string, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: stola mcp list|validate|render|tools|call [--manifest PATH]")
		return 2
	}
	switch args[0] {
	case "validate":
		fs := flag.NewFlagSet("mcp validate", flag.ContinueOnError)
		fs.SetOutput(stderr)
		manifestPath := fs.String("manifest", "", "MCP manifest path")
		policyFile := fs.String("policy-file", "", "MCP policy overlay path")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		path := mcp.ResolveManifestPath(*manifestPath)
		manifest, _, policyVersion, err := mcp.LoadEffectiveFile(path, *policyFile)
		if err != nil {
			fmt.Fprintf(stderr, "mcp manifest invalid: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "mcp manifest valid: %s name=%s servers=%d optional=%d policy_version=%s\n", path, manifest.Name, len(manifest.Servers), len(manifest.OptionalServers), policyVersion)
		return 0
	case "list":
		fs := flag.NewFlagSet("mcp list", flag.ContinueOnError)
		fs.SetOutput(stderr)
		manifestPath := fs.String("manifest", "", "MCP manifest path")
		policyFile := fs.String("policy-file", "", "MCP policy overlay path")
		includeOptional := fs.Bool("include-optional", true, "include optional disabled MCP servers")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		path := mcp.ResolveManifestPath(*manifestPath)
		manifest, _, _, err := mcp.LoadEffectiveFile(path, *policyFile)
		if err != nil {
			fmt.Fprintf(stderr, "mcp manifest invalid: %v\n", err)
			return 1
		}
		for _, server := range manifest.ServerViews(*includeOptional) {
			fmt.Fprintf(stdout, "id=%s enabled=%t optional=%t risk=%s transport=%s", server.ID, server.Enabled, server.Optional, server.Risk, server.Transport)
			if len(server.Command) > 0 {
				fmt.Fprintf(stdout, " command=%q", strings.Join(server.Command, " "))
			}
			if server.URL != "" {
				fmt.Fprintf(stdout, " url=%s", server.URL)
			}
			if secrets := server.SecretNames(); len(secrets) > 0 {
				fmt.Fprintf(stdout, " secrets=%s", strings.Join(secrets, ","))
			}
			fmt.Fprintln(stdout)
		}
		return 0
	case "render":
		fs := flag.NewFlagSet("mcp render", flag.ContinueOnError)
		fs.SetOutput(stderr)
		manifestPath := fs.String("manifest", "", "MCP manifest path")
		policyFile := fs.String("policy-file", "", "MCP policy overlay path")
		workspace := fs.String("workspace", "", "optional host workspace directory to write generated config")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		path := mcp.ResolveManifestPath(*manifestPath)
		bundle, policyVersion, err := mcp.LoadEffectiveAndRender(path, *policyFile)
		if err != nil {
			fmt.Fprintf(stderr, "mcp render failed: %v\n", err)
			return 1
		}
		if *workspace != "" {
			if err := mcp.WriteBundleToDir(bundle, *workspace); err != nil {
				fmt.Fprintf(stderr, "mcp render write failed: %v\n", err)
				return 1
			}
			fmt.Fprintf(stdout, "mcp config written: %s\n", *workspace)
		}
		names := make([]string, 0, len(bundle.Files))
		for name := range bundle.Files {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(stdout, "%s bytes=%d\n", name, len(bundle.Files[name]))
		}
		fmt.Fprintf(stdout, "policy_version %s\n", policyVersion)
		return 0
	case "tools":
		return cmdMCPTools(ctx, configPath, args[1:], stdout, stderr)
	case "call":
		return cmdMCPCall(ctx, configPath, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown mcp command: %s\n", args[0])
		return 2
	}
}

func cmdMCPTools(ctx context.Context, configPath string, args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("mcp tools", flag.ContinueOnError)
	fs.SetOutput(stderr)
	manifestPath := fs.String("manifest", "", "MCP manifest path")
	policyFile := fs.String("policy-file", "", "MCP policy overlay path")
	serverID := fs.String("server", "", "MCP server id")
	id := fs.String("id", "", "VPS id")
	name := fs.String("name", "", "VPS name")
	timeoutText := fs.String("timeout", "45s", "MCP request timeout")
	jsonOutput := fs.Bool("json", false, "print raw JSON result")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	timeout, err := time.ParseDuration(*timeoutText)
	if err != nil {
		fmt.Fprintf(stderr, "invalid timeout: %v\n", err)
		return 2
	}
	runtime, ok := openMCPRuntime(ctx, configPath, *manifestPath, *policyFile, *serverID, *id, *name, stderr)
	if !ok {
		return 1
	}
	defer runtime.cleanup()
	started := time.Now()
	result, err := runtime.client.MCPListTools(ctx, runtime.vps, runtime.server, timeout)
	duration := time.Since(started)
	if err != nil {
		recordMCPAudit(runtime.logger, "mcp.tools", runtime.vps.ID, "failure", map[string]any{
			"server":      runtime.server.ID,
			"duration_ms": duration.Milliseconds(),
			"error":       err.Error(),
		})
		fmt.Fprintf(stderr, "mcp tools failed: %v\n", err)
		return 1
	}
	recordMCPAudit(runtime.logger, "mcp.tools", runtime.vps.ID, "success", map[string]any{
		"server":      runtime.server.ID,
		"duration_ms": duration.Milliseconds(),
		"tool_count":  len(result.Tools),
		"tools":       mcp.ToolNames(result.Tools),
	})
	if *jsonOutput {
		printJSON(stdout, result)
		return 0
	}
	for _, tool := range result.Tools {
		fmt.Fprintf(stdout, "tool=%s", tool.Name)
		if tool.Description != "" {
			fmt.Fprintf(stdout, " description=%q", oneLine(tool.Description))
		}
		fmt.Fprintln(stdout)
	}
	return 0
}

func cmdMCPCall(ctx context.Context, configPath string, args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("mcp call", flag.ContinueOnError)
	fs.SetOutput(stderr)
	manifestPath := fs.String("manifest", "", "MCP manifest path")
	policyFile := fs.String("policy-file", "", "MCP policy overlay path")
	serverID := fs.String("server", "", "MCP server id")
	toolName := fs.String("tool", "", "MCP tool name")
	inputText := fs.String("input", "{}", "tool arguments JSON or @path")
	approvalID := fs.String("approval-id", "", "one-shot approval id for require_approval MCP tool calls")
	approvalToken := fs.String("approval-token", "", "signed one-shot approval token from the control-plane API")
	approvalTTLText := fs.String("approval-ttl", "10m", "approval intent TTL when one is created")
	policyUserID := fs.String("policy-user-id", os.Getenv("STOLA_POLICY_USER_ID"), "policy user id for scoped MCP policy")
	id := fs.String("id", "", "VPS id")
	name := fs.String("name", "", "VPS name")
	timeoutText := fs.String("timeout", "45s", "MCP request timeout")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*toolName) == "" {
		fmt.Fprintln(stderr, "--tool is required")
		return 2
	}
	timeout, err := time.ParseDuration(*timeoutText)
	if err != nil {
		fmt.Fprintf(stderr, "invalid timeout: %v\n", err)
		return 2
	}
	approvalTTL, err := time.ParseDuration(*approvalTTLText)
	if err != nil {
		fmt.Fprintf(stderr, "invalid approval ttl: %v\n", err)
		return 2
	}
	if approvalTTL <= 0 {
		fmt.Fprintln(stderr, "invalid approval ttl: must be greater than zero")
		return 2
	}
	input, err := readJSONInput(*inputText)
	if err != nil {
		fmt.Fprintf(stderr, "invalid input: %v\n", err)
		return 2
	}
	runtime, ok := openMCPRuntime(ctx, configPath, *manifestPath, *policyFile, *serverID, *id, *name, stderr)
	if !ok {
		return 1
	}
	defer runtime.cleanup()

	argumentSummary := mcp.ArgumentSummary(input)
	argumentDigest := mcp.ArgumentDigest(input)
	approvalTokenValue, err := readSecretValue(*approvalToken)
	if err != nil {
		fmt.Fprintf(stderr, "approval token read failed: %v\n", err)
		return 2
	}
	approvalIDValue := strings.TrimSpace(*approvalID)
	var signedApproval *controlplane.ApprovalPayload
	policyUserIDValue := strings.TrimSpace(*policyUserID)
	if approvalTokenValue != "" {
		key, err := controlplane.LoadSigningKey(runtime.cfg.API.SigningKeyFile)
		if err != nil {
			fmt.Fprintf(stderr, "approval token verification failed: %v\n", err)
			return 1
		}
		payload, err := controlplane.VerifyApprovalToken(key, approvalTokenValue, time.Now().UTC())
		if err != nil {
			fmt.Fprintf(stderr, "approval token rejected: %v\n", err)
			return 1
		}
		if payload.VPSID != runtime.vps.ID || payload.Server != runtime.server.ID || payload.Tool != *toolName || payload.ArgumentDigest != argumentDigest {
			fmt.Fprintln(stderr, "approval token rejected: token does not match this MCP request")
			return 1
		}
		if payload.PolicyVersion != runtime.policyVersion {
			fmt.Fprintf(stderr, "approval token rejected: policy version mismatch token=%s current=%s\n", payload.PolicyVersion, runtime.policyVersion)
			return 1
		}
		if approvalIDValue != "" && approvalIDValue != payload.ApprovalID {
			fmt.Fprintln(stderr, "approval token rejected: --approval-id does not match token approval_id")
			return 1
		}
		approvalIDValue = payload.ApprovalID
		if policyUserIDValue == "" {
			policyUserIDValue = payload.UserID
		} else if policyUserIDValue != payload.UserID {
			fmt.Fprintln(stderr, "approval token rejected: --policy-user-id does not match token user_id")
			return 1
		}
		signedApproval = &payload
	}
	policyContext := mcp.ArgumentPolicyContext{
		VPSID:  runtime.vps.ID,
		Plan:   runtime.vps.Plan,
		UserID: policyUserIDValue,
	}
	if err := mcp.ValidateArgumentPathPolicy(runtime.manifest.Workspace.Root, runtime.server, *toolName, input, policyContext); err != nil {
		details := mergeDetails(map[string]any{
			"server":      runtime.server.ID,
			"tool":        *toolName,
			"reason":      "argument_path_policy_denied",
			"error":       err.Error(),
			"vps_plan":    runtime.vps.Plan,
			"policy_user": policyContext.UserID,
		}, argumentSummary)
		var policyErr *mcp.ArgumentPolicyError
		if errors.As(err, &policyErr) {
			details = mergeDetails(details, policyErr.AuditDetails())
		}
		recordMCPAudit(runtime.logger, "mcp.argument_policy", runtime.vps.ID, "denied", details)
		fmt.Fprintf(stderr, "mcp argument policy denied for tool %q: %v\n", *toolName, err)
		return 1
	}
	executionContext := mcp.ExecutionPolicyContext{
		VPSID:  runtime.vps.ID,
		Plan:   runtime.vps.Plan,
		UserID: policyContext.UserID,
	}
	decision, err := mcp.EvaluateExecutionPolicy(runtime.manifest, runtime.server, *toolName, executionContext)
	if err != nil {
		recordMCPAudit(runtime.logger, "mcp.policy", runtime.vps.ID, "denied", mergeDetails(map[string]any{
			"server": runtime.server.ID,
			"tool":   *toolName,
			"reason": "execution_policy_error",
			"error":  err.Error(),
		}, argumentSummary))
		fmt.Fprintf(stderr, "mcp execution policy failed for tool %q: %v\n", *toolName, err)
		return 1
	}
	if decision.Action == mcp.ToolActionDeny {
		recordMCPAudit(runtime.logger, "mcp.policy", runtime.vps.ID, "denied", mergeDetails(executionPolicyAuditDetails(runtime.server.ID, *toolName, decision, executionContext), argumentSummary))
		fmt.Fprintf(stderr, "mcp tool %q denied by execution policy: %s\n", *toolName, decision.Reason)
		return 1
	}

	listStarted := time.Now()
	tools, err := runtime.client.MCPListTools(ctx, runtime.vps, runtime.server, timeout)
	listDuration := time.Since(listStarted)
	if err != nil {
		recordMCPAudit(runtime.logger, "mcp.call", runtime.vps.ID, "failure", mergeDetails(map[string]any{
			"server":           runtime.server.ID,
			"tool":             *toolName,
			"phase":            "tools/list",
			"list_duration_ms": listDuration.Milliseconds(),
			"error":            err.Error(),
		}, argumentSummary))
		fmt.Fprintf(stderr, "mcp tool preflight failed: %v\n", err)
		return 1
	}
	if !mcp.HasTool(tools.Tools, *toolName) {
		recordMCPAudit(runtime.logger, "mcp.call", runtime.vps.ID, "denied", mergeDetails(map[string]any{
			"server": runtime.server.ID,
			"tool":   *toolName,
			"reason": "tool_not_advertised",
			"tools":  mcp.ToolNames(tools.Tools),
		}, argumentSummary))
		fmt.Fprintf(stderr, "mcp tool %q is not advertised by server %q\n", *toolName, runtime.server.ID)
		return 1
	}

	if decision.RateLimit != nil {
		limit, err := runtime.store.ConsumeMCPRateLimit(ctx, decision.RateLimit.Key, decision.RateLimit.MaxCalls, decision.RateLimit.Window, time.Now().UTC())
		if err != nil {
			details := mergeDetails(executionPolicyAuditDetails(runtime.server.ID, *toolName, decision, executionContext), argumentSummary)
			details["rate_limit_key"] = decision.RateLimit.Key
			details["rate_limit_rule"] = decision.RateLimit.Rule
			details["rate_limit_max_calls"] = decision.RateLimit.MaxCalls
			details["rate_limit_window_seconds"] = int(decision.RateLimit.Window.Seconds())
			var exceeded state.MCPRateLimitExceededError
			if errors.As(err, &exceeded) {
				details["reset_at"] = exceeded.ResetAt.Format(time.RFC3339Nano)
				recordMCPAudit(runtime.logger, "mcp.rate_limit", runtime.vps.ID, "denied", details)
				fmt.Fprintf(stderr, "mcp rate limit denied for tool %q: reset_at=%s\n", *toolName, exceeded.ResetAt.Format(time.RFC3339Nano))
				return 1
			}
			details["error"] = err.Error()
			recordMCPAudit(runtime.logger, "mcp.rate_limit", runtime.vps.ID, "failure", details)
			fmt.Fprintf(stderr, "mcp rate limit failed for tool %q: %v\n", *toolName, err)
			return 1
		}
		recordMCPAudit(runtime.logger, "mcp.rate_limit", runtime.vps.ID, "allowed", mergeDetails(map[string]any{
			"server":       runtime.server.ID,
			"tool":         *toolName,
			"rate_rule":    decision.RateLimit.Rule,
			"limit":        limit.Limit,
			"count":        limit.Count,
			"remaining":    limit.Remaining,
			"reset_at":     limit.ResetAt.Format(time.RFC3339Nano),
			"window_start": limit.WindowStart.Format(time.RFC3339Nano),
		}, argumentSummary))
	}
	switch decision.Action {
	case mcp.ToolActionRequireApproval:
		if approvalIDValue == "" {
			now := time.Now().UTC()
			approval, err := runtime.store.CreateMCPApproval(ctx, state.MCPApproval{
				VPSID:          runtime.vps.ID,
				Server:         runtime.server.ID,
				Tool:           *toolName,
				ArgumentDigest: argumentDigest,
				ArgumentKeys:   mcp.ArgumentKeys(input),
				CreatedAt:      now,
				ExpiresAt:      now.Add(approvalTTL),
				Reason:         decision.Reason,
				Rule:           decision.Rule,
				ApprovedBy:     policyContext.UserID,
				PolicyVersion:  runtime.policyVersion,
			})
			if err != nil {
				recordMCPAudit(runtime.logger, "mcp.approval", runtime.vps.ID, "failure", mergeDetails(map[string]any{
					"server": runtime.server.ID,
					"tool":   *toolName,
					"action": decision.Action,
					"reason": "approval_create_failed",
					"rule":   decision.Rule,
					"error":  err.Error(),
				}, argumentSummary))
				fmt.Fprintf(stderr, "mcp approval creation failed: %v\n", err)
				return 1
			}
			recordMCPAudit(runtime.logger, "mcp.approval", runtime.vps.ID, "required", mergeDetails(map[string]any{
				"server":         runtime.server.ID,
				"tool":           *toolName,
				"action":         decision.Action,
				"reason":         decision.Reason,
				"rule":           decision.Rule,
				"approval_id":    approval.ID,
				"policy_user":    policyContext.UserID,
				"policy_version": runtime.policyVersion,
				"expires_at":     approval.ExpiresAt.Format(time.RFC3339Nano),
			}, argumentSummary))
			printMCPApprovalRequired(stdout, approval)
			return 1
		}
		approval, err := runtime.store.ConsumeMCPApproval(ctx, approvalIDValue, runtime.vps.ID, runtime.server.ID, *toolName, argumentDigest, time.Now().UTC())
		if err != nil {
			recordMCPAudit(runtime.logger, "mcp.approval", runtime.vps.ID, "denied", mergeDetails(map[string]any{
				"server":      runtime.server.ID,
				"tool":        *toolName,
				"action":      decision.Action,
				"reason":      "approval_invalid",
				"rule":        decision.Rule,
				"approval_id": approvalIDValue,
				"error":       err.Error(),
			}, argumentSummary))
			fmt.Fprintf(stderr, "mcp approval %q rejected: %v\n", approvalIDValue, err)
			return 1
		}
		approvalDetails := mergeDetails(map[string]any{
			"server":          runtime.server.ID,
			"tool":            *toolName,
			"action":          decision.Action,
			"reason":          decision.Reason,
			"rule":            decision.Rule,
			"approval_id":     approval.ID,
			"approved_by":     approval.ApprovedBy,
			"policy_version":  approval.PolicyVersion,
			"signed_approval": signedApproval != nil,
			"created_at":      approval.CreatedAt.Format(time.RFC3339Nano),
			"expires_at":      approval.ExpiresAt.Format(time.RFC3339Nano),
			"used_at":         approval.UsedAt.Format(time.RFC3339Nano),
		}, argumentSummary)
		if signedApproval != nil {
			approvalDetails["token_user"] = signedApproval.UserID
		}
		recordMCPAudit(runtime.logger, "mcp.approval", runtime.vps.ID, "approved", approvalDetails)
	case mcp.ToolActionAllow:
		if approvalIDValue != "" {
			recordMCPAudit(runtime.logger, "mcp.approval", runtime.vps.ID, "denied", mergeDetails(map[string]any{
				"server":      runtime.server.ID,
				"tool":        *toolName,
				"action":      decision.Action,
				"reason":      "approval_not_required",
				"rule":        decision.Rule,
				"approval_id": approvalIDValue,
			}, argumentSummary))
			fmt.Fprintf(stderr, "--approval-id/--approval-token is only accepted for tools marked require_approval\n")
			return 1
		}
		recordMCPAudit(runtime.logger, "mcp.approval", runtime.vps.ID, "not_required", mergeDetails(map[string]any{
			"server": runtime.server.ID,
			"tool":   *toolName,
			"action": decision.Action,
			"reason": decision.Reason,
			"rule":   decision.Rule,
		}, argumentSummary))
	default:
		recordMCPAudit(runtime.logger, "mcp.approval", runtime.vps.ID, "denied", map[string]any{
			"server": runtime.server.ID,
			"tool":   *toolName,
			"action": decision.Action,
			"reason": "invalid_policy_action",
		})
		fmt.Fprintf(stderr, "mcp tool %q denied by invalid policy action %q\n", *toolName, decision.Action)
		return 1
	}

	callStarted := time.Now()
	result, err := runtime.client.MCPCallTool(ctx, runtime.vps, runtime.server, *toolName, input, timeout)
	callDuration := time.Since(callStarted)
	if err != nil {
		recordMCPAudit(runtime.logger, "mcp.call", runtime.vps.ID, "failure", mergeDetails(map[string]any{
			"server":           runtime.server.ID,
			"tool":             *toolName,
			"call_duration_ms": callDuration.Milliseconds(),
			"error":            err.Error(),
		}, mcp.ArgumentSummary(input)))
		fmt.Fprintf(stderr, "mcp call failed: %v\n", err)
		return 1
	}
	details := mergeDetails(map[string]any{
		"server":           runtime.server.ID,
		"tool":             *toolName,
		"list_duration_ms": listDuration.Milliseconds(),
		"call_duration_ms": callDuration.Milliseconds(),
	}, mcp.ArgumentSummary(input))
	details = mergeDetails(details, mcp.ResultSummary(result))
	recordMCPAudit(runtime.logger, "mcp.call", runtime.vps.ID, "success", details)
	printJSON(stdout, result)
	return 0
}

type mcpRuntime struct {
	cfg           config.Config
	store         *state.Store
	logger        *audit.Logger
	cleanup       func()
	client        dockerbackend.Client
	vps           state.VPS
	manifest      mcp.Manifest
	policyVersion string
	server        mcp.Server
}

func openMCPRuntime(ctx context.Context, configPath string, manifestPath string, policyFile string, serverID string, vpsID string, vpsName string, stderr io.Writer) (mcpRuntime, bool) {
	if strings.TrimSpace(serverID) == "" {
		fmt.Fprintln(stderr, "--server is required")
		return mcpRuntime{}, false
	}
	cfg, err := loadValidConfig(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "config failed: %v\n", err)
		return mcpRuntime{}, false
	}
	if cfg.Backend.Type != "docker" {
		fmt.Fprintf(stderr, "mcp execution requires backend.type=docker, got %s\n", cfg.Backend.Type)
		return mcpRuntime{}, false
	}
	store, logger, cleanup, err := openRuntime(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "runtime open failed: %v\n", err)
		return mcpRuntime{}, false
	}
	closeOnFail := true
	defer func() {
		if closeOnFail {
			cleanup()
		}
	}()
	vps, err := findVPS(ctx, store, vpsID, vpsName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			fmt.Fprintln(stderr, "vps not found")
		} else {
			fmt.Fprintf(stderr, "vps lookup failed: %v\n", err)
		}
		return mcpRuntime{}, false
	}
	if vps.Backend != "docker" {
		fmt.Fprintf(stderr, "vps %s uses backend %s; mcp execution currently supports docker only\n", vps.ID, vps.Backend)
		return mcpRuntime{}, false
	}
	client, err := dockerClientFromConfig(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "docker config failed: %v\n", err)
		return mcpRuntime{}, false
	}
	resolvedManifestPath := mcp.ResolveManifestPath(firstNonEmpty(manifestPath, cfg.API.MCPManifest))
	resolvedPolicyFile := firstNonEmpty(policyFile, cfg.API.PolicyFile)
	manifest, _, policyVersion, err := mcp.LoadEffectiveFile(resolvedManifestPath, resolvedPolicyFile)
	if err != nil {
		fmt.Fprintf(stderr, "mcp manifest invalid: %v\n", err)
		return mcpRuntime{}, false
	}
	server, err := runnableMCPServer(manifest, serverID)
	if err != nil {
		recordMCPAudit(logger, "mcp.policy", vps.ID, "denied", map[string]any{
			"server": serverID,
			"reason": err.Error(),
		})
		fmt.Fprintf(stderr, "mcp server denied: %v\n", err)
		return mcpRuntime{}, false
	}
	closeOnFail = false
	return mcpRuntime{
		cfg:           cfg,
		store:         store,
		logger:        logger,
		cleanup:       cleanup,
		client:        client,
		vps:           vps,
		manifest:      manifest,
		policyVersion: policyVersion,
		server:        server,
	}, true
}

func runnableMCPServer(manifest mcp.Manifest, serverID string) (mcp.Server, error) {
	for _, view := range manifest.ServerViews(true) {
		if view.ID != serverID {
			continue
		}
		if !view.Enabled {
			return mcp.Server{}, fmt.Errorf("server %q is disabled by manifest", serverID)
		}
		if view.Optional {
			return mcp.Server{}, fmt.Errorf("server %q is optional and not enabled by default", serverID)
		}
		if view.Transport != "stdio" {
			return mcp.Server{}, fmt.Errorf("server %q transport %q is not supported for direct execution", serverID, view.Transport)
		}
		if len(view.Command) == 0 {
			return mcp.Server{}, fmt.Errorf("server %q command is empty", serverID)
		}
		return view.Server, nil
	}
	return mcp.Server{}, fmt.Errorf("server %q not found in manifest", serverID)
}

func readJSONInput(value string) (json.RawMessage, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "{}"
	}
	var raw []byte
	if strings.HasPrefix(value, "@") {
		path := strings.TrimPrefix(value, "@")
		if strings.TrimSpace(path) == "" {
			return nil, errors.New("@ input requires a file path")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		raw = data
	} else {
		raw = []byte(value)
	}
	if !json.Valid(raw) {
		return nil, errors.New("input must be valid JSON")
	}
	return json.RawMessage(raw), nil
}

func readSecretValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "@") {
		path := strings.TrimPrefix(value, "@")
		if strings.TrimSpace(path) == "" {
			return "", errors.New("@ value requires a file path")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		value = strings.TrimSpace(string(raw))
	}
	return value, nil
}

func writeSecretFile(path string, value string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("secret file path is required")
	}
	if strings.TrimSpace(value) == "" {
		return errors.New("secret value is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(value)), 0o600)
}

func printMCPApprovalRequired(w io.Writer, approval state.MCPApproval) {
	keys := strings.Join(approval.ArgumentKeys, ",")
	if keys == "" {
		keys = "-"
	}
	fmt.Fprintln(w, "approval required")
	fmt.Fprintf(w, "approval_id: %s\n", approval.ID)
	fmt.Fprintf(w, "server: %s\n", approval.Server)
	fmt.Fprintf(w, "tool: %s\n", approval.Tool)
	fmt.Fprintf(w, "argument_keys: %s\n", keys)
	fmt.Fprintf(w, "argument_digest: %s\n", approval.ArgumentDigest)
	if approval.ApprovedBy != "" {
		fmt.Fprintf(w, "approved_by: %s\n", approval.ApprovedBy)
	}
	if approval.PolicyVersion != "" {
		fmt.Fprintf(w, "policy_version: %s\n", approval.PolicyVersion)
	}
	fmt.Fprintf(w, "expires_at: %s\n", approval.ExpiresAt.Format(time.RFC3339Nano))
	fmt.Fprintf(w, "rerun_with: --approval-id %s\n", approval.ID)
	fmt.Fprintf(w, "api_approve: POST /v1/mcp/approvals/%s/approve\n", approval.ID)
	fmt.Fprintln(w, "rerun_with_signed: --approval-token <approval_token>")
}

func executionPolicyAuditDetails(serverID string, toolName string, decision mcp.ExecutionDecision, ctx mcp.ExecutionPolicyContext) map[string]any {
	return map[string]any{
		"server":        serverID,
		"tool":          toolName,
		"action":        decision.Action,
		"reason":        decision.Reason,
		"rule":          decision.Rule,
		"matched_rules": decision.MatchedRules,
		"risk":          decision.Risk,
		"network":       decision.Network,
		"command_bin":   decision.CommandBin,
		"vps_plan":      ctx.Plan,
		"policy_user":   ctx.UserID,
	}
}

func recordMCPAudit(logger *audit.Logger, action string, target string, outcome string, details map[string]any) {
	if logger == nil {
		return
	}
	_ = logger.Record(audit.Event{
		Actor:   "stola-cli",
		Action:  action,
		Target:  target,
		Outcome: outcome,
		Details: details,
	})
}

func cmdApprovals(ctx context.Context, configPath string, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: stola approvals list|approve [--api URL]")
		return 2
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("approvals list", flag.ContinueOnError)
		fs.SetOutput(stderr)
		apiURL := fs.String("api", "", "control-plane API URL")
		tokenEnv := fs.String("token-env", "STOLA_API_TOKEN", "environment variable containing the API bearer token")
		tokenFile := fs.String("token-file", "", "file containing the API bearer token")
		status := fs.String("status", state.MCPApprovalStatusPending, "approval status filter")
		limit := fs.Int("limit", 50, "maximum approvals to list")
		jsonOutput := fs.Bool("json", false, "print raw JSON response")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		cfg, err := loadValidConfig(configPath)
		if err != nil {
			fmt.Fprintf(stderr, "config failed: %v\n", err)
			return 1
		}
		client, err := newControlPlaneAPIClient(cfg, *apiURL, *tokenEnv, *tokenFile)
		if err != nil {
			fmt.Fprintf(stderr, "approvals auth failed: %v\n", err)
			return 1
		}
		response, err := client.ListApprovals(ctx, *status, *limit)
		if err != nil {
			fmt.Fprintf(stderr, "approvals list failed: %v\n", err)
			return 1
		}
		if *jsonOutput {
			printJSON(stdout, response)
			return 0
		}
		if len(response.Approvals) == 0 {
			fmt.Fprintln(stdout, "no approvals")
			return 0
		}
		for _, approval := range response.Approvals {
			keys := strings.Join(approval.ArgumentKeys, ",")
			if keys == "" {
				keys = "-"
			}
			fmt.Fprintf(stdout, "id=%s status=%s vps=%s server=%s tool=%s keys=%s expires_at=%s",
				approval.ID,
				approval.Status,
				approval.VPSID,
				approval.Server,
				approval.Tool,
				keys,
				approval.ExpiresAt.Format(time.RFC3339Nano),
			)
			if approval.ApprovedBy != "" {
				fmt.Fprintf(stdout, " approved_by=%s", approval.ApprovedBy)
			}
			if approval.PolicyVersion != "" {
				fmt.Fprintf(stdout, " policy_version=%s", approval.PolicyVersion)
			}
			if approval.Rule != "" {
				fmt.Fprintf(stdout, " rule=%s", approval.Rule)
			}
			fmt.Fprintln(stdout)
		}
		return 0
	case "approve":
		fs := flag.NewFlagSet("approvals approve", flag.ContinueOnError)
		fs.SetOutput(stderr)
		apiURL := fs.String("api", "", "control-plane API URL")
		tokenEnv := fs.String("token-env", "STOLA_API_TOKEN", "environment variable containing the API bearer token")
		tokenFile := fs.String("token-file", "", "file containing the API bearer token")
		id := fs.String("id", "", "approval id")
		writeToken := fs.String("write-token", "", "write signed approval token to this file")
		printToken := fs.Bool("print-token", false, "print signed approval token to stdout")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if strings.TrimSpace(*id) == "" {
			fmt.Fprintln(stderr, "--id is required")
			return 2
		}
		cfg, err := loadValidConfig(configPath)
		if err != nil {
			fmt.Fprintf(stderr, "config failed: %v\n", err)
			return 1
		}
		client, err := newControlPlaneAPIClient(cfg, *apiURL, *tokenEnv, *tokenFile)
		if err != nil {
			fmt.Fprintf(stderr, "approvals auth failed: %v\n", err)
			return 1
		}
		response, err := client.ApproveApproval(ctx, *id)
		if err != nil {
			fmt.Fprintf(stderr, "approval approve failed: %v\n", err)
			return 1
		}
		if strings.TrimSpace(*writeToken) != "" {
			if err := writeSecretFile(*writeToken, response.ApprovalToken); err != nil {
				fmt.Fprintf(stderr, "approval token write failed: %v\n", err)
				return 1
			}
		}
		fmt.Fprintf(stdout, "approved approval_id=%s vps=%s server=%s tool=%s approved_by=%s policy_version=%s expires_at=%s\n",
			response.ApprovalID,
			response.VPSID,
			response.Server,
			response.Tool,
			response.ApprovedBy,
			response.PolicyVersion,
			response.ExpiresAt.Format(time.RFC3339Nano),
		)
		if strings.TrimSpace(*writeToken) != "" {
			fmt.Fprintf(stdout, "token_file: %s\n", *writeToken)
			fmt.Fprintf(stdout, "rerun_with: --approval-token @%s\n", *writeToken)
		} else if *printToken {
			fmt.Fprintf(stdout, "approval_token: %s\n", response.ApprovalToken)
		} else {
			fmt.Fprintln(stdout, "approval_token: hidden")
			fmt.Fprintln(stdout, "rerun_with: use --write-token PATH or --print-token")
		}
		return 0
	default:
		fmt.Fprintf(stderr, "unknown approvals command: %s\n", args[0])
		return 2
	}
}

type controlPlaneAPIClient struct {
	baseURL string
	token   string
	client  *http.Client
}

type controlPlaneApprovalListResponse struct {
	Approvals []controlPlaneApprovalView `json:"approvals"`
}

type controlPlaneApprovalView struct {
	ID             string    `json:"id"`
	VPSID          string    `json:"vps_id"`
	Server         string    `json:"server"`
	Tool           string    `json:"tool"`
	ArgumentDigest string    `json:"argument_digest"`
	ArgumentKeys   []string  `json:"argument_keys,omitempty"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	UsedAt         time.Time `json:"used_at,omitempty"`
	Reason         string    `json:"reason,omitempty"`
	Rule           string    `json:"rule,omitempty"`
	ApprovedBy     string    `json:"approved_by,omitempty"`
	PolicyVersion  string    `json:"policy_version,omitempty"`
}

type controlPlaneApprovalResponse struct {
	ApprovalID     string    `json:"approval_id"`
	ApprovalToken  string    `json:"approval_token"`
	VPSID          string    `json:"vps_id"`
	Server         string    `json:"server"`
	Tool           string    `json:"tool"`
	ArgumentDigest string    `json:"argument_digest"`
	ArgumentKeys   []string  `json:"argument_keys,omitempty"`
	ApprovedBy     string    `json:"approved_by"`
	PolicyVersion  string    `json:"policy_version"`
	ExpiresAt      time.Time `json:"expires_at"`
}

func newControlPlaneAPIClient(cfg config.Config, explicitURL string, tokenEnv string, tokenFile string) (controlPlaneAPIClient, error) {
	token, err := controlPlaneAPIToken(tokenEnv, tokenFile)
	if err != nil {
		return controlPlaneAPIClient{}, err
	}
	return controlPlaneAPIClient{
		baseURL: controlPlaneAPIBaseURL(cfg, explicitURL),
		token:   token,
		client:  &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (c controlPlaneAPIClient) ListApprovals(ctx context.Context, status string, limit int) (controlPlaneApprovalListResponse, error) {
	values := url.Values{}
	if strings.TrimSpace(status) != "" {
		values.Set("status", strings.TrimSpace(status))
	}
	if limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", limit))
	}
	endpoint := c.baseURL + "/v1/mcp/approvals"
	if encoded := values.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	var response controlPlaneApprovalListResponse
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return controlPlaneApprovalListResponse{}, err
	}
	return response, nil
}

func (c controlPlaneAPIClient) ApproveApproval(ctx context.Context, id string) (controlPlaneApprovalResponse, error) {
	var response controlPlaneApprovalResponse
	endpoint := c.baseURL + "/v1/mcp/approvals/" + url.PathEscape(strings.TrimSpace(id)) + "/approve"
	if err := c.doJSON(ctx, http.MethodPost, endpoint, nil, &response); err != nil {
		return controlPlaneApprovalResponse{}, err
	}
	return response, nil
}

func (c controlPlaneAPIClient) doJSON(ctx context.Context, method string, endpoint string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if apiErr.Error != "" {
			return fmt.Errorf("%s: %s", resp.Status, apiErr.Error)
		}
		return fmt.Errorf("%s", resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func controlPlaneAPIBaseURL(cfg config.Config, explicit string) string {
	base := strings.TrimRight(strings.TrimSpace(explicit), "/")
	if base != "" {
		return base
	}
	listen := strings.TrimSpace(cfg.API.Listen)
	if listen == "" {
		listen = "127.0.0.1:8750"
	}
	if strings.HasPrefix(listen, "http://") || strings.HasPrefix(listen, "https://") {
		return strings.TrimRight(listen, "/")
	}
	return "http://" + listen
}

func controlPlaneAPIToken(tokenEnv string, tokenFile string) (string, error) {
	if strings.TrimSpace(tokenFile) != "" {
		raw, err := os.ReadFile(tokenFile)
		if err != nil {
			return "", err
		}
		token := strings.TrimSpace(string(raw))
		if token == "" {
			return "", fmt.Errorf("API token file is empty: %s", tokenFile)
		}
		return token, nil
	}
	if strings.TrimSpace(tokenEnv) == "" {
		tokenEnv = "STOLA_API_TOKEN"
	}
	token := strings.TrimSpace(os.Getenv(tokenEnv))
	if token == "" {
		return "", fmt.Errorf("set %s or pass --token-file", tokenEnv)
	}
	return token, nil
}

func mergeDetails(base map[string]any, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func printJSON(w io.Writer, value any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(value)
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func cmdDoctor(configPath string, stdout io.Writer, stderr io.Writer) int {
	cfg, loaded, err := config.LoadOrDefault(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "config: error: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "version: %s\n", version.Version)
	if loaded {
		fmt.Fprintf(stdout, "config: %s\n", configPath)
	} else {
		fmt.Fprintf(stdout, "config: %s (not found; using defaults)\n", configPath)
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(stderr, "validation: failed: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "validation: ok")
	fmt.Fprintf(stdout, "node: %s\n", cfg.Node.ID)
	fmt.Fprintf(stdout, "backend: %s\n", cfg.Backend.Type)
	if cfg.Backend.Type == "mock" {
		fmt.Fprintln(stdout, "proxmox: not required in mock mode")
	}
	fmt.Fprintf(stdout, "state: %s\n", cfg.Paths.State)
	fmt.Fprintf(stdout, "audit_log: %s\n", cfg.Paths.AuditLog)
	return 0
}

func cmdInit(configPath string, args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	force := fs.Bool("force", false, "overwrite existing config")
	nodeID := fs.String("node-id", "stola-local", "node id")
	statePath := fs.String("state", config.DefaultStatePath, "state database path")
	auditPath := fs.String("audit-log", config.DefaultAuditPath, "audit log path")
	backend := fs.String("backend", "mock", "backend type: mock, docker, or proxmox")
	proxmoxEndpoint := fs.String("proxmox-endpoint", "", "Proxmox API endpoint")
	proxmoxNode := fs.String("proxmox-node", "", "Proxmox node name")
	proxmoxTokenFile := fs.String("proxmox-token-file", "", "file containing Proxmox API token")
	proxmoxInsecure := fs.Bool("proxmox-insecure-skip-tls-verify", false, "allow self-signed Proxmox TLS certificates")
	dockerCLIPath := fs.String("docker-cli-path", "", "Docker CLI path")
	dockerImage := fs.String("docker-image", "", "default Docker image for workspaces")
	dockerNetworkPrefix := fs.String("docker-network-prefix", "", "Docker network prefix")
	dockerVolumePrefix := fs.String("docker-volume-prefix", "", "Docker volume prefix")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg := config.Default()
	cfg.Node.ID = *nodeID
	cfg.Backend.Type = *backend
	cfg.Backend.Proxmox.Endpoint = *proxmoxEndpoint
	cfg.Backend.Proxmox.Node = *proxmoxNode
	cfg.Backend.Proxmox.TokenFile = *proxmoxTokenFile
	cfg.Backend.Proxmox.InsecureSkipTLSVerify = *proxmoxInsecure
	cfg.Backend.Docker.CLIPath = *dockerCLIPath
	cfg.Backend.Docker.Image = *dockerImage
	cfg.Backend.Docker.NetworkPrefix = *dockerNetworkPrefix
	cfg.Backend.Docker.VolumePrefix = *dockerVolumePrefix
	cfg.Paths.State = *statePath
	cfg.Paths.AuditLog = *auditPath
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(stderr, "default config invalid: %v\n", err)
		return 1
	}
	if err := config.WriteFile(configPath, cfg, *force); err != nil {
		fmt.Fprintf(stderr, "init failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "created config: %s\n", configPath)
	return 0
}

func cmdDocker(ctx context.Context, configPath string, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: stola docker check")
		return 2
	}
	cfg, err := loadValidConfig(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "config failed: %v\n", err)
		return 1
	}
	client, err := dockerClientFromConfig(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "docker config failed: %v\n", err)
		return 1
	}

	switch args[0] {
	case "check":
		result, err := client.Check(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "docker check failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "server_version: %s\n", result.ServerVersion)
		fmt.Fprintf(stdout, "security: %s\n", result.Security)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown docker command: %s\n", args[0])
		return 2
	}
}

func cmdProxmox(ctx context.Context, configPath string, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: stola proxmox check|nodes|storage [--node NODE]")
		return 2
	}
	cfg, err := loadValidConfig(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "config failed: %v\n", err)
		return 1
	}
	client, err := proxmoxClientFromConfig(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "proxmox config failed: %v\n", err)
		return 1
	}

	switch args[0] {
	case "check":
		result, err := client.Check(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "proxmox check failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "version: %s\n", result.Version.Version)
		fmt.Fprintf(stdout, "release: %s\n", result.Version.Release)
		fmt.Fprintf(stdout, "repoid: %s\n", result.Version.RepoID)
		fmt.Fprintf(stdout, "nodes: %d\n", len(result.Nodes))
		for _, node := range result.Nodes {
			fmt.Fprintf(stdout, "node: %s status=%s\n", node.Node, node.Status)
		}
		fmt.Fprintf(stdout, "storage[%s]: %d\n", cfg.Backend.Proxmox.Node, len(result.Storage))
		for _, storage := range result.Storage {
			fmt.Fprintf(stdout, "storage: %s type=%s active=%d content=%s\n", storage.Storage, storage.Type, storage.Active, storage.Content)
		}
		return 0
	case "nodes":
		nodes, err := client.Nodes(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "proxmox nodes failed: %v\n", err)
			return 1
		}
		for _, node := range nodes {
			fmt.Fprintf(stdout, "%s status=%s cpu=%.4f mem=%d uptime=%d\n", node.Node, node.Status, node.CPU, node.Memory, node.Uptime)
		}
		return 0
	case "storage":
		fs := flag.NewFlagSet("storage", flag.ContinueOnError)
		fs.SetOutput(stderr)
		node := fs.String("node", cfg.Backend.Proxmox.Node, "Proxmox node")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		storage, err := client.Storage(ctx, *node)
		if err != nil {
			fmt.Fprintf(stderr, "proxmox storage failed: %v\n", err)
			return 1
		}
		for _, item := range storage {
			fmt.Fprintf(stdout, "%s type=%s active=%d content=%s total=%d used=%d avail=%d\n", item.Storage, item.Type, item.Active, item.Content, item.Total, item.Used, item.Avail)
		}
		return 0
	default:
		fmt.Fprintf(stderr, "unknown proxmox command: %s\n", args[0])
		return 2
	}
}

func cmdVPS(ctx context.Context, configPath string, args []string, stdout io.Writer, stderr io.Writer, surface string) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "usage: stola %s create|list|status|logs|exec|stop|start|delete\n", surface)
		return 2
	}
	if args[0] != "create" {
		return cmdVPSLifecycle(ctx, configPath, args, stdout, stderr, surface)
	}
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("name", "stola-nano", "VPS/container name")
	plan := fs.String("plan", "nano", "Stola plan")
	image := fs.String("image", "", "Docker image for container backend")
	vmid := fs.Int("vmid", 0, "Proxmox VMID/CTID")
	template := fs.String("template", "", "Proxmox LXC ostemplate, e.g. local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst")
	storageName := fs.String("storage", "", "Proxmox storage for rootfs")
	rootFSSize := fs.String("rootfs-size", "8G", "rootfs size for Nano LXC")
	bridge := fs.String("bridge", "vmbr0", "network bridge")
	memory := fs.Int("memory", 1024, "memory in MiB")
	swap := fs.Int("swap", 512, "swap in MiB")
	cores := fs.Int("cores", 1, "CPU cores")
	pidsLimit := fs.Int("pids-limit", 256, "process limit for Docker backend")
	mcpManifest := fs.String("mcp-manifest", "", "MCP manifest path to inject into Docker workspace")
	providersManifest := fs.String("providers-manifest", "", "provider manifest path to inject into Docker workspace")
	secretsFile := fs.String("secrets-file", "", "provider secret env file to inject into Docker workspace")
	noMCPInject := fs.Bool("no-mcp-inject", false, "disable MCP config injection for Docker backend")
	noProvidersInject := fs.Bool("no-provider-inject", false, "disable provider config injection for Docker backend")
	llmProxyURL := fs.String("llm-proxy-url", "", "Stolas LLM proxy URL to inject into Docker workspace")
	llmProvider := fs.String("llm-provider", "openrouter", "LLM provider id for Stolas proxy virtual key")
	llmModels := fs.String("llm-models", "", "comma-separated model allowlist for Stolas proxy virtual key")
	llmBudgetUSD := fs.Float64("llm-budget-usd", 0, "LLM proxy budget in USD for this workspace key")
	llmRPM := fs.Int("llm-rpm", 0, "LLM proxy requests-per-minute limit for this workspace key")
	llmTPM := fs.Int("llm-tpm", 0, "LLM proxy estimated tokens-per-minute limit for this workspace key")
	llmMaxConcurrency := fs.Int("llm-max-concurrency", 0, "LLM proxy max concurrent requests for this workspace key")
	start := fs.Bool("start", false, "start the container after creation")
	privileged := fs.Bool("privileged", false, "create a privileged container")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	cfg, err := loadValidConfig(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "config failed: %v\n", err)
		return 1
	}
	store, logger, cleanup, err := openRuntime(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "runtime open failed: %v\n", err)
		return 1
	}
	defer cleanup()
	var secretNames []string
	proxyURL := strings.TrimSpace(*llmProxyURL)
	proxyModels := splitCSV(*llmModels)
	if proxyURL != "" {
		if cfg.Backend.Type != "docker" {
			fmt.Fprintln(stderr, "--llm-proxy-url is supported only with backend.type=docker")
			return 2
		}
		if strings.TrimSpace(*llmProvider) == "" {
			fmt.Fprintln(stderr, "--llm-provider is required when --llm-proxy-url is set")
			return 2
		}
		if *llmBudgetUSD < 0 || *llmRPM < 0 || *llmTPM < 0 || *llmMaxConcurrency < 0 {
			fmt.Fprintln(stderr, "llm budget, rpm, tpm, and max concurrency must not be negative")
			return 2
		}
	}
	if strings.TrimSpace(*secretsFile) != "" {
		if cfg.Backend.Type != "docker" {
			fmt.Fprintln(stderr, "--secrets-file is supported only with backend.type=docker")
			return 2
		}
		secretBundle, manifestPath, err := loadProviderSecrets(*secretsFile, *providersManifest)
		if err != nil {
			fmt.Fprintf(stderr, "secret file invalid: %v\n", err)
			return 1
		}
		secretNames = secrets.Names(secretBundle.Secrets)
		fmt.Fprintf(stdout, "provider secrets selected: names=%d providers_manifest=%s\n", len(secretNames), manifestPath)
	}

	var backend provisioner.Backend
	switch cfg.Backend.Type {
	case "mock":
		backend = provisioner.MockBackend{}
	case "docker":
		client, err := dockerClientFromConfig(cfg)
		if err != nil {
			fmt.Fprintf(stderr, "docker config failed: %v\n", err)
			return 1
		}
		backend = client
	case "proxmox":
		client, err := proxmoxClientFromConfig(cfg)
		if err != nil {
			fmt.Fprintf(stderr, "proxmox config failed: %v\n", err)
			return 1
		}
		backend = client
	default:
		fmt.Fprintf(stderr, "unsupported backend: %s\n", cfg.Backend.Type)
		return 1
	}
	spec := provisioner.CreateSpec{
		Name:              *name,
		Plan:              *plan,
		Image:             *image,
		VMID:              *vmid,
		Template:          *template,
		Storage:           *storageName,
		RootFSSize:        *rootFSSize,
		Bridge:            *bridge,
		MemoryMB:          *memory,
		SwapMB:            *swap,
		Cores:             *cores,
		PIDsLimit:         *pidsLimit,
		InjectMCP:         cfg.Backend.Type == "docker" && !*noMCPInject,
		MCPManifest:       *mcpManifest,
		InjectProviders:   cfg.Backend.Type == "docker" && !*noProvidersInject,
		ProvidersManifest: *providersManifest,
		InjectSecrets:     cfg.Backend.Type == "docker" && strings.TrimSpace(*secretsFile) != "",
		SecretsFile:       *secretsFile,
		SecretNames:       secretNames,
		Start:             *start,
		Unprivileged:      !*privileged,
	}
	vps, err := (provisioner.Service{State: store, Audit: logger, Back: backend}).Create(ctx, spec)
	if err != nil {
		fmt.Fprintf(stderr, "%s create failed: %v\n", surface, err)
		return 1
	}
	fmt.Fprintf(stdout, "created %s: id=%s name=%s plan=%s backend=%s status=%s\n", surface, vps.ID, vps.Name, vps.Plan, vps.Backend, vps.Status)
	if vps.ExternalID != "" {
		fmt.Fprintf(stdout, "external_id: %s\n", vps.ExternalID)
	}
	if vps.TaskID != "" {
		fmt.Fprintf(stdout, "task_id: %s\n", vps.TaskID)
	}
	if proxyURL != "" {
		rawKey, err := state.GenerateLLMProxyKey()
		if err != nil {
			fmt.Fprintf(stderr, "llm proxy key generation failed: %v\n", err)
			return 1
		}
		key, err := store.CreateLLMProxyKey(ctx, state.LLMProxyKey{
			KeyHash:        state.HashLLMProxyKey(rawKey),
			WorkspaceID:    vps.ID,
			UserID:         "local-cli",
			ProviderID:     strings.TrimSpace(*llmProvider),
			Models:         proxyModels,
			BudgetUSD:      *llmBudgetUSD,
			RPM:            *llmRPM,
			TPM:            *llmTPM,
			MaxConcurrency: *llmMaxConcurrency,
		})
		if err != nil {
			fmt.Fprintf(stderr, "llm proxy key create failed: %v\n", err)
			return 1
		}
		dockerClient, ok := backend.(dockerbackend.Client)
		if !ok {
			_, _ = store.RevokeLLMProxyKey(ctx, key.ID)
			fmt.Fprintln(stderr, "docker backend unavailable for llm proxy injection")
			return 1
		}
		if err := dockerClient.InjectLLMProxy(ctx, vps, dockerbackend.LLMProxyInjection{
			URL:        proxyURL,
			Token:      rawKey,
			ProviderID: key.ProviderID,
			Models:     key.Models,
			KeyID:      key.ID,
		}); err != nil {
			_, _ = store.RevokeLLMProxyKey(ctx, key.ID)
			fmt.Fprintf(stderr, "llm proxy injection failed: %v\n", err)
			return 1
		}
		if logger != nil {
			_ = logger.Record(audit.Event{
				Actor:   "stola-cli",
				Action:  "llm_proxy.inject",
				Target:  vps.ID,
				Outcome: "success",
				Details: map[string]any{
					"key_id":          key.ID,
					"provider":        key.ProviderID,
					"models":          key.Models,
					"budget_usd":      key.BudgetUSD,
					"rpm":             key.RPM,
					"tpm":             key.TPM,
					"max_concurrency": key.MaxConcurrency,
				},
			})
		}
		fmt.Fprintf(stdout, "llm proxy injected: key_id=%s provider=%s models=%s\n", key.ID, key.ProviderID, strings.Join(key.Models, ","))
	}
	return 0
}

func cmdVPSLifecycle(ctx context.Context, configPath string, args []string, stdout io.Writer, stderr io.Writer, surface string) int {
	cfg, err := loadValidConfig(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "config failed: %v\n", err)
		return 1
	}
	store, logger, cleanup, err := openRuntime(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "runtime open failed: %v\n", err)
		return 1
	}
	defer cleanup()

	switch args[0] {
	case "list":
		list, err := store.ListVPS(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "%s list failed: %v\n", surface, err)
			return 1
		}
		for _, vps := range list {
			fmt.Fprintf(stdout, "id=%s name=%s plan=%s backend=%s status=%s external_id=%s\n", vps.ID, vps.Name, vps.Plan, vps.Backend, vps.Status, vps.ExternalID)
		}
		return 0
	case "status", "logs", "exec", "stop", "start", "delete":
	default:
		fmt.Fprintf(stderr, "unknown %s command: %s\n", surface, args[0])
		return 2
	}

	vps, remaining, ok := parseVPSRef(ctx, store, args[0], args[1:], stderr)
	if !ok {
		return 2
	}
	if vps.Backend != "docker" {
		fmt.Fprintf(stderr, "%s %s uses backend %s; lifecycle commands currently support docker only\n", surface, vps.ID, vps.Backend)
		return 1
	}
	if cfg.Backend.Type != "docker" {
		fmt.Fprintf(stderr, "config backend is %s; docker lifecycle requires backend.type=docker\n", cfg.Backend.Type)
		return 1
	}
	client, err := dockerClientFromConfig(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "docker config failed: %v\n", err)
		return 1
	}

	switch args[0] {
	case "status":
		status, err := client.Status(ctx, vps)
		if err != nil {
			fmt.Fprintf(stderr, "%s status failed: %v\n", surface, err)
			return 1
		}
		_ = store.UpdateVPSStatus(ctx, vps.ID, status)
		fmt.Fprintf(stdout, "id=%s name=%s backend=docker status=%s external_id=%s\n", vps.ID, vps.Name, status, vps.ExternalID)
		recordCLIEvent(logger, "vps.status", vps.ID, "success", map[string]any{"status": status})
		return 0
	case "logs":
		tail := 100
		if len(remaining) > 0 {
			fmt.Fprintf(stderr, "unexpected logs arguments: %s\n", strings.Join(remaining, " "))
			return 2
		}
		if parsedTail, ok := vpsTailFlag(args[1:]); ok {
			tail = parsedTail
		}
		logs, err := client.Logs(ctx, vps, tail)
		if err != nil {
			fmt.Fprintf(stderr, "%s logs failed: %v\n", surface, err)
			return 1
		}
		fmt.Fprint(stdout, logs)
		recordCLIEvent(logger, "vps.logs", vps.ID, "success", map[string]any{"tail": tail})
		return 0
	case "exec":
		if len(remaining) == 0 {
			fmt.Fprintf(stderr, "usage: stola %s exec --id ID -- COMMAND [ARG...]\n", surface)
			return 2
		}
		out, err := client.Exec(ctx, vps, remaining)
		if err != nil {
			fmt.Fprintf(stderr, "%s exec failed: %v\n", surface, err)
			recordCLIEvent(logger, "vps.exec", vps.ID, "failure", execAuditDetails(remaining))
			return 1
		}
		fmt.Fprint(stdout, out)
		recordCLIEvent(logger, "vps.exec", vps.ID, "success", execAuditDetails(remaining))
		return 0
	case "stop":
		if err := client.Stop(ctx, vps); err != nil {
			fmt.Fprintf(stderr, "%s stop failed: %v\n", surface, err)
			return 1
		}
		_ = store.UpdateVPSStatus(ctx, vps.ID, "stopped")
		fmt.Fprintf(stdout, "stopped %s: id=%s name=%s\n", surface, vps.ID, vps.Name)
		recordCLIEvent(logger, "vps.stop", vps.ID, "success", nil)
		return 0
	case "start":
		if err := client.Start(ctx, vps); err != nil {
			fmt.Fprintf(stderr, "%s start failed: %v\n", surface, err)
			return 1
		}
		_ = store.UpdateVPSStatus(ctx, vps.ID, "running")
		fmt.Fprintf(stdout, "started %s: id=%s name=%s\n", surface, vps.ID, vps.Name)
		recordCLIEvent(logger, "vps.start", vps.ID, "success", nil)
		return 0
	case "delete":
		if err := client.Delete(ctx, vps); err != nil {
			fmt.Fprintf(stderr, "%s delete failed: %v\n", surface, err)
			return 1
		}
		if err := store.DeleteVPS(ctx, vps.ID); err != nil {
			fmt.Fprintf(stderr, "%s state delete failed: %v\n", surface, err)
			return 1
		}
		fmt.Fprintf(stdout, "deleted %s: id=%s name=%s\n", surface, vps.ID, vps.Name)
		recordCLIEvent(logger, "vps.delete", vps.ID, "success", nil)
		return 0
	default:
		return 2
	}
}

func parseVPSRef(ctx context.Context, store *state.Store, command string, args []string, stderr io.Writer) (state.VPS, []string, bool) {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	id := fs.String("id", "", "VPS id")
	name := fs.String("name", "", "VPS name")
	tail := fs.Int("tail", 100, "log lines to print")
	if command != "logs" {
		_ = tail
	}
	if err := fs.Parse(args); err != nil {
		return state.VPS{}, nil, false
	}
	vps, err := findVPS(ctx, store, *id, *name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			fmt.Fprintln(stderr, "vps not found")
		} else {
			fmt.Fprintf(stderr, "vps lookup failed: %v\n", err)
		}
		return state.VPS{}, nil, false
	}
	return vps, fs.Args(), true
}

func findVPS(ctx context.Context, store *state.Store, id string, name string) (state.VPS, error) {
	if id != "" {
		return store.GetVPS(ctx, id)
	}
	if name == "" {
		return state.VPS{}, errors.New("--id or --name is required")
	}
	list, err := store.ListVPS(ctx)
	if err != nil {
		return state.VPS{}, err
	}
	for _, vps := range list {
		if vps.Name == name {
			return vps, nil
		}
	}
	return state.VPS{}, sql.ErrNoRows
}

func vpsTailFlag(args []string) (int, bool) {
	fs := flag.NewFlagSet("logs-tail", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	tail := fs.Int("tail", 100, "log lines to print")
	if err := fs.Parse(args); err != nil {
		return 100, false
	}
	return *tail, true
}

func recordCLIEvent(logger *audit.Logger, action string, target string, outcome string, details map[string]any) {
	if logger == nil {
		return
	}
	_ = logger.Record(audit.Event{
		Actor:   "stola-cli",
		Action:  action,
		Target:  target,
		Outcome: outcome,
		Details: details,
	})
}

func execAuditDetails(argv []string) map[string]any {
	details := map[string]any{"argc": len(argv)}
	if len(argv) == 0 {
		return details
	}
	command := argv[0]
	if idx := strings.LastIndexAny(command, `/\`); idx >= 0 {
		command = command[idx+1:]
	}
	lower := strings.ToLower(command)
	if strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "api_key") || strings.Contains(lower, "apikey") || strings.Contains(lower, "sk-") {
		command = audit.Redacted
	}
	details["command"] = command
	return details
}

func cmdConfig(configPath string, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) != 1 || args[0] != "validate" {
		fmt.Fprintln(stderr, "usage: stola config validate")
		return 2
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "config load failed: %v\n", err)
		return 1
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(stderr, "config invalid: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "config valid: %s\n", configPath)
	return 0
}

func cmdNode(configPath string, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) != 1 || args[0] != "status" {
		fmt.Fprintln(stderr, "usage: stola node status")
		return 2
	}
	cfg, loaded, err := config.LoadOrDefault(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "config load failed: %v\n", err)
		return 1
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(stderr, "config invalid: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "version: %s\n", version.Version)
	fmt.Fprintf(stdout, "node_id: %s\n", cfg.Node.ID)
	fmt.Fprintf(stdout, "backend: %s\n", cfg.Backend.Type)
	fmt.Fprintf(stdout, "config_loaded: %t\n", loaded)
	fmt.Fprintf(stdout, "config_path: %s\n", configPath)
	fmt.Fprintf(stdout, "state_path: %s\n", cfg.Paths.State)
	fmt.Fprintf(stdout, "audit_log_path: %s\n", cfg.Paths.AuditLog)
	fmt.Fprintf(stdout, "runtime_dir: %s\n", cfg.Paths.RuntimeDir)
	return 0
}

func cmdProvisioner(ctx context.Context, configPath string, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "mock-create" {
		fmt.Fprintln(stderr, "usage: stola provisioner mock-create [--name NAME] [--plan PLAN]")
		return 2
	}
	fs := flag.NewFlagSet("mock-create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("name", "stola-nano", "mock VPS name")
	plan := fs.String("plan", "nano", "mock VPS plan")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	cfg, err := loadValidConfig(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "config failed: %v\n", err)
		return 1
	}
	store, logger, cleanup, err := openRuntime(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "runtime open failed: %v\n", err)
		return 1
	}
	defer cleanup()

	svc := provisioner.Service{State: store, Audit: logger, Back: provisioner.MockBackend{}}
	vps, err := svc.MockCreate(ctx, provisioner.CreateSpec{Name: *name, Plan: *plan})
	if err != nil {
		fmt.Fprintf(stderr, "mock create failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "created mock vps: id=%s name=%s plan=%s\n", vps.ID, vps.Name, vps.Plan)
	return 0
}

func cmdService(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) != 1 || args[0] != "status" {
		fmt.Fprintln(stderr, "usage: stola service status")
		return 2
	}
	units := []string{"stola-api.service", "stola-llm-proxy.service", "stola-provisioner.service", "stola-auditd.service"}
	for _, unit := range units {
		fmt.Fprintf(stdout, "%s unit_file_present=%t\n", unit, unitFilePresent(unit))
	}
	return 0
}

func unitFilePresent(unit string) bool {
	for depth := 0; depth < 6; depth++ {
		parts := make([]string, 0, depth+3)
		for i := 0; i < depth; i++ {
			parts = append(parts, "..")
		}
		parts = append(parts, "packaging", "systemd", unit)
		if _, err := os.Stat(filepath.Join(parts...)); err == nil {
			return true
		}
	}
	return false
}

func cmdDaemon(ctx context.Context, configPath string, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: stola daemon api|llm-proxy|provisioner|auditd [--once]")
		return 2
	}
	name := args[0]
	if name != "api" && name != "llm-proxy" && name != "provisioner" && name != "auditd" {
		fmt.Fprintf(stderr, "unknown daemon: %s\n", name)
		return 2
	}
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	fs.SetOutput(stderr)
	once := fs.Bool("once", false, "write one heartbeat and exit")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	cfg, err := loadValidConfig(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "config failed: %v\n", err)
		return 1
	}
	store, logger, cleanup, err := openRuntime(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "runtime open failed: %v\n", err)
		return 1
	}
	defer cleanup()

	serviceName := "stola-" + name
	if logger != nil {
		_ = logger.Record(audit.Event{
			Actor:   serviceName,
			Action:  "daemon.start",
			Target:  cfg.Node.ID,
			Outcome: "success",
			Details: map[string]any{"service": serviceName},
		})
	}
	if *once {
		if err := store.SetHeartbeat(ctx, serviceName, "running", time.Now().UTC()); err != nil {
			fmt.Fprintf(stderr, "heartbeat failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s heartbeat written\n", serviceName)
		return 0
	}

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if name == "api" {
		if err := store.SetHeartbeat(ctx, serviceName, "running", time.Now().UTC()); err != nil {
			fmt.Fprintf(stderr, "heartbeat failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s running listen=%s\n", serviceName, cfg.API.Listen)
		if err := controlplane.RunAPI(runCtx, cfg, store, logger); err != nil {
			fmt.Fprintf(stderr, "daemon failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s stopped\n", serviceName)
		return 0
	}
	if name == "llm-proxy" {
		if err := store.SetHeartbeat(ctx, serviceName, "running", time.Now().UTC()); err != nil {
			fmt.Fprintf(stderr, "heartbeat failed: %v\n", err)
			return 1
		}
		listen := strings.TrimSpace(cfg.LLMProxy.Listen)
		if listen == "" {
			listen = "127.0.0.1:8760"
		}
		fmt.Fprintf(stdout, "%s running listen=%s\n", serviceName, listen)
		if err := llmproxy.Run(runCtx, cfg, store, logger); err != nil {
			fmt.Fprintf(stderr, "daemon failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s stopped\n", serviceName)
		return 0
	}
	fmt.Fprintf(stdout, "%s running\n", serviceName)
	if err := provisioner.RunHeartbeat(runCtx, store, serviceName, 5*time.Second); err != nil {
		fmt.Fprintf(stderr, "daemon failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "%s stopped\n", serviceName)
	return 0
}

func loadValidConfig(path string) (config.Config, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func proxmoxClientFromConfig(cfg config.Config) (proxmox.Client, error) {
	if cfg.Backend.Type != "proxmox" {
		return proxmox.Client{}, fmt.Errorf("backend.type must be proxmox, got %q", cfg.Backend.Type)
	}
	client := proxmox.New(cfg.Backend.Proxmox)
	if err := client.Validate(); err != nil {
		return proxmox.Client{}, err
	}
	return client, nil
}

func dockerClientFromConfig(cfg config.Config) (dockerbackend.Client, error) {
	if cfg.Backend.Type != "docker" {
		return dockerbackend.Client{}, fmt.Errorf("backend.type must be docker, got %q", cfg.Backend.Type)
	}
	client := dockerbackend.New(cfg.Backend.Docker)
	if err := client.Validate(); err != nil {
		return dockerbackend.Client{}, err
	}
	return client, nil
}

func splitCSV(raw string) []string {
	items := strings.Split(raw, ",")
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func openRuntime(cfg config.Config) (*state.Store, *audit.Logger, func(), error) {
	store, err := state.Open(cfg.Paths.State)
	if err != nil {
		return nil, nil, func() {}, err
	}
	logger := audit.New(cfg.Paths.AuditLog)
	cleanup := func() {
		_ = store.Close()
	}
	return store, logger, cleanup, nil
}
