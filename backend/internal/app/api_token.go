package app

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"yingce/backend/internal/auth"
	"yingce/backend/internal/kernel"
	"yingce/backend/internal/model"

	"gorm.io/gorm"
)

const (
	APITokenScopeRead          = "read"
	APITokenScopeCanvasWrite   = "canvas:write"
	APITokenScopeAssetUpload   = "asset:upload"
	APITokenScopeTaskWrite     = "task:write"
	APITokenScopeAgentRun      = "agent:run"
	APITokenScopeGenerateImage = "generate:image"
	APITokenScopeGenerateVideo = "generate:video"
	APITokenScopeGenerateAudio = "generate:audio"
)

var DefaultCLIScopes = []string{
	APITokenScopeRead,
	APITokenScopeAssetUpload,
	APITokenScopeCanvasWrite,
	APITokenScopeGenerateImage,
	APITokenScopeGenerateVideo,
	APITokenScopeGenerateAudio,
	APITokenScopeTaskWrite,
	APITokenScopeAgentRun,
}

var validAPITokenScopes = map[string]struct{}{
	APITokenScopeRead:          {},
	APITokenScopeCanvasWrite:   {},
	APITokenScopeAssetUpload:   {},
	APITokenScopeTaskWrite:     {},
	APITokenScopeAgentRun:      {},
	APITokenScopeGenerateImage: {},
	APITokenScopeGenerateVideo: {},
	APITokenScopeGenerateAudio: {},
}

type CLIAuthorizeRequest struct {
	RedirectURI         string `json:"redirectUri" form:"redirect_uri"`
	State               string `json:"state" form:"state"`
	CodeChallenge       string `json:"codeChallenge" form:"code_challenge"`
	CodeChallengeMethod string `json:"codeChallengeMethod" form:"code_challenge_method"`
	Name                string `json:"name" form:"name"`
}

type CLIAuthorizePreview struct {
	ClientName          string   `json:"clientName"`
	RedirectURI         string   `json:"redirectUri"`
	State               string   `json:"state"`
	CodeChallenge       string   `json:"codeChallenge"`
	CodeChallengeMethod string   `json:"codeChallengeMethod"`
	Scopes              []string `json:"scopes"`
	ExpiresIn           int      `json:"expiresIn"`
}

type CLIApproveRequest struct {
	CLIAuthorizeRequest
	Scopes []string `json:"scopes" form:"scope"`
}

const (
	apiTokenTTL    = 30 * 24 * time.Hour
	apiTokenMaxTTL = 90 * 24 * time.Hour
	cliAuthCodeTTL = 60 * time.Second
	apiTokenPrefix = "yc_"
)

func (s *Service) IssueAPIToken(userID, name string, scopes []string, ttl time.Duration) (string, *model.APIToken, error) {
	if strings.TrimSpace(userID) == "" {
		return "", nil, kernel.BadAuthRequest("用户不能为空")
	}
	normalized, err := normalizeAPITokenScopes(scopes)
	if err != nil {
		return "", nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Yingce CLI"
	}
	if len(name) > 64 {
		return "", nil, kernel.BadAuthRequest("令牌名称过长")
	}
	if ttl <= 0 {
		ttl = apiTokenTTL
	}
	if ttl > apiTokenMaxTTL {
		return "", nil, kernel.BadAuthRequest("令牌有效期最长 90 天")
	}
	raw, hash, err := newRawAPIToken()
	if err != nil {
		return "", nil, err
	}
	now := time.Now()
	record := &model.APIToken{
		ID:          kernel.NewID(),
		UserID:      userID,
		Name:        name,
		TokenPrefix: raw[:10],
		TokenHash:   hash,
		ScopesJSON:  mustJSON(normalized),
		CreatedAt:   now,
		ExpiresAt:   now.Add(ttl),
	}
	if err := s.repo.CreateAPIToken(record); err != nil {
		return "", nil, err
	}
	return raw, record, nil
}

