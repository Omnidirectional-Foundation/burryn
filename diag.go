package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

type Diag struct {
	IsErr bool
	Code  string
	Msg   string
	Help  string
	Span  Span
}

// lineIndex maps byte offsets to line/column positions: lineIndex[i] is the
// byte offset where 1-based line i+1 starts.
type lineIndex []int

func newLineIndex(src string) lineIndex {
	ix := lineIndex{0}
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			ix = append(ix, i+1)
		}
	}
	return ix
}

// lineCol converts a byte offset to 1-based line and column.
func (ix lineIndex) lineCol(off int) (line, col int) {
	if off < 0 {
		off = 0
	}
	line = sort.Search(len(ix), func(i int) bool { return ix[i] > off })
	return line, off - ix[line-1] + 1
}

func (ix lineIndex) line(off int) int {
	l, _ := ix.lineCol(off)
	return l
}

// renderDiags prints diagnostics rustc-style:
//
//	error[E0308]: mismatched types
//	 --> examples\foo.bur:3:13
//	  |
//	3 |     let x = 1 + "a"
//	  |             ^^^ expected `int`, found `str`
//	  |
//	  = help: ...
func renderDiags(w io.Writer, diags []Diag, file, src string) (errs, warns int) {
	lines := strings.Split(src, "\n")
	ix := newLineIndex(src)
	for _, d := range diags {
		head := "error"
		if !d.IsErr {
			head = "warning"
		}
		if d.Code != "" {
			fmt.Fprintf(w, "%s[%s]: %s\n", head, d.Code, d.Msg)
		} else {
			fmt.Fprintf(w, "%s: %s\n", head, d.Msg)
		}
		start := min(max(d.Span.Start, 0), len(src))
		line, col := ix.lineCol(start)
		if line >= 1 && line <= len(lines) {
			raw := lines[line-1]
			srcLine := strings.ReplaceAll(raw, "\t", "    ")
			gutter := fmt.Sprintf("%d", line)
			pad := strings.Repeat(" ", len(gutter))
			fmt.Fprintf(w, "%s--> %s:%d:%d\n", pad, file, line, col)
			fmt.Fprintf(w, "%s |\n", pad)
			fmt.Fprintf(w, "%s | %s\n", gutter, srcLine)
			// underline the span, clamped to the end of its first line
			width := max(min(d.Span.End, ix[line-1]+len(raw))-start, 1)
			// adjust for expanded tabs before and inside the underlined range
			prefix := raw[:min(col-1, len(raw))]
			seg := raw[min(col-1, len(raw)):min(col-1+width, len(raw))]
			expCol := col + 3*strings.Count(prefix, "\t")
			width += 3 * strings.Count(seg, "\t")
			fmt.Fprintf(w, "%s | %s%s\n", pad, strings.Repeat(" ", expCol-1), strings.Repeat("^", width))
		}
		if d.Help != "" {
			fmt.Fprintf(w, "  = help: %s\n", d.Help)
		}
		fmt.Fprintln(w)
		if d.IsErr {
			errs++
		} else {
			warns++
		}
	}
	return errs, warns
}
