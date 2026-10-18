package schema

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

type Pos struct {
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
}

func (p Pos) IsValid() bool {
	return p.File != ""
}

func (p Pos) String() string {
	switch {
	case p.File == "":
		return ""
	case p.Line == 0:
		return p.File
	case p.Column == 0:
		return fmt.Sprintf("%s:%d", p.File, p.Line)
	default:
		return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Column)
	}
}

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

type Diagnostic struct {
	Pos
	Severity   Severity `json:"severity"`
	Code       string   `json:"code"`
	Message    string   `json:"message"`
	Suggestion string   `json:"suggestion,omitempty"`
}

func (d Diagnostic) String() string {
	var b strings.Builder
	if loc := d.Pos.String(); loc != "" {
		b.WriteString(loc)
		b.WriteString(": ")
	}
	if d.Severity == SeverityWarning {
		b.WriteString("warning: ")
	}
	b.WriteString(d.Message)
	if d.Suggestion != "" {
		fmt.Fprintf(&b, " (did you mean %q?)", d.Suggestion)
	}
	fmt.Fprintf(&b, " [%s]", d.Code)
	return b.String()
}

type Diagnostics []Diagnostic

func (ds Diagnostics) HasErrors() bool {
	for _, d := range ds {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

func (ds Diagnostics) Errors() Diagnostics {
	var out Diagnostics
	for _, d := range ds {
		if d.Severity == SeverityError {
			out = append(out, d)
		}
	}
	return out
}

func (ds Diagnostics) Warnings() Diagnostics {
	var out Diagnostics
	for _, d := range ds {
		if d.Severity == SeverityWarning {
			out = append(out, d)
		}
	}
	return out
}

func (ds Diagnostics) Sort() {
	slices.SortStableFunc(ds, func(a, b Diagnostic) int {
		return cmp.Or(
			cmp.Compare(a.File, b.File),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Column, b.Column),
			cmp.Compare(a.Code, b.Code),
			cmp.Compare(a.Message, b.Message),
		)
	})
}

func (ds Diagnostics) Error() string {
	lines := make([]string, len(ds))
	for i, d := range ds {
		lines[i] = d.String()
	}
	return strings.Join(lines, "\n")
}
