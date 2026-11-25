import { beforeEach, expect, test } from "bun:test";

import { resolveGenerationWorkflowExecution } from "@/lib/generation-workflow-execution";
import { defaultConfig, normalizeConfigSnapshot, type AiConfig } from "@/stores/use-config-store";
import { usePluginStore } from "@/stores/use-plugin-store";

beforeEach(() => {
    usePluginStore.setState({ runtimeStatuses: { "runninghub-workflow-provider": "enabled" } });
});

function comfyUIConfig(overrides: Partial<AiConfig["runningHub"]> = {}): AiConfig {
    return {
        ...defaultConfig,
        taskWorkflowProvider: "runninghub",
        runningHub: {
            ...defaultConfig.runningHub,
            connection: "comfyui",
            enabled: true,
            baseUrl: "http://127.0.0.1:8188",
            apiKey: "",
            workflowId: "wf-1",
            workflows: [
                {
                    workflowId: "wf-1",
                    title: "flux",
                    kind: "workflow",
                    capability: "video",
                    workflowJson: { "3": { class_type: "KSampler", inputs: {} } },
                },
            ],
            ...overrides,
        },
    };
}

test("resolves a self-hosted ComfyUI workflow without an API key", () => {
    const execution = resolveGenerationWorkflowExecution(comfyUIConfig(), "video");
    expect(execution?.interfaceType).toBe("comfyui-workflow-video");
    expect(execution?.workflowId).toBe("wf-1");
    expect(execution?.taskModel).toContain("ComfyUI");
});

test("still requires an API key for RunningHub cloud workflows", () => {
    const config = comfyUIConfig({ connection: "runninghub", apiKey: "" });
    expect(() => resolveGenerationWorkflowExecution(config, "video")).toThrow();
});

test("normalization keeps ComfyUI mode and drops an incompatible App selection", () => {
    const normalized = normalizeConfigSnapshot({
        config: comfyUIConfig({
            selectedKind: "app",
            workflowId: "app-1",
            workflows: [{ workflowId: "app-1", kind: "app", capability: "video" }],
        }),
    }).config;
    expect(normalized.runningHub.connection).toBe("comfyui");
    expect(normalized.runningHub.selectedKind).toBe("workflow");
    expect(normalized.runningHub.workflowId).toBe("");
});
