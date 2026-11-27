import { describe, expect, test } from "bun:test";
import { createModelChannel, defaultConfig, encodeChannelModel, normalizeConfigSnapshot } from "../src/stores/use-config-store";

function channel(scope: "system" | "user", id: string) {
    return createModelChannel({
        id,
        name: id,
        scope,
        models: ["whisper-large-v3"],
        modelCosts: [{ model: "whisper-large-v3", capability: "transcription", protocol: "openai-transcription", billingMode: "per_second", unitPriceMicrocredits: 1 }],
    });
}

describe("default transcription model", () => {
    test("keeps a selected system transcription model", () => {
        const system = channel("system", "speech");
        const config = normalizeConfigSnapshot({
            config: { ...defaultConfig, channels: [system], transcriptionModel: encodeChannelModel("speech", "whisper-large-v3") },
        }).config;
        expect(config.transcriptionModels).toEqual([encodeChannelModel("speech", "whisper-large-v3")]);
        expect(config.transcriptionModel).toBe(encodeChannelModel("speech", "whisper-large-v3"));
    });

    test("drops custom-channel transcription models because transcription runs on system channels", () => {
        const custom = channel("user", "custom-speech");
        const config = normalizeConfigSnapshot({
            config: { ...defaultConfig, channels: [custom], transcriptionModel: encodeChannelModel("custom-speech", "whisper-large-v3") },
        }).config;
        expect(config.transcriptionModels).toEqual([]);
        expect(config.transcriptionModel).toBe("");
    });
});
