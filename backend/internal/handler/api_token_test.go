package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"yingce/backend/internal/auth"
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
	"yingce/backend/internal/service"
)

const apiTokenTestSession = "api-token-session"

func apiTokenFixture(t *testing.T) (*gin.Engine, *service.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "tokens.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.User{}, &model.AuthSession{}, &model.APIToken{}, &model.CLIAuthCode{}); err != nil {
		t.Fatal(err)
	}
	user := model.User{ID: "user-1", Username: "user-1", DisplayName: "User", Role: model.UserRoleUser, Status: model.UserStatusActive}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.AuthSession{
		ID:        user.ID,
		UserID:    user.ID,
		TokenHash: auth.HashToken(apiTokenTestSession),
		ExpiresAt: time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := service.New(repository.New(db), dir)
	t.Cleanup(func() { _ = svc.Close() })

	router := gin.New()
	api := router.Group("/api")
	api.Use(AuthenticateBearer(svc))
	api.Use(TokenScopeGuard())
	RegisterAPITokenRoutes(api, svc)
	api.GET("/projects", func(c *gin.Context) {
		current, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"projects": []any{}, "userId": current.ID})
	})
	api.PATCH("/canvas-projects/:id/operations", func(c *gin.Context) {
		ok(c, gin.H{"applied": true})
	})
	api.GET("/admin/users", func(c *gin.Context) {
		ok(c, gin.H{"users": []any{}})
	})
	return router, svc
}

func tokenTestRequest(router *gin.Engine, method, target string, body any, bearer string, withSession bool) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		payload, _ := json.Marshal(body)
		reader = bytes.NewReader(payload)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if withSession {
		req.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: "user-1." + apiTokenTestSession})
	}
	out := httptest.NewRecorder()
	router.ServeHTTP(out, req)
	return out
}

type tokenEnvelope struct {
	Code   int             `json:"code"`
	Data   json.RawMessage `json:"data"`
	Msg    string          `json:"msg"`
	Reason string          `json:"reason"`
}

func decodeTokenEnvelope(t *testing.T, out *httptest.ResponseRecorder) tokenEnvelope {
	t.Helper()
	var envelope tokenEnvelope
	if err := json.Unmarshal(out.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid envelope: %v (%s)", err, out.Body.String())
	}
	return envelope
}

