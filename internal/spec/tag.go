package spec

import (
	"errors"
	"strconv"
	"strings"
)

type tagEntry struct {
	key    string
	value  string
	offset int
}

var errMalformedTag = errors.New("malformed struct tag")

func parseStructTag(tag string) ([]tagEntry, int, error) {
	var entries []tagEntry
	offset := 0
	for tag != "" {
		i := 0
		for i < len(tag) && tag[i] == ' ' {
			i++
		}
		tag = tag[i:]
		offset += i
		if tag == "" {
			break
		}
		i = 0
		for i < len(tag) && tag[i] > ' ' && tag[i] != ':' && tag[i] != '"' && tag[i] != 0x7f {
			i++
		}
		if i == 0 || i+1 >= len(tag) || tag[i] != ':' || tag[i+1] != '"' {
			return entries, offset, errMalformedTag
		}
		key := tag[:i]
		tag = tag[i+1:]
		offset += i + 1
		i = 1
		for i < len(tag) && tag[i] != '"' {
			if tag[i] == '\\' {
				i++
			}
			i++
		}
		if i >= len(tag) {
			return entries, offset, errMalformedTag
		}
		value, err := strconv.Unquote(tag[:i+1])
		if err != nil {
			return entries, offset, errMalformedTag
		}
		entries = append(entries, tagEntry{key: key, value: value, offset: offset + 1})
		tag = tag[i+1:]
		offset += i + 1
	}
	return entries, offset, nil
}

type rawMarker struct {
	key      string
	value    string
	hasValue bool
	offset   int
}

func splitMarkers(value string) []rawMarker {
	var markers []rawMarker
	start := 0
	for i := 0; i <= len(value); i++ {
		if i < len(value) && value[i] != ',' {
			continue
		}
		part := value[start:i]
		trimmedLeft := strings.TrimLeft(part, " \t")
		offset := start + len(part) - len(trimmedLeft)
		text := strings.TrimRight(trimmedLeft, " \t")
		m := rawMarker{offset: offset}
		if key, val, ok := strings.Cut(text, "="); ok {
			m.key = strings.TrimSpace(key)
			m.value = strings.TrimSpace(val)
			m.hasValue = true
		} else {
			m.key = text
		}
		markers = append(markers, m)
		start = i + 1
	}
	return markers
}
