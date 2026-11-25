package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func comfyUITestWorkflow() map[string]interface{} {
	return map[string]interface{}{
		"3": map[string]interface{}{"class_type": "KSampler", "inputs": map[string]interface{}{"seed": float64(1), "steps": float64(20)}},
		"6": map[string]interface{}{"class_type": "CLIPTextEncode", "inputs": map[string]interface{}{"text": "default"}},
		"7": map[string]interface{}{"class_type": "LoadImage", "inputs": map[string]interface{}{"image": "default.png"}},
		"9": map[string]interface{}{"class_type": "SaveImage", "inputs": map[string]interface{}{}},
	}
}

func comfyUITestFields() []WorkflowField {
	enabled := true
	return []WorkflowField{
		{ID: "6::text", NodeID: "6", FieldName: "text", FieldType: "TEXT", Source: "prompt", Enabled: &enabled, Required: true},
		{ID: "3::steps", NodeID: "3", FieldName: "steps", FieldType: "NUMBER", FieldValue: "30", Enabled: &enabled},
		{ID: "7::image", NodeID: "7", FieldName: "image", FieldType: "IMAGE", Source: "referenceImage", Enabled: &enabled},
	}
}

func TestParseComfyUIWorkflowJSONAcceptsObjectAndString(t *testing.T) {
	objectPayload, err := json.Marshal(comfyUITestWorkflow())
	if err != nil {
		t.Fatal(err)
	}
	quotedPayload, err := json.Marshal(string(objectPayload))
	if err != nil {
		t.Fatal(err)
	}
	for name, payload := range map[string][]byte{"object": objectPayload, "string": quotedPayload} {
		workflow, err := parseComfyUIWorkflowJSON(payload)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(workflow) != 4 {
			t.Fatalf("%s: expected 4 nodes, got %d", name, len(workflow))
		}
	}
}

func TestParseComfyUIWorkflowJSONRejectsUIGraph(t *testing.T) {
	payload := []byte(`{"version":0.4,"nodes":[{"id":1,"type":"KSampler"}],"links":[]}`)
	if _, err := parseComfyUIWorkflowJSON(payload); err == nil {
		t.Fatal("expected UI graph to be rejected")
	}
}

func TestApplyComfyUIWorkflowFieldsKeepsNativeTypes(t *testing.T) {
	workflow := comfyUITestWorkflow()
	input := canvasGenerationInput{
		Mode:   "image",
		Prompt: "a cat",
		ReferenceImages: []providerMedia{{
			ID:      "img1",
			DataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("png")),
		}},
	}
	files := map[string]string{"img1": "uploaded.png"}
	if err := applyComfyUIWorkflowFields(workflow, comfyUITestFields(), files, input); err != nil {
		t.Fatal(err)
	}
	text := workflow["6"].(map[string]interface{})["inputs"].(map[string]interface{})["text"]
	if text != "a cat" {
		t.Fatalf("prompt = %v", text)
	}
	steps := workflow["3"].(map[string]interface{})["inputs"].(map[string]interface{})["steps"]
	if value, ok := steps.(int64); !ok || value != 30 {
		t.Fatalf("steps = %T(%v), want int64(30)", steps, steps)
	}
	image := workflow["7"].(map[string]interface{})["inputs"].(map[string]interface{})["image"]
	if image != "uploaded.png" {
		t.Fatalf("image = %v", image)
	}
}

func TestApplyComfyUIWorkflowFieldsSkipsLinkedInputs(t *testing.T) {
	workflow := comfyUITestWorkflow()
	workflow["6"].(map[string]interface{})["inputs"].(map[string]interface{})["text"] = []interface{}{"5", float64(0)}
	enabled := true
	fields := []WorkflowField{{NodeID: "6", FieldName: "text", Source: "prompt", Enabled: &enabled}}
	if err := applyComfyUIWorkflowFields(workflow, fields, nil, canvasGenerationInput{Prompt: "a cat"}); err != nil {
		t.Fatal(err)
	}
	value := workflow["6"].(map[string]interface{})["inputs"].(map[string]interface{})["text"]
	if !isWorkflowLinkValue(value) {
		t.Fatalf("linked input was overwritten: %v", value)
	}
}

