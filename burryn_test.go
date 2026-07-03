package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// interpretMode runs a Burryn source string and returns its stdout.
// With skipCheck=false the static checker runs first; its errors come back
// as compileError. Warnings never fail a run. skipCheck=true is white-box
// only — it bypasses the checker to exercise the VM's defensive runtime
// traps (the language itself has no dynamic mode).
func interpretMode(src string, skipCheck bool) (out string, compileError error, runtimeError error) {
	toks, lexDiags := lex(src)
	if err := diagsToErr(lexDiags); err != nil {
		return "", err, nil
	}
	stmts, parseDiags := parse(toks)
	if err := diagsToErr(parseDiags); err != nil {
		return "", err, nil
	}
	if !skipCheck {
		diags := typecheck(stmts)
		var msgs []string
		for _, d := range diags {
			if d.IsErr {
				msgs = append(msgs, fmt.Sprintf("[%s] %s", d.Code, d.Msg))
			}
		}
		if len(msgs) > 0 {
			return "", fmt.Errorf("%s", strings.Join(msgs, "\n")), nil
		}
	}
	gc := newGC()
	fn, shared, compDiags := compileProgram(gc, src, stmts)
	if err := diagsToErr(compDiags); err != nil {
		return "", err, nil
	}
	var buf bytes.Buffer
	vm := newVM(gc, shared)
	vm.out = &buf
	if err := vm.run(fn); err != nil {
		return buf.String(), nil, err
	}
	return buf.String(), nil, nil
}

func interpret(src string) (string, error, error) { return interpretMode(src, false) }

