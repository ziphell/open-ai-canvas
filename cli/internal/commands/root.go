package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"yingce/cli/internal/client"
	"yingce/cli/internal/config"
)

const ContractVersion = "cli/v1"

// 版本元数据由发布流水线通过 -ldflags 注入；本地直接构建时保留 dev 标识。
var (
	Version = "0.1.0-dev"
	Commit  = ""
)

type Global struct {
	BaseURL   string
	Token     string
	ConfigDir string
	JSON      bool
	Quiet     bool
	NoColor   bool
	Timeout   time.Duration
	Insecure  bool
	Debug     bool
}

func (g *Global) register(fs *flag.FlagSet) {
	fs.StringVar(&g.BaseURL, "base-url", "", "服务端地址")
	fs.StringVar(&g.Token, "token", "", "访问令牌（调试用，优先使用环境变量）")
	fs.StringVar(&g.ConfigDir, "config-dir", "", "配置目录")
	fs.BoolVar(&g.JSON, "json", false, "输出机器可读 JSON")
	fs.BoolVar(&g.Quiet, "quiet", false, "只输出结果")
	fs.BoolVar(&g.NoColor, "no-color", false, "关闭颜色")
	fs.DurationVar(&g.Timeout, "timeout", 30*time.Second, "单次请求超时")
	fs.BoolVar(&g.Insecure, "insecure", false, "跳过 TLS 校验（仅 localhost）")
	fs.BoolVar(&g.Debug, "debug", false, "输出脱敏 HTTP 日志")
}

type App struct {
	G      Global
	Dir    string
	Config config.Config
	Creds  config.Credentials
	Out    io.Writer
	Err    io.Writer
	In     io.Reader
}

type usageError struct{ message string }

func (e *usageError) Error() string { return e.message }

func usagef(format string, args ...any) error {
	return &usageError{message: fmt.Sprintf(format, args...)}
}

func newApp(g Global, out, errw io.Writer, in io.Reader) (*App, error) {
	app := &App{G: g, Out: out, Err: errw, In: in}
	dir, err := config.Dir(g.ConfigDir)
	if err != nil {
		return app, err
	}
	app.Dir = dir
	cfg, creds, err := config.Load(dir)
	if err != nil {
		return app, err
	}
	app.Config = cfg
	app.Creds = creds
	if g.BaseURL == "" {
		g.BaseURL = os.Getenv("YINGCE_BASE_URL")
	}
	if g.BaseURL == "" {
		g.BaseURL = cfg.BaseURL
	}
	if g.Token == "" {
		g.Token = os.Getenv("YINGCE_TOKEN")
	}
	if g.Token == "" {
		g.Token = creds.Token
	}
	g.BaseURL = strings.TrimRight(g.BaseURL, "/")
	app.G = g
	return app, nil
}

func (a *App) client() *client.Client {
	return client.New(a.G.BaseURL, a.G.Token, a.G.Timeout, a.G.Insecure, a.G.Debug, a.Err)
}

func (a *App) requireBaseURL() error {
	if a.G.BaseURL == "" {
		return usagef("缺少服务端地址：请传 --base-url 或设置 YINGCE_BASE_URL")
	}
	return nil
}

func (a *App) requireToken() error {
	if a.G.Token == "" {
		return &client.APIError{Status: 401, Reason: "unauthorized", Message: "尚未登录：请运行 yingce login"}
	}
	return nil
}

func (a *App) ready() error {
	if err := a.requireBaseURL(); err != nil {
		return err
	}
	return a.requireToken()
}

func (a *App) emit(data json.RawMessage) error {
	if len(data) == 0 || string(data) == "null" {
		data = json.RawMessage(`{}`)
	}
	if a.G.JSON {
		payload, err := json.Marshal(map[string]any{"ok": true, "data": data})
		if err != nil {
			return err
		}
		fmt.Fprintln(a.Out, string(payload))
		return nil
	}
	if a.G.Quiet {
		fmt.Fprintln(a.Out, string(data))
		return nil
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		fmt.Fprintln(a.Out, string(data))
		return nil
	}
	fmt.Fprintln(a.Out, buf.String())
	return nil
}

