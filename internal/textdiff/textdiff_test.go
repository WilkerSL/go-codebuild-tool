package textdiff

import (
	"strings"
	"testing"
)

func TestStats(t *testing.T) {
	tests := []struct {
		old, new       string
		added, removed int
	}{
		{"", "", 0, 0},
		{"a\nb\n", "a\nb\n", 0, 0},
		{"", "a\nb\n", 2, 0},
		{"a\nb\n", "", 0, 2},
		{"a\nb\nc\n", "a\nx\nc\n", 1, 1},
		{"a\nb\nc\n", "a\nb\nc\nd\n", 1, 0},
		{"a\nb\nc\nd\n", "b\nd\ne\n", 1, 2},
	}
	for _, tt := range tests {
		added, removed := Stats(tt.old, tt.new)
		if added != tt.added || removed != tt.removed {
			t.Errorf("Stats(%q, %q) = +%d -%d, want +%d -%d", tt.old, tt.new, added, removed, tt.added, tt.removed)
		}
	}
}

func TestDiffReconstructs(t *testing.T) {
	a := Lines("one\ntwo\nthree\nfour\nfive\n")
	b := Lines("zero\none\nthree\nfour\nsix\nfive\n")
	var oldOut, newOut []string
	for _, op := range Diff(a, b) {
		switch op.Kind {
		case Equal:
			oldOut = append(oldOut, op.Line)
			newOut = append(newOut, op.Line)
		case Delete:
			oldOut = append(oldOut, op.Line)
		case Insert:
			newOut = append(newOut, op.Line)
		}
	}
	if strings.Join(oldOut, "") != strings.Join(a, "") || strings.Join(newOut, "") != strings.Join(b, "") {
		t.Fatalf("diff does not reconstruct inputs")
	}
}

func TestUnified(t *testing.T) {
	got := Unified("f.go", "a\nb\nc\n", "a\nx\nc\n", 1)
	want := "--- a/f.go\n+++ b/f.go\n@@ -1,3 +1,3 @@\n a\n-b\n+x\n c\n"
	if got != want {
		t.Fatalf("Unified =\n%s\nwant\n%s", got, want)
	}
	if Unified("f.go", "same\n", "same\n", 3) != "" {
		t.Fatal("expected empty diff")
	}
}
