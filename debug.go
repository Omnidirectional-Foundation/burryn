package main

import "fmt"

// disasmAll prints the bytecode of a function and, recursively, of every
// function found in its constant table.
func disasmAll(fn *OFunc, shared *Shared) {
	disasm(fn, shared.lines[fn.File])
	for _, c := range fn.Chunk.Consts {
		if c.T == VObj {
			if sub, ok := c.O.(*OFunc); ok {
				fmt.Println()
				disasmAll(sub, shared)
			}
		}
	}
}

func disasm(fn *OFunc, lines lineIndex) {
	name := fn.Name
	if name == "" {
		name = "<anonymous>"
	}
	fmt.Printf("== %s (arity %d, upvals %d) ==\n", name, fn.Arity, fn.NumUpvals)
	ch := &fn.Chunk
	for ip := 0; ip < len(ch.Code); {
		ip = disasmInst(ch, lines, ip)
	}
}

func disasmInst(ch *Chunk, lines lineIndex, ip int) int {
	line := 0 // synthetic code has the zero span and stays line 0
	if ch.Spans[ip] != (Span{}) {
		line = lines.line(ch.Spans[ip].Start)
	}
	fmt.Printf("%04d %4d  ", ip, line)
	op := ch.Code[ip]
	simple := func(name string) int {
		fmt.Println(name)
		return ip + 1
	}
	u16 := func(name string) int {
		v := readU16(ch.Code, ip+1)
		fmt.Printf("%-16s %d\n", name, v)
		return ip + 3
	}
	u16c := func(name string) int {
		v := readU16(ch.Code, ip+1)
		fmt.Printf("%-16s %d (%s)\n", name, v, repr(ch.Consts[v]))
		return ip + 3
	}
	u8 := func(name string) int {
		fmt.Printf("%-16s %d\n", name, ch.Code[ip+1])
		return ip + 2
	}
	jump := func(name string, sign int) int {
		v := int(readU16(ch.Code, ip+1))
		fmt.Printf("%-16s %04d -> %04d\n", name, ip, ip+3+sign*v)
		return ip + 3
	}

	switch op {
	case OpConst:
		return u16c("CONST")
	case OpUnit:
		return simple("UNIT")
	case OpTrue:
		return simple("TRUE")
	case OpFalse:
		return simple("FALSE")
	case OpPop:
		return simple("POP")
	case OpPopN:
		return u8("POP_N")
	case OpCloseUpvalue:
		return simple("CLOSE_UPVALUE")
	case OpEndBlock:
		return u8("END_BLOCK")
	case OpGetLocal:
		return u16("GET_LOCAL")
	case OpSetLocal:
		return u16("SET_LOCAL")
	case OpGetGlobal:
		return u16c("GET_GLOBAL")
	case OpDefGlobal:
		return u16c("DEF_GLOBAL")
	case OpSetGlobal:
		return u16c("SET_GLOBAL")
	case OpGetUpval:
		return u16("GET_UPVAL")
	case OpSetUpval:
		return u16("SET_UPVAL")
	case OpEq:
		return simple("EQ")
	case OpNeq:
		return simple("NEQ")
	case OpGt:
		return simple("GT")
	case OpGtEq:
		return simple("GT_EQ")
	case OpLt:
		return simple("LT")
	case OpLtEq:
		return simple("LT_EQ")
	case OpAdd:
		return simple("ADD")
	case OpSub:
		return simple("SUB")
	case OpMul:
		return simple("MUL")
	case OpDiv:
		return simple("DIV")
	case OpMod:
		return simple("MOD")
	case OpNeg:
		return simple("NEG")
	case OpNot:
		return simple("NOT")
	case OpJump:
		return jump("JUMP", 1)
	case OpJumpIfFalse:
		return jump("JUMP_IF_FALSE", 1)
	case OpJumpIfTrue:
		return jump("JUMP_IF_TRUE", 1)
	case OpJumpIfFalsePop:
		return jump("JUMP_IF_FALSE_P", 1)
	case OpLoop:
		return jump("LOOP", -1)
	case OpCall:
		return u8("CALL")
	case OpClosure:
		v := readU16(ch.Code, ip+1)
		fn := ch.Consts[v].O.(*OFunc)
		fmt.Printf("%-16s %d (%s)\n", "CLOSURE", v, repr(ch.Consts[v]))
		next := ip + 3
		for i := 0; i < fn.NumUpvals; i++ {
			kind := "upval"
			if ch.Code[next] == 1 {
				kind = "local"
			}
			fmt.Printf("%04d    |    capture %s %d\n", next, kind, readU16(ch.Code, next+1))
			next += 3
		}
		return next
	case OpReturn:
		return simple("RETURN")
	case OpList:
		return u16("LIST")
	case OpIndexGet:
		return simple("INDEX_GET")
	case OpIndexSet:
		return simple("INDEX_SET")
	case OpLen:
		return simple("LEN")
	case OpTestVariant:
		return u8("TEST_VARIANT")
	case OpGetField:
		return u8("GET_FIELD")
	case OpNoMatch:
		return simple("NO_MATCH")
	case OpTry:
		return simple("TRY")
	case OpSpawn:
		return u8("SPAWN")
	case OpSend:
		return simple("SEND")
	case OpRecv:
		return simple("RECV")
	case OpChanNext:
		return jump("CHAN_NEXT", 1)
	case OpSelect:
		nArms := int(ch.Code[ip+1])
		hasDefault := ch.Code[ip+2] != 0
		p := ip + 3
		fmt.Printf("%-16s arms=%d default=%v\n", "SELECT", nArms, hasDefault)
		for i := 0; i < nArms; i++ {
			kind := "recv"
			if ch.Code[p] == 1 {
				kind = "send"
			}
			target := p + 3 + int(readU16(ch.Code, p+1))
			fmt.Printf("%04d    |    arm %d %s -> %04d\n", p, i, kind, target)
			p += 3
		}
		if hasDefault {
			target := p + 2 + int(readU16(ch.Code, p))
			fmt.Printf("%04d    |    default -> %04d\n", p, target)
			p += 2
		}
		return p
	}
	fmt.Printf("UNKNOWN %d\n", op)
	return ip + 1
}
