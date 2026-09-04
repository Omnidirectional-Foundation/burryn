// Burryn C runtime — multi-file implementation unit.
//
// Definitions previously kept static in burrt.h: all mutable runtime
// globals plus the sequential core, GC, and scheduler functions.
// Cross-unit callers reach them via the extern declarations in burrt.h.
#include "burrt.h"

Fiber *bur_cur;                 // the running fiber
Fiber *bur_main_fiber;          // program ends (Go semantics) when this returns
BurFiberCtx bur_sched_ctx;      // the scheduler's own context (ucontext / Win32 fiber)

Fiber **bur_fibers;             // every fiber ever created, for GC root scanning
int64_t bur_nfibers, bur_fiberscap;
Fiber **bur_ready;              // FIFO ready queue
int64_t bur_ready_head, bur_ready_len, bur_ready_cap;
int bur_next_fiber_id;
int64_t bur_ntimers;            // fibers currently parked on a timer
int64_t bur_nio;                // fibers currently parked on process IO
bool bur_deterministic;         // BUR_DETERMINISTIC=1: serialize IO

#define BUR_TIMESLICE 10000            // instructions per fiber turn (matches vm.go)
#define BUR_STACK_SIZE (1 << 20)       // 1 MiB per spawned fiber
#define BUR_MAIN_STACK_SIZE (8 << 20)  // 8 MiB for the main fiber (deep recursion)

// ---- runtime state ----------------------------------------------------

Obj *bur_gc_head;
int64_t bur_gc_count, bur_gc_threshold = 256, bur_gc_cycles, bur_gc_last_freed;
bool bur_gc_ready; // collection disabled until boot completes

// the closure currently executing; generated functions snapshot this at
// entry to reach their upvalues (bur_call saves/restores it around calls)
OClosure *bur_cur_closure;

OEnumType *bur_opt_enum, *bur_res_enum, *bur_out_enum;
struct timespec bur_start_time;
int64_t bur_start_ns;
int bur_argc;
char **bur_argv;

// ---- traps ------------------------------------------------------------

// BUR_LN: generated code records the live source line of the running
// frame before each instruction; bur_trap walks these for the trace.
#define BUR_LN(n) (bur_cur->trace_ln[bur_cur->call_depth] = (n))

void bur_trap(const char *fmt, ...) {
    fflush(stdout);
    va_list ap;
    va_start(ap, fmt);
    fputs("runtime error: ", stderr);
    vfprintf(stderr, fmt, ap);
    fputc('\n', stderr);
    va_end(ap);
    if (bur_cur && bur_cur->trace_fn) {
        for (int d = bur_cur->call_depth; d >= 0; d--) {
            if (d >= bur_cur->trace_cap) continue;
            OFunc *fn = bur_cur->trace_fn[d];
            if (!fn || !fn->file) continue; // synthetic glue has no source
            fprintf(stderr, "  at %s (%s:%d)\n",
                    fn->name && fn->name[0] ? fn->name : "<fn>",
                    fn->file, bur_cur->trace_ln[d]);
        }
    }
    exit(4);
}

// ---- forward decls ----------------------------------------------------

void bur_gc_collect(void);
void bur_format(Buf *b, Value v, bool quote);

// ---- dynamic byte buffer ----------------------------------------------


void buf_reserve(Buf *b, int64_t extra) {
    if (b->len + extra <= b->cap) return;
    int64_t nc = b->cap * 2 + 64;
    while (nc < b->len + extra) nc *= 2;
    b->data = (char *)realloc(b->data, (size_t)nc);
    b->cap = nc;
}
void buf_bytes(Buf *b, const char *s, int64_t n) {
    buf_reserve(b, n);
    memcpy(b->data + b->len, s, (size_t)n);
    b->len += n;
}
void buf_str(Buf *b, const char *s) { buf_bytes(b, s, (int64_t)strlen(s)); }
void buf_char(Buf *b, char c) { buf_reserve(b, 1); b->data[b->len++] = c; }
void buf_free(Buf *b) { free(b->data); b->data = NULL; b->len = b->cap = 0; }

// ---- allocation + GC --------------------------------------------------

Obj *bur_alloc(size_t size, ObjType type) {
    if (bur_gc_ready && bur_gc_count + 1 > bur_gc_threshold) {
        bur_gc_collect();
        bur_gc_threshold = bur_gc_count + 128;
    }
    Obj *o = (Obj *)calloc(1, size);
    o->type = type;
    o->marked = false;
    o->next = bur_gc_head;
    bur_gc_head = o;
    bur_gc_count++;
    return o;
}

OString *bur_new_string_n(const char *s, int64_t n) {
    OString *o = (OString *)bur_alloc(sizeof(OString), OBJ_STRING);
    o->data = (char *)malloc((size_t)n + 1);
    memcpy(o->data, s, (size_t)n);
    o->data[n] = '\0';
    o->len = n;
    return o;
}
OString *bur_new_string(const char *s) { return bur_new_string_n(s, (int64_t)strlen(s)); }

OList *bur_new_list(Value *elems, int64_t n) {
    OList *o = (OList *)bur_alloc(sizeof(OList), OBJ_LIST);
    o->len = o->cap = n;
    if (n > 0) {
        o->elems = (Value *)malloc(sizeof(Value) * (size_t)n);
        memcpy(o->elems, elems, sizeof(Value) * (size_t)n);
    }
    return o;
}

void list_push(OList *l, Value v) {
    if (l->len == l->cap) {
        l->cap = l->cap * 2 + 8;
        l->elems = (Value *)realloc(l->elems, sizeof(Value) * (size_t)l->cap);
    }
    l->elems[l->len++] = v;
}

OEnumInst *bur_new_inst(OEnumType *e, int variant, Value *fields, int nfields) {
    OEnumInst *o = (OEnumInst *)bur_alloc(sizeof(OEnumInst), OBJ_ENUMINST);
    o->enm = e;
    o->variant = variant;
    o->nfields = nfields;
    if (nfields > 0) {
        o->fields = (Value *)malloc(sizeof(Value) * (size_t)nfields);
        memcpy(o->fields, fields, sizeof(Value) * (size_t)nfields);
    }
    return o;
}

ORecord *bur_new_record(OString **names, Value *fields, int nfields) {
    ORecord *o = (ORecord *)bur_alloc(sizeof(ORecord), OBJ_RECORD);
    o->nfields = nfields;
    if (nfields > 0) {
        o->names = (OString **)malloc(sizeof(OString *) * (size_t)nfields);
        memcpy(o->names, names, sizeof(OString *) * (size_t)nfields);
        o->fields = (Value *)malloc(sizeof(Value) * (size_t)nfields);
        memcpy(o->fields, fields, sizeof(Value) * (size_t)nfields);
    }
    return o;
}

OTuple *bur_new_tuple(Value *elems, int64_t n) {
    OTuple *o = (OTuple *)bur_alloc(sizeof(OTuple), OBJ_TUPLE);
    o->n = n;
    if (n > 0) {
        o->elems = (Value *)malloc(sizeof(Value) * (size_t)n);
        memcpy(o->elems, elems, sizeof(Value) * (size_t)n);
    }
    return o;
}

void bur_push(Value v);
Value bur_pop(void);
void bur_trap(const char *fmt, ...);

void bur_record_make(int n, const char *names_enc) {
    int base = bur_cur->top - n;
    const char *p = names_enc;
    for (int i = 0; i < n; i++) {
        const char *end = strchr(p, '\n');
        int64_t len = end ? (int64_t)(end - p) : (int64_t)strlen(p);
        OString *s = bur_new_string_n(p, len);
        bur_push(bur_obj((Obj *)s));
        p = end ? end + 1 : p + len;
    }
    OString **names = (OString **)malloc(sizeof(OString *) * (size_t)n);
    Value *fields = (Value *)malloc(sizeof(Value) * (size_t)n);
    for (int i = 0; i < n; i++) {
        names[i] = (OString *)bur_cur->stack[base + n + i].u.o;
        fields[i] = bur_cur->stack[base + i];
    }
    ORecord *r = bur_new_record(names, fields, n);
    free(names);
    free(fields);
    bur_cur->top = base;
    bur_push(bur_obj((Obj *)r));
}

void bur_tuple_make(int n) {
    Value *elems = (Value *)malloc(sizeof(Value) * (size_t)n);
    for (int i = 0; i < n; i++) elems[i] = bur_cur->stack[bur_cur->top - n + i];
    OTuple *t = bur_new_tuple(elems, n);
    free(elems);
    bur_cur->top -= n;
    bur_push(bur_obj((Obj *)t));
}

void bur_record_get(const char *fname) {
    Value rv = bur_pop();
    if (rv.t != VOBJ || rv.u.o->type != OBJ_RECORD)
        bur_trap("field access needs a record");
    ORecord *r = (ORecord *)rv.u.o;
    int64_t flen = (int64_t)strlen(fname);
    for (int i = 0; i < r->nfields; i++) {
        if (r->names[i]->len == flen && memcmp(r->names[i]->data, fname, (size_t)flen) == 0) {
            bur_push(r->fields[i]);
            return;
        }
    }
    bur_trap("record has no field \"%s\"", fname);
}

