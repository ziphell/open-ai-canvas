// 声明式插件的运行期适配器（manifestAdapter）：按清单组装请求、解析响应中的任务 ID、状态与媒体。
//
// 媒体提取同时支持 JSON 路径、Markdown 图片、HTML 视频标签与内联 data URL，
// 兼容不同上游的返回形态。

package protocol

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strings"
)

type manifestAdapter struct{ manifest Manifest }

func (a manifestAdapter) Metadata() Metadata { return a.manifest.Metadata }

func (a manifestAdapter) AgentAvailable() bool {
	return a.manifest.Agent != nil && a.manifest.AgentResponse != nil
}

func (a manifestAdapter) ResultAvailable() bool { return a.manifest.ResultOperation != nil }

func (a manifestAdapter) UploadKinds() []string {
	if a.manifest.Upload == nil {
		return nil
	}
	return a.manifest.Upload.Kinds
}

func (a manifestAdapter) BuildUpload(_ context.Context, c RequestContext, media MediaReference) (RequestSpec, error) {
	if a.manifest.Upload == nil {
		return RequestSpec{}, fmt.Errorf("protocol %s has no upload operation", a.manifest.Metadata.ID)
	}
	values := manifestMediaValues([]MediaReference{media})
	return buildManifestOperationWithEnv(a.manifest.Upload.ManifestOperation, a.manifest.Auth, c.Request, "", map[string]any{"media": values[0]})
}

func (a manifestAdapter) ParseUpload(_ context.Context, body []byte) (string, error) {
	if a.manifest.Upload == nil {
		return "", fmt.Errorf("protocol %s has no upload operation", a.manifest.Metadata.ID)
	}
	payload, err := decodeObject(body)
	if err != nil {
		return "", err
	}
	id := manifestResponseString(a.manifest.Upload.ID, map[string]any{"response": payload})
	if id == "" {
		return "", fmt.Errorf("上游素材上传没有返回素材 ID")
	}
	return id, nil
}

func (a manifestAdapter) BuildCreate(_ context.Context, c RequestContext) (RequestSpec, error) {
	if len(a.manifest.Contributes.Providers) == 0 {
		return RequestSpec{}, fmt.Errorf("plugin %s does not provide a provider", a.manifest.Metadata.ID)
	}
	request, err := ApplyImageParameters(c.Request, a.manifest.Metadata.ImageParameters)
	if err != nil {
		return RequestSpec{}, err
	}
	if err := validateManifestRequest(a.manifest.Validations, request); err != nil {
		return RequestSpec{}, err
	}
	spec, err := buildManifestOperation(a.manifest.Create, a.manifest.Auth, request, "")
	if err != nil {
		return RequestSpec{}, err
	}
	return completeCommonImageRequest(spec, a.manifest.Create, request)
}

func (a manifestAdapter) BuildAgent(_ context.Context, c AgentRequestContext) (RequestSpec, error) {
	if a.manifest.Agent == nil {
		return RequestSpec{}, fmt.Errorf("protocol %s has no agent operation", a.manifest.Metadata.ID)
	}
	request := GenerationRequest{Model: c.Model, Extra: map[string]any{"agent": c.Request}}
	return buildManifestOperation(*a.manifest.Agent, a.manifest.Auth, request, "")
}

func (a manifestAdapter) ParseAgent(_ context.Context, body []byte) (AgentResult, error) {
	if a.manifest.AgentResponse == nil {
		return AgentResult{}, fmt.Errorf("protocol %s has no agent response mapping", a.manifest.Metadata.ID)
	}
	payload, err := decodeObject(body)
	if err != nil {
		return AgentResult{}, err
	}
	response := a.manifest.AgentResponse
	result := AgentResult{Text: firstPathValue(payload, response.TextPaths...), Reasoning: firstPathValue(payload, response.ReasoningPaths...)}
	if response.ToolCallsPath == "" {
		return result, nil
	}
	for index, item := range arrayValue(pathValue(payload, response.ToolCallsPath)) {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		call := AgentToolCall{
			ID:        firstPathValue(object, response.ToolCallIDPaths...),
			Name:      firstPathValue(object, response.ToolCallNamePaths...),
			Arguments: firstPathJSONValue(object, response.ToolCallArgsPaths...),
		}
		// Signatures are opaque protocol state, not display text to normalize.
		for _, path := range response.ToolCallSignaturePaths {
			if signature, ok := pathValue(object, path).(string); ok && signature != "" {
				call.ThoughtSignature = signature
				break
			}
		}
		if strings.TrimSpace(call.Name) != "" {
			if strings.TrimSpace(call.ID) == "" {
				call.ID = syntheticAgentToolCallID(body, index)
			}
			result.ToolCalls = append(result.ToolCalls, call)
		}
	}
	return result, nil
}

