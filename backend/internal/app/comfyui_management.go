// 自建 ComfyUI 工作流导入：只解析用户提交的 API 格式 JSON，不访问上游。
//
// RunningHub 通过 workflowId 从云端拉取拓扑；自建 ComfyUI 没有等价接口，
// 因此由设置页粘贴或上传 JSON，宿主只负责字段推断和保存。

package app

import (
	"context"
	"encoding/json"
	"strings"
)

type ComfyUIWorkflowImportRequest struct {
	// WorkflowJSON 同时接受 JSON 对象和字符串化的 JSON。
	WorkflowJSON json.RawMessage `json:"workflowJson"`
	WorkflowID   string          `json:"workflowId,omitempty"`
	Title        string          `json:"title,omitempty"`
	Capability   string          `json:"capability,omitempty"`
}

func (s *Service) ImportComfyUIWorkflow(_ context.Context, req ComfyUIWorkflowImportRequest) (map[string]any, error) {
	workflow, err := parseComfyUIWorkflowJSON(req.WorkflowJSON)
	if err != nil {
		return nil, err
	}
	workflowID := firstNonEmptyString(strings.TrimSpace(req.WorkflowID), "comfyui-workflow")
	return map[string]any{
		"workflowId":   workflowID,
		"kind":         "workflow",
		"title":        firstNonEmptyString(strings.TrimSpace(req.Title), workflowID),
		"fields":       collectManagementWorkflowFields(workflow, req.Capability),
		"workflowJson": workflow,
	}, nil
}

func parseComfyUIWorkflowJSON(raw json.RawMessage) (map[string]any, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, BadAuthRequest("请先粘贴 ComfyUI API 格式工作流 JSON")
	}
	payload := []byte(trimmed)
	if strings.HasPrefix(trimmed, "\"") {
		var text string
		if err := json.Unmarshal(payload, &text); err != nil {
			return nil, BadAuthRequest("ComfyUI 工作流 JSON 字符串格式无效")
		}
		payload = []byte(text)
	}
	var workflow map[string]any
	if err := json.Unmarshal(payload, &workflow); err != nil {
		return nil, BadAuthRequest("ComfyUI 工作流 JSON 格式无效")
	}
	if len(workflow) == 0 {
		return nil, BadAuthRequest("ComfyUI 工作流 JSON 不能为空")
	}
	nodeCount := 0
	for _, rawNode := range workflow {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(stringValue(node["class_type"])) != "" {
			nodeCount++
		}
	}
	if nodeCount == 0 {
		return nil, BadAuthRequest("工作流 JSON 不是 ComfyUI API 格式：请在 ComfyUI 中选择 Save (API Format) 导出")
	}
	return workflow, nil
}
