package main

import "fmt"

// The bytecode verifier walks every control-flow path of a compiled chunk
// tracking the exact operand-stack depth, and reports any inconsistency:
// stack underflow, conflicting depths where paths merge (loop headers,
// if/else joins), local-slot access above the live stack, or falling off
// the end of the chunk. This pins down the compiler's temp bookkeeping
// (c.temps), which local slot numbering depends on — every compile runs
// through it, so any miscount fails fast as an internal error instead of
// corrupting the VM stack at a distance.

// verifyAll verifies fn and, recursively, every function in its constants.
func verifyAll(fn *OFunc) error {
	if err := verifyStack(fn); err != nil {
		return err
	}
	for _, c := range fn.Chunk.Consts {
		if c.T == VObj {
			if sub, ok := c.O.(*OFunc); ok {
				if err := verifyAll(sub); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func verifyStack(fn *OFunc) error {
	code := fn.Chunk.Code
	consts := fn.Chunk.Consts
	name := fn.Name
	if name == "" {
		name = "<anonymous>"
	}
	errf := func(ip int, format string, a ...any) error {
		return fmt.Errorf("internal compiler error: bytecode verifier: %s @%04d: %s",
			name, ip, fmt.Sprintf(format, a...))
	}

	// depth recorded at each instruction start; -1 = not yet visited
	seen := make([]int, len(code))
	for i := range seen {
		seen[i] = -1
	}
	type state struct{ ip, depth int }
	// frame layout: slot 0 holds the closure itself, params fill 1..arity.
	// Those slots are the frame floor: readable via GetLocal but never
	// popped as operands — depth may not dip below it inside the frame.
	floor := fn.Arity + 1
	work := []state{{0, floor}}

	for len(work) > 0 {
		st := work[len(work)-1]
		work = work[:len(work)-1]
		ip, depth := st.ip, st.depth

	path:
		for {
			if ip >= len(code) {
				return errf(ip, "fell off the end of the chunk")
			}
			if d := seen[ip]; d >= 0 {
				if d != depth {
					return errf(ip, "paths merge at different stack depths: %d vs %d", depth, d)
				}
				break // rest already explored from here
			}
			seen[ip] = depth
			op := code[ip]

			// operand decoding
			u8 := func() int { return int(code[ip+1]) }
			u16 := func() int { return int(readU16(code, ip+1)) }

			var pops, pushes, next int
			switch op {
			case OpConst, OpGetGlobal:
				pushes, next = 1, ip+3
			case OpUnit, OpTrue, OpFalse:
				pushes, next = 1, ip+1
			case OpPop, OpCloseUpvalue:
				pops, next = 1, ip+1
			case OpPopN:
				pops, next = u8(), ip+2
			case OpEndBlock:
				n := u8()
				pops, pushes, next = n+1, 1, ip+2
			case OpGetLocal, OpSetLocal:
				slot := u16()
				if slot >= depth {
					return errf(ip, "local slot %d touched with stack depth %d", slot, depth)
				}
				if op == OpGetLocal {
					pushes = 1
				}
				next = ip + 3
			case OpDefGlobal:
				pops, next = 1, ip+3
			case OpSetGlobal:
				next = ip + 3
			case OpGetUpval:
				pushes, next = 1, ip+3
			case OpSetUpval:
				next = ip + 3
			case OpEq, OpNeq, OpGt, OpGtEq, OpLt, OpLtEq,
				OpAdd, OpSub, OpMul, OpDiv, OpMod:
				pops, pushes, next = 2, 1, ip+1
			case OpNeg, OpNot, OpLen:
				pops, pushes, next = 1, 1, ip+1
			case OpJump:
				ip += 3 + u16()
				continue path
			case OpJumpIfFalse, OpJumpIfTrue, OpJumpIfFalsePop:
				if depth < 1 {
					return errf(ip, "conditional jump with empty stack")
				}
				if op == OpJumpIfFalsePop {
					depth--
				}
				work = append(work, state{ip + 3 + u16(), depth})
				ip += 3
				continue path
			case OpLoop:
				ip += 3 - u16()
				if ip < 0 {
					return errf(st.ip, "loop target out of range")
				}
				continue path
			case OpCall:
				argc := u8()
				pops, pushes, next = argc+1, 1, ip+2
			case OpClosure:
				idx := u16()
				if idx >= len(consts) || consts[idx].T != VObj {
					return errf(ip, "closure operand %d is not a function constant", idx)
				}
				sub, ok := consts[idx].O.(*OFunc)
				if !ok {
					return errf(ip, "closure operand %d is not a function constant", idx)
				}
				pushes, next = 1, ip+3+3*sub.NumUpvals
			case OpReturn, OpNoMatch:
				if depth < floor+1 {
					return errf(ip, "%s with no value above the frame slots (depth %d, floor %d)",
						opName(op), depth, floor)
				}
				break path // terminal
			case OpList:
				n := u16()
				pops, pushes, next = n, 1, ip+3
			case OpIndexGet:
				pops, pushes, next = 2, 1, ip+1
			case OpIndexSet:
				pops, next = 3, ip+1
			case OpTestVariant:
				pops, pushes, next = 2, 1, ip+2
			case OpGetField:
				pops, pushes, next = 1, 1, ip+2
			case OpTry:
				pops, pushes, next = 1, 1, ip+1
			case OpSpawn:
				pops, next = u8()+1, ip+2
			case OpSend:
				pops, next = 2, ip+1
			case OpRecv:
				pops, pushes, next = 1, 1, ip+1
			case OpChanNext:
				// pops the channel; on a value it pushes one and falls through
				// (net zero), on closure it jumps with nothing pushed
				if depth < floor+1 {
					return errf(ip, "chan-next with no channel on the stack (depth %d, floor %d)", depth, floor)
				}
				work = append(work, state{ip + 3 + u16(), depth - 1})
				next = ip + 3 // fall-through depth unchanged: -1 chan, +1 value
			case OpSelect:
				// operands (channels/values) sit on the stack; OpSelect pops
				// them all and transfers to one arm body (a received value is
				// pushed for receive arms). It never falls through.
				nArms := int(code[ip+1])
				hasDefault := code[ip+2] != 0
				p := ip + 3
				slots := 0
				type armT struct {
					recv   bool
					target int
				}
				armv := make([]armT, nArms)
				for i := 0; i < nArms; i++ {
					kind := code[p]
					p++
					target := p + 2 + int(readU16(code, p))
					p += 2
					armv[i] = armT{recv: kind == 0, target: target}
					if kind == 1 {
						slots += 2
					} else {
						slots++
					}
				}
				base := depth - slots
				if base < floor {
					return errf(ip, "select pops %d operand(s) into the frame slots (depth %d, floor %d)", slots, depth, floor)
				}
				for _, a := range armv {
					d := base
					if a.recv {
						d = base + 1 // received value is bound in the arm body
					}
					work = append(work, state{a.target, d})
				}
				if hasDefault {
					target := p + 2 + int(readU16(code, p))
					work = append(work, state{target, base})
				}
				break path // OpSelect always transfers to an arm
			default:
				return errf(ip, "unknown opcode %d", op)
			}

			if depth-pops < floor {
				return errf(ip, "stack underflow: %s pops %d into the frame slots (depth %d, floor %d)",
					opName(op), pops, depth, floor)
			}
			depth += pushes - pops
			ip = next
		}
	}
	return nil
}

func opName(op Op) string {
	names := map[Op]string{
		OpReturn: "RETURN", OpNoMatch: "NO_MATCH", OpPop: "POP", OpPopN: "POP_N",
		OpEndBlock: "END_BLOCK", OpCall: "CALL", OpList: "LIST", OpSend: "SEND",
		OpSpawn: "SPAWN", OpIndexSet: "INDEX_SET", OpIndexGet: "INDEX_GET",
	}
	if n, ok := names[op]; ok {
		return n
	}
	return fmt.Sprintf("op %d", op)
}
