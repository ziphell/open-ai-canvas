package commands

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runAsset(args []string, out, errw io.Writer, in io.Reader) int {
	if len(args) == 0 {
		fmt.Fprintln(errw, "用法：yingce asset <list|upload>")
		return 2
	}
	switch args[0] {
	case "list":
		return runAssetList(args[1:], out, errw, in)
	case "upload":
		return runAssetUpload(args[1:], out, errw, in)
	default:
		fmt.Fprintf(errw, "未知 asset 子命令：%s\n", args[0])
		return 2
	}
}

func runAssetList(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var projectID, kind string
	var limit int
	fs := newFlagSet("asset list", &App{Err: errw})
	g.register(fs)
	fs.StringVar(&projectID, "project", "", "项目 ID")
	fs.StringVar(&kind, "kind", "", "类型过滤")
	fs.IntVar(&limit, "limit", 20, "最多返回条数")
	if code, done := parseFlags(fs, args); done {
		return code
	}
	if projectID == "" {
		fmt.Fprintln(errw, "需要 --project <id>")
		return 2
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	if err := app.ready(); err != nil {
		return app.fail(err)
	}
	query := url.Values{}
	if kind != "" {
		query.Set("kind", kind)
	}
	data, _, err := app.client().Do(context.Background(), http.MethodGet, "/projects/"+url.PathEscape(projectID)+"/assets", query, nil, nil)
	if err != nil {
		return app.fail(err)
	}
	return app.fail(app.emit(data))
}

func runAssetUpload(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var kind, name, idempotencyKey string
	fs := newFlagSet("asset upload", &App{Err: errw})
	g.register(fs)
	fs.StringVar(&kind, "kind", "", "素材类型：image/video/audio")
	fs.StringVar(&name, "name", "", "名称")
	fs.StringVar(&idempotencyKey, "idempotency-key", "", "幂等键")
	positional, err := splitPositional(args)
	if err != nil {
		fmt.Fprintln(errw, err.Error())
		return 2
	}
	if code, done := parseFlags(fs, positional.flags); done {
		return code
	}
	if positional.value == "" {
		fmt.Fprintln(errw, "用法：yingce asset upload <path> [--kind image]")
		return 2
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	if err := app.ready(); err != nil {
		return app.fail(err)
	}
	resource, err := uploadResource(context.Background(), app, positional.value, kind, idempotencyKey)
	if err != nil {
		return app.fail(err)
	}
	if name != "" {
		resource["name"] = name
	}
	return app.fail(app.emit(marshal(resource)))
}

func runGenerate(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var mode, modelID, channelID, channelModel, operation, prompt, promptFile string
	var projectID, downloadDir, idempotencyKey, size, quality, imageResolution string
	var duration, pollSeconds int
	var maxCredits float64
	var refs, refResources stringList
	fs := newFlagSet("generate", &App{Err: errw})
	g.register(fs)
	fs.StringVar(&mode, "mode", "", "image/video/audio")
	fs.StringVar(&modelID, "model", "", "逻辑模型 ID")
	fs.StringVar(&channelID, "channel", "", "渠道 ID")
	fs.StringVar(&channelModel, "channel-model", "", "渠道模型 key")
	fs.StringVar(&operation, "operation", "", "操作")
	fs.StringVar(&prompt, "prompt", "", "提示词")
	fs.StringVar(&promptFile, "prompt-file", "", "从文件读取提示词")
	fs.StringVar(&projectID, "project", "", "项目 ID")
	fs.StringVar(&downloadDir, "download-dir", "", "下载产物目录")
	fs.StringVar(&idempotencyKey, "idempotency-key", "", "幂等键")
	fs.StringVar(&size, "size", "", "画幅")
	fs.StringVar(&quality, "quality", "", "质量")
	fs.StringVar(&imageResolution, "image-resolution", "", "图片分辨率")
	fs.IntVar(&duration, "duration", 0, "视频时长（秒）")
	fs.IntVar(&pollSeconds, "poll", 0, "提交后等待秒数，0 表示不等待")
	fs.Float64Var(&maxCredits, "max-credits", 0, "报价超过该积分时拒绝提交")
	fs.Var(&refs, "ref", "本地参考文件，可重复")
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
	if mode != "image" && mode != "video" && mode != "audio" {
		return app.fail(usagef("--mode 必须是 image、video 或 audio"))
	}
	hasModel := modelID != ""
	hasChannel := channelID != "" && channelModel != ""
	if hasModel == hasChannel {
		return app.fail(usagef("需要 --model，或 --channel 与 --channel-model，二选一"))
	}
	prompt, err = readPrompt(prompt, promptFile)
	if err != nil {
		return app.fail(err)
	}
	if prompt == "" {
		return app.fail(usagef("需要 --prompt 或 --prompt-file"))
	}
	ctx := context.Background()
	resourceIDs := make([]string, 0, len(refs)+len(refResources))
	for _, path := range refs {
		resource, uploadErr := uploadResource(ctx, app, path, "", "")
		if uploadErr != nil {
			return app.fail(uploadErr)
		}
		if id, ok := resource["id"].(string); ok && id != "" {
			resourceIDs = append(resourceIDs, id)
		}
	}
	resourceIDs = append(resourceIDs, refResources...)

	if operation == "" {
		operation = defaultOperation(mode, len(resourceIDs) > 0)
	}
	intent, err := buildIntent(mode, operation, "", "", duration, size, quality, imageResolution, "", len(resourceIDs), 0, 0)
	if err != nil {
		return app.fail(err)
	}
	if maxCredits > 0 {
		if quoteErr := enforceMaxCredits(ctx, app, modelID, channelID, channelModel, intent, maxCredits); quoteErr != nil {
			return app.fail(quoteErr)
		}
	}
	if idempotencyKey == "" {
		idempotencyKey = randomKey()
	}
	payload := map[string]any{
		"type":      "canvas_" + mode,
		"operation": operation,
		"prompt":    prompt,
		"input":     generationInput(mode, prompt, resourceIDs, duration, size, quality, imageResolution),
	}
	if projectID != "" {
		payload["projectId"] = projectID
	}
	if modelID != "" {
		payload["logicalModelId"] = modelID
	}
	if channelID != "" && channelModel != "" {
		payload["model"] = channelModel
		payload["input"].(map[string]any)["config"] = map[string]any{
			"channelId": channelID,
			"model":     channelModel,
		}
	}
	headers := map[string]string{"X-Idempotency-Key": idempotencyKey}
	data, _, err := app.client().Do(ctx, http.MethodPost, "/tasks", nil, payload, headers)
	if err != nil {
		return app.fail(err)
	}
	var task map[string]any
	_ = json.Unmarshal(data, &task)
	if task["idempotencyKey"] == nil {
		task["idempotencyKey"] = idempotencyKey
	}
	data = marshal(task)
	if pollSeconds > 0 {
		taskID, _ := task["id"].(string)
		if taskID == "" {
			return app.fail(app.emit(data))
		}
		pollCtx, cancel := context.WithTimeout(ctx, time.Duration(pollSeconds)*time.Second)
		defer cancel()
		polled, timedOut, pollErr := pollTask(pollCtx, app, taskID, 3*time.Second)
		if pollErr != nil && !timedOut {
			return app.fail(pollErr)
		}
		var paths []string
		if downloadDir != "" && !timedOut {
			var downloadErr error
			paths, downloadErr = downloadArtifacts(app, polled, downloadDir)
			if downloadErr != nil {
				fmt.Fprintf(errw, "warning: 产物下载失败：%v\n", downloadErr)
			}
		}
		var value map[string]any
		if json.Unmarshal(polled, &value) == nil {
			if value["idempotencyKey"] == nil {
				value["idempotencyKey"] = idempotencyKey
			}
			if len(paths) > 0 {
				value["downloaded"] = paths
			}
			polled = marshal(value)
		}
		if emitErr := app.emit(polled); emitErr != nil {
			return app.fail(emitErr)
		}
		if timedOut {
			return 10
		}
		return 0
	}
	return app.fail(app.emit(data))
}

func runRun(args []string, out, errw io.Writer, in io.Reader) int {
	if len(args) == 0 {
		fmt.Fprintln(errw, "用法：yingce run <create|get|events|cancel>")
		return 2
	}
	switch args[0] {
	case "create":
		return runRunCreate(args[1:], out, errw, in)
	case "get":
		return runRunGet(args[1:], out, errw, in)
	case "events":
		return runRunEvents(args[1:], out, errw, in)
	case "cancel":
		return runRunCancel(args[1:], out, errw, in)
	default:
		fmt.Fprintf(errw, "未知 run 子命令：%s\n", args[0])
		return 2
	}
}

func runRunCreate(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var prompt, promptFile, canvasID, modelID, permissionMode, reasoningMode, idempotencyKey string
	var maxCredits float64
	var maxTasks, maxVideoSeconds int
	fs := newFlagSet("run create", &App{Err: errw})
	g.register(fs)
	fs.StringVar(&prompt, "prompt", "", "提示词")
	fs.StringVar(&promptFile, "prompt-file", "", "从文件读取提示词")
	fs.StringVar(&canvasID, "canvas", "", "画布 ID")
	fs.StringVar(&modelID, "model", "", "逻辑模型 ID")
	fs.StringVar(&permissionMode, "permission-mode", "read_only", "read_only 或 auto")
	fs.StringVar(&reasoningMode, "reasoning-mode", "", "off/auto/deep")
	fs.StringVar(&idempotencyKey, "idempotency-key", "", "幂等键")
	fs.Float64Var(&maxCredits, "budget-credits", 0, "积分预算")
	fs.IntVar(&maxTasks, "budget-tasks", 0, "生成任务预算")
	fs.IntVar(&maxVideoSeconds, "budget-video-seconds", 0, "视频秒数预算")
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
	text, err := readPrompt(prompt, promptFile)
	if err != nil {
		return app.fail(err)
	}
	if text == "" {
		return app.fail(usagef("需要 --prompt 或 --prompt-file"))
	}
	if permissionMode != "read_only" && permissionMode != "auto" {
		return app.fail(usagef("--permission-mode 只允许 read_only 或 auto"))
	}
	if idempotencyKey == "" {
		idempotencyKey = randomKey()
	}
	contextScope := []string{}
	if canvasID != "" {
		contextScope = []string{"canvas"}
	}
	body := map[string]any{
		"canvasId":       canvasID,
		"prompt":         text,
		"permissionMode": permissionMode,
		"contextScope":   contextScope,
		"budget": map[string]any{
			"maxCredits":         maxCredits,
			"maxGenerationTasks": maxTasks,
			"maxVideoSeconds":    maxVideoSeconds,
		},
		"idempotencyKey": idempotencyKey,
	}
	if modelID != "" {
		body["logicalModelId"] = modelID
	}
	if reasoningMode != "" {
		body["reasoningMode"] = reasoningMode
	}
	data, _, err := app.client().Do(context.Background(), http.MethodPost, "/agent/runs", nil, body, nil)
	if err != nil {
		return app.fail(err)
	}
	return app.fail(app.emit(data))
}

func runRunGet(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	fs := newFlagSet("run get", &App{Err: errw})
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
		fmt.Fprintln(errw, "用法：yingce run get <id>")
		return 2
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	if err := app.ready(); err != nil {
		return app.fail(err)
	}
	data, _, err := app.client().Do(context.Background(), http.MethodGet, "/agent/runs/"+url.PathEscape(positional.value), nil, nil, nil)
	if err != nil {
		return app.fail(err)
	}
	return app.fail(app.emit(data))
}

func runRunEvents(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	var after int
	var follow bool
	var maxWait time.Duration
	fs := newFlagSet("run events", &App{Err: errw})
	g.register(fs)
	fs.IntVar(&after, "after", 0, "从该 seq 之后开始")
	fs.BoolVar(&follow, "follow", false, "持续跟随")
	fs.DurationVar(&maxWait, "max-wait", 30*time.Second, "最长读取时间")
	positional, err := splitPositional(args)
	if err != nil {
		fmt.Fprintln(errw, err.Error())
		return 2
	}
	if code, done := parseFlags(fs, positional.flags); done {
		return code
	}
	if positional.value == "" {
		fmt.Fprintln(errw, "用法：yingce run events <id> [--after 0] [--follow]")
		return 2
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	if err := app.ready(); err != nil {
		return app.fail(err)
	}
	query := url.Values{}
	query.Set("after", fmt.Sprint(after))
	endpoint := app.G.BaseURL + "/api/agent/runs/" + url.PathEscape(positional.value) + "/events?" + query.Encode()
	ctx, cancel := context.WithTimeout(context.Background(), maxWait)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return app.fail(err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if app.G.Token != "" {
		req.Header.Set("Authorization", "Bearer "+app.G.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return app.fail(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return app.fail(fmt.Errorf("事件流返回 HTTP %d：%s", resp.StatusCode, strings.TrimSpace(string(payload))))
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		fmt.Fprintln(out, payload)
		if !follow {
			break
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return app.fail(err)
	}
	if ctx.Err() != nil {
		return 10
	}
	return 0
}

func runRunCancel(args []string, out, errw io.Writer, in io.Reader) int {
	var g Global
	fs := newFlagSet("run cancel", &App{Err: errw})
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
		fmt.Fprintln(errw, "用法：yingce run cancel <id>")
		return 2
	}
	app, err := newApp(g, out, errw, in)
	if err != nil {
		return app.fail(err)
	}
	if err := app.ready(); err != nil {
		return app.fail(err)
	}
	data, _, err := app.client().Do(context.Background(), http.MethodPost, "/agent/runs/"+url.PathEscape(positional.value)+"/cancel", nil, nil, nil)
	if err != nil {
		return app.fail(err)
	}
	return app.fail(app.emit(data))
}

func uploadResource(ctx context.Context, app *App, path, kind, idempotencyKey string) (map[string]any, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s 是目录", path)
	}
	if kind == "" {
		kind = kindFromPath(path)
	}
	if info.Size() > chunkUploadThreshold {
		return uploadResourceChunked(ctx, app, path, kind, idempotencyKey, info.Size())
	}
	headers := map[string]string{}
	if idempotencyKey != "" {
		headers["X-Idempotency-Key"] = idempotencyKey
	}
	data, _, err := app.client().Upload(ctx, path, kind, nil, headers)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Resource map[string]any `json:"resource"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if payload.Resource == nil {
		return nil, fmt.Errorf("上传响应缺少 resource")
	}
	return payload.Resource, nil
}

// 超过该大小改用分片上传；后端单片固定 8 MiB，整体文件仍受资源上限约束。
const chunkUploadThreshold = 8 << 20

func uploadResourceChunked(ctx context.Context, app *App, path, kind, idempotencyKey string, size int64) (map[string]any, error) {
	initBody := map[string]any{
		"fileName": filepath.Base(path),
		"kind":     kind,
		"size":     size,
	}
	if idempotencyKey != "" {
		initBody["idempotencyKey"] = idempotencyKey
	}
	data, _, err := app.client().Do(ctx, http.MethodPost, "/resources/uploads", nil, initBody, nil)
	if err != nil {
		return nil, err
	}
	var session struct {
		UploadID   string `json:"uploadId"`
		ChunkSize  int64  `json:"chunkSize"`
		ChunkCount int    `json:"chunkCount"`
	}
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("分片上传响应格式错误: %w", err)
	}
	if session.UploadID == "" || session.ChunkSize <= 0 || session.ChunkCount <= 0 {
		return nil, fmt.Errorf("分片上传响应缺少会话信息")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	for index := 0; index < session.ChunkCount; index++ {
		offset := int64(index) * session.ChunkSize
		length := session.ChunkSize
		if remaining := size - offset; remaining < length {
			length = remaining
		}
		if length <= 0 {
			return nil, fmt.Errorf("分片 %d 大小无效", index)
		}
		if !app.G.Quiet {
			fmt.Fprintf(app.Err, "上传分片 %d/%d\n", index+1, session.ChunkCount)
		}
		section := io.NewSectionReader(file, offset, length)
		if _, _, err := app.client().UploadChunk(ctx, session.UploadID, index, section, length); err != nil {
			return nil, fmt.Errorf("上传分片 %d/%d 失败: %w", index+1, session.ChunkCount, err)
		}
	}

	data, _, err = app.client().Do(ctx, http.MethodPost, "/resources/uploads/"+url.PathEscape(session.UploadID)+"/complete", nil, nil, nil)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Resource map[string]any `json:"resource"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("合并响应格式错误: %w", err)
	}
	if payload.Resource == nil {
		return nil, fmt.Errorf("合并响应缺少 resource")
	}
	return payload.Resource, nil
}

func kindFromPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp":
		return "image"
	case ".mp4", ".mov", ".webm", ".mkv":
		return "video"
	case ".mp3", ".wav", ".m4a", ".aac", ".flac":
		return "audio"
	default:
		return "image"
	}
}

func buildIntent(mode, operation, prompt, promptFile string, duration int, size, quality, imageResolution, rawInput string, imageCount, videoCount, audioCount int) (map[string]any, error) {
	if mode == "" {
		return nil, usagef("需要 --mode")
	}
	options := map[string]any{}
	if duration > 0 {
		options["durationSeconds"] = duration
	}
	if size != "" {
		options["size"] = size
	}
	if quality != "" {
		options["quality"] = quality
	}
	if imageResolution != "" {
		options["imageResolution"] = imageResolution
	}
	intent := map[string]any{
		"capability": mode,
		"operation":  operation,
		"inputs": map[string]int{
			"image": imageCount,
			"video": videoCount,
			"audio": audioCount,
		},
		"options": options,
	}
	if rawInput != "" {
		var extra map[string]any
		if err := json.Unmarshal([]byte(rawInput), &extra); err != nil {
			return nil, usagef("--input 不是有效 JSON：%v", err)
		}
		for key, value := range extra {
			intent[key] = value
		}
	}
	return intent, nil
}

func generationInput(mode, prompt string, resourceIDs []string, duration int, size, quality, imageResolution string) map[string]any {
	input := map[string]any{
		"mode":     mode,
		"prompt":   prompt,
		"metadata": map[string]any{"source": "yingce-cli"},
	}
	refs := make([]map[string]any, 0, len(resourceIDs))
	for _, id := range resourceIDs {
		refs = append(refs, map[string]any{"storageKey": "resource:" + id})
	}
	switch mode {
	case "image":
		input["referenceImages"] = refs
	case "video":
		input["referenceImages"] = refs
	case "audio":
		input["referenceAudios"] = refs
	}
	options := map[string]any{}
	if duration > 0 {
		options["durationSeconds"] = duration
	}
	if size != "" {
		options["size"] = size
	}
	if quality != "" {
		options["quality"] = quality
	}
	if imageResolution != "" {
		options["imageResolution"] = imageResolution
	}
	if len(options) > 0 {
		input["capabilityOptions"] = options
	}
	return input
}

func defaultOperation(mode string, hasReference bool) string {
	switch mode {
	case "image":
		return "image"
	case "audio":
		return "audio"
	default:
		if hasReference {
			return "image2video"
		}
		return "text2video"
	}
}

func enforceMaxCredits(ctx context.Context, app *App, modelID, channelID, channelModel string, intent map[string]any, maxCredits float64) error {
	var data json.RawMessage
	var err error
	if modelID != "" {
		data, _, err = app.client().Do(ctx, http.MethodPost, "/models/"+url.PathEscape(modelID)+"/quote", nil, intent, nil)
	} else if channelID != "" && channelModel != "" {
		data, _, err = app.client().Do(ctx, http.MethodPost, "/model-catalog/quote", nil, map[string]any{
			"channelId": channelID,
			"modelKey":  channelModel,
			"intent":    intent,
		}, nil)
	} else {
		return usagef("需要 --model，或 --channel 与 --channel-model")
	}
	if err != nil {
		return err
	}
	var quote struct {
		Quote struct {
			AmountMicrocredits int64 `json:"amountMicrocredits"`
		} `json:"quote"`
	}
	if err := json.Unmarshal(data, &quote); err != nil {
		return err
	}
	credits := float64(quote.Quote.AmountMicrocredits) / 1e6
	if credits > maxCredits {
		return fmt.Errorf("报价 %.2f 积分超过 --max-credits %.2f", credits, maxCredits)
	}
	return nil
}

func readPrompt(prompt, promptFile string) (string, error) {
	if prompt != "" && promptFile != "" {
		return "", usagef("--prompt 与 --prompt-file 只能二选一")
	}
	if promptFile == "" {
		return strings.TrimSpace(prompt), nil
	}
	payload, err := os.ReadFile(promptFile)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(payload)), nil
}

func randomKey() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("cli-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func pollTask(ctx context.Context, app *App, id string, interval time.Duration) (json.RawMessage, bool, error) {
	if interval <= 0 {
		interval = 3 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		data, _, err := app.client().Do(ctx, http.MethodGet, "/tasks/"+url.PathEscape(id), nil, nil, nil)
		if err != nil {
			if ctx.Err() != nil {
				return nil, true, nil
			}
			return nil, false, err
		}
		var task struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(data, &task)
		switch task.Status {
		case "succeeded", "failed", "cancelled":
			return data, false, nil
		}
		select {
		case <-ctx.Done():
			return data, true, nil
		case <-ticker.C:
		}
	}
}

type artifact struct {
	URL        string
	ResourceID string
	Name       string
}

func downloadArtifacts(app *App, taskData json.RawMessage, dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var task map[string]any
	if err := json.Unmarshal(taskData, &task); err != nil {
		return nil, err
	}
	var collected []artifact
	if result, ok := task["resultJson"].(string); ok && result != "" {
		var parsed any
		if json.Unmarshal([]byte(result), &parsed) == nil {
			collectArtifacts(parsed, &collected)
		}
	}
	collected = uniqueArtifacts(collected)
	if len(collected) == 0 {
		taskID, _ := task["id"].(string)
		path := filepath.Join(dir, "task-"+safeName(taskID)+".json")
		if err := writeJSONFile(path, taskData); err != nil {
			return nil, err
		}
		return []string{path}, fmt.Errorf("任务结果中没有可下载 URL，已保存任务 JSON")
	}
	paths := make([]string, 0, len(collected))
	downloadClient := &http.Client{Timeout: 2 * time.Minute}
	for index, item := range collected {
		target := item.URL
		if target == "" && item.ResourceID != "" {
			target = app.G.BaseURL + "/api/resources/" + url.PathEscape(item.ResourceID) + "/file"
		}
		if strings.HasPrefix(target, "/") {
			target = app.G.BaseURL + target
		}
		if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
			continue
		}
		req, err := http.NewRequest(http.MethodGet, target, nil)
		if err != nil {
			continue
		}
		if app.G.Token != "" {
			req.Header.Set("Authorization", "Bearer "+app.G.Token)
		}
		resp, err := downloadClient.Do(req)
		if err != nil {
			continue
		}
		name := item.Name
		if name == "" {
			name = filepath.Base(req.URL.Path)
		}
		if name == "" || name == "." || name == "/" {
			name = fmt.Sprintf("artifact-%d", index+1)
		}
		if extension := extensionFor(resp.Header.Get("Content-Type")); extension != "" && filepath.Ext(name) == "" {
			name += extension
		}
		path := filepath.Join(dir, safeName(name))
		file, err := os.Create(path)
		if err != nil {
			_ = resp.Body.Close()
			continue
		}
		_, copyErr := io.Copy(file, resp.Body)
		_ = file.Close()
		_ = resp.Body.Close()
		if copyErr == nil {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

func collectArtifacts(value any, out *[]artifact) {
	switch typed := value.(type) {
	case map[string]any:
		item := artifact{}
		if urlValue, ok := typed["url"].(string); ok {
			item.URL = urlValue
		}
		if id, ok := typed["resourceId"].(string); ok {
			item.ResourceID = id
		}
		if key, ok := typed["storageKey"].(string); ok && strings.HasPrefix(key, "resource:") {
			item.ResourceID = strings.TrimPrefix(key, "resource:")
		}
		if name, ok := typed["name"].(string); ok {
			item.Name = name
		}
		if item.URL != "" || item.ResourceID != "" {
			*out = append(*out, item)
		}
		for _, nested := range typed {
			collectArtifacts(nested, out)
		}
	case []any:
		for _, nested := range typed {
			collectArtifacts(nested, out)
		}
	}
}

func uniqueArtifacts(items []artifact) []artifact {
	seen := map[string]bool{}
	result := make([]artifact, 0, len(items))
	for _, item := range items {
		key := item.URL + "|" + item.ResourceID
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item)
	}
	return result
}

func safeName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\\", "_")
	value = strings.ReplaceAll(value, "/", "_")
	if value == "" || value == "." || value == ".." {
		return "artifact"
	}
	return value
}

func extensionFor(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	extensions, _ := mime.ExtensionsByType(mediaType)
	if len(extensions) == 0 {
		return ""
	}
	return extensions[0]
}