func syntheticAgentToolCallID(body []byte, index int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s#%d", body, index)))
	return fmt.Sprintf("call_%x", sum[:8])
}

func (a manifestAdapter) ParseCreate(ctx context.Context, body []byte) (CreateResult, error) {
	return a.ParseCreateWithRequest(ctx, GenerationRequest{}, body)
}

func (a manifestAdapter) ParseCreateWithRequest(ctx context.Context, request GenerationRequest, body []byte) (CreateResult, error) {
	return a.ParseCreateWithRequestAndHeaders(ctx, request, nil, body)
}

// ParseCreateWithRequestAndHeaders 是带响应头的创建响应解析；空响应体按空对象处理，
// 供只在响应头返回任务状态的上游协议使用。
func (a manifestAdapter) ParseCreateWithRequestAndHeaders(_ context.Context, request GenerationRequest, headers http.Header, body []byte) (CreateResult, error) {
	if a.manifest.Response.StreamedJSONAudio {
		format := ""
		if request.Extra != nil {
			format = manifestString(request.Extra["audioFormat"])
		}
		return streamedJSONAudioCreateResult(body, a.manifest.Response.ResultKind, format)
	}
	if a.manifest.Response.BinaryPayload {
		return binaryPayloadCreateResult(a.manifest.Response.ResultKind, body)
	}
	payload, err := a.decodeBody(body)
	if err != nil {
		return CreateResult{}, err
	}
	// 响应映射可读取 request.*（如以宿主幂等键作为上游任务 ID 的协议）。
	return a.parseWithEnv(payload, PollContext{Model: request.Model, Request: request}, a.responseHeaderEnv(headers)), nil
}

// streamedJSONAudioCreateResult decodes the chunked JSON response used by
// Doubao's unidirectional TTS endpoint. Go exposes HTTP chunked transfer as a
// single byte stream, so the decoder accepts both concatenated JSON objects and
// newline/SSE-style data frames.
const maxStreamedJSONAudioBytes = 64 << 20

func streamedJSONAudioCreateResult(body []byte, resultKind string, format string) (CreateResult, error) {
	if len(body) == 0 {
		return CreateResult{}, fmt.Errorf("streamed JSON audio response is empty")
	}
	if len(body) > maxStreamedJSONAudioBytes {
		return CreateResult{}, fmt.Errorf("streamed JSON audio response exceeds %d bytes", maxStreamedJSONAudioBytes)
	}
	frames, err := decodeJSONFrames(body)
	if err != nil {
		return CreateResult{}, fmt.Errorf("decode streamed JSON audio response: %w", err)
	}
	var audio bytes.Buffer
	var usage map[string]any
	for _, frame := range frames {
		if event := strings.ToLower(strings.TrimSpace(manifestString(firstNonNilPathValue(frame, "event")))); event == "error" {
			message := strings.TrimSpace(manifestString(firstNonNilPathValue(frame, "error.message", "message", "msg")))
			if message == "" {
				message = "上游返回失败状态"
			}
			code := strings.TrimSpace(manifestString(firstNonNilPathValue(frame, "error.code", "code")))
			if code != "" {
				return CreateResult{}, fmt.Errorf("流式音频合成失败（code %s）：%s", code, message)
			}
			return CreateResult{}, fmt.Errorf("流式音频合成失败：%s", message)
		}
		code := strings.ToLower(strings.TrimSpace(manifestString(firstNonNilPathValue(frame, "code", "status_code", "statusCode"))))
		if code != "" && code != "0" && code != "ok" && code != "success" && code != "succeeded" {
			message := strings.TrimSpace(manifestString(firstNonNilPathValue(frame, "message", "msg", "error.message")))
			if message == "" {
				message = "上游返回失败状态"
			}
			return CreateResult{}, fmt.Errorf("流式音频合成失败（code %s）：%s", code, message)
		}
		if value := firstPathValue(frame, "data", "audio", "audio.data"); value != "" {
			decoded, decodeErr := decodeStreamedAudioBase64(value)
			if decodeErr != nil {
				return CreateResult{}, fmt.Errorf("decode streamed audio data: %w", decodeErr)
			}
			_, _ = audio.Write(decoded)
		}
		if rawUsage := pathValue(frame, "usage"); rawUsage != nil {
			usage = manifestObject(rawUsage)
		}
	}
	if audio.Len() == 0 {
		return CreateResult{}, fmt.Errorf("streamed JSON audio response contains no audio data")
	}
	mimeType := streamedAudioMIMEType(format)
	reference := MediaReference{DataURL: "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(audio.Bytes()), MIMEType: mimeType}
	return CreateResult{Status: StatusSucceeded, Result: &Result{Audios: []MediaReference{reference}, Usage: usage}}, nil
}

func decodeJSONFrames(body []byte) ([]map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	frames := make([]map[string]any, 0, 4)
	for {
		var frame map[string]any
		err := decoder.Decode(&frame)
		if err == io.EOF {
			return frames, nil
		}
		if err != nil {
			break
		}
		if frame != nil {
			frames = append(frames, frame)
		}
	}
	// Some gateways wrap each JSON object in an SSE-like "data:" line.
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if line == "" || line == "[DONE]" {
			continue
		}
		var frame map[string]any
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			return nil, err
		}
		frames = append(frames, frame)
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("no JSON frames found")
	}
	return frames, nil
}

