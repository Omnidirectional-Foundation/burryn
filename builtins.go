package main

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type nativeDef struct {
	name  string
	arity int // -1 variadic
	fn    NativeFn
}

var nativeDefs = []nativeDef{
	{"print", -1, func(vm *VM, args []Value) (Value, error) {
		fmt.Fprint(vm.out, joinDisplay(args))
		return Unit, nil
	}},
	{"println", -1, func(vm *VM, args []Value) (Value, error) {
		fmt.Fprintln(vm.out, joinDisplay(args))
		return Unit, nil
	}},
	{"eprintln", -1, func(vm *VM, args []Value) (Value, error) {
		fmt.Fprintln(os.Stderr, joinDisplay(args))
		return Unit, nil
	}},
	{"len", 1, func(vm *VM, args []Value) (Value, error) {
		n, err := lengthOf(args[0], Span{})
		if err != nil {
			return Unit, fmt.Errorf("len() needs a list, string, or map, got %s", typeOf(args[0]))
		}
		return IntV(n), nil
	}},
	{"map", 0, func(vm *VM, args []Value) (Value, error) {
		m := &OMap{index: map[mapKey]int{}}
		vm.gc.alloc(m)
		return ObjV(m), nil
	}},
	{"get", 2, func(vm *VM, args []Value) (Value, error) {
		m, ok := asMap(args[0])
		if !ok {
			return Unit, fmt.Errorf("get() needs a map, got %s", typeOf(args[0]))
		}
		k, ok := toMapKey(args[1])
		if !ok {
			return Unit, fmt.Errorf("map keys must be int or str, got %s", typeOf(args[1]))
		}
		if v, found := m.get(k); found {
			return vm.some(v), nil
		}
		return vm.none(), nil
	}},
	{"put", 3, func(vm *VM, args []Value) (Value, error) {
		m, ok := asMap(args[0])
		if !ok {
			return Unit, fmt.Errorf("put() needs a map, got %s", typeOf(args[0]))
		}
		k, ok := toMapKey(args[1])
		if !ok {
			return Unit, fmt.Errorf("map keys must be int or str, got %s", typeOf(args[1]))
		}
		m.set(k, args[1], args[2])
		return Unit, nil
	}},
	{"delete", 2, func(vm *VM, args []Value) (Value, error) {
		m, ok := asMap(args[0])
		if !ok {
			return Unit, fmt.Errorf("delete() needs a map, got %s", typeOf(args[0]))
		}
		k, ok := toMapKey(args[1])
		if !ok {
			return Unit, fmt.Errorf("map keys must be int or str, got %s", typeOf(args[1]))
		}
		m.del(k)
		return Unit, nil
	}},
	{"keys", 1, func(vm *VM, args []Value) (Value, error) {
		m, ok := asMap(args[0])
		if !ok {
			return Unit, fmt.Errorf("keys() needs a map, got %s", typeOf(args[0]))
		}
		elems := make([]Value, len(m.entries))
		for i, e := range m.entries {
			elems[i] = e.key
		}
		return ObjV(vm.gc.newList(elems)), nil
	}},
	{"push", 2, func(vm *VM, args []Value) (Value, error) {
		lst, ok := asList(args[0])
		if !ok {
			return Unit, fmt.Errorf("push() needs a list, got %s", typeOf(args[0]))
		}
		lst.Elems = append(lst.Elems, args[1])
		return Unit, nil
	}},
	{"pop", 1, func(vm *VM, args []Value) (Value, error) {
		lst, ok := asList(args[0])
		if !ok {
			return Unit, fmt.Errorf("pop() needs a list, got %s", typeOf(args[0]))
		}
		if len(lst.Elems) == 0 {
			return Unit, fmt.Errorf("pop() on empty list")
		}
		v := lst.Elems[len(lst.Elems)-1]
		lst.Elems = lst.Elems[:len(lst.Elems)-1]
		return v, nil
	}},
	{"str", 1, func(vm *VM, args []Value) (Value, error) {
		return ObjV(vm.gc.newString(display(args[0]))), nil
	}},
	{"trunc", 1, func(vm *VM, args []Value) (Value, error) {
		if args[0].T != VFloat {
			return Unit, fmt.Errorf("trunc() needs a float, got %s", typeOf(args[0]))
		}
		return IntV(int64(args[0].F)), nil
	}},
	{"to_float", 1, func(vm *VM, args []Value) (Value, error) {
		if args[0].T != VInt {
			return Unit, fmt.Errorf("to_float() needs an int, got %s", typeOf(args[0]))
		}
		return FloatV(float64(args[0].I)), nil
	}},
	{"float_bits", 1, func(vm *VM, args []Value) (Value, error) {
		if args[0].T != VFloat {
			return Unit, fmt.Errorf("float_bits() needs a float, got %s", typeOf(args[0]))
		}
		return IntV(int64(math.Float64bits(args[0].F))), nil
	}},
	{"parse_int", 1, func(vm *VM, args []Value) (Value, error) {
		s, ok := asString(args[0])
		if !ok {
			return Unit, fmt.Errorf("parse_int() needs a str, got %s", typeOf(args[0]))
		}
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			return vm.none(), nil
		}
		return vm.some(IntV(n)), nil
	}},
	{"parse_float", 1, func(vm *VM, args []Value) (Value, error) {
		s, ok := asString(args[0])
		if !ok {
			return Unit, fmt.Errorf("parse_float() needs a str, got %s", typeOf(args[0]))
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return vm.none(), nil
		}
		return vm.some(FloatV(f)), nil
	}},
	{"str_len", 1, func(vm *VM, args []Value) (Value, error) {
		s, ok := asString(args[0])
		if !ok {
			return Unit, fmt.Errorf("str_len() needs a str, got %s", typeOf(args[0]))
		}
		return IntV(int64(len(s))), nil
	}},
	{"char_at", 2, func(vm *VM, args []Value) (Value, error) {
		s, ok := asString(args[0])
		if !ok || args[1].T != VInt {
			return Unit, fmt.Errorf("char_at() needs (str, int)")
		}
		i := args[1].I
		if i < 0 || i >= int64(len(s)) {
			return Unit, fmt.Errorf("char_at index %d out of bounds (len %d)", i, len(s))
		}
		return ObjV(vm.gc.newString(string(s[i]))), nil
	}},
	{"range", 2, func(vm *VM, args []Value) (Value, error) {
		if args[0].T != VInt || args[1].T != VInt {
			return Unit, fmt.Errorf("range() needs two ints")
		}
		a, b := args[0].I, args[1].I
		var elems []Value
		for i := a; i < b; i++ {
			elems = append(elems, IntV(i))
		}
		return ObjV(vm.gc.newList(elems)), nil
	}},
	{"split", 2, func(vm *VM, args []Value) (Value, error) {
		s, ok := asString(args[0])
		sep, ok2 := asString(args[1])
		if !ok || !ok2 {
			return Unit, fmt.Errorf("split() needs (str, str)")
		}
		parts := strings.Split(s, sep)
		// root the list first, then grow it: each newString may trigger a
		// collection, and only the list keeps the earlier strings reachable
		lst := vm.gc.newList(make([]Value, 0, len(parts)))
		f := vm.current
		f.push(ObjV(lst))
		for _, p := range parts {
			lst.Elems = append(lst.Elems, ObjV(vm.gc.newString(p)))
		}
		f.pop()
		return ObjV(lst), nil
	}},
	{"join", 2, func(vm *VM, args []Value) (Value, error) {
		lst, ok := asList(args[0])
		sep, ok2 := asString(args[1])
		if !ok || !ok2 {
			return Unit, fmt.Errorf("join() needs ([str], str)")
		}
		parts := make([]string, len(lst.Elems))
		for i, e := range lst.Elems {
			p, ok := asString(e)
			if !ok {
				return Unit, fmt.Errorf("join() needs a list of str, got %s element", typeOf(e))
			}
			parts[i] = p
		}
		return ObjV(vm.gc.newString(strings.Join(parts, sep))), nil
	}},
	{"substr", 3, func(vm *VM, args []Value) (Value, error) {
		s, ok := asString(args[0])
		if !ok || args[1].T != VInt || args[2].T != VInt {
			return Unit, fmt.Errorf("substr() needs (str, int, int)")
		}
		start, n := args[1].I, args[2].I
		if start < 0 || n < 0 || start+n > int64(len(s)) {
			return Unit, fmt.Errorf("substr(%d, %d) out of bounds (len %d)", start, n, len(s))
		}
		return ObjV(vm.gc.newString(s[start : start+n])), nil
	}},
	{"str_contains", 2, func(vm *VM, args []Value) (Value, error) {
		s, ok := asString(args[0])
		sub, ok2 := asString(args[1])
		if !ok || !ok2 {
			return Unit, fmt.Errorf("str_contains() needs (str, str)")
		}
		return BoolV(strings.Contains(s, sub)), nil
	}},
	{"str_index_of", 2, func(vm *VM, args []Value) (Value, error) {
		s, ok := asString(args[0])
		sub, ok2 := asString(args[1])
		if !ok || !ok2 {
			return Unit, fmt.Errorf("str_index_of() needs (str, str)")
		}
		if i := strings.Index(s, sub); i >= 0 {
			return vm.some(IntV(int64(i))), nil
		}
		return vm.none(), nil
	}},
	{"trim", 1, func(vm *VM, args []Value) (Value, error) {
		s, ok := asString(args[0])
		if !ok {
			return Unit, fmt.Errorf("trim() needs a str, got %s", typeOf(args[0]))
		}
		return ObjV(vm.gc.newString(strings.TrimSpace(s))), nil
	}},
	{"slice", 3, func(vm *VM, args []Value) (Value, error) {
		lst, ok := asList(args[0])
		if !ok || args[1].T != VInt || args[2].T != VInt {
			return Unit, fmt.Errorf("slice() needs ([a], int, int)")
		}
		start, end := args[1].I, args[2].I
		if start < 0 || end < start || end > int64(len(lst.Elems)) {
			return Unit, fmt.Errorf("slice(%d, %d) out of bounds (len %d)", start, end, len(lst.Elems))
		}
		elems := make([]Value, end-start)
		copy(elems, lst.Elems[start:end])
		return ObjV(vm.gc.newList(elems)), nil
	}},
	{"concat", 2, func(vm *VM, args []Value) (Value, error) {
		xs, ok := asList(args[0])
		ys, ok2 := asList(args[1])
		if !ok || !ok2 {
			return Unit, fmt.Errorf("concat() needs ([a], [a])")
		}
		elems := make([]Value, 0, len(xs.Elems)+len(ys.Elems))
		elems = append(elems, xs.Elems...)
		elems = append(elems, ys.Elems...)
		return ObjV(vm.gc.newList(elems)), nil
	}},
	{"contains", 2, func(vm *VM, args []Value) (Value, error) {
		lst, ok := asList(args[0])
		if !ok {
			return Unit, fmt.Errorf("contains() needs a list, got %s", typeOf(args[0]))
		}
		for _, e := range lst.Elems {
			if valuesEqual(e, args[1]) {
				return BoolV(true), nil
			}
		}
		return BoolV(false), nil
	}},
	{"read_file", 1, func(vm *VM, args []Value) (Value, error) {
		path, ok := asString(args[0])
		if !ok {
			return Unit, fmt.Errorf("read_file() needs a str, got %s", typeOf(args[0]))
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return vm.errStr(err.Error()), nil
		}
		return vm.okStr(string(data)), nil
	}},
	{"write_file", 2, func(vm *VM, args []Value) (Value, error) {
		path, ok := asString(args[0])
		contents, ok2 := asString(args[1])
		if !ok || !ok2 {
			return Unit, fmt.Errorf("write_file() needs (str, str)")
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			return vm.errStr(err.Error()), nil
		}
		return vm.ok(Unit), nil
	}},
	{"file_exists", 1, func(vm *VM, args []Value) (Value, error) {
		path, ok := asString(args[0])
		if !ok {
			return Unit, fmt.Errorf("file_exists() needs a str, got %s", typeOf(args[0]))
		}
		_, err := os.Stat(path)
		return BoolV(err == nil), nil
	}},
	{"read_dir", 1, func(vm *VM, args []Value) (Value, error) {
		path, ok := asString(args[0])
		if !ok {
			return Unit, fmt.Errorf("read_dir() needs a str, got %s", typeOf(args[0]))
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return vm.errStr(err.Error()), nil
		}
		// root the list first, then grow it with fresh name strings
		lst := vm.gc.newList(make([]Value, 0, len(entries)))
		f := vm.current
		f.push(ObjV(lst))
		for _, e := range entries {
			lst.Elems = append(lst.Elems, ObjV(vm.gc.newString(e.Name())))
		}
		res := vm.ok(f.peek(0))
		f.pop()
		return res, nil
	}},
	{"exec", 2, func(vm *VM, args []Value) (Value, error) {
		cmd, ok := asString(args[0])
		argList, ok2 := asList(args[1])
		if !ok || !ok2 {
			return Unit, fmt.Errorf("exec() needs (str, [str])")
		}
		cmdArgs := make([]string, len(argList.Elems))
		for i, e := range argList.Elems {
			s, ok := asString(e)
			if !ok {
				return Unit, fmt.Errorf("exec() args must be str, got %s", typeOf(e))
			}
			cmdArgs[i] = s
		}
		c := exec.Command(cmd, cmdArgs...)
		var stdout, stderr bytes.Buffer
		c.Stdout, c.Stderr = &stdout, &stderr
		code := 0
		if runErr := c.Run(); runErr != nil {
			ee, ok := runErr.(*exec.ExitError)
			if !ok { // spawn failure: command not found, not executable, ...
				return vm.errStr(runErr.Error()), nil
			}
			code = ee.ExitCode()
		}
		return vm.output(code, stdout.String(), stderr.String()), nil
	}},
	{"args", 0, func(vm *VM, args []Value) (Value, error) {
		// root the list first, then grow it with fresh argument strings
		lst := vm.gc.newList(make([]Value, 0, len(vm.args)))
		f := vm.current
		f.push(ObjV(lst))
		for _, a := range vm.args {
			lst.Elems = append(lst.Elems, ObjV(vm.gc.newString(a)))
		}
		f.pop()
		return ObjV(lst), nil
	}},
	{"exit", 1, func(vm *VM, args []Value) (Value, error) {
		if args[0].T != VInt {
			return Unit, fmt.Errorf("exit() needs an int, got %s", typeOf(args[0]))
		}
		return Unit, &exitRequest{code: int(args[0].I)}
	}},
	{"chan", -1, func(vm *VM, args []Value) (Value, error) {
		capacity := 0
		if len(args) > 1 {
			return Unit, fmt.Errorf("chan() takes at most one argument")
		}
		if len(args) == 1 {
			if args[0].T != VInt || args[0].I < 0 {
				return Unit, fmt.Errorf("chan() capacity must be a non-negative int")
			}
			capacity = int(args[0].I)
		}
		ch := &OChannel{cap: capacity}
		vm.gc.alloc(ch)
		return ObjV(ch), nil
	}},
	{"close", 1, func(vm *VM, args []Value) (Value, error) {
		ch, ok := asChannelV(args[0])
		if !ok {
			return Unit, fmt.Errorf("close() needs a channel, got %s", typeOf(args[0]))
		}
		if ch.closed {
			return Unit, fmt.Errorf("close of closed channel")
		}
		ch.closed = true
		// wake every blocked receiver: each re-runs its receive, drains any
		// buffered values, then observes closure
		for _, r := range ch.recvq {
			vm.schedule(r)
		}
		ch.recvq = nil
		vm.wakeWaiters(ch) // select arms on this channel are now ready
		return Unit, nil
	}},
	{"recv", 1, func(vm *VM, args []Value) (Value, error) {
		ch, ok := asChannelV(args[0])
		if !ok {
			return Unit, fmt.Errorf("recv() needs a channel, got %s", typeOf(args[0]))
		}
		if v, ready := vm.chanTryRecv(ch); ready {
			f := vm.current
			f.push(v) // root v across the Some allocation
			opt := vm.some(f.peek(0))
			f.pop()
			vm.wakeWaiters(ch)
			return opt, nil
		}
		if ch.closed {
			return vm.none(), nil
		}
		vm.parkRecv = ch // OpCall parks this fiber and retries once woken
		return Unit, nil
	}},
	{"chr", 1, func(vm *VM, args []Value) (Value, error) {
		if args[0].T != VInt || args[0].I < 0 || args[0].I > 0x10ffff {
			return Unit, fmt.Errorf("chr() needs an int code point")
		}
		return ObjV(vm.gc.newString(string(rune(args[0].I)))), nil
	}},
	{"ord", 1, func(vm *VM, args []Value) (Value, error) {
		if args[0].T == VObj {
			if s, ok := args[0].O.(*OString); ok && len(s.S) > 0 {
				return IntV(int64([]rune(s.S)[0])), nil
			}
		}
		return Unit, fmt.Errorf("ord() needs a non-empty string")
	}},
	{"clock", 0, func(vm *VM, args []Value) (Value, error) {
		return FloatV(time.Since(vm.start).Seconds()), nil
	}},
	{"type_of", 1, func(vm *VM, args []Value) (Value, error) {
		return ObjV(vm.gc.newString(typeOf(args[0]))), nil
	}},
	{"assert", 2, func(vm *VM, args []Value) (Value, error) {
		if args[0].T != VBool {
			return Unit, fmt.Errorf("assert() needs a bool, got %s", typeOf(args[0]))
		}
		if !args[0].B {
			return Unit, fmt.Errorf("assertion failed: %s", display(args[1]))
		}
		return Unit, nil
	}},
	{"gc", 0, func(vm *VM, args []Value) (Value, error) {
		return IntV(int64(vm.gc.collect())), nil
	}},
	{"heap_objects", 0, func(vm *VM, args []Value) (Value, error) {
		return IntV(int64(vm.gc.count)), nil
	}},
	{"gc_cycles", 0, func(vm *VM, args []Value) (Value, error) {
		return IntV(int64(vm.gc.cycles)), nil
	}},
}

