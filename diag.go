package main

import (
	"fmt"
	"io"
	"strings"
)

type Diag struct {
	IsErr bool
	Code  string
	Msg   string
	Help  string
	Line  int
	Col   int
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
		if d.Line >= 1 && d.Line <= len(lines) {
			srcLine := strings.ReplaceAll(lines[d.Line-1], "\t", "    ")
			gutter := fmt.Sprintf("%d", d.Line)
			pad := strings.Repeat(" ", len(gutter))
			fmt.Fprintf(w, "%s--> %s:%d:%d\n", pad, file, d.Line, max(d.Col, 1))
			fmt.Fprintf(w, "%s |\n", pad)
			fmt.Fprintf(w, "%s | %s\n", gutter, srcLine)
			col := d.Col
			if col < 1 || col > len(srcLine)+1 {
				col = 1
			}
			// underline the token starting at col (tabs already expanded)
			width := tokenWidthAt(lines[d.Line-1], d.Col)
			// adjust col for expanded tabs before it
			expCol := col
			if d.Line-1 < len(lines) {
				prefix := lines[d.Line-1]
				if col-1 <= len(prefix) {
					prefix = prefix[:col-1]
				}
				expCol = col + 3*strings.Count(prefix, "\t")
			}
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

// tokenWidthAt estimates how many characters to underline: the run of
// identifier characters (or one char) starting at 1-based column col.
func tokenWidthAt(line string, col int) int {
	i := col - 1
	if i < 0 || i >= len(line) {
		return 1
	}
	c := line[i]
	if isAlpha(c) || isDigit(c) {
		j := i
		for j < len(line) && (isAlpha(line[j]) || isDigit(line[j])) {
			j++
		}
		return j - i
	}
	// operators: extend across a short symbol run
	j := i
	for j < len(line) && strings.ContainsRune("+-*/%<>=!&|?", rune(line[j])) && j-i < 2 {
		j++
	}
	if j == i {
		return 1
	}
	return j - i
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