func streamedAudioMIMEType(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "wav":
		return "audio/wav"
	case "ogg_opus", "ogg":
		return "audio/ogg"
	case "pcm":
		return "audio/pcm"
	default:
		return "audio/mpeg"
	}
}

func firstNonNilPathValue(payload map[string]any, paths ...string) any {
	for _, path := range paths {
		if value := pathValue(payload, path); value != nil {
			return value
		}
	}
	return nil
}

func decodeStreamedAudioBase64(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err == nil {
		return decoded, nil
	}
	decoded, rawErr := base64.RawStdEncoding.DecodeString(value)
	if rawErr == nil {
		return decoded, nil
	}
	return nil, err
}

// binaryPayloadCreateResult wraps a synchronous binary response as one media result.
func binaryPayloadCreateResult(resultKind string, body []byte) (CreateResult, error) {
	if len(body) == 0 {
		return CreateResult{}, fmt.Errorf("binary payload response is empty")
	}
	mimeType := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(body), ";")[0]))
	reference := MediaReference{DataURL: "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(body), MIMEType: mimeType}
	result := &Result{}
	switch resultKind {
	case "audio":
		result.Audios = []MediaReference{reference}
	case "image":
		result.Images = []MediaReference{reference}
	case "video":
		result.Videos = []MediaReference{reference}
	default:
		return CreateResult{}, fmt.Errorf("binary payload response requires resultKind image, video or audio, got %q", resultKind)
	}
	return CreateResult{Status: StatusSucceeded, Result: result}, nil
}

func (a manifestAdapter) BuildPoll(_ context.Context, c PollContext) (RequestSpec, error) {
	if a.manifest.Poll == nil {
		return RequestSpec{}, fmt.Errorf("protocol %s has no poll operation", a.manifest.Metadata.ID)
	}
	request := c.Request
	request.Model = c.Model
	return buildManifestOperation(*a.manifest.Poll, a.manifest.Auth, request, c.TaskID)
}

func (a manifestAdapter) ParsePoll(ctx context.Context, c PollContext, body []byte) (PollResult, error) {
	return a.ParsePollWithHeaders(ctx, c, nil, body)
}

// ParsePollWithHeaders 允许查询响应通过 headers.* 映射业务状态（如 X-Api-Status-Code）。
func (a manifestAdapter) ParsePollWithHeaders(_ context.Context, c PollContext, headers http.Header, body []byte) (PollResult, error) {
	payload, err := a.decodeBody(body)
	if err != nil {
		return PollResult{}, err
	}
	result := a.parseWithEnv(payload, c, a.responseHeaderEnv(headers))
	return PollResult{TaskID: result.TaskID, ResultID: result.ResultID, Status: result.Status, Result: result.Result, Message: result.Message, Stage: result.Stage, QueueAhead: result.QueueAhead}, nil
}

