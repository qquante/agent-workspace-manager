package llmproxy

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"stola-os/internal/audit"
	"stola-os/internal/config"
	"stola-os/internal/providers"
	"stola-os/internal/secrets"
	"stola-os/internal/state"
)

const defaultListen = "127.0.0.1:8760"

type Server struct {
	cfg        config.Config
	store      *state.Store
	logger     *audit.Logger
	manifest   providers.Manifest
	secretVals map[string]string
	httpClient *http.Client
	mu         sync.Mutex
	active     map[string]int
}

func NewServer(cfg config.Config, store *state.Store, logger *audit.Logger) (*Server, error) {
	manifestPath := providers.ResolveManifestPath(cfg.LLMProxy.ProvidersManifest)
	manifest, _, err := providers.LoadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("load llm proxy providers manifest: %w", err)
	}
	if strings.TrimSpace(cfg.LLMProxy.SecretsFile) == "" {
		return nil, errors.New("llm_proxy.secrets_file is required")
	}
	bundle, err := secrets.LoadProviderEnvFile(cfg.LLMProxy.SecretsFile, providers.EnvNames(manifest))
	if err != nil {
		return nil, fmt.Errorf("load llm proxy secrets: %w", err)
	}
	return &Server{
		cfg:        cfg,
		store:      store,
		logger:     logger,
		manifest:   manifest,
		secretVals: secrets.ValuesByName(bundle.Secrets),
		httpClient: &http.Client{Timeout: 60 * time.Second},
		active:     map[string]int{},
	}, nil
}

