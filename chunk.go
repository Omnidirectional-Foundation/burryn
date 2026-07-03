package main

type Op = byte

const (
	OpConst Op = iota // u16 const idx
	OpUnit
	OpTrue
	OpFalse
	OpPop
	OpPopN         // u8
	OpCloseUpvalue // close top slot, pop it
	OpEndBlock     // u8 n: v=pop; close upvals >= top-n; pop n; push v
	OpGetLocal     // u16 slot
	OpSetLocal     // u16 slot (peeks)
	OpGetGlobal    // u16 name const
	OpDefGlobal    // u16 name const
	OpSetGlobal    // u16 name const (peeks)
	OpGetUpval     // u16
	OpSetUpval     // u16 (peeks)
	OpEq
	OpNeq
	OpGt
	OpGtEq
	OpLt
	OpLtEq
	OpAdd
	OpSub
	OpMul
	OpDiv
	OpMod
	OpNeg
	OpNot
	OpJump           // u16 fwd
	OpJumpIfFalse    // u16 fwd, no pop (&&)
	OpJumpIfTrue     // u16 fwd, no pop (||)
	OpJumpIfFalsePop // u16 fwd, pops cond
	OpLoop           // u16 back
	OpCall           // u8 argc
	OpClosure        // u16 fn const, then per-upval: u8 isLocal, u16 idx
	OpReturn
	OpList        // u16 count
	OpIndexGet    // pops idx, target; push elem
	OpIndexSet    // pops val, idx, target
	OpLen         // pops, push int
	OpTestVariant // u8 variant idx: pops enum type, pops candidate, push bool
	OpGetField    // u8 field idx: pops enum inst, push field
	OpNoMatch     // runtime error: unmatched value at top
	OpTry         // ?: unwrap Ok/Some or early-return Err/None
	OpSpawn       // u8 argc: pops callee+args, new fiber
	OpSend        // pops val, chan (may park fiber)
	OpRecv        // pops chan, push received (may park fiber)
)

type Chunk struct {
	Code   []byte
	Spans  []Span // parallel to Code: source span of each instruction byte
	Consts []Value
}

func (c *Chunk) write(b byte, sp Span) {
	c.Code = append(c.Code, b)
	c.Spans = append(c.Spans, sp)
}

func (c *Chunk) writeU16(v uint16, sp Span) {
	c.write(byte(v>>8), sp)
	c.write(byte(v&0xff), sp)
}

func (c *Chunk) addConst(v Value) uint16 {
	c.Consts = append(c.Consts, v)
	return uint16(len(c.Consts) - 1)
}

func readU16(code []byte, ip int) uint16 {
	return uint16(code[ip])<<8 | uint16(code[ip+1])
}
