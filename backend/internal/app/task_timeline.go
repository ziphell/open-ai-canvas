package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"yingce/backend/internal/model"
	"yingce/backend/internal/protocol"
	"yingce/backend/internal/repository"
)

// timeline 转写任务输入，由画布提交，字段与前端契约一致。
type timelineTranscriptionInput struct {
	ResourceID           string                      `json:"resourceId"`
	Language             string                      `json:"language"`
	Mode                 string                      `json:"mode,omitempty"`
	Config               timelineTranscriptionConfig `json:"config,omitempty"`
	TranscriptionSeconds int64                       `json:"transcriptionSeconds,omitempty"`
}

type timelineTranscriptionConfig struct {
	ChannelID       string `json:"channelId,omitempty"`
	ChannelModelKey string `json:"channelModelKey,omitempty"`
	Model           string `json:"model,omitempty"`
	PriceTierID     string `json:"priceTierId,omitempty"`
}

// processTimelineTranscription 执行时间线字幕转写：
// 读取资源 -> ffmpeg 预处理为 16k 单声道 wav -> 按渠道模型协议转写 ->
// 结果写回任务（segments + srt）。任何失败都落到任务终态并携带用户可读原因。
func (w *taskWorkerCoordinator) processTimelineTranscription(task *model.Task, ctx context.Context) error {
	s := w.service
	decrypted, err := s.decryptTaskInputJSON(task.InputJSON)
	if err != nil {
		return w.failTimelineTask(task, "转写失败", "任务输入无法解密")
	}
	var input timelineTranscriptionInput
	if err := json.Unmarshal([]byte(decrypted), &input); err != nil || strings.TrimSpace(input.ResourceID) == "" {
		return w.failTimelineTask(task, "转写失败", "任务缺少有效的资源引用")
	}
	if err := s.RequireFeature(FeatureTimelineTranscription); err != nil {
		return w.failTimelineTask(task, "转写失败", "字幕转写暂未开放")
	}
	if strings.TrimSpace(input.Config.ChannelID) == "" || (strings.TrimSpace(input.Config.ChannelModelKey) == "" && strings.TrimSpace(input.Config.Model) == "") {
		_ = s.RefundBilling(task.BillingOrderID, "未配置转写模型")
		return w.failTimelineTask(task, "转写失败", "未配置转写模型，请在管理后台配置转写协议和渠道模型")
	}
	if err := s.MarkBillingRunning(task.BillingOrderID); err != nil {
		_ = s.RefundBilling(task.BillingOrderID, "转写计费状态更新失败")
		return w.failTimelineTask(task, "转写失败", "转写计费状态更新失败")
	}
	s.logInfo(task.UserID, task.ID, "时间线转写任务开始", "")

	resource, reader, err := s.OpenResource(task.UserID, input.ResourceID)
	if err != nil || reader == nil {
		_ = s.RefundBilling(task.BillingOrderID, "无法读取待转写媒体")
		return w.failTimelineTask(task, "转写失败", "无法读取待转写媒体，可能已被删除")
	}
	defer reader.Close()
	if resource == nil || !isTranscribableMime(resource.MimeType) {
		_ = s.RefundBilling(task.BillingOrderID, "不支持的转写媒体类型")
		return w.failTimelineTask(task, "转写失败", "仅支持音视频文件转写")
	}

	if err := w.progress(task, "等待转写服务", 15); err != nil {
		return err
	}
	wavPath, cleanup, err := prepareWhisperWav(ctx, reader, resource.MimeType)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		_ = s.RefundBilling(task.BillingOrderID, "音频预处理失败")
		return w.failTimelineTask(task, "转写失败", err.Error())
	}
	durationMs := probeTranscriptionDurationMs(ctx, wavPath)
	if durationMs <= 0 {
		durationMs = resource.DurationMs
	}
	if durationMs <= 0 {
		// 16 kHz 单声道 16 bit PCM 每秒 32000 字节；无 ffprobe 时按文件大小兜底。
		if info, statErr := os.Stat(wavPath); statErr == nil && info.Size() > 0 {
			durationMs = info.Size() * 1000 / 32000
		}
	}
	if err := w.progress(task, "正在转写…", 40); err != nil {
		return err
	}
	segments, language, err := w.transcribeViaProtocol(ctx, task, &input, wavPath)
	if err != nil {
		_ = s.RefundBilling(task.BillingOrderID, err.Error())
		return w.failTimelineTask(task, "转写失败", err.Error())
	}
	if err := w.progress(task, "整理字幕…", 80); err != nil {
		return err
	}

	result := timelineTranscriptionResult{
		Segments: segments,
		SRT:      buildTimelineSRT(segments),
		Language: language,
	}
	payload, err := json.Marshal(result)
	if err != nil {
		_ = s.RefundBilling(task.BillingOrderID, "转写结果序列化失败")
		return w.failTimelineTask(task, "转写失败", "转写结果序列化失败")
	}
	if err := s.taskBilling().SettleBillingWithAudioDuration(task.BillingOrderID, "", durationMs); err != nil {
		return w.failTimelineTask(task, "转写失败", "转写成功但积分结算失败："+err.Error())
	}
	task.Status = model.TaskStatusSucceeded
	task.Stage = "转写完成"
	task.Progress = 100
	task.ResultJSON = string(payload)
	completedAt := time.Now()
	task.CompletedAt = &completedAt
	if err := s.repo.SaveTaskCompletion(task, model.TaskStatusRunning, nil); err != nil {
		// 冲突/租约已失效时不覆盖他人终态，交由上层判定。
		return fmt.Errorf("写入转写完成态失败: %w", err)
	}
	s.logInfo(task.UserID, task.ID, fmt.Sprintf("时间线转写完成，段落 %d", len(segments)), "")
	return nil
}