void bur_record_update(int n, const char *names_enc) {
    int top0 = bur_cur->top;
    Value *new_vals = (Value *)malloc(sizeof(Value) * (size_t)n);
    for (int i = 0; i < n; i++) new_vals[i] = bur_cur->stack[top0 - n + i];
    bur_cur->top -= n;
    Value base_v = bur_pop();
    if (base_v.t != VOBJ || base_v.u.o->type != OBJ_RECORD)
        bur_trap("record update needs a record");
    bur_push(base_v);
    for (int i = 0; i < n; i++) bur_push(new_vals[i]);
    const char *p = names_enc;
    for (int i = 0; i < n; i++) {
        const char *end = strchr(p, '\n');
        int64_t len = end ? (int64_t)(end - p) : (int64_t)strlen(p);
        OString *s = bur_new_string_n(p, len);
        bur_push(bur_obj((Obj *)s));
        p = end ? end + 1 : p + len;
    }
    int upd_base = bur_cur->top - n;
    int vals_base = upd_base - n;
    int base_idx = vals_base - 1;
    ORecord *base = (ORecord *)bur_cur->stack[base_idx].u.o;
    OString **upd_names = (OString **)malloc(sizeof(OString *) * (size_t)n);
    for (int i = 0; i < n; i++) upd_names[i] = (OString *)bur_cur->stack[upd_base + i].u.o;
    for (int i = 0; i < n; i++) new_vals[i] = bur_cur->stack[vals_base + i];
    OString **rnames = (OString **)malloc(sizeof(OString *) * (size_t)base->nfields);
    Value *rvals = (Value *)malloc(sizeof(Value) * (size_t)base->nfields);
    for (int i = 0; i < base->nfields; i++) {
        rnames[i] = base->names[i];
        rvals[i] = base->fields[i];
        for (int j = 0; j < n; j++) {
            if (upd_names[j]->len == base->names[i]->len &&
                memcmp(upd_names[j]->data, base->names[i]->data, (size_t)base->names[i]->len) == 0) {
                rvals[i] = new_vals[j];
                break;
            }
        }
    }
    ORecord *r = bur_new_record(rnames, rvals, base->nfields);
    free(new_vals);
    free(upd_names);
    free(rnames);
    free(rvals);
    bur_cur->top = base_idx;
    bur_push(bur_obj((Obj *)r));
}

// stack roots (declared here, used by the collector)
void bur_push(Value v);
Value bur_pop(void);

void bur_mark_value(Value v);

Obj **bur_gray;
int64_t bur_gray_len, bur_gray_cap;

void bur_gray_push(Obj *o) {
    if (o == NULL || o->marked) return;
    o->marked = true;
    if (bur_gray_len == bur_gray_cap) {
        bur_gray_cap = bur_gray_cap * 2 + 64;
        bur_gray = (Obj **)realloc(bur_gray, sizeof(Obj *) * (size_t)bur_gray_cap);
    }
    bur_gray[bur_gray_len++] = o;
}
void bur_mark_value(Value v) {
    if (v.t == VOBJ) bur_gray_push(v.u.o);
}

void bur_gc_trace(Obj *o) {
    switch (o->type) {
    case OBJ_STRING: case OBJ_NATIVE: break;
    case OBJ_LIST: {
        OList *l = (OList *)o;
        for (int64_t i = 0; i < l->len; i++) bur_mark_value(l->elems[i]);
        break;
    }
    case OBJ_MAP: {
        OMap *m = (OMap *)o;
        for (int64_t i = 0; i < m->len; i++) {
            bur_mark_value(m->entries[i].key);
            bur_mark_value(m->entries[i].val);
        }
        break;
    }
    case OBJ_FUNC: {
        OFunc *fn = (OFunc *)o;
        for (int i = 0; i < fn->nconsts; i++) bur_mark_value(fn->consts[i]);
        break;
    }
    case OBJ_CLOSURE: {
        OClosure *c = (OClosure *)o;
        bur_gray_push((Obj *)c->fn);
        for (int i = 0; i < c->nupvals; i++) bur_gray_push((Obj *)c->upvals[i]);
        break;
    }
    case OBJ_UPVALUE: {
        OUpvalue *u = (OUpvalue *)o;
        if (!u->open) bur_mark_value(u->closed);
        break;
    }
    case OBJ_VARIANTCTOR:
        bur_gray_push((Obj *)((OVariantCtor *)o)->enm);
        break;
    case OBJ_ENUMTYPE:
        break;
    case OBJ_ENUMINST: {
        OEnumInst *in = (OEnumInst *)o;
        bur_gray_push((Obj *)in->enm);
        for (int i = 0; i < in->nfields; i++) bur_mark_value(in->fields[i]);
        break;
    }
    case OBJ_CHANNEL: {
        OChannel *ch = (OChannel *)o;
        for (int i = 0; i < ch->buflen; i++) bur_mark_value(ch->buf[i]);
        // blocked senders' pending values are marked via fiber roots
        break;
    }
    case OBJ_RECORD: {
        ORecord *r = (ORecord *)o;
        for (int i = 0; i < r->nfields; i++) {
            bur_gray_push((Obj *)r->names[i]);
            bur_mark_value(r->fields[i]);
        }
        break;
    }
    case OBJ_TUPLE: {
        OTuple *t = (OTuple *)o;
        for (int i = 0; i < t->n; i++) bur_mark_value(t->elems[i]);
        break;
    }
    }
}

// globals table (declared early for GC roots)
GlobalSlot *bur_globals;
int64_t bur_globals_cap, bur_globals_len;

// permanent roots: every compile-time constant object (strings, functions,
// enum types, constructors, singletons) is pinned here so it lives for the
// whole program, mirroring the VM keeping chunk constants reachable.
Obj **bur_roots;
int64_t bur_nroots, bur_rootcap;
void bur_add_root(Obj *o) {
    if (bur_nroots == bur_rootcap) {
        bur_rootcap = bur_rootcap * 2 + 64;
        bur_roots = (Obj **)realloc(bur_roots, sizeof(Obj *) * (size_t)bur_rootcap);
    }
    bur_roots[bur_nroots++] = o;
}

void bur_gc_collect(void) {
    bur_gc_cycles++;
    bur_gray_len = 0;

    // roots: permanent constants, globals, and every fiber's operand stack,
    // open upvalues, and pending send value (mirrors gc.go scanning all fibers)
    for (int64_t i = 0; i < bur_nroots; i++) bur_gray_push(bur_roots[i]);
    for (int64_t i = 0; i < bur_globals_cap; i++)
        if (bur_globals[i].used) bur_mark_value(bur_globals[i].val);
    for (int64_t fi = 0; fi < bur_nfibers; fi++) {
        Fiber *f = bur_fibers[fi];
        for (int i = 0; i < f->top; i++) bur_mark_value(f->stack[i]);
        for (int i = 0; i < f->nopen; i++) bur_gray_push((Obj *)f->openUpvals[i]);
        for (int i = 0; i < f->ndefers; i++) bur_mark_value(f->defers[i]);
        bur_mark_value(f->sendVal);
    }

    while (bur_gray_len > 0) bur_gc_trace(bur_gray[--bur_gray_len]);

    int64_t freed = 0;
    Obj *prev = NULL, *o = bur_gc_head;
    while (o) {
        Obj *next = o->next;
        if (o->marked) {
            o->marked = false;
            prev = o;
        } else {
            freed++;
            if (prev) prev->next = next; else bur_gc_head = next;
            switch (o->type) {
            case OBJ_STRING: free(((OString *)o)->data); break;
            case OBJ_LIST: free(((OList *)o)->elems); break;
            case OBJ_MAP: {
                OMap *m = (OMap *)o;
                for (int64_t i = 0; i < m->len; i++) if (m->entries[i].k.is_str) free(m->entries[i].k.s);
                free(m->entries); free(m->index);
                break;
            }
            case OBJ_CLOSURE: free(((OClosure *)o)->upvals); break;
            case OBJ_ENUMINST: free(((OEnumInst *)o)->fields); break;
            case OBJ_CHANNEL: {
                OChannel *ch = (OChannel *)o;
                free(ch->buf); free(ch->sendq); free(ch->recvq); free(ch->waiters);
                break;
            }
            case OBJ_TUPLE: free(((OTuple *)o)->elems); break;
            case OBJ_RECORD: {
                ORecord *r = (ORecord *)o;
                free(r->names); free(r->fields);
                break;
            }
            default: break;
            }
            free(o);
            bur_gc_count--;
        }
        o = next;
    }
    bur_gc_last_freed = freed;
}

// ---- fiber stack ------------------------------------------------------

void bur_push(Value v) {
    if (bur_cur->top == bur_cur->cap) {
        bur_cur->cap = bur_cur->cap * 2 + 64;
        bur_cur->stack = (Value *)realloc(bur_cur->stack, sizeof(Value) * (size_t)bur_cur->cap);
    }
    bur_cur->stack[bur_cur->top++] = v;
}
Value bur_pop(void) { return bur_cur->stack[--bur_cur->top]; }
Value bur_peek(int n) { return bur_cur->stack[bur_cur->top - 1 - n]; }

// ---- type names -------------------------------------------------------

const char *bur_typename(Value v) {
    switch (v.t) {
    case VUNIT: return "unit";
    case VBOOL: return "bool";
    case VINT: return "int";
    case VFLOAT: return "float";
    case VOBJ:
        switch (v.u.o->type) {
        case OBJ_STRING: return "string";
        case OBJ_LIST: return "list";
        case OBJ_MAP: return "map";
        case OBJ_FUNC: case OBJ_CLOSURE: return "function";
        case OBJ_UPVALUE: return "upvalue";
        case OBJ_ENUMTYPE: return "enum";
        case OBJ_VARIANTCTOR: return "variant constructor";
        case OBJ_ENUMINST: return ((OEnumInst *)v.u.o)->enm->name;
        case OBJ_NATIVE: return "native function";
        case OBJ_CHANNEL: return "channel";
        case OBJ_TUPLE: return "tuple";
        case OBJ_RECORD: return "record";
        }
    }
    return "?";
}

