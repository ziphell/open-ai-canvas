package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func runProject(args []string, out, errw io.Writer, in io.Reader) int {
	if len(args) == 0 {
		fmt.Fprintln(errw, "用法：yingce project <list|get>")
		return 2
	}
	switch args[0] {
	case "list":
		return runProjectList(args[1:], out, errw, in)
	case "get":
		return runProjectGet(args[1:], out, errw, in)
	default:
		fmt.Fprintf(errw, "未知 project 子命令：%s\n", args[0])
		return 2
	}
}

func runProjectList(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var query string
	var limit int
	fs := newFlagSet("project list", &App{Err: errw})
	g.register(fs)
	fs.StringVar(&query, "query", "", "名称过滤")
	fs.IntVar(&limit, "limit", 20, "最多返回条数")
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
	data, _, err := app.client().Do(context.Background(), http.MethodGet, "/projects", nil, nil, nil)
	if err != nil {
		return app.fail(err)
	}
	items := listField(data, "projects")
	if query != "" {
		lowered := strings.ToLower(query)
		filtered := items[:0]
		for _, item := range items {
			if strings.Contains(strings.ToLower(fmt.Sprint(item["title"])), lowered) ||
				strings.Contains(strings.ToLower(fmt.Sprint(item["name"])), lowered) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	if items == nil {
		items = []map[string]any{}
	}
	return app.fail(app.emit(marshal(map[string]any{"projects": items})))
}

func runProjectGet(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var withOverview bool
	fs := newFlagSet("project get", &App{Err: errw})
	g.register(fs)
	fs.BoolVar(&withOverview, "with-overview", false, "同时获取概览")
	positional, err := splitPositional(args)
	if err != nil {
		fmt.Fprintln(errw, err.Error())
		return 2
	}
	if code, done := parseFlags(fs, positional.flags); done {
		return code
	}
	if positional.value == "" {
		fmt.Fprintln(errw, "用法：yingce project get <id> [--with-overview]")
		return 2
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
	client := app.client()
	project, _, err := client.Do(context.Background(), http.MethodGet, "/projects/"+url.PathEscape(positional.value), nil, nil, nil)
	if err != nil {
		return app.fail(err)
	}
	result := map[string]any{"project": json.RawMessage(project)}
	if withOverview {
		overview, _, overviewErr := client.Do(context.Background(), http.MethodGet, "/projects/"+url.PathEscape(positional.value)+"/overview", nil, nil, nil)
		if overviewErr != nil {
			return app.fail(overviewErr)
		}
		result["overview"] = json.RawMessage(overview)
	}
	return app.fail(app.emit(marshal(result)))
}

func runCanvas(args []string, out, errw io.Writer, in io.Reader) int {
	if len(args) == 0 {
		fmt.Fprintln(errw, "用法：yingce canvas <list|get|apply>")
		return 2
	}
	switch args[0] {
	case "list":
		return runCanvasList(args[1:], out, errw, in)
	case "get":
		return runCanvasGet(args[1:], out, errw, in)
	case "apply":
		return runCanvasApply(args[1:], out, errw, in)
	default:
		fmt.Fprintf(errw, "未知 canvas 子命令：%s\n", args[0])
		return 2
	}
}

func runCanvasList(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var projectID string
	var limit int
	fs := newFlagSet("canvas list", &App{Err: errw})
	g.register(fs)
	fs.StringVar(&projectID, "project", "", "限定项目")
	fs.IntVar(&limit, "limit", 20, "最多返回条数")
	if code, done := parseFlags(fs, args); done {
		return code
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	query := url.Values{}
	if projectID != "" {
		query.Set("projectId", projectID)
		query.Set("page", "1")
		query.Set("pageSize", strconv.Itoa(max(limit, 1)))
	}
	if err := app.requireBaseURL(); err != nil {
		return app.fail(err)
	}
	if err := app.requireToken(); err != nil {
		return app.fail(err)
	}
	data, _, err := app.client().Do(context.Background(), http.MethodGet, "/canvas-projects", query, nil, nil)
	if err != nil {
		return app.fail(err)
	}
	items := listField(data, "projects")
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	if items == nil {
		items = []map[string]any{}
	}
	return app.fail(app.emit(marshal(map[string]any{"canvases": items})))
}

func runCanvasGet(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var outPath string
	var nodesOnly, includeTimeline bool
	fs := newFlagSet("canvas get", &App{Err: errw})
	g.register(fs)
	fs.StringVar(&outPath, "out", "", "写入完整画布文件")
	fs.BoolVar(&nodesOnly, "nodes-only", false, "只输出节点与连线")
	fs.BoolVar(&includeTimeline, "include-timeline", false, "包含时间轴")
	positional, err := splitPositional(args)
	if err != nil {
		fmt.Fprintln(errw, err.Error())
		return 2
	}
	if code, done := parseFlags(fs, positional.flags); done {
		return code
	}
	if positional.value == "" {
		fmt.Fprintln(errw, "用法：yingce canvas get <id> [--out canvas.json]")
		return 2
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
	data, _, err := app.client().Do(context.Background(), http.MethodGet, "/canvas-projects/"+url.PathEscape(positional.value), nil, nil, nil)
	if err != nil {
		return app.fail(err)
	}
	project := fieldRaw(data, "project")
	if len(project) == 0 {
		project = data
	}
	var document map[string]any
	_ = json.Unmarshal(project, &document)
	if nodesOnly {
		reduced := map[string]any{"id": document["id"], "revision": document["revision"]}
		if nodes, ok := document["nodes"]; ok {
			reduced["nodes"] = nodes
		}
		if connections, ok := document["connections"]; ok {
			reduced["connections"] = connections
		}
		project = marshal(reduced)
	}
	if outPath != "" {
		if !filepath.IsAbs(outPath) {
			absolute, absErr := filepath.Abs(outPath)
			if absErr != nil {
				return app.fail(absErr)
			}
			outPath = absolute
		}
		pretty, _ := json.MarshalIndent(json.RawMessage(project), "", "  ")
		if err := os.WriteFile(outPath, append(pretty, '\n'), 0o600); err != nil {
			return app.fail(err)
		}
		summary := map[string]any{
			"id":              document["id"],
			"revision":        document["revision"],
			"path":            outPath,
			"nodeCount":       lengthOf(document["nodes"]),
			"connectionCount": lengthOf(document["connections"]),
		}
		return app.fail(app.emit(marshal(summary)))
	}
	return app.fail(app.emit(project))
}

func runCanvasApply(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var opsPath, mutationID string
	var baseRevision int64
	var dryRun bool
	fs := newFlagSet("canvas apply", &App{Err: errw})
	g.register(fs)
	fs.StringVar(&opsPath, "ops", "", "操作 JSON 文件")
	fs.StringVar(&mutationID, "mutation-id", "", "变更 ID")
	fs.Int64Var(&baseRevision, "base-revision", 0, "读取时的 revision")
	fs.BoolVar(&dryRun, "dry-run", false, "只校验不写入")
	positional, err := splitPositional(args)
	if err != nil {
		fmt.Fprintln(errw, err.Error())
		return 2
	}
	if code, done := parseFlags(fs, positional.flags); done {
		return code
	}
	if positional.value == "" || opsPath == "" {
		fmt.Fprintln(errw, "用法：yingce canvas apply <id> --ops ops.json --base-revision <n> --mutation-id <id>")
		return 2
	}
	payload, err := os.ReadFile(opsPath)
	if err != nil {
		fmt.Fprintf(errw, "读取 ops 文件失败：%v\n", err)
		return 2
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		fmt.Fprintf(errw, "ops 文件不是有效 JSON：%v\n", err)
		return 2
	}
	if mutationID != "" {
		body["mutationId"] = mutationID
	}
	if baseRevision > 0 {
		body["baseRevision"] = baseRevision
	}
	if fmt.Sprint(body["mutationId"]) == "" {
		fmt.Fprintln(errw, "缺少 mutationId：在 ops 文件或 --mutation-id 中提供")
		return 2
	}
	if body["baseRevision"] == nil {
		fmt.Fprintln(errw, "缺少 baseRevision：在 ops 文件或 --base-revision 中提供")
		return 2
	}
	if operations, ok := body["operations"].([]any); !ok || len(operations) == 0 {
		fmt.Fprintln(errw, "ops 文件缺少非空 operations")
		return 2
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	if dryRun {
		return app.fail(app.emit(marshal(map[string]any{
			"dryRun":     true,
			"mutationId": body["mutationId"],
			"operations": len(body["operations"].([]any)),
		})))
	}
	if err := app.requireBaseURL(); err != nil {
		return app.fail(err)
	}
	if err := app.requireToken(); err != nil {
		return app.fail(err)
	}
	data, _, err := app.client().Do(context.Background(), http.MethodPatch, "/canvas-projects/"+url.PathEscape(positional.value)+"/operations", nil, body, nil)
	if err != nil {
		return app.fail(err)
	}
	return app.fail(app.emit(data))
}

func runTask(args []string, out, errw io.Writer, in io.Reader) int {
	if len(args) == 0 {
		fmt.Fprintln(errw, "用法：yingce task <list|get|wait|cancel>")
		return 2
	}
	switch args[0] {
	case "list":
		return runTaskList(args[1:], out, errw, in)
	case "get":
		return runTaskGet(args[1:], out, errw, in)
	case "wait":
		return runTaskWait(args[1:], out, errw, in)
	case "cancel":
		return runTaskCancel(args[1:], out, errw, in)
	default:
		fmt.Fprintf(errw, "未知 task 子命令：%s\n", args[0])
		return 2
	}
}

func runTaskList(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var projectID, status, taskType string
	var limit int
	var recent bool
	fs := newFlagSet("task list", &App{Err: errw})
	g.register(fs)
	fs.StringVar(&projectID, "project", "", "限定项目")
	fs.StringVar(&status, "status", "", "状态过滤")
	fs.StringVar(&taskType, "type", "", "类型过滤")
	fs.IntVar(&limit, "limit", 20, "最多返回条数")
	fs.BoolVar(&recent, "recent", false, "按创建时间倒序")
	if code, done := parseFlags(fs, args); done {
		return code
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	if err := app.ready(); err != nil {
		return app.fail(err)
	}
	query := url.Values{}
	query.Set("pageSize", strconv.Itoa(max(limit, 1)))
	if projectID != "" {
		query.Set("projectId", projectID)
	}
	data, _, err := app.client().Do(context.Background(), http.MethodGet, "/tasks", query, nil, nil)
	if err != nil {
		return app.fail(err)
	}
	var tasks []map[string]any
	_ = json.Unmarshal(data, &tasks)
	if status != "" || taskType != "" {
		filtered := tasks[:0]
		for _, task := range tasks {
			if status != "" && fmt.Sprint(task["status"]) != status {
				continue
			}
			if taskType != "" && fmt.Sprint(task["type"]) != taskType {
				continue
			}
			filtered = append(filtered, task)
		}
		tasks = filtered
	}
	if recent {
		sort.SliceStable(tasks, func(i, j int) bool {
			return fmt.Sprint(tasks[i]["createdAt"]) > fmt.Sprint(tasks[j]["createdAt"])
		})
	}
	if limit > 0 && len(tasks) > limit {
		tasks = tasks[:limit]
	}
	if tasks == nil {
		tasks = []map[string]any{}
	}
	return app.fail(app.emit(marshal(map[string]any{"tasks": tasks})))
}

func runTaskGet(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var outPath string
	fs := newFlagSet("task get", &App{Err: errw})
	g.register(fs)
	fs.StringVar(&outPath, "out", "", "写入任务 JSON")
	positional, err := splitPositional(args)
	if err != nil {
		fmt.Fprintln(errw, err.Error())
		return 2
	}
	if code, done := parseFlags(fs, positional.flags); done {
		return code
	}
	if positional.value == "" {
		fmt.Fprintln(errw, "用法：yingce task get <id> [--out task.json]")
		return 2
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	if err := app.ready(); err != nil {
		return app.fail(err)
	}
	data, _, err := app.client().Do(context.Background(), http.MethodGet, "/tasks/"+url.PathEscape(positional.value), nil, nil, nil)
	if err != nil {
		return app.fail(err)
	}
	if outPath != "" {
		if err := writeJSONFile(outPath, data); err != nil {
			return app.fail(err)
		}
		return app.fail(app.emit(marshal(map[string]any{"id": positional.value, "path": outPath})))
	}
	return app.fail(app.emit(data))
}

func runTaskWait(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var waitTimeout, interval time.Duration
	var downloadDir string
	fs := newFlagSet("task wait", &App{Err: errw})
	g.register(fs)
	fs.DurationVar(&waitTimeout, "max-wait", 600*time.Second, "最长等待时间")
	fs.DurationVar(&interval, "interval", 3*time.Second, "轮询间隔")
	fs.StringVar(&downloadDir, "download-dir", "", "下载产物目录")
	positional, err := splitPositional(args)
	if err != nil {
		fmt.Fprintln(errw, err.Error())
		return 2
	}
	if code, done := parseFlags(fs, positional.flags); done {
		return code
	}
	if positional.value == "" {
		fmt.Fprintln(errw, "用法：yingce task wait <id> [--max-wait 600s] [--download-dir ./out]")
		return 2
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	if err := app.ready(); err != nil {
		return app.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	data, timedOut, err := pollTask(ctx, app, positional.value, interval)
	if err != nil && !timedOut {
		return app.fail(err)
	}
	if downloadDir != "" && !timedOut {
		paths, downloadErr := downloadArtifacts(app, data, downloadDir)
		if downloadErr != nil {
			fmt.Fprintf(errw, "warning: 产物下载失败：%v\n", downloadErr)
		}
		if len(paths) > 0 && app.G.JSON {
			var task map[string]any
			_ = json.Unmarshal(data, &task)
			task["downloaded"] = paths
			data = marshal(task)
		}
	}
	if emitErr := app.emit(data); emitErr != nil {
		return app.fail(emitErr)
	}
	if timedOut {
		return 10
	}
	return 0
}

func runTaskCancel(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	fs := newFlagSet("task cancel", &App{Err: errw})
	g.register(fs)
	positional, err := splitPositional(args)
	if err != nil {
		fmt.Fprintln(errw, err.Error())
		return 2
	}
	if code, done := parseFlags(fs, positional.flags); done {
		return code
	}
	if positional.value == "" {
		fmt.Fprintln(errw, "用法：yingce task cancel <id>")
		return 2
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	if err := app.ready(); err != nil {
		return app.fail(err)
	}
	data, _, err := app.client().Do(context.Background(), http.MethodPost, "/tasks/"+url.PathEscape(positional.value)+"/cancel", nil, nil, nil)
	if err != nil {
		return app.fail(err)
	}
	return app.fail(app.emit(data))
}

func runModel(args []string, out, errw io.Writer, in io.Reader) int {
	if len(args) == 0 {
		fmt.Fprintln(errw, "用法：yingce model <list|quote>")
		return 2
	}
	switch args[0] {
	case "list":
		return runModelList(args[1:], out, errw, in)
	case "quote":
		return runModelQuote(args[1:], out, errw, in)
	default:
		fmt.Fprintf(errw, "未知 model 子命令：%s\n", args[0])
		return 2
	}
}

func runModelList(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var mode string
	fs := newFlagSet("model list", &App{Err: errw})
	g.register(fs)
	fs.StringVar(&mode, "mode", "", "能力过滤：image/video/audio")
	if code, done := parseFlags(fs, args); done {
		return code
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	if err := app.ready(); err != nil {
		return app.fail(err)
	}
	query := url.Values{}
	if mode != "" {
		query.Set("mode", mode)
	}
	data, _, err := app.client().Do(context.Background(), http.MethodGet, "/model-catalog", query, nil, nil)
	if err != nil {
		return app.fail(err)
	}
	return app.fail(app.emit(data))
}

func runModelQuote(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var modelID, channelID, channelModel, mode, operation, prompt, promptFile string
	var duration int
	var size, quality, imageResolution, rawInput string
	var refs, refResources stringList
	fs := newFlagSet("model quote", &App{Err: errw})
	g.register(fs)
	fs.StringVar(&modelID, "model", "", "逻辑模型 ID")
	fs.StringVar(&channelID, "channel", "", "渠道 ID")
	fs.StringVar(&channelModel, "channel-model", "", "渠道模型 key")
	fs.StringVar(&mode, "mode", "", "image/video/audio")
	fs.StringVar(&operation, "operation", "", "操作")
	fs.StringVar(&prompt, "prompt", "", "提示词")
	fs.StringVar(&promptFile, "prompt-file", "", "从文件读取提示词")
	fs.IntVar(&duration, "duration", 0, "视频时长（秒）")
	fs.StringVar(&size, "size", "", "画幅")
	fs.StringVar(&quality, "quality", "", "质量")
	fs.StringVar(&imageResolution, "image-resolution", "", "图片分辨率")
	fs.StringVar(&rawInput, "input", "", "高级 intent JSON")
	fs.Var(&refs, "ref", "参考文件，可重复")
	fs.Var(&refResources, "ref-resource", "已上传素材 ID，可重复")
	if code, done := parseFlags(fs, args); done {
		return code
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	if err := app.ready(); err != nil {
		return app.fail(err)
	}
	intent, err := buildIntent(mode, operation, prompt, promptFile, duration, size, quality, imageResolution, rawInput, len(refs)+len(refResources), 0, 0)
	if err != nil {
		return app.fail(err)
	}
	if (modelID != "") == (channelID != "" && channelModel != "") {
		return app.fail(usagef("需要 --model，或 --channel 与 --channel-model，二选一"))
	}
	var data json.RawMessage
	if modelID != "" {
		data, _, err = app.client().Do(context.Background(), http.MethodPost, "/models/"+url.PathEscape(modelID)+"/quote", nil, intent, nil)
	} else if channelID != "" && channelModel != "" {
		data, _, err = app.client().Do(context.Background(), http.MethodPost, "/model-catalog/quote", nil, map[string]any{
			"channelId": channelID,
			"modelKey":  channelModel,
			"intent":    intent,
		}, nil)
	} else {
		return app.fail(usagef("需要 --model，或 --channel 与 --channel-model"))
	}
	if err != nil {
		return app.fail(err)
	}
	return app.fail(app.emit(data))
}

func runCredit(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	fs := newFlagSet("credit", &App{Err: errw})
	g.register(fs)
	if code, done := parseFlags(fs, args); done {
		return code
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	if err := app.ready(); err != nil {
		return app.fail(err)
	}
	data, _, err := app.client().Do(context.Background(), http.MethodGet, "/wallet", nil, nil, nil)
	if err != nil {
		return app.fail(err)
	}
	return app.fail(app.emit(data))
}

func runOpenAPI(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var outPath string
	fs := newFlagSet("openapi", &App{Err: errw})
	g.register(fs)
	fs.StringVar(&outPath, "out", "", "写入文件")
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
	payload, _, err := app.client().DoRaw(context.Background(), "/openapi.yaml", nil)
	if err != nil {
		return app.fail(err)
	}
	if outPath == "" {
		fmt.Fprint(out, string(payload))
		return 0
	}
	if err := os.WriteFile(outPath, payload, 0o600); err != nil {
		return app.fail(err)
	}
	return app.fail(app.emit(marshal(map[string]any{"path": outPath, "bytes": len(payload)})))
}

// stringList is a repeatable string flag.
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type positionalArgs struct {
	flags []string
	value string
}

// splitPositional extracts one positional argument while keeping flags parseable.
func splitPositional(args []string) (positionalArgs, error) {
	var result positionalArgs
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			if index+1 < len(args) && result.value == "" {
				result.value = args[index+1]
			}
			break
		}
		if strings.HasPrefix(arg, "-") {
			result.flags = append(result.flags, arg)
			if !strings.Contains(arg, "=") && flagTakesValue(arg) && index+1 < len(args) {
				index++
				result.flags = append(result.flags, args[index])
			}
			continue
		}
		if result.value == "" {
			result.value = arg
		}
	}
	return result, nil
}

func flagTakesValue(arg string) bool {
	switch arg {
	case "--base-url", "--token", "--config-dir", "--timeout", "--out", "--ops", "--mutation-id",
		"--base-revision", "--project", "--status", "--type", "--limit", "--query", "--mode", "--model",
		"--channel", "--channel-model", "--operation", "--prompt", "--prompt-file", "--duration",
		"--size", "--quality", "--image-resolution", "--input", "--ref", "--ref-resource",
		"--max-wait", "--interval", "--download-dir", "--agent", "--dir", "--name", "--port", "--kind",
		"--budget-credits", "--budget-tasks", "--budget-video-seconds", "--permission-mode",
		"--reasoning-mode", "--idempotency-key", "--canvas", "--after",
		"--max-credits", "--poll":
		return true
	}
	return false
}

func listField(data json.RawMessage, key string) []map[string]any {
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(data, &wrapper); err == nil {
		for _, name := range []string{key, "items", "projects", "canvases"} {
			if raw, ok := wrapper[name]; ok {
				var items []map[string]any
				if json.Unmarshal(raw, &items) == nil && len(items) > 0 {
					return items
				}
			}
		}
	}
	var direct []map[string]any
	if err := json.Unmarshal(data, &direct); err == nil {
		return direct
	}
	return nil
}

func fieldRaw(data json.RawMessage, key string) json.RawMessage {
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil
	}
	return wrapper[key]
}

func lengthOf(value any) int {
	if items, ok := value.([]any); ok {
		return len(items)
	}
	return 0
}

func writeJSONFile(path string, data json.RawMessage) error {
	if !filepath.IsAbs(path) {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		path = absolute
	}
	pretty, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(pretty, '\n'), 0o600)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
