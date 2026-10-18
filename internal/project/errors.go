package project

import (
	"strings"

	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

type ErrorKind int

const (
	KindInternal ErrorKind = iota
	KindUsage
	KindInvalidSpec
	KindUnsupportedGo
	KindToolchain
	KindOwnership
	KindConfig
)

type Error struct {
	Kind        ErrorKind          `json:"-"`
	Code        string             `json:"code"`
	Message     string             `json:"message"`
	Details     map[string]string  `json:"details,omitempty"`
	Diagnostics schema.Diagnostics `json:"diagnostics,omitempty"`
}

func (e *Error) Error() string {
	if len(e.Diagnostics) == 0 {
		return e.Message
	}
	var b strings.Builder
	b.WriteString(e.Message)
	for _, d := range e.Diagnostics {
		b.WriteString("\n  ")
		b.WriteString(d.String())
	}
	return b.String()
}

func newError(kind ErrorKind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}