// ---- equality ---------------------------------------------------------

bool bur_obj_eq(Obj *a, Obj *b);

bool bur_eq(Value a, Value b) {
    if (a.t == VINT && b.t == VFLOAT) return (double)a.u.i == b.u.f;
    if (a.t == VFLOAT && b.t == VINT) return a.u.f == (double)b.u.i;
    if (a.t != b.t) return false;
    switch (a.t) {
    case VUNIT: return true;
    case VBOOL: return a.u.b == b.u.b;
    case VINT: return a.u.i == b.u.i;
    case VFLOAT: return a.u.f == b.u.f;
    case VOBJ: return bur_obj_eq(a.u.o, b.u.o);
    }
    return false;
}

bool map_get(OMap *m, MapKey k, Value *out);
bool mapkey_of(Value v, MapKey *out);

bool bur_obj_eq(Obj *a, Obj *b) {
    if (a->type != b->type) return a == b;
    switch (a->type) {
    case OBJ_STRING: {
        OString *x = (OString *)a, *y = (OString *)b;
        return x->len == y->len && memcmp(x->data, y->data, (size_t)x->len) == 0;
    }
    case OBJ_LIST: {
        OList *x = (OList *)a, *y = (OList *)b;
        if (x->len != y->len) return false;
        for (int64_t i = 0; i < x->len; i++)
            if (!bur_eq(x->elems[i], y->elems[i])) return false;
        return true;
    }
    case OBJ_MAP: {
        OMap *x = (OMap *)a, *y = (OMap *)b;
        if (x->len != y->len) return false;
        for (int64_t i = 0; i < x->len; i++) {
            Value yv;
            if (!map_get(y, x->entries[i].k, &yv)) return false;
            if (!bur_eq(x->entries[i].val, yv)) return false;
        }
        return true;
    }
    case OBJ_ENUMINST: {
        OEnumInst *x = (OEnumInst *)a, *y = (OEnumInst *)b;
        if (x->enm != y->enm || x->variant != y->variant) return false;
        for (int i = 0; i < x->nfields; i++)
            if (!bur_eq(x->fields[i], y->fields[i])) return false;
        return true;
    }
    case OBJ_RECORD: {
        ORecord *x = (ORecord *)a, *y = (ORecord *)b;
        if (x->nfields != y->nfields) return false;
        for (int i = 0; i < x->nfields; i++) {
            OString *nx = x->names[i], *ny = y->names[i];
            if (nx->len != ny->len || memcmp(nx->data, ny->data, (size_t)nx->len) != 0) return false;
            if (!bur_eq(x->fields[i], y->fields[i])) return false;
        }
        return true;
    }
    case OBJ_TUPLE: {
        OTuple *x = (OTuple *)a, *y = (OTuple *)b;
        if (x->n != y->n) return false;
        for (int64_t i = 0; i < x->n; i++)
            if (!bur_eq(x->elems[i], y->elems[i])) return false;
        return true;
    }
    default: return a == b;
    }
}

// ---- formatting -------------------------------------------------------

// Shortest round-tripping decimal, matching Go's strconv 'g' shortest for
// the magnitudes the sequential examples exercise.
void bur_format_float(Buf *b, double f) {
    if (isinf(f)) { buf_str(b, f < 0 ? "-Inf" : "+Inf"); return; }
    if (isnan(f)) { buf_str(b, "NaN"); return; }
    char tmp[64];
    for (int prec = 1; prec <= 17; prec++) {
        snprintf(tmp, sizeof tmp, "%.*g", prec, f);
        if (strtod(tmp, NULL) == f) break;
    }
    // Go appends ".0" when the shortest form has no '.', 'e', or 'E'.
    if (!strpbrk(tmp, ".eE")) {
        size_t n = strlen(tmp);
        tmp[n] = '.'; tmp[n + 1] = '0'; tmp[n + 2] = '\0';
    }
    buf_str(b, tmp);
}

void bur_quote(Buf *b, const char *s, int64_t n) {
    buf_char(b, '"');
    for (int64_t i = 0; i < n; i++) {
        unsigned char c = (unsigned char)s[i];
        switch (c) {
        case '"': buf_str(b, "\\\""); break;
        case '\\': buf_str(b, "\\\\"); break;
        case '\n': buf_str(b, "\\n"); break;
        case '\t': buf_str(b, "\\t"); break;
        case '\r': buf_str(b, "\\r"); break;
        default:
            if (c >= 0x20 && c < 0x7f) {
                buf_char(b, (char)c);
            } else {
                char hex[5];
                snprintf(hex, sizeof hex, "\\x%02x", c);
                buf_str(b, hex);
            }
        }
    }
    buf_char(b, '"');
}

void bur_format(Buf *b, Value v, bool quote) {
    char tmp[32];
    switch (v.t) {
    case VUNIT: buf_str(b, "()"); return;
    case VBOOL: buf_str(b, v.u.b ? "true" : "false"); return;
    case VINT:
        snprintf(tmp, sizeof tmp, "%" PRId64, v.u.i);
        buf_str(b, tmp);
        return;
    case VFLOAT: bur_format_float(b, v.u.f); return;
    case VOBJ: break;
    }
    Obj *o = v.u.o;
    switch (o->type) {
    case OBJ_STRING: {
        OString *s = (OString *)o;
        if (quote) bur_quote(b, s->data, s->len);
        else buf_bytes(b, s->data, s->len);
        return;
    }
    case OBJ_LIST: {
        OList *l = (OList *)o;
        buf_char(b, '[');
        for (int64_t i = 0; i < l->len; i++) {
            if (i > 0) buf_str(b, ", ");
            bur_format(b, l->elems[i], true);
        }
        buf_char(b, ']');
        return;
    }
    case OBJ_MAP: {
        OMap *m = (OMap *)o;
        buf_char(b, '{');
        for (int64_t i = 0; i < m->len; i++) {
            if (i > 0) buf_str(b, ", ");
            bur_format(b, m->entries[i].key, true);
            buf_str(b, ": ");
            bur_format(b, m->entries[i].val, true);
        }
        buf_char(b, '}');
        return;
    }
    case OBJ_FUNC: {
        buf_str(b, "<fn ");
        buf_str(b, ((OFunc *)o)->name);
        buf_char(b, '>');
        return;
    }
    case OBJ_CLOSURE: {
        const char *name = ((OClosure *)o)->fn->name;
        if (name == NULL || name[0] == '\0') name = "anonymous";
        buf_str(b, "<fn ");
        buf_str(b, name);
        buf_char(b, '>');
        return;
    }
    case OBJ_NATIVE:
        buf_str(b, "<native ");
        buf_str(b, ((ONative *)o)->name);
        buf_char(b, '>');
        return;
    case OBJ_ENUMTYPE:
        buf_str(b, "<enum ");
        buf_str(b, ((OEnumType *)o)->name);
        buf_char(b, '>');
        return;
    case OBJ_VARIANTCTOR: {
        OVariantCtor *c = (OVariantCtor *)o;
        buf_str(b, "<variant ");
        buf_str(b, c->enm->name);
        buf_char(b, '.');
        buf_str(b, c->enm->variants[c->idx].name);
        buf_char(b, '>');
        return;
    }
    case OBJ_ENUMINST: {
        OEnumInst *in = (OEnumInst *)o;
        buf_str(b, in->enm->variants[in->variant].name);
        if (in->nfields == 0) return;
        buf_char(b, '(');
        for (int i = 0; i < in->nfields; i++) {
            if (i > 0) buf_str(b, ", ");
            bur_format(b, in->fields[i], true);
        }
        buf_char(b, ')');
        return;
    }
    case OBJ_CHANNEL: {
        OChannel *ch = (OChannel *)o;
        char t[64];
        snprintf(t, sizeof t, "<chan cap=%d len=%d>", ch->cap, ch->buflen);
        buf_str(b, t);
        return;
    }
    case OBJ_TUPLE: {
        OTuple *t = (OTuple *)o;
        buf_char(b, '(');
        for (int64_t i = 0; i < t->n; i++) {
            if (i > 0) buf_str(b, ", ");
            bur_format(b, t->elems[i], true);
        }
        buf_char(b, ')');
        return;
    }
    case OBJ_RECORD: {
        ORecord *r = (ORecord *)o;
        buf_str(b, "record { ");
        for (int i = 0; i < r->nfields; i++) {
            if (i > 0) buf_str(b, ", ");
            buf_bytes(b, r->names[i]->data, r->names[i]->len);
            buf_str(b, ": ");
            bur_format(b, r->fields[i], true);
        }
        buf_str(b, " }");
        return;
    }
    default: return;
    }
}

// print a value the way print()/println() do (strings unquoted)
void bur_write_display(Value v) {
    Buf b = {0};
    bur_format(&b, v, false);
    fwrite(b.data, 1, (size_t)b.len, stdout);
    buf_free(&b);
}

// ---- Option/Result/Output constructors (root fresh strings on the
// operand stack across allocations, mirroring the VM) -------------------

