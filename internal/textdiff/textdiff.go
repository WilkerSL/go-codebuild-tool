// Package textdiff is a small line diff used for plan summaries and --diff output.
package textdiff

import (
	"fmt"
	"strings"
)

type OpKind int

const (
	Equal OpKind = iota
	Insert
	Delete
)

type Op struct {
	Kind OpKind
	Line string
}

func Lines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func Diff(a, b []string) []Op {
	n, m := len(a), len(b)
	offset := n + m
	v := make([]int, 2*offset+2)
	var trace [][]int
	for d := 0; d <= offset; d++ {
		snapshot := make([]int, len(v))
		copy(snapshot, v)
		trace = append(trace, snapshot)
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = x
			if x >= n && y >= m {
				return backtrack(trace, a, b, d, offset)
			}
		}
	}
	return nil
}

func backtrack(trace [][]int, a, b []string, d, offset int) []Op {
	x, y := len(a), len(b)
	var ops []Op
	for ; d > 0; d-- {
		v := trace[d]
		k := x - y
		var prevK int
		if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := v[offset+prevK]
		prevY := prevX - prevK
		for x > prevX && y > prevY {
			x--
			y--
			ops = append(ops, Op{Kind: Equal, Line: a[x]})
		}
		if x == prevX {
			y--
			ops = append(ops, Op{Kind: Insert, Line: b[y]})
		} else {
			x--
			ops = append(ops, Op{Kind: Delete, Line: a[x]})
		}
	}
	for x > 0 && y > 0 {
		x--
		y--
		ops = append(ops, Op{Kind: Equal, Line: a[x]})
	}
	for i, j := 0, len(ops)-1; i < j; i, j = i+1, j-1 {
		ops[i], ops[j] = ops[j], ops[i]
	}
	return ops
}

func Stats(oldText, newText string) (added, removed int) {
	for _, op := range Diff(Lines(oldText), Lines(newText)) {
		switch op.Kind {
		case Insert:
			added++
		case Delete:
			removed++
		}
	}
	return added, removed
}

func Unified(name, oldText, newText string, context int) string {
	ops := Diff(Lines(oldText), Lines(newText))
	changed := false
	for _, op := range ops {
		if op.Kind != Equal {
			changed = true
			break
		}
	}
	if !changed {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "--- a/%s\n+++ b/%s\n", name, name)
	i := 0
	oldLine, newLine := 1, 1
	for i < len(ops) {
		if ops[i].Kind == Equal {
			oldLine++
			newLine++
			i++
			continue
		}
		start := max(0, i-context)
		end := i
		for end < len(ops) {
			if ops[end].Kind != Equal {
				end++
				continue
			}
			run := end
			for run < len(ops) && ops[run].Kind == Equal {
				run++
			}
			if run-end > 2*context || run == len(ops) {
				end = min(len(ops), end+context)
				break
			}
			end = run
		}
		hunkOld, hunkNew := oldLine-(i-start), newLine-(i-start)
		var oldCount, newCount int
		var body strings.Builder
		for _, op := range ops[start:end] {
			line := op.Line
			if !strings.HasSuffix(line, "\n") {
				line += "\n"
			}
			switch op.Kind {
			case Equal:
				oldCount++
				newCount++
				body.WriteString(" " + line)
			case Delete:
				oldCount++
				body.WriteString("-" + line)
			case Insert:
				newCount++
				body.WriteString("+" + line)
			}
		}
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", hunkOld, oldCount, hunkNew, newCount)
		b.WriteString(body.String())
		for _, op := range ops[i:end] {
			switch op.Kind {
			case Equal:
				oldLine++
				newLine++
			case Delete:
				oldLine++
			case Insert:
				newLine++
			}
		}
		i = end
	}
	return b.String()
}
