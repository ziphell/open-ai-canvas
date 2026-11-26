package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectListJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/projects" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("missing bearer token")
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"projects":[{"id":"p1","title":"Demo"}]},"msg":"ok"}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	writeConfig(t, dir, server.URL)
	var out, errOut bytes.Buffer
	code := Run([]string{"project", "list", "--json", "--config-dir", dir}, &out, &errOut, strings.NewReader(""))
	if code != 0 {
		t.Fatalf("exit %d, stderr %s", code, errOut.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Projects []map[string]any `json:"projects"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON: %v (%s)", err, out.String())
	}
	if !payload.OK || len(payload.Data.Projects) != 1 || payload.Data.Projects[0]["id"] != "p1" {
		t.Fatalf("unexpected payload %s", out.String())
	}
}

func TestUnauthorizedExitCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":401,"msg":"未登录","reason":"unauthorized"}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	writeConfig(t, dir, server.URL)
	var out, errOut bytes.Buffer
	code := Run([]string{"whoami", "--json", "--config-dir", dir}, &out, &errOut, strings.NewReader(""))
	if code != 3 {
		t.Fatalf("expected exit 3, got %d (%s)", code, out.String())
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Fatalf("unexpected output %s", out.String())
	}
}

func TestMissingTokenExitCode(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"baseUrl":"http://127.0.0.1:1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := Run([]string{"whoami", "--json", "--config-dir", dir}, &out, &errOut, strings.NewReader(""))
	if code != 3 {
		t.Fatalf("expected exit 3, got %d (%s)", code, out.String())
	}
	if !strings.Contains(out.String(), `"reason":"unauthorized"`) {
		t.Fatalf("unexpected output %s", out.String())
	}
}

func TestUsageExitCode(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "http://127.0.0.1:1")
	var out, errOut bytes.Buffer
	code := Run([]string{"generate", "--json", "--config-dir", dir}, &out, &errOut, strings.NewReader(""))
	if code != 2 {
		t.Fatalf("expected exit 2, got %d (%s)", code, out.String())
	}
}

func writeConfig(t *testing.T, dir, baseURL string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"baseUrl":"`+baseURL+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "credentials.json"), []byte(`{"token":"test-token"}`), 0o600); err != nil {
		t.Fatal(err)
	}
}