func testPKCE(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func TestCLIBrowserAuthorizationAndTokenScopes(t *testing.T) {
	router, _ := apiTokenFixture(t)
	verifier := strings.Repeat("a", 43)
	challenge := testPKCE(verifier)
	redirectURI := "http://127.0.0.1:43210/callback"
	state := "state-12345678"

	query := url.Values{
		"redirect_uri":          {redirectURI},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"name":                  {"Claude Code"},
	}
	getOut := tokenTestRequest(router, http.MethodGet, "/api/auth/cli/authorize?"+query.Encode(), nil, "", true)
	if getOut.Code != http.StatusOK {
		t.Fatalf("preview status %d: %s", getOut.Code, getOut.Body.String())
	}
	preview := decodeTokenEnvelope(t, getOut)
	if !strings.Contains(string(preview.Data), `"clientName":"Claude Code"`) {
		t.Fatalf("unexpected preview %s", preview.Data)
	}

	approveOut := tokenTestRequest(router, http.MethodPost, "/api/auth/cli/authorize", map[string]any{
		"redirectUri":         redirectURI,
		"state":               state,
		"codeChallenge":       challenge,
		"codeChallengeMethod": "S256",
		"name":                "Claude Code",
		"scopes":              []string{"read"},
	}, "", true)
	if approveOut.Code != http.StatusOK {
		t.Fatalf("approve status %d: %s", approveOut.Code, approveOut.Body.String())
	}
	approved := decodeTokenEnvelope(t, approveOut)
	var approveData struct {
		RedirectURL string `json:"redirectUrl"`
	}
	if err := json.Unmarshal(approved.Data, &approveData); err != nil {
		t.Fatal(err)
	}
	redirected, err := url.Parse(approveData.RedirectURL)
	if err != nil {
		t.Fatal(err)
	}
	code := redirected.Query().Get("code")
	if code == "" || redirected.Query().Get("state") != state {
		t.Fatalf("unexpected redirect %s", approveData.RedirectURL)
	}

	exchangeOut := tokenTestRequest(router, http.MethodPost, "/api/auth/cli/token", map[string]any{
		"code":         code,
		"codeVerifier": verifier,
		"redirectUri":  redirectURI,
	}, "", false)
	if exchangeOut.Code != http.StatusOK {
		t.Fatalf("exchange status %d: %s", exchangeOut.Code, exchangeOut.Body.String())
	}
	exchange := decodeTokenEnvelope(t, exchangeOut)
	var tokenData struct {
		Token  string   `json:"token"`
		Scopes []string `json:"scopes"`
	}
	if err := json.Unmarshal(exchange.Data, &tokenData); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tokenData.Token, "yc_") {
		t.Fatalf("unexpected token %q", tokenData.Token)
	}
	if len(tokenData.Scopes) != 1 || tokenData.Scopes[0] != service.APITokenScopeRead {
		t.Fatalf("unexpected scopes %v", tokenData.Scopes)
	}

	projects := tokenTestRequest(router, http.MethodGet, "/api/projects", nil, tokenData.Token, false)
	if projects.Code != http.StatusOK {
		t.Fatalf("projects status %d: %s", projects.Code, projects.Body.String())
	}
	canvas := tokenTestRequest(router, http.MethodPatch, "/api/canvas-projects/c1/operations", map[string]any{}, tokenData.Token, false)
	if canvas.Code != http.StatusForbidden {
		t.Fatalf("canvas status %d: %s", canvas.Code, canvas.Body.String())
	}
	if reason := decodeTokenEnvelope(t, canvas).Reason; reason != "scope_missing" {
		t.Fatalf("unexpected reason %q", reason)
	}
	admin := tokenTestRequest(router, http.MethodGet, "/api/admin/users", nil, tokenData.Token, false)
	if admin.Code != http.StatusForbidden {
		t.Fatalf("admin status %d: %s", admin.Code, admin.Body.String())
	}
	revoke := tokenTestRequest(router, http.MethodDelete, "/api/auth/tokens/current", nil, tokenData.Token, false)
	if revoke.Code != http.StatusOK {
		t.Fatalf("revoke status %d: %s", revoke.Code, revoke.Body.String())
	}
	afterRevoke := tokenTestRequest(router, http.MethodGet, "/api/projects", nil, tokenData.Token, false)
	if afterRevoke.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token status %d: %s", afterRevoke.Code, afterRevoke.Body.String())
	}
}

func TestCLIAuthCodeRejectsWrongPKCEWithoutConsuming(t *testing.T) {
	router, _ := apiTokenFixture(t)
	verifier := strings.Repeat("b", 43)
	challenge := testPKCE(verifier)
	redirectURI := "http://127.0.0.1:43211/callback"
	state := "state-abcdefgh"

	approveOut := tokenTestRequest(router, http.MethodPost, "/api/auth/cli/authorize", map[string]any{
		"redirectUri":         redirectURI,
		"state":               state,
		"codeChallenge":       challenge,
		"codeChallengeMethod": "S256",
		"name":                "Test",
		"scopes":              []string{"read"},
	}, "", true)
	if approveOut.Code != http.StatusOK {
		t.Fatalf("approve status %d: %s", approveOut.Code, approveOut.Body.String())
	}
	var approveData struct {
		RedirectURL string `json:"redirectUrl"`
	}
	_ = json.Unmarshal(decodeTokenEnvelope(t, approveOut).Data, &approveData)
	parsed, _ := url.Parse(approveData.RedirectURL)
	code := parsed.Query().Get("code")

	wrong := tokenTestRequest(router, http.MethodPost, "/api/auth/cli/token", map[string]any{
		"code":         code,
		"codeVerifier": strings.Repeat("c", 43),
		"redirectUri":  redirectURI,
	}, "", false)
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong pkce status %d: %s", wrong.Code, wrong.Body.String())
	}
	correct := tokenTestRequest(router, http.MethodPost, "/api/auth/cli/token", map[string]any{
		"code":         code,
		"codeVerifier": verifier,
		"redirectUri":  redirectURI,
	}, "", false)
	if correct.Code != http.StatusOK {
		t.Fatalf("correct pkce status %d: %s", correct.Code, correct.Body.String())
	}
}