func Run(ctx context.Context, cfg config.Config, store *state.Store, logger *audit.Logger) error {
	server, err := NewServer(cfg, store, logger)
	if err != nil {
		return err
	}
	listen := strings.TrimSpace(cfg.LLMProxy.Listen)
	if listen == "" {
		listen = defaultListen
	}
	httpServer := &http.Server{
		Addr:              listen,
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

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /v1/models", s.handleModels)
	mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("POST /chat/completions", s.handleChatCompletions)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	key, ok := s.authorize(w, r)
	if !ok {
		return
	}
	data := make([]map[string]any, 0, len(key.Models))
	for _, model := range key.Models {
		data = append(data, map[string]any{"id": model, "object": "model"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	key, ok := s.authorize(w, r)
	if !ok {
		return
	}
	provider, err := s.providerForKey(key)
	if err != nil {
		s.writeProxyError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.allowedByBudget(r.Context(), w, key) {
		return
	}
	if !s.allowedByRPM(r.Context(), w, key) {
		return
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, 8*1024*1024))
	if err != nil {
		s.writeProxyError(w, http.StatusBadRequest, "request body read failed")
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		s.writeProxyError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	model, _ := payload["model"].(string)
	model = strings.TrimSpace(model)
	if model == "" {
		s.writeProxyError(w, http.StatusBadRequest, "model is required")
		return
	}
	if !modelAllowed(key, model) {
		record(s.logger, "stola-llm-proxy", "llm_proxy.model", key.ID, "denied", map[string]any{"workspace_id": key.WorkspaceID, "provider": key.ProviderID, "model": model})
		s.writeProxyError(w, http.StatusForbidden, "model is not allowed for this key")
		return
	}
	estimatedTokens := estimateRequestTokens(payload)
	if !s.allowedByTPM(r.Context(), w, key, estimatedTokens) {
		return
	}
	release, ok := s.acquireConcurrency(w, key)
	if !ok {
		return
	}
	defer release()

	upstreamModel := stripProviderPrefix(key.ProviderID, model)
	payload["model"] = upstreamModel
	stream, _ := payload["stream"].(bool)
	if stream {
		ensureStreamUsageOptions(payload)
	}
	forwardBody, err := json.Marshal(payload)
	if err != nil {
		s.writeProxyError(w, http.StatusBadRequest, "request body encode failed")
		return
	}
	respStatus, respBody, contentType, err := s.forward(r.Context(), provider, forwardBody, stream)
	latency := time.Since(start)
	if err != nil {
		s.recordUsage(r.Context(), key, model, "error", err.Error(), usageFields{}, latency)
		s.writeProxyError(w, http.StatusBadGateway, "upstream request failed")
		return
	}
	if contentType == "" {
		if stream {
			contentType = "text/event-stream"
		} else {
			contentType = "application/json"
		}
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(respStatus)
	_, _ = w.Write(respBody)

	usage := parseUsage(respBody, model)
	if stream {
		usage = parseStreamingUsage(respBody, model, estimatedTokens)
	}
	status := "success"
	errorText := ""
	if respStatus < 200 || respStatus >= 300 {
		status = "error"
		errorText = fmt.Sprintf("upstream_http_%d", respStatus)
	}
	s.recordUsage(r.Context(), key, model, status, errorText, usage, latency)
	_ = s.store.TouchLLMProxyKeyUsedAt(r.Context(), key.ID, time.Now().UTC())
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) (state.LLMProxyKey, bool) {
	raw := bearerToken(r.Header.Get("Authorization"))
	if raw == "" {
		s.writeProxyError(w, http.StatusUnauthorized, "missing bearer token")
		return state.LLMProxyKey{}, false
	}
	key, err := s.store.FindLLMProxyKeyByHash(r.Context(), state.HashLLMProxyKey(raw))
	if err != nil {
		status := http.StatusUnauthorized
		message := "invalid bearer token"
		if !errors.Is(err, sql.ErrNoRows) {
			status = http.StatusInternalServerError
			message = "key lookup failed"
		}
		s.writeProxyError(w, status, message)
		return state.LLMProxyKey{}, false
	}
	if key.Status != "active" {
		s.writeProxyError(w, http.StatusForbidden, "key is not active")
		return state.LLMProxyKey{}, false
	}
	if !key.ExpiresAt.IsZero() && !time.Now().UTC().Before(key.ExpiresAt) {
		s.writeProxyError(w, http.StatusForbidden, "key is expired")
		return state.LLMProxyKey{}, false
	}
	return key, true
}

func (s *Server) providerForKey(key state.LLMProxyKey) (providers.Provider, error) {
	provider, ok := providers.FindProvider(s.manifest, key.ProviderID)
	if !ok {
		return providers.Provider{}, fmt.Errorf("provider %q not found", key.ProviderID)
	}
	if !provider.Enabled {
		return providers.Provider{}, fmt.Errorf("provider %q is disabled", provider.ID)
	}
	switch provider.Kind {
	case "openai_compatible", "litellm_proxy", "local":
	default:
		return providers.Provider{}, fmt.Errorf("provider %q kind %q is not supported by llm-proxy v0", provider.ID, provider.Kind)
	}
	if strings.TrimSpace(provider.BaseURL) == "" {
		return providers.Provider{}, fmt.Errorf("provider %q has no base_url", provider.ID)
	}
	if strings.TrimSpace(provider.APIKeyEnv) == "" {
		return providers.Provider{}, fmt.Errorf("provider %q has no api_key_env", provider.ID)
	}
	if strings.TrimSpace(s.secretVals[provider.APIKeyEnv]) == "" {
		return providers.Provider{}, fmt.Errorf("provider %q requires secret %s", provider.ID, provider.APIKeyEnv)
	}
	return provider, nil
}

func (s *Server) allowedByBudget(ctx context.Context, w http.ResponseWriter, key state.LLMProxyKey) bool {
	if key.BudgetUSD <= 0 {
		return true
	}
	summary, err := s.store.SummarizeUsage(ctx, state.UsageFilter{KeyID: key.ID})
	if err != nil {
		s.writeProxyError(w, http.StatusInternalServerError, "budget lookup failed")
		return false
	}
	if summary.CostUSD >= key.BudgetUSD {
		record(s.logger, "stola-llm-proxy", "llm_proxy.budget", key.ID, "denied", map[string]any{"workspace_id": key.WorkspaceID, "spend_usd": summary.CostUSD, "budget_usd": key.BudgetUSD})
		s.writeProxyError(w, http.StatusTooManyRequests, "llm budget exceeded")
		return false
	}
	return true
}

func (s *Server) allowedByRPM(ctx context.Context, w http.ResponseWriter, key state.LLMProxyKey) bool {
	if key.RPM <= 0 {
		return true
	}
	_, err := s.store.ConsumeMCPRateLimit(ctx, "llm_proxy|key="+key.ID+"|rpm", key.RPM, time.Minute, time.Now().UTC())
	if err == nil {
		return true
	}
	var exceeded state.MCPRateLimitExceededError
	if errors.As(err, &exceeded) {
		record(s.logger, "stola-llm-proxy", "llm_proxy.rate_limit", key.ID, "denied", map[string]any{"workspace_id": key.WorkspaceID, "rpm": key.RPM, "reset_at": exceeded.ResetAt.Format(time.RFC3339Nano)})
		s.writeProxyError(w, http.StatusTooManyRequests, "llm rate limit exceeded")
		return false
	}
	s.writeProxyError(w, http.StatusInternalServerError, "rate limit lookup failed")
	return false
}

func (s *Server) allowedByTPM(ctx context.Context, w http.ResponseWriter, key state.LLMProxyKey, amount int) bool {
	if key.TPM <= 0 {
		return true
	}
	if amount <= 0 {
		amount = 1
	}
	_, err := s.store.ConsumeRateLimitAmount(ctx, "llm_proxy|key="+key.ID+"|tpm", amount, key.TPM, time.Minute, time.Now().UTC())
	if err == nil {
		return true
	}
	var exceeded state.MCPRateLimitExceededError
	if errors.As(err, &exceeded) {
		record(s.logger, "stola-llm-proxy", "llm_proxy.tpm", key.ID, "denied", map[string]any{"workspace_id": key.WorkspaceID, "tpm": key.TPM, "estimated_tokens": amount, "reset_at": exceeded.ResetAt.Format(time.RFC3339Nano)})
		s.writeProxyError(w, http.StatusTooManyRequests, "llm token rate limit exceeded")
		return false
	}
	s.writeProxyError(w, http.StatusInternalServerError, "token rate limit lookup failed")
	return false
}

func (s *Server) acquireConcurrency(w http.ResponseWriter, key state.LLMProxyKey) (func(), bool) {
	if key.MaxConcurrency <= 0 {
		return func() {}, true
	}
	s.mu.Lock()
	if s.active == nil {
		s.active = map[string]int{}
	}
	active := s.active[key.ID]
	if active >= key.MaxConcurrency {
		s.mu.Unlock()
		record(s.logger, "stola-llm-proxy", "llm_proxy.concurrency", key.ID, "denied", map[string]any{"workspace_id": key.WorkspaceID, "max_concurrency": key.MaxConcurrency, "active": active})
		s.writeProxyError(w, http.StatusTooManyRequests, "llm concurrency limit exceeded")
		return nil, false
	}
	s.active[key.ID] = active + 1
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.active[key.ID] <= 1 {
			delete(s.active, key.ID)
			return
		}
		s.active[key.ID]--
	}, true
}

func (s *Server) forward(ctx context.Context, provider providers.Provider, body []byte, stream bool) (int, []byte, string, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(provider.BaseURL), "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.secretVals[provider.APIKeyEnv])
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	addOpenRouterHeaders(req, provider, s.secretVals)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, nil, "", err
	}
	defer resp.Body.Close()
	limit := int64(16 * 1024 * 1024)
	if stream {
		limit = 32 * 1024 * 1024
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return resp.StatusCode, nil, resp.Header.Get("Content-Type"), err
	}
	return resp.StatusCode, raw, resp.Header.Get("Content-Type"), nil
}

func (s *Server) recordUsage(ctx context.Context, key state.LLMProxyKey, model string, status string, errorText string, usage usageFields, latency time.Duration) {
	event := state.UsageEvent{
		KeyID:        key.ID,
		WorkspaceID:  key.WorkspaceID,
		UserID:       key.UserID,
		ProviderID:   key.ProviderID,
		Model:        model,
		Operation:    "chat_completion",
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		TotalTokens:  usage.TotalTokens,
		CostUSD:      usage.CostUSD,
		LatencyMS:    int(latency.Milliseconds()),
		Status:       status,
		Error:        errorText,
	}
	if _, err := s.store.RecordUsage(ctx, event); err != nil {
		record(s.logger, "stola-llm-proxy", "llm_proxy.usage", key.ID, "failure", map[string]any{"workspace_id": key.WorkspaceID, "provider": key.ProviderID, "model": model, "error": err.Error()})
		return
	}
	record(s.logger, "stola-llm-proxy", "llm_proxy.call", key.ID, status, map[string]any{
		"workspace_id":  key.WorkspaceID,
		"user_id":       key.UserID,
		"provider":      key.ProviderID,
		"model":         model,
		"input_tokens":  event.InputTokens,
		"output_tokens": event.OutputTokens,
		"total_tokens":  event.TotalTokens,
		"cost_usd":      event.CostUSD,
		"latency_ms":    event.LatencyMS,
	})
}

func (s *Server) writeProxyError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"message": message, "type": "stolas_llm_proxy_error"}})
}

