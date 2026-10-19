package proxmox

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"stola-os/internal/config"
	"stola-os/internal/provisioner"
	"stola-os/internal/state"
)

type Client struct {
	Config config.ProxmoxConfig
	HTTP   *http.Client
}

type Version struct {
	Version string `json:"version"`
	Release string `json:"release"`
	RepoID  string `json:"repoid"`
}

type Node struct {
	Node   string  `json:"node"`
	Status string  `json:"status"`
	CPU    float64 `json:"cpu"`
	Memory int64   `json:"mem"`
	Uptime int64   `json:"uptime"`
}

type Storage struct {
	Storage string `json:"storage"`
	Type    string `json:"type"`
	Content string `json:"content"`
	Active  int    `json:"active"`
	Total   int64  `json:"total"`
	Used    int64  `json:"used"`
	Avail   int64  `json:"avail"`
}

type CheckResult struct {
	Version Version
	Nodes   []Node
	Storage []Storage
}

type LXCCreateRequest struct {
	VMID         int
	Hostname     string
	Template     string
	Storage      string
	RootFSSize   string
	Bridge       string
	MemoryMB     int
	SwapMB       int
	Cores        int
	Start        bool
	Unprivileged bool
}

func New(cfg config.ProxmoxConfig) Client {
	return Client{Config: cfg}
}

func (c Client) WithHTTPClient(client *http.Client) Client {
	c.HTTP = client
	return c
}

func (c Client) Type() string {
	return "proxmox"
}

func (c Client) Validate() error {
	if c.Config.Endpoint == "" {
		return errors.New("proxmox endpoint is required")
	}
	if c.Config.Node == "" {
		return errors.New("proxmox node is required")
	}
	if c.Config.TokenFile == "" {
		return errors.New("proxmox token_file is required")
	}
	return nil
}

