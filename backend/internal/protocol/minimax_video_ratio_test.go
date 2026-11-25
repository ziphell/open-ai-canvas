package protocol

import (
	"context"
	"strings"
	"testing"
)

func TestMiniMaxVideoRatioFollowsDocumentedModes(t *testing.T) {
	adapter := officialPackageAdapter(t, "minimax-video.yingce-plugin", "minimax-video")
	firstFrame := MediaReference{Kind: "image", Role: "first_frame", URL: "https://cdn.example/first.png"}
	reference := MediaReference{Kind: "image", Role: "reference_image", URL: "https://cdn.example/cast.png", Order: 1}
	for _, tc := range []struct {
		name   string
		model  string
		images []MediaReference
		ratio  string
		want   string
	}{
		{"first frame forces adaptive", "MiniMax-H3", []MediaReference{firstFrame}, "9:16", "adaptive"},
		{"references keep requested ratio", "MiniMax-H3", []MediaReference{reference}, "9:16", "9:16"},
		{"references default to adaptive", "MiniMax-H3", []MediaReference{reference}, "", "adaptive"},
		{"text only keeps requested ratio", "MiniMax-H3", nil, "9:16", "9:16"},
		{"lite references require explicit ratio", "MiniMax-H3-Lite", []MediaReference{reference}, "9:16", "9:16"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{Model: tc.model, Prompt: "test", Images: tc.images, AspectRatio: tc.ratio, Duration: 15, Resolution: "768P"}})
			if err != nil {
				t.Fatal(err)
			}
			if got := manifestTestBody(t, spec)["ratio"]; got != tc.want {
				t.Fatalf("ratio = %v, want %s", got, tc.want)
			}
		})
	}
}

func TestMiniMaxVideoRejectsUndocumentedCombinations(t *testing.T) {
	adapter := officialPackageAdapter(t, "minimax-video.yingce-plugin", "minimax-video")
	firstFrame := MediaReference{Kind: "image", Role: "first_frame", URL: "https://cdn.example/first.png"}
	reference := MediaReference{Kind: "image", Role: "reference_image", URL: "https://cdn.example/cast.png", Order: 1}
	for _, tc := range []struct {
		name    string
		request GenerationRequest
		message string
	}{
		{
			"first frame with reference images",
			GenerationRequest{Model: "MiniMax-H3", Prompt: "test", Images: []MediaReference{firstFrame, reference}, AspectRatio: "9:16", Duration: 5, Resolution: "768P"},
			"不能混用",
		},
		{
			"text to video without concrete ratio",
			GenerationRequest{Model: "MiniMax-H3", Prompt: "test", AspectRatio: "adaptive", Duration: 5, Resolution: "768P"},
			"必须指定具体画幅比例",
		},
		{
			"lite reference mode without ratio",
			GenerationRequest{Model: "MiniMax-H3-Lite", Prompt: "test", Images: []MediaReference{reference}, Duration: 5, Resolution: "768P"},
			"MiniMax-H3-Lite 多模态参考生视频暂不支持 adaptive",
		},
		{
			"max does not support 2k",
			GenerationRequest{Model: "MiniMax-H3-Max", Prompt: "test", AspectRatio: "16:9", Duration: 5, Resolution: "2K"},
			"Highspeed、Lite、Max 仅支持 480P、768P",
		},
		{
			"h3 does not support 480p",
			GenerationRequest{Model: "MiniMax-H3", Prompt: "test", AspectRatio: "16:9", Duration: 5, Resolution: "480P"},
			"MiniMax-H3 分辨率仅支持 768P、2K",
		},
		{
			"unknown model",
			GenerationRequest{Model: "image-01", Prompt: "test", AspectRatio: "16:9", Duration: 5, Resolution: "768P"},
			"MiniMax 视频仅支持",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := adapter.BuildCreate(context.Background(), RequestContext{Request: tc.request})
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("error = %v, want %q", err, tc.message)
			}
		})
	}
}

func TestMiniMaxVideoAudioAndWatermark(t *testing.T) {
	adapter := officialPackageAdapter(t, "minimax-video.yingce-plugin", "minimax-video")
	spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{Model: "MiniMax-H3-Max", Prompt: "test", AspectRatio: "16:9", Duration: 5, Resolution: "768P", GenerateAudio: true}})
	if err != nil {
		t.Fatal(err)
	}
	body := manifestTestBody(t, spec)
	if body["generate_audio"] != true || body["aigc_watermark"] != false {
		t.Fatalf("flags = %#v, want generate_audio=true and aigc_watermark=false", body)
	}
}

func TestMiniMaxVideoPromptExpansionMode(t *testing.T) {
	adapter := officialPackageAdapter(t, "minimax-video.yingce-plugin", "minimax-video")
	build := func(model string, options map[string]any) (map[string]any, error) {
		request := GenerationRequest{Model: model, Prompt: "test", AspectRatio: "16:9", Duration: 5, Resolution: "768P"}
		if options != nil {
			request.ProviderOptions = map[string]map[string]any{"minimax-video": options}
		}
		spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: request})
		if err != nil {
			return nil, err
		}
		return manifestTestBody(t, spec), nil
	}

	body, err := build("MiniMax-H3-Max", map[string]any{"prompt_expansion_mode": " QUALITY "})
	if err != nil {
		t.Fatal(err)
	}
	extra, _ := body["extra"].(map[string]any)
	if extra["prompt_expansion_mode"] != "quality" {
		t.Fatalf("extra = %#v, want prompt_expansion_mode=quality", body["extra"])
	}

	body, err = build("MiniMax-H3-Max", nil)
	if err != nil {
		t.Fatal(err)
	}
	if extra, ok := body["extra"]; ok && extra != nil {
		t.Fatalf("extra = %#v, want omitted when prompt_expansion_mode is not set", extra)
	}

	if _, err := build("MiniMax-H3-Max", map[string]any{"prompt_expansion_mode": "balance"}); err == nil || !strings.Contains(err.Error(), "disabled、balanced 或 quality") {
		t.Fatalf("invalid prompt_expansion_mode error = %v", err)
	}
	if _, err := build("MiniMax-H3", map[string]any{"prompt_expansion_mode": "balanced"}); err == nil || !strings.Contains(err.Error(), "仅 MiniMax-H3-Max 支持") {
		t.Fatalf("non-max prompt_expansion_mode error = %v", err)
	}
}
