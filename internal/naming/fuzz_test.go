package naming

import "testing"

func FuzzNames(f *testing.F) {
	for _, seed := range []string{"OrgID", "user_profiles", "HTTPServer", "Café", "ÆØÅ", "a--b", "", "x1Y2"} {
		f.Add(seed, "OrgIDs")
	}
	f.Fuzz(func(t *testing.T, s, other string) {
		snake := Snake(s)
		if Snake(snake) != snake {
			t.Fatalf("Snake is not idempotent: %q -> %q -> %q", s, snake, Snake(snake))
		}
		_ = Kebab(s)
		_ = LowerCamel(s)
		_ = Pascal(s)
		_ = Plural(snake)
		_ = IsSQLIdentifier(s)
		if d := Distance(s, other); d < 0 || d > max(len([]rune(s)), len([]rune(other))) {
			t.Fatalf("Distance(%q, %q) = %d", s, other, d)
		}
		if got := Suggest(s, []string{other}); got != "" && got != other {
			t.Fatalf("Suggest returned %q, which is not a candidate", got)
		}
	})
}