Value bur_some(Value v) { return bur_obj((Obj *)bur_new_inst(bur_opt_enum, 0, &v, 1)); }
Value bur_none(void)    { return bur_obj((Obj *)bur_new_inst(bur_opt_enum, 1, NULL, 0)); }
Value bur_ok(Value v)   { return bur_obj((Obj *)bur_new_inst(bur_res_enum, 0, &v, 1)); }
Value bur_err(Value v)  { return bur_obj((Obj *)bur_new_inst(bur_res_enum, 1, &v, 1)); }

Value bur_ok_str(const char *s, int64_t n) {
    bur_push(bur_obj((Obj *)bur_new_string_n(s, n)));
    Value r = bur_ok(bur_peek(0));
    bur_pop();
    return r;
}
Value bur_err_str(const char *s) {
    bur_push(bur_obj((Obj *)bur_new_string(s)));
    Value r = bur_err(bur_peek(0));
    bur_pop();
    return r;
}

// ---- map internals ----------------------------------------------------

uint64_t mapkey_hash(MapKey k) {
    uint64_t h = 1469598103934665603ULL;
    if (k.is_str) {
        h ^= 0x11;
        for (int64_t i = 0; i < k.slen; i++) { h ^= (unsigned char)k.s[i]; h *= 1099511628211ULL; }
    } else {
        uint64_t x = (uint64_t)k.i;
        for (int j = 0; j < 8; j++) { h ^= (x & 0xff); h *= 1099511628211ULL; x >>= 8; }
    }
    return h;
}
bool mapkey_eq(MapKey a, MapKey b) {
    if (a.is_str != b.is_str) return false;
    if (a.is_str) return a.slen == b.slen && memcmp(a.s, b.s, (size_t)a.slen) == 0;
    return a.i == b.i;
}
bool mapkey_of(Value v, MapKey *out) {
    if (v.t == VINT) { out->is_str = false; out->i = v.u.i; out->s = NULL; out->slen = 0; return true; }
    if (v.t == VOBJ && v.u.o->type == OBJ_STRING) {
        OString *s = (OString *)v.u.o;
        out->is_str = true; out->s = s->data; out->slen = s->len; out->i = 0;
        return true;
    }
    return false;
}
void map_reindex(OMap *m) {
    for (int64_t i = 0; i < m->icap; i++) m->index[i] = -1;
    for (int64_t e = 0; e < m->len; e++) {
        uint64_t h = mapkey_hash(m->entries[e].k) & (uint64_t)(m->icap - 1);
        while (m->index[h] != -1) h = (h + 1) & (uint64_t)(m->icap - 1);
        m->index[h] = (int)e;
    }
}
bool map_get(OMap *m, MapKey k, Value *out) {
    if (m->icap == 0) return false;
    uint64_t h = mapkey_hash(k) & (uint64_t)(m->icap - 1);
    while (m->index[h] != -1) {
        if (mapkey_eq(m->entries[m->index[h]].k, k)) { *out = m->entries[m->index[h]].val; return true; }
        h = (h + 1) & (uint64_t)(m->icap - 1);
    }
    return false;
}
void map_set(OMap *m, MapKey k, Value key, Value val) {
    if (m->icap == 0) return; // set only after ensure
    uint64_t h = mapkey_hash(k) & (uint64_t)(m->icap - 1);
    while (m->index[h] != -1) {
        if (mapkey_eq(m->entries[m->index[h]].k, k)) { m->entries[m->index[h]].val = val; return; }
        h = (h + 1) & (uint64_t)(m->icap - 1);
    }
    if (m->len == m->cap) {
        m->cap = m->cap * 2 + 8;
        m->entries = (MapEntry *)realloc(m->entries, sizeof(MapEntry) * (size_t)m->cap);
    }
    MapKey ck = k;
    if (k.is_str) { ck.s = (char *)malloc((size_t)k.slen); memcpy(ck.s, k.s, (size_t)k.slen); }
    m->entries[m->len].k = ck;
    m->entries[m->len].key = key;
    m->entries[m->len].val = val;
    m->index[h] = (int)m->len;
    m->len++;
}
void map_ensure(OMap *m) {
    if ((m->len + 1) * 4 >= m->icap * 3 || m->icap == 0) {
        m->icap = m->icap == 0 ? 16 : m->icap * 2;
        m->index = (int *)realloc(m->index, sizeof(int) * (size_t)m->icap);
        map_reindex(m);
    }
}
void map_del(OMap *m, MapKey k) {
    int64_t pos = -1;
    for (int64_t i = 0; i < m->len; i++)
        if (mapkey_eq(m->entries[i].k, k)) { pos = i; break; }
    if (pos < 0) return;
    if (m->entries[pos].k.is_str) free(m->entries[pos].k.s);
    memmove(&m->entries[pos], &m->entries[pos + 1], sizeof(MapEntry) * (size_t)(m->len - pos - 1));
    m->len--;
    map_reindex(m);
}

// ---- arithmetic, comparison, indexing ---------------------------------

Value bur_add(Value a, Value b) {
    if (a.t == VOBJ && b.t == VOBJ &&
        a.u.o->type == OBJ_STRING && b.u.o->type == OBJ_STRING) {
        OString *x = (OString *)a.u.o, *y = (OString *)b.u.o;
        OString *r = (OString *)bur_alloc(sizeof(OString), OBJ_STRING);
        r->len = x->len + y->len;
        r->data = (char *)malloc((size_t)r->len + 1);
        memcpy(r->data, x->data, (size_t)x->len);
        memcpy(r->data + x->len, y->data, (size_t)y->len);
        r->data[r->len] = '\0';
        return bur_obj((Obj *)r);
    }
    if (a.t == VINT && b.t == VINT) {
        int64_t r = (int64_t)((uint64_t)a.u.i + (uint64_t)b.u.i);
        if ((r > a.u.i) != (b.u.i > 0)) bur_trap("integer overflow: %" PRId64 " + %" PRId64, a.u.i, b.u.i);
        return bur_int(r);
    }
    if ((a.t == VINT || a.t == VFLOAT) && (b.t == VINT || b.t == VFLOAT)) {
        double af = a.t == VINT ? (double)a.u.i : a.u.f;
        double bf = b.t == VINT ? (double)b.u.i : b.u.f;
        return bur_float(af + bf);
    }
    bur_trap("cannot apply \"+\" to %s and %s", bur_typename(a), bur_typename(b));
    return bur_unit();
}

Value bur_arith(Value a, Value b, char op) {
    if (a.t == VINT && b.t == VINT) {
        int64_t x = a.u.i, y = b.u.i, r;
        switch (op) {
        case '-':
            r = (int64_t)((uint64_t)x - (uint64_t)y);
            if ((r < x) != (y > 0)) bur_trap("integer overflow: %" PRId64 " - %" PRId64, x, y);
            return bur_int(r);
        case '*':
            r = (int64_t)((uint64_t)x * (uint64_t)y);
            if (x != 0 && (r / x != y || (x == -1 && y == INT64_MIN))) bur_trap("integer overflow: %" PRId64 " * %" PRId64, x, y);
            return bur_int(r);
        case '/':
            if (y == 0) bur_trap("integer division by zero");
            if (x == INT64_MIN && y == -1) bur_trap("integer overflow: %" PRId64 " / %" PRId64, x, y);
            return bur_int(x / y);
        case '%':
            if (y == 0) bur_trap("integer modulo by zero");
            return bur_int(x % y);
        }
    }
    if (op != '%' && (a.t == VINT || a.t == VFLOAT) && (b.t == VINT || b.t == VFLOAT)) {
        double af = a.t == VINT ? (double)a.u.i : a.u.f;
        double bf = b.t == VINT ? (double)b.u.i : b.u.f;
        switch (op) {
        case '-': return bur_float(af - bf);
        case '*': return bur_float(af * bf);
        case '/': return bur_float(af / bf);
        }
    }
    const char *name = op == '-' ? "-" : op == '*' ? "*" : op == '/' ? "/" : "%";
    bur_trap("cannot apply \"%s\" to %s and %s", name, bur_typename(a), bur_typename(b));
    return bur_unit();
}

Value bur_neg(Value v) {
    if (v.t == VINT) {
        if (v.u.i == INT64_MIN) bur_trap("integer overflow: -(%" PRId64 ")", v.u.i);
        return bur_int(-v.u.i);
    }
    if (v.t == VFLOAT) return bur_float(-v.u.f);
    bur_trap("operand of '-' must be a number, got %s", bur_typename(v));
    return bur_unit();
}

Value bur_not(Value v) {
    if (v.t != VBOOL) bur_trap("operand of '!' must be a bool, got %s", bur_typename(v));
    return bur_bool(!v.u.b);
}

