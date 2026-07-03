package main

import (
	"fmt"
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
	{"len", 1, func(vm *VM, args []Value) (Value, error) {
		n, err := lengthOf(args[0], Span{})
		if err != nil {
			return Unit, fmt.Errorf("len() needs a list or string, got %s", typeOf(args[0]))
		}
		return IntV(n), nil
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
