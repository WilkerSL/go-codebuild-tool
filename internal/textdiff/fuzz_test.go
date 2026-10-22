package textdiff

import (
	"strings"
	"testing"
)

func FuzzDiff(f *testing.F) {
	f.Add("a\nb\nc\n", "a\nc\nd\n")
	f.Add("", "x\n")
	f.Add("same\n", "same\n")
	f.Add("no newline", "no newline\n")
	f.Fuzz(func(t *testing.T, oldText, newText string) {
		if len(oldText) > 4096 || len(newText) > 4096 {
			return
		}
		a, b := Lines(oldText), Lines(newText)
		var rebuiltOld, rebuiltNew []string
		for _, op := range Diff(a, b) {
			switch op.Kind {
			case Equal:
				rebuiltOld = append(rebuiltOld, op.Line)
				rebuiltNew = append(rebuiltNew, op.Line)
			case Delete:
				rebuiltOld = append(rebuiltOld, op.Line)
			case Insert:
				rebuiltNew = append(rebuiltNew, op.Line)
			}
		}
		if strings.Join(rebuiltOld, "") != strings.Join(a, "") || strings.Join(rebuiltNew, "") != strings.Join(b, "") {
			t.Fatal("applying the diff does not rebuild both texts")
		}
		added, removed := Stats(oldText, newText)
		if added < 0 || removed < 0 || added > len(b) || removed > len(a) {
			t.Fatalf("Stats = +%d -%d for %d and %d lines", added, removed, len(a), len(b))
		}
		_ = Unified("f", oldText, newText, 3)
	})
}
