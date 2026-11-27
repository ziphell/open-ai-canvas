import { registerPlugin } from "../plugin-registry";
import { PLUGIN_API_VERSION, type PluginManifest, type RegisteredPlugin } from "../plugin-types";

export const RUNNINGHUB_PLUGIN_ID = "runninghub-workflow-provider";
export const COMFYUI_PLUGIN_ID = "comfyui-workflow-provider";

export type WorkflowProvider = "runninghub" | "comfyui";

export function workflowPluginId(provider: WorkflowProvider) {
    return provider === "comfyui" ? COMFYUI_PLUGIN_ID : RUNNINGHUB_PLUGIN_ID;
}

export function workflowProviderPluginEnabled(statuses: Record<string, string>, provider: WorkflowProvider) {
    return statuses[workflowPluginId(provider)] === "enabled";
}

function workflowContributions(providerId: string, label: string) {
    return (["image", "video", "audio"] as const).map((capability) => ({
        id: `${providerId}-${capability}`,
        label: `${label} · ${{ image: "图片", video: "视频", audio: "音频" }[capability]}`,
        providerId,
        capability,
        parameters: [],
    }));
}

const runningHubManifest: PluginManifest = {
    id: RUNNINGHUB_PLUGIN_ID,
    name: "RunningHub 工作流",
    version: "1.1.0",
    publishedAt: "2026-08-25",
    updatedAt: "2026-11-27",
    apiVersion: PLUGIN_API_VERSION,
    description: "在画布中执行 RunningHub Workflow 与 AI App，按工作流字段生成图片、视频和音频。",
    documentation: "# RunningHub 工作流\n\n该插件把 RunningHub 的 Workflow / App 接入画布工作流节点。API Key、工作流参数和字段映射仍由宿主安全保存并提交，插件本身不接触密钥。\n\n在插件设置中可以拉取 RunningHub 工作流，编辑字段映射并测试请求。",
    author: "内置工作流",
    surfaces: ["node", "settings"],
    permissions: ["generation.run", "external.open"],
    trusted: true,
    runtime: { backend: "trusted-backend", web: "trusted-backend" },
    contributes: { workflows: workflowContributions(RUNNINGHUB_PLUGIN_ID, "RunningHub") },
};

const comfyUIManifest: PluginManifest = {
    id: COMFYUI_PLUGIN_ID,
    name: "ComfyUI 工作流",
    version: "1.0.0",
    publishedAt: "2026-11-27",
    updatedAt: "2026-11-27",
    apiVersion: PLUGIN_API_VERSION,
    description: "在后端直连自建 ComfyUI，执行导入的 API 格式工作流，按工作流字段生成图片、视频和音频。",
    documentation: "# ComfyUI 工作流\n\n该插件让后端直连自建 ComfyUI。用户在设置页填入 ComfyUI 根地址，导入 ComfyUI「Save (API Format)」导出的 JSON，编辑字段映射并测试请求。\n\nComfyUI 默认无鉴权，仅在反向代理要求 Bearer 时填写 API Key。",
    author: "内置工作流",
    surfaces: ["node", "settings"],
    permissions: ["generation.run", "external.open"],
    trusted: true,
    runtime: { backend: "trusted-backend", web: "trusted-backend" },
    contributes: { workflows: workflowContributions(COMFYUI_PLUGIN_ID, "ComfyUI") },
};

registerPlugin({ manifest: runningHubManifest } satisfies RegisteredPlugin);
registerPlugin({ manifest: comfyUIManifest } satisfies RegisteredPlugin);
