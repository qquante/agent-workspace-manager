package proxmox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"stola-os/internal/config"
	"stola-os/internal/provisioner"
)

func TestCheckUsesTokenAndReadsVersionNodesStorage(t *testing.T) {
	tokenPath := writeToken(t, "root@pam!stola=secret")
	seenAuth := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "PVEAPIToken=root@pam!stola=secret" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		seenAuth[r.URL.Path] = true
		switch r.URL.Path {
		case "/api2/json/version":
			writeJSON(t, w, map[string]any{"data": map[string]any{"version": "9.0", "release": "9.0", "repoid": "test"}})
		case "/api2/json/nodes":
			writeJSON(t, w, map[string]any{"data": []map[string]any{{"node": "pve", "status": "online", "cpu": 0.1, "mem": 1024, "uptime": 99}}})
		case "/api2/json/nodes/pve/storage":
			writeJSON(t, w, map[string]any{"data": []map[string]any{{"storage": "local-lvm", "type": "lvmthin", "content": "rootdir,images", "active": 1}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(config.ProxmoxConfig{
		Endpoint:  server.URL,
		Node:      "pve",
		TokenFile: tokenPath,
	})
	result, err := client.Check(context.Background())
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result.Version.Version != "9.0" {
		t.Fatalf("unexpected version: %+v", result.Version)
	}
	if len(result.Nodes) != 1 || result.Nodes[0].Node != "pve" {
		t.Fatalf("unexpected nodes: %+v", result.Nodes)
	}
	if len(result.Storage) != 1 || result.Storage[0].Storage != "local-lvm" {
		t.Fatalf("unexpected storage: %+v", result.Storage)
	}
	for _, path := range []string{"/api2/json/version", "/api2/json/nodes", "/api2/json/nodes/pve/storage"} {
		if !seenAuth[path] {
			t.Fatalf("endpoint not called: %s", path)
		}
	}
}

func TestCreateLXCPostsNanoForm(t *testing.T) {
	tokenPath := writeToken(t, "PVEAPIToken=root@pam!stola=secret")
	var form url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api2/json/nodes/pve/lxc" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "PVEAPIToken=root@pam!stola=secret" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm returned error: %v", err)
		}
		form = r.PostForm
		writeJSON(t, w, map[string]any{"data": "UPID:pve:123"})
	}))
	defer server.Close()

	client := New(config.ProxmoxConfig{Endpoint: server.URL, Node: "pve", TokenFile: tokenPath})
	vps, err := client.Create(context.Background(), provisioner.CreateSpec{
		Name:         "nano-a",
		Plan:         "nano",
		VMID:         101,
		Template:     "local:vztmpl/debian-12-standard.tar.zst",
		Storage:      "local-lvm",
		RootFSSize:   "8G",
		Bridge:       "vmbr0",
		MemoryMB:     1024,
		SwapMB:       512,
		Cores:        1,
		Unprivileged: true,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if vps.ID != "proxmox-lxc-101" || vps.ExternalID != "101" || vps.TaskID != "UPID:pve:123" {
		t.Fatalf("unexpected VPS: %+v", vps)
	}
	assertForm(t, form, "vmid", "101")
	assertForm(t, form, "hostname", "nano-a")
	assertForm(t, form, "ostemplate", "local:vztmpl/debian-12-standard.tar.zst")
	assertForm(t, form, "memory", "1024")
	assertForm(t, form, "swap", "512")
	assertForm(t, form, "cores", "1")
	assertForm(t, form, "rootfs", "local-lvm:8G")
	assertForm(t, form, "net0", "name=eth0,bridge=vmbr0,ip=dhcp")
	assertForm(t, form, "unprivileged", "1")
}

func TestCreateLXCRequiresExplicitTemplateStorageAndVMID(t *testing.T) {
	client := New(config.ProxmoxConfig{Endpoint: "http://example.invalid", Node: "pve", TokenFile: "missing"})
	_, err := client.Create(context.Background(), provisioner.CreateSpec{Name: "nano-a"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func writeToken(t *testing.T, token string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pve.token")
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	return path
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
}

func assertForm(t *testing.T, form url.Values, key string, want string) {
	t.Helper()
	if got := form.Get(key); got != want {
		t.Fatalf("form[%s] = %q, want %q", key, got, want)
	}
}
