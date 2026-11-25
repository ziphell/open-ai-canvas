import { normalizeRunningHubCapability, type AiConfig, type RunningHubCapability, type RunningHubWorkflowKind, type WorkflowFieldMapping } from "@/stores/use-config-store";
import { workflowProviderPluginEnabled } from "@/lib/plugins/builtin/workflows";
import { usePluginStore } from "@/stores/use-plugin-store";

export type GenerationWorkflowMode = "text" | "image" | "video" | "audio";

export type GenerationWorkflowExecution = {
    provider: "runninghub";
    kind: RunningHubWorkflowKind;
    capability: RunningHubCapability;
    interfaceType: string;
    name: string;
    taskModel: string;
    providerModel: string;
    workflowId: string;
    webappId: string;
    workflowJson: Record<string, unknown>;
    workflowFields: WorkflowFieldMapping[];
};

// 画布样式计划、任务记录和后端协议必须共用同一个工作流选择结果，避免再次回退到普通模型。
export function resolveGenerationWorkflowExecution(config: AiConfig, mode: GenerationWorkflowMode): GenerationWorkflowExecution | null {
    const workflowProvider = mode === "text" ? "model" : config.taskWorkflowProvider || "model";
    if (workflowProvider === "model") return null;

    if (workflowProvider === "runninghub") {
        if (!workflowProviderPluginEnabled(usePluginStore.getState().runtimeStatuses, "runninghub")) throw new Error("工作流插件未启用");
        const runningHub = config.runningHub;
        const connection = runningHub.connection === "comfyui" ? "comfyui" : "runninghub";
        const providerLabel = connection === "comfyui" ? "ComfyUI" : "RunningHub";
        const workflowId = runningHub.workflowId.trim();
        // 自建 ComfyUI 没有 App 概念，也不要求积分 Key；只有用户套了鉴权反代时才填 Key。
        const kind: RunningHubWorkflowKind = connection === "comfyui" ? "workflow" : runningHub.selectedKind === "app" ? "app" : "workflow";
        const apiKeyReady = connection === "comfyui" || Boolean(runningHub.apiKey.trim());
        if (!runningHub.enabled || !runningHub.baseUrl.trim() || !apiKeyReady || !workflowId) {
            throw new Error(connection === "comfyui" ? "ComfyUI 工作流配置不完整，请填写 Base URL 并选择已导入的工作流" : "RunningHub 工作流配置不完整，请填写积分 API Key 并选择已保存条目");
        }
        const workflow = runningHub.workflows.find((item) => item.workflowId.trim() === workflowId && (item.kind === "app" ? "app" : "workflow") === kind);
        if (!workflow) throw new Error(`当前 ${providerLabel} 工作流条目不存在，请重新选择已保存条目`);
        if (connection === "comfyui" && !workflow.workflowJson) throw new Error("当前 ComfyUI 条目缺少 API 格式工作流 JSON，请重新导入");
        const capability = normalizeRunningHubCapability(workflow.capability, normalizeRunningHubCapability(runningHub.capability));
        assertWorkflowCapability(providerLabel, capability, mode);
        const webappId = kind === "app" ? (workflow.webappId || workflow.workflowId).trim() : "";
        const name = workflow.title?.trim() || webappId || workflowId;
        return {
            provider: "runninghub",
            kind,
            capability,
            interfaceType: `${connection === "comfyui" ? "comfyui" : "runninghub"}-workflow-${capability}`,
            name,
            taskModel: workflowTaskModel(`${providerLabel} · `, name),
            providerModel: workflowId,
            workflowId: kind === "workflow" ? workflowId : "",
            webappId,
            workflowJson: workflow.workflowJson || {},
            workflowFields: workflow.fields || [],
        };
    }

    throw new Error("未知工作流提供方");
}

function assertWorkflowCapability(providerName: string, capability: RunningHubCapability, mode: GenerationWorkflowMode) {
    if (capability === mode) return;
    const capabilityName = workflowCapabilityName(capability);
    const modeName = workflowCapabilityName(mode);
    throw new Error(`${providerName} 已保存条目的用途为${capabilityName}，当前节点是${modeName}生成，请切换匹配的工作流`);
}

function workflowCapabilityName(value: GenerationWorkflowMode) {
    if (value === "video") return "视频";
    if (value === "audio") return "音频";
    if (value === "text") return "文本";
    return "图片";
}

function workflowTaskModel(prefix: string, name: string) {
    return `${prefix}${Array.from(name)
        .slice(0, Math.max(0, 120 - prefix.length))
        .join("")}`;
}
