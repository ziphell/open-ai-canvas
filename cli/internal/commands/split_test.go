package commands

import "testing"

func TestSplitPositionalValueFlags(t *testing.T) {
	result, err := splitPositional([]string{"/tmp/a.png", "--kind", "image", "--config-dir", "/tmp/cfg", "--follow"})
	if err != nil {
		t.Fatal(err)
	}
	if result.value != "/tmp/a.png" {
		t.Fatalf("unexpected value %q", result.value)
	}
	want := []string{"--kind", "image", "--config-dir", "/tmp/cfg", "--follow"}
	if len(result.flags) != len(want) {
		t.Fatalf("unexpected flags %v", result.flags)
	}
	for index := range want {
		if result.flags[index] != want[index] {
			t.Fatalf("flags = %v, want %v", result.flags, want)
		}
	}
}

func TestSplitPositional(t *testing.T) {
	result, err := splitPositional([]string{"canvas_1", "--json", "--base-revision", "12", "--out", "canvas.json"})
	if err != nil {
		t.Fatal(err)
	}
	if result.value != "canvas_1" {
		t.Fatalf("unexpected value %q", result.value)
	}
	want := []string{"--json", "--base-revision", "12", "--out", "canvas.json"}
	if len(result.flags) != len(want) {
		t.Fatalf("unexpected flags %v", result.flags)
	}
	for index := range want {
		if result.flags[index] != want[index] {
			t.Fatalf("flags = %v, want %v", result.flags, want)
		}
	}
}
