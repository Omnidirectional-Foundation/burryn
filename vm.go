package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"time"
)

type FiberStatus uint8

const (
	FReady FiberStatus = iota
	FBlockedSend
	FBlockedRecv
	FDone
)

const timeSlice = 10000 // instructions per fiber before a forced yield

type Frame struct {
	closure *OClosure
	ip      int
	base    int
}

type Fiber struct {
	id         int
	stack      []Value
	top        int
	frames     []Frame
	status     FiberStatus
	sendVal    Value // pending value while blocked on send
	openUpvals []*OUpvalue
}

func (f *Fiber) push(v Value) {
	if f.top == len(f.stack) {
		ns := make([]Value, len(f.stack)*2+64)
		copy(ns, f.stack)
		f.stack = ns
	}
	f.stack[f.top] = v
	f.top++
}

func (f *Fiber) pop() Value {
	f.top--
	return f.stack[f.top]
}

func (f *Fiber) peek(n int) Value { return f.stack[f.top-1-n] }

type VM struct {
	gc        *GC
	globals   map[string]Value
	fibers    []*Fiber
	ready     []*Fiber
	current   *Fiber
	main      *Fiber
	nextFiber int
	start     time.Time
	optEnum   *OEnumType
	resEnum   *OEnumType
	yieldFlag bool
	out       io.Writer
}

type runtimeErr struct {
	msg  string
	line int
}

func (e *runtimeErr) Error() string {
	return fmt.Sprintf("runtime error at line %d: %s", e.line, e.msg)
}

func newVM(gc *GC, shared *Shared) *VM {
	vm := &VM{
		gc:      gc,
		globals: map[string]Value{},
		start:   time.Now(),
		optEnum: shared.enums["Option"].runtime,
		resEnum: shared.enums["Result"].runtime,
		out:     os.Stdout,
	}
	gc.vm = vm
	vm.globals["Option"] = ObjV(vm.optEnum)
	vm.globals["Result"] = ObjV(vm.resEnum)
	registerNatives(vm)
	return vm
}

func (vm *VM) newFiber(cl *OClosure, args []Value) *Fiber {
	f := &Fiber{id: vm.nextFiber, stack: make([]Value, 256)}
	vm.nextFiber++
	f.push(ObjV(cl))
	for _, a := range args {
		f.push(a)
	}
	f.frames = append(f.frames, Frame{closure: cl, ip: 0, base: 0})
	vm.fibers = append(vm.fibers, f)
	return f
}

func (vm *VM) run(mainFn *OFunc) error {
	cl := &OClosure{Fn: mainFn}
	vm.gc.alloc(cl)
	vm.main = vm.newFiber(cl, nil)
	vm.ready = append(vm.ready, vm.main)

	for {
		if len(vm.ready) == 0 {
			// main finished => program over (checked in exec); otherwise deadlock
			blocked := 0
			for _, f := range vm.fibers {
				if f.status == FBlockedSend || f.status == FBlockedRecv {
					blocked++
				}
			}
			if blocked > 0 {
				return fmt.Errorf("fatal: deadlock — all %d remaining fiber(s) are blocked on channels", blocked)
			}
			return nil
		}
		f := vm.ready[0]
		vm.ready = vm.ready[1:]
		if f.status != FReady {
			continue
		}
		vm.current = f
		done, err := vm.exec(f)
		vm.current = nil
		if err != nil {
			return err
		}
		if done && f == vm.main {
			return nil // Go semantics: program exits when main returns
		}
	}
}

func (vm *VM) schedule(f *Fiber) {
	f.status = FReady
	vm.ready = append(vm.ready, f)
}

