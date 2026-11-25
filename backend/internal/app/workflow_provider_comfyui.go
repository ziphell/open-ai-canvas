// 自建 ComfyUI 的原生工作流协议：上传素材、提交 /prompt、轮询 /history、下载 /view。
//
// 与 RunningHub 不同，ComfyUI 没有 workflowId 和 nodeInfoList；用户一次性导入
// API 格式工作流 JSON，提交时直接修改 JSON 中对应节点的 inputs 后整体发送。

package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

func comfyUIRootURL(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func (s *Service) runComfyUIWorkflow(ctx context.Context, input canvasGenerationInput) (map[string]interface{}, error) {
	root := comfyUIRootURL(input.Config.BaseURL)
	if root == "" {
		return nil, errors.New("自建 ComfyUI 缺少 Base URL")
	}
	if resumed := resumedProviderRequestID(ctx); resumed != "" {
		return s.pollComfyUIWorkflow(ctx, input.Config, root, resumed)
	}
	workflow, err := cloneComfyUIWorkflow(input.Config.WorkflowJSON)
	if err != nil {
		return nil, err
	}
	fields := input.Config.WorkflowFields
	if len(fields) == 0 {
		fields, err = workflowFieldsFromManagement(workflow, input.Mode)
		if err != nil {
			return nil, err
		}
	}
	fields = workflowFieldsForMode(fields, input.Mode)
	if err := validateWorkflowMediaInputs(fields, input); err != nil {
		return nil, err
	}
	files := map[string]string{}
	for _, media := range append(append(append([]providerMedia{}, input.ReferenceImages...), input.ReferenceVideos...), input.ReferenceAudios...) {
		name, err := s.uploadComfyUIMedia(ctx, root, input.Config, media, false)
		if err != nil {
			return nil, err
		}
		files[media.ID] = name
	}
	if input.Mask != nil {
		name, err := s.uploadComfyUIMedia(ctx, root, input.Config, *input.Mask, true)
		if err != nil {
			return nil, err
		}
		files[input.Mask.ID] = name
	}
	if err := applyComfyUIWorkflowFields(workflow, fields, files, input); err != nil {
		return nil, err
	}
	if !workflowFieldsBindPrompt(fields) {
		for _, override := range runningHubPromptFallback(workflow, input.Prompt) {
			applyComfyUIPromptFallback(workflow, override)
		}
	}
	taskID, err := s.submitComfyUIPrompt(ctx, root, input.Config, workflow)
	if err != nil {
		return nil, err
	}
	if err := s.recordWorkflowProviderRequest(ctx, taskID, "submitted", nil); err != nil {
		metadata, _ := ctx.Value(providerAnalyticsKey{}).(providerAnalyticsContext)
		_ = s.log(metadata.UserID, metadata.TaskID, "error", "ComfyUI 请求状态保存失败", taskID+"："+err.Error())
	}
	return s.pollComfyUIWorkflow(ctx, input.Config, root, taskID)
}

func cloneComfyUIWorkflow(workflow map[string]interface{}) (map[string]interface{}, error) {
	if len(workflow) == 0 {
		return nil, errors.New("自建 ComfyUI 工作流尚未导入，请在设置中导入 API 格式工作流 JSON")
	}
	encoded, err := json.Marshal(workflow)
	if err != nil {
		return nil, fmt.Errorf("ComfyUI 工作流编码失败：%w", err)
	}
	var cloned map[string]interface{}
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil, fmt.Errorf("ComfyUI 工作流解析失败：%w", err)
	}
	return cloned, nil
}

func applyComfyUIWorkflowFields(workflow map[string]interface{}, fields []WorkflowField, files map[string]string, input canvasGenerationInput) error {
	for _, field := range fields {
		if field.Enabled != nil && !*field.Enabled || strings.TrimSpace(field.NodeID) == "" || strings.TrimSpace(field.FieldName) == "" {
			continue
		}
		if field.SafeToOverride != nil && !*field.SafeToOverride {
			continue
		}
		node, ok := workflow[strings.TrimSpace(field.NodeID)].(map[string]interface{})
		if !ok {
			continue
		}
		inputs, ok := node["inputs"].(map[string]interface{})
		if !ok {
			continue
		}
		original, exists := inputs[strings.TrimSpace(field.FieldName)]
		if exists && isWorkflowLinkValue(original) {
			// 已连接的输入属于工作流拓扑，不能被字段映射覆盖成静态值。
			continue
		}
		value, present, err := resolveWorkflowFieldValue(field, files, input)
		if err != nil {
			return err
		}
		if !present {
			if field.Required {
				return fmt.Errorf("工作流字段 %s 缺少值", firstNonEmptyString(field.ID, field.FieldName))
			}
			continue
		}
		inputs[strings.TrimSpace(field.FieldName)] = coerceComfyUIValue(value, original, field)
	}
	return nil
}

