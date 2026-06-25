package controlplane

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"stola-os/internal/audit"
	"stola-os/internal/config"
	dockerbackend "stola-os/internal/docker"
	"stola-os/internal/mcp"
	"stola-os/internal/providers"
	"stola-os/internal/provisioner"
	"stola-os/internal/secrets"
	"stola-os/internal/state"
)

type APIServer struct {
	cfg        config.Config
	store      *state.Store
	logger     *audit.Logger
	auth       Authenticator
	signingKey []byte

	mu     sync.RWMutex
	policy PolicySnapshot
}

type PolicySnapshot struct {
	Version      string    `json:"version"`
	ManifestPath string    `json:"manifest_path"`
	PolicyFile   string    `json:"policy_file,omitempty"`
	LoadedAt     time.Time `json:"loaded_at"`
}

type ApprovalCreateRequest struct {
	VPSID          string   `json:"vps_id"`
	Server         string   `json:"server"`
	Tool           string   `json:"tool"`
	ArgumentDigest string   `json:"argument_digest"`
	ArgumentKeys   []string `json:"argument_keys,omitempty"`
	Reason         string   `json:"reason,omitempty"`
	Rule           string   `json:"rule,omitempty"`
	ExpiresIn      string   `json:"expires_in,omitempty"`
}

type ApprovalCreateResponse struct {
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

type ApprovalListResponse struct {
	Approvals []ApprovalView `json:"approvals"`
}

type WorkspaceCreateRequest struct {
	Name              string   `json:"name"`
	Plan              string   `json:"plan,omitempty"`
	Image             string   `json:"image,omitempty"`
	MemoryMB          int      `json:"memory_mb,omitempty"`
	SwapMB            int      `json:"swap_mb,omitempty"`
	Cores             int      `json:"cores,omitempty"`
	PIDsLimit         int      `json:"pids_limit,omitempty"`
	Provider          string   `json:"provider,omitempty"`
	SecretsFile       string   `json:"secrets_file,omitempty"`
	MCPManifest       string   `json:"mcp_manifest,omitempty"`
	ProvidersManifest string   `json:"providers_manifest,omitempty"`
	Start             bool     `json:"start,omitempty"`
	LLMProxyURL       string   `json:"llm_proxy_url,omitempty"`
	LLMProviderID     string   `json:"llm_provider_id,omitempty"`
	LLMModels         []string `json:"llm_models,omitempty"`
	LLMBudgetUSD      float64  `json:"llm_budget_usd,omitempty"`
	LLMRPM            int      `json:"llm_rpm,omitempty"`
	LLMTPM            int      `json:"llm_tpm,omitempty"`
	LLMMaxConcurrency int      `json:"llm_max_concurrency,omitempty"`
}

type WorkspaceCreateResponse struct {
	Workspace   WorkspaceView `json:"workspace"`
	SecretNames []string      `json:"secret_names,omitempty"`
	LLMKey      *LLMKeyView   `json:"llm_key,omitempty"`
}

type WorkspaceListResponse struct {
	Workspaces []WorkspaceView `json:"workspaces"`
}

type WorkspaceView struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Plan       string    `json:"plan"`
	Status     string    `json:"status"`
	Backend    string    `json:"backend"`
	CreatedAt  time.Time `json:"created_at"`
	ExternalID string    `json:"external_id,omitempty"`
	TaskID     string    `json:"task_id,omitempty"`
}

type UsageRecordRequest struct {
	WorkspaceID  string  `json:"workspace_id"`
	UserID       string  `json:"user_id,omitempty"`
	ProviderID   string  `json:"provider_id"`
	Model        string  `json:"model"`
	Operation    string  `json:"operation,omitempty"`
	InputTokens  int     `json:"input_tokens,omitempty"`
	OutputTokens int     `json:"output_tokens,omitempty"`
	TotalTokens  int     `json:"total_tokens,omitempty"`
	CostUSD      float64 `json:"cost_usd,omitempty"`
	LatencyMS    int     `json:"latency_ms,omitempty"`
	Status       string  `json:"status,omitempty"`
	Error        string  `json:"error,omitempty"`
}

type UsageRecordResponse struct {
	Usage UsageEventView `json:"usage"`
}

type UsageListResponse struct {
	Usage []UsageEventView `json:"usage"`
}

type UsageSummaryResponse struct {
	Summary UsageSummaryView `json:"summary"`
}