func (c Client) Check(ctx context.Context) (CheckResult, error) {
	version, err := c.Version(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	nodes, err := c.Nodes(ctx)
	if err != nil {
		return CheckResult{}, err
	}
	storage, err := c.Storage(ctx, c.Config.Node)
	if err != nil {
		return CheckResult{}, err
	}
	return CheckResult{Version: version, Nodes: nodes, Storage: storage}, nil
}

func (c Client) Version(ctx context.Context) (Version, error) {
	var out Version
	if err := c.get(ctx, "/version", &out); err != nil {
		return Version{}, err
	}
	return out, nil
}

func (c Client) Nodes(ctx context.Context) ([]Node, error) {
	var out []Node
	if err := c.get(ctx, "/nodes", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c Client) Storage(ctx context.Context, node string) ([]Storage, error) {
	if node == "" {
		node = c.Config.Node
	}
	var out []Storage
	path := fmt.Sprintf("/nodes/%s/storage", url.PathEscape(node))
	if err := c.get(ctx, path, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c Client) Create(ctx context.Context, spec provisioner.CreateSpec) (state.VPS, error) {
	req := LXCCreateRequest{
		VMID:         spec.VMID,
		Hostname:     spec.Name,
		Template:     spec.Template,
		Storage:      spec.Storage,
		RootFSSize:   spec.RootFSSize,
		Bridge:       spec.Bridge,
		MemoryMB:     spec.MemoryMB,
		SwapMB:       spec.SwapMB,
		Cores:        spec.Cores,
		Start:        spec.Start,
		Unprivileged: spec.Unprivileged,
	}
	if req.Hostname == "" {
		req.Hostname = "stola-nano"
	}
	if req.RootFSSize == "" {
		req.RootFSSize = "8G"
	}
	if req.Bridge == "" {
		req.Bridge = "vmbr0"
	}
	if req.MemoryMB == 0 {
		req.MemoryMB = 1024
	}
	if req.SwapMB == 0 {
		req.SwapMB = 512
	}
	if req.Cores == 0 {
		req.Cores = 1
	}
	if err := req.Validate(); err != nil {
		return state.VPS{}, err
	}
	upid, err := c.CreateLXC(ctx, req)
	if err != nil {
		return state.VPS{}, err
	}
	now := time.Now().UTC()
	plan := spec.Plan
	if plan == "" {
		plan = "nano"
	}
	return state.VPS{
		ID:         fmt.Sprintf("proxmox-lxc-%d", req.VMID),
		Name:       req.Hostname,
		Plan:       plan,
		Status:     "creating",
		Backend:    c.Type(),
		CreatedAt:  now,
		ExternalID: strconv.Itoa(req.VMID),
		TaskID:     upid,
	}, nil
}

func (c Client) CreateLXC(ctx context.Context, req LXCCreateRequest) (string, error) {
	if err := req.Validate(); err != nil {
		return "", err
	}
	form := url.Values{}
	form.Set("vmid", strconv.Itoa(req.VMID))
	form.Set("ostemplate", req.Template)
	form.Set("hostname", req.Hostname)
	form.Set("memory", strconv.Itoa(req.MemoryMB))
	form.Set("swap", strconv.Itoa(req.SwapMB))
	form.Set("cores", strconv.Itoa(req.Cores))
	form.Set("rootfs", fmt.Sprintf("%s:%s", req.Storage, req.RootFSSize))
	form.Set("net0", fmt.Sprintf("name=eth0,bridge=%s,ip=dhcp", req.Bridge))
	if req.Unprivileged {
		form.Set("unprivileged", "1")
	}
	if req.Start {
		form.Set("start", "1")
	}

	var upid string
	path := fmt.Sprintf("/nodes/%s/lxc", url.PathEscape(c.Config.Node))
	if err := c.postForm(ctx, path, form, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

func (r LXCCreateRequest) Validate() error {
	var problems []string
	if r.VMID <= 0 {
		problems = append(problems, "vmid is required")
	}
	if strings.TrimSpace(r.Hostname) == "" {
		problems = append(problems, "hostname is required")
	}
	if strings.TrimSpace(r.Template) == "" {
		problems = append(problems, "template is required")
	}
	if strings.TrimSpace(r.Storage) == "" {
		problems = append(problems, "storage is required")
	}
	if strings.TrimSpace(r.RootFSSize) == "" {
		problems = append(problems, "rootfs size is required")
	}
	if strings.TrimSpace(r.Bridge) == "" {
		problems = append(problems, "bridge is required")
	}
	if r.MemoryMB <= 0 {
		problems = append(problems, "memory must be positive")
	}
	if r.SwapMB < 0 {
		problems = append(problems, "swap cannot be negative")
	}
	if r.Cores <= 0 {
		problems = append(problems, "cores must be positive")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func (c Client) get(ctx context.Context, path string, out any) error {
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c Client) postForm(ctx context.Context, path string, form url.Values, out any) error {
	req, err := c.newRequest(ctx, http.MethodPost, path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req, out)
}

func (c Client) newRequest(ctx context.Context, method string, apiPath string, body io.Reader) (*http.Request, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(c.Config.Endpoint, "/")
	req, err := http.NewRequestWithContext(ctx, method, endpoint+"/api2/json"+apiPath, body)
	if err != nil {
		return nil, err
	}
	token, err := readToken(c.Config.TokenFile)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func (c Client) do(req *http.Request, out any) error {
	client := c.httpClient()
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("proxmox api %s %s failed: status=%d body=%s", req.Method, req.URL.Path, resp.StatusCode, sanitizeBody(body))
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode proxmox response: %w", err)
	}
	if out == nil {
		return nil
	}
	if len(envelope.Data) == 0 || bytes.Equal(envelope.Data, []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("decode proxmox data: %w", err)
	}
	return nil
}

func (c Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	if c.Config.InsecureSkipTLSVerify {
		return &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // explicit config opt-in for lab/self-signed Proxmox hosts
			},
		}
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func readToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", errors.New("proxmox token file is empty")
	}
	if strings.HasPrefix(token, "PVEAPIToken=") {
		return token, nil
	}
	return "PVEAPIToken=" + token, nil
}

func sanitizeBody(body []byte) string {
	text := string(body)
	if len(text) > 512 {
		text = text[:512] + "..."
	}
	return strings.ReplaceAll(text, "\n", " ")
}
