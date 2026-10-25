package schema

import "strings"

// MaxDecimalPrecision is the largest precision every dialect accepts: SQL
// Server stops at 38 digits.
const MaxDecimalPrecision = 38

// NormalizeDecimal returns the canonical text of a decimal value with the
// given scale, so "1999.9" with scale 2 becomes "1999.90", and false when the
// text is not a plain decimal number that fits the precision and scale.
// Trailing zeros past the scale are accepted and dropped.
func NormalizeDecimal(s string, precision, scale int) (string, bool) {
	negative := false
	switch {
	case strings.HasPrefix(s, "-"):
		negative, s = true, s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}
	whole, frac, dotted := strings.Cut(s, ".")
	if whole == "" || (dotted && frac == "") || !allDigits(whole) || !allDigits(frac) {
		return "", false
	}
	whole = strings.TrimLeft(whole, "0")
	frac = strings.TrimRight(frac, "0")
	if len(whole) > precision-scale || len(frac) > scale {
		return "", false
	}
	if whole == "" {
		whole = "0"
	}
	out := whole
	if scale > 0 {
		out += "." + frac + strings.Repeat("0", scale-len(frac))
	}
	if negative && strings.Trim(out, "0.") != "" {
		out = "-" + out
	}
	return out, true
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