type UsageEventView struct {
	ID           string    `json:"id"`
	WorkspaceID  string    `json:"workspace_id"`
	UserID       string    `json:"user_id"`
	ProviderID   string    `json:"provider_id"`
	Model        string    `json:"model"`
	Operation    string    `json:"operation"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	TotalTokens  int       `json:"total_tokens"`
	CostUSD      float64   `json:"cost_usd"`
	LatencyMS    int       `json:"latency_ms"`
	Status       string    `json:"status"`
	Error        string    `json:"error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type UsageSummaryView struct {
	Events       int     `json:"events"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Errors       int     `json:"errors"`
}

type LLMKeyCreateRequest struct {
	WorkspaceID    string   `json:"workspace_id"`
	UserID         string   `json:"user_id,omitempty"`
	ProviderID     string   `json:"provider_id"`
	Models         []string `json:"models,omitempty"`
	BudgetUSD      float64  `json:"budget_usd,omitempty"`
	RPM            int      `json:"rpm,omitempty"`
	TPM            int      `json:"tpm,omitempty"`
	MaxConcurrency int      `json:"max_concurrency,omitempty"`
	ExpiresIn      string   `json:"expires_in,omitempty"`
}

type LLMKeyCreateResponse struct {
	Key   LLMKeyView `json:"key"`
	Token string     `json:"token"`
}

type LLMKeyRotateResponse struct {
	Key   LLMKeyView `json:"key"`
	Token string     `json:"token"`
}

type LLMKeyListResponse struct {
	Keys []LLMKeyView `json:"keys"`
}

type LLMKeyView struct {
	ID             string    `json:"id"`
	WorkspaceID    string    `json:"workspace_id"`
	UserID         string    `json:"user_id"`
	ProviderID     string    `json:"provider_id"`
	Models         []string  `json:"models,omitempty"`
	BudgetUSD      float64   `json:"budget_usd"`
	RPM            int       `json:"rpm"`
	TPM            int       `json:"tpm"`
	MaxConcurrency int       `json:"max_concurrency"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at,omitempty"`
	LastUsedAt     time.Time `json:"last_used_at,omitempty"`
	SpendUSD       float64   `json:"spend_usd"`
}

type ApprovalView struct {
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

func NewAPIServer(cfg config.Config, store *state.Store, logger *audit.Logger) (*APIServer, error) {
	if strings.TrimSpace(cfg.API.UsersFile) == "" {
		return nil, errors.New("api.users_file is required")
	}
	auth, err := LoadAuthenticator(cfg.API.UsersFile)
	if err != nil {
		return nil, fmt.Errorf("load api users: %w", err)
	}
	signingKey, err := LoadOrCreateSigningKey(cfg.API.SigningKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load approval signing key: %w", err)
	}
	server := &APIServer{
		cfg:        cfg,
		store:      store,
		logger:     logger,
		auth:       auth,
		signingKey: signingKey,
	}
	if _, err := server.reloadPolicy(); err != nil {
		return nil, fmt.Errorf("load policy: %w", err)
	}
	return server, nil
}

func RunAPI(ctx context.Context, cfg config.Config, store *state.Store, logger *audit.Logger) error {
	server, err := NewAPIServer(cfg, store, logger)
	if err != nil {
		return err
	}
	httpServer := &http.Server{
		Addr:              cfg.API.Listen,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	err = httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *APIServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /v1/policy", s.require(PermissionPolicyRead, s.handlePolicyGet))
	mux.HandleFunc("POST /v1/policy/reload", s.require(PermissionPolicyReload, s.handlePolicyReload))
	mux.HandleFunc("GET /v1/workspaces", s.require(PermissionWorkspaceRead, s.handleWorkspaceList))
	mux.HandleFunc("POST /v1/workspaces", s.require(PermissionWorkspaceCreate, s.handleWorkspaceCreate))
	mux.HandleFunc("GET /v1/workspaces/{id}", s.require(PermissionWorkspaceRead, s.handleWorkspaceGet))
	mux.HandleFunc("GET /v1/usage", s.require(PermissionUsageRead, s.handleUsageList))
	mux.HandleFunc("POST /v1/usage", s.require(PermissionUsageWrite, s.handleUsageRecord))
	mux.HandleFunc("GET /v1/usage/summary", s.require(PermissionUsageRead, s.handleUsageSummary))
	mux.HandleFunc("GET /v1/llm/keys", s.require(PermissionLLMKeyRead, s.handleLLMKeyList))
	mux.HandleFunc("POST /v1/llm/keys", s.require(PermissionLLMKeyCreate, s.handleLLMKeyCreate))
	mux.HandleFunc("POST /v1/llm/keys/{id}/rotate", s.require(PermissionLLMKeyRotate, s.handleLLMKeyRotate))
	mux.HandleFunc("POST /v1/llm/keys/{id}/revoke", s.require(PermissionLLMKeyRevoke, s.handleLLMKeyRevoke))
	mux.HandleFunc("GET /v1/mcp/approvals", s.require(PermissionMCPApprove, s.handleApprovalList))
	mux.HandleFunc("POST /v1/mcp/approvals", s.require(PermissionMCPApprove, s.handleApprovalCreate))
	mux.HandleFunc("POST /v1/mcp/approvals/{id}/approve", s.require(PermissionMCPApprove, s.handleApprovalApprove))
	return mux
}

func (s *APIServer) PolicySnapshot() PolicySnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.policy
}

func (s *APIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *APIServer) handlePolicyGet(w http.ResponseWriter, r *http.Request, principal Principal) {
	writeJSON(w, http.StatusOK, s.PolicySnapshot())
}

func (s *APIServer) handlePolicyReload(w http.ResponseWriter, r *http.Request, principal Principal) {
	snapshot, err := s.reloadPolicy()
	if err != nil {
		recordAPIAudit(s.logger, principal.ID, "api.policy.reload", "policy", "failure", map[string]any{"error": err.Error()})
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	recordAPIAudit(s.logger, principal.ID, "api.policy.reload", "policy", "success", map[string]any{"policy_version": snapshot.Version})
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *APIServer) handleWorkspaceList(w http.ResponseWriter, r *http.Request, principal Principal) {
	items, err := s.store.ListVPS(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "workspace list failed")
		return
	}
	out := WorkspaceListResponse{Workspaces: make([]WorkspaceView, 0, len(items))}
	for _, item := range items {
		out.Workspaces = append(out.Workspaces, workspaceView(item))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *APIServer) handleWorkspaceGet(w http.ResponseWriter, r *http.Request, principal Principal) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "workspace id is required")
		return
	}
	item, err := s.store.GetVPS(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "workspace not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "workspace lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, workspaceView(item))
}