func nativeNames() []string {
	names := make([]string, len(nativeDefs))
	for i, d := range nativeDefs {
		names[i] = d.name
	}
	// yield is registered specially in registerNatives
	return append(names, "yield")
}

func registerNatives(vm *VM) {
	for _, d := range nativeDefs {
		n := &ONative{Name: d.name, Arity: d.arity, Fn: d.fn}
		vm.gc.alloc(n)
		vm.globals[d.name] = ObjV(n)
	}
	// yield: cooperative handoff — reschedule current fiber at the back
	y := &ONative{Name: "yield", Arity: 0, Fn: func(vm *VM, args []Value) (Value, error) {
		vm.yieldFlag = true
		return Unit, nil
	}}
	vm.gc.alloc(y)
	vm.globals["yield"] = ObjV(y)
}

func joinDisplay(args []Value) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = display(a)
	}
	return strings.Join(parts, " ")
}

func asList(v Value) (*OList, bool) {
	if v.T == VObj {
		l, ok := v.O.(*OList)
		return l, ok
	}
	return nil, false
}

func asMap(v Value) (*OMap, bool) {
	if v.T == VObj {
		m, ok := v.O.(*OMap)
		return m, ok
	}
	return nil, false
}

func asString(v Value) (string, bool) {
	if v.T == VObj {
		if s, ok := v.O.(*OString); ok {
			return s.S, true
		}
	}
	return "", false
}