// responseHeaderEnv 按清单的 response.headers 映射读取响应头，值统一为去空白字符串。
func (a manifestAdapter) responseHeaderEnv(headers http.Header) map[string]any {
	if len(a.manifest.Response.Headers) == 0 {
		return nil
	}
	values := make(map[string]any, len(a.manifest.Response.Headers))
	for name, header := range a.manifest.Response.Headers {
		values[name] = strings.TrimSpace(headers.Get(header))
	}
	return map[string]any{"headers": values}
}

// decodeBody 只对声明了响应头映射的协议容忍空响应体：这类上游（火山引擎）
// 把状态码放在响应头，成功和失败的 body 都可能为空；普通协议仍要求 JSON 响应。
func (a manifestAdapter) decodeBody(body []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(body)) == 0 && len(a.manifest.Response.Headers) > 0 {
		return map[string]any{}, nil
	}
	return decodeObject(body)
}

func (a manifestAdapter) BuildCancel(_ context.Context, c PollContext) (RequestSpec, error) {
	if a.manifest.Cancel == nil {
		return RequestSpec{}, fmt.Errorf("protocol %s does not support cancellation", a.manifest.Metadata.ID)
	}
	request := c.Request
	request.Model = c.Model
	return buildManifestOperation(*a.manifest.Cancel, a.manifest.Auth, request, c.TaskID)
}

func (a manifestAdapter) BuildResult(_ context.Context, c PollContext) (RequestSpec, error) {
	if a.manifest.ResultOperation == nil {
		return RequestSpec{}, fmt.Errorf("protocol %s has no result operation", a.manifest.Metadata.ID)
	}
	request := c.Request
	request.Model = c.Model
	return buildManifestOperationWithEnv(*a.manifest.ResultOperation, a.manifest.Auth, request, c.TaskID, map[string]any{"resultId": c.ResultID})
}

func (a manifestAdapter) parse(payload map[string]any, c PollContext) CreateResult {
	return a.parseWithEnv(payload, c, nil)
}

func (a manifestAdapter) parseWithEnv(payload map[string]any, c PollContext, extra map[string]any) CreateResult {
	response := a.manifest.Response
	env := map[string]any{"response": payload, "taskId": c.TaskID, "request": manifestRequestValues(c.Request)}
	for key, value := range extra {
		if _, reserved := env[key]; !reserved {
			env[key] = value
		}
	}
	id := manifestResponseString(response.TaskID, env)
	if id == "" {
		id = firstPathValue(payload, response.TaskIDPaths...)
	}
	if id == "" {
		id = c.TaskID
	}
	resultID := manifestResponseString(response.ResultID, env)
	statusText := manifestResponseString(response.Status, env)
	if statusText == "" {
		statusText = firstPathValue(payload, response.StatusPaths...)
	}
	status := NormalizeStatus(statusText)
	if status == "" {
		status = StatusPending
	}
	message := manifestResponseString(response.Message, env)
	if message == "" {
		message = firstPathValue(payload, response.MessagePaths...)
	}
	if manifestError(payload, response.ErrorPaths...) {
		status = StatusFailed
	}
	stage := manifestResponseString(response.Stage, env)
	queueAhead := -1
	if response.QueueAhead != nil {
		if value, err := evaluateManifestValue(response.QueueAhead, env); err == nil && value != nil {
			queueAhead = max(0, manifestInt(value))
		}
	}
	result := &Result{
		Text:      manifestResponseString(response.Text, env),
		Reasoning: manifestResponseString(response.Reasoning, env),
	}
	if result.Text == "" {
		result.Text = firstPathValue(payload, response.TextPaths...)
	}
	if result.Reasoning == "" {
		result.Reasoning = firstPathValue(payload, response.ReasoningPaths...)
	}
	result.Images = manifestResponseMedia(response.Images, env, "image", response.ResultEphemeral)
	result.Videos = manifestResponseMedia(response.Videos, env, "video", response.ResultEphemeral)
	result.Audios = manifestResponseMedia(response.Audios, env, "audio", response.ResultEphemeral)
	if response.Usage != nil {
		if value, err := evaluateManifestValue(response.Usage, env); err == nil {
			result.Usage = manifestObject(value)
		}
	}
	if response.Transcription != nil {
		result.Transcription = manifestTranscription(response.Transcription, env)
	}
	paths := append([]string{}, a.manifest.Response.ResultURLPaths...)
	paths = append(paths, a.manifest.Response.ResultPaths...)
	for _, path := range paths {
		for _, value := range mediaPathValues(payload, path) {
			item := MediaReference{URL: value, Kind: a.manifest.Response.ResultKind, Ephemeral: a.manifest.Response.ResultEphemeral}
			switch a.manifest.Response.ResultKind {
			case "image":
				result.Images = append(result.Images, item)
			case "audio":
				result.Audios = append(result.Audios, item)
			default:
				result.Videos = append(result.Videos, item)
			}
		}
	}
	if status == StatusPending && (result.Text != "" || len(result.Images) > 0 || len(result.Videos) > 0 || len(result.Audios) > 0) {
		status = StatusSucceeded
	}
	// 独立下载接口稍后提供媒体时，保留轮询中的实际时长和清晰度。
	keepUsageForDownload := a.manifest.ResultOperation != nil && len(result.Usage) > 0
	if result.Text == "" && result.Reasoning == "" && len(result.Images) == 0 && len(result.Videos) == 0 && len(result.Audios) == 0 && result.Transcription == nil && !keepUsageForDownload {
		result = nil
	}
	return CreateResult{TaskID: id, ResultID: resultID, Status: status, Result: result, Message: message, Stage: stage, QueueAhead: queueAhead}
}