func (s *APIServer) handleWorkspaceCreate(w http.ResponseWriter, r *http.Request, principal Principal) {
	var req WorkspaceCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if strings.TrimSpace(req.Plan) == "" {
		req.Plan = "nano"
	}
	if req.MemoryMB == 0 {
		req.MemoryMB = 1024
	}
	if req.Cores == 0 {
		req.Cores = 1
	}
	if req.PIDsLimit == 0 {
		req.PIDsLimit = 256
	}
	llmProxyURL := strings.TrimSpace(req.LLMProxyURL)
	if llmProxyURL != "" {
		if s.cfg.Backend.Type != "docker" {
			writeError(w, http.StatusBadRequest, "llm_proxy_url is supported only with backend.type=docker")
			return
		}
		if req.LLMProviderID == "" {
			req.LLMProviderID = req.Provider
		}
		if strings.TrimSpace(req.LLMProviderID) == "" {
			req.LLMProviderID = "openrouter"
		}
		if req.LLMBudgetUSD < 0 || req.LLMRPM < 0 || req.LLMTPM < 0 || req.LLMMaxConcurrency < 0 {
			writeError(w, http.StatusBadRequest, "llm budget, rpm, tpm, and max_concurrency must not be negative")
			return
		}
	}

	var secretNames []string
	if strings.TrimSpace(req.SecretsFile) != "" {
		if s.cfg.Backend.Type != "docker" {
			writeError(w, http.StatusBadRequest, "secrets_file is supported only with backend.type=docker")
			return
		}
		bundle, providerManifest, err := loadWorkspaceSecrets(req.SecretsFile, req.ProvidersManifest, req.Provider)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		req.ProvidersManifest = providerManifest
		secretNames = secrets.Names(bundle.Secrets)
	}

	backend, err := s.workspaceBackend()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	spec := provisioner.CreateSpec{
		Name:              req.Name,
		Plan:              req.Plan,
		Image:             req.Image,
		MemoryMB:          req.MemoryMB,
		SwapMB:            req.SwapMB,
		Cores:             req.Cores,
		PIDsLimit:         req.PIDsLimit,
		InjectMCP:         s.cfg.Backend.Type == "docker",
		MCPManifest:       req.MCPManifest,
		InjectProviders:   s.cfg.Backend.Type == "docker",
		ProvidersManifest: req.ProvidersManifest,
		InjectSecrets:     s.cfg.Backend.Type == "docker" && strings.TrimSpace(req.SecretsFile) != "",
		SecretsFile:       req.SecretsFile,
		SecretNames:       secretNames,
		Start:             req.Start,
		Unprivileged:      true,
	}
	workspace, err := (provisioner.Service{State: s.store, Audit: s.logger, Back: backend}).Create(r.Context(), spec)
	if err != nil {
		recordAPIAudit(s.logger, principal.ID, "api.workspace.create", req.Name, "failure", map[string]any{"provider": req.Provider, "error": err.Error()})
		writeError(w, http.StatusInternalServerError, "workspace create failed")
		return
	}
	var llmKeyView *LLMKeyView
	if llmProxyURL != "" {
		key, rawKey, err := s.createLLMProxyKey(r.Context(), principal, LLMKeyCreateRequest{
			WorkspaceID:    workspace.ID,
			UserID:         principal.ID,
			ProviderID:     req.LLMProviderID,
			Models:         req.LLMModels,
			BudgetUSD:      req.LLMBudgetUSD,
			RPM:            req.LLMRPM,
			TPM:            req.LLMTPM,
			MaxConcurrency: req.LLMMaxConcurrency,
		})
		if err != nil {
			recordAPIAudit(s.logger, principal.ID, "api.workspace.llm_proxy", workspace.ID, "failure", map[string]any{"provider": req.LLMProviderID, "error": err.Error()})
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		dockerClient, ok := backend.(dockerbackend.Client)
		if !ok {
			_, _ = s.store.RevokeLLMProxyKey(r.Context(), key.ID)
			writeError(w, http.StatusInternalServerError, "docker backend unavailable for llm proxy injection")
			return
		}
		if err := dockerClient.InjectLLMProxy(r.Context(), workspace, dockerbackend.LLMProxyInjection{
			URL:        llmProxyURL,
			Token:      rawKey,
			ProviderID: key.ProviderID,
			Models:     key.Models,
			KeyID:      key.ID,
		}); err != nil {
			_, _ = s.store.RevokeLLMProxyKey(r.Context(), key.ID)
			recordAPIAudit(s.logger, principal.ID, "api.workspace.llm_proxy", workspace.ID, "failure", map[string]any{"provider": key.ProviderID, "key_id": key.ID, "error": err.Error()})
			writeError(w, http.StatusInternalServerError, "llm proxy injection failed")
			return
		}
		view := s.llmKeyView(r.Context(), key)
		llmKeyView = &view
		recordAPIAudit(s.logger, principal.ID, "api.workspace.llm_proxy", workspace.ID, "success", map[string]any{
			"provider":        key.ProviderID,
			"models":          key.Models,
			"key_id":          key.ID,
			"budget_usd":      key.BudgetUSD,
			"rpm":             key.RPM,
			"tpm":             key.TPM,
			"max_concurrency": key.MaxConcurrency,
		})
	}
	recordAPIAudit(s.logger, principal.ID, "api.workspace.create", workspace.ID, "success", map[string]any{
		"name":         workspace.Name,
		"plan":         workspace.Plan,
		"backend":      workspace.Backend,
		"provider":     req.Provider,
		"secret_names": secretNames,
	})
	writeJSON(w, http.StatusCreated, WorkspaceCreateResponse{Workspace: workspaceView(workspace), SecretNames: secretNames, LLMKey: llmKeyView})
}

func (s *APIServer) handleUsageRecord(w http.ResponseWriter, r *http.Request, principal Principal) {
	var req UsageRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.WorkspaceID = strings.TrimSpace(req.WorkspaceID)
	req.UserID = strings.TrimSpace(req.UserID)
	req.ProviderID = strings.TrimSpace(req.ProviderID)
	req.Model = strings.TrimSpace(req.Model)
	if req.UserID == "" {
		req.UserID = principal.ID
	}
	if req.WorkspaceID == "" || req.ProviderID == "" || req.Model == "" {
		writeError(w, http.StatusBadRequest, "workspace_id, provider_id, and model are required")
		return
	}
	if _, err := s.store.GetVPS(r.Context(), req.WorkspaceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "workspace not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "workspace lookup failed")
		return
	}
	event, err := s.store.RecordUsage(r.Context(), state.UsageEvent{
		WorkspaceID:  req.WorkspaceID,
		UserID:       req.UserID,
		ProviderID:   req.ProviderID,
		Model:        req.Model,
		Operation:    req.Operation,
		InputTokens:  req.InputTokens,
		OutputTokens: req.OutputTokens,
		TotalTokens:  req.TotalTokens,
		CostUSD:      req.CostUSD,
		LatencyMS:    req.LatencyMS,
		Status:       req.Status,
		Error:        req.Error,
	})
	if err != nil {
		recordAPIAudit(s.logger, principal.ID, "api.usage.record", req.WorkspaceID, "failure", map[string]any{"provider": req.ProviderID, "model": req.Model, "error": err.Error()})
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	recordAPIAudit(s.logger, principal.ID, "api.usage.record", event.WorkspaceID, "success", map[string]any{
		"user_id":      event.UserID,
		"provider":     event.ProviderID,
		"model":        event.Model,
		"operation":    event.Operation,
		"total_tokens": event.TotalTokens,
		"cost_usd":     event.CostUSD,
		"latency_ms":   event.LatencyMS,
		"status":       event.Status,
	})
	writeJSON(w, http.StatusCreated, UsageRecordResponse{Usage: usageView(event)})
}

func (s *APIServer) handleUsageList(w http.ResponseWriter, r *http.Request, principal Principal) {
	filter, ok := usageFilterFromRequest(w, r)
	if !ok {
		return
	}
	items, err := s.store.ListUsage(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "usage list failed")
		return
	}
	out := UsageListResponse{Usage: make([]UsageEventView, 0, len(items))}
	for _, item := range items {
		out.Usage = append(out.Usage, usageView(item))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *APIServer) handleUsageSummary(w http.ResponseWriter, r *http.Request, principal Principal) {
	filter, ok := usageFilterFromRequest(w, r)
	if !ok {
		return
	}
	summary, err := s.store.SummarizeUsage(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "usage summary failed")
		return
	}
	writeJSON(w, http.StatusOK, UsageSummaryResponse{Summary: usageSummaryView(summary)})
}

func (s *APIServer) handleLLMKeyCreate(w http.ResponseWriter, r *http.Request, principal Principal) {
	var req LLMKeyCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	key, rawKey, err := s.createLLMProxyKey(r.Context(), principal, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	recordAPIAudit(s.logger, principal.ID, "api.llm.key.create", key.ID, "success", map[string]any{
		"workspace_id":    key.WorkspaceID,
		"user_id":         key.UserID,
		"provider":        key.ProviderID,
		"models":          key.Models,
		"budget_usd":      key.BudgetUSD,
		"rpm":             key.RPM,
		"tpm":             key.TPM,
		"max_concurrency": key.MaxConcurrency,
	})
	writeJSON(w, http.StatusCreated, LLMKeyCreateResponse{Key: s.llmKeyView(r.Context(), key), Token: rawKey})
}

func (s *APIServer) createLLMProxyKey(ctx context.Context, principal Principal, req LLMKeyCreateRequest) (state.LLMProxyKey, string, error) {
	req.WorkspaceID = strings.TrimSpace(req.WorkspaceID)
	req.UserID = strings.TrimSpace(req.UserID)
	req.ProviderID = strings.TrimSpace(req.ProviderID)
	if req.UserID == "" {
		req.UserID = principal.ID
	}
	if req.WorkspaceID == "" || req.ProviderID == "" {
		return state.LLMProxyKey{}, "", errors.New("workspace_id and provider_id are required")
	}
	if req.BudgetUSD < 0 || req.RPM < 0 || req.TPM < 0 || req.MaxConcurrency < 0 {
		return state.LLMProxyKey{}, "", errors.New("budget_usd, rpm, tpm, and max_concurrency must not be negative")
	}
	if _, err := s.store.GetVPS(ctx, req.WorkspaceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return state.LLMProxyKey{}, "", errors.New("workspace not found")
		}
		return state.LLMProxyKey{}, "", errors.New("workspace lookup failed")
	}
	expiresAt := time.Time{}
	if raw := strings.TrimSpace(req.ExpiresIn); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err != nil || ttl <= 0 {
			return state.LLMProxyKey{}, "", errors.New("expires_in must be a positive Go duration")
		}
		expiresAt = time.Now().UTC().Add(ttl)
	}
	rawKey, err := state.GenerateLLMProxyKey()
	if err != nil {
		return state.LLMProxyKey{}, "", errors.New("llm key generation failed")
	}
	key, err := s.store.CreateLLMProxyKey(ctx, state.LLMProxyKey{
		KeyHash:        state.HashLLMProxyKey(rawKey),
		WorkspaceID:    req.WorkspaceID,
		UserID:         req.UserID,
		ProviderID:     req.ProviderID,
		Models:         req.Models,
		BudgetUSD:      req.BudgetUSD,
		RPM:            req.RPM,
		TPM:            req.TPM,
		MaxConcurrency: req.MaxConcurrency,
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		recordAPIAudit(s.logger, principal.ID, "api.llm.key.create", req.WorkspaceID, "failure", map[string]any{"provider": req.ProviderID, "error": err.Error()})
		return state.LLMProxyKey{}, "", err
	}
	return key, rawKey, nil
}

func (s *APIServer) handleLLMKeyList(w http.ResponseWriter, r *http.Request, principal Principal) {
	filter, ok := llmKeyFilterFromRequest(w, r)
	if !ok {
		return
	}
	keys, err := s.store.ListLLMProxyKeys(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "llm key list failed")
		return
	}
	out := LLMKeyListResponse{Keys: make([]LLMKeyView, 0, len(keys))}
	for _, key := range keys {
		out.Keys = append(out.Keys, s.llmKeyView(r.Context(), key))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *APIServer) handleLLMKeyRotate(w http.ResponseWriter, r *http.Request, principal Principal) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "llm key id is required")
		return
	}
	rawKey, err := state.GenerateLLMProxyKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "llm key generation failed")
		return
	}
	key, err := s.store.RotateLLMProxyKey(r.Context(), id, state.HashLLMProxyKey(rawKey), time.Now().UTC())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "llm key not found")
			return
		}
		recordAPIAudit(s.logger, principal.ID, "api.llm.key.rotate", id, "failure", map[string]any{"error": err.Error()})
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	recordAPIAudit(s.logger, principal.ID, "api.llm.key.rotate", key.ID, "success", map[string]any{
		"workspace_id": key.WorkspaceID,
		"user_id":      key.UserID,
		"provider":     key.ProviderID,
	})
	writeJSON(w, http.StatusOK, LLMKeyRotateResponse{Key: s.llmKeyView(r.Context(), key), Token: rawKey})
}