func (w *taskWorkerCoordinator) transcribeViaProtocol(ctx context.Context, task *model.Task, input *timelineTranscriptionInput, wavPath string) ([]timelineTranscriptionSegment, string, error) {
	s := w.service
	cfg, err := s.resolveProviderConfig(providerConfig{
		ChannelID:       strings.TrimSpace(input.Config.ChannelID),
		ChannelModelKey: strings.TrimSpace(input.Config.ChannelModelKey),
		Model:           strings.TrimSpace(input.Config.Model),
		PriceTierID:     strings.TrimSpace(input.Config.PriceTierID),
	})
	if err != nil {
		return nil, "", err
	}
	adapter, ok := s.protocolRegistry().Resolve(cfg.InterfaceType)
	if !ok {
		return nil, "", fmt.Errorf("转写协议 %s 未安装", cfg.InterfaceType)
	}
	metadata := adapter.Metadata()
	if !protocolHasCapability(metadata.Categories, protocol.CapabilityTranscription) {
		return nil, "", fmt.Errorf("转写协议 %s 未声明 transcription 能力", cfg.InterfaceType)
	}
	// 火山引擎等上游只接受公网音频地址；其余本地协议继续使用内嵌数据，避免额外存储。
	audio, cleanup, err := w.transcriptionAudioReference(task, wavPath, metadata.RequiresPublicMediaURLs)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, "", err
	}
	request := protocol.GenerationRequest{
		Capability: protocol.CapabilityTranscription,
		Model:      cfg.Model,
		Audios:     []protocol.MediaReference{audio},
		Extra: map[string]any{
			"language":       strings.TrimSpace(input.Language),
			"idempotencyKey": task.ID,
			"audioFormat":    "wav",
		},
	}
	spec, err := adapter.BuildCreate(ctx, protocol.RequestContext{BaseURL: cfg.BaseURL, Request: request})
	if err != nil {
		return nil, "", err
	}
	requestCtx := context.WithValue(ctx, providerAnalyticsKey{}, providerAnalyticsContext{
		Service: s, Billing: s.taskBilling(), UserID: task.UserID, TaskID: task.ID, BillingOrderID: task.BillingOrderID,
		ChannelID: cfg.ChannelID, Capability: "transcription", Operation: "transcription", Model: task.Model, RequestKind: "create",
	})
	body, headers, err := executeProtocolRequestWithHeaders(requestCtx, cfg, spec)
	if err != nil {
		return nil, "", err
	}
	created, err := parseProtocolCreateWithHeaders(requestCtx, adapter, request, headers, body)
	if err != nil {
		return nil, "", err
	}
	if created.Status == protocol.StatusFailed || created.Status == protocol.StatusCancelled {
		return nil, "", fmt.Errorf("%s", defaultString(created.Message, "转写失败"))
	}
	transcription := (*protocol.TranscriptionResult)(nil)
	if created.Result != nil {
		transcription = created.Result.Transcription
	}
	if transcription == nil || len(transcription.Segments) == 0 {
		// 异步协议（火山引擎）提交只返回任务 ID，结果需要按同一 ID 回查。
		if strings.TrimSpace(metadata.Poll) == "" || strings.TrimSpace(created.TaskID) == "" {
			return nil, "", fmt.Errorf("转写服务未识别出语音内容")
		}
		transcription, err = w.pollTranscription(requestCtx, task, cfg, adapter, request, created.TaskID)
		if err != nil {
			return nil, "", err
		}
	}
	segments := make([]timelineTranscriptionSegment, 0, len(transcription.Segments))
	for _, segment := range transcription.Segments {
		if strings.TrimSpace(segment.Text) == "" || segment.EndMs <= segment.StartMs {
			continue
		}
		segments = append(segments, timelineTranscriptionSegment{StartMs: segment.StartMs, EndMs: segment.EndMs, Text: strings.TrimSpace(segment.Text)})
	}
	if len(segments) == 0 {
		return nil, "", fmt.Errorf("转写服务未识别出可用字幕")
	}
	return segments, transcription.Language, nil
}

