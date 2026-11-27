package protocol

import (
	"context"
	"net/http"
	"os"
	"testing"
)

func transcriptionRequest() GenerationRequest {
	return GenerationRequest{
		Capability: CapabilityTranscription,
		Model:      "whisper-large-v3",
		Audios:     []MediaReference{{DataURL: "data:audio/wav;base64,YXVkaW8=", MIMEType: "audio/wav", Name: "audio.wav"}},
		Extra:      map[string]any{"language": "zh", "idempotencyKey": "request-1"},
	}
}

func TestOpenAITranscriptionPackageBuildsMultipartRequest(t *testing.T) {
	manifest, err := os.ReadFile("../../../plugin-packages/openai-transcription/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := LoadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: transcriptionRequest()})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != "/v1/audio/transcriptions" || spec.ContentType != "multipart/form-data" {
		t.Fatalf("spec = %#v", spec)
	}
	body := spec.Body.(map[string]any)
	if body["model"] != "whisper-large-v3" || body["response_format"] != "verbose_json" || body["language"] != "zh" {
		t.Fatalf("body = %#v", body)
	}
	if len(spec.Files) != 1 || spec.Files[0].Name != "file" || spec.Files[0].Reference.DataURL != "data:audio/wav;base64,YXVkaW8=" {
		t.Fatalf("files = %#v", spec.Files)
	}
}

func TestWhisperCppTranscriptionPackageOmitsModelAndAuth(t *testing.T) {
	manifest, err := os.ReadFile("../../../plugin-packages/whisper-cpp-transcription/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := LoadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: transcriptionRequest()})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != "/inference" || spec.ContentType != "multipart/form-data" {
		t.Fatalf("spec = %#v", spec)
	}
	body := spec.Body.(map[string]any)
	if _, exists := body["model"]; exists {
		t.Fatalf("whisper.cpp body must omit model: %#v", body)
	}
	if body["response_format"] != "verbose_json" || body["language"] != "zh" {
		t.Fatalf("body = %#v", body)
	}
	if spec.Auth.Type != "none" {
		t.Fatalf("auth = %#v", spec.Auth)
	}
}