// exec runs one fiber until it finishes, blocks, or exhausts its time slice.
// Returns done=true when the fiber ran to completion.
func (vm *VM) exec(f *Fiber) (bool, error) {
	budget := timeSlice
	for {
		frame := &f.frames[len(f.frames)-1]
		chunk := &frame.closure.Fn.Chunk
		if budget <= 0 { // preemption point
			vm.schedule(f)
			return false, nil
		}
		budget--
		op := chunk.Code[frame.ip]
		line := chunk.Lines[frame.ip]
		frame.ip++

		rtErr := func(format string, args ...any) error {
			return &runtimeErr{msg: fmt.Sprintf(format, args...), line: line}
		}

		switch op {
		case OpConst:
			idx := readU16(chunk.Code, frame.ip)
			frame.ip += 2
			f.push(chunk.Consts[idx])
		case OpUnit:
			f.push(Unit)
		case OpTrue:
			f.push(BoolV(true))
		case OpFalse:
			f.push(BoolV(false))
		case OpPop:
			f.pop()
		case OpPopN:
			n := int(chunk.Code[frame.ip])
			frame.ip++
			f.top -= n
		case OpCloseUpvalue:
			vm.closeUpvalues(f, f.top-1)
			f.pop()
		case OpEndBlock:
			n := int(chunk.Code[frame.ip])
			frame.ip++
			v := f.pop()
			vm.closeUpvalues(f, f.top-n)
			f.top -= n
			f.push(v)
		case OpGetLocal:
			slot := int(readU16(chunk.Code, frame.ip))
			frame.ip += 2
			f.push(f.stack[frame.base+slot])
		case OpSetLocal:
			slot := int(readU16(chunk.Code, frame.ip))
			frame.ip += 2
			f.stack[frame.base+slot] = f.peek(0)
		case OpGetGlobal:
			idx := readU16(chunk.Code, frame.ip)
			frame.ip += 2
			name := chunk.Consts[idx].O.(*OString).S
			v, ok := vm.globals[name]
			if !ok {
				return false, rtErr("undefined variable %q", name)
			}
			f.push(v)
		case OpDefGlobal:
			idx := readU16(chunk.Code, frame.ip)
			frame.ip += 2
			name := chunk.Consts[idx].O.(*OString).S
			vm.globals[name] = f.pop()
		case OpSetGlobal:
			idx := readU16(chunk.Code, frame.ip)
			frame.ip += 2
			name := chunk.Consts[idx].O.(*OString).S
			if _, ok := vm.globals[name]; !ok {
				return false, rtErr("undefined variable %q", name)
			}
			vm.globals[name] = f.peek(0)
		case OpGetUpval:
			idx := readU16(chunk.Code, frame.ip)
			frame.ip += 2
			f.push(frame.closure.Upvals[idx].get())
		case OpSetUpval:
			idx := readU16(chunk.Code, frame.ip)
			frame.ip += 2
			frame.closure.Upvals[idx].set(f.peek(0))
		case OpEq:
			b := f.pop()
			a := f.pop()
			f.push(BoolV(valuesEqual(a, b)))
		case OpNeq:
			b := f.pop()
			a := f.pop()
			f.push(BoolV(!valuesEqual(a, b)))
		case OpGt, OpGtEq, OpLt, OpLtEq:
			if err := vm.compare(f, op, line); err != nil {
				return false, err
			}
		case OpAdd, OpSub, OpMul, OpDiv, OpMod:
			if err := vm.arith(f, op, line); err != nil {
				return false, err
			}
		case OpNeg:
			v := f.pop()
			switch v.T {
			case VInt:
				if v.I == math.MinInt64 {
					return false, rtErr("integer overflow: -(%d)", v.I)
				}
				f.push(IntV(-v.I))
			case VFloat:
				f.push(FloatV(-v.F))
			default:
				return false, rtErr("operand of '-' must be a number, got %s", typeOf(v))
			}
		case OpNot:
			v := f.pop()
			if v.T != VBool {
				return false, rtErr("operand of '!' must be a bool, got %s", typeOf(v))
			}
			f.push(BoolV(!v.B))
		case OpJump:
			frame.ip += int(readU16(chunk.Code, frame.ip)) + 2
		case OpJumpIfFalse, OpJumpIfTrue, OpJumpIfFalsePop:
			cond := f.peek(0)
			if cond.T != VBool {
				return false, rtErr("condition must be a bool, got %s", typeOf(cond))
			}
			dist := int(readU16(chunk.Code, frame.ip))
			frame.ip += 2
			if op == OpJumpIfFalsePop {
				f.pop()
			}
			if (op == OpJumpIfTrue) == cond.B { // false-jumps when !B, true-jumps when B
				frame.ip += dist
			}
		case OpLoop:
			frame.ip = frame.ip + 2 - int(readU16(chunk.Code, frame.ip))
		case OpCall:
			argc := int(chunk.Code[frame.ip])
			frame.ip++
			if err := vm.callValue(f, argc, line); err != nil {
				return false, err
			}
			if vm.yieldFlag { // a native (yield) asked us to hand off
				vm.yieldFlag = false
				vm.schedule(f)
				return false, nil
			}
		case OpClosure:
			idx := readU16(chunk.Code, frame.ip)
			frame.ip += 2
			fn := chunk.Consts[idx].O.(*OFunc)
			cl := &OClosure{Fn: fn, Upvals: make([]*OUpvalue, fn.NumUpvals)}
			vm.gc.alloc(cl)
			f.push(ObjV(cl))
			for i := 0; i < fn.NumUpvals; i++ {
				isLocal := chunk.Code[frame.ip] == 1
				frame.ip++
				uidx := int(readU16(chunk.Code, frame.ip))
				frame.ip += 2
				if isLocal {
					cl.Upvals[i] = vm.captureUpvalue(f, frame.base+uidx)
				} else {
					cl.Upvals[i] = frame.closure.Upvals[uidx]
				}
			}
		case OpReturn:
			result := f.pop()
			vm.closeUpvalues(f, frame.base)
			f.frames = f.frames[:len(f.frames)-1]
			if len(f.frames) == 0 {
				f.status = FDone
				return true, nil
			}
			f.top = frame.base
			f.push(result)
		case OpList:
			n := int(readU16(chunk.Code, frame.ip))
			frame.ip += 2
			elems := make([]Value, n)
			copy(elems, f.stack[f.top-n:f.top])
			lst := vm.gc.newList(elems) // alloc before popping: elements stay rooted
			f.top -= n
			f.push(ObjV(lst))
		case OpIndexGet:
			idx := f.pop()
			target := f.pop()
			v, err := vm.indexGet(target, idx, line)
			if err != nil {
				return false, err
			}
			f.push(v)
		case OpIndexSet:
			val := f.pop()
			idx := f.pop()
			target := f.pop()
			if target.T != VObj {
				return false, rtErr("cannot index-assign into %s", typeOf(target))
			}
			lst, ok := target.O.(*OList)
			if !ok {
				return false, rtErr("cannot index-assign into %s", typeOf(target))
			}
			if idx.T != VInt {
				return false, rtErr("list index must be an int, got %s", typeOf(idx))
			}
			if idx.I < 0 || idx.I >= int64(len(lst.Elems)) {
				return false, rtErr("list index %d out of bounds (len %d)", idx.I, len(lst.Elems))
			}
			lst.Elems[idx.I] = val
		case OpLen:
			v := f.pop()
			n, err := lengthOf(v, line)
			if err != nil {
				return false, err
			}
			f.push(IntV(n))
		case OpTestVariant:
			vidx := int(chunk.Code[frame.ip])
			frame.ip++
			et := f.pop().O.(*OEnumType)
			cand := f.pop()
			match := false
			if cand.T == VObj {
				if inst, ok := cand.O.(*OEnumInst); ok {
					match = inst.Enum == et && inst.Variant == vidx
				}
			}
			f.push(BoolV(match))
		case OpGetField:
			fidx := int(chunk.Code[frame.ip])
			frame.ip++
			inst := f.pop().O.(*OEnumInst)
			f.push(inst.Fields[fidx])
		case OpNoMatch:
			return false, rtErr("no pattern matched value %s", repr(f.peek(0)))
		case OpTry:
			v := f.peek(0)
			inst, ok := (*OEnumInst)(nil), false
			if v.T == VObj {
				inst, ok = v.O.(*OEnumInst)
			}
			if !ok || (inst.Enum != vm.optEnum && inst.Enum != vm.resEnum) {
				return false, rtErr("'?' needs an Option or Result, got %s", typeOf(v))
			}
			vname := inst.Enum.Variants[inst.Variant].Name
			if vname == "Some" || vname == "Ok" {
				f.pop()
				f.push(inst.Fields[0])
			} else {
				// early-return the None/Err itself
				f.pop()
				vm.closeUpvalues(f, frame.base)
				f.frames = f.frames[:len(f.frames)-1]
				if len(f.frames) == 0 {
					f.status = FDone
					return true, nil
				}
				f.top = frame.base
				f.push(v)
			}
		case OpSpawn:
			argc := int(chunk.Code[frame.ip])
			frame.ip++
			callee := f.peek(argc)
			cl, ok := (*OClosure)(nil), false
			if callee.T == VObj {
				cl, ok = callee.O.(*OClosure)
			}
			if !ok {
				return false, rtErr("spawn needs a function, got %s", typeOf(callee))
			}
			if cl.Fn.Arity != argc {
				return false, rtErr("%s expects %d argument(s), got %d", display(callee), cl.Fn.Arity, argc)
			}
			args := make([]Value, argc)
			copy(args, f.stack[f.top-argc:f.top])
			nf := vm.newFiber(cl, args)
			vm.schedule(nf)
			f.top -= argc + 1
		case OpSend:
			val := f.pop()
			ch, err := popChannel(f, "send to", line)
			if err != nil {
				return false, err
			}
			if len(ch.recvq) > 0 {
				r := ch.recvq[0]
				ch.recvq = ch.recvq[1:]
				r.push(val)
				vm.schedule(r)
			} else if len(ch.buf) < ch.cap {
				ch.buf = append(ch.buf, val)
			} else {
				f.sendVal = val
				f.status = FBlockedSend
				ch.sendq = append(ch.sendq, f)
				return false, nil
			}
		case OpRecv:
			ch, err := popChannel(f, "receive from", line)
			if err != nil {
				return false, err
			}
			if len(ch.buf) > 0 {
				v := ch.buf[0]
				ch.buf = ch.buf[1:]
				if len(ch.sendq) > 0 {
					s := ch.sendq[0]
					ch.sendq = ch.sendq[1:]
					ch.buf = append(ch.buf, s.sendVal)
					s.sendVal = Unit
					vm.schedule(s)
				}
				f.push(v)
			} else if len(ch.sendq) > 0 { // unbuffered rendezvous
				s := ch.sendq[0]
				ch.sendq = ch.sendq[1:]
				f.push(s.sendVal)
				s.sendVal = Unit
				vm.schedule(s)
			} else {
				f.status = FBlockedRecv
				ch.recvq = append(ch.recvq, f)
				return false, nil
			}
		default:
			return false, rtErr("bad opcode %d (VM bug)", op)
		}
	}
}