const (
	timelineTranscriptionPollInterval = 3 * time.Second
	timelineTranscriptionPollTimeout  = 15 * time.Minute
)

// pollTranscription 回查异步转写任务。火山引擎把业务状态放在响应头，
// 因此轮询同样使用 header-aware 解析。
func (w *taskWorkerCoordinator) pollTranscription(ctx context.Context, task *model.Task, cfg providerConfig, adapter protocol.Adapter, request protocol.GenerationRequest, taskID string) (*protocol.TranscriptionResult, error) {
	deadline := time.Now().Add(timelineTranscriptionPollTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	_ = w.progress(task, "等待转写结果…", 55)
	for attempt := 0; ; attempt++ {
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("转写服务处理超时，请稍后重试")
		}
		interval := timelineTranscriptionPollInterval
		if attempt >= 10 {
			interval *= 2
		}
		if err := sleepContext(ctx, interval); err != nil {
			return nil, err
		}
		pollContext := protocol.PollContext{BaseURL: cfg.BaseURL, Model: cfg.Model, Request: request, TaskID: taskID}
		spec, err := adapter.BuildPoll(ctx, pollContext)
		if err != nil {
			return nil, err
		}
		pollCtx := withProviderRequestKind(ctx, "poll")
		body, headers, err := executeProtocolRequestWithHeaders(pollCtx, cfg, spec)
		if err != nil {
			return nil, err
		}
		state, err := parseProtocolPollWithHeaders(pollCtx, adapter, pollContext, headers, body)
		if err != nil {
			return nil, err
		}
		switch state.Status {
		case protocol.StatusSucceeded:
			if state.Result == nil || state.Result.Transcription == nil || len(state.Result.Transcription.Segments) == 0 {
				return nil, fmt.Errorf("转写服务未识别出语音内容")
			}
			return state.Result.Transcription, nil
		case protocol.StatusFailed, protocol.StatusCancelled:
			return nil, fmt.Errorf("%s", defaultString(state.Message, "转写失败"))
		}
		if attempt == 5 {
			_ = w.progress(task, "正在转写…", 65)
		}
	}
}

