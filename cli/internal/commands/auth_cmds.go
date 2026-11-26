package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"yingce/cli/internal/auth"
	"yingce/cli/internal/client"
	"yingce/cli/internal/config"
)

func runLogin(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var noBrowser, tokenStdin bool
	var name string
	var port int
	fs := newFlagSet("login", &App{Err: errw})
	g.register(fs)
	fs.BoolVar(&noBrowser, "no-browser", false, "不自动打开浏览器")
	fs.BoolVar(&tokenStdin, "token-stdin", false, "从 stdin 读取 token")
	fs.StringVar(&name, "name", "", "token 名称")
	fs.IntVar(&port, "port", 0, "本地回调端口，0 表示随机")
	if code, done := parseFlags(fs, args); done {
		return code
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	if err := app.requireBaseURL(); err != nil {
		return app.fail(err)
	}
	ctx := context.Background()

	if tokenStdin {
		payload, readErr := io.ReadAll(io.LimitReader(in, 64<<10))
		if readErr != nil {
			return app.fail(readErr)
		}
		creds, loginErr := auth.TokenFromStdin(ctx, app.G.BaseURL, string(payload), app.G.Timeout)
		if loginErr != nil {
			return app.fail(loginErr)
		}
		if err := config.SaveCredentials(app.Dir, creds); err != nil {
			return app.fail(err)
		}
		if err := app.rememberBaseURL(); err != nil {
			return app.fail(err)
		}
		return app.fail(app.emit(marshal(map[string]any{
			"user": map[string]any{"id": creds.UserID},
		})))
	}

	result, err := auth.Login(ctx, auth.LoginOptions{
		BaseURL:   app.G.BaseURL,
		Name:      name,
		NoBrowser: noBrowser,
		Port:      port,
		Timeout:   app.G.Timeout * 10,
		Out:       out,
		Err:       errw,
	})
	if err != nil {
		return app.fail(err)
	}
	creds := config.Credentials{
		Token:     result.Token,
		TokenID:   result.TokenID,
		UserID:    result.UserID,
		Scopes:    result.Scopes,
		ExpiresAt: result.ExpiresAt,
	}
	if err := config.SaveCredentials(app.Dir, creds); err != nil {
		return app.fail(err)
	}
	if err := app.rememberBaseURL(); err != nil {
		return app.fail(err)
	}
	return app.fail(app.emit(marshal(map[string]any{
		"user":      map[string]any{"id": result.UserID},
		"tokenId":   result.TokenID,
		"scopes":    result.Scopes,
		"expiresAt": result.ExpiresAt,
	})))
}

func (a *App) rememberBaseURL() error {
	cfg := a.Config
	cfg.BaseURL = a.G.BaseURL
	return config.SaveConfig(a.Dir, cfg)
}

func runLogout(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	fs := newFlagSet("logout", &App{Err: errw})
	g.register(fs)
	if code, done := parseFlags(fs, args); done {
		return code
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	var revokeErr error
	if app.G.Token != "" && app.G.BaseURL != "" {
		_, _, revokeErr = app.client().Do(context.Background(), http.MethodDelete, "/auth/tokens/current", nil, nil, nil)
	}
	if err := config.ClearCredentials(app.Dir); err != nil {
		return app.fail(err)
	}
	if revokeErr != nil {
		fmt.Fprintf(errw, "warning: 服务端撤销失败，本地凭据已删除：%v\n", revokeErr)
	}
	return app.fail(app.emit(marshal(map[string]any{"loggedOut": true})))
}

func runWhoami(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	fs := newFlagSet("whoami", &App{Err: errw})
	g.register(fs)
	if code, done := parseFlags(fs, args); done {
		return code
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	if err := app.requireBaseURL(); err != nil {
		return app.fail(err)
	}
	if err := app.requireToken(); err != nil {
		return app.fail(err)
	}
	data, _, err := app.client().Do(context.Background(), http.MethodGet, "/auth/session", nil, nil, nil)
	if err != nil {
		return app.fail(err)
	}
	if !hasUser(data) {
		return app.fail(&client.APIError{Status: http.StatusUnauthorized, Reason: "unauthorized", Message: "token 无效或已失效"})
	}
	return app.fail(app.emit(data))
}

func runDoctor(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	fs := newFlagSet("doctor", &App{Err: errw})
	g.register(fs)
	if code, done := parseFlags(fs, args); done {
		return code
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	if err := app.requireBaseURL(); err != nil {
		return app.fail(err)
	}
	type check struct {
		Name    string `json:"name"`
		OK      bool   `json:"ok"`
		Message string `json:"message,omitempty"`
	}
	checks := make([]check, 0, 4)
	failed := false
	doctorClient := client.New(app.G.BaseURL, app.G.Token, 5*time.Second, app.G.Insecure, app.G.Debug, app.Err)
	for _, item := range []struct {
		name string
		path string
	}{
		{"health", "/health/ready"},
		{"auth", "/auth/session"},
		{"agent", "/agent/capabilities"},
	} {
		_, _, callErr := doctorClient.Do(context.Background(), http.MethodGet, item.path, nil, nil, nil)
		if callErr != nil {
			checks = append(checks, check{Name: item.name, OK: false, Message: callErr.Error()})
			failed = true
			continue
		}
		checks = append(checks, check{Name: item.name, OK: true})
	}
	if !failed {
		checks = append(checks, check{Name: "contract", OK: true, Message: ContractVersion})
	}
	if app.G.JSON {
		payload, _ := json.Marshal(map[string]any{"ok": !failed, "data": map[string]any{"checks": checks}})
		fmt.Fprintln(out, string(payload))
		if failed {
			return 9
		}
		return 0
	}
	for _, item := range checks {
		status := "ok"
		if !item.OK {
			status = "fail"
		}
		line := fmt.Sprintf("%-8s %s", item.Name, status)
		if item.Message != "" {
			line += "  " + item.Message
		}
		fmt.Fprintln(out, line)
	}
	if failed {
		return 9
	}
	return 0
}

func marshal(value any) json.RawMessage {
	payload, _ := json.Marshal(value)
	return payload
}

func hasUser(data json.RawMessage) bool {
	var value struct {
		User json.RawMessage `json:"user"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return false
	}
	trimmed := strings.TrimSpace(string(value.User))
	return trimmed != "" && trimmed != "null"
}
