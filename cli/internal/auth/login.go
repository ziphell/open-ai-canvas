package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"yingce/cli/internal/client"
	"yingce/cli/internal/config"
)

type LoginOptions struct {
	BaseURL   string
	Name      string
	NoBrowser bool
	Port      int
	Timeout   time.Duration
	Out       io.Writer
	Err       io.Writer
}

type TokenResult struct {
	Token     string
	TokenID   string
	UserID    string
	Scopes    []string
	ExpiresAt time.Time
}

// Login runs the browser authorization flow with a loopback callback.
func Login(ctx context.Context, opts LoginOptions) (TokenResult, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Minute
	}
	verifier, err := RandomURLSafe(32)
	if err != nil {
		return TokenResult{}, err
	}
	state, err := RandomURLSafe(16)
	if err != nil {
		return TokenResult{}, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(opts.Port))
	if err != nil {
		return TokenResult{}, fmt.Errorf("无法监听本地回调端口：%w", err)
	}
	defer listener.Close()

	redirectURI := "http://" + listener.Addr().String() + "/callback"
	authorizeURL := strings.TrimRight(opts.BaseURL, "/") + "/api/auth/cli/authorize?" + url.Values{
		"redirect_uri":          {redirectURI},
		"state":                 {state},
		"code_challenge":        {Challenge(verifier)},
		"code_challenge_method": {"S256"},
		"name":                  {opts.Name},
	}.Encode()

	result := make(chan callbackResult, 1)
	server := &http.Server{Handler: callbackHandler(state, result)}
	go func() {
		_ = server.Serve(listener)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	fmt.Fprintln(opts.Err, "请在浏览器中确认授权：")
	fmt.Fprintln(opts.Err, authorizeURL)
	if !opts.NoBrowser {
		if err := openBrowser(authorizeURL); err != nil {
			fmt.Fprintf(opts.Err, "无法自动打开浏览器：%v\n", err)
		}
	}

	timer := time.NewTimer(opts.Timeout)
	defer timer.Stop()
	var cb callbackResult
	select {
	case cb = <-result:
	case <-timer.C:
		return TokenResult{}, fmt.Errorf("授权超时，请重新运行 yingce login")
	case <-ctx.Done():
		return TokenResult{}, ctx.Err()
	}
	if cb.err != nil {
		return TokenResult{}, cb.err
	}

	return Exchange(ctx, opts.BaseURL, cb.code, verifier, redirectURI, 30*time.Second)
}

type callbackResult struct {
	code string
	err  error
}

func callbackHandler(state string, result chan<- callbackResult) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if value := query.Get("error"); value != "" {
			message := "授权未完成：" + value
			fmt.Fprintf(w, "<html><body><h3>%s</h3><p>可以关闭此页面。</p></body></html>", message)
			deliver(result, callbackResult{err: fmt.Errorf("%s", message)})
			return
		}
		if query.Get("state") != state {
			fmt.Fprint(w, "<html><body><h3>授权状态不匹配</h3><p>请重新运行 yingce login。</p></body></html>")
			deliver(result, callbackResult{err: fmt.Errorf("授权 state 不匹配")})
			return
		}
		code := query.Get("code")
		if code == "" {
			fmt.Fprint(w, "<html><body><h3>缺少授权码</h3><p>请重新运行 yingce login。</p></body></html>")
			deliver(result, callbackResult{err: fmt.Errorf("回调缺少 code")})
			return
		}
		fmt.Fprint(w, "<html><body><h3>授权完成</h3><p>可以关闭此页面并返回终端。</p></body></html>")
		deliver(result, callbackResult{code: code})
	})
	return mux
}

func deliver(ch chan<- callbackResult, value callbackResult) {
	select {
	case ch <- value:
	default:
	}
}

func Exchange(ctx context.Context, baseURL, code, verifier, redirectURI string, timeout time.Duration) (TokenResult, error) {
	api := client.New(baseURL, "", timeout, false, false, io.Discard)
	data, _, err := api.Do(ctx, http.MethodPost, "/auth/cli/token", nil, map[string]string{
		"code":         code,
		"codeVerifier": verifier,
		"redirectUri":  redirectURI,
	}, nil)
	if err != nil {
		return TokenResult{}, err
	}
	var payload struct {
		Token     string          `json:"token"`
		TokenID   string          `json:"tokenId"`
		Scopes    []string        `json:"scopes"`
		ExpiresAt string          `json:"expiresAt"`
		User      json.RawMessage `json:"user"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return TokenResult{}, err
	}
	if payload.Token == "" {
		return TokenResult{}, fmt.Errorf("服务端没有返回 token")
	}
	result := TokenResult{Token: payload.Token, TokenID: payload.TokenID, Scopes: payload.Scopes}
	if payload.ExpiresAt != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, payload.ExpiresAt); parseErr == nil {
			result.ExpiresAt = parsed
		}
	}
	result.UserID = extractUserID(payload.User)
	return result, nil
}

func TokenFromStdin(ctx context.Context, baseURL, token string, timeout time.Duration) (config.Credentials, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return config.Credentials{}, fmt.Errorf("token 不能为空")
	}
	api := client.New(baseURL, token, timeout, false, false, io.Discard)
	data, _, err := api.Do(ctx, http.MethodGet, "/auth/session", nil, nil, nil)
	if err != nil {
		return config.Credentials{}, err
	}
	var session struct {
		User json.RawMessage `json:"user"`
	}
	if err := json.Unmarshal(data, &session); err != nil {
		return config.Credentials{}, err
	}
	userID := extractUserID(session.User)
	if userID == "" {
		return config.Credentials{}, fmt.Errorf("token 无效或已失效")
	}
	return config.Credentials{Token: token, UserID: userID}, nil
}

func extractUserID(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var user struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &user); err == nil {
		return user.ID
	}
	return ""
}

func openBrowser(target string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", target)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	return command.Start()
}