func (vm *VM) some(v Value) Value {
	inst := &OEnumInst{Enum: vm.optEnum, Variant: 0, Fields: []Value{v}}
	vm.gc.alloc(inst)
	return ObjV(inst)
}

func (vm *VM) none() Value {
	inst := &OEnumInst{Enum: vm.optEnum, Variant: 1}
	vm.gc.alloc(inst)
	return ObjV(inst)
}

func (vm *VM) ok(v Value) Value {
	inst := &OEnumInst{Enum: vm.resEnum, Variant: 0, Fields: []Value{v}}
	vm.gc.alloc(inst)
	return ObjV(inst)
}

func (vm *VM) err(v Value) Value {
	inst := &OEnumInst{Enum: vm.resEnum, Variant: 1, Fields: []Value{v}}
	vm.gc.alloc(inst)
	return ObjV(inst)
}

// okStr builds Ok(s) and errStr builds Err(msg), each rooting the fresh
// string on the fiber stack across the enum allocation.
func (vm *VM) okStr(s string) Value {
	f := vm.current
	f.push(ObjV(vm.gc.newString(s)))
	res := vm.ok(f.peek(0))
	f.pop()
	return res
}

func (vm *VM) errStr(msg string) Value {
	f := vm.current
	f.push(ObjV(vm.gc.newString(msg)))
	res := vm.err(f.peek(0))
	f.pop()
	return res
}

// output builds Ok(Output(code, stdout, stderr)), keeping each fresh string
// and the enum instance rooted on the fiber stack across every allocation.
func (vm *VM) output(code int, stdout, stderr string) Value {
	f := vm.current
	f.push(ObjV(vm.gc.newString(stdout))) // rooted at peek(1)
	f.push(ObjV(vm.gc.newString(stderr))) // rooted at peek(0)
	out := &OEnumInst{Enum: vm.outEnum, Variant: 0,
		Fields: []Value{IntV(int64(code)), f.peek(1), f.peek(0)}}
	vm.gc.alloc(out)
	f.pop()                      // drop stderr; still reachable via out
	f.stack[f.top-1] = ObjV(out) // replace stdout root with the Output root
	res := vm.ok(f.peek(0))
	f.pop()
	return res
}