func (s *APIServer) handleLLMKeyRevoke(w http.ResponseWriter, r *http.Request, principal Principal) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "llm key id is required")
		return
	}
	key, err := s.store.RevokeLLMProxyKey(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "llm key not found")
			return
		}
		recordAPIAudit(s.logger, principal.ID, "api.llm.key.revoke", id, "failure", map[string]any{"error": err.Error()})
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	recordAPIAudit(s.logger, principal.ID, "api.llm.key.revoke", key.ID, "success", map[string]any{
		"workspace_id": key.WorkspaceID,
		"user_id":      key.UserID,
		"provider":     key.ProviderID,
	})
	writeJSON(w, http.StatusOK, s.llmKeyView(r.Context(), key))
}

func (s *APIServer) handleApprovalList(w http.ResponseWriter, r *http.Request, principal Principal) {
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = parsed
	}
	approvals, err := s.store.ListMCPApprovals(r.Context(), state.MCPApprovalFilter{
		Status: r.URL.Query().Get("status"),
		Limit:  limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "approval list failed")
		return
	}
	out := ApprovalListResponse{Approvals: make([]ApprovalView, 0, len(approvals))}
	for _, approval := range approvals {
		out.Approvals = append(out.Approvals, approvalView(approval))
	}
	writeJSON(w, http.StatusOK, out)
}

