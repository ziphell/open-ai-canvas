package app

import (
	"testing"

	"yingce/backend/internal/model"
)

func TestUpdateUserModelDefaultsPersistsTranscriptionModel(t *testing.T) {
	svc, _ := newTimelineTaskTestService(t)
	preference, err := svc.UpdateUserModelDefaults(&model.User{ID: "user-transcription-default"}, UpdateUserModelDefaultsRequest{
		ImageModel:         "img::image-1",
		VideoModel:         "vid::video-1",
		TextModel:          "txt::text-1",
		AudioModel:         "aud::audio-1",
		TranscriptionModel: "speech::whisper-large-v3",
	})
	if err != nil {
		t.Fatalf("UpdateUserModelDefaults: %v", err)
	}
	if preference.TranscriptionModel != "speech::whisper-large-v3" {
		t.Fatalf("transcription model = %q", preference.TranscriptionModel)
	}
	reloaded, err := svc.UserModelDefaults(&model.User{ID: "user-transcription-default"})
	if err != nil {
		t.Fatalf("UserModelDefaults: %v", err)
	}
	if reloaded.TranscriptionModel != "speech::whisper-large-v3" {
		t.Fatalf("reloaded transcription model = %q", reloaded.TranscriptionModel)
	}
}