// diagsToErr flattens error diagnostics into a single error for test helpers
// that assert on message substrings; nil when the list has no errors.
func diagsToErr(diags []Diag) error {
	var msgs []string
	for _, d := range diags {
		if !d.IsErr {
			continue
		}
		if d.Code != "" {
			msgs = append(msgs, fmt.Sprintf("[%s] %s", d.Code, d.Msg))
		} else {
			msgs = append(msgs, d.Msg)
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(msgs, "\n"))
}

// checkDiags returns all diagnostics (errors and warnings) for a source.
func checkDiags(t *testing.T, src string) []Diag {
	t.Helper()
	toks, lexDiags := lex(src)
	if err := diagsToErr(lexDiags); err != nil {
		t.Fatalf("lex error: %v", err)
	}
	stmts, parseDiags := parse(toks)
	if err := diagsToErr(parseDiags); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return typecheck(stmts)
}

func expectOut(t *testing.T, src, want string) {
	t.Helper()
	out, cerr, rerr := interpret(src)
	if cerr != nil {
		t.Fatalf("compile error: %v\nsource:\n%s", cerr, src)
	}
	if rerr != nil {
		t.Fatalf("runtime error: %v\noutput so far:\n%s\nsource:\n%s", rerr, out, src)
	}
	if out != want {
		t.Fatalf("wrong output\n--- got ---\n%s\n--- want ---\n%s\nsource:\n%s", out, want, src)
	}
}

func expectOutUnchecked(t *testing.T, src, want string) {
	t.Helper()
	out, cerr, rerr := interpretMode(src, true)
	if cerr != nil || rerr != nil {
		t.Fatalf("error (unchecked): %v %v\nsource:\n%s", cerr, rerr, src)
	}
	if out != want {
		t.Fatalf("wrong output (unchecked)\n--- got ---\n%s\n--- want ---\n%s", out, want)
	}
}

func expectCompileError(t *testing.T, src, contains string) {
	t.Helper()
	_, cerr, rerr := interpret(src)
	if cerr == nil {
		t.Fatalf("expected compile error containing %q, got none (runtime err: %v)\nsource:\n%s", contains, rerr, src)
	}
	if !strings.Contains(cerr.Error(), contains) {
		t.Fatalf("compile error %q does not contain %q", cerr.Error(), contains)
	}
}

func expectTypeError(t *testing.T, src, code string) {
	t.Helper()
	for _, d := range checkDiags(t, src) {
		if d.IsErr && d.Code == code {
			return
		}
	}
	t.Fatalf("expected a %s error, got none\nsource:\n%s", code, src)
}

func expectWarning(t *testing.T, src, contains string) {
	t.Helper()
	for _, d := range checkDiags(t, src) {
		if !d.IsErr && strings.Contains(d.Msg, contains) {
			return
		}
	}
	t.Fatalf("expected a warning containing %q\nsource:\n%s", contains, src)
}

func expectRuntimeError(t *testing.T, src, contains string) {
	t.Helper()
	_, cerr, rerr := interpret(src)
	if cerr != nil {
		t.Fatalf("unexpected compile error: %v\nsource:\n%s", cerr, src)
	}
	if rerr == nil {
		t.Fatalf("expected runtime error containing %q, got none\nsource:\n%s", contains, src)
	}
	if !strings.Contains(rerr.Error(), contains) {
		t.Fatalf("runtime error %q does not contain %q", rerr.Error(), contains)
	}
}

// ---- lexer diagnostics ----

func TestLexErrorsAreCollected(t *testing.T) {
	_, diags := lex("let a = 1 & 2\nlet b = $\nlet s = \"x\\qy\"")
	want := []string{"E1001", "E1001", "E1003"}
	if len(diags) != len(want) {
		t.Fatalf("expected %d lex diags, got %d: %v", len(want), len(diags), diags)
	}
	for i, d := range diags {
		if d.Code != want[i] {
			t.Errorf("diag %d: expected code %s, got %s (%s)", i, want[i], d.Code, d.Msg)
		}
		if !d.IsErr || d.Span.End <= d.Span.Start {
			t.Errorf("diag %d: bad IsErr/span: %+v", i, d)
		}
	}
}

func TestLexUnterminatedString(t *testing.T) {
	_, diags := lex("let s = \"oops")
	if len(diags) != 1 || diags[0].Code != "E1002" {
		t.Fatalf("expected one E1002 diag, got %v", diags)
	}
}

// ---- parser diagnostics ----

func parseSrc(t *testing.T, src string) ([]Stmt, []Diag) {
	t.Helper()
	toks, lexDiags := lex(src)
	if err := diagsToErr(lexDiags); err != nil {
		t.Fatalf("lex error: %v", err)
	}
	return parse(toks)
}

func TestParseErrorRecoveryCollectsSeveral(t *testing.T) {
	stmts, diags := parseSrc(t, "let = 1\nlet b = )\nlet c = 3")
	want := []string{"E1101", "E1109"}
	if len(diags) != len(want) {
		t.Fatalf("expected %d parse diags, got %d: %v", len(want), len(diags), diags)
	}
	for i, d := range diags {
		if d.Code != want[i] {
			t.Errorf("diag %d: expected code %s, got %s (%s)", i, want[i], d.Code, d.Msg)
		}
		if !d.IsErr || d.Span.End <= d.Span.Start {
			t.Errorf("diag %d: bad IsErr/span: %+v", i, d)
		}
	}
	if len(stmts) != 1 {
		t.Fatalf("expected the healthy trailing statement to survive, got %d stmts", len(stmts))
	}
}

func TestParseErrorRecoveryInsideBlock(t *testing.T) {
	stmts, diags := parseSrc(t, "fn f() {\n    let = 3\n}\nlet ok = 1")
	if len(diags) != 1 {
		t.Fatalf("expected one parse diag (no cascade from the block closer), got %d: %v", len(diags), diags)
	}
	if len(stmts) != 1 {
		t.Fatalf("expected the statement after the broken fn to survive, got %d stmts", len(stmts))
	}
}

func TestParseErrorCap(t *testing.T) {
	_, diags := parseSrc(t, strings.Repeat("let = 1\n", maxParseDiags+10))
	if len(diags) != maxParseDiags {
		t.Fatalf("expected recovery to stop at %d diags, got %d", maxParseDiags, len(diags))
	}
}

// ---- basics ----

func TestArithmetic(t *testing.T) {
	expectOut(t, `println(1 + 2 * 3 - 4 / 2)`, "5\n")
	expectOut(t, `println(10 % 3, 2.5 * 2.0, 7 / 2, 7.0 / 2.0)`, "1 5.0 3 3.5\n")
	expectOut(t, `println(-(3 + 4), !(1 > 2))`, "-7 true\n")
	expectOut(t, `println(1 < 2 && 2 < 3 || false)`, "true\n")
}

func TestStrings(t *testing.T) {
	expectOut(t, `println("bur" + "ryn", str_len("burryn"), "abc" < "abd")`, "burryn 6 true\n")
	expectOut(t, `let s = "ring"
println(char_at(s, 0) + char_at(s, 3))`, "rg\n")
	expectOut(t, `println("tab\tquote\"done")`, "tab\tquote\"done\n")
}

func TestIntegerOverflowTraps(t *testing.T) {
	// overflow always traps — never wraps silently
	max := "9223372036854775807"
	expectRuntimeError(t, `println(`+max+` + 1)`, "integer overflow")
	expectRuntimeError(t, `println(0 - `+max+` - 2)`, "integer overflow")
	expectRuntimeError(t, `println(`+max+` * 2)`, "integer overflow")
	expectRuntimeError(t, `println((0 - `+max+` - 1) * (0 - 1))`, "integer overflow")
	expectRuntimeError(t, `println((0 - `+max+` - 1) / (0 - 1))`, "integer overflow")
	expectRuntimeError(t, `let n = 0 - `+max+` - 1
println(0 - n)`, "integer overflow")
	expectRuntimeError(t, `let n = 0 - `+max+` - 1
println(-n)`, "integer overflow")
	// boundary values themselves are fine
	expectOut(t, `println(`+max+`, 0 - `+max+` - 1)`,
		"9223372036854775807 -9223372036854775808\n")
	expectOut(t, `println((0 - `+max+` - 1) % (0 - 1))`, "0\n")
}

func TestDivisionByZero(t *testing.T) {
	expectRuntimeError(t, `println(1 / 0)`, "division by zero")
	expectRuntimeError(t, `println(1 % 0)`, "modulo by zero")
}

// ---- the strict type system ----

func TestNoImplicitConversions(t *testing.T) {
	expectTypeError(t, `let x = 1 + 2.0`, "E0308")
	expectTypeError(t, `let b = 1 < 2.0`, "E0308")
	expectTypeError(t, `let e = 1 == "one"`, "E0308")
	expectOut(t, `println(to_float(1) + 2.0, trunc(2.9) + 1)`, "3.0 3\n")
}

func TestConditionsMustBeBool(t *testing.T) {
	expectTypeError(t, `if 1 { println("no") }`, "E0308")
	expectTypeError(t, `let x = !3`, "E0308")
	expectTypeError(t, `while "yes" { }`, "E0308")
}

func TestIfWithoutElseMustBeUnit(t *testing.T) {
	expectTypeError(t, `let v = if true { 1 }`, "E0308")
	expectOut(t, `if true { println("side effects are fine") }`, "side effects are fine\n")
}

func TestListsAreHomogeneous(t *testing.T) {
	expectTypeError(t, `let l = [1, "two"]`, "E0308")
	expectOut(t, `println([1, 2] == [1, 2], [1] == [2])`, "true false\n")
}

func TestInferenceIsGeneric(t *testing.T) {
	expectOut(t, `fn id(x) { x }
println(id(42), id("ring"))`, "42 ring\n")
	// constrained polymorphism: + works for int/float/str, nothing else
	expectOut(t, `fn add(a, b) { a + b }
println(add(1, 2), add("bu", "r"))`, "3 bur\n")
	expectTypeError(t, `fn add(a, b) { a + b }
let x = add(true, false)`, "E0308")
}

func TestArityCheckedStatically(t *testing.T) {
	expectTypeError(t, `fn two(a, b) { a }
let x = two(1)`, "E0061")
}

func TestUndefinedName(t *testing.T) {
	expectTypeError(t, `println(nowhere)`, "E0425")
}

func TestChannelsAreTyped(t *testing.T) {
	expectTypeError(t, `let ch = chan()
ch <- 1
ch <- "two"`, "E0308")
	expectTypeError(t, `let n = <-42`, "E0308")
}

func TestMustUseResult(t *testing.T) {
	expectTypeError(t, `fn f() { Ok(1) }
f()`, "unused_must_use")
	expectOut(t, `fn f() { Ok(1) }
let _ = f()
println("fine")`, "fine\n")
}

func TestUnusedVariableWarning(t *testing.T) {
	expectWarning(t, `fn f() {
    let ghost = 1
    2
}
println(f())`, "unused variable: `ghost`")
	expectWarning(t, `fn helper(x) { x }
println(1)`, "unused function: `helper`")
}

func TestTryTypeDiscipline(t *testing.T) {
	// ? on a Result inside a fn returning Option: error types must line up
	expectTypeError(t, `fn r() { Ok(1) }
fn f() {
    let v = r()?
    Some(v)
}
println(f())`, "E0308")
	expectTypeError(t, `let x = 3?`, "E0277")
}

// ---- variables ----

func TestImmutableByDefault(t *testing.T) {
	expectCompileError(t, `let x = 1
x = 2`, "immutable")
	expectOut(t, `let mut x = 1
x = x + 41
println(x)`, "42\n")
}

func TestShadowing(t *testing.T) {
	expectOut(t, `let x = 1
let x = x + 1
let x = x * 10
println(x)`, "20\n")
}

func TestUndeclaredAssign(t *testing.T) {
	expectCompileError(t, `ghost = 1`, "undeclared")
}

// ---- expression orientation ----

func TestIfIsAnExpression(t *testing.T) {
	expectOut(t, `let v = if 3 > 2 { "yes" } else { "no" }
println(v)`, "yes\n")
	expectOut(t, `println(if false { 1 } else if true { 2 } else { 3 })`, "2\n")
}

func TestBlockValue(t *testing.T) {
	expectOut(t, `let v = {
    let a = 40
    let b = 2
    a + b
}
println(v)`, "42\n")
	expectOut(t, `println(1 + { let a = 2; a * 3 })`, "7\n")
}

func TestFunctionImplicitReturn(t *testing.T) {
	expectOut(t, `fn add(a, b) { a + b }
println(add(2, 3))`, "5\n")
	expectOut(t, `fn f(flag) {
    if flag { return 1 }
    2
}
println(f(true), f(false))`, "1 2\n")
}

// ---- closures ----

func TestClosureCounter(t *testing.T) {
	expectOut(t, `fn make() {
    let mut n = 0
    fn() { n = n + 1
        n }
}
let a = make()
let b = make()
let _ = a()
let _ = a()
println(a(), b())`, "3 1\n")
}

func TestClosureSharedUpvalue(t *testing.T) {
	expectOut(t, `fn pair() {
    let mut n = 0
    let bump = fn() { n = n + 1
        n }
    let get = fn() { n }
    [bump, get]
}
let p = pair()
let bump = p[0]
let get = p[1]
let _ = bump()
let _ = bump()
println(get())`, "2\n")
}

func TestLoopVariableCapture(t *testing.T) {
	expectOut(t, `let mut fns = []
for i in range(0, 3) {
    push(fns, fn() { i })
}
let f0 = fns[0]
let f1 = fns[1]
let f2 = fns[2]
println(f0(), f1(), f2())`, "0 1 2\n")
}

// ---- loops ----

func TestWhileBreakContinue(t *testing.T) {
	expectOut(t, `let mut i = 0
let mut acc = []
while true {
    i = i + 1
    if i % 2 == 0 { continue }
    if i > 7 { break }
    push(acc, i)
}
println(acc)`, "[1, 3, 5, 7]\n")
}

func TestForOverList(t *testing.T) {
	expectOut(t, `let mut s = ""
for w in ["a", "b", "c"] {
    s = s + w
}
println(s)`, "abc\n")
}

func TestBreakOutsideLoop(t *testing.T) {
	expectCompileError(t, `break`, "outside of a loop")
}

// ---- lists ----

func TestListOps(t *testing.T) {
	expectOut(t, `let mut l = [1, 2, 3]
push(l, 4)
l[0] = 10
println(l, len(l))
println(pop(l), l)`, "[10, 2, 3, 4] 4\n4 [10, 2, 3]\n")
	expectOut(t, `println([[1], [2, 3]])`, "[[1], [2, 3]]\n")
}

// ---- deep mut: an immutable binding freezes its contents too ----

func TestDeepMutPushNeedsMut(t *testing.T) {
	expectTypeError(t, `let l = [1]
push(l, 2)`, "E0596")
	expectTypeError(t, `let l = [1]
let _ = pop(l)`, "E0596")
}

func TestDeepMutIndexAssignNeedsMut(t *testing.T) {
	expectTypeError(t, `let l = [1]
l[0] = 2`, "E0596")
}

func TestDeepMutReachesThroughNesting(t *testing.T) {
	expectTypeError(t, `let m = [[1]]
m[0][0] = 2`, "E0596")
	expectOut(t, `let mut m = [[1]]
m[0][0] = 2
push(m[0], 3)
println(m)`, "[[2, 3]]\n")
}

func TestDeepMutImmutableAlias(t *testing.T) {
	// an immutable alias of a mut list cannot be used to mutate it
	expectTypeError(t, `let mut a = [1]
let b = a
push(b, 2)`, "E0596")
}

func TestDeepMutParamsAreImmutable(t *testing.T) {
	expectTypeError(t, `fn add(xs, v) { push(xs, v) }
let mut l = [1]
add(l, 2)`, "E0596")
}

func TestDeepMutTemporariesAreFree(t *testing.T) {
	// unnamed temporaries have no binding to freeze
	expectOut(t, `fn fresh() { [1] }
push(fresh(), 2)
println("ok")`, "ok\n")
}

func TestDeepMutReadsStayLegal(t *testing.T) {
	expectOut(t, `let l = [10, 20]
println(l[1], len(l))`, "20 2\n")
}

func TestDeepMutShadowedPushIsNotChecked(t *testing.T) {
	// a user fn named push is an ordinary call, not the mutating native
	expectOut(t, `fn push(a, b) { a + b }
let l = [1]
println(push(2, 3), l)`, "5 [1]\n")
}

func TestIndexOutOfBounds(t *testing.T) {
	expectRuntimeError(t, `let l = [1]
println(l[5])`, "out of bounds")
}

// ---- enums and match ----

func TestEnumMatch(t *testing.T) {
	expectOut(t, `enum Shape { Circle(int), Rect(int, int), Dot }
fn area(s) {
    match s {
        Circle(r) => r * r * 3,
        Shape.Rect(w, h) => w * h,
        Dot => 0,
    }
}
println(area(Circle(2)), area(Rect(3, 4)), area(Shape.Dot))`, "12 12 0\n")
}

func TestGenericEnum(t *testing.T) {
	expectOut(t, `enum Box(a) { Full(a), Empt }
fn unwrap_or(b, alt) {
    match b {
        Full(v) => v,
        Empt => alt,
    }
}
println(unwrap_or(Full(9), 0), unwrap_or(Empt, 7), unwrap_or(Full("hi"), ""))`, "9 7 hi\n")
}

func TestMatchLiteralsAndBinding(t *testing.T) {
	expectOut(t, `fn name(n) {
    match n {
        0 => "zero",
        1 => "one",
        other => "many(" + str(other) + ")",
    }
}
println(name(0), name(1), name(9))`, "zero one many(9)\n")
}

func TestMatchIsExpressionInsideCall(t *testing.T) {
	// regression: bindings must resolve correct slots with temporaries below
	expectOut(t, `enum E { V(int) }
println("got " + match E.V(7) { V(x) => str(x * 2) })`, "got 14\n")
}

func TestMatchExhaustiveness(t *testing.T) {
	expectTypeError(t, `enum Light { Red, Amber, Green }
let s = match Red {
    Red => "stop",
    Green => "go",
}`, "E0004")
	expectTypeError(t, `let s = match Some(1) { Some(v) => v }`, "E0004")
	expectTypeError(t, `let n = match 42 { 1 => "one" }`, "E0004")
	expectOut(t, `println(match true { true => "y", false => "n" })`, "y\n")
	expectTypeError(t, `let s = match true { true => "y" }`, "E0004")
}

func TestMatchArmTypesMustAgree(t *testing.T) {
	expectTypeError(t, `let x = match 1 { 1 => "one", _ => 2 }`, "E0308")
}

func TestEnumErrors(t *testing.T) {
	expectCompileError(t, `enum A { X }
enum A { Y }`, "more than once")
	expectCompileError(t, `enum B { V(int) }
let v = B.V(1)
let x = match v { V(a, b) => a }`, "binds 2")
	expectCompileError(t, `enum C { P(int, int) }
let x = C.P(1)`, "E0061")
	expectOut(t, `enum D { Q(int) }
println(D.Q(5), type_of(D.Q(5)))`, "Q(5) D\n")
	expectTypeError(t, `enum F { W(nosuchtype) }`, "E0412")
}

func TestVMMatchEnumIdentityNotJustIndex(t *testing.T) {
	// VM defense regression test: variant tests check enum identity, not tag
	expectOutUnchecked(t, `enum A { X(v) }
enum B { Y(v) }
fn what(e) {
    match e {
        X(v) => "A.X",
        Y(v) => "B.Y",
        _ => "?",
    }
}
println(what(A.X(1)), what(B.Y(1)), what(42))`, "A.X B.Y ?\n")
}

func TestVMMatchNoMatchTrap(t *testing.T) {
	_, _, rerr := interpretMode(`match 42 { 1 => "one" }`, true)
	if rerr == nil || !strings.Contains(rerr.Error(), "no pattern matched") {
		t.Fatalf("expected VM no-match trap, got %v", rerr)
	}
}

// ---- Option / Result / ? ----

func TestOptionResult(t *testing.T) {
	expectOut(t, `println(Some(1), None, Ok("y"), Err(9))`, "Some(1) None Ok(\"y\") Err(9)\n")
	expectOut(t, `println(Some(1) == Some(1), Some(1) == None)`, "true false\n")
}

func TestTryOperator(t *testing.T) {
	expectOut(t, `fn half(n) {
    if n % 2 == 0 { Ok(n / 2) } else { Err("odd: " + str(n)) }
}
fn quarter(n) {
    let h = half(n)?
    half(h)
}
println(quarter(8), quarter(6), quarter(5))`, "Ok(2) Err(\"odd: 3\") Err(\"odd: 5\")\n")
	expectOut(t, `fn first(l) {
    if len(l) > 0 { Some(l[0]) } else { None }
}
fn first_doubled(l) {
    let v = first(l)?
    Some(v * 2)
}
println(first_doubled([21]), first_doubled([]))`, "Some(42) None\n")
}

func TestParseIsAnOption(t *testing.T) {
	expectOut(t, `println(parse_int("42"), parse_int("nope"))`, "Some(42) None\n")
	expectOut(t, `fn or_zero(s) {
    match parse_int(s) {
        Some(n) => n,
        None => 0,
    }
}
println(or_zero("7") + or_zero("x"))`, "7\n")
}

// ---- fibers and channels ----

func TestUnbufferedRendezvous(t *testing.T) {
	expectOut(t, `fn worker(ch) {
    ch <- "pong"
}
let ch = chan()
spawn worker(ch)
println(<-ch)`, "pong\n")
}

func TestBufferedChannel(t *testing.T) {
	expectOut(t, `let ch = chan(3)
ch <- 1
ch <- 2
ch <- 3
println(<-ch, <-ch, <-ch)`, "1 2 3\n")
}

func TestChannelPingPong(t *testing.T) {
	expectOut(t, `fn echo(in_ch, out_ch) {
    while true {
        let v = <-in_ch
        out_ch <- v * 10
    }
}
let a = chan()
let b = chan()
spawn echo(a, b)
a <- 1
println(<-b)
a <- 7
println(<-b)`, "10\n70\n")
}

func TestDeadlockDetection(t *testing.T) {
	expectRuntimeError(t, `let ch = chan()
let _v = <-ch
println(1)`, "deadlock")
	expectRuntimeError(t, `let ch = chan()
ch <- 1`, "deadlock")
}

func TestSieveSmall(t *testing.T) {
	expectOut(t, `fn generate(ch) {
    let mut i = 2
    while true { ch <- i
        i = i + 1 }
}
fn filter(in_ch, out_ch, p) {
    while true {
        let n = <-in_ch
        if n % p != 0 { out_ch <- n }
    }
}
let mut ch = chan()
spawn generate(ch)
let mut primes = []
while len(primes) < 5 {
    let p = <-ch
    push(primes, p)
    let nx = chan()
    spawn filter(ch, nx, p)
    ch = nx
}
println(primes)`, "[2, 3, 5, 7, 11]\n")
}

func TestVMSpawnNeedsFunctionTrap(t *testing.T) {
	_, _, rerr := interpretMode(`spawn println("x")()`, true)
	if rerr == nil || !strings.Contains(rerr.Error(), "spawn needs a function") {
		t.Fatalf("expected VM spawn trap, got %v", rerr)
	}
}

func TestMainExitEndsProgram(t *testing.T) {
	expectOut(t, `fn spin() { while true { yield() } }
spawn spin()
println("main done")`, "main done\n")
}

// ---- GC ----

func TestGCCollectsGarbage(t *testing.T) {
	expectOut(t, `let base = { let _ = gc()
    heap_objects() }
for i in range(0, 2000) {
    let _junk = [str(i), str(i + 1)]
}
let _ = gc()
let after = heap_objects()
println(after - base < 50)
println(gc_cycles() > 0)`, "true\ntrue\n")
}

func TestGCKeepsLiveData(t *testing.T) {
	expectOut(t, `let keep = ["deep", "nested", "values"]
for i in range(0, 3000) {
    let _junk = str(i) + "!"
}
let _ = gc()
println(keep)`, "[\"deep\", \"nested\", \"values\"]\n")
}

func TestGCKeepsClosedUpvalues(t *testing.T) {
	expectOut(t, `fn hoard() {
    let treasure = "gold-" + str(999)
    fn() { treasure }
}
let get = hoard()
for i in range(0, 3000) {
    let _junk = [str(i)]
}
let _ = gc()
println(get())`, "gold-999\n")
}

// ---- recursion ----

func TestRecursion(t *testing.T) {
	expectOut(t, `fn fib(n) {
    if n < 2 { n } else { fib(n - 1) + fib(n - 2) }
}
println(fib(15))`, "610\n")
}

func TestLocalFnRecursion(t *testing.T) {
	expectOut(t, `fn outer() {
    fn fact(n) {
        if n <= 1 { 1 } else { n * fact(n - 1) }
    }
    fact(5)
}
println(outer())`, "120\n")
}

func TestStackOverflow(t *testing.T) {
	expectRuntimeError(t, `fn boom() { boom() }
boom()`, "stack overflow")
}

// ---- misc ----

func TestAssert(t *testing.T) {
	expectRuntimeError(t, `assert(1 == 2, "math is broken")`, "math is broken")
	expectOut(t, `assert(true, "fine")
println("ok")`, "ok\n")
}

func TestGlobalDefinedLaterVisibleInFn(t *testing.T) {
	expectOut(t, `fn use_it() { later * 2 }
let later = 21
println(use_it())`, "42\n")
}