func workspaceView(v state.VPS) WorkspaceView {
	return WorkspaceView{
		ID:         v.ID,
		Name:       v.Name,
		Plan:       v.Plan,
		Status:     v.Status,
		Backend:    v.Backend,
		CreatedAt:  v.CreatedAt,
		ExternalID: v.ExternalID,
		TaskID:     v.TaskID,
	}
}

func usageView(event state.UsageEvent) UsageEventView {
	return UsageEventView{
		ID:           event.ID,
		WorkspaceID:  event.WorkspaceID,
		UserID:       event.UserID,
		ProviderID:   event.ProviderID,
		Model:        event.Model,
		Operation:    event.Operation,
		InputTokens:  event.InputTokens,
		OutputTokens: event.OutputTokens,
		TotalTokens:  event.TotalTokens,
		CostUSD:      event.CostUSD,
		LatencyMS:    event.LatencyMS,
		Status:       event.Status,
		Error:        event.Error,
		CreatedAt:    event.CreatedAt,
	}
}

func usageSummaryView(summary state.UsageSummary) UsageSummaryView {
	return UsageSummaryView{
		Events:       summary.Events,
		InputTokens:  summary.InputTokens,
		OutputTokens: summary.OutputTokens,
		TotalTokens:  summary.TotalTokens,
		CostUSD:      summary.CostUSD,
		Errors:       summary.Errors,
	}
}