// comparison operators: 0=Gt 1=GtEq 2=Lt 3=LtEq
Value bur_compare(Value a, Value b, int kind) {
    if (a.t == VOBJ && b.t == VOBJ &&
        a.u.o->type == OBJ_STRING && b.u.o->type == OBJ_STRING) {
        OString *x = (OString *)a.u.o, *y = (OString *)b.u.o;
        int64_t n = x->len < y->len ? x->len : y->len;
        int c = memcmp(x->data, y->data, (size_t)n);
        if (c == 0) c = x->len < y->len ? -1 : x->len > y->len ? 1 : 0;
        switch (kind) {
        case 0: return bur_bool(c > 0);
        case 1: return bur_bool(c >= 0);
        case 2: return bur_bool(c < 0);
        default: return bur_bool(c <= 0);
        }
    }
    if (a.t == VINT && b.t == VINT) {
        switch (kind) {
        case 0: return bur_bool(a.u.i > b.u.i);
        case 1: return bur_bool(a.u.i >= b.u.i);
        case 2: return bur_bool(a.u.i < b.u.i);
        default: return bur_bool(a.u.i <= b.u.i);
        }
    }
    if ((a.t == VINT || a.t == VFLOAT) && (b.t == VINT || b.t == VFLOAT)) {
        double af = a.t == VINT ? (double)a.u.i : a.u.f;
        double bf = b.t == VINT ? (double)b.u.i : b.u.f;
        switch (kind) {
        case 0: return bur_bool(af > bf);
        case 1: return bur_bool(af >= bf);
        case 2: return bur_bool(af < bf);
        default: return bur_bool(af <= bf);
        }
    }
    bur_trap("cannot compare %s and %s", bur_typename(a), bur_typename(b));
    return bur_unit();
}

Value bur_index_get(Value target, Value idx) {
    if (target.t == VOBJ && target.u.o->type == OBJ_LIST) {
        OList *l = (OList *)target.u.o;
        if (idx.t != VINT) bur_trap("list index must be an int, got %s", bur_typename(idx));
        if (idx.u.i < 0 || idx.u.i >= l->len)
            bur_trap("list index %" PRId64 " out of bounds (len %" PRId64 ")", idx.u.i, l->len);
        return l->elems[idx.u.i];
    }
    if (target.t == VOBJ && target.u.o->type == OBJ_STRING) {
        OString *s = (OString *)target.u.o;
        if (idx.t != VINT) bur_trap("string index must be an int, got %s", bur_typename(idx));
        if (idx.u.i < 0 || idx.u.i >= s->len)
            bur_trap("string index %" PRId64 " out of bounds (len %" PRId64 ")", idx.u.i, s->len);
        return bur_obj((Obj *)bur_new_string_n(s->data + idx.u.i, 1));
    }
    if (target.t == VOBJ && target.u.o->type == OBJ_TUPLE) {
        OTuple *t = (OTuple *)target.u.o;
        if (idx.t != VINT) bur_trap("tuple index must be an int, got %s", bur_typename(idx));
        if (idx.u.i < 0 || idx.u.i >= t->n)
            bur_trap("tuple index %" PRId64 " out of bounds (len %" PRId64 ")", idx.u.i, t->n);
        return t->elems[idx.u.i];
    }
    bur_trap("cannot index %s", bur_typename(target));
    return bur_unit();
}

void bur_index_set(Value target, Value idx, Value val) {
    if (target.t != VOBJ || target.u.o->type != OBJ_LIST)
        bur_trap("cannot index-assign into %s", bur_typename(target));
    OList *l = (OList *)target.u.o;
    if (idx.t != VINT) bur_trap("list index must be an int, got %s", bur_typename(idx));
    if (idx.u.i < 0 || idx.u.i >= l->len)
        bur_trap("list index %" PRId64 " out of bounds (len %" PRId64 ")", idx.u.i, l->len);
    l->elems[idx.u.i] = val;
}

int64_t bur_len(Value v) {
    if (v.t == VOBJ) {
        if (v.u.o->type == OBJ_LIST) return ((OList *)v.u.o)->len;
        if (v.u.o->type == OBJ_STRING) return ((OString *)v.u.o)->len;
        if (v.u.o->type == OBJ_MAP) return ((OMap *)v.u.o)->len;
    }
    bur_trap("len() needs a list, string, or map, got %s", bur_typename(v));
    return 0;
}

// ---- upvalues ---------------------------------------------------------

Value upvalue_get(OUpvalue *u) { return u->open ? u->fiber->stack[u->slot] : u->closed; }
void upvalue_set(OUpvalue *u, Value v) {
    if (u->open) u->fiber->stack[u->slot] = v; else u->closed = v;
}

OUpvalue *bur_capture_upvalue(int slot) {
    for (int i = 0; i < bur_cur->nopen; i++)
        if (bur_cur->openUpvals[i]->slot == slot) return bur_cur->openUpvals[i];
    OUpvalue *u = (OUpvalue *)bur_alloc(sizeof(OUpvalue), OBJ_UPVALUE);
    u->fiber = bur_cur;
    u->slot = slot;
    u->open = true;
    if (bur_cur->nopen == bur_cur->opencap) {
        bur_cur->opencap = bur_cur->opencap * 2 + 8;
        bur_cur->openUpvals = (OUpvalue **)realloc(bur_cur->openUpvals, sizeof(OUpvalue *) * (size_t)bur_cur->opencap);
    }
    bur_cur->openUpvals[bur_cur->nopen++] = u;
    return u;
}

// bur_new_closed_cell wraps a value in an independent closed cell
// (value-capture snapshots: never open, never shared)
OUpvalue *bur_new_closed_cell(Value v) {
    OUpvalue *u = (OUpvalue *)bur_alloc(sizeof(OUpvalue), OBJ_UPVALUE);
    u->fiber = NULL;
    u->slot = 0;
    u->open = false;
    u->closed = v;
    return u;
}

// bur_capture_value captures a value-capture snapshot from a stack slot
OUpvalue *bur_capture_value(int slot) {
    return bur_new_closed_cell(bur_cur->stack[slot]);
}

void bur_close_upvalues(int from) {
    int kept = 0;
    for (int i = 0; i < bur_cur->nopen; i++) {
        OUpvalue *u = bur_cur->openUpvals[i];
        if (u->slot >= from) {
            u->closed = bur_cur->stack[u->slot];
            u->open = false;
        } else {
            bur_cur->openUpvals[kept++] = u;
        }
    }
    bur_cur->nopen = kept;
}

// ---- calls ------------------------------------------------------------

// Invoke the value at peek(argc). Closures run their compiled body via the
// C call stack; natives and variant constructors are handled inline. The
// callee and its argc arguments occupy the top argc+1 stack slots and are
// replaced by the single result.
void bur_call(int argc) {
    Value callee = bur_peek(argc);
    if (callee.t != VOBJ) bur_trap("cannot call %s", bur_typename(callee));
    switch (callee.u.o->type) {
    case OBJ_CLOSURE: {
        OClosure *cl = (OClosure *)callee.u.o;
        if (argc != cl->fn->arity)
            bur_trap("%s expects %d argument(s), got %d", cl->fn->name, cl->fn->arity, argc);
        if (++bur_cur->call_depth > 2048) bur_trap("stack overflow (call depth > 2048)");
        int d = bur_cur->call_depth;
        if (d >= bur_cur->trace_cap) {
            int nc = bur_cur->trace_cap ? bur_cur->trace_cap : 8;
            while (nc <= d) nc *= 2;
            bur_cur->trace_fn = (OFunc **)realloc(bur_cur->trace_fn, sizeof(OFunc *) * nc);
            bur_cur->trace_ln = (int *)realloc(bur_cur->trace_ln, sizeof(int) * nc);
            bur_cur->trace_cap = nc;
        }
        bur_cur->trace_fn[d] = cl->fn;
        bur_cur->trace_ln[d] = 0;
        OClosure *prev = bur_cur_closure;
        bur_cur_closure = cl;
        cl->fn->code();
        bur_cur_closure = prev;
        bur_cur->call_depth--;
        return;
    }
    case OBJ_NATIVE: {
        ONative *n = (ONative *)callee.u.o;
        if (n->arity >= 0 && argc != n->arity)
            bur_trap("%s expects %d argument(s), got %d", n->name, n->arity, argc);
        Value res = n->fn(&bur_cur->stack[bur_cur->top - argc], argc);
        bur_cur->top -= argc + 1;
        bur_push(res);
        return;
    }
    case OBJ_VARIANTCTOR: {
        OVariantCtor *c = (OVariantCtor *)callee.u.o;
        int arity = c->enm->variants[c->idx].arity;
        if (argc != arity)
            bur_trap("%s.%s expects %d field(s), got %d", c->enm->name, c->enm->variants[c->idx].name, arity, argc);
        OEnumInst *in = bur_new_inst(c->enm, c->idx, &bur_cur->stack[bur_cur->top - argc], argc);
        bur_cur->top -= argc + 1;
        bur_push(bur_obj((Obj *)in));
        return;
    }
    default:
        bur_trap("cannot call %s", bur_typename(callee));
    }
}

// ---- defers -----------------------------------------------------------

// bur_defer_push registers a closure on the current fiber's defer stack;
// the registering frame runs its slice LIFO on every function exit
void bur_defer_push(Value cl) {
    if (bur_cur->ndefers == bur_cur->defercap) {
        bur_cur->defercap = bur_cur->defercap * 2 + 8;
        bur_cur->defers = (Value *)realloc(bur_cur->defers, sizeof(Value) * (size_t)bur_cur->defercap);
    }
    bur_cur->defers[bur_cur->ndefers++] = cl;
}

// bur_run_defers pops and calls the frame's defers (those above dbase)
// LIFO; callers run it before popping the exit value so that value stays
// rooted on the stack across the deferred calls
void bur_run_defers(int dbase) {
    while (bur_cur->ndefers > dbase) {
        Value cl = bur_cur->defers[--bur_cur->ndefers];
        bur_push(cl);
        bur_call(0);
        bur_pop();
    }
}

// ---- runtime bootstrap helpers ----------------------------------------

void bur_globals_put(const char *key, int64_t klen, Value v);