func (s *Service) AuthenticateAPIToken(raw string) (*model.User, *model.APIToken, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, apiTokenPrefix) || len(raw) < 20 {
		return nil, nil, kernel.Unauthorized("访问令牌无效或已失效")
	}
	record, err := s.repo.APITokenByHash(auth.HashToken(raw))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, kernel.Unauthorized("访问令牌无效或已失效")
		}
		return nil, nil, err
	}
	now := time.Now()
	if record.RevokedAt != nil || now.After(record.ExpiresAt) {
		return nil, nil, kernel.Unauthorized("访问令牌无效或已失效")
	}
	user, err := s.repo.User(record.UserID)
	if err != nil {
		return nil, nil, kernel.Unauthorized("访问令牌无效或已失效")
	}
	if user.Status != model.UserStatusActive {
		return nil, nil, kernel.Forbidden("该账号已被禁用")
	}
	_ = s.repo.TouchAPIToken(record.ID, now)
	return user, record, nil
}

func (s *Service) ListAPITokens(user *model.User) ([]model.APIToken, error) {
	return s.repo.ListAPITokensForUser(user.ID)
}

func (s *Service) RevokeAPIToken(user *model.User, id string) (bool, error) {
	return s.repo.RevokeAPITokenForUser(user.ID, id, time.Now())
}

func (s *Service) PreviewCLIAuthorize(user *model.User, req CLIAuthorizeRequest) (*CLIAuthorizePreview, error) {
	redirectURI, err := validateLoopbackRedirect(req.RedirectURI)
	if err != nil {
		return nil, err
	}
	codeChallenge, method, err := normalizePKCE(req.CodeChallenge, req.CodeChallengeMethod)
	if err != nil {
		return nil, err
	}
	state := strings.TrimSpace(req.State)
	if len(state) < 8 || len(state) > 256 {
		return nil, kernel.BadAuthRequest("state 必须为 8–256 个字符")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "Yingce CLI"
	}
	if len(name) > 64 {
		return nil, kernel.BadAuthRequest("客户端名称过长")
	}
	return &CLIAuthorizePreview{
		ClientName:          name,
		RedirectURI:         redirectURI,
		State:               state,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: method,
		Scopes:              append([]string(nil), DefaultCLIScopes...),
		ExpiresIn:           int(cliAuthCodeTTL.Seconds()),
	}, nil
}

func (s *Service) ApproveCLIAuthorize(user *model.User, req CLIApproveRequest, ip, userAgent string) (string, error) {
	preview, err := s.PreviewCLIAuthorize(user, req.CLIAuthorizeRequest)
	if err != nil {
		return "", err
	}
	scopes, err := normalizeAPITokenScopes(req.Scopes)
	if err != nil {
		return "", err
	}
	rawCode, err := randomURLSafe(32)
	if err != nil {
		return "", err
	}
	now := time.Now()
	record := &model.CLIAuthCode{
		ID:                  kernel.NewID(),
		CodeHash:            auth.HashToken(rawCode),
		UserID:              user.ID,
		ClientName:          preview.ClientName,
		RedirectURI:         preview.RedirectURI,
		CodeChallenge:       preview.CodeChallenge,
		CodeChallengeMethod: preview.CodeChallengeMethod,
		ScopesJSON:          mustJSON(scopes),
		IPHash:              auth.HashToken(ip),
		UserAgentHash:       auth.HashToken(userAgent),
		ExpiresAt:           now.Add(cliAuthCodeTTL),
		CreatedAt:           now,
	}
	if err := s.repo.CreateCLIAuthCode(record); err != nil {
		return "", err
	}
	target, err := url.Parse(preview.RedirectURI)
	if err != nil {
		return "", err
	}
	query := target.Query()
	query.Set("code", rawCode)
	query.Set("state", preview.State)
	target.RawQuery = query.Encode()
	return target.String(), nil
}