func (a *App) fail(err error) int {
	if err == nil {
		return 0
	}
	code := client.ExitCode(err)
	var usage *usageError
	if errors.As(err, &usage) {
		code = 2
	}
	if a.G.JSON {
		payload := map[string]any{
			"ok": false,
			"error": map[string]any{
				"code":    code,
				"reason":  reasonOf(err),
				"message": err.Error(),
			},
		}
		encoded, _ := json.Marshal(payload)
		fmt.Fprintln(a.Out, string(encoded))
	} else {
		fmt.Fprintf(a.Err, "error: %v\n", err)
	}
	return code
}

func reasonOf(err error) string {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.Reason != "" {
		return apiErr.Reason
	}
	return "error"
}

func Run(args []string, out, errw io.Writer, in io.Reader) int {
	if len(args) == 0 {
		usage(errw)
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help":
		usage(out)
		return 0
	case "version":
		return runVersion(args[1:], out, errw, in)
	case "login":
		return runLogin(args[1:], out, errw, in)
	case "logout":
		return runLogout(args[1:], out, errw, in)
	case "whoami":
		return runWhoami(args[1:], out, errw, in)
	case "doctor":
		return runDoctor(args[1:], out, errw, in)
	case "project":
		return runProject(args[1:], out, errw, in)
	case "canvas":
		return runCanvas(args[1:], out, errw, in)
	case "task":
		return runTask(args[1:], out, errw, in)
	case "model":
		return runModel(args[1:], out, errw, in)
	case "asset":
		return runAsset(args[1:], out, errw, in)
	case "generate":
		return runGenerate(args[1:], out, errw, in)
	case "credit":
		return runCredit(args[1:], out, errw, in)
	case "run":
		return runRun(args[1:], out, errw, in)
	case "openapi":
		return runOpenAPI(args[1:], out, errw, in)
	case "setup":
		return runSetup(args[1:], out, errw, in)
	default:
		fmt.Fprintf(errw, "未知命令：%s\n\n", args[0])
		usage(errw)
		return 2
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `影策 CLI

用法：
  yingce <命令> [参数]

命令：
  login          浏览器授权登录
  logout         撤销当前 token
  whoami         当前身份
  doctor         诊断连接与认证
  version        版本信息
  project        项目列表/详情
  canvas         画布读取/写入
  task           任务查询/等待/取消
  model          模型目录/报价
  asset          素材列表/上传
  generate       提交图片/视频/音频生成
  credit         额度
  run            云端 Agent 运行
  openapi        保存接口规范
  setup          安装 SKILL.md

通用参数：
  --base-url <url>   服务端地址
  --json             机器可读输出
  --timeout <dur>    单次请求超时
  --debug            脱敏 HTTP 日志

示例：
  yingce login
  yingce project list --json
  yingce canvas get <id> --out canvas.json
  yingce model quote --model <id> --mode video --prompt "..."
  yingce generate --model <id> --mode image --prompt "..."
`)
}

func newFlagSet(name string, a *App) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.Err)
	fs.Usage = func() {
		fmt.Fprintf(a.Err, "用法：yingce %s [参数]\n\n参数：\n", name)
		fs.PrintDefaults()
	}
	return fs
}

func parseFlags(fs *flag.FlagSet, args []string) (int, bool) {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, true
		}
		return 2, true
	}
	return 0, false
}

func runVersion(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(errw)
	g.register(fs)
	if code, done := parseFlags(fs, args); done {
		return code
	}
	if g.JSON {
		payload, _ := json.Marshal(map[string]any{"version": Version, "contract": ContractVersion, "commit": Commit})
		fmt.Fprintln(out, string(payload))
		return 0
	}
	fmt.Fprintf(out, "yingce %s (%s)\n", Version, ContractVersion)
	if Commit != "" {
		fmt.Fprintf(out, "commit %s\n", Commit)
	}
	return 0
}
