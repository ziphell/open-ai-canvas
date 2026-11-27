package app

import (
	"strings"
	"testing"
)

func TestFormatSRTTimestamp(t *testing.T) {
	cases := []struct {
		ms   int64
		want string
	}{
		{0, "00:00:00,000"},
		{2015, "00:00:02,015"},
		{61001, "00:01:01,001"},
		{3661001, "01:01:01,001"},
		{-5, "00:00:00,000"},
	}
	for _, c := range cases {
		if got := formatSRTTimestamp(c.ms); got != c.want {
			t.Fatalf("formatSRTTimestamp(%d) = %q, want %q", c.ms, got, c.want)
		}
	}
}

func TestBuildTimelineSRT(t *testing.T) {
	segments := []timelineTranscriptionSegment{
		{StartMs: 0, EndMs: 2015, Text: "大家好"},
		{StartMs: 2500, EndMs: 6121, Text: "今天天气不错"},
	}
	srt := buildTimelineSRT(segments)
	want := "1\n00:00:00,000 --> 00:00:02,015\n大家好\n\n2\n00:00:02,500 --> 00:00:06,121\n今天天气不错\n\n"
	if srt != want {
		t.Fatalf("srt mismatch:\n got %q\nwant %q", srt, want)
	}
}

func TestExtForMimeAndTranscribable(t *testing.T) {
	transcribable := []string{"video/mp4", "audio/wav", "audio/mpeg", "video/webm", "audio/flac", "video/quicktime", "audio/aac", "audio/ogg"}
	for _, mime := range transcribable {
		if !isTranscribableMime(mime) {
			t.Fatalf("isTranscribableMime(%q) = false, want true", mime)
		}
		if strings.TrimPrefix(extForMime(mime), ".") == "bin" {
			t.Fatalf("extForMime(%q) fell back to .bin", mime)
		}
	}
	for _, mime := range []string{"image/png", "text/plain", ""} {
		if isTranscribableMime(mime) {
			t.Fatalf("isTranscribableMime(%q) = true, want false", mime)
		}
	}
}