func popChannel(f *Fiber, what string, line int) (*OChannel, error) {
	v := f.pop()
	if v.T == VObj {
		if ch, ok := v.O.(*OChannel); ok {
			return ch, nil
		}
	}
	return nil, &runtimeErr{msg: fmt.Sprintf("cannot %s %s (need a channel)", what, typeOf(v)), line: line}
}

func (vm *VM) callValue(f *Fiber, argc int, line int) error {
	callee := f.peek(argc)
	if callee.T != VObj {
		return &runtimeErr{msg: fmt.Sprintf("cannot call %s", typeOf(callee)), line: line}
	}
	switch o := callee.O.(type) {
	case *OClosure:
		if argc != o.Fn.Arity {
			return &runtimeErr{msg: fmt.Sprintf("%s expects %d argument(s), got %d", display(callee), o.Fn.Arity, argc), line: line}
		}
		if len(f.frames) >= 2048 {
			return &runtimeErr{msg: "stack overflow (call depth > 2048)", line: line}
		}
		f.frames = append(f.frames, Frame{closure: o, ip: 0, base: f.top - argc - 1})
		return nil
	case *ONative:
		if o.Arity >= 0 && argc != o.Arity {
			return &runtimeErr{msg: fmt.Sprintf("%s expects %d argument(s), got %d", o.Name, o.Arity, argc), line: line}
		}
		args := f.stack[f.top-argc : f.top]
		res, err := o.Fn(vm, args)
		if err != nil {
			return &runtimeErr{msg: err.Error(), line: line}
		}
		f.top -= argc + 1
		f.push(res)
		return nil
	case *OVariantCtor:
		arity := o.Enum.Variants[o.Idx].Arity
		if argc != arity {
			return &runtimeErr{msg: fmt.Sprintf("%s.%s expects %d field(s), got %d", o.Enum.Name, o.Enum.Variants[o.Idx].Name, arity, argc), line: line}
		}
		fields := make([]Value, argc)
		copy(fields, f.stack[f.top-argc:f.top])
		inst := &OEnumInst{Enum: o.Enum, Variant: o.Idx, Fields: fields}
		vm.gc.alloc(inst) // args still on stack: fields stay rooted during collection
		f.top -= argc + 1
		f.push(ObjV(inst))
		return nil
	}
	return &runtimeErr{msg: fmt.Sprintf("cannot call %s", typeOf(callee)), line: line}
}