// ComfyUI 直接消费 JSON，数值和开关必须保持原生类型；RunningHub 的字符串 fieldValue
// 不能复用，否则 steps/seed 等节点会因类型不符被上游拒绝。
func coerceComfyUIValue(value interface{}, original interface{}, field WorkflowField) interface{} {
	switch original.(type) {
	case bool:
		return comfyUIBoolValue(value)
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return comfyUINumberValue(value)
	}
	if original != nil {
		if text, ok := value.(string); ok {
			return text
		}
		return workflowScalarString(value)
	}
	switch strings.ToUpper(strings.TrimSpace(field.FieldType)) {
	case "BOOLEAN", "BOOL":
		return comfyUIBoolValue(value)
	case "NUMBER", "INTEGER", "INT", "FLOAT", "SLIDER":
		return comfyUINumberValue(value)
	default:
		if text, ok := value.(string); ok {
			return text
		}
		return workflowScalarString(value)
	}
}

func comfyUIBoolValue(value interface{}) bool {
	switch item := value.(type) {
	case bool:
		return item
	case string:
		return strings.EqualFold(strings.TrimSpace(item), "true")
	default:
		return false
	}
}

func comfyUINumberValue(value interface{}) interface{} {
	switch item := value.(type) {
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return item
	}
	text := strings.TrimSpace(workflowScalarString(value))
	if text == "" {
		return value
	}
	if parsed, err := strconv.ParseInt(text, 10, 64); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseFloat(text, 64); err == nil {
		return parsed
	}
	return value
}

func applyComfyUIPromptFallback(workflow map[string]interface{}, override map[string]any) {
	node, ok := workflow[strings.TrimSpace(stringValue(override["nodeId"]))].(map[string]interface{})
	if !ok {
		return
	}
	inputs, ok := node["inputs"].(map[string]interface{})
	if !ok {
		return
	}
	inputs[strings.TrimSpace(stringValue(override["fieldName"]))] = stringValue(override["fieldValue"])
}

