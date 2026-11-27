package protocol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestDoubaoStreamingTTSPackageBuildsOfficialRequest(t *testing.T) {
	manifest := []byte(`{
		"apiVersion":"yingce.plugin/v2",
		"id":"doubao-streaming-tts","name":"Doubao Streaming TTS","version":"2.0.0","author":"Volcengine",
		"contributes":{"providers":[{"id":"doubao-streaming-tts","label":"Doubao Streaming TTS","capabilities":["audio"],"scopes":["canvas"],"auth":{"type":"header","field":"apiKey","header":"X-Api-Key"},"create":{"method":"POST","path":"/api/v3/tts/unidirectional","contentType":"application/json","body":{"req_params":{"$json":{"text":{"$ref":"request.prompt"},"speaker":{"$ref":"request.extra.audioVoice"},"model":{"$ref":"request.model"}}}},"headers":{"X-Api-Resource-Id":{"$coalesce":[{"$ref":"request.providerOptions.doubao-streaming-tts.resourceId"},"seed-tts-2.0"]},"X-Api-Request-Id":{"$ref":"request.extra.idempotencyKey"}}},"response":{"streamedJsonAudio":true,"resultKind":"audio"}}]}
	}`)
	adapter, err := LoadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{
		Capability: CapabilityAudio,
		Model:      "seed-tts-2.0",
		Prompt:     "你好",
		Extra:      map[string]any{"audioVoice": "speaker-1", "idempotencyKey": "request-1"},
		ProviderOptions: map[string]map[string]any{
			"doubao-streaming-tts": {"resourceId": "seed-icl-2.0"},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Headers["X-Api-Resource-Id"] != "seed-icl-2.0" || spec.Headers["X-Api-Request-Id"] != "request-1" {
		t.Fatalf("headers = %#v", spec.Headers)
	}
	body, ok := spec.Body.(map[string]any)
	if !ok {
		t.Fatalf("body type = %T", spec.Body)
	}
	params, ok := body["req_params"].(string)
	if !ok {
		t.Fatalf("req_params type = %T, want JSON string", body["req_params"])
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(params), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["text"] != "你好" || decoded["speaker"] != "speaker-1" || decoded["model"] != "seed-tts-2.0" {
		t.Fatalf("req_params = %#v", decoded)
	}
}

func TestStreamedJSONAudioCreateResultConcatenatesFrames(t *testing.T) {
	first := base64.StdEncoding.EncodeToString([]byte("first"))
	second := base64.StdEncoding.EncodeToString([]byte("second"))
	body := []byte(`{"code":0,"data":"` + first + `"}{"code":0,"data":"` + second + `","usage":{"characters":6}}`)
	adapter := manifestAdapter{manifest: Manifest{Response: ManifestResponse{StreamedJSONAudio: true, ResultKind: "audio"}}}
	result, err := adapter.ParseCreate(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusSucceeded || result.Result == nil || len(result.Result.Audios) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if !strings.HasPrefix(result.Result.Audios[0].DataURL, "data:audio/mpeg;base64,") {
		t.Fatalf("data URL = %q", result.Result.Audios[0].DataURL)
	}
	encoded := strings.TrimPrefix(result.Result.Audios[0].DataURL, "data:audio/mpeg;base64,")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || string(decoded) != "firstsecond" {
		t.Fatalf("audio bytes = %q, err = %v", decoded, err)
	}
}

func TestStreamedJSONAudioCreateResultRejectsBusinessError(t *testing.T) {
	adapter := manifestAdapter{manifest: Manifest{Response: ManifestResponse{StreamedJSONAudio: true, ResultKind: "audio"}}}
	_, err := adapter.ParseCreate(context.Background(), []byte(`{"code":30000001,"message":"invalid speaker"}`))
	if err == nil || !strings.Contains(err.Error(), "invalid speaker") {
		t.Fatalf("error = %v", err)
	}
}

func TestDoubaoSeedAudioCreateUsesTextPromptWithoutResourceHeader(t *testing.T) {
	manifest, err := os.ReadFile("../../../plugin-packages/doubao-streaming-tts/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := LoadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{Capability: CapabilityAudio, Model: "seed-audio-1.0", Prompt: "你好"}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != "/api/v3/tts/create" {
		t.Fatalf("path = %s", spec.Path)
	}
	if _, ok := spec.Headers["X-Api-Resource-Id"]; ok {
		t.Fatalf("seed-audio create must not send a TTS resource header: %#v", spec.Headers)
	}
	body := spec.Body.(map[string]any)
	if body["model"] != "seed-audio-1.0" || body["text_prompt"] != "你好" || body["audio_mode"] != "tts" || body["references"] != nil {
		t.Fatalf("text-only body = %#v", body)
	}
}

func TestDoubaoSeedAudioCreateSendsReferenceAudioAndImage(t *testing.T) {
	manifest, err := os.ReadFile("../../../plugin-packages/doubao-streaming-tts/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := LoadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	audioSpec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{
		Capability: CapabilityAudio,
		Model:      "seed-audio-1.0",
		Prompt:     "@Audio1 用这个声音说你好",
		Audios:     []MediaReference{{URL: "https://cdn.example.com/voice.mp3", Kind: "audio"}},
		Extra:      map[string]any{"audioVoice": "zh_female_qingxinnvsheng_uranus_bigtts", "audioFormat": "mp3"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	audioBody := audioSpec.Body.(map[string]any)
	references, _ := audioBody["references"].([]any)
	if len(references) != 2 {
		t.Fatalf("audio references = %#v", audioBody["references"])
	}
	reference, _ := references[0].(map[string]any)
	speaker, _ := references[1].(map[string]any)
	if reference["audio_url"] != "https://cdn.example.com/voice.mp3" || reference["audio_data"] != nil || speaker["speaker"] != "zh_female_qingxinnvsheng_uranus_bigtts" {
		t.Fatalf("audio references = %#v", references)
	}

	imageSpec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{
		Capability: CapabilityAudio,
		Model:      "seed-audio-1.0",
		Prompt:     "根据图片内容生成音频",
		Images:     []MediaReference{{DataURL: "data:image/png;base64,aGVsbG8=", Kind: "image"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	imageBody := imageSpec.Body.(map[string]any)
	imageReferences, _ := imageBody["references"].([]any)
	if len(imageReferences) != 1 {
		t.Fatalf("image references = %#v", imageBody["references"])
	}
	imageReference, _ := imageReferences[0].(map[string]any)
	if imageReference["image_data"] != "aGVsbG8=" || imageReference["image_url"] != nil {
		t.Fatalf("image reference = %#v", imageReference)
	}
}

func TestDoubaoVoiceTTS30PackageBuildsStreamingRequest(t *testing.T) {
	manifest, err := os.ReadFile("../../../plugin-packages/doubao-voice-tts-3-0/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := LoadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{
		Capability: CapabilityAudio,
		Model:      "doubao-seed-tts-3.0",
		Prompt:     "你好，欢迎使用语音合成。",
		Extra:      map[string]any{"audioVoice": "multi_female_tuanzi_v3_uranus_bigtts", "audioFormat": "mp3", "audioSpeed": "1.2", "audioInstructions": "温柔自然", "idempotencyKey": "request-1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != "/api/v3/text-to-speech/multi_female_tuanzi_v3_uranus_bigtts/streaming" {
		t.Fatalf("path = %s", spec.Path)
	}
	if spec.Headers["X-Request-Id"] != "request-1" {
		t.Fatalf("headers = %#v", spec.Headers)
	}
	body := spec.Body.(map[string]any)
	if body["text"] != "你好，欢迎使用语音合成。" || body["model_id"] != "doubao-seed-tts-3.0" || body["enable_subtitle"] != false {
		t.Fatalf("body = %#v", body)
	}
	format := body["output_format"].(map[string]any)
	if format["format"] != "mp3" {
		t.Fatalf("output_format = %#v", format)
	}
	settings := body["voice_settings"].(map[string]any)
	if settings["speed"] != 1.2 {
		t.Fatalf("voice_settings = %#v", settings)
	}
	instructions, _ := settings["instructions"].([]any)
	if len(instructions) != 1 || instructions[0] != "温柔自然" {
		t.Fatalf("voice_settings = %#v", settings)
	}
}

func TestDoubaoVoiceTTS30RequiresVoice(t *testing.T) {
	manifest, err := os.ReadFile("../../../plugin-packages/doubao-voice-tts-3-0/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := LoadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{
		Capability: CapabilityAudio,
		Model:      "doubao-seed-tts-3.0",
		Prompt:     "你好",
	}})
	if err == nil || !strings.Contains(err.Error(), "音色 ID") {
		t.Fatalf("error = %v", err)
	}
}

func loadTTSPlugin(t *testing.T, id string) Adapter {
	t.Helper()
	manifest, err := os.ReadFile("../../../plugin-packages/" + id + "/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := LoadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func TestDoubaoVoiceTTS20BuildsUnidirectionalRequest(t *testing.T) {
	adapter := loadTTSPlugin(t, "doubao-voice-tts-2-0")
	spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{
		Capability: CapabilityAudio,
		Model:      "seed-tts-2.0",
		Prompt:     "你好，欢迎使用语音合成。",
		Extra:      map[string]any{"audioVoice": "zh_female_vv_uranus_bigtts", "audioFormat": "mp3", "audioSpeed": "1.2", "idempotencyKey": "request-2"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != "/api/v3/tts/unidirectional" {
		t.Fatalf("path = %s", spec.Path)
	}
	if spec.Headers["X-Api-Resource-Id"] != "seed-tts-2.0" || spec.Headers["X-Api-Request-Id"] != "request-2" {
		t.Fatalf("headers = %#v", spec.Headers)
	}
	params := spec.Body.(map[string]any)["req_params"].(map[string]any)
	if params["text"] != "你好，欢迎使用语音合成。" || params["speaker"] != "zh_female_vv_uranus_bigtts" {
		t.Fatalf("req_params = %#v", params)
	}
	audioParams := params["audio_params"].(map[string]any)
	if audioParams["format"] != "mp3" || audioParams["speech_rate"] != 20 {
		t.Fatalf("audio_params = %#v", audioParams)
	}
}

func TestDoubaoVoiceTTS20AsyncSubmitAndQuery(t *testing.T) {
	adapter := loadTTSPlugin(t, "doubao-voice-tts-2-0-async")
	spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{
		Capability: CapabilityAudio,
		Model:      "seed-tts-2.0",
		Prompt:     "长文本",
		Extra:      map[string]any{"audioVoice": "zh_female_vv_uranus_bigtts", "audioFormat": "mp3", "idempotencyKey": "request-3"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != "/api/v3/tts/submit" || spec.Headers["X-Api-Resource-Id"] != "seed-tts-2.0" {
		t.Fatalf("spec = %#v", spec)
	}
	body := spec.Body.(map[string]any)
	if body["unique_id"] != "request-3" {
		t.Fatalf("body = %#v", body)
	}
	created, err := adapter.ParseCreate(context.Background(), []byte(`{"code":20000000,"message":"ok","data":{"task_id":"task-9","task_status":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != StatusProcessing || created.TaskID != "task-9" {
		t.Fatalf("created = %#v", created)
	}
	pollSpec, err := adapter.BuildPoll(context.Background(), PollContext{TaskID: "task-9", Request: GenerationRequest{Model: "seed-tts-2.0"}})
	if err != nil {
		t.Fatal(err)
	}
	if pollSpec.Path != "/api/v3/tts/query" || pollSpec.Body.(map[string]any)["task_id"] != "task-9" {
		t.Fatalf("poll spec = %#v", pollSpec)
	}
	state, err := adapter.ParsePoll(context.Background(), PollContext{TaskID: "task-9"}, []byte(`{"code":20000000,"message":"ok","data":{"task_id":"task-9","task_status":2,"audio_url":"https://cdn.example.com/tts.mp3"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != StatusSucceeded || state.Result == nil || len(state.Result.Audios) != 1 || state.Result.Audios[0].URL != "https://cdn.example.com/tts.mp3" || !state.Result.Audios[0].Ephemeral {
		t.Fatalf("state = %#v", state)
	}
	failed, err := adapter.ParsePoll(context.Background(), PollContext{TaskID: "task-9"}, []byte(`{"code":20000000,"message":"ok","data":{"task_id":"task-9","task_status":3}}`))
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != StatusFailed || failed.Message == "" {
		t.Fatalf("failed = %#v", failed)
	}
}

func TestMiniMaxTTSBuildsSyncRequest(t *testing.T) {
	adapter := loadTTSPlugin(t, "minimax-tts")
	spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{
		Capability: CapabilityAudio,
		Model:      "speech-2.8-hd",
		Prompt:     "今天是不是很开心呀",
		Extra:      map[string]any{"audioVoice": "male-qn-qingse", "audioFormat": "mp3", "audioSpeed": "1.1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != "/v1/t2a_v2" {
		t.Fatalf("path = %s", spec.Path)
	}
	if spec.Auth.Type != "bearer" {
		t.Fatalf("auth = %#v", spec.Auth)
	}
	body := spec.Body.(map[string]any)
	if body["model"] != "speech-2.8-hd" || body["text"] != "今天是不是很开心呀" {
		t.Fatalf("body = %#v", body)
	}
	voice := body["voice_setting"].(map[string]any)
	if voice["voice_id"] != "male-qn-qingse" || voice["speed"] != 1.1 {
		t.Fatalf("voice_setting = %#v", voice)
	}
	audio := body["audio_setting"].(map[string]any)
	if audio["format"] != "mp3" || audio["sample_rate"] != float64(32000) {
		t.Fatalf("audio_setting = %#v", audio)
	}
	created, err := adapter.ParseCreate(context.Background(), []byte(`{"data":{"audio":"68656c6c6f","status":2},"extra_info":{"audio_length":9900,"usage_characters":26},"base_resp":{"status_code":0,"status_msg":"success"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != StatusSucceeded || created.Result == nil || len(created.Result.Audios) != 1 || created.Result.Audios[0].DataURL != "data:audio/mpeg;base64,aGVsbG8=" {
		t.Fatalf("created = %#v", created)
	}
	failed, err := adapter.ParseCreate(context.Background(), []byte(`{"base_resp":{"status_code":1004,"status_msg":"invalid api key"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != StatusFailed || failed.Message != "invalid api key" {
		t.Fatalf("failed = %#v", failed)
	}
}

func TestMiniMaxTTSAsyncCreatePollAndResult(t *testing.T) {
	adapter := loadTTSPlugin(t, "minimax-tts-async")
	spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{
		Capability: CapabilityAudio,
		Model:      "speech-2.8-hd",
		Prompt:     "长文本",
		Extra:      map[string]any{"audioVoice": "audiobook_male_1", "audioFormat": "mp3"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != "/v1/t2a_async_v2" {
		t.Fatalf("path = %s", spec.Path)
	}
	created, err := adapter.ParseCreate(context.Background(), []byte(`{"task_id":95157322514444,"file_id":95157322514496,"usage_characters":101,"base_resp":{"status_code":0,"status_msg":"success"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != StatusPending || created.TaskID != "95157322514444" || created.ResultID != "95157322514496" {
		t.Fatalf("created = %#v", created)
	}
	pollSpec, err := adapter.BuildPoll(context.Background(), PollContext{TaskID: created.TaskID, Request: GenerationRequest{Model: "speech-2.8-hd"}})
	if err != nil {
		t.Fatal(err)
	}
	if pollSpec.Method != "GET" || pollSpec.Path != "/v1/query/t2a_async_query_v2" || pollSpec.Query["task_id"][0] != created.TaskID {
		t.Fatalf("poll spec = %#v", pollSpec)
	}
	state, err := adapter.ParsePoll(context.Background(), PollContext{TaskID: created.TaskID}, []byte(`{"task_id":95157322514444,"status":"Success","file_id":95157322514496,"base_resp":{"status_code":0,"status_msg":"success"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != StatusSucceeded || state.ResultID != "95157322514496" {
		t.Fatalf("state = %#v", state)
	}
	resultSpec, err := adapter.(ResultAdapter).BuildResult(context.Background(), PollContext{TaskID: created.TaskID, ResultID: state.ResultID, Request: GenerationRequest{Model: "speech-2.8-hd"}})
	if err != nil {
		t.Fatal(err)
	}
	if resultSpec.Path != "/v1/files/retrieve" || resultSpec.Query["file_id"][0] != state.ResultID {
		t.Fatalf("result spec = %#v", resultSpec)
	}
	resultState, err := adapter.ParsePoll(context.Background(), PollContext{TaskID: created.TaskID}, []byte(`{"file":{"file_id":95157322514496,"download_url":"https://cdn.example.com/async.mp3"},"base_resp":{"status_code":0,"status_msg":"success"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if resultState.Result == nil || len(resultState.Result.Audios) != 1 || resultState.Result.Audios[0].URL != "https://cdn.example.com/async.mp3" {
		t.Fatalf("result state = %#v", resultState)
	}
}

func TestStreamedJSONLTTSEventsConcatenateAudio(t *testing.T) {
	first := base64.StdEncoding.EncodeToString([]byte("first"))
	second := base64.StdEncoding.EncodeToString([]byte("second"))
	body := []byte(`{"event":"ready","session_id":"s1"}
{"event":"audio","audio":"` + first + `"}
{"event":"audio","audio":"` + second + `"}
{"event":"final","is_final":true,"reason":"TTS_STREAM_FINAL_REASON_CLOSED"}
`)
	adapter := manifestAdapter{manifest: Manifest{Response: ManifestResponse{StreamedJSONAudio: true, ResultKind: "audio"}}}
	result, err := adapter.ParseCreate(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusSucceeded || result.Result == nil || len(result.Result.Audios) != 1 {
		t.Fatalf("result = %#v", result)
	}
	encoded := strings.TrimPrefix(result.Result.Audios[0].DataURL, "data:audio/mpeg;base64,")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || string(decoded) != "firstsecond" {
		t.Fatalf("audio bytes = %q, err = %v", decoded, err)
	}
}

func TestStreamedJSONLTTSErrorEventFails(t *testing.T) {
	adapter := manifestAdapter{manifest: Manifest{Response: ManifestResponse{StreamedJSONAudio: true, ResultKind: "audio"}}}
	_, err := adapter.ParseCreate(context.Background(), []byte(`{"event":"error","error":{"type":"validation_error","code":"invalid_voice","message":"voice not found"}}`))
	if err == nil || !strings.Contains(err.Error(), "voice not found") {
		t.Fatalf("error = %v", err)
	}
}

func TestDoubaoSeedAudioMapsModeAndReferenceCount(t *testing.T) {
	manifest, err := os.ReadFile("../../../plugin-packages/doubao-streaming-tts/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := LoadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{
		Capability: CapabilityAudio,
		Model:      "seed-audio-1.0",
		Prompt:     "参考音频1的音色",
		Audios:     []MediaReference{{DataURL: "data:audio/mpeg;base64,YXVkaW8=", MIMEType: "audio/mpeg"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	body := spec.Body.(map[string]any)
	if body["audio_mode"] != "unified_edit" {
		t.Fatalf("audio_mode = %#v", body["audio_mode"])
	}
	references, ok := body["references"].([]any)
	if !ok || len(references) != 1 || references[0].(map[string]any)["audio_data"] != "YXVkaW8=" {
		t.Fatalf("references = %#v", body["references"])
	}
}