func (s *Service) ExchangeCLIAuthCode(code, verifier, redirectURI string) (string, *model.APIToken, *model.User, error) {
	code = strings.TrimSpace(code)
	verifier = strings.TrimSpace(verifier)
	redirectURI = strings.TrimSpace(redirectURI)
	if code == "" || verifier == "" || redirectURI == "" || len(verifier) < 43 || len(verifier) > 128 {
		return "", nil, nil, kernel.Unauthorized("授权码无效或已失效")
	}
	record, err := s.repo.CLIAuthCodeByHash(auth.HashToken(code))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil, nil, kernel.Unauthorized("授权码无效或已失效")
		}
		return "", nil, nil, err
	}
	now := time.Now()
	if record.ConsumedAt != nil || now.After(record.ExpiresAt) {
		return "", nil, nil, kernel.Unauthorized("授权码无效或已失效")
	}
	if record.RedirectURI != redirectURI {
		return "", nil, nil, kernel.Unauthorized("回调地址不匹配")
	}
	if pkceChallenge(verifier) != record.CodeChallenge {
		return "", nil, nil, kernel.Unauthorized("PKCE 校验失败")
	}
	user, err := s.repo.User(record.UserID)
	if err != nil || user.Status != model.UserStatusActive {
		return "", nil, nil, kernel.Unauthorized("授权用户不可用")
	}
	raw, hash, err := newRawAPIToken()
	if err != nil {
		return "", nil, nil, err
	}
	token := &model.APIToken{
		ID:          kernel.NewID(),
		UserID:      user.ID,
		Name:        record.ClientName,
		TokenPrefix: raw[:10],
		TokenHash:   hash,
		ScopesJSON:  record.ScopesJSON,
		CreatedAt:   now,
		ExpiresAt:   now.Add(apiTokenTTL),
	}
	consumed, err := s.repo.ConsumeCLIAuthCodeAndCreateToken(record.ID, now, token)
	if err != nil {
		return "", nil, nil, err
	}
	if !consumed {
		return "", nil, nil, kernel.Unauthorized("授权码无效或已失效")
	}
	return raw, token, user, nil
}

func normalizeAPITokenScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 {
		return append([]string(nil), DefaultCLIScopes...), nil
	}
	seen := make(map[string]struct{}, len(scopes))
	result := make([]string, 0, len(scopes)+1)
	result = append(result, APITokenScopeRead)
	seen[APITokenScopeRead] = struct{}{}
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := validAPITokenScopes[scope]; !ok {
			return nil, kernel.BadAuthRequest("未知权限：" + scope)
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		result = append(result, scope)
	}
	return result, nil
}

func validateLoopbackRedirect(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Fragment != "" {
		return "", kernel.BadAuthRequest("回调地址必须是 http loopback 地址")
	}
	host := parsed.Hostname()
	if host != "127.0.0.1" && host != "::1" {
		return "", kernel.BadAuthRequest("回调地址必须是 127.0.0.1 或 ::1")
	}
	if parsed.Port() == "" {
		return "", kernel.BadAuthRequest("回调地址必须包含端口")
	}
	if parsed.Path != "/callback" || parsed.RawQuery != "" {
		return "", kernel.BadAuthRequest("回调地址必须是 /callback")
	}
	return parsed.String(), nil
}

func normalizePKCE(challenge, method string) (string, string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "S256"
	}
	if method != "S256" {
		return "", "", kernel.BadAuthRequest("只支持 PKCE S256")
	}
	challenge = strings.TrimSpace(challenge)
	if len(challenge) < 43 || len(challenge) > 128 {
		return "", "", kernel.BadAuthRequest("code_challenge 长度无效")
	}
	for _, char := range challenge {
		if !(char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-' || char == '_') {
			return "", "", kernel.BadAuthRequest("code_challenge 格式无效")
		}
	}
	return challenge, method, nil
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomURLSafe(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func newRawAPIToken() (string, string, error) {
	body, err := randomURLSafe(32)
	if err != nil {
		return "", "", err
	}
	raw := apiTokenPrefix + body
	return raw, auth.HashToken(raw), nil
}

func decodeScopes(raw string) ([]string, error) {
	var scopes []string
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	if err := json.Unmarshal([]byte(raw), &scopes); err != nil {
		return nil, err
	}
	return scopes, nil
}

// FormatAPITokenScopes exposes the stored scopes without the hash.
func FormatAPITokenScopes(record *model.APIToken) []string {
	if record == nil {
		return nil
	}
	scopes, _ := decodeScopes(record.ScopesJSON)
	return scopes
}
