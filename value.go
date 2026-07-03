package main

import (
	"fmt"
	"strconv"
	"strings"
)

type ValType uint8

const (
	VUnit ValType = iota // zero value of Value is Unit
	VBool
	VInt
	VFloat
	VObj
)

type Value struct {
	T ValType
	B bool
	I int64
	F float64
	O Obj
}

var Unit = Value{T: VUnit}

func BoolV(b bool) Value     { return Value{T: VBool, B: b} }
func IntV(i int64) Value     { return Value{T: VInt, I: i} }
func FloatV(f float64) Value { return Value{T: VFloat, F: f} }
func ObjV(o Obj) Value       { return Value{T: VObj, O: o} }

// ---- heap objects ----

type GCHeader struct {
	marked bool
	next   Obj
}

type Obj interface {
	hdr() *GCHeader
	typeName() string
}

type OString struct {
	GCHeader
	S string
}

type OList struct {
	GCHeader
	Elems []Value
}

// compiled function (created at compile time, lives in chunk constants)
type OFunc struct {
	GCHeader
	Name      string
	Arity     int
	NumUpvals int
	Chunk     Chunk
}

type OClosure struct {
	GCHeader
	Fn     *OFunc
	Upvals []*OUpvalue
}

// an upvalue references a stack slot of its origin fiber while open,
// and owns the value once closed
type OUpvalue struct {
	GCHeader
	fiber  *Fiber
	slot   int
	open   bool
	Closed Value
}

func (u *OUpvalue) get() Value {
	if u.open {
		return u.fiber.stack[u.slot]
	}
	return u.Closed
}

func (u *OUpvalue) set(v Value) {
	if u.open {
		u.fiber.stack[u.slot] = v
	} else {
		u.Closed = v
	}
}

type VariantInfo struct {
	Name  string
	Arity int
}

type OEnumType struct {
	GCHeader
	Name     string
	Variants []VariantInfo
}

type OVariantCtor struct {
	GCHeader
	Enum *OEnumType
	Idx  int
}

type OEnumInst struct {
	GCHeader
	Enum    *OEnumType
	Variant int
	Fields  []Value
}

type OChannel struct {
	GCHeader
	cap   int
	buf   []Value
	sendq []*Fiber // fibers blocked sending (value in fiber.sendVal)
	recvq []*Fiber // fibers blocked receiving
}

type NativeFn func(vm *VM, args []Value) (Value, error)

type ONative struct {
	GCHeader
	Name  string
	Arity int // -1 = variadic
	Fn    NativeFn
}

func (o *OString) hdr() *GCHeader      { return &o.GCHeader }
func (o *OList) hdr() *GCHeader        { return &o.GCHeader }
func (o *OFunc) hdr() *GCHeader        { return &o.GCHeader }
func (o *OClosure) hdr() *GCHeader     { return &o.GCHeader }
func (o *OUpvalue) hdr() *GCHeader     { return &o.GCHeader }
func (o *OEnumType) hdr() *GCHeader    { return &o.GCHeader }
func (o *OVariantCtor) hdr() *GCHeader { return &o.GCHeader }
func (o *OEnumInst) hdr() *GCHeader    { return &o.GCHeader }
func (o *OChannel) hdr() *GCHeader     { return &o.GCHeader }
func (o *ONative) hdr() *GCHeader      { return &o.GCHeader }

func (o *OString) typeName() string      { return "string" }
func (o *OList) typeName() string        { return "list" }
func (o *OFunc) typeName() string        { return "function" }
func (o *OClosure) typeName() string     { return "function" }
func (o *OUpvalue) typeName() string     { return "upvalue" }
func (o *OEnumType) typeName() string    { return "enum" }
func (o *OVariantCtor) typeName() string { return "variant constructor" }
func (o *OEnumInst) typeName() string    { return o.Enum.Name }
func (o *OChannel) typeName() string     { return "channel" }
func (o *ONative) typeName() string      { return "native function" }

func typeOf(v Value) string {
	switch v.T {
	case VUnit:
		return "unit"
	case VBool:
		return "bool"
	case VInt:
		return "int"
	case VFloat:
		return "float"
	case VObj:
		return v.O.typeName()
	}
	return "?"
}

// ---- equality (deep for lists and enum instances) ----

func valuesEqual(a, b Value) bool {
	if a.T == VInt && b.T == VFloat {
		return float64(a.I) == b.F
	}
	if a.T == VFloat && b.T == VInt {
		return a.F == float64(b.I)
	}
	if a.T != b.T {
		return false
	}
	switch a.T {
	case VUnit:
		return true
	case VBool:
		return a.B == b.B
	case VInt:
		return a.I == b.I
	case VFloat:
		return a.F == b.F
	case VObj:
		return objEqual(a.O, b.O)
	}
	return false
}

func objEqual(a, b Obj) bool {
	switch x := a.(type) {
	case *OString:
		y, ok := b.(*OString)
		return ok && x.S == y.S
	case *OList:
		y, ok := b.(*OList)
		if !ok || len(x.Elems) != len(y.Elems) {
			return false
		}
		for i := range x.Elems {
			if !valuesEqual(x.Elems[i], y.Elems[i]) {
				return false
			}
		}
		return true
	case *OEnumInst:
		y, ok := b.(*OEnumInst)
		if !ok || x.Enum != y.Enum || x.Variant != y.Variant {
			return false
		}
		for i := range x.Fields {
			if !valuesEqual(x.Fields[i], y.Fields[i]) {
				return false
			}
		}
		return true
	}
	return a == b // reference equality for functions, channels, etc.
}

// ---- display ----

// display: how print() shows a value (strings unquoted)
func display(v Value) string { return format(v, false) }

// repr: how values look inside containers (strings quoted)
func repr(v Value) string { return format(v, true) }

func format(v Value, quote bool) string {
	switch v.T {
	case VUnit:
		return "()"
	case VBool:
		return strconv.FormatBool(v.B)
	case VInt:
		return strconv.FormatInt(v.I, 10)
	case VFloat:
		s := strconv.FormatFloat(v.F, 'g', -1, 64)
		if !strings.ContainsAny(s, ".eE") && !strings.Contains(s, "Inf") && !strings.Contains(s, "NaN") {
			s += ".0"
		}
		return s
	case VObj:
		switch o := v.O.(type) {
		case *OString:
			if quote {
				return strconv.Quote(o.S)
			}
			return o.S
		case *OList:
			var b strings.Builder
			b.WriteByte('[')
			for i, e := range o.Elems {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(repr(e))
			}
			b.WriteByte(']')
			return b.String()
		case *OFunc:
			return "<fn " + o.Name + ">"
		case *OClosure:
			name := o.Fn.Name
			if name == "" {
				name = "anonymous"
			}
			return "<fn " + name + ">"
		case *ONative:
			return "<native " + o.Name + ">"
		case *OEnumType:
			return "<enum " + o.Name + ">"
		case *OVariantCtor:
			return "<variant " + o.Enum.Name + "." + o.Enum.Variants[o.Idx].Name + ">"
		case *OEnumInst:
			name := o.Enum.Variants[o.Variant].Name
			if len(o.Fields) == 0 {
				return name
			}
			var b strings.Builder
			b.WriteString(name)
			b.WriteByte('(')
			for i, f := range o.Fields {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(repr(f))
			}
			b.WriteByte(')')
			return b.String()
		case *OChannel:
			return fmt.Sprintf("<chan cap=%d len=%d>", o.cap, len(o.buf))
		}
	}
	return "?"
}
