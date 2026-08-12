package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Rustc-style diagnostic rendering, mirroring tooling/render.bur.

func padNum(n, w int) string {
	s := strconv.Itoa(n)
	for len(s) < w {
		s = " " + s
	}
	return s
}

func spacesN(n int) string { return strings.Repeat(" ", n) }

func carets(n int) string { return strings.Repeat("^", n) }

// lineText returns a line's text (byte offsets from the line-start table).
func lineText(src string, starts []int, ln int) string {
	s0 := starts[ln-1]
	e0 := len(src)
	if ln < len(starts) {
		e0 = starts[ln] - 1
	}
	if e0 > s0 {
		return src[s0:e0]
	}
	return ""
}

// snippet builds a code snippet with annotations.
func snippet(parts *[]string, src string, starts []int, file string, s, e int, label string) {
	ln := lineOf(starts, s)
	col := s - starts[ln-1] + 1
	*parts = append(*parts, fmt.Sprintf("  --> %s:%d:%d\n", file, ln, col))
	text := lineText(src, starts, ln)
	lineEnd := starts[ln-1] + len(text)
	stop := e
	if stop > lineEnd {
		stop = lineEnd
	}
	width := stop - s
	if width < 1 {
		width = 1
	}
	nw := len(strconv.Itoa(ln))
	*parts = append(*parts, spacesN(nw+1)+"|\n")
	*parts = append(*parts, padNum(ln, nw)+" | "+text+"\n")
	mark := spacesN(nw+1) + "| " + spacesN(col-1) + carets(width)
	if label != "" {
		mark += " " + label
	}
	*parts = append(*parts, mark+"\n")
}

// renderDiag renders one diagnostic (without the trailing newline),
// mirroring render_diag.
func renderDiag(d Diag, file string) string {
	var parts []string
	kind := "warning"
	if d.IsErr {
		kind = "error"
	}
	parts = append(parts, fmt.Sprintf("%s[%s]: %s\n", kind, d.Code, d.Msg))
	if srcBytes, err := os.ReadFile(file); err == nil {
		src := string(srcBytes)
		starts := lineStarts(src)
		snippet(&parts, src, starts, file, d.Span.Start, d.Span.End, "")
	} else {
		parts = append(parts, fmt.Sprintf("  --> %s:%d..%d (source unavailable)\n", file, d.Span.Start, d.Span.End))
	}
	if d.Help != "" {
		parts = append(parts, "  = help: "+d.Help+"\n")
	}
	out := strings.Join(parts, "")
	return out[:len(out)-1]
}

// lineOf returns the 1-based line containing byte offset off.
func lineOf(starts []int, off int) int {
	lo, hi := 0, len(starts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if starts[mid] <= off {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo + 1
}

// lineStarts mirrors lib::line_starts: offsets of each line's first byte.
func lineStarts(src string) []int {
	var starts []int
	starts = append(starts, 0)
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}