OClosure *bur_new_closure(OFunc *fn) {
    OClosure *cl = (OClosure *)bur_alloc(sizeof(OClosure), OBJ_CLOSURE);
    cl->fn = fn;
    cl->nupvals = fn->numUpvals;
    if (fn->numUpvals > 0)
        cl->upvals = (OUpvalue **)calloc((size_t)fn->numUpvals, sizeof(OUpvalue *));
    return cl;
}

// ---- globals table ----------------------------------------------------

uint64_t str_hash(const char *s, int64_t n) {
    uint64_t h = 1469598103934665603ULL;
    for (int64_t i = 0; i < n; i++) { h ^= (unsigned char)s[i]; h *= 1099511628211ULL; }
    return h;
}
void bur_globals_grow(void) {
    int64_t oldcap = bur_globals_cap;
    GlobalSlot *old = bur_globals;
    bur_globals_cap = oldcap == 0 ? 64 : oldcap * 2;
    bur_globals = (GlobalSlot *)calloc((size_t)bur_globals_cap, sizeof(GlobalSlot));
    bur_globals_len = 0;
    for (int64_t i = 0; i < oldcap; i++)
        if (old[i].used) bur_globals_put(old[i].key, old[i].klen, old[i].val);
    free(old);
}
GlobalSlot *bur_globals_slot(const char *key, int64_t klen) {
    uint64_t h = str_hash(key, klen) & (uint64_t)(bur_globals_cap - 1);
    while (bur_globals[h].used) {
        if (bur_globals[h].klen == klen && memcmp(bur_globals[h].key, key, (size_t)klen) == 0)
            return &bur_globals[h];
        h = (h + 1) & (uint64_t)(bur_globals_cap - 1);
    }
    return &bur_globals[h];
}
void bur_globals_put(const char *key, int64_t klen, Value v) {
    if (bur_globals_cap == 0 || (bur_globals_len + 1) * 4 >= bur_globals_cap * 3) bur_globals_grow();
    GlobalSlot *s = bur_globals_slot(key, klen);
    if (!s->used) {
        s->key = (char *)malloc((size_t)klen + 1);
        memcpy(s->key, key, (size_t)klen);
        s->key[klen] = '\0';
        s->klen = klen;
        s->used = true;
        bur_globals_len++;
    }
    s->val = v;
}
bool bur_globals_get(const char *key, int64_t klen, Value *out) {
    if (bur_globals_cap == 0) return false;
    GlobalSlot *s = bur_globals_slot(key, klen);
    if (!s->used) return false;
    *out = s->val;
    return true;
}

// opcode helpers used directly by generated code
Value bur_get_global(const char *name, int64_t n) {
    Value v;
    if (!bur_globals_get(name, n, &v)) bur_trap("undefined variable \"%.*s\"", (int)n, name);
    return v;
}
void bur_set_global(const char *name, int64_t n, Value v) {
    Value old;
    if (!bur_globals_get(name, n, &old)) bur_trap("undefined variable \"%.*s\"", (int)n, name);
    bur_globals_put(name, n, v);
}

// ---- scheduler, channels, and blocking operations ---------------------
//
// A faithful C port of vm.go's cooperative scheduler. Because generated
// functions run on the native C call stack (frames == C frames), a fiber's
// suspended state is its whole C stack: blocking swaps that stack out to the
// scheduler with swapcontext and resumes it later. The VM's "rewind ip and
// retry once woken" becomes a real C-level retry loop around bur_park.

void bur_fiber_entry(void); // trampoline for a fiber's first resume
#ifdef _WIN32
void CALLBACK bur_fiber_trampoline(LPVOID param); // CreateFiber entry point
#endif

// growable FIFO of fibers (used for channel send/recv/waiter queues)
void fq_push(Fiber ***a, int *n, int *cap, Fiber *f) {
    if (*n == *cap) { *cap = *cap * 2 + 4; *a = (Fiber **)realloc(*a, sizeof(Fiber *) * (size_t)(*cap)); }
    (*a)[(*n)++] = f;
}
Fiber *fq_pop(Fiber ***a, int *n) {
    Fiber *f = (*a)[0];
    memmove(*a, *a + 1, sizeof(Fiber *) * (size_t)(*n - 1));
    (*n)--;
    return f;
}

// channel buffer: a bounded FIFO of Values
void chan_buf_push(OChannel *ch, Value v) {
    if (ch->buflen == ch->bufcap) { ch->bufcap = ch->bufcap * 2 + 4; ch->buf = (Value *)realloc(ch->buf, sizeof(Value) * (size_t)ch->bufcap); }
    ch->buf[ch->buflen++] = v;
}
Value chan_buf_pop(OChannel *ch) {
    Value v = ch->buf[0];
    memmove(ch->buf, ch->buf + 1, sizeof(Value) * (size_t)(ch->buflen - 1));
    ch->buflen--;
    return v;
}

// ready queue (FIFO, head-index drained)
void bur_ready_push(Fiber *f) {
    if (bur_ready_len == bur_ready_cap) {
        bur_ready_cap = bur_ready_cap * 2 + 64;
        bur_ready = (Fiber **)realloc(bur_ready, sizeof(Fiber *) * (size_t)bur_ready_cap);
    }
    bur_ready[bur_ready_len++] = f;
}
Fiber *bur_ready_pop(void) {
    if (bur_ready_head == bur_ready_len) return NULL;
    Fiber *f = bur_ready[bur_ready_head++];
    if (bur_ready_head == bur_ready_len) bur_ready_head = bur_ready_len = 0;
    return f;
}
void bur_fibers_push(Fiber *f) {
    if (bur_nfibers == bur_fiberscap) {
        bur_fiberscap = bur_fiberscap * 2 + 16;
        bur_fibers = (Fiber **)realloc(bur_fibers, sizeof(Fiber *) * (size_t)bur_fiberscap);
    }
    bur_fibers[bur_nfibers++] = f;
}

void bur_schedule(Fiber *f) { f->status = FREADY; bur_ready_push(f); }

// wakeWaiters: reschedule every fiber parked in a select on ch so each
// re-polls its arms; a fiber woken elsewhere first is skipped.
void bur_wake_waiters(OChannel *ch) {
    if (ch->nwait == 0) return;
    for (int i = 0; i < ch->nwait; i++)
        if (ch->waiters[i]->status == FBLOCKED_SELECT) bur_schedule(ch->waiters[i]);
    ch->nwait = 0;
}
void bur_remove_waiter(OChannel *ch, Fiber *f) {
    for (int i = 0; i < ch->nwait; i++)
        if (ch->waiters[i] == f) {
            memmove(&ch->waiters[i], &ch->waiters[i + 1], sizeof(Fiber *) * (size_t)(ch->nwait - i - 1));
            ch->nwait--;
            return;
        }
}
void bur_clear_select(Fiber *f) {
    for (int i = 0; i < f->nselect; i++) bur_remove_waiter(f->selectChans[i], f);
    f->nselect = 0;
}
void bur_select_add(Fiber *f, OChannel *ch) {
    if (f->nselect == f->selectcap) { f->selectcap = f->selectcap * 2 + 4; f->selectChans = (OChannel **)realloc(f->selectChans, sizeof(OChannel *) * (size_t)f->selectcap); }
    f->selectChans[f->nselect++] = ch;
}

bool chan_recv_ready(OChannel *ch) { return ch->buflen > 0 || ch->nsendq > 0 || ch->closed; }
bool chan_send_ready(OChannel *ch) { return ch->closed || ch->nrecvq > 0 || ch->buflen < ch->cap; }

// chanTryRecv: non-blocking receive, waking one blocked sender to refill the
// slot it drains. Returns false when nothing is available yet.
bool chan_try_recv(OChannel *ch, Value *out) {
    if (ch->buflen > 0) {
        *out = chan_buf_pop(ch);
        if (ch->nsendq > 0) {
            Fiber *s = fq_pop(&ch->sendq, &ch->nsendq);
            chan_buf_push(ch, s->sendVal);
            s->sendVal = bur_unit();
            bur_schedule(s);
        }
        return true;
    }
    if (ch->nsendq > 0) { // unbuffered rendezvous
        Fiber *s = fq_pop(&ch->sendq, &ch->nsendq);
        *out = s->sendVal;
        s->sendVal = bur_unit();
        bur_schedule(s);
        return true;
    }
    return false;
}

// park the running fiber (swap its C stack out to the scheduler); execution
// resumes here once the scheduler picks it again, with a fresh time slice.
void bur_park(FiberStatus st) {
    bur_cur->status = st;
#ifdef _WIN32
    SwitchToFiber(bur_sched_ctx);
#else
    swapcontext(&bur_cur->ctx, &bur_sched_ctx);
#endif
    bur_cur->budget = BUR_TIMESLICE;
}
void bur_wait_current_fd(intptr_t fd, short events) {
    bur_cur->io_proc = -1;
    bur_cur->io_fd = fd;
    bur_cur->io_events = events;
    bur_cur->io_ready = false;
    bur_nio++;
    bur_park(FBLOCKED_IO);
}
void bur_switch_to_sched(void) {
#ifdef _WIN32
    SwitchToFiber(bur_sched_ctx);
#else
    swapcontext(&bur_cur->ctx, &bur_sched_ctx);
#endif
    bur_cur->budget = BUR_TIMESLICE;
}
// preemption: the running fiber has spent its time slice; yield the CPU by
// putting itself at the back of the ready queue. Invoked from the budget
// hooks the backend plants at back-edges and function entry.
void bur_preempt(void) {
    bur_schedule(bur_cur);
    bur_switch_to_sched();
}