// transcriptionAudioReference 生成协议需要的音频输入：本地协议内嵌 WAV，
// 需要公网地址的协议（火山引擎）先落为临时资源再签发厂商访问地址。
func (w *taskWorkerCoordinator) transcriptionAudioReference(task *model.Task, wavPath string, requireURL bool) (protocol.MediaReference, func(), error) {
	reference := protocol.MediaReference{MIMEType: "audio/wav", Name: "audio.wav", Kind: "audio"}
	if !requireURL {
		audio, err := os.ReadFile(wavPath)
		if err != nil {
			return reference, nil, fmt.Errorf("读取待转写音频失败: %w", err)
		}
		reference.DataURL = "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(audio)
		return reference, nil, nil
	}
	s := w.service
	file, err := os.Open(wavPath)
	if err != nil {
		return reference, nil, fmt.Errorf("读取待转写音频失败: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return reference, nil, fmt.Errorf("读取待转写音频失败: %w", err)
	}
	uploadKey := "transcription:" + task.ID
	resource, _, err := s.storeResource(task.UserID, "audio", "transcription.wav", "audio/wav", info.Size(), 0, 0, 0, file, &uploadKey, false)
	if err != nil {
		return reference, nil, fmt.Errorf("准备转写音频地址失败: %w", err)
	}
	url, err := s.providerResourceURL(resource, time.Time{})
	if err != nil {
		s.deleteTransientResource(task.UserID, resource)
		return reference, nil, fmt.Errorf("准备转写音频地址失败: %w", err)
	}
	reference.URL = url
	return reference, func() { s.deleteTransientResource(task.UserID, resource) }, nil
}

// deleteTransientResource 清理转写任务自己创建的临时音频；对象清理失败时保留记录，
// 避免留下指向已删除对象的资源行。
func (s *Service) deleteTransientResource(userID string, resource *model.Resource) {
	if resource == nil {
		return
	}
	if err := s.deleteStoredResourceObject(userID, resource); err != nil {
		return
	}
	_ = s.repo.DeleteResource(userID, resource.ID)
}

func parseProtocolCreateWithHeaders(ctx context.Context, adapter protocol.Adapter, request protocol.GenerationRequest, headers http.Header, body []byte) (protocol.CreateResult, error) {
	if parser, ok := adapter.(protocol.HeaderAwareCreateParser); ok {
		return parser.ParseCreateWithRequestAndHeaders(ctx, request, headers, body)
	}
	if parser, ok := adapter.(protocol.RequestAwareCreateParser); ok {
		return parser.ParseCreateWithRequest(ctx, request, body)
	}
	return adapter.ParseCreate(ctx, body)
}

func parseProtocolPollWithHeaders(ctx context.Context, adapter protocol.Adapter, pollContext protocol.PollContext, headers http.Header, body []byte) (protocol.PollResult, error) {
	if parser, ok := adapter.(protocol.HeaderAwarePollParser); ok {
		return parser.ParsePollWithHeaders(ctx, pollContext, headers, body)
	}
	return adapter.ParsePoll(ctx, pollContext, body)
}

func protocolHasCapability(capabilities []protocol.Capability, target protocol.Capability) bool {
	for _, capability := range capabilities {
		if capability == target {
			return true
		}
	}
	return false
}

func probeTranscriptionDurationMs(ctx context.Context, path string) int64 {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return 0
	}
	output, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=nw=1:nk=1", path).Output()
	if err != nil {
		return 0
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return int64(seconds * 1000)
}

func (w *taskWorkerCoordinator) failTimelineTask(task *model.Task, stage string, message string) error {
	s := w.service
	done, err := s.repo.UpdateTaskTerminalState(task.ID, task.LeaseOwner, model.TaskStatusRunning, model.TaskStatusFailed, stage, message, time.Now())
	if err != nil {
		return fmt.Errorf("写入时间线任务失败态失败: %w", err)
	}
	if !done {
		return fmt.Errorf("时间线任务状态或租约已变化：%w", repository.ErrTaskStateConflict)
	}
	s.logInfo(task.UserID, task.ID, fmt.Sprintf("%s: %s", stage, message), "")
	return nil
}

func (w *taskWorkerCoordinator) progress(task *model.Task, stage string, progress int) error {
	if err := w.service.repo.UpdateTaskProgressForLease(task.ID, task.LeaseOwner, stage, progress); err != nil {
		return fmt.Errorf("更新转写进度失败: %w", err)
	}
	return nil
}

func (s *Service) logInfo(userID string, taskID string, message string, extra string) {
	s.log(userID, taskID, "info", message, extra)
}

func isTranscribableMime(mime string) bool {
	return strings.HasPrefix(mime, "video/") || strings.HasPrefix(mime, "audio/")
}

// prepareWhisperWav 将媒体流经本地 ffmpeg 转为 whisper.cpp 期望的
// 16k 单声道 PCM wav；返回临时文件路径与清理函数。
func prepareWhisperWav(ctx context.Context, reader io.Reader, mime string) (string, func(), error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "", nil, fmt.Errorf("音频预处理依赖未安装（需要 ffmpeg）")
	}
	tmpDir, err := os.MkdirTemp("", "yingce-whisper-*")
	if err != nil {
		return "", nil, fmt.Errorf("创建临时目录失败: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }
	inPath := filepath.Join(tmpDir, "input"+extForMime(mime))
	inFile, err := os.Create(inPath)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("写入待转写媒体失败: %w", err)
	}
	if _, err := io.Copy(inFile, reader); err != nil {
		inFile.Close()
		cleanup()
		return "", nil, fmt.Errorf("读取待转写媒体失败: %w", err)
	}
	if err := inFile.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("关闭临时文件失败: %w", err)
	}
	wavPath := filepath.Join(tmpDir, "audio16k.wav")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-y", "-i", inPath, "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", wavPath)
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		cleanup()
		detail := strings.TrimSpace(string(output))
		if len(detail) > 400 {
			detail = detail[len(detail)-400:]
		}
		return "", nil, fmt.Errorf("音频预处理失败（ffmpeg）: %s", detail)
	}
	return wavPath, cleanup, nil
}

