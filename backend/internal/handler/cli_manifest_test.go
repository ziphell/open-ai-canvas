package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"yingce/backend/internal/service"
)

func TestCLIManifest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterCLIRoutes(router.Group("/api"))

	request := httptest.NewRequest(http.MethodGet, "/api/cli/manifest", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ContractVersion string   `json:"contractVersion"`
			MinCliVersion   string   `json:"minCliVersion"`
			Deprecated      []string `json:"deprecated"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON: %v (%s)", err, response.Body.String())
	}
	if payload.Code != 0 {
		t.Fatalf("code=%d msg=%s", payload.Code, payload.Msg)
	}
	if payload.Data.ContractVersion != service.CLIContractVersion {
		t.Fatalf("contractVersion=%q want %q", payload.Data.ContractVersion, service.CLIContractVersion)
	}
	if payload.Data.MinCliVersion != service.CLIMinCLIVersion {
		t.Fatalf("minCliVersion=%q want %q", payload.Data.MinCliVersion, service.CLIMinCLIVersion)
	}
	if payload.Data.Deprecated == nil {
		t.Fatal("deprecated must be an array")
	}
}