func (vm *VM) captureUpvalue(f *Fiber, slot int) *OUpvalue {
	for _, uv := range f.openUpvals {
		if uv.slot == slot {
			return uv
		}
	}
	uv := &OUpvalue{fiber: f, slot: slot, open: true}
	vm.gc.alloc(uv)
	f.openUpvals = append(f.openUpvals, uv)
	return uv
}

func (vm *VM) closeUpvalues(f *Fiber, from int) {
	kept := f.openUpvals[:0]
	for _, uv := range f.openUpvals {
		if uv.slot >= from {
			uv.Closed = f.stack[uv.slot]
			uv.open = false
		} else {
			kept = append(kept, uv)
		}
	}
	f.openUpvals = kept
}

func (vm *VM) arith(f *Fiber, op Op, line int) error {
	b := f.peek(0)
	a := f.peek(1)
	fail := func() error {
		opName := map[Op]string{OpAdd: "+", OpSub: "-", OpMul: "*", OpDiv: "/", OpMod: "%"}[op]
		return &runtimeErr{msg: fmt.Sprintf("cannot apply %q to %s and %s", opName, typeOf(a), typeOf(b)), line: line}
	}
	// string concatenation
	if op == OpAdd && a.T == VObj && b.T == VObj {
		as, aok := a.O.(*OString)
		bs, bok := b.O.(*OString)
		if aok && bok {
			s := vm.gc.newString(as.S + bs.S) // alloc before popping
			f.top -= 2
			f.push(ObjV(s))
			return nil
		}
		return fail()
	}
	if a.T == VInt && b.T == VInt {
		// integer overflow always traps — never wraps silently (GOALS)
		overflow := func(sym string) error {
			return &runtimeErr{msg: fmt.Sprintf("integer overflow: %d %s %d", a.I, sym, b.I), line: line}
		}
		f.top -= 2
		switch op {
		case OpAdd:
			r := a.I + b.I
			if (r > a.I) != (b.I > 0) {
				return overflow("+")
			}
			f.push(IntV(r))
		case OpSub:
			r := a.I - b.I
			if (r < a.I) != (b.I > 0) {
				return overflow("-")
			}
			f.push(IntV(r))
		case OpMul:
			r := a.I * b.I
			if a.I != 0 && (r/a.I != b.I || (a.I == -1 && b.I == math.MinInt64)) {
				return overflow("*")
			}
			f.push(IntV(r))
		case OpDiv:
			if b.I == 0 {
				return &runtimeErr{msg: "integer division by zero", line: line}
			}
			if a.I == math.MinInt64 && b.I == -1 {
				return overflow("/")
			}
			f.push(IntV(a.I / b.I))
		case OpMod:
			if b.I == 0 {
				return &runtimeErr{msg: "integer modulo by zero", line: line}
			}
			f.push(IntV(a.I % b.I))
		}
		return nil
	}
	af, bf, ok := toFloats(a, b)
	if !ok || op == OpMod {
		return fail()
	}
	f.top -= 2
	switch op {
	case OpAdd:
		f.push(FloatV(af + bf))
	case OpSub:
		f.push(FloatV(af - bf))
	case OpMul:
		f.push(FloatV(af * bf))
	case OpDiv:
		f.push(FloatV(af / bf))
	}
	return nil
}