OChannel *as_channel_opt(Value v) {
    return (v.t == VOBJ && v.u.o->type == OBJ_CHANNEL) ? (OChannel *)v.u.o : NULL;
}

Fiber *bur_new_fiber(OClosure *cl, Value *args, int argc, size_t stacksize) {
    Fiber *f = (Fiber *)calloc(1, sizeof(Fiber));
    f->id = bur_next_fiber_id++;
    f->cap = 256;
    f->stack = (Value *)malloc(sizeof(Value) * 256);
    f->status = FREADY;
    f->sendVal = bur_unit();
    f->budget = BUR_TIMESLICE;
    f->io_proc = -1;
    f->io_fd = -1;
    f->entry = cl;
    f->trace_cap = 8;
    f->trace_fn = (OFunc **)calloc(8, sizeof(OFunc *));
    f->trace_ln = (int *)calloc(8, sizeof(int));
    f->trace_fn[0] = cl->fn; // the entry body runs at depth 0, outside bur_call
    f->stack[f->top++] = bur_obj((Obj *)cl); // closure + args, like vm.newFiber
    for (int i = 0; i < argc; i++) f->stack[f->top++] = args[i];
#ifdef _WIN32
    f->ctx = CreateFiber(stacksize, bur_fiber_trampoline, f);
#else
    f->cstack = (char *)malloc(stacksize);
    getcontext(&f->ctx);
    f->ctx.uc_stack.ss_sp = f->cstack;
    f->ctx.uc_stack.ss_size = stacksize;
    f->ctx.uc_link = &bur_sched_ctx;
    makecontext(&f->ctx, bur_fiber_entry, 0);
#endif
    bur_fibers_push(f);
    return f;
}

// shared body of a fiber's first resume; the platform tails hand control
// back to the scheduler once it returns
static void bur_fiber_run_body(void) {
    Fiber *f = bur_cur;
    bur_cur_closure = f->entry;
    f->budget = BUR_TIMESLICE;
    f->entry->fn->code(); // run the body on this fiber's own C stack
    f->status = FDONE;
}
#ifndef _WIN32
void bur_fiber_entry(void) {
    bur_fiber_run_body();
    setcontext(&bur_sched_ctx); // hand control back for good
}
#else
void CALLBACK bur_fiber_trampoline(LPVOID param) {
    (void)param; // the scheduler already parked the fiber pointer in bur_cur
    bur_fiber_run_body();
    SwitchToFiber(bur_sched_ctx); // hand control back for good
}
#endif

int64_t bur_now_ns(void) { return bur_mono_ns(); }

// ---- child processes (exec_start/exec_poll and the exec native) --------
//
// A spawned child is a slot here: its pipes are drained non-blockingly by
// bur_proc_pump, the scheduler's idle poll watches every live fd, and a
// slot is complete once both pipes hit EOF and the child is reaped.


BurProc *bur_procs;
int64_t bur_nprocs, bur_procscap;



BurWaitSet bur_waitset;

void bur_wait_reset(void) {
    bur_waitset.n = 0;
    bur_waitset.deadline_ns = INT64_MAX;
}

void bur_wait_fd(intptr_t fd, short events, int64_t owner, BurWaitReadyFn ready) {
    if (fd < 0) return;
    if (bur_waitset.n == bur_waitset.cap) {
        bur_waitset.cap = bur_waitset.cap * 2 + 16;
        size_t size = (size_t)bur_waitset.cap;
        bur_waitset.fds = (struct pollfd *)realloc(bur_waitset.fds, sizeof(struct pollfd) * size);
        bur_waitset.owners = (int64_t *)realloc(bur_waitset.owners, sizeof(int64_t) * size);
        bur_waitset.ready = (BurWaitReadyFn *)realloc(bur_waitset.ready, sizeof(BurWaitReadyFn) * size);
    }
    int i = bur_waitset.n++;
    bur_waitset.fds[i].fd = fd; // SOCKET on Windows; implicit intptr_t fit
    bur_waitset.fds[i].events = events;
    bur_waitset.fds[i].revents = 0;
    bur_waitset.owners[i] = owner;
    bur_waitset.ready[i] = ready;
}

void bur_wait_timer(int64_t deadline_ns) {
    if (deadline_ns < bur_waitset.deadline_ns) bur_waitset.deadline_ns = deadline_ns;
}

bool bur_proc_valid(int64_t h) {
    return h >= 0 && h < bur_nprocs && bur_procs[h].used && !bur_procs[h].consumed;
}

// drain whatever the pipes hold without blocking; reap once both hit EOF.
// POSIX polls raw fds; Windows peeks named pipes because pipe handles are
// not fds, and reaps through the process handle instead of waitpid.
#ifndef _WIN32
void bur_proc_pump(BurProc *p) {
    if (p->complete) return;
    char chunk[8192]; ssize_t r;
    if (p->failfd >= 0) {
        r = read((int)p->failfd, &p->child_err, sizeof p->child_err);
        if (r == (ssize_t)sizeof p->child_err) p->have_err = true;
        if (r == 0 || r > 0) { close((int)p->failfd); p->failfd = -1; }
    }
    if (p->outfd >= 0) {
        while ((r = read((int)p->outfd, chunk, sizeof chunk)) > 0) buf_bytes(&p->ob, chunk, (int64_t)r);
        if (r == 0) { close((int)p->outfd); p->outfd = -1; }
    }
    if (p->errfd >= 0) {
        while ((r = read((int)p->errfd, chunk, sizeof chunk)) > 0) buf_bytes(&p->eb, chunk, (int64_t)r);
        if (r == 0) { close((int)p->errfd); p->errfd = -1; }
    }
    if (p->outfd < 0 && p->errfd < 0) {
        if (p->failfd >= 0) { // exec succeeded: CLOEXEC closed it; drain the EOF
            r = read((int)p->failfd, &p->child_err, sizeof p->child_err);
            if (r == (ssize_t)sizeof p->child_err) p->have_err = true;
            close((int)p->failfd); p->failfd = -1;
        }
        int status = 0;
        waitpid((pid_t)p->pid, &status, 0); // both pipes are EOF, the child is gone
        p->code = WIFEXITED(status) ? WEXITSTATUS(status) : 128 + (WIFSIGNALED(status) ? WTERMSIG(status) : 0);
        p->complete = true;
    }
}
#else
void bur_proc_pump(BurProc *p) {
    if (p->complete) return;
    char chunk[8192];
    HANDLE h[2] = { (HANDLE)p->outfd, (HANDLE)p->errfd };
    Buf *into[2] = { &p->ob, &p->eb };
    for (int k = 0; k < 2; k++) {
        if ((intptr_t)h[k] == -1) continue;
        DWORD avail = 0;
        if (!PeekNamedPipe(h[k], NULL, 0, NULL, &avail, NULL)) { // broken pipe => EOF
            CloseHandle(h[k]);
            if (k == 0) p->outfd = -1; else p->errfd = -1;
            continue;
        }
        while (avail > 0) {
            DWORD want = avail > sizeof chunk ? (DWORD)sizeof chunk : avail, got = 0;
            if (!ReadFile(h[k], chunk, want, &got, NULL) || got == 0) break;
            buf_bytes(into[k], chunk, (int64_t)got);
            avail -= got;
        }
    }
    if (p->outfd == -1 && p->errfd == -1 &&
        WaitForSingleObject((HANDLE)p->pid, 0) == WAIT_OBJECT_0) {
        DWORD code = 0;
        GetExitCodeProcess((HANDLE)p->pid, &code);
        p->code = (int)code;
        CloseHandle((HANDLE)p->pid);
        p->complete = true;
    }
}
#endif

void bur_proc_ready(int64_t owner, short revents) {
    (void)revents;
    bur_proc_pump(&bur_procs[owner]);
}

void bur_fiber_fd_ready(int64_t owner, short revents) {
    (void)revents;
    Fiber *f = bur_fibers[owner];
    if (f->status == FBLOCKED_IO && f->io_fd >= 0) f->io_ready = true;
}

// Register every idle source through one wait set, then poll once. The
// timer entry supplies poll's timeout; fd callbacks handle ready sources.
void bur_wait_poll(bool block) {
    bur_wait_reset();
    for (int64_t i = 0; i < bur_nprocs; i++) {
        BurProc *p = &bur_procs[i];
        if (!p->used || p->complete) continue;
        bur_wait_fd(p->outfd, POLLIN, i, bur_proc_ready);
        bur_wait_fd(p->errfd, POLLIN, i, bur_proc_ready);
        bur_wait_fd(p->failfd, POLLIN, i, bur_proc_ready);
    }
    for (int64_t i = 0; i < bur_nfibers; i++) {
        Fiber *f = bur_fibers[i];
        if (f->status == FBLOCKED_TIMER) bur_wait_timer(f->wake_ns);
        else if (f->status == FBLOCKED_IO && f->io_fd >= 0)
            bur_wait_fd(f->io_fd, f->io_events, i, bur_fiber_fd_ready);
    }
    int timeout_ms = 0;
    if (block) {
        timeout_ms = -1;
        if (bur_waitset.deadline_ns != INT64_MAX) {
            int64_t d = bur_waitset.deadline_ns - bur_now_ns();
            timeout_ms = d <= 0 ? 0 : (int)(d / 1000000) + 1;
        }
    }
#ifdef _WIN32
    WSAPoll(bur_waitset.fds, (ULONG)bur_waitset.n, timeout_ms);
#else
    poll(bur_waitset.fds, (nfds_t)bur_waitset.n, timeout_ms);
#endif
    for (int i = 0; i < bur_waitset.n; i++)
        if (bur_waitset.fds[i].revents)
            bur_waitset.ready[i](bur_waitset.owners[i], bur_waitset.fds[i].revents);
    if (bur_nio == 0) return;
    for (int64_t i = 0; i < bur_nfibers; i++) { // wake in fiber id order
        Fiber *f = bur_fibers[i];
        bool ready = f->io_ready;
        if (!ready && f->status == FBLOCKED_IO && f->io_proc >= 0)
            ready = bur_procs[f->io_proc].complete;
        if (f->status == FBLOCKED_IO && ready) {
            bur_nio--;
            f->io_proc = -1;
            f->io_fd = -1;
            f->io_events = 0;
            f->io_ready = false;
            bur_schedule(f);
        }
    }
}