func validateManifestRequest(rules []ManifestValidation, request GenerationRequest) error {
	if len(rules) == 0 {
		return nil
	}
	env := map[string]any{"request": manifestRequestValues(request)}
	for _, rule := range rules {
		value, err := evaluateManifestValue(rule.Assert, env)
		if err != nil {
			return fmt.Errorf("协议参数校验表达式错误：%w", err)
		}
		if !manifestTruthy(value) {
			return fmt.Errorf("%s", strings.TrimSpace(rule.Message))
		}
	}
	return nil
}

func manifestResponseString(template any, env map[string]any) string {
	if template == nil {
		return ""
	}
	value, err := evaluateManifestValue(template, env)
	if err != nil {
		return ""
	}
	items := manifestArray(value)
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if text := strings.TrimSpace(manifestString(item)); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "")
}

func manifestTranscription(template *ManifestTranscription, env map[string]any) *TranscriptionResult {
	if template == nil {
		return nil
	}
	language := strings.TrimSpace(manifestResponseString(template.Language, env))
	if template.Segments == nil {
		if language == "" {
			return nil
		}
		return &TranscriptionResult{Language: language}
	}
	value, err := evaluateManifestValue(template.Segments, env)
	if err != nil {
		return nil
	}
	segments := make([]TranscriptionSegment, 0)
	for _, item := range manifestArray(value) {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		text := strings.TrimSpace(firstString(entry, "text", "content"))
		if text == "" {
			continue
		}
		startMs := int64(math.Round(manifestFloat(firstNonNilPathValue(entry, "start", "startTime")) * 1000))
		endMs := int64(math.Round(manifestFloat(firstNonNilPathValue(entry, "end", "endTime")) * 1000))
		if raw := firstNonNilPathValue(entry, "startMs", "start_ms"); raw != nil {
			startMs = int64(manifestInt(raw))
		}
		if raw := firstNonNilPathValue(entry, "endMs", "end_ms"); raw != nil {
			endMs = int64(manifestInt(raw))
		}
		if endMs < startMs {
			endMs = startMs
		}
		segments = append(segments, TranscriptionSegment{StartMs: startMs, EndMs: endMs, Text: text})
	}
	if len(segments) == 0 {
		return nil
	}
	return &TranscriptionResult{Segments: segments, Language: language}
}

func manifestResponseMedia(template any, env map[string]any, kind string, ephemeral bool) []MediaReference {
	if template == nil {
		return nil
	}
	value, err := evaluateManifestValue(template, env)
	if err != nil {
		return nil
	}
	return mediaReferencesFromManifestValue(value, kind, ephemeral)
}