type usageFields struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	CostUSD      float64
}

func parseUsage(raw []byte, model string) usageFields {
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return usageFields{}
	}
	usage, _ := parsed["usage"].(map[string]any)
	out := usageFields{
		InputTokens:  int(numberField(usage, "prompt_tokens", "input_tokens")),
		OutputTokens: int(numberField(usage, "completion_tokens", "output_tokens")),
		TotalTokens:  int(numberField(usage, "total_tokens")),
		CostUSD:      numberField(usage, "cost_usd", "total_cost", "cost"),
	}
	if out.CostUSD == 0 {
		out.CostUSD = numberField(parsed, "response_cost", "_response_cost", "cost_usd")
	}
	if out.TotalTokens == 0 {
		out.TotalTokens = out.InputTokens + out.OutputTokens
	}
	if out.CostUSD == 0 && out.TotalTokens > 0 {
		out.CostUSD = estimateCostUSD(model, out.InputTokens, out.OutputTokens)
	}
	return out
}

func parseStreamingUsage(raw []byte, model string, fallbackInputTokens int) usageFields {
	var out usageFields
	var content strings.Builder
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		usage := parseUsage([]byte(data), model)
		if usage.TotalTokens > 0 || usage.CostUSD > 0 {
			out = usage
		}
		content.WriteString(streamContentDelta([]byte(data)))
	}
	if out.TotalTokens == 0 {
		out.InputTokens = fallbackInputTokens
		out.OutputTokens = estimateTextTokens(content.String())
		out.TotalTokens = out.InputTokens + out.OutputTokens
	}
	if out.CostUSD == 0 && out.TotalTokens > 0 {
		out.CostUSD = estimateCostUSD(model, out.InputTokens, out.OutputTokens)
	}
	return out
}

