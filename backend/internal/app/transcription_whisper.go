package app

import (
	"fmt"
	"strings"
)

// 时间线转写的统一结果类型。具体上游由声明式转写协议执行，
// 这里只负责把协议返回的段落整理为时间线字幕使用的 SRT。

type timelineTranscriptionSegment struct {
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	Text    string `json:"text"`
}

type timelineTranscriptionResult struct {
	Segments []timelineTranscriptionSegment `json:"segments"`
	SRT      string                         `json:"srt"`
	Language string                         `json:"language"`
}

// formatSRTTimestamp 将毫秒时间格式化为 SRT 时间戳 hh:mm:ss,mmm。
func formatSRTTimestamp(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	hours := ms / 3600000
	minutes := (ms % 3600000) / 60000
	seconds := (ms % 60000) / 1000
	millis := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, millis)
}

// buildTimelineSRT 由段落生成 SRT 字幕文本，供前端直接消费或导出。
func buildTimelineSRT(segments []timelineTranscriptionSegment) string {
	var out strings.Builder
	for i, seg := range segments {
		fmt.Fprintf(&out, "%d\n%s --> %s\n%s\n\n", i+1, formatSRTTimestamp(seg.StartMs), formatSRTTimestamp(seg.EndMs), seg.Text)
	}
	return out.String()
}
