import { describe, expect, test } from "bun:test";
import {
    audioCapabilityForConfig,
    audioFormatOptionsForConfig,
    audioModeOptionsForConfig,
    effectiveAudioMode,
    audioVoiceOptionsForConfig,
    doubaoAudioVoiceOptions,
    isCustomVoiceAudioConfig,
    isDoubaoAudioConfig,
    normalizeAudioSpeedValue,
    normalizeAudioVoiceForConfig,
    normalizeAudioVoiceValue,
} from "../src/lib/audio-generation";

const config = (protocol: string) =>
    ({
        provider: "custom",
        model: "models/index-tts",
        audioModel: "models/index-tts",
        apiKey: "",
        baseUrl: "",
        channels: [{ models: ["index-tts"], modelCosts: [], interfaceType: protocol, baseUrl: "", apiKey: "" }],
    }) as any;

describe("audio speed normalization", () => {
    test("uses normal speed for missing or blank values", () => {
        for (const value of ["", "   ", undefined, null, "invalid"]) {
            expect(normalizeAudioSpeedValue(value as any)).toBe("1");
        }
    });

    test("preserves explicit speeds and clamps numeric limits", () => {
        expect(normalizeAudioSpeedValue("0.8")).toBe("0.8");
        expect(normalizeAudioSpeedValue("1")).toBe("1");
        expect(normalizeAudioSpeedValue("1.25")).toBe("1.25");
        expect(normalizeAudioSpeedValue("0")).toBe("0.25");
        expect(normalizeAudioSpeedValue("5")).toBe("4");
    });
});

describe("audio voice protocol handling", () => {
    test("keeps custom voice identifiers for non-OpenAI channels", () => {
        expect(normalizeAudioVoiceValue("voice_001")).toBe("voice_001");
    });

    test("uses provider-specific Doubao defaults instead of OpenAI alloy", () => {
        const doubao = { ...config("doubao-streaming-tts"), model: "seed-audio-1.0", audioModel: "seed-audio-1.0" } as any;
        expect(isDoubaoAudioConfig(doubao)).toBe(true);
        expect(normalizeAudioVoiceForConfig(doubao, "alloy")).toBe("");
    });

    test("lists every official Doubao 1.0 voice in Chinese", () => {
        const values = doubaoAudioVoiceOptions.map((item) => item.value);
        expect(doubaoAudioVoiceOptions).toHaveLength(103);
        expect(new Set(values).size).toBe(values.length);
        expect(values).toContain("zh_female_shuangkuaisisi_moon_bigtts");
        expect(values).toContain("zh_female_yueyunv_mars_bigtts");
        expect(values).toContain("zh_male_qingcang_mars_bigtts");
        expect(values).not.toContain("zh_female_vv_uranus_bigtts");
        expect(values).not.toContain("zh_female_xiaohe_uranus_bigtts");
        expect(doubaoAudioVoiceOptions.every((item) => /[\u4e00-\u9fff]/.test(item.label) && /[\u4e00-\u9fff]/.test(item.scene))).toBe(true);
    });

    test("preserves manually entered official Doubao speaker IDs", () => {
        const doubao = { ...config("doubao-streaming-tts"), model: "seed-audio-1.0", audioModel: "seed-audio-1.0" } as any;
        expect(normalizeAudioVoiceForConfig(doubao, "zh_male_custom_official_speaker")).toBe("zh_male_custom_official_speaker");
    });

    test("seed-audio declares both audio modes and reference capacity", () => {
        const doubao = { ...config("doubao-streaming-tts"), model: "seed-audio-1.0", audioModel: "seed-audio-1.0" } as any;
        expect(audioModeOptionsForConfig(doubao).map((option) => option.value)).toEqual(["tts", "generation"]);
        expect(effectiveAudioMode(doubao)).toBe("generation");
        expect(audioCapabilityForConfig(doubao).references).toEqual({ maxAudios: 3, maxImages: 1 });
        expect(audioFormatOptionsForConfig(doubao).map((option) => option.value)).toEqual(["mp3", "wav", "ogg_opus", "pcm"]);
    });

    test("Doubao TTS 3.0 only supports speech synthesis without references", () => {
        const tts3 = { ...config("doubao-voice-tts-3-0"), model: "doubao-seed-tts-3.0", audioModel: "doubao-seed-tts-3.0" } as any;
        expect(isDoubaoAudioConfig(tts3)).toBe(true);
        expect(audioModeOptionsForConfig(tts3).map((option) => option.value)).toEqual(["tts"]);
        expect(effectiveAudioMode(tts3)).toBe("tts");
        expect(audioCapabilityForConfig(tts3).references).toEqual({ maxAudios: 0, maxImages: 0 });
        expect(audioFormatOptionsForConfig(tts3).map((option) => option.value)).toEqual(["mp3", "pcm", "ogg_opus"]);
    });

    test("Doubao TTS 2.0 sync and async use free voice IDs and their supported formats", () => {
        const sync = { ...config("doubao-voice-tts-2-0"), model: "seed-tts-2.0", audioModel: "seed-tts-2.0" } as any;
        expect(isDoubaoAudioConfig(sync)).toBe(true);
        expect(audioModeOptionsForConfig(sync).map((option) => option.value)).toEqual(["tts"]);
        expect(audioFormatOptionsForConfig(sync).map((option) => option.value)).toEqual(["mp3", "pcm", "ogg_opus", "wav"]);

        const asyncConfig = { ...config("doubao-voice-tts-2-0-async"), model: "seed-tts-2.0", audioModel: "seed-tts-2.0" } as any;
        expect(isDoubaoAudioConfig(asyncConfig)).toBe(true);
        expect(audioFormatOptionsForConfig(asyncConfig).map((option) => option.value)).toEqual(["mp3", "pcm", "ogg_opus"]);
    });

    test("MiniMax TTS uses free-form voice IDs without preset suggestions", () => {
        for (const protocol of ["minimax-tts", "minimax-tts-async"]) {
            const minimax = { ...config(protocol), model: "speech-2.8-hd", audioModel: "speech-2.8-hd" } as any;
            expect(isDoubaoAudioConfig(minimax)).toBe(false);
            expect(isCustomVoiceAudioConfig(minimax)).toBe(true);
            expect(audioVoiceOptionsForConfig(minimax)).toEqual([]);
            expect(normalizeAudioVoiceForConfig(minimax, "male-qn-qingse")).toBe("male-qn-qingse");
            expect(audioFormatOptionsForConfig(minimax).map((option) => option.value)).toEqual(["mp3", "wav", "flac"]);
        }
    });
});