func (vm *VM) compare(f *Fiber, op Op, line int) error {
	b := f.pop()
	a := f.pop()
	if a.T == VObj && b.T == VObj {
		as, aok := a.O.(*OString)
		bs, bok := b.O.(*OString)
		if aok && bok {
			var r bool
			switch op {
			case OpGt:
				r = as.S > bs.S
			case OpGtEq:
				r = as.S >= bs.S
			case OpLt:
				r = as.S < bs.S
			case OpLtEq:
				r = as.S <= bs.S
			}
			f.push(BoolV(r))
			return nil
		}
	}
	af, bf, ok := toFloats(a, b)
	if !ok {
		return &runtimeErr{msg: fmt.Sprintf("cannot compare %s and %s", typeOf(a), typeOf(b)), line: line}
	}
	var r bool
	switch op {
	case OpGt:
		r = af > bf
	case OpGtEq:
		r = af >= bf
	case OpLt:
		r = af < bf
	case OpLtEq:
		r = af <= bf
	}
	f.push(BoolV(r))
	return nil
}

func toFloats(a, b Value) (float64, float64, bool) {
	var af, bf float64
	switch a.T {
	case VInt:
		af = float64(a.I)
	case VFloat:
		af = a.F
	default:
		return 0, 0, false
	}
	switch b.T {
	case VInt:
		bf = float64(b.I)
	case VFloat:
		bf = b.F
	default:
		return 0, 0, false
	}
	return af, bf, true
}

