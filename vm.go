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
	FBlockedSelect
	FDone
)

const timeSlice = 10000 // instructions per fiber before a forced yield

type Frame struct {
	closure *OClosure
	ip      int
	base    int
}

type Fiber struct {
	id          int
	stack       []Value
	top         int
	frames      []Frame
	status      FiberStatus
	sendVal     Value       // pending value while blocked on send
	selectChans []*OChannel // channels this fiber waits on while blocked in select
	openUpvals  []*OUpvalue
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
	outEnum   *OEnumType
	yieldFlag bool
	parkRecv  *OChannel // set by the recv() native to ask OpCall to park & retry
	out       io.Writer
	args      []string // user command-line arguments, exposed via args()
}

type runtimeErr struct {
	msg  string
	file string // source file the span refers to; "" in scripts
	span Span
}

func (e *runtimeErr) Error() string {
	return "runtime error: " + e.msg
}

// exitRequest is raised by the exit() native and unwinds the fiber loop
// unwrapped, so main can translate it into a process exit code rather than
// reporting it as a runtime trap.
type exitRequest struct{ code int }

func (e *exitRequest) Error() string { return "exit" }

func newVM(gc *GC, shared *Shared) *VM {
	vm := &VM{
		gc:      gc,
		globals: map[string]Value{},
		start:   time.Now(),
		optEnum: shared.enums["Option"].runtime,
		resEnum: shared.enums["Result"].runtime,
		outEnum: shared.enums["Output"].runtime,
		out:     os.Stdout,
	}
	gc.vm = vm
	vm.globals["Option"] = ObjV(vm.optEnum)
	vm.globals["Result"] = ObjV(vm.resEnum)
	vm.globals["Output"] = ObjV(vm.outEnum)
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
				if f.status == FBlockedSend || f.status == FBlockedRecv || f.status == FBlockedSelect {
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
			// the erroring frame is still on top: attribute the span's file
			if re, ok := err.(*runtimeErr); ok && re.file == "" && len(f.frames) > 0 {
				re.file = f.frames[len(f.frames)-1].closure.Fn.File
			}
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
		startIP := frame.ip // opcode position, for rewinding a parked receive
		op := chunk.Code[frame.ip]
		sp := chunk.Spans[frame.ip]
		frame.ip++

		rtErr := func(format string, args ...any) error {
			return &runtimeErr{msg: fmt.Sprintf(format, args...), span: sp}
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
			if err := vm.compare(f, op, sp); err != nil {
				return false, err
			}
		case OpAdd, OpSub, OpMul, OpDiv, OpMod:
			if err := vm.arith(f, op, sp); err != nil {
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
			if err := vm.callValue(f, argc, sp); err != nil {
				return false, err
			}
			if vm.parkRecv != nil { // the recv() native asked to block on a channel
				ch := vm.parkRecv
				vm.parkRecv = nil
				f.status = FBlockedRecv
				ch.recvq = append(ch.recvq, f)
				vm.wakeWaiters(ch) // a select send arm can now proceed
				frame.ip = startIP // re-run the call (and recv) once woken
				return false, nil
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
			v, err := vm.indexGet(target, idx, sp)
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
			n, err := lengthOf(v, sp)
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
			ch, err := popChannel(f, "send to", sp)
			if err != nil {
				return false, err
			}
			if ch.closed {
				return false, rtErr("send on closed channel")
			}
			if len(ch.recvq) > 0 {
				// hand the value off through the buffer; the woken receiver
				// re-runs its receive and finds it there
				ch.buf = append(ch.buf, val)
				r := ch.recvq[0]
				ch.recvq = ch.recvq[1:]
				vm.schedule(r)
				vm.wakeWaiters(ch)
			} else if len(ch.buf) < ch.cap {
				ch.buf = append(ch.buf, val)
				vm.wakeWaiters(ch)
			} else {
				f.sendVal = val
				f.status = FBlockedSend
				ch.sendq = append(ch.sendq, f)
				vm.wakeWaiters(ch) // a select receive arm can now proceed
				return false, nil
			}
		case OpRecv:
			chv := f.peek(0) // keep the channel on the stack in case we park
			ch, ok := asChannelV(chv)
			if !ok {
				f.pop()
				return false, rtErr("cannot receive from %s (need a channel)", typeOf(chv))
			}
			if v, ready := vm.chanTryRecv(ch); ready {
				f.pop()
				f.push(v)
				vm.wakeWaiters(ch)
			} else if ch.closed {
				f.pop()
				return false, rtErr("receive on closed channel")
			} else {
				f.status = FBlockedRecv
				ch.recvq = append(ch.recvq, f)
				vm.wakeWaiters(ch) // a select send arm can now proceed
				frame.ip = startIP // re-run once woken
				return false, nil
			}
		case OpChanNext:
			chv := f.peek(0)
			ch, ok := asChannelV(chv)
			if !ok {
				f.pop()
				return false, rtErr("cannot iterate %s (need a channel)", typeOf(chv))
			}
			if v, ready := vm.chanTryRecv(ch); ready {
				f.pop()
				f.push(v)
				vm.wakeWaiters(ch)
				frame.ip += 2 // skip the jump operand: fall through into the body
			} else if ch.closed {
				f.pop()
				frame.ip += 2 + int(readU16(chunk.Code, frame.ip)) // exit the loop
			} else {
				f.status = FBlockedRecv
				ch.recvq = append(ch.recvq, f)
				vm.wakeWaiters(ch) // a select send arm can now proceed
				frame.ip = startIP // re-run once woken
				return false, nil
			}
		case OpSelect:
			// on re-entry after a park, drop stale waiter registrations
			if len(f.selectChans) > 0 {
				for _, ch := range f.selectChans {
					removeWaiter(ch, f)
				}
				f.selectChans = f.selectChans[:0]
			}
			p := frame.ip
			nArms := int(chunk.Code[p])
			hasDefault := chunk.Code[p+1] != 0
			p += 2
			type selArm struct {
				send   bool
				target int
			}
			arms := make([]selArm, nArms)
			slots := 0 // operand stack slots consumed by all arms
			for i := 0; i < nArms; i++ {
				kind := chunk.Code[p]
				p++
				dist := int(readU16(chunk.Code, p))
				p += 2
				arms[i] = selArm{send: kind == 1, target: p + dist}
				if kind == 1 {
					slots += 2
				} else {
					slots++
				}
			}
			defaultTarget := -1
			if hasDefault {
				dist := int(readU16(chunk.Code, p))
				p += 2
				defaultTarget = p + dist
			}
			base := f.top - slots
			// resolve each arm's operand positions from the bottom up
			chanPos := make([]int, nArms)
			valPos := make([]int, nArms)
			off := base
			for i := range arms {
				chanPos[i] = off
				if arms[i].send {
					valPos[i] = off + 1
					off += 2
				} else {
					off++
				}
			}
			chanAt := func(pos int) (*OChannel, error) {
				ch, ok := asChannelV(f.stack[pos])
				if !ok {
					return nil, rtErr("select arm needs a channel, got %s", typeOf(f.stack[pos]))
				}
				return ch, nil
			}
			// pick the first ready arm in declaration order
			chosen := -1
			for i := range arms {
				ch, err := chanAt(chanPos[i])
				if err != nil {
					return false, err
				}
				if arms[i].send {
					if chanSendReady(ch) {
						chosen = i
						break
					}
				} else if chanRecvReady(ch) {
					chosen = i
					break
				}
			}
			if chosen >= 0 {
				ch, _ := chanAt(chanPos[chosen])
				if arms[chosen].send {
					if ch.closed {
						return false, rtErr("send on closed channel")
					}
					val := f.stack[valPos[chosen]]
					if len(ch.recvq) > 0 {
						ch.buf = append(ch.buf, val)
						r := ch.recvq[0]
						ch.recvq = ch.recvq[1:]
						vm.schedule(r)
					} else {
						ch.buf = append(ch.buf, val)
					}
					vm.wakeWaiters(ch)
					f.top = base
				} else {
					v, ready := vm.chanTryRecv(ch)
					if !ready { // ready implies closed here
						f.top = base
						return false, rtErr("receive on closed channel")
					}
					vm.wakeWaiters(ch)
					f.top = base
					f.push(v) // left on the stack for a binding arm
				}
				frame.ip = arms[chosen].target
			} else if hasDefault {
				f.top = base
				frame.ip = defaultTarget
			} else {
				// nothing ready: park on every arm's channel and retry when woken
				for i := range arms {
					ch, _ := chanAt(chanPos[i])
					ch.waiters = append(ch.waiters, f)
					f.selectChans = append(f.selectChans, ch)
				}
				f.status = FBlockedSelect
				frame.ip = startIP
				return false, nil
			}
		default:
			return false, rtErr("bad opcode %d (VM bug)", op)
		}
	}
}

// wakeWaiters reschedules every fiber parked in a select on ch, so each
// re-polls its arms; a fiber woken elsewhere first is skipped.
func (vm *VM) wakeWaiters(ch *OChannel) {
	if len(ch.waiters) == 0 {
		return
	}
	for _, w := range ch.waiters {
		if w.status == FBlockedSelect {
			vm.schedule(w)
		}
	}
	ch.waiters = nil
}

func removeWaiter(ch *OChannel, f *Fiber) {
	for i, w := range ch.waiters {
		if w == f {
			ch.waiters = append(ch.waiters[:i], ch.waiters[i+1:]...)
			return
		}
	}
}

func chanRecvReady(ch *OChannel) bool {
	return len(ch.buf) > 0 || len(ch.sendq) > 0 || ch.closed
}

func chanSendReady(ch *OChannel) bool {
	return ch.closed || len(ch.recvq) > 0 || len(ch.buf) < ch.cap
}

func asChannelV(v Value) (*OChannel, bool) {
	if v.T == VObj {
		ch, ok := v.O.(*OChannel)
		return ch, ok
	}
	return nil, false
}

// chanTryRecv performs a non-blocking receive, waking one blocked sender to
// refill the slot it drains. ready is false when nothing is available yet
// (the caller decides whether to block or observe closure).
func (vm *VM) chanTryRecv(ch *OChannel) (Value, bool) {
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
		return v, true
	}
	if len(ch.sendq) > 0 { // unbuffered rendezvous
		s := ch.sendq[0]
		ch.sendq = ch.sendq[1:]
		v := s.sendVal
		s.sendVal = Unit
		vm.schedule(s)
		return v, true
	}
	return Unit, false
}

func popChannel(f *Fiber, what string, sp Span) (*OChannel, error) {
	v := f.pop()
	if v.T == VObj {
		if ch, ok := v.O.(*OChannel); ok {
			return ch, nil
		}
	}
	return nil, &runtimeErr{msg: fmt.Sprintf("cannot %s %s (need a channel)", what, typeOf(v)), span: sp}
}

func (vm *VM) callValue(f *Fiber, argc int, sp Span) error {
	callee := f.peek(argc)
	if callee.T != VObj {
		return &runtimeErr{msg: fmt.Sprintf("cannot call %s", typeOf(callee)), span: sp}
	}
	switch o := callee.O.(type) {
	case *OClosure:
		if argc != o.Fn.Arity {
			return &runtimeErr{msg: fmt.Sprintf("%s expects %d argument(s), got %d", display(callee), o.Fn.Arity, argc), span: sp}
		}
		if len(f.frames) >= 2048 {
			return &runtimeErr{msg: "stack overflow (call depth > 2048)", span: sp}
		}
		f.frames = append(f.frames, Frame{closure: o, ip: 0, base: f.top - argc - 1})
		return nil
	case *ONative:
		if o.Arity >= 0 && argc != o.Arity {
			return &runtimeErr{msg: fmt.Sprintf("%s expects %d argument(s), got %d", o.Name, o.Arity, argc), span: sp}
		}
		args := f.stack[f.top-argc : f.top]
		res, err := o.Fn(vm, args)
		if err != nil {
			if _, ok := err.(*exitRequest); ok {
				return err // exit(): propagate unwrapped for main to os.Exit
			}
			return &runtimeErr{msg: err.Error(), span: sp}
		}
		if vm.parkRecv != nil {
			return nil // leave callee+args on the stack; OpCall parks and retries
		}
		f.top -= argc + 1
		f.push(res)
		return nil
	case *OVariantCtor:
		arity := o.Enum.Variants[o.Idx].Arity
		if argc != arity {
			return &runtimeErr{msg: fmt.Sprintf("%s.%s expects %d field(s), got %d", o.Enum.Name, o.Enum.Variants[o.Idx].Name, arity, argc), span: sp}
		}
		fields := make([]Value, argc)
		copy(fields, f.stack[f.top-argc:f.top])
		inst := &OEnumInst{Enum: o.Enum, Variant: o.Idx, Fields: fields}
		vm.gc.alloc(inst) // args still on stack: fields stay rooted during collection
		f.top -= argc + 1
		f.push(ObjV(inst))
		return nil
	}
	return &runtimeErr{msg: fmt.Sprintf("cannot call %s", typeOf(callee)), span: sp}
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

func (vm *VM) arith(f *Fiber, op Op, sp Span) error {
	b := f.peek(0)
	a := f.peek(1)
	fail := func() error {
		opName := map[Op]string{OpAdd: "+", OpSub: "-", OpMul: "*", OpDiv: "/", OpMod: "%"}[op]
		return &runtimeErr{msg: fmt.Sprintf("cannot apply %q to %s and %s", opName, typeOf(a), typeOf(b)), span: sp}
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
			return &runtimeErr{msg: fmt.Sprintf("integer overflow: %d %s %d", a.I, sym, b.I), span: sp}
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
				return &runtimeErr{msg: "integer division by zero", span: sp}
			}
			if a.I == math.MinInt64 && b.I == -1 {
				return overflow("/")
			}
			f.push(IntV(a.I / b.I))
		case OpMod:
			if b.I == 0 {
				return &runtimeErr{msg: "integer modulo by zero", span: sp}
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

func (vm *VM) compare(f *Fiber, op Op, sp Span) error {
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
		return &runtimeErr{msg: fmt.Sprintf("cannot compare %s and %s", typeOf(a), typeOf(b)), span: sp}
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

func (vm *VM) indexGet(target, idx Value, sp Span) (Value, error) {
	if target.T != VObj {
		return Unit, &runtimeErr{msg: fmt.Sprintf("cannot index %s", typeOf(target)), span: sp}
	}
	switch o := target.O.(type) {
	case *OList:
		if idx.T != VInt {
			return Unit, &runtimeErr{msg: fmt.Sprintf("list index must be an int, got %s", typeOf(idx)), span: sp}
		}
		if idx.I < 0 || idx.I >= int64(len(o.Elems)) {
			return Unit, &runtimeErr{msg: fmt.Sprintf("list index %d out of bounds (len %d)", idx.I, len(o.Elems)), span: sp}
		}
		return o.Elems[idx.I], nil
	case *OString:
		if idx.T != VInt {
			return Unit, &runtimeErr{msg: fmt.Sprintf("string index must be an int, got %s", typeOf(idx)), span: sp}
		}
		if idx.I < 0 || idx.I >= int64(len(o.S)) {
			return Unit, &runtimeErr{msg: fmt.Sprintf("string index %d out of bounds (len %d)", idx.I, len(o.S)), span: sp}
		}
		return ObjV(vm.gc.newString(string(o.S[idx.I]))), nil
	}
	return Unit, &runtimeErr{msg: fmt.Sprintf("cannot index %s", typeOf(target)), span: sp}
}

func lengthOf(v Value, sp Span) (int64, error) {
	if v.T == VObj {
		switch o := v.O.(type) {
		case *OList:
			return int64(len(o.Elems)), nil
		case *OString:
			return int64(len(o.S)), nil
		}
	}
	return 0, &runtimeErr{msg: fmt.Sprintf("len() needs a list or string, got %s", typeOf(v)), span: sp}
}