func TestTranscriptionResponseMapsSegmentsAndLanguage(t *testing.T) {
	manifest := []byte(`{"apiVersion":"yingce.plugin/v2","id":"transcription-fixture","name":"Fixture","version":"1.0.0","author":"test","contributes":{"providers":[{"id":"transcription-fixture","label":"Fixture","capabilities":["transcription"],"scopes":["admin.system-channel"],"auth":{"type":"none","field":"apiKey"},"create":{"method":"POST","path":"/transcribe","contentType":"application/json"},"response":{"transcription":{"segments":{"$ref":"response.segments"},"language":{"$ref":"response.language"}}}}]}}`)
	adapter, err := LoadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"language":"zh","segments":[{"start":0.0,"end":2.015,"text":" 大家好 "},{"start":4.12,"end":6.121,"text":"今天天气不错"}]}`)
	result, err := adapter.ParseCreate(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	if result.Result == nil || result.Result.Transcription == nil {
		t.Fatalf("result = %#v", result)
	}
	transcription := result.Result.Transcription
	if transcription.Language != "zh" || len(transcription.Segments) != 2 {
		t.Fatalf("transcription = %#v", transcription)
	}
	if transcription.Segments[0].StartMs != 0 || transcription.Segments[0].EndMs != 2015 || transcription.Segments[0].Text != "大家好" {
		t.Fatalf("segment = %#v", transcription.Segments[0])
	}
}

func volcengineTranscriptionRequest() GenerationRequest {
	return GenerationRequest{
		Capability: CapabilityTranscription,
		Model:      "volc.seedasr.auc",
		Audios:     []MediaReference{{URL: "https://cdn.example.com/audio.wav", MIMEType: "audio/wav", Name: "audio.wav"}},
		Extra:      map[string]any{"language": "zh-CN", "idempotencyKey": "task-1", "audioFormat": "wav"},
	}
}

func loadVolcengineTranscriptionAdapter(t *testing.T) Adapter {
	t.Helper()
	manifest, err := os.ReadFile("../../../plugin-packages/volcengine-transcription/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := LoadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !adapter.Metadata().RequiresPublicMediaURLs {
		t.Fatal("volcengine transcription must require public media URLs")
	}
	return adapter
}

func TestVolcengineTranscriptionBuildsSubmitRequest(t *testing.T) {
	adapter := loadVolcengineTranscriptionAdapter(t)
	spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: volcengineTranscriptionRequest()})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Method != http.MethodPost || spec.Path != "/api/v3/auc/bigmodel/submit" {
		t.Fatalf("spec = %#v", spec)
	}
	if spec.Headers["X-Api-Resource-Id"] != "volc.seedasr.auc" || spec.Headers["X-Api-Request-Id"] != "task-1" || spec.Headers["X-Api-Sequence"] != "-1" {
		t.Fatalf("headers = %#v", spec.Headers)
	}
	if spec.Auth.Type != "header" || spec.Auth.Header != "X-Api-Key" {
		t.Fatalf("auth = %#v", spec.Auth)
	}
	body := spec.Body.(map[string]any)
	audio := body["audio"].(map[string]any)
	if audio["url"] != "https://cdn.example.com/audio.wav" || audio["format"] != "wav" {
		t.Fatalf("audio = %#v", audio)
	}
	request := body["request"].(map[string]any)
	if request["model_name"] != "bigmodel" || request["show_utterances"] != true || request["language"] != "zh-CN" {
		t.Fatalf("request = %#v", request)
	}
}

func TestVolcengineTranscriptionSubmitReadsStatusHeader(t *testing.T) {
	adapter := loadVolcengineTranscriptionAdapter(t)
	headers := http.Header{"X-Api-Status-Code": []string{"20000000"}, "X-Api-Message": []string{"OK"}}
	created, err := adapter.(HeaderAwareCreateParser).ParseCreateWithRequestAndHeaders(context.Background(), volcengineTranscriptionRequest(), headers, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != StatusSucceeded || created.TaskID != "task-1" {
		t.Fatalf("created = %#v", created)
	}
	if created.Result != nil {
		t.Fatalf("submit response must not fabricate a transcription result: %#v", created.Result)
	}
}

func TestVolcengineTranscriptionQueryMapsUtterances(t *testing.T) {
	adapter := loadVolcengineTranscriptionAdapter(t)
	headers := http.Header{"X-Api-Status-Code": []string{"20000000"}, "X-Api-Message": []string{"OK"}}
	body := []byte(`{"audio_info":{"duration":6312},"result":{"text":"你好","utterances":[{"start_time":480,"end_time":5880,"text":" 你好 "},{"start_time":6000,"end_time":7000,"text":"世界"}]}}`)
	state, err := adapter.(HeaderAwarePollParser).ParsePollWithHeaders(context.Background(), PollContext{TaskID: "task-1", Request: volcengineTranscriptionRequest()}, headers, body)
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != StatusSucceeded || state.Result == nil || state.Result.Transcription == nil {
		t.Fatalf("state = %#v", state)
	}
	segments := state.Result.Transcription.Segments
	if len(segments) != 2 || segments[0].StartMs != 480 || segments[0].EndMs != 5880 || segments[0].Text != "你好" {
		t.Fatalf("segments = %#v", segments)
	}
	if state.Result.Transcription.Language != "zh-CN" {
		t.Fatalf("language = %q", state.Result.Transcription.Language)
	}
}

func TestVolcengineTranscriptionQueryPendingAndFailedHeaders(t *testing.T) {
	adapter := loadVolcengineTranscriptionAdapter(t)
	poll := func(code, message string) PollResult {
		t.Helper()
		headers := http.Header{"X-Api-Status-Code": []string{code}, "X-Api-Message": []string{message}}
		state, err := adapter.(HeaderAwarePollParser).ParsePollWithHeaders(context.Background(), PollContext{TaskID: "task-1", Request: volcengineTranscriptionRequest()}, headers, []byte(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		return state
	}
	if state := poll("20000002", "queued"); state.Status != StatusProcessing {
		t.Fatalf("queued state = %#v", state)
	}
	if state := poll("45000001", "invalid audio"); state.Status != StatusFailed || state.Message != "invalid audio" {
		t.Fatalf("failed state = %#v", state)
	}
}

func loadMiniMaxTranscriptionAdapter(t *testing.T) Adapter {
	t.Helper()
	manifest, err := os.ReadFile("../../../plugin-packages/minimax-transcription/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := LoadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func TestMiniMaxTranscriptionBuildsMultipartRequest(t *testing.T) {
	adapter := loadMiniMaxTranscriptionAdapter(t)
	request := transcriptionRequest()
	request.Model = "asr-1.0"
	spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Path != "/v1/speech_to_text" || spec.ContentType != "multipart/form-data" {
		t.Fatalf("spec = %#v", spec)
	}
	body := spec.Body.(map[string]any)
	if body["model"] != "asr-1.0" || body["response_format"] != "verbose_json" || body["timestamp_level"] != "sentence" {
		t.Fatalf("body = %#v", body)
	}
	if spec.Headers["language"] != "zh" {
		t.Fatalf("headers = %#v", spec.Headers)
	}
	if len(spec.Files) != 1 || spec.Files[0].Name != "file" || spec.Files[0].Reference.DataURL == "" {
		t.Fatalf("files = %#v", spec.Files)
	}
	if spec.Auth.Type != "bearer" {
		t.Fatalf("auth = %#v", spec.Auth)
	}
}

func TestMiniMaxTranscriptionMapsVerboseJSONSegments(t *testing.T) {
	adapter := loadMiniMaxTranscriptionAdapter(t)
	body := []byte(`{"text":"你好世界","duration":26.325,"n_speakers":2,"segments":[{"id":0,"start":0.48,"end":5.88,"speaker":"S1","text":" 你好 "},{"id":1,"start":6.0,"end":7.2,"speaker":"S2","text":"世界"}],"trace_id":"trace-1"}`)
	request := transcriptionRequest()
	request.Model = "asr-1.0"
	created, err := adapter.(RequestAwareCreateParser).ParseCreateWithRequest(context.Background(), request, body)
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != StatusSucceeded || created.Result == nil || created.Result.Transcription == nil {
		t.Fatalf("created = %#v", created)
	}
	segments := created.Result.Transcription.Segments
	if len(segments) != 2 || segments[0].StartMs != 480 || segments[0].EndMs != 5880 || segments[0].Text != "你好" {
		t.Fatalf("segments = %#v", segments)
	}
	if created.Result.Transcription.Language != "zh" {
		t.Fatalf("language = %q", created.Result.Transcription.Language)
	}
}

func TestMiniMaxTranscriptionBaseRespFailure(t *testing.T) {
	adapter := loadMiniMaxTranscriptionAdapter(t)
	body := []byte(`{"base_resp":{"status_code":1004,"status_msg":"invalid api key"}}`)
	created, err := adapter.ParseCreate(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != StatusFailed || created.Message != "invalid api key" {
		t.Fatalf("created = %#v", created)
	}
}

func TestTranscriptionPackageRequiresAudio(t *testing.T) {
	manifest, err := os.ReadFile("../../../plugin-packages/openai-transcription/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := LoadManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{Capability: CapabilityTranscription, Model: "whisper-large-v3"}})
	if err == nil || err.Error() == "" {
		t.Fatal("expected audio validation error")
	}
}
