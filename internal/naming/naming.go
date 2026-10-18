package naming

import (
	"sort"
	"strings"
	"unicode"
)

var initialisms = map[string]bool{
	"acl": true, "api": true, "ascii": true, "cpu": true, "css": true, "dns": true,
	"eof": true, "guid": true, "html": true, "http": true, "https": true, "id": true,
	"ip": true, "json": true, "lhs": true, "qps": true, "ram": true, "rhs": true,
	"rpc": true, "sla": true, "smtp": true, "sql": true, "ssh": true, "tcp": true,
	"tls": true, "ttl": true, "udp": true, "ui": true, "uid": true, "uri": true,
	"url": true, "utf8": true, "uuid": true, "vm": true, "xml": true, "xmpp": true,
	"xsrf": true, "xss": true,
}

var irregularPlurals = map[string]string{
	"calf":      "calves",
	"child":     "children",
	"criterion": "criteria",
	"echo":      "echoes",
	"elf":       "elves",
	"foot":      "feet",
	"goose":     "geese",
	"half":      "halves",
	"hero":      "heroes",
	"knife":     "knives",
	"leaf":      "leaves",
	"life":      "lives",
	"loaf":      "loaves",
	"man":       "men",
	"mouse":     "mice",
	"person":    "people",
	"potato":    "potatoes",
	"quiz":      "quizzes",
	"scarf":     "scarves",
	"self":      "selves",
	"shelf":     "shelves",
	"thief":     "thieves",
	"tomato":    "tomatoes",
	"tooth":     "teeth",
	"wife":      "wives",
	"wolf":      "wolves",
	"woman":     "women",
}

var uncountable = map[string]bool{
	"data":        true,
	"equipment":   true,
	"feedback":    true,
	"information": true,
	"metadata":    true,
	"news":        true,
	"series":      true,
	"species":     true,
}

func isSeparator(r rune) bool {
	return r == '_' || r == '-' || r == ' ' || r == '.'
}

func Words(s string) []string {
	runes := []rune(s)
	var words []string
	var current []rune
	flush := func() {
		if len(current) > 0 {
			words = append(words, string(current))
			current = nil
		}
	}
	for i, r := range runes {
		if isSeparator(r) {
			flush()
			continue
		}
		if i > 0 && unicode.IsUpper(r) && len(current) > 0 {
			prev := runes[i-1]
			switch {
			case unicode.IsLower(prev) || unicode.IsDigit(prev):
				flush()
			case unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]):
				pluralAcronym := runes[i+1] == 's' && (i+2 == len(runes) || unicode.IsUpper(runes[i+2]) || isSeparator(runes[i+2]))
				if !pluralAcronym {
					flush()
				}
			}
		}
		current = append(current, r)
	}
	flush()
	return words
}

func Snake(s string) string {
	words := Words(s)
	for i, w := range words {
		words[i] = strings.ToLower(w)
	}
	return strings.Join(words, "_")
}

func Kebab(s string) string {
	words := Words(s)
	for i, w := range words {
		words[i] = strings.ToLower(w)
	}
	return strings.Join(words, "-")
}

func LowerCamel(s string) string {
	words := Words(s)
	var b strings.Builder
	for i, w := range words {
		lower := strings.ToLower(w)
		if i == 0 {
			b.WriteString(lower)
			continue
		}
		b.WriteString(capitalize(lower))
	}
	return b.String()
}

func Pascal(s string) string {
	words := Words(s)
	var b strings.Builder
	for _, w := range words {
		lower := strings.ToLower(w)
		if initialisms[lower] {
			b.WriteString(strings.ToUpper(lower))
			continue
		}
		b.WriteString(capitalize(lower))
	}
	return b.String()
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func Plural(snake string) string {
	idx := strings.LastIndex(snake, "_")
	prefix, last := "", snake
	if idx >= 0 {
		prefix, last = snake[:idx+1], snake[idx+1:]
	}
	return prefix + pluralWord(last)
}

func pluralWord(w string) string {
	if w == "" || uncountable[w] {
		return w
	}
	if p, ok := irregularPlurals[w]; ok {
		return p
	}
	switch {
	case strings.HasSuffix(w, "is") && len(w) > 3:
		return w[:len(w)-2] + "es"
	case strings.HasSuffix(w, "s"), strings.HasSuffix(w, "x"), strings.HasSuffix(w, "z"),
		strings.HasSuffix(w, "ch"), strings.HasSuffix(w, "sh"):
		return w + "es"
	case strings.HasSuffix(w, "y") && len(w) > 1 && !isVowel(rune(w[len(w)-2])):
		return w[:len(w)-1] + "ies"
	default:
		return w + "s"
	}
}

func isVowel(r rune) bool {
	return strings.ContainsRune("aeiou", r)
}

func IsSQLIdentifier(s string) bool {
	if s == "" || len(s) > 63 {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_', r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

func Suggest(input string, candidates []string) string {
	if input == "" || len(candidates) == 0 {
		return ""
	}
	sorted := append([]string(nil), candidates...)
	sort.Strings(sorted)
	limit := 1
	switch n := len([]rune(input)); {
	case n > 6:
		limit = 3
	case n > 3:
		limit = 2
	}
	best, bestDistance := "", limit+1
	lowerInput := strings.ToLower(input)
	for _, c := range sorted {
		d := Distance(lowerInput, strings.ToLower(c))
		if d < bestDistance {
			best, bestDistance = c, d
		}
	}
	if bestDistance > limit {
		return ""
	}
	return best
}

func Distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	n, m := len(ra), len(rb)
	d := make([][]int, n+1)
	for i := range d {
		d[i] = make([]int, m+1)
		d[i][0] = i
	}
	for j := 0; j <= m; j++ {
		d[0][j] = j
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[n][m]
}