func TestComfyUIOutputFilesPrefersOutputType(t *testing.T) {
	outputs := map[string]any{
		"9": map[string]any{
			"images": []any{
				map[string]any{"filename": "preview.png", "subfolder": "", "type": "temp"},
				map[string]any{"filename": "final.png", "subfolder": "", "type": "output"},
				map[string]any{"filename": "final.png", "subfolder": "", "type": "output"},
			},
		},
	}
	files := comfyUIOutputFiles(outputs)
	if len(files) != 1 || files[0].Filename != "final.png" {
		t.Fatalf("files = %#v", files)
	}
}

func TestComfyUIOutputURLsEncodeViewQuery(t *testing.T) {
	root := "http://127.0.0.1:8188"
	urls := comfyUIOutputURLs(root, map[string]any{
		"9": map[string]any{
			"gifs": []any{map[string]any{"filename": "my clip.mp4", "subfolder": "anime", "type": "output"}},
		},
	})
	if len(urls) != 1 {
		t.Fatalf("urls = %#v", urls)
	}
	if !strings.HasPrefix(urls[0], root+"/view?") ||
		!strings.Contains(urls[0], "filename=my+clip.mp4") ||
		!strings.Contains(urls[0], "subfolder=anime") ||
		!strings.Contains(urls[0], "type=output") {
		t.Fatalf("url = %s", urls[0])
	}
}

func TestComfyUIErrorMessageReadsNodeErrors(t *testing.T) {
	response := map[string]any{
		"error": map[string]any{"type": "prompt_outputs_failed_validation", "message": "Prompt outputs failed validation"},
		"node_errors": map[string]any{
			"3": map[string]any{"errors": []any{map[string]any{"message": "Value not in list"}}},
		},
	}
	message := comfyUIErrorMessage(response)
	if !strings.Contains(message, "节点 3") || !strings.Contains(message, "Value not in list") {
		t.Fatalf("message = %s", message)
	}
}

func TestValidateComfyUIWorkflowProviderConfigAllowsLocalHostWithAllowlist(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	config := providerConfig{
		InterfaceType: "comfyui-workflow-image",
		BaseURL:       "http://127.0.0.1:8188",
		WorkflowJSON:  comfyUITestWorkflow(),
	}
	if err := validateWorkflowProviderConfig("image", config); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	config.WorkflowJSON = nil
	if err := validateWorkflowProviderConfig("image", config); err == nil {
		t.Fatal("expected missing workflow JSON to fail")
	}
}

func TestRunComfyUIWorkflowAgainstFakeServer(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	var submitted map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/upload/image":
			if err := r.ParseMultipartForm(4 << 20); err != nil {
				t.Errorf("parse upload: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"uploaded.png","subfolder":"","type":"input"}`))
		case r.URL.Path == "/prompt":
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &submitted); err != nil {
				t.Errorf("decode prompt: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"prompt_id":"task-1","number":1,"node_errors":{}}`))
		case r.URL.Path == "/history/task-1":
			_, _ = w.Write([]byte(`{"task-1":{"status":{"status_str":"success","completed":true},"outputs":{"9":{"images":[{"filename":"out.png","subfolder":"","type":"output"}]}}}}`))
		case r.URL.Path == "/view":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("\x89PNG\r\n\x1a\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := &Service{}
	input := canvasGenerationInput{
		Mode:   "image",
		Prompt: "a cat",
		Config: providerConfig{
			InterfaceType:  "comfyui-workflow-image",
			BaseURL:        server.URL,
			WorkflowJSON:   comfyUITestWorkflow(),
			WorkflowFields: comfyUITestFields(),
		},
		ReferenceImages: []providerMedia{{
			ID:      "img1",
			DataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("png")),
		}},
	}
	result, err := service.runComfyUIWorkflow(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	images, _ := result["images"].([]map[string]interface{})
	if result["mode"] != "image" || len(images) != 1 {
		t.Fatalf("result = %#v", result)
	}
	promptPayload, _ := submitted["prompt"].(map[string]any)
	inputs, _ := promptPayload["7"].(map[string]any)["inputs"].(map[string]any)
	if inputs["image"] != "uploaded.png" {
		t.Fatalf("prompt image = %v", inputs["image"])
	}
}