func (vm *VM) indexGet(target, idx Value, line int) (Value, error) {
	if target.T != VObj {
		return Unit, &runtimeErr{msg: fmt.Sprintf("cannot index %s", typeOf(target)), line: line}
	}
	switch o := target.O.(type) {
	case *OList:
		if idx.T != VInt {
			return Unit, &runtimeErr{msg: fmt.Sprintf("list index must be an int, got %s", typeOf(idx)), line: line}
		}
		if idx.I < 0 || idx.I >= int64(len(o.Elems)) {
			return Unit, &runtimeErr{msg: fmt.Sprintf("list index %d out of bounds (len %d)", idx.I, len(o.Elems)), line: line}
		}
		return o.Elems[idx.I], nil
	case *OString:
		if idx.T != VInt {
			return Unit, &runtimeErr{msg: fmt.Sprintf("string index must be an int, got %s", typeOf(idx)), line: line}
		}
		if idx.I < 0 || idx.I >= int64(len(o.S)) {
			return Unit, &runtimeErr{msg: fmt.Sprintf("string index %d out of bounds (len %d)", idx.I, len(o.S)), line: line}
		}
		return ObjV(vm.gc.newString(string(o.S[idx.I]))), nil
	}
	return Unit, &runtimeErr{msg: fmt.Sprintf("cannot index %s", typeOf(target)), line: line}
}

func lengthOf(v Value, line int) (int64, error) {
	if v.T == VObj {
		switch o := v.O.(type) {
		case *OList:
			return int64(len(o.Elems)), nil
		case *OString:
			return int64(len(o.S)), nil
		}
	}
	return 0, &runtimeErr{msg: fmt.Sprintf("len() needs a list or string, got %s", typeOf(v)), line: line}
}
