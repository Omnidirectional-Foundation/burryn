package main

// Mark-sweep garbage collector over Burryn's own heap objects.
// All language-level objects are registered in an intrusive linked list at
// allocation time; collection marks everything reachable from the VM roots
// (globals, every fiber's stack/frames/open upvalues/pending sends, and
// chunk constants via reachable functions) and unlinks the rest.
type GC struct {
	head       Obj
	count      int   // live objects (approximate until next sweep)
	threshold  int   // next collection trigger
	totalAlloc int64 // cumulative allocations, for stats
	totalFreed int64
	cycles     int
	vm         *VM // set once the VM exists; nil during compilation
}

func newGC() *GC {
	return &GC{threshold: 256}
}

// alloc registers an object with the collector, triggering a collection
// when the live-object count crosses the current threshold.
func (g *GC) alloc(o Obj) Obj {
	if g.vm != nil && g.count+1 > g.threshold {
		g.collect()
		g.threshold = g.count*2 + 64
	}
	h := o.hdr()
	h.next = g.head
	g.head = o
	g.count++
	g.totalAlloc++
	return o
}

func (g *GC) newString(s string) *OString {
	o := &OString{S: s}
	g.alloc(o)
	return o
}

func (g *GC) newList(elems []Value) *OList {
	o := &OList{Elems: elems}
	g.alloc(o)
	return o
}

func (g *GC) collect() int {
	if g.vm == nil {
		return 0
	}
	g.cycles++
	var gray []Obj

	markValue := func(v Value) {
		if v.T == VObj && v.O != nil && !v.O.hdr().marked {
			v.O.hdr().marked = true
			gray = append(gray, v.O)
		}
	}
	markObj := func(o Obj) {
		if o != nil && !o.hdr().marked {
			o.hdr().marked = true
			gray = append(gray, o)
		}
	}

	// roots
	vm := g.vm
	for _, v := range vm.globals {
		markValue(v)
	}
	for _, f := range vm.fibers {
		for i := 0; i < f.top; i++ {
			markValue(f.stack[i])
		}
		for i := range f.frames {
			markObj(f.frames[i].closure)
		}
		for _, uv := range f.openUpvals {
			markObj(uv)
		}
		markValue(f.sendVal)
	}

	// trace
	for len(gray) > 0 {
		o := gray[len(gray)-1]
		gray = gray[:len(gray)-1]
		switch t := o.(type) {
		case *OList:
			for _, e := range t.Elems {
				markValue(e)
			}
		case *OFunc:
			for _, c := range t.Chunk.Consts {
				markValue(c)
			}
		case *OClosure:
			markObj(t.Fn)
			for _, uv := range t.Upvals {
				markObj(uv)
			}
		case *OUpvalue:
			if !t.open {
				markValue(t.Closed)
			}
		case *OVariantCtor:
			markObj(t.Enum)
		case *OEnumInst:
			markObj(t.Enum)
			for _, f := range t.Fields {
				markValue(f)
			}
		case *OChannel:
			for _, v := range t.buf {
				markValue(v)
			}
			// blocked senders' pending values are marked via fiber roots
		}
	}

	// sweep
	freed := 0
	var prev Obj
	o := g.head
	for o != nil {
		h := o.hdr()
		next := h.next
		if h.marked {
			h.marked = false
			prev = o
		} else {
			if prev == nil {
				g.head = next
			} else {
				prev.hdr().next = next
			}
			h.next = nil
			freed++
		}
		o = next
	}
	g.count -= freed
	g.totalFreed += int64(freed)
	return freed
}
