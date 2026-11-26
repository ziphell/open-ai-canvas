package commands

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		left  string
		right string
		want  int
	}{
		{"0.1.0", "0.1.0", 0},
		{"v0.2.0", "0.1.0", 1},
		{"0.9.9", "1.0.0", -1},
		{"0.1.0-rc.1", "0.1.0", 0},
		{"1.0", "1.0.0", 0},
		{"1.0.1", "1.0", 1},
		{"", "0.0.1", -1},
		{"1.2.3+build", "1.2.3", 0},
	}
	for _, item := range cases {
		if got := compareVersions(item.left, item.right); got != item.want {
			t.Fatalf("compareVersions(%q, %q)=%d want %d", item.left, item.right, got, item.want)
		}
	}
}