func (s *APIServer) llmKeyView(ctx context.Context, key state.LLMProxyKey) LLMKeyView {
	summary, _ := s.store.SummarizeUsage(ctx, state.UsageFilter{KeyID: key.ID})
	return LLMKeyView{
		ID:             key.ID,
		WorkspaceID:    key.WorkspaceID,
		UserID:         key.UserID,
		ProviderID:     key.ProviderID,
		Models:         append([]string(nil), key.Models...),
		BudgetUSD:      key.BudgetUSD,
		RPM:            key.RPM,
		TPM:            key.TPM,
		MaxConcurrency: key.MaxConcurrency,
		Status:         key.Status,
		CreatedAt:      key.CreatedAt,
		ExpiresAt:      key.ExpiresAt,
		LastUsedAt:     key.LastUsedAt,
		SpendUSD:       summary.CostUSD,
	}
}

func usageFilterFromRequest(w http.ResponseWriter, r *http.Request) (state.UsageFilter, bool) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return state.UsageFilter{}, false
		}
		limit = parsed
	}
	return state.UsageFilter{
		WorkspaceID: r.URL.Query().Get("workspace_id"),
		UserID:      r.URL.Query().Get("user_id"),
		ProviderID:  r.URL.Query().Get("provider_id"),
		Limit:       limit,
	}, true
}

func llmKeyFilterFromRequest(w http.ResponseWriter, r *http.Request) (state.LLMProxyKeyFilter, bool) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return state.LLMProxyKeyFilter{}, false
		}
		limit = parsed
	}
	return state.LLMProxyKeyFilter{
		WorkspaceID: r.URL.Query().Get("workspace_id"),
		UserID:      r.URL.Query().Get("user_id"),
		ProviderID:  r.URL.Query().Get("provider_id"),
		Status:      r.URL.Query().Get("status"),
		Limit:       limit,
	}, true
}

func approvalView(approval state.MCPApproval) ApprovalView {
	return ApprovalView{
		ID:             approval.ID,
		VPSID:          approval.VPSID,
		Server:         approval.Server,
		Tool:           approval.Tool,
		ArgumentDigest: approval.ArgumentDigest,
		ArgumentKeys:   append([]string(nil), approval.ArgumentKeys...),
		Status:         approval.Status,
		CreatedAt:      approval.CreatedAt,
		ExpiresAt:      approval.ExpiresAt,
		UsedAt:         approval.UsedAt,
		Reason:         approval.Reason,
		Rule:           approval.Rule,
		ApprovedBy:     approval.ApprovedBy,
		PolicyVersion:  approval.PolicyVersion,
	}
}

