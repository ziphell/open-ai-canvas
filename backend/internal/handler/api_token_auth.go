package handler

import (
	"net/http"
	"strings"

	"yingce/backend/internal/model"
	"yingce/backend/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	tokenUserContextKey   = "yingce.auth.token.user"
	tokenRecordContextKey = "yingce.auth.token.record"
	tokenScopesContextKey = "yingce.auth.token.scopes"
)

// AuthenticateBearer resolves Authorization: Bearer tokens before handlers run.
// Requests without a Bearer header keep using the existing cookie session.
func AuthenticateBearer(svc *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if header == "" {
			c.Next()
			return
		}
		scheme, raw, found := strings.Cut(header, " ")
		raw = strings.TrimSpace(raw)
		if !found || !strings.EqualFold(scheme, "Bearer") {
			// 其他 Authorization 方案保持旧行为：忽略并交给 Cookie 鉴权。
			c.Next()
			return
		}
		if !strings.HasPrefix(raw, "yc_") {
			// 只接管影策 CLI 令牌，避免影响历史上携带任意 Bearer 头的调用方。
			c.Next()
			return
		}
		route := strings.TrimPrefix(c.FullPath(), "/api")
		if strings.HasPrefix(route, "/admin") || (strings.HasPrefix(route, "/auth/tokens") && route != "/auth/tokens/current") {
			writeFailure(c, http.StatusForbidden, service.CodeForbidden, "forbidden", "访问令牌不能访问该接口")
			c.Abort()
			return
		}
		user, record, err := svc.AuthenticateAPIToken(raw)
		if err != nil {
			failService(c, err)
			c.Abort()
			return
		}
		if expected := strings.TrimSpace(c.GetHeader("X-Canvas-Expected-User")); expected != "" && expected != user.ID {
			writeFailure(c, http.StatusForbidden, service.CodeForbidden, "account_switched", "账号已切换，请重新加载后重试")
			c.Abort()
			return
		}
		c.Set(tokenUserContextKey, user)
		c.Set(tokenRecordContextKey, record)
		c.Set(tokenScopesContextKey, service.FormatAPITokenScopes(record))
		c.Next()
	}
}

// TokenScopeGuard limits Bearer requests to the CLI contract. Cookie requests
// are untouched so the existing web surface keeps its own authorization.
func TokenScopeGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		scopes, ok := tokenContextScopes(c)
		if !ok {
			c.Next()
			return
		}
		route := strings.TrimPrefix(c.FullPath(), "/api")
		if route == "/auth/tokens/current" {
			c.Next()
			return
		}
		required := requiredTokenScope(c.Request.Method, route)
		if required == "" || !tokenScopeSatisfied(scopes, required) {
			message := "访问令牌缺少该接口权限"
			if required != "" {
				message = "访问令牌缺少权限：" + required
			}
			writeFailure(c, http.StatusForbidden, service.CodeForbidden, "scope_missing", message)
			c.Abort()
			return
		}
		c.Next()
	}
}

func requiredTokenScope(method, route string) string {
	switch method {
	case http.MethodGet:
		switch {
		case route == "/auth/session", route == "/openapi.yaml", route == "/cli/manifest":
			return service.APITokenScopeRead
		case route == "/projects", strings.HasPrefix(route, "/projects/"):
			return service.APITokenScopeRead
		case route == "/canvas-projects", strings.HasPrefix(route, "/canvas-projects/"):
			return service.APITokenScopeRead
		case route == "/tasks", strings.HasPrefix(route, "/tasks/"):
			return service.APITokenScopeRead
		case route == "/model-catalog", strings.HasPrefix(route, "/models/"):
			return service.APITokenScopeRead
		case route == "/wallet", route == "/resources", strings.HasPrefix(route, "/resources/"):
			return service.APITokenScopeRead
		case strings.HasPrefix(route, "/agent/"):
			return service.APITokenScopeRead
		case route == "/skills", strings.HasPrefix(route, "/skills/"):
			return service.APITokenScopeRead
		}
	case http.MethodPost:
		switch {
		case route == "/tasks":
			return "generate"
		case route == "/tasks/:id/cancel":
			return service.APITokenScopeTaskWrite
		case route == "/canvas-projects/:id/edit-lease":
			return service.APITokenScopeCanvasWrite
		case route == "/resources", strings.HasPrefix(route, "/resources/uploads"):
			return service.APITokenScopeAssetUpload
		case route == "/agent/runs", route == "/agent/runs/:id/cancel":
			return service.APITokenScopeAgentRun
		case route == "/models/:id/quote", route == "/model-catalog/quote":
			return service.APITokenScopeRead
		}
	case http.MethodPatch:
		if route == "/canvas-projects/:id/operations" {
			return service.APITokenScopeCanvasWrite
		}
	}
	return ""
}

func tokenScopeSatisfied(scopes []string, required string) bool {
	if required == "generate" {
		for _, scope := range scopes {
			if strings.HasPrefix(scope, "generate:") {
				return true
			}
		}
		return false
	}
	for _, scope := range scopes {
		if scope == required {
			return true
		}
	}
	return false
}

// missingTokenTaskScope 按任务模式细分 generate 权限，避免只持有图片权限的令牌提交视频。
func missingTokenTaskScope(c *gin.Context, req service.CreateTaskRequest) string {
	scopes, ok := tokenContextScopes(c)
	if !ok {
		return ""
	}
	mode := ""
	if raw, ok := req.Input["mode"].(string); ok {
		mode = strings.ToLower(strings.TrimSpace(raw))
	}
	if mode == "" {
		switch req.Type {
		case "canvas_image":
			mode = "image"
		case "canvas_video":
			mode = "video"
		case "canvas_audio":
			mode = "audio"
		}
	}
	switch mode {
	case "image", "video", "audio":
	default:
		return ""
	}
	required := "generate:" + mode
	if tokenScopeSatisfied(scopes, required) {
		return ""
	}
	return required
}

func tokenContextUser(c *gin.Context) (*model.User, bool) {
	value, exists := c.Get(tokenUserContextKey)
	if !exists {
		return nil, false
	}
	user, ok := value.(*model.User)
	return user, ok && user != nil
}

func tokenContextRecord(c *gin.Context) (*model.APIToken, bool) {
	value, exists := c.Get(tokenRecordContextKey)
	if !exists {
		return nil, false
	}
	record, ok := value.(*model.APIToken)
	return record, ok && record != nil
}

func tokenContextScopes(c *gin.Context) ([]string, bool) {
	if _, ok := tokenContextUser(c); !ok {
		return nil, false
	}
	value, exists := c.Get(tokenScopesContextKey)
	if !exists {
		return nil, true
	}
	scopes, _ := value.([]string)
	return scopes, true
}
