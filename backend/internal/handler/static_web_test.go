package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newStaticWebTestServer(t *testing.T, webDir string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	staticWeb, err := StaticWebHandler(webDir)
	if err != nil {
		t.Fatalf("StaticWebHandler: %v", err)
	}
	// 模拟 /api 继续走后续 NoRoute 处理器，静态托管不得吞掉 API 路径。
	router.NoRoute(staticWeb, func(c *gin.Context) {
		c.String(http.StatusTeapot, "api-passthrough")
	})
	return router
}

func writeStaticWebFixture(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	webDir := filepath.Join(parent, "web")
	if err := os.MkdirAll(filepath.Join(webDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"index.html":        "<!doctype html><title>canvas</title>",
		"assets/app-abc.js": "console.log('canvas')",
		"wasm/worker.wasm":  "wasm-bytes",
	}
	for name, content := range files {
		path := filepath.Join(webDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(parent, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	return webDir
}

func TestStaticWebHandlerServesFilesAndSPA(t *testing.T) {
	router := newStaticWebTestServer(t, writeStaticWebFixture(t))

	cases := []struct {
		name         string
		target       string
		status       int
		body         string
		cacheControl string
	}{
		{name: "index", target: "/", status: http.StatusOK, body: "canvas", cacheControl: staticWebCacheRevalidate},
		{name: "hashed asset", target: "/assets/app-abc.js", status: http.StatusOK, body: "canvas", cacheControl: staticWebCacheImmutable},
		{name: "missing asset", target: "/assets/app-missing.js", status: http.StatusNotFound, cacheControl: "no-store"},
		{name: "spa route", target: "/projects/42", status: http.StatusOK, body: "canvas", cacheControl: staticWebCacheRevalidate},
		{name: "wasm asset", target: "/wasm/worker.wasm", status: http.StatusOK, body: "wasm-bytes"},
		{name: "publish lock", target: "/.canvas-publish-lock", status: http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.target, nil))
			if recorder.Code != tc.status {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.status)
			}
			if tc.body != "" && !strings.Contains(recorder.Body.String(), tc.body) {
				t.Fatalf("body = %q, want to contain %q", recorder.Body.String(), tc.body)
			}
			if tc.cacheControl != "" && recorder.Header().Get("Cache-Control") != tc.cacheControl {
				t.Fatalf("Cache-Control = %q, want %q", recorder.Header().Get("Cache-Control"), tc.cacheControl)
			}
		})
	}
}

func TestStaticWebHandlerLeavesAPIPathsToProxy(t *testing.T) {
	router := newStaticWebTestServer(t, writeStaticWebFixture(t))

	for _, target := range []string{"/api/tasks", "/api"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusTeapot || recorder.Body.String() != "api-passthrough" {
			t.Fatalf("%s -> %d %q, want passthrough", target, recorder.Code, recorder.Body.String())
		}
	}
}

func TestStaticWebHandlerDoesNotEscapeRoot(t *testing.T) {
	webDir := writeStaticWebFixture(t)
	router := newStaticWebTestServer(t, webDir)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/../secret.txt", nil))
	if strings.Contains(recorder.Body.String(), "secret") {
		t.Fatalf("served a file outside the web root: %q", recorder.Body.String())
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want SPA fallback", recorder.Code)
	}
}

func TestStaticWebHandlerRejectsNonGet(t *testing.T) {
	router := newStaticWebTestServer(t, writeStaticWebFixture(t))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/projects", nil))
	if recorder.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want passthrough for non-GET", recorder.Code)
	}
}

func TestStaticWebHandlerRequiresIndex(t *testing.T) {
	dir := t.TempDir()
	if _, err := StaticWebHandler(dir); err == nil {
		t.Fatal("expected an error when index.html is missing")
	}
}