func (s *APIServer) workspaceBackend() (provisioner.Backend, error) {
	switch s.cfg.Backend.Type {
	case "mock":
		return provisioner.MockBackend{}, nil
	case "docker":
		client := dockerbackend.New(s.cfg.Backend.Docker)
		if err := client.Validate(); err != nil {
			return nil, err
		}
		return client, nil
	default:
		return nil, fmt.Errorf("workspace API supports backend.type=docker or mock, got %q", s.cfg.Backend.Type)
	}
}

func loadWorkspaceSecrets(secretsFile string, providersManifest string, providerID string) (secrets.Bundle, string, error) {
	manifestPath := providers.ResolveManifestPath(providersManifest)
	manifest, _, err := providers.LoadFile(manifestPath)
	if err != nil {
		return secrets.Bundle{}, manifestPath, fmt.Errorf("provider manifest invalid: %w", err)
	}
	bundle, err := secrets.LoadProviderEnvFile(secretsFile, providers.EnvNames(manifest))
	if err != nil {
		return secrets.Bundle{}, manifestPath, fmt.Errorf("secret file invalid: %w", err)
	}
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return bundle, manifestPath, nil
	}
	provider, ok := providers.FindProvider(manifest, providerID)
	if !ok {
		return secrets.Bundle{}, manifestPath, fmt.Errorf("provider %q not found", providerID)
	}
	if !provider.Enabled {
		return secrets.Bundle{}, manifestPath, fmt.Errorf("provider %q is disabled", providerID)
	}
	if strings.TrimSpace(provider.APIKeyEnv) == "" {
		return secrets.Bundle{}, manifestPath, fmt.Errorf("provider %q has no api_key_env", providerID)
	}
	if _, ok := secrets.ValuesByName(bundle.Secrets)[provider.APIKeyEnv]; !ok {
		return secrets.Bundle{}, manifestPath, fmt.Errorf("provider %q requires secret %s", providerID, provider.APIKeyEnv)
	}
	return bundle, manifestPath, nil
}

func (s *APIServer) handleApprovalCreate(w http.ResponseWriter, r *http.Request, principal Principal) {
	var req ApprovalCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.VPSID = strings.TrimSpace(req.VPSID)
	req.Server = strings.TrimSpace(req.Server)
	req.Tool = strings.TrimSpace(req.Tool)
	req.ArgumentDigest = strings.TrimSpace(req.ArgumentDigest)
	if req.VPSID == "" || req.Server == "" || req.Tool == "" || req.ArgumentDigest == "" {
		writeError(w, http.StatusBadRequest, "vps_id, server, tool, and argument_digest are required")
		return
	}
	if _, err := s.store.GetVPS(r.Context(), req.VPSID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "vps not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "vps lookup failed")
		return
	}
	ttl := 10 * time.Minute
	if strings.TrimSpace(req.ExpiresIn) != "" {
		parsed, err := time.ParseDuration(req.ExpiresIn)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "expires_in must be a positive Go duration")
			return
		}
		ttl = parsed
	}
	if ttl > time.Hour {
		writeError(w, http.StatusBadRequest, "expires_in must be <= 1h")
		return
	}
	now := time.Now().UTC()
	snapshot := s.PolicySnapshot()
	approval, err := s.store.CreateMCPApproval(r.Context(), state.MCPApproval{
		VPSID:          req.VPSID,
		Server:         req.Server,
		Tool:           req.Tool,
		ArgumentDigest: req.ArgumentDigest,
		ArgumentKeys:   req.ArgumentKeys,
		CreatedAt:      now,
		ExpiresAt:      now.Add(ttl),
		Reason:         req.Reason,
		Rule:           req.Rule,
		ApprovedBy:     principal.ID,
		PolicyVersion:  snapshot.Version,
	})
	if err != nil {
		recordAPIAudit(s.logger, principal.ID, "api.mcp.approval.create", req.VPSID, "failure", map[string]any{"server": req.Server, "tool": req.Tool, "error": err.Error()})
		writeError(w, http.StatusInternalServerError, "approval create failed")
		return
	}
	payload := ApprovalPayload{
		ApprovalID:     approval.ID,
		VPSID:          approval.VPSID,
		Server:         approval.Server,
		Tool:           approval.Tool,
		ArgumentDigest: approval.ArgumentDigest,
		ArgumentKeys:   approval.ArgumentKeys,
		UserID:         principal.ID,
		PolicyVersion:  snapshot.Version,
		ExpiresAt:      approval.ExpiresAt,
	}
	token, err := SignApproval(s.signingKey, payload)
	if err != nil {
		recordAPIAudit(s.logger, principal.ID, "api.mcp.approval.sign", req.VPSID, "failure", map[string]any{"server": req.Server, "tool": req.Tool, "approval_id": approval.ID, "error": err.Error()})
		writeError(w, http.StatusInternalServerError, "approval sign failed")
		return
	}
	recordAPIAudit(s.logger, principal.ID, "api.mcp.approval.create", approval.VPSID, "success", map[string]any{
		"server":         approval.Server,
		"tool":           approval.Tool,
		"approval_id":    approval.ID,
		"policy_version": snapshot.Version,
		"argument_keys":  approval.ArgumentKeys,
		"expires_at":     approval.ExpiresAt.Format(time.RFC3339Nano),
	})
	writeJSON(w, http.StatusCreated, ApprovalCreateResponse{
		ApprovalID:     approval.ID,
		ApprovalToken:  token,
		VPSID:          approval.VPSID,
		Server:         approval.Server,
		Tool:           approval.Tool,
		ArgumentDigest: approval.ArgumentDigest,
		ArgumentKeys:   approval.ArgumentKeys,
		ApprovedBy:     principal.ID,
		PolicyVersion:  snapshot.Version,
		ExpiresAt:      approval.ExpiresAt,
	})
}