func streamContentDelta(raw []byte) string {
	var parsed struct {
		Choices []struct {
			Delta struct {
				Content any `json:"content"`
			} `json:"delta"`
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ""
	}
	var out strings.Builder
	for _, choice := range parsed.Choices {
		out.WriteString(contentValue(choice.Delta.Content))
		out.WriteString(contentValue(choice.Message.Content))
	}
	return out.String()
}

func contentValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []any:
		var out strings.Builder
		for _, item := range v {
			part, _ := item.(map[string]any)
			if text, _ := part["text"].(string); text != "" {
				out.WriteString(text)
			}
		}
		return out.String()
	default:
		return ""
	}
}

func estimateRequestTokens(payload map[string]any) int {
	raw, _ := json.Marshal(payload)
	estimate := len(raw) / 3
	if maxTokens := int(numberField(payload, "max_tokens", "max_completion_tokens")); maxTokens > 0 {
		estimate += maxTokens
	}
	if estimate < 1 {
		return 1
	}
	return estimate
}

func estimateTextTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	tokens := len(text) / 4
	if tokens == 0 {
		tokens = 1
	}
	return tokens
}

type modelPrice struct {
	InputPerMillion  float64
	OutputPerMillion float64
}

var fallbackModelPrices = map[string]modelPrice{
	"gpt-4o-mini":        {InputPerMillion: 0.15, OutputPerMillion: 0.60},
	"openai/gpt-4o-mini": {InputPerMillion: 0.15, OutputPerMillion: 0.60},
}

func estimateCostUSD(model string, inputTokens int, outputTokens int) float64 {
	price, ok := fallbackModelPrices[normalizeCostModel(model)]
	if !ok {
		return 0
	}
	return (float64(inputTokens)*price.InputPerMillion + float64(outputTokens)*price.OutputPerMillion) / 1_000_000
}

func normalizeCostModel(model string) string {
	model = strings.TrimSpace(model)
	if price := fallbackModelPrices[model]; price.InputPerMillion > 0 || price.OutputPerMillion > 0 {
		return model
	}
	if strings.HasPrefix(model, "openrouter/") {
		return strings.TrimPrefix(model, "openrouter/")
	}
	parts := strings.Split(model, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return model
}

func ensureStreamUsageOptions(payload map[string]any) {
	options, _ := payload["stream_options"].(map[string]any)
	if options == nil {
		options = map[string]any{}
		payload["stream_options"] = options
	}
	options["include_usage"] = true
}

func numberField(values map[string]any, names ...string) float64 {
	if values == nil {
		return 0
	}
	for _, name := range names {
		switch value := values[name].(type) {
		case float64:
			return value
		case int:
			return float64(value)
		case json.Number:
			parsed, _ := value.Float64()
			return parsed
		}
	}
	return 0
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func modelAllowed(key state.LLMProxyKey, model string) bool {
	if len(key.Models) == 0 {
		return true
	}
	normalized := stripProviderPrefix(key.ProviderID, model)
	for _, allowed := range key.Models {
		if model == allowed || normalized == stripProviderPrefix(key.ProviderID, allowed) {
			return true
		}
	}
	return false
}

func stripProviderPrefix(providerID string, model string) string {
	prefix := strings.TrimSpace(providerID) + "/"
	if strings.HasPrefix(model, prefix) {
		return strings.TrimPrefix(model, prefix)
	}
	return model
}

func addOpenRouterHeaders(req *http.Request, provider providers.Provider, values map[string]string) {
	if provider.ID != "openrouter" {
		return
	}
	if referer := firstPresent(values, "OPENROUTER_HTTP_REFERER", "OR_SITE_URL"); referer != "" {
		req.Header.Set("HTTP-Referer", referer)
	}
	if title := firstPresent(values, "OPENROUTER_APP_TITLE", "OR_APP_NAME"); title != "" {
		req.Header.Set("X-Title", title)
	}
}

func firstPresent(values map[string]string, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(values[name]); value != "" {
			return value
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func record(logger *audit.Logger, actor string, action string, target string, outcome string, details map[string]any) {
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