func (s *Service) uploadComfyUIMedia(ctx context.Context, root string, config providerConfig, media providerMedia, mask bool) (string, error) {
	raw, mimeType, err := mediaBytes(media)
	if err != nil && isPublicMediaURL(strings.TrimSpace(media.URL)) {
		raw, mimeType, err = getExternalBinary(withProviderRequestKind(ctx, "upload"), strings.TrimSpace(media.URL))
	}
	if err != nil {
		return "", err
	}
	if len(raw) == 0 {
		return "", errors.New("ComfyUI 参考素材为空")
	}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("overwrite", "true")
	_ = writer.WriteField("type", "input")
	filename := providerMediaFilename(media, mimeType)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "image", "filename": filename}))
	header.Set("Content-Type", mimeType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(raw); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	endpoint := root + "/upload/image"
	if mask {
		endpoint = root + "/upload/mask"
	}
	req, err := http.NewRequestWithContext(withProviderRequestKind(ctx, "upload"), http.MethodPost, endpoint, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	applyComfyUIAuth(req, config)
	ApplyOutboundHeaders(req, config.Headers)
	data, _, err := doBinary(req)
	if err != nil {
		return "", fmt.Errorf("ComfyUI 素材上传失败：%w", err)
	}
	var response map[string]any
	if err := json.Unmarshal(data, &response); err != nil {
		return "", errors.New("ComfyUI 上传响应不是有效 JSON")
	}
	name := strings.TrimSpace(stringValue(response["name"]))
	if name == "" {
		return "", errors.New("ComfyUI 上传响应缺少文件名")
	}
	if subfolder := strings.Trim(strings.TrimSpace(stringValue(response["subfolder"])), "/"); subfolder != "" {
		return path.Join(subfolder, name), nil
	}
	return name, nil
}

func (s *Service) submitComfyUIPrompt(ctx context.Context, root string, config providerConfig, workflow map[string]interface{}) (string, error) {
	clientID, err := comfyUIClientID()
	if err != nil {
		return "", err
	}
	payload := map[string]any{"prompt": workflow, "client_id": clientID}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(withProviderRequestKind(ctx, "create"), http.MethodPost, root+"/prompt", bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	applyComfyUIAuth(req, config)
	ApplyOutboundHeaders(req, config.Headers)
	data, _, err := doBinary(req)
	if err != nil {
		return "", fmt.Errorf("ComfyUI 工作流提交失败：%w", err)
	}
	var response map[string]any
	if err := json.Unmarshal(data, &response); err != nil {
		return "", errors.New("ComfyUI 提交响应不是有效 JSON")
	}
	if message := comfyUIErrorMessage(response); message != "" {
		return "", fmt.Errorf("ComfyUI 工作流提交失败：%s", message)
	}
	taskID := firstNonEmptyString(stringValue(response["prompt_id"]), stringValue(response["promptId"]))
	if taskID == "" {
		return "", errors.New("ComfyUI 未返回 prompt_id")
	}
	return taskID, nil
}

func comfyUIErrorMessage(response map[string]any) string {
	if nodeErrors, ok := response["node_errors"].(map[string]any); ok && len(nodeErrors) > 0 {
		nodeIDs := make([]string, 0, len(nodeErrors))
		for nodeID := range nodeErrors {
			nodeIDs = append(nodeIDs, nodeID)
		}
		sort.Strings(nodeIDs)
		if detail := comfyUINodeErrorDetail(nodeErrors[nodeIDs[0]]); detail != "" {
			return fmt.Sprintf("节点 %s：%s", nodeIDs[0], detail)
		}
		return fmt.Sprintf("节点 %s 参数校验失败", nodeIDs[0])
	}
	if errValue, ok := response["error"]; ok && errValue != nil {
		if errMap, ok := errValue.(map[string]any); ok {
			if message := strings.TrimSpace(stringValue(errMap["message"])); message != "" {
				return message
			}
			if details := strings.TrimSpace(stringValue(errMap["details"])); details != "" {
				return details
			}
		}
		if message := strings.TrimSpace(stringValue(errValue)); message != "" {
			return message
		}
	}
	return ""
}

func comfyUINodeErrorDetail(raw any) string {
	nodeError, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	if errorsValue, ok := nodeError["errors"].([]any); ok && len(errorsValue) > 0 {
		if first, ok := errorsValue[0].(map[string]any); ok {
			if detail := firstNonEmptyString(stringValue(first["message"]), stringValue(first["details"])); detail != "" {
				return detail
			}
		}
	}
	return ""
}

func (s *Service) pollComfyUIWorkflow(ctx context.Context, config providerConfig, root string, taskID string) (map[string]interface{}, error) {
	for deadline := providerPollingDeadline(ctx); time.Now().Before(deadline); {
		var response map[string]any
		if err := s.comfyUIJSON(ctx, config, root+"/history/"+url.PathEscape(taskID), &response); err != nil {
			return nil, fmt.Errorf("ComfyUI 查询任务失败：%w", err)
		}
		entry, _ := response[taskID].(map[string]any)
		if entry == nil {
			_ = s.updateWorkflowProviderState(ctx, taskID, "running", ptr(time.Now().Add(2*time.Second)))
			if err := sleepContext(ctx, 2*time.Second); err != nil {
				return nil, err
			}
			continue
		}
		status, _ := entry["status"].(map[string]any)
		statusStr := strings.ToLower(strings.TrimSpace(stringValue(status["status_str"])))
		if statusStr == "error" {
			return nil, fmt.Errorf("ComfyUI 任务失败：%s", comfyUIHistoryFailureMessage(status))
		}
		outputs, _ := entry["outputs"].(map[string]any)
		completed, _ := status["completed"].(bool)
		if completed || statusStr == "success" || len(outputs) > 0 {
			if len(outputs) == 0 {
				return nil, errors.New("ComfyUI 任务完成但没有返回产物")
			}
			urls := comfyUIOutputURLs(root, outputs)
			if len(urls) == 0 {
				return nil, errors.New("ComfyUI 任务完成但没有可下载的产物")
			}
			_ = s.updateWorkflowProviderState(ctx, taskID, "succeeded", nil)
			return s.downloadWorkflowOutputsFor(ctx, "ComfyUI", config, urls)
		}
		_ = s.updateWorkflowProviderState(ctx, taskID, "running", ptr(time.Now().Add(2*time.Second)))
		if err := sleepContext(ctx, 2*time.Second); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("ComfyUI 任务超时（%s）", taskID)
}

func comfyUIHistoryFailureMessage(status map[string]any) string {
	messages, _ := status["messages"].([]any)
	for _, item := range messages {
		entry, ok := item.([]any)
		if !ok || len(entry) < 2 {
			continue
		}
		if strings.TrimSpace(stringValue(entry[0])) != "execution_error" {
			continue
		}
		if detail, ok := entry[1].(map[string]any); ok {
			if message := firstNonEmptyString(stringValue(detail["exception_message"]), stringValue(detail["exception_type"])); message != "" {
				return message
			}
		}
	}
	if message := strings.TrimSpace(stringValue(status["status_str"])); message != "" && !strings.EqualFold(message, "error") {
		return message
	}
	return "上游执行失败"
}

type comfyUIOutputFile struct {
	Filename  string
	Subfolder string
	Type      string
}

func comfyUIOutputFiles(outputs map[string]any) []comfyUIOutputFile {
	all := make([]comfyUIOutputFile, 0)
	appended := false
	for _, rawNode := range outputs {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		for key, rawValue := range node {
			if strings.EqualFold(key, "animated") {
				continue
			}
			items, ok := rawValue.([]any)
			if !ok {
				continue
			}
			for _, rawItem := range items {
				item, ok := rawItem.(map[string]any)
				if !ok {
					continue
				}
				filename := strings.TrimSpace(stringValue(item["filename"]))
				if filename == "" {
					continue
				}
				file := comfyUIOutputFile{Filename: filename, Subfolder: strings.TrimSpace(stringValue(item["subfolder"])), Type: strings.TrimSpace(stringValue(item["type"]))}
				if file.Type == "" {
					file.Type = "output"
				}
				all = append(all, file)
				if file.Type == "output" {
					appended = true
				}
			}
		}
	}
	result := all
	if appended {
		result = make([]comfyUIOutputFile, 0, len(all))
		for _, file := range all {
			if file.Type == "output" {
				result = append(result, file)
			}
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Subfolder != result[j].Subfolder {
			return result[i].Subfolder < result[j].Subfolder
		}
		return result[i].Filename < result[j].Filename
	})
	seen := make(map[string]bool, len(result))
	unique := make([]comfyUIOutputFile, 0, len(result))
	for _, file := range result {
		key := file.Subfolder + "/" + file.Filename
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, file)
	}
	return unique
}

func comfyUIOutputURLs(root string, outputs map[string]any) []string {
	files := comfyUIOutputFiles(outputs)
	urls := make([]string, 0, len(files))
	for _, file := range files {
		query := url.Values{}
		query.Set("filename", file.Filename)
		query.Set("subfolder", file.Subfolder)
		query.Set("type", file.Type)
		urls = append(urls, strings.TrimRight(root, "/")+"/view?"+query.Encode())
	}
	return urls
}

func (s *Service) comfyUIJSON(ctx context.Context, config providerConfig, endpoint string, target *map[string]any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	applyComfyUIAuth(req, config)
	ApplyOutboundHeaders(req, config.Headers)
	data, _, err := doBinary(req)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func getComfyUIAuthorizedBinary(ctx context.Context, config providerConfig, rawURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	applyComfyUIAuth(req, config)
	ApplyOutboundHeaders(req, config.Headers)
	return doBinary(req)
}

func applyComfyUIAuth(req *http.Request, config providerConfig) {
	if token := strings.TrimSpace(config.APIKey); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func comfyUIClientID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("生成 ComfyUI client_id 失败：%w", err)
	}
	return hex.EncodeToString(buffer), nil
}