func (s *APIServer) handleApprovalApprove(w http.ResponseWriter, r *http.Request, principal Principal) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "approval id is required")
		return
	}
	snapshot := s.PolicySnapshot()
	approval, err := s.store.ApproveMCPApproval(r.Context(), id, principal.ID, snapshot.Version, time.Now().UTC())
	if err != nil {
		recordAPIAudit(s.logger, principal.ID, "api.mcp.approval.approve", id, "denied", map[string]any{"error": err.Error()})
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	payload := ApprovalPayload{
		ApprovalID:     approval.ID,
		VPSID:          approval.VPSID,
		Server:         approval.Server,
		Tool:           approval.Tool,
		ArgumentDigest: approval.ArgumentDigest,
		ArgumentKeys:   approval.ArgumentKeys,
		UserID:         principal.ID,
		PolicyVersion:  snapshot.Version,
		ExpiresAt:      approval.ExpiresAt,
	}
	token, err := SignApproval(s.signingKey, payload)
	if err != nil {
		recordAPIAudit(s.logger, principal.ID, "api.mcp.approval.sign", approval.VPSID, "failure", map[string]any{"server": approval.Server, "tool": approval.Tool, "approval_id": approval.ID, "error": err.Error()})
		writeError(w, http.StatusInternalServerError, "approval sign failed")
		return
	}
	recordAPIAudit(s.logger, principal.ID, "api.mcp.approval.approve", approval.VPSID, "success", map[string]any{
		"server":         approval.Server,
		"tool":           approval.Tool,
		"approval_id":    approval.ID,
		"policy_version": snapshot.Version,
		"argument_keys":  approval.ArgumentKeys,
		"expires_at":     approval.ExpiresAt.Format(time.RFC3339Nano),
	})
	writeJSON(w, http.StatusOK, ApprovalCreateResponse{
		ApprovalID:     approval.ID,
		ApprovalToken:  token,
		VPSID:          approval.VPSID,
		Server:         approval.Server,
		Tool:           approval.Tool,
		ArgumentDigest: approval.ArgumentDigest,
		ArgumentKeys:   approval.ArgumentKeys,
		ApprovedBy:     principal.ID,
		PolicyVersion:  snapshot.Version,
		ExpiresAt:      approval.ExpiresAt,
	})
}

func (s *APIServer) reloadPolicy() (PolicySnapshot, error) {
	manifestPath := mcp.ResolveManifestPath(s.cfg.API.MCPManifest)
	policyFile := mcp.ResolvePolicyPath(s.cfg.API.PolicyFile)
	_, _, version, err := mcp.LoadEffectiveFile(manifestPath, policyFile)
	if err != nil {
		return PolicySnapshot{}, err
	}
	snapshot := PolicySnapshot{
		Version:      version,
		ManifestPath: manifestPath,
		PolicyFile:   policyFile,
		LoadedAt:     time.Now().UTC(),
	}
	s.mu.Lock()
	s.policy = snapshot
	s.mu.Unlock()
	return snapshot, nil
}

func (s *APIServer) require(permission string, next func(http.ResponseWriter, *http.Request, Principal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, err := s.auth.AuthenticateBearer(r.Header.Get("Authorization"))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if !principal.HasPermission(permission) {
			recordAPIAudit(s.logger, principal.ID, "api.authz", permission, "denied", map[string]any{"path": r.URL.Path})
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		next(w, r, principal)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}

func recordAPIAudit(logger *audit.Logger, actor string, action string, target string, outcome string, details map[string]any) {
	if logger == nil {
		return
	}
	_ = logger.Record(audit.Event{
		Actor:   actor,
		Action:  action,
		Target:  target,
		Outcome: outcome,
		Details: details,
	})
}