func mediaReferencesFromManifestValue(value any, kind string, ephemeral bool) []MediaReference {
	result := make([]MediaReference, 0)
	for _, item := range manifestArray(value) {
		switch typed := item.(type) {
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed == "" {
				continue
			}
			reference := MediaReference{Kind: kind, Ephemeral: ephemeral}
			if strings.HasPrefix(trimmed, "data:") {
				reference.DataURL = trimmed
			} else {
				reference.URL = trimmed
			}
			result = append(result, reference)
		case map[string]any:
			reference := MediaReference{
				ID: manifestString(typed["id"]), URL: manifestString(typed["url"]), DataURL: manifestString(typed["dataUrl"]),
				Kind: defaultValue(manifestString(typed["kind"]), kind), Role: manifestString(typed["role"]), MIMEType: manifestString(typed["mimeType"]),
				Name: manifestString(typed["name"]), Order: manifestInt(typed["order"]), Weight: manifestFloat(typed["weight"]), Ephemeral: ephemeral || manifestTruthy(typed["ephemeral"]),
			}
			reference.Metadata = make(map[string]any)
			metadata, _ := typed["metadata"].(map[string]any)
			for _, key := range []string{"size", "z_index", "bounding_box", "description", "text_recognition", "background", "output_format"} {
				if value, exists := typed[key]; exists {
					reference.Metadata[key] = value
				} else if value, exists := metadata[key]; exists {
					reference.Metadata[key] = value
				}
			}
			if len(reference.Metadata) == 0 {
				reference.Metadata = nil
			}
			if reference.URL == "" {
				reference.URL = firstString(typed, "file_url", "fileUrl", "image_url", "imageUrl", "video_url", "videoUrl", "audio_url", "audioUrl", "uri")
			}
			if reference.DataURL == "" {
				reference.DataURL = firstString(typed, "data_url", "b64_json")
			}
			// OpenAI / Ark 等渠道常直接返回裸 b64_json；声明式结果下载要求 data URL。
			reference.DataURL = normalizeManifestInlineDataURL(reference.DataURL, reference.Kind, firstString(typed, "output_format", "mime_type", "mimeType"))
			if reference.URL != "" || reference.DataURL != "" {
				result = append(result, reference)
			}
		}
	}
	return result
}

func normalizeManifestInlineDataURL(value, kind, formatHint string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "data:") {
		return value
	}
	// 某些 Images 网关不返回 output_format；按文件头识别，避免把 GIF 动画标为 PNG。
	if kind == "image" {
		prefix := value[:min(len(value), 48)]
		if data, err := base64.StdEncoding.DecodeString(prefix); err == nil {
			if detected := http.DetectContentType(data); strings.HasPrefix(detected, "image/") {
				formatHint = detected
			}
		}
	}
	return "data:" + manifestInlineMediaMIME(kind, formatHint) + ";base64," + value
}

func manifestInlineMediaMIME(kind, formatHint string) string {
	hint := strings.ToLower(strings.TrimSpace(formatHint))
	switch hint {
	case "image/png", "image/jpeg", "image/webp", "image/gif", "audio/mpeg", "audio/wav", "audio/ogg", "video/mp4", "video/webm":
		return hint
	case "image/jpg":
		return "image/jpeg"
	case "audio/mp3":
		return "audio/mpeg"
	case "png":
		return "image/png"
	case "jpeg", "jpg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	case "gif":
		return "image/gif"
	case "mp3", "mpeg":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "mp4":
		return "video/mp4"
	case "webm":
		return "video/webm"
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "audio":
		return "audio/mpeg"
	case "video":
		return "video/mp4"
	default:
		return "image/png"
	}
}

var (
	manifestMDImageRegex   = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)\)`)
	manifestHTMLVideoRegex = regexp.MustCompile(`(?i)<video[^>]+src=['"]([^'"]+)['"]`)
)

func mediaPathValues(payload map[string]any, path string) []string {
	value := pathValue(payload, path)
	if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
		trimmed := strings.TrimSpace(text)
		if matches := manifestMDImageRegex.FindAllStringSubmatch(trimmed, -1); len(matches) > 0 {
			var res []string
			for _, m := range matches {
				if len(m) > 1 && m[1] != "" {
					res = append(res, m[1])
				}
			}
			if len(res) > 0 {
				return res
			}
		}
		if matches := manifestHTMLVideoRegex.FindAllStringSubmatch(trimmed, -1); len(matches) > 0 {
			var res []string
			for _, m := range matches {
				if len(m) > 1 && m[1] != "" {
					res = append(res, m[1])
				}
			}
			if len(res) > 0 {
				return res
			}
		}
		return []string{trimmed}
	}
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		switch typed := item.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				values = append(values, strings.TrimSpace(typed))
			}
		case map[string]any:
			if url := firstString(typed, "url", "file_url", "fileUrl", "video_url", "videoUrl", "image_url", "imageUrl"); url != "" {
				values = append(values, url)
			}
		}
	}
	return values
}