func extForMime(mime string) string {
	switch {
	case strings.HasPrefix(mime, "video/mp4"), strings.HasPrefix(mime, "audio/mp4"):
		return ".mp4"
	case strings.HasPrefix(mime, "video/webm"), strings.HasPrefix(mime, "audio/webm"):
		return ".webm"
	case strings.HasPrefix(mime, "video/quicktime"):
		return ".mov"
	case strings.HasPrefix(mime, "audio/mpeg"), strings.HasPrefix(mime, "audio/mp3"):
		return ".mp3"
	case strings.HasPrefix(mime, "audio/wav"), strings.HasPrefix(mime, "audio/x-wav"), strings.HasPrefix(mime, "audio/wave"):
		return ".wav"
	case strings.HasPrefix(mime, "audio/flac"):
		return ".flac"
	case strings.HasPrefix(mime, "audio/aac"):
		return ".aac"
	case strings.HasPrefix(mime, "audio/ogg"), strings.HasPrefix(mime, "video/ogg"):
		return ".ogg"
	default:
		return ".bin"
	}
}

// TimelineTranscriptionCreateRequest 是画布提交字幕转写任务的入参。
type TimelineTranscriptionCreateRequest struct {
	ResourceID string `json:"resourceId"`
	Language   string `json:"language"`
	ProjectID  string `json:"projectId"`
	// ChannelID/Model 来自用户设置中的默认转写服务；为空时回退到第一个可用转写模型。
	ChannelID string `json:"channelId,omitempty"`
	Model     string `json:"model,omitempty"`
}