func TestRevokeTokenByIDViaCookie(t *testing.T) {
	router, svc := apiTokenFixture(t)
	raw, record, err := svc.IssueAPIToken("user-1", "laptop", []string{service.APITokenScopeRead}, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := tokenTestRequest(router, http.MethodDelete, "/api/auth/tokens/"+record.ID, nil, "", true)
	if out.Code != http.StatusOK {
		t.Fatalf("revoke status %d: %s", out.Code, out.Body.String())
	}
	var data struct {
		Revoked bool `json:"revoked"`
	}
	if err := json.Unmarshal(decodeTokenEnvelope(t, out).Data, &data); err != nil || !data.Revoked {
		t.Fatalf("unexpected revoke response %s", out.Body.String())
	}
	after := tokenTestRequest(router, http.MethodGet, "/api/projects", nil, raw, false)
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token status %d: %s", after.Code, after.Body.String())
	}
}

func TestChunkedUploadScope(t *testing.T) {
	if got := requiredTokenScope(http.MethodPut, "/resources/uploads/abc/chunks/0"); got != service.APITokenScopeAssetUpload {
		t.Fatalf("chunk scope = %q", got)
	}
	if got := requiredTokenScope(http.MethodPost, "/resources/uploads/abc/complete"); got != service.APITokenScopeAssetUpload {
		t.Fatalf("complete scope = %q", got)
	}
}

func TestMissingTokenTaskScopeUsesTaskMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(tokenUserContextKey, &model.User{ID: "user-1"})
	c.Set(tokenScopesContextKey, []string{service.APITokenScopeRead, service.APITokenScopeGenerateImage})

	if got := missingTokenTaskScope(c, service.CreateTaskRequest{Type: "canvas_image"}); got != "" {
		t.Fatalf("image scope rejected: %q", got)
	}
	if got := missingTokenTaskScope(c, service.CreateTaskRequest{Type: "canvas_video"}); got != "generate:video" {
		t.Fatalf("video scope = %q", got)
	}
	if got := missingTokenTaskScope(c, service.CreateTaskRequest{
		Type:  "canvas_video",
		Input: map[string]any{"mode": "image"},
	}); got != "" {
		t.Fatalf("explicit image mode rejected: %q", got)
	}
}

func TestAuthorizePageRequiresLogin(t *testing.T) {
	router, _ := apiTokenFixture(t)
	verifier := strings.Repeat("e", 43)
	query := url.Values{
		"redirect_uri":          {"http://127.0.0.1:43213/callback"},
		"state":                 {"state-login-123"},
		"code_challenge":        {testPKCE(verifier)},
		"code_challenge_method": {"S256"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/cli/authorize?"+query.Encode(), nil)
	req.Header.Set("Accept", "text/html")
	out := httptest.NewRecorder()
	router.ServeHTTP(out, req)
	if out.Code != http.StatusOK {
		t.Fatalf("status %d: %s", out.Code, out.Body.String())
	}
	if !strings.Contains(out.Body.String(), "请先登录影策") {
		t.Fatalf("unexpected page %s", out.Body.String())
	}
}

func TestAuthorizePageRendersHTML(t *testing.T) {
	router, _ := apiTokenFixture(t)
	verifier := strings.Repeat("d", 43)
	query := url.Values{
		"redirect_uri":          {"http://127.0.0.1:43212/callback"},
		"state":                 {"state-html-123"},
		"code_challenge":        {testPKCE(verifier)},
		"code_challenge_method": {"S256"},
		"name":                  {"Test Agent"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/auth/cli/authorize?"+query.Encode(), nil)
	req.Header.Set("Accept", "text/html")
	req.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: "user-1." + apiTokenTestSession})
	out := httptest.NewRecorder()
	router.ServeHTTP(out, req)
	if out.Code != http.StatusOK {
		t.Fatalf("status %d: %s", out.Code, out.Body.String())
	}
	if !strings.Contains(out.Body.String(), "授权影策 CLI") || !strings.Contains(out.Body.String(), "Test Agent") {
		t.Fatalf("unexpected page %s", out.Body.String())
	}
}
