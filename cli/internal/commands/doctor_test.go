package commands

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoctorManifestCompatibility(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		wantCode int
	}{
		{
			name:     "compatible",
			manifest: `{"code":0,"data":{"contractVersion":"cli/v1","minCliVersion":"0.1.0","deprecated":[]},"msg":"ok"}`,
			wantCode: 0,
		},
		{
			name:     "incompatible contract",
			manifest: `{"code":0,"data":{"contractVersion":"cli/v2","minCliVersion":"0.1.0","deprecated":[]},"msg":"ok"}`,
			wantCode: 9,
		},
		{
			name:     "cli too old",
			manifest: `{"code":0,"data":{"contractVersion":"cli/v1","minCliVersion":"9.0.0","deprecated":[]},"msg":"ok"}`,
			wantCode: 9,
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/cli/manifest" {
					_, _ = w.Write([]byte(item.manifest))
					return
				}
				_, _ = w.Write([]byte(`{"code":0,"data":{},"msg":"ok"}`))
			}))
			defer server.Close()

			dir := t.TempDir()
			writeConfig(t, dir, server.URL)
			var out, errOut bytes.Buffer
			code := Run([]string{"doctor", "--json", "--config-dir", dir}, &out, &errOut, strings.NewReader(""))
			if code != item.wantCode {
				t.Fatalf("exit=%d want %d, stderr=%s, stdout=%s", code, item.wantCode, errOut.String(), out.String())
			}
		})
	}
}
