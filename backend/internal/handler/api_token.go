package handler

import (
	"bytes"
	"errors"
	"html/template"
	"net/http"
	"strings"
	"time"

	"yingce/backend/internal/model"
	"yingce/backend/internal/service"

	"github.com/gin-gonic/gin"
)

func RegisterAPITokenRoutes(api *gin.RouterGroup, svc *service.Service) {
	api.DELETE("/auth/tokens/current", func(c *gin.Context) {
		if !apiTokensEnabled() {
			writeFailure(c, http.StatusForbidden, service.CodeForbidden, "api_tokens_disabled", "服务端未开启 CLI 访问")
			return
		}
		user, record, valid := currentToken(c, svc)
		if !valid {
			return
		}
		revoked, err := svc.RevokeAPIToken(user, record.ID)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"revoked": revoked})
	})

	api.GET("/auth/tokens", func(c *gin.Context) {
		if !apiTokensEnabled() {
			writeFailure(c, http.StatusForbidden, service.CodeForbidden, "api_tokens_disabled", "服务端未开启 CLI 访问")
			return
		}
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		tokens, err := svc.ListAPITokens(user)
		if err != nil {
			failService(c, err)
			return
		}
		views := make([]apiTokenView, 0, len(tokens))
		for index := range tokens {
			views = append(views, newAPITokenView(&tokens[index]))
		}
		ok(c, gin.H{"tokens": views})
	})

	api.DELETE("/auth/tokens/:id", func(c *gin.Context) {
		if !apiTokensEnabled() {
			writeFailure(c, http.StatusForbidden, service.CodeForbidden, "api_tokens_disabled", "服务端未开启 CLI 访问")
			return
		}
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		revoked, err := svc.RevokeAPIToken(user, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"revoked": revoked})
	})

	api.GET("/auth/cli/authorize", func(c *gin.Context) {
		if !apiTokensEnabled() {
			writeFailure(c, http.StatusForbidden, service.CodeForbidden, "api_tokens_disabled", "服务端未开启 CLI 访问")
			return
		}
		user, err := currentUser(c, svc)
		if err != nil {
			if wantsHTML(c) {
				renderAuthorizeLoginRequired(c)
				return
			}
			failService(c, err)
			return
		}
		preview, err := svc.PreviewCLIAuthorize(user, authorizeRequestFromQuery(c))
		if err != nil {
			failService(c, err)
			return
		}
		if wantsHTML(c) {
			renderAuthorizePage(c, preview)
			return
		}
		ok(c, preview)
	})

	api.POST("/auth/cli/authorize", func(c *gin.Context) {
		if !apiTokensEnabled() {
			writeFailure(c, http.StatusForbidden, service.CodeForbidden, "api_tokens_disabled", "服务端未开启 CLI 访问")
			return
		}
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		approval, isForm, err := bindCLIApproveRequest(c)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		redirectURL, err := svc.ApproveCLIAuthorize(user, approval, c.ClientIP(), c.Request.UserAgent())
		if err != nil {
			failService(c, err)
			return
		}
		if isForm {
			c.Redirect(http.StatusFound, redirectURL)
			return
		}
		ok(c, gin.H{"redirectUrl": redirectURL})
	})

	api.POST("/auth/cli/token", func(c *gin.Context) {
		if !apiTokensEnabled() {
			writeFailure(c, http.StatusForbidden, service.CodeForbidden, "api_tokens_disabled", "服务端未开启 CLI 访问")
			return
		}
		var req struct {
			Code         string `json:"code"`
			CodeVerifier string `json:"codeVerifier"`
			RedirectURI  string `json:"redirectUri"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			fail(c, http.StatusBadRequest, errors.New("兑换请求格式错误"))
			return
		}
		raw, record, user, err := svc.ExchangeCLIAuthCode(req.Code, req.CodeVerifier, req.RedirectURI)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{
			"token":     raw,
			"tokenId":   record.ID,
			"scopes":    service.FormatAPITokenScopes(record),
			"expiresAt": record.ExpiresAt,
			"user": gin.H{
				"id":          user.ID,
				"username":    user.Username,
				"displayName": user.DisplayName,
			},
		})
	})
}

type apiTokenView struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"tokenPrefix"`
	Scopes      []string   `json:"scopes"`
	CreatedAt   time.Time  `json:"createdAt"`
	ExpiresAt   time.Time  `json:"expiresAt"`
	LastUsedAt  *time.Time `json:"lastUsedAt"`
	RevokedAt   *time.Time `json:"revokedAt"`
}

func newAPITokenView(token *model.APIToken) apiTokenView {
	return apiTokenView{
		ID:          token.ID,
		Name:        token.Name,
		TokenPrefix: token.TokenPrefix,
		Scopes:      service.FormatAPITokenScopes(token),
		CreatedAt:   token.CreatedAt,
		ExpiresAt:   token.ExpiresAt,
		LastUsedAt:  token.LastUsedAt,
		RevokedAt:   token.RevokedAt,
	}
}

func currentToken(c *gin.Context, svc *service.Service) (*model.User, *model.APIToken, bool) {
	user, ok := tokenContextUser(c)
	if !ok {
		fail(c, http.StatusBadRequest, errors.New("该接口只接受 Bearer 访问令牌"))
		return nil, nil, false
	}
	record, ok := tokenContextRecord(c)
	if !ok {
		fail(c, http.StatusBadRequest, errors.New("访问令牌上下文缺失"))
		return nil, nil, false
	}
	return user, record, true
}

func authorizeRequestFromQuery(c *gin.Context) service.CLIAuthorizeRequest {
	return service.CLIAuthorizeRequest{
		RedirectURI:         c.Query("redirect_uri"),
		State:               c.Query("state"),
		CodeChallenge:       c.Query("code_challenge"),
		CodeChallengeMethod: c.Query("code_challenge_method"),
		Name:                c.Query("name"),
	}
}

func bindCLIApproveRequest(c *gin.Context) (service.CLIApproveRequest, bool, error) {
	if strings.Contains(c.ContentType(), "application/json") {
		var req struct {
			service.CLIAuthorizeRequest
			Scopes []string `json:"scopes"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			return service.CLIApproveRequest{}, false, errors.New("授权请求格式错误")
		}
		return service.CLIApproveRequest{
			CLIAuthorizeRequest: req.CLIAuthorizeRequest,
			Scopes:              req.Scopes,
		}, false, nil
	}
	if err := c.Request.ParseForm(); err != nil {
		return service.CLIApproveRequest{}, true, errors.New("授权表单格式错误")
	}
	return service.CLIApproveRequest{
		CLIAuthorizeRequest: service.CLIAuthorizeRequest{
			RedirectURI:         c.PostForm("redirect_uri"),
			State:               c.PostForm("state"),
			CodeChallenge:       c.PostForm("code_challenge"),
			CodeChallengeMethod: c.PostForm("code_challenge_method"),
			Name:                c.PostForm("name"),
		},
		Scopes: c.PostFormArray("scope"),
	}, true, nil
}

func wantsHTML(c *gin.Context) bool {
	return strings.Contains(strings.ToLower(c.GetHeader("Accept")), "text/html")
}

type authorizeScopeOption struct {
	Value       string
	Label       string
	Description string
}

type authorizePageData struct {
	ClientName          string
	RedirectURI         string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Scopes              []authorizeScopeOption
	ExpiresIn           int
}

var authorizePageTemplate = template.Must(template.New("authorize").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>授权影策 CLI</title>
<style>
body{font-family:system-ui,-apple-system,"Segoe UI",sans-serif;background:#f6f7f9;color:#1f2328;margin:0;padding:40px 16px}
main{max-width:520px;margin:0 auto;background:#fff;border:1px solid #e4e7ec;border-radius:14px;padding:28px}
h1{font-size:20px;margin:0 0 8px}
p{line-height:1.6;color:#4b5563}
code{background:#f2f4f7;padding:2px 6px;border-radius:6px}
label{display:flex;gap:10px;align-items:flex-start;padding:10px 0;border-top:1px solid #f0f2f5}
label span small{display:block;color:#6b7280;margin-top:2px}
.permissions{list-style:none;padding:0;margin:16px 0;border-top:1px solid #f0f2f5}
.permissions li{padding:10px 0;border-bottom:1px solid #f0f2f5}
.permissions small{display:block;color:#6b7280;margin-top:2px}
button{margin-top:20px;width:100%;padding:12px;border:0;border-radius:10px;background:#111827;color:#fff;font-size:15px;cursor:pointer}
.meta{font-size:13px;color:#6b7280;word-break:break-all}
</style>
</head>
<body>
<main>
<h1>授权影策 CLI</h1>
<p><strong>{{.ClientName}}</strong> 请求访问你的影策账号。</p>
<p>授权后，CLI 将作为当前账号的另一个入口，获得以下权限：</p>
<ul class="permissions">
{{range .Scopes}}<li><strong>{{.Label}}</strong><small>{{.Description}}</small></li>{{end}}
</ul>
<p class="meta">回调地址：<code>{{.RedirectURI}}</code></p>
<form method="post" action="/api/auth/cli/authorize">
  <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
  <input type="hidden" name="state" value="{{.State}}">
  <input type="hidden" name="code_challenge" value="{{.CodeChallenge}}">
  <input type="hidden" name="code_challenge_method" value="{{.CodeChallengeMethod}}">
  <input type="hidden" name="name" value="{{.ClientName}}">
  {{range .Scopes}}<input type="hidden" name="scope" value="{{.Value}}">{{end}}
  <button type="submit">授权</button>
</form>
<p class="meta">授权码 {{.ExpiresIn}} 秒内有效。CLI 与网页共用同一账号的积分、并发和存储配额。</p>
</main>
</body>
</html>`))

func renderAuthorizePage(c *gin.Context, preview *service.CLIAuthorizePreview) {
	data := authorizePageData{
		ClientName:          preview.ClientName,
		RedirectURI:         preview.RedirectURI,
		State:               preview.State,
		CodeChallenge:       preview.CodeChallenge,
		CodeChallengeMethod: preview.CodeChallengeMethod,
		ExpiresIn:           preview.ExpiresIn,
	}
	for _, scope := range preview.Scopes {
		data.Scopes = append(data.Scopes, authorizeScopeOption{
			Value:       scope,
			Label:       scopeLabel(scope),
			Description: scopeDescription(scope),
		})
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'")
	var body bytes.Buffer
	if err := authorizePageTemplate.Execute(&body, data); err != nil {
		c.String(http.StatusInternalServerError, "授权页渲染失败")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", body.Bytes())
}

func renderAuthorizeLoginRequired(c *gin.Context) {
	const page = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>请先登录影策</title>
<style>body{font-family:system-ui,-apple-system,"Segoe UI",sans-serif;background:#f6f7f9;margin:0;padding:40px 16px}main{max-width:480px;margin:0 auto;background:#fff;border:1px solid #e4e7ec;border-radius:14px;padding:28px;line-height:1.6}a{color:#111827}</style>
</head><body><main>
<h1>请先登录影策</h1>
<p>完成 <a href="/login" target="_blank" rel="noopener">登录</a> 后，回到此页面刷新即可继续授权。</p>
<p>如果页面已经关闭，请重新运行 <code>yingce login</code>。</p>
</main></body></html>`
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(page))
}

func scopeLabel(scope string) string {
	switch scope {
	case service.APITokenScopeRead:
		return "读取项目与画布"
	case service.APITokenScopeCanvasWrite:
		return "写入画布"
	case service.APITokenScopeAssetUpload:
		return "上传素材"
	case service.APITokenScopeGenerateImage:
		return "生成图片"
	case service.APITokenScopeGenerateVideo:
		return "生成视频"
	case service.APITokenScopeGenerateAudio:
		return "生成音频"
	case service.APITokenScopeTaskWrite:
		return "取消任务"
	case service.APITokenScopeAgentRun:
		return "云端 Agent"
	default:
		return scope
	}
}

func scopeDescription(scope string) string {
	switch scope {
	case service.APITokenScopeRead:
		return "查看项目、画布、任务和模型目录"
	case service.APITokenScopeCanvasWrite:
		return "创建、修改画布节点和连线"
	case service.APITokenScopeAssetUpload:
		return "上传本地素材到影策"
	case service.APITokenScopeGenerateImage:
		return "提交图片生成任务并消耗积分"
	case service.APITokenScopeGenerateVideo:
		return "提交视频生成任务并消耗积分"
	case service.APITokenScopeGenerateAudio:
		return "提交音频生成任务并消耗积分"
	case service.APITokenScopeTaskWrite:
		return "取消由该令牌提交的任务"
	case service.APITokenScopeAgentRun:
		return "创建云端 Agent 运行"
	default:
		return ""
	}
}
