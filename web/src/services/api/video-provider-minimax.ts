import { getMediaBlob } from "@/services/file-storage";
import { imageToDataUrl } from "@/services/image-storage";
import { boolConfig } from "@/lib/seedance-video";
import { modelOptionName } from "@/stores/use-config-store";
import { isPublicMediaUrl } from "./video-validation";
import { blobToDataUrl } from "./video-response";
import type { ReferenceImage } from "@/types/image";
import type { ReferenceAudio, ReferenceVideo } from "@/types/media";

import type { MiniMaxVideoCreateResponse, MiniMaxVideoTask, RequestOptions, ResolvedAiConfig, VideoGenerationTask, VideoGenerationTaskState } from "./video-contracts";
import type { VideoProviderDeps } from "./video-provider-deps";
import { resolveVideoImageReferences } from "./video-reference-roles";

export async function createMiniMaxVideoTask(
    deps: VideoProviderDeps,
    config: ResolvedAiConfig,
    model: string,
    prompt: string,
    references: ReferenceImage[],
    videoReferences: ReferenceVideo[],
    audioReferences: ReferenceAudio[],
    options?: RequestOptions,
): Promise<VideoGenerationTask> {
    const imageUrls = await Promise.all(references.slice(0, 9).map((image) => resolveMiniMaxImageUrl(image)));
    const videoUrls = await Promise.all(videoReferences.slice(0, 3).map((video) => resolveMiniMaxMediaUrl(video, "参考视频")));
    const audioUrls = await Promise.all(audioReferences.slice(0, 3).map((audio) => resolveMiniMaxMediaUrl(audio, "参考音频")));
    const content: Array<Record<string, unknown>> = [{ type: "text", text: prompt.trim() }];
    const imagePlan = resolveVideoImageReferences(references.slice(0, 9), options, { videoCount: videoUrls.length, audioCount: audioUrls.length, inferFrames: false });
    const frameMode = imagePlan.some(({ role }) => role === "first_frame" || role === "last_frame");
    const hasReferenceMedia = videoUrls.length > 0 || audioUrls.length > 0 || imagePlan.some(({ role }) => role === "reference_image");
    if (frameMode && hasReferenceMedia) throw new Error("MiniMax 图生视频（首帧/尾帧）与多模态参考不能混用，请只保留首尾帧或改用参考生视频");
    imageUrls.forEach((url, index) => {
        content.push({ type: "image_url", image_url: { url }, role: imagePlan[index]?.role || "reference_image" });
    });
    videoUrls.forEach((url) => content.push({ type: "video_url", video_url: { url }, role: "reference_video" }));
    audioUrls.forEach((url) => content.push({ type: "audio_url", audio_url: { url }, role: "reference_audio" }));
    const modelName = modelOptionName(model);
    const hasMedia = content.length > 1;
    const payload = {
        model: modelName,
        content,
        resolution: normalizeMiniMaxResolution(config.vquality, modelName),
        duration: normalizeMiniMaxDuration(config.videoSeconds, modelName),
        ratio: normalizeMiniMaxRatio(config.size, { frameMode, hasMedia, isLite: /minimax-h3-lite$/i.test(modelName.trim()) }),
        aigc_watermark: boolConfig(config.videoWatermark, false),
        generate_audio: boolConfig(config.videoGenerateAudio, true),
    };
    try {
        const created = await deps.transport.post<MiniMaxVideoCreateResponse>(miniMaxVideoUrl(config, "/video_generation"), payload, options);
        const id = created.task_id || created.request_id || created.data?.task_id || created.data?.id || "";
        if (!id) throw new Error("MiniMax 视频接口没有返回任务 ID");
        return { id, provider: "minimax", model };
    } catch (error) {
        throw new Error(deps.response.readAxiosError(error, "MiniMax 视频任务创建失败"));
    }
}

export async function pollMiniMaxVideoTask(deps: VideoProviderDeps, config: ResolvedAiConfig, task: VideoGenerationTask, options?: RequestOptions): Promise<VideoGenerationTaskState> {
    try {
        const response = await deps.transport.get<{ task?: MiniMaxVideoTask }>(miniMaxVideoUrl(config, `/query/video_generation/${encodeURIComponent(task.id)}`), options);
        const state = response.task || {};
        const status = String(state.status || "").toLowerCase();
        if (status === "succeeded" || status === "completed") {
            const url = state.content?.url || "";
            if (!url) return { status: "failed", error: "MiniMax 视频任务已完成但没有返回视频地址" };
            return { status: "completed", result: await deps.response.videoResultFromUrl(url, options) };
        }
        if (status === "failed" || status === "cancelled") {
            const code = state.error?.code ? `${state.error.code}：` : "";
            return { status: "failed", error: `${code}${state.error?.message || "MiniMax 视频生成失败"}` };
        }
        return { status: "pending" };
    } catch (error) {
        throw new Error(deps.response.readAxiosError(error, "MiniMax 视频任务查询失败"));
    }
}

function miniMaxVideoUrl(config: ResolvedAiConfig, path: string) {
    const base = config.baseUrl.replace(/\/+$/, "");
    return /\/v2$/i.test(base) ? `${base}${path}` : `${base}/v2${path}`;
}

function normalizeMiniMaxResolution(value: string, model: string) {
    const normalized = value.trim().toLowerCase();
    if (model.trim().toLowerCase() === "minimax-h3") {
        return ["2k", "4k", "high", "1080", "1080p", "1440p", "2160p"].includes(normalized) ? "2K" : "768P";
    }
    return ["480", "480p"].includes(normalized) ? "480P" : "768P";
}

function normalizeMiniMaxDuration(value: string, model: string) {
    const seconds = Math.floor(Number(value) || 5);
    const min = /minimax-h3-max$/i.test(model.trim()) ? 5 : 4;
    return Math.max(min, Math.min(15, seconds));
}

function normalizeMiniMaxRatio(value: string, mode: { frameMode: boolean; hasMedia: boolean; isLite: boolean }) {
    const concrete = ["21:9", "16:9", "4:3", "1:1", "3:4", "9:16"];
    if (mode.frameMode) return "adaptive";
    if (!mode.hasMedia) return concrete.includes(value) ? value : "16:9";
    if (concrete.includes(value)) return value;
    if (value === "adaptive" && !mode.isLite) return "adaptive";
    return mode.isLite ? "16:9" : "adaptive";
}

export async function resolveMiniMaxImageUrl(image: ReferenceImage) {
    const directUrl = image.url || image.dataUrl;
    if (isPublicMediaUrl(directUrl) || directUrl.startsWith("data:")) return directUrl;
    const dataUrl = await imageToDataUrl(image);
    if (!dataUrl) throw new Error("MiniMax 参考图读取失败，请换一张图片或重新上传");
    return dataUrl;
}

async function resolveMiniMaxMediaUrl(media: ReferenceVideo | ReferenceAudio, label: string) {
    if (isPublicMediaUrl(media.url) || media.url?.startsWith("data:")) return media.url;
    let blob: Blob | null = null;
    if (media.storageKey) blob = await getMediaBlob(media.storageKey);
    if (!blob && media.url?.startsWith("blob:")) blob = await (await fetch(media.url)).blob();
    if (!blob) throw new Error(`MiniMax ${label}必须是公网 URL 或本地已保存素材`);
    return blobToDataUrl(blob);
}
