package schema

import "testing"

func TestNormalizeDecimal(t *testing.T) {
	valid := map[string]string{"1999.9": "1999.90", "+007.5": "7.50", "-0": "0.00", "-12": "-12.00", "3.1400": "3.14", "99999999.99": "99999999.99"}
	for in, want := range valid {
		if got, ok := NormalizeDecimal(in, 10, 2); !ok || got != want {
			t.Errorf("NormalizeDecimal(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "-", ".5", "5.", "1e3", "1.234", "100000000.00", "1,5", " 1"} {
		if got, ok := NormalizeDecimal(in, 10, 2); ok {
			t.Errorf("NormalizeDecimal(%q) = %q, want an error", in, got)
		}
	}
	if got, ok := NormalizeDecimal("42", 4, 0); !ok || got != "42" {
		t.Errorf("scale 0 = %q, %v", got, ok)
	}
}
