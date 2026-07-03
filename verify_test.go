package main

import (
	"strings"
	"testing"
)

// The corpus coverage comes for free: every test program and every example
// compiles through verifyAll inside compileProgram. These tests feed
// hand-built bad bytecode straight to verifyStack to prove it actually bites.

func badFn(arity int, code ...byte) *OFunc {
	fn := &OFunc{Name: "bad", Arity: arity}
	for _, b := range code {
		fn.Chunk.write(b, Span{})
	}
	return fn
}

func expectVerifyError(t *testing.T, fn *OFunc, contains string) {
	t.Helper()
	err := verifyStack(fn)
	if err == nil {
		t.Fatalf("expected verifier error containing %q, got none", contains)
	}
	if !strings.Contains(err.Error(), contains) {
		t.Fatalf("verifier error %q does not contain %q", err.Error(), contains)
	}
}

func TestVerifyUnderflow(t *testing.T) {
	// POP with nothing above the frame slots dips into the closure slot
	expectVerifyError(t, badFn(0, OpPop, OpUnit, OpUnit, OpReturn), "stack underflow")
}

func TestVerifyBinaryUnderflow(t *testing.T) {
	// ADD needs two operands; only one sits above the frame floor
	expectVerifyError(t, badFn(0, OpUnit, OpAdd, OpReturn), "stack underflow")
}

func TestVerifyFallOffEnd(t *testing.T) {
	expectVerifyError(t, badFn(0, OpUnit, OpPop), "fell off the end")
}

func TestVerifyMergeMismatch(t *testing.T) {
	// if-true path pushes one extra value before joining the false path
	expectVerifyError(t, badFn(0,
		OpTrue,
		OpJumpIfFalsePop, 0, 1, // skip the next OpUnit when false
		OpUnit, // true path only: depths disagree at the join
		OpUnit, OpReturn,
	), "merge at different stack depths")
}

func TestVerifyLocalAboveStack(t *testing.T) {
	expectVerifyError(t, badFn(0, OpGetLocal, 0, 9, OpReturn), "local slot")
}

func TestVerifyReturnWithoutResult(t *testing.T) {
	// RETURN pops the result; here nothing sits above the frame slots
	expectVerifyError(t, badFn(0, OpReturn), "RETURN with no value")
}

func TestVerifyAcceptsGoodChunk(t *testing.T) {
	fn := badFn(0, OpUnit, OpTrue, OpPop, OpReturn)
	if err := verifyStack(fn); err != nil {
		t.Fatalf("verifier rejected a balanced chunk: %v", err)
	}
}
