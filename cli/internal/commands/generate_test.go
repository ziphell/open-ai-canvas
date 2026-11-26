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

func TestGenerateUploadsReferenceAndSubmitsTask(t *testing.T) {
	var submitted map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/resources":
			if err := r.ParseMultipartForm(8 << 20); err != nil {
				t.Fatalf("parse multipart: %v", err)
			}
			if r.FormValue("kind") != "image" {
				t.Fatalf("unexpected kind %q", r.FormValue("kind"))
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"resource":{"id":"res_1"}},"msg":"ok"}`))
		case "/api/tasks":
			if err := json.NewDecoder(r.Body).Decode(&submitted); err != nil {
				t.Fatalf("decode task: %v", err)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":"task_1","status":"queued"},"msg":"ok"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	writeConfig(t, dir, server.URL)
	imagePath := filepath.Join(dir, "ref.png")
	if err := os.WriteFile(imagePath, []byte("fake-png"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := Run([]string{
		"generate", "--json", "--config-dir", dir,
		"--mode", "image", "--model", "logical_1",
		"--prompt", "test", "--ref", imagePath,
	}, &out, &errOut, strings.NewReader(""))
	if code != 0 {
		t.Fatalf("exit %d, stderr %s", code, errOut.String())
	}
	if submitted["logicalModelId"] != "logical_1" || submitted["type"] != "canvas_image" {
		t.Fatalf("unexpected task %+v", submitted)
	}
	input, _ := submitted["input"].(map[string]any)
	refs, _ := input["referenceImages"].([]any)
	if len(refs) != 1 {
		t.Fatalf("unexpected refs %+v", refs)
	}
	first, _ := refs[0].(map[string]any)
	if first["storageKey"] != "resource:res_1" {
		t.Fatalf("unexpected storageKey %+v", first)
	}
	if !strings.Contains(out.String(), `"task_1"`) {
		t.Fatalf("unexpected output %s", out.String())
	}
}

func TestCanvasApplyDryRunDoesNotCallServer(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "http://127.0.0.1:1")
	opsPath := filepath.Join(dir, "ops.json")
	if err := os.WriteFile(opsPath, []byte(`{"mutationId":"m1","baseRevision":3,"operations":[{"type":"set_field","field":"title","value":"x"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := Run([]string{
		"canvas", "apply", "canvas_1", "--json", "--config-dir", dir,
		"--ops", opsPath, "--dry-run",
	}, &out, &errOut, strings.NewReader(""))
	if code != 0 {
		t.Fatalf("exit %d, stderr %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"dryRun":true`) {
		t.Fatalf("unexpected output %s", out.String())
	}
}