// wake every timer whose deadline has passed, in (deadline, fiber id) order;
// bur_fibers is in creation (= id) order, so a strictly-less scan suffices
void bur_wake_due_timers(void) {
    int64_t now = bur_now_ns();
    while (bur_ntimers > 0) {
        Fiber *best = NULL;
        for (int64_t i = 0; i < bur_nfibers; i++) {
            Fiber *f = bur_fibers[i];
            if (f->status == FBLOCKED_TIMER && f->wake_ns <= now &&
                (!best || f->wake_ns < best->wake_ns)) best = f;
        }
        if (!best) return;
        bur_ntimers--;
        bur_schedule(best);
    }
}

// the scheduler loop: run ready fibers FIFO until the main fiber returns
// (Go semantics) or nothing is left. All fibers blocked on channels =>
// deadlock; timer waiters are alive, so an idle scheduler sleeps until the
// nearest deadline instead.
void bur_scheduler(void) {
    for (;;) {
        if (bur_main_fiber->status == FDONE) return;
        if (bur_ntimers > 0) bur_wake_due_timers();
        if (bur_nio > 0) bur_wait_poll(false); // keep child pipes drained
        Fiber *f = bur_ready_pop();
        if (!f) {
            if (bur_ntimers > 0 || bur_nio > 0) {
                bur_wait_poll(true);
                continue;
            }
            int blocked = 0;
            for (int64_t i = 0; i < bur_nfibers; i++) {
                FiberStatus s = bur_fibers[i]->status;
                if (s == FBLOCKED_SEND || s == FBLOCKED_RECV || s == FBLOCKED_SELECT) blocked++;
            }
            if (blocked > 0) {
                fflush(stdout);
                fprintf(stderr, "fatal: deadlock \xe2\x80\x94 all %d remaining fiber(s) are blocked on channels\n", blocked);
                exit(4);
            }
            return;
        }
        if (f->status != FREADY) continue; // stale ready entry (already woken elsewhere)
        bur_cur = f;
#ifdef _WIN32
        SwitchToFiber(f->ctx);
#else
        swapcontext(&bur_sched_ctx, &f->ctx);
#endif
    }
}

// ---- concurrency opcodes ----------------------------------------------

void bur_spawn(int argc) {
    Value callee = bur_peek(argc);
    if (callee.t != VOBJ || callee.u.o->type != OBJ_CLOSURE)
        bur_trap("spawn needs a function, got %s", bur_typename(callee));
    OClosure *cl = (OClosure *)callee.u.o;
    if (cl->fn->arity != argc)
        bur_trap("%s expects %d argument(s), got %d", cl->fn->name, cl->fn->arity, argc);
    Fiber *nf = bur_new_fiber(cl, &bur_cur->stack[bur_cur->top - argc], argc, BUR_STACK_SIZE);
    bur_schedule(nf);
    bur_cur->top -= argc + 1;
}

void bur_send(Value chv, Value val) {
    OChannel *ch = as_channel_opt(chv);
    if (!ch) bur_trap("cannot send to %s (need a channel)", bur_typename(chv));
    if (ch->closed) bur_trap("send on closed channel");
    if (ch->nrecvq > 0) {
        chan_buf_push(ch, val); // hand off through the buffer; woken receiver finds it
        bur_schedule(fq_pop(&ch->recvq, &ch->nrecvq));
        bur_wake_waiters(ch);
    } else if (ch->buflen < ch->cap) {
        chan_buf_push(ch, val);
        bur_wake_waiters(ch);
    } else {
        bur_cur->sendVal = val;
        fq_push(&ch->sendq, &ch->nsendq, &ch->sendqcap, bur_cur);
        bur_wake_waiters(ch);       // a select receive arm can now proceed
        bur_park(FBLOCKED_SEND);    // woken means a receiver took our sendVal
    }
}

Value bur_recv(Value chv) {
    OChannel *ch = as_channel_opt(chv);
    if (!ch) bur_trap("cannot receive from %s (need a channel)", bur_typename(chv));
    for (;;) {
        Value v;
        if (chan_try_recv(ch, &v)) { bur_wake_waiters(ch); return v; }
        if (ch->closed) bur_trap("receive on closed channel");
        fq_push(&ch->recvq, &ch->nrecvq, &ch->recvqcap, bur_cur);
        bur_wake_waiters(ch);       // a select send arm can now proceed
        bur_park(FBLOCKED_RECV);    // re-run once woken
    }
}

// for v in ch: yield successive values; returns false once closed and drained
bool bur_chan_next(Value chv, Value *out) {
    OChannel *ch = as_channel_opt(chv);
    if (!ch) bur_trap("cannot iterate %s (need a channel)", bur_typename(chv));
    for (;;) {
        if (chan_try_recv(ch, out)) { bur_wake_waiters(ch); return true; }
        if (ch->closed) return false;
        fq_push(&ch->recvq, &ch->nrecvq, &ch->recvqcap, bur_cur);
        bur_wake_waiters(ch);
        bur_park(FBLOCKED_RECV);
    }
}

// select over kinds[] (1=send arm, 0=recv arm). Returns the chosen arm index
// (nArms when the default arm runs); a chosen recv leaves its value on the
// stack for a binding arm. Operands were pushed in declaration order.
int bur_select(const unsigned char *kinds, int nArms, bool hasDefault) {
    int slots = 0;
    for (int i = 0; i < nArms; i++) slots += kinds[i] ? 2 : 1;
    int base = bur_cur->top - slots;
    // MSVC has no VLAs; one allocation holds both position tables
    int *chanPos = (int *)malloc(sizeof(int) * (size_t)nArms * 2);
    int *valPos = chanPos + nArms;
    int off = base;
    for (int i = 0; i < nArms; i++) {
        chanPos[i] = off;
        if (kinds[i]) { valPos[i] = off + 1; off += 2; } else { valPos[i] = -1; off++; }
    }
    for (;;) {
        bur_clear_select(bur_cur); // drop stale waiter registrations from a prior park
        int chosen = -1;
        for (int i = 0; i < nArms; i++) {
            OChannel *ch = as_channel_opt(bur_cur->stack[chanPos[i]]);
            if (!ch) bur_trap("select arm needs a channel, got %s", bur_typename(bur_cur->stack[chanPos[i]]));
            if (kinds[i] ? chan_send_ready(ch) : chan_recv_ready(ch)) { chosen = i; break; }
        }
        if (chosen >= 0) {
            OChannel *ch = (OChannel *)bur_cur->stack[chanPos[chosen]].u.o;
            if (kinds[chosen]) {
                if (ch->closed) bur_trap("send on closed channel");
                Value val = bur_cur->stack[valPos[chosen]];
                if (ch->nrecvq > 0) { chan_buf_push(ch, val); bur_schedule(fq_pop(&ch->recvq, &ch->nrecvq)); }
                else chan_buf_push(ch, val);
                bur_wake_waiters(ch);
                bur_cur->top = base;
            } else {
                Value v;
                if (!chan_try_recv(ch, &v)) { bur_cur->top = base; bur_trap("receive on closed channel"); }
                bur_wake_waiters(ch);
                bur_cur->top = base;
                bur_push(v); // left on the stack for a binding arm
            }
            free(chanPos);
            return chosen;
        }
        if (hasDefault) { bur_cur->top = base; free(chanPos); return nArms; }
        // nothing ready: park on every arm's channel, retry when any wakes us
        for (int i = 0; i < nArms; i++) {
            OChannel *ch = (OChannel *)bur_cur->stack[chanPos[i]].u.o;
            fq_push(&ch->waiters, &ch->nwait, &ch->waitcap, bur_cur);
            bur_select_add(bur_cur, ch);
        }
        bur_park(FBLOCKED_SELECT);
    }
}

#include "burrt_natives.h"

// ---- boot -------------------------------------------------------------

void bur_boot(int argc, char **argv) {
    bur_argc = argc;
    bur_argv = argv;
    const char *det = getenv("BUR_DETERMINISTIC");
    bur_deterministic = det && det[0] == '1' && det[1] == '\0';
    bur_start_ns = bur_mono_ns();
#ifdef _WIN32
    WSADATA wsa;
    WSAStartup(MAKEWORD(2, 2), &wsa);
    bur_sched_ctx = ConvertThreadToFiber(NULL); // the main thread schedules
#endif
    setvbuf(stdout, NULL, _IOFBF, 1 << 16);
    // fibers (including the main one) are created by generated main()
}