// CreateTimelineTranscriptionTask 创建时间线字幕转写任务：按管理员配置的
// 转写渠道模型执行，按秒计费；创建时校验功能门控、资源归属与可转写类型，
// 排队计入 active 限额。
func (s *Service) CreateTimelineTranscriptionTask(userID string, req TimelineTranscriptionCreateRequest) (*model.Task, error) {
	if s.IsDraining() {
		return nil, &AppError{Status: 503, Code: 503, Message: "服务正在维护，暂不接受新的生成任务", Retryable: true}
	}
	if err := s.RequireFeature(FeatureTimelineTranscription); err != nil {
		return nil, err
	}
	resourceID := strings.TrimSpace(req.ResourceID)
	if resourceID == "" {
		return nil, BadAuthRequest("必须指定待转写媒体")
	}
	resource, err := s.Resource(userID, resourceID)
	if err != nil || resource == nil {
		return nil, BadAuthRequest("无法读取待转写媒体，可能已被删除")
	}
	if !isTranscribableMime(resource.MimeType) {
		return nil, BadAuthRequest("仅支持音视频文件转写")
	}
	channel, channelModel, err := s.transcriptionChannelModel(req.ChannelID, req.Model)
	if err != nil {
		return nil, BadAuthRequest(err.Error())
	}
	policy, err := s.RuntimePolicy()
	if err != nil {
		return nil, err
	}
	seconds := int64(1)
	if resource.DurationMs > 0 {
		seconds = (resource.DurationMs + 999) / 1000
	}
	input := map[string]any{
		"resourceId": resourceID,
		"language":   strings.TrimSpace(req.Language),
		"mode":       "transcription",
		"config": map[string]any{
			"channelId":            channel.ID,
			"channelModelKey":      channelModel.ModelKey,
			"model":                channelModel.ModelKey,
			"transcriptionSeconds": seconds,
		},
		"transcriptionSeconds": seconds,
	}
	task := model.Task{
		ID: newID(), UserID: userID, ProjectID: req.ProjectID, ChannelModelID: channelModel.ID,
		Type: model.TaskTypeTimelineTranscription, Status: model.TaskStatusQueued,
		Stage: "等待队列调度", Progress: 5, Prompt: "字幕转写", Operation: "transcription",
		Provider: channel.Name, Model: channelModel.ModelKey,
	}
	billingOrder, err := s.taskBillingOrder(userID, &task, input)
	if err != nil {
		return nil, err
	}
	inputJSON, _ := json.Marshal(input)
	task.InputJSON = string(inputJSON)
	if billingOrder != nil {
		task.BillingOrderID = billingOrder.ID
	}
	if err := s.createTaskWithinStorageQuota(&task, billingOrder, policy); err != nil {
		if errors.Is(err, repository.ErrActiveTaskLimit) {
			return nil, BadAuthRequest(fmt.Sprintf("同时排队或运行的任务最多 %d 个，请等待已有任务完成", policy.Task.ActiveTaskLimit))
		}
		return nil, err
	}
	s.recordActivity(userID, "task", 1)
	_ = s.log(userID, task.ID, "info", "字幕转写任务已进入队列", "")
	return taskForOutput(task), nil
}

// transcriptionChannelModel 优先使用用户在设置中选择的默认转写服务；
// 未指定时回退到第一个已启用且协议可用的系统转写模型。
func (s *Service) transcriptionChannelModel(channelID string, modelKey string) (*model.ModelChannel, *model.ChannelModel, error) {
	channelID = strings.TrimSpace(channelID)
	modelKey = strings.TrimSpace(modelKey)
	if channelID != "" || modelKey != "" {
		if channelID == "" || modelKey == "" {
			return nil, nil, errors.New("默认转写服务无效，请重新选择")
		}
		channel, err := s.SystemChannel(channelID)
		if err != nil {
			return nil, nil, errors.New("默认转写服务所在渠道不存在或已停用")
		}
		item, err := s.repo.ChannelModelByKey(channel.ID, modelKey)
		if err != nil || item == nil || !item.Enabled || normalizeCapability(item.Capability) != "transcription" {
			return nil, nil, errors.New("默认转写服务不可用，请重新选择")
		}
		if !s.transcriptionProtocolAvailable(item.Protocol) {
			return nil, nil, errors.New("默认转写服务的协议未安装或不可用")
		}
		return channel, item, nil
	}
	channels, err := s.repo.SystemChannels(false)
	if err != nil {
		return nil, nil, err
	}
	for index := range channels {
		channel := &channels[index]
		models, listErr := s.repo.ChannelModels(channel.ID, false)
		if listErr != nil {
			return nil, nil, listErr
		}
		for modelIndex := range models {
			item := &models[modelIndex]
			if !item.Enabled || normalizeCapability(item.Capability) != "transcription" {
				continue
			}
			if !s.transcriptionProtocolAvailable(item.Protocol) {
				continue
			}
			return channel, item, nil
		}
	}
	return nil, nil, errors.New("未配置转写模型，请在管理后台为系统渠道配置转写协议和模型")
}

