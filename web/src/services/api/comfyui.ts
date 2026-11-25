import { http } from "@/services/api/request";
import type { RunningHubCapability, RunningHubWorkflow } from "@/stores/use-config-store";

export type ComfyUIWorkflowImportResult = RunningHubWorkflow & { kind: "workflow" };

/**
 * 自建 ComfyUI 没有按 ID 拉取拓扑的接口，工作流 JSON 由用户一次性导入，
 * 后端只负责解析字段并返回字段映射。
 */
export function importComfyUIWorkflow(payload: { workflowJson: string; workflowId?: string; title?: string; capability: RunningHubCapability }) {
    return http.post<ComfyUIWorkflowImportResult>("/comfyui/workflow-info", payload);
}
