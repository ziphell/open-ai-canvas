package commands

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"hash"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestAssetUploadUsesChunksForLargeFile(t *testing.T) {
	const chunkSize = 8 << 20
	payload := make([]byte, chunkSize+12345)
	for index := range payload {
		payload[index] = byte(index % 251)
	}
	filePath := filepath.Join(t.TempDir(), "large.bin")
	if err := os.WriteFile(filePath, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	var (
		initBody    map[string]any
		chunkBytes  = make([][]byte, 2)
		completeHit bool
		received    hash.Hash = sha256.New()
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/resources/uploads":
			if err := json.NewDecoder(r.Body).Decode(&initBody); err != nil {
				t.Fatalf("decode init: %v", err)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"uploadId":"up-1","chunkSize":8388608,"chunkCount":2},"msg":"ok"}`))
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/resources/uploads/up-1/chunks/"):
			index, err := strconv.Atoi(filepath.Base(r.URL.Path))
			if err != nil || index < 0 || index > 1 {
				t.Fatalf("unexpected chunk path %s", r.URL.Path)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			chunkBytes[index] = body
			_, _ = received.Write(body)
			_, _ = w.Write([]byte(`{"code":0,"data":{"index":` + strconv.Itoa(index) + `},"msg":"ok"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/resources/uploads/up-1/complete":
			completeHit = true
			_, _ = w.Write([]byte(`{"code":0,"data":{"resource":{"id":"res-1"}},"msg":"ok"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	writeConfig(t, dir, server.URL)
	var out, errOut bytes.Buffer
	code := Run([]string{"asset", "upload", filePath, "--idempotency-key", "upload-key", "--json", "--config-dir", dir}, &out, &errOut, strings.NewReader(""))
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if !completeHit {
		t.Fatal("complete was not called")
	}
	if initBody["fileName"] != "large.bin" || initBody["idempotencyKey"] != "upload-key" {
		t.Fatalf("unexpected init body %v", initBody)
	}
	if size, ok := initBody["size"].(float64); !ok || int64(size) != int64(len(payload)) {
		t.Fatalf("unexpected size %v", initBody["size"])
	}
	if len(chunkBytes[0]) != chunkSize || len(chunkBytes[1]) != 12345 {
		t.Fatalf("chunk sizes = %d, %d", len(chunkBytes[0]), len(chunkBytes[1]))
	}
	expected := sha256.Sum256(payload)
	if !bytes.Equal(received.Sum(nil), expected[:]) {
		t.Fatal("uploaded bytes do not match file")
	}

	var result struct {
		OK   bool `json:"ok"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid output %s: %v", out.String(), err)
	}
	if !result.OK || result.Data.ID != "res-1" {
		t.Fatalf("unexpected output %s", out.String())
	}
}