func (s *Service) transcriptionProtocolAvailable(protocolID model.ChannelInterfaceType) bool {
	adapter, ok := s.protocolRegistry().Resolve(string(protocolID))
	return ok && protocolHasCapability(adapter.Metadata().Categories, protocol.CapabilityTranscription)
}

// TimelineRenderCreateRequest 是画布提交时间线渲染任务的入参；
// Timeline 为前端 TimelineProject 快照（v2：tracks/clips 平铺）。
// 片段通过 directMedia.storageKey=resource:<id> 引用后端资源。
type TimelineRenderCreateRequest struct {
	prepareOnly bool
	ProjectID   string        `json:"projectId"`
	Timeline    renderProject `json:"timeline"`
}

type timelineRenderInput struct {
	ProjectID string        `json:"projectId"`
	Timeline  renderProject `json:"timeline"`
}

type timelineRenderResult struct {
	ResourceID  string `json:"resourceId"`
	FileName    string `json:"fileName"`
	Size        int64  `json:"size"`
	DurationMs  int64  `json:"durationMs"`
	SubtitleSRT string `json:"subtitleSrt,omitempty"`
}

// CreateTimelineRenderTask 创建时间线渲染任务：本地 ffmpeg 合成，不经模型
// 路由与计费；创建时校验快照至少包含一个可渲染媒体片段。
func (s *Service) CreateTimelineRenderTask(userID string, req TimelineRenderCreateRequest) (*model.Task, error) {
	if s.IsDraining() {
		return nil, &AppError{Status: 503, Code: 503, Message: "服务正在维护，暂不接受新的生成任务", Retryable: true}
	}
	plan := buildRenderPlan(req.Timeline)
	if !plan.HasMedia {
		return nil, BadAuthRequest("时间线没有可渲染的媒体片段")
	}
	policy, err := s.RuntimePolicy()
	if err != nil {
		return nil, err
	}
	input := timelineRenderInput{ProjectID: strings.TrimSpace(req.ProjectID), Timeline: req.Timeline}
	inputJSON, _ := json.Marshal(input)
	task := model.Task{
		ID: newID(), UserID: userID, ProjectID: strings.TrimSpace(req.ProjectID),
		Type: model.TaskTypeTimelineRender, Status: model.TaskStatusQueued,
		Stage: "等待队列调度", Progress: 5, Prompt: "时间线渲染",
		Provider: "local", Model: "ffmpeg", InputJSON: string(inputJSON),
	}
	if req.prepareOnly {
		return &task, nil
	}
	if err := s.createTaskWithinStorageQuota(&task, nil, policy); err != nil {
		if errors.Is(err, repository.ErrActiveTaskLimit) {
			return nil, BadAuthRequest(fmt.Sprintf("同时排队或运行的任务最多 %d 个，请等待已有任务完成", policy.Task.ActiveTaskLimit))
		}
		return nil, err
	}
	s.recordActivity(userID, "task", 1)
	_ = s.log(userID, task.ID, "info", "时间线渲染任务已进入队列", "")
	return taskForOutput(task), nil
}
