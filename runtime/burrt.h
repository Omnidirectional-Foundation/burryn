// Burryn C runtime — sequential core.
//
// A faithful port of the Go bytecode VM's value model, precise mark-sweep
// GC, and stack machine. Generated programs (see cbackend.go) include this
// header, define one C function per compiled Burryn function, and provide a
// main() that boots the runtime. Concurrency (fibers, channels, select) is
// hosted separately and layered on top later.
//
// Parity contract: stdout bytes + process exit code match the VM. Trap and
// diagnostic text are explicitly outside that contract, so trap messages
// here stay terse.
#ifndef BURRT_H
#define BURRT_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdarg.h>
#include <math.h>
#include <inttypes.h>
#include <time.h>
#include <ctype.h>
#include <errno.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <dirent.h>
#include <unistd.h>
#include <sys/wait.h>
#include <fcntl.h>

// ---- Value ------------------------------------------------------------

typedef enum { VUNIT, VBOOL, VINT, VFLOAT, VOBJ } ValType;

typedef struct Obj Obj;

typedef struct {
    ValType t;
    union {
        bool b;
        int64_t i;
        double f;
        Obj *o;
    } u;
} Value;

static inline Value bur_unit(void)      { Value v; v.t = VUNIT; v.u.o = NULL; return v; }
static inline Value bur_bool(bool b)    { Value v; v.t = VBOOL; v.u.b = b; return v; }
static inline Value bur_int(int64_t i)  { Value v; v.t = VINT; v.u.i = i; return v; }
static inline Value bur_float(double f) { Value v; v.t = VFLOAT; v.u.f = f; return v; }
static inline Value bur_obj(Obj *o)     { Value v; v.t = VOBJ; v.u.o = o; return v; }

// ---- heap objects -----------------------------------------------------

typedef enum {
    OBJ_STRING, OBJ_LIST, OBJ_MAP, OBJ_FUNC, OBJ_CLOSURE,
    OBJ_UPVALUE, OBJ_ENUMTYPE, OBJ_VARIANTCTOR, OBJ_ENUMINST, OBJ_NATIVE
} ObjType;

struct Obj {
    ObjType type;
    bool marked;
    Obj *next; // intrusive GC list
};

typedef struct {
    Obj obj;
    char *data; // may hold arbitrary bytes (UTF-8), not NUL-terminated
    int64_t len;
} OString;

typedef struct {
    Obj obj;
    Value *elems;
    int64_t len, cap;
} OList;

typedef struct {
    bool is_str;
    int64_t i;   // int key
    char *s;     // str key bytes (owned copy)
    int64_t slen;
} MapKey;

typedef struct {
    MapKey k;
    Value key, val;
} MapEntry;

typedef struct {
    Obj obj;
    MapEntry *entries; // insertion order
    int64_t len, cap;
    int *index;        // open-addressing: hash slot -> entry idx, -1 empty
    int64_t icap;
} OMap;

typedef struct {
    char *name;
    int arity;
} VariantInfo;

typedef struct Fiber Fiber;

typedef struct OFunc {
    Obj obj;
    const char *name;
    const char *file;
    int arity;
    int numUpvals;
    void (*code)(void); // compiled body; runs on the current fiber
    Value *consts;
    int nconsts;
} OFunc;

typedef struct OUpvalue {
    Obj obj;
    Fiber *fiber; // stack this upvalue references while open
    int slot;
    bool open;
    Value closed;
} OUpvalue;

struct OClosure {
    Obj obj;
    OFunc *fn;
    OUpvalue **upvals;
    int nupvals;
};

typedef struct OEnumType {
    Obj obj;
    const char *name;
    VariantInfo *variants;
    int nvariants;
} OEnumType;

typedef struct {
    Obj obj;
    OEnumType *enm;
    int idx;
} OVariantCtor;

typedef struct {
    Obj obj;
    OEnumType *enm;
    int variant;
    Value *fields;
    int nfields;
} OEnumInst;

typedef Value (*NativeFn)(Value *args, int argc);

typedef struct {
    Obj obj;
    const char *name;
    int arity; // -1 variadic
    NativeFn fn;
} ONative;

// ---- fiber (single, in the sequential core) ---------------------------

struct Fiber {
    Value *stack;
    int top, cap;
    OUpvalue **openUpvals;
    int nopen, opencap;
};

static Fiber *bur_cur;

// ---- runtime state ----------------------------------------------------

static Obj *bur_gc_head;
static int64_t bur_gc_count, bur_gc_threshold = 256, bur_gc_cycles, bur_gc_last_freed;
static bool bur_gc_ready; // collection disabled until boot completes

// the closure currently executing; generated functions snapshot this at
// entry to reach their upvalues (bur_call saves/restores it around calls)
typedef struct OClosure OClosure;
static OClosure *bur_cur_closure;

static OEnumType *bur_opt_enum, *bur_res_enum, *bur_out_enum;
static struct timespec bur_start_time;
static int bur_argc;
static char **bur_argv;

// ---- traps ------------------------------------------------------------

static void bur_trap(const char *fmt, ...) {
    fflush(stdout);
    va_list ap;
    va_start(ap, fmt);
    fputs("runtime error: ", stderr);
    vfprintf(stderr, fmt, ap);
    fputc('\n', stderr);
    va_end(ap);
    exit(4);
}

// ---- forward decls ----------------------------------------------------

typedef struct Buf Buf;
static void bur_gc_collect(void);
static void bur_format(Buf *b, Value v, bool quote);

// ---- dynamic byte buffer ----------------------------------------------

struct Buf {
    char *data;
    int64_t len, cap;
};

static void buf_reserve(Buf *b, int64_t extra) {
    if (b->len + extra <= b->cap) return;
    int64_t nc = b->cap * 2 + 64;
    while (nc < b->len + extra) nc *= 2;
    b->data = (char *)realloc(b->data, (size_t)nc);
    b->cap = nc;
}
static void buf_bytes(Buf *b, const char *s, int64_t n) {
    buf_reserve(b, n);
    memcpy(b->data + b->len, s, (size_t)n);
    b->len += n;
}
static void buf_str(Buf *b, const char *s) { buf_bytes(b, s, (int64_t)strlen(s)); }
static void buf_char(Buf *b, char c) { buf_reserve(b, 1); b->data[b->len++] = c; }
static void buf_free(Buf *b) { free(b->data); b->data = NULL; b->len = b->cap = 0; }

// ---- allocation + GC --------------------------------------------------

static Obj *bur_alloc(size_t size, ObjType type) {
    if (bur_gc_ready && bur_gc_count + 1 > bur_gc_threshold) {
        bur_gc_collect();
        bur_gc_threshold = bur_gc_count * 2 + 64;
    }
    Obj *o = (Obj *)calloc(1, size);
    o->type = type;
    o->marked = false;
    o->next = bur_gc_head;
    bur_gc_head = o;
    bur_gc_count++;
    return o;
}

static OString *bur_new_string_n(const char *s, int64_t n) {
    OString *o = (OString *)bur_alloc(sizeof(OString), OBJ_STRING);
    o->data = (char *)malloc((size_t)n + 1);
    memcpy(o->data, s, (size_t)n);
    o->data[n] = '\0';
    o->len = n;
    return o;
}
static OString *bur_new_string(const char *s) { return bur_new_string_n(s, (int64_t)strlen(s)); }

static OList *bur_new_list(Value *elems, int64_t n) {
    OList *o = (OList *)bur_alloc(sizeof(OList), OBJ_LIST);
    o->len = o->cap = n;
    if (n > 0) {
        o->elems = (Value *)malloc(sizeof(Value) * (size_t)n);
        memcpy(o->elems, elems, sizeof(Value) * (size_t)n);
    }
    return o;
}

static void list_push(OList *l, Value v) {
    if (l->len == l->cap) {
        l->cap = l->cap * 2 + 8;
        l->elems = (Value *)realloc(l->elems, sizeof(Value) * (size_t)l->cap);
    }
    l->elems[l->len++] = v;
}

static OEnumInst *bur_new_inst(OEnumType *e, int variant, Value *fields, int nfields) {
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

// stack roots (declared here, used by the collector)
static void bur_push(Value v);
static Value bur_pop(void);

static void bur_mark_value(Value v);

static Obj **bur_gray;
static int64_t bur_gray_len, bur_gray_cap;

static void bur_gray_push(Obj *o) {
    if (o == NULL || o->marked) return;
    o->marked = true;
    if (bur_gray_len == bur_gray_cap) {
        bur_gray_cap = bur_gray_cap * 2 + 64;
        bur_gray = (Obj **)realloc(bur_gray, sizeof(Obj *) * (size_t)bur_gray_cap);
    }
    bur_gray[bur_gray_len++] = o;
}
static void bur_mark_value(Value v) {
    if (v.t == VOBJ) bur_gray_push(v.u.o);
}

static void bur_gc_trace(Obj *o) {
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
    }
}

// globals table (declared early for GC roots)
typedef struct { char *key; int64_t klen; Value val; bool used; } GlobalSlot;
static GlobalSlot *bur_globals;
static int64_t bur_globals_cap, bur_globals_len;

// permanent roots: every compile-time constant object (strings, functions,
// enum types, constructors, singletons) is pinned here so it lives for the
// whole program, mirroring the VM keeping chunk constants reachable.
static Obj **bur_roots;
static int64_t bur_nroots, bur_rootcap;
static void bur_add_root(Obj *o) {
    if (bur_nroots == bur_rootcap) {
        bur_rootcap = bur_rootcap * 2 + 64;
        bur_roots = (Obj **)realloc(bur_roots, sizeof(Obj *) * (size_t)bur_rootcap);
    }
    bur_roots[bur_nroots++] = o;
}

static void bur_gc_collect(void) {
    bur_gc_cycles++;
    bur_gray_len = 0;

    // roots: permanent constants, globals, fiber stack, open upvalues
    for (int64_t i = 0; i < bur_nroots; i++) bur_gray_push(bur_roots[i]);
    for (int64_t i = 0; i < bur_globals_cap; i++)
        if (bur_globals[i].used) bur_mark_value(bur_globals[i].val);
    if (bur_cur) {
        for (int i = 0; i < bur_cur->top; i++) bur_mark_value(bur_cur->stack[i]);
        for (int i = 0; i < bur_cur->nopen; i++) bur_gray_push((Obj *)bur_cur->openUpvals[i]);
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

static void bur_push(Value v) {
    if (bur_cur->top == bur_cur->cap) {
        bur_cur->cap = bur_cur->cap * 2 + 64;
        bur_cur->stack = (Value *)realloc(bur_cur->stack, sizeof(Value) * (size_t)bur_cur->cap);
    }
    bur_cur->stack[bur_cur->top++] = v;
}
static Value bur_pop(void) { return bur_cur->stack[--bur_cur->top]; }
static Value bur_peek(int n) { return bur_cur->stack[bur_cur->top - 1 - n]; }

// ---- type names -------------------------------------------------------

static const char *bur_typename(Value v) {
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
        }
    }
    return "?";
}

// ---- equality ---------------------------------------------------------

static bool bur_obj_eq(Obj *a, Obj *b);

static bool bur_eq(Value a, Value b) {
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

static bool map_get(OMap *m, MapKey k, Value *out);
static bool mapkey_of(Value v, MapKey *out);

static bool bur_obj_eq(Obj *a, Obj *b) {
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
    default: return a == b;
    }
}

// ---- formatting -------------------------------------------------------

// Shortest round-tripping decimal, matching Go's strconv 'g' shortest for
// the magnitudes the sequential examples exercise.
static void bur_format_float(Buf *b, double f) {
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

static void bur_quote(Buf *b, const char *s, int64_t n) {
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

static void bur_format(Buf *b, Value v, bool quote) {
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
    default: return;
    }
}

// print a value the way print()/println() do (strings unquoted)
static void bur_write_display(Value v) {
    Buf b = {0};
    bur_format(&b, v, false);
    fwrite(b.data, 1, (size_t)b.len, stdout);
    buf_free(&b);
}

// ---- Option/Result/Output constructors (root fresh strings on the
// operand stack across allocations, mirroring the VM) -------------------

static Value bur_some(Value v) { return bur_obj((Obj *)bur_new_inst(bur_opt_enum, 0, &v, 1)); }
static Value bur_none(void)    { return bur_obj((Obj *)bur_new_inst(bur_opt_enum, 1, NULL, 0)); }
static Value bur_ok(Value v)   { return bur_obj((Obj *)bur_new_inst(bur_res_enum, 0, &v, 1)); }
static Value bur_err(Value v)  { return bur_obj((Obj *)bur_new_inst(bur_res_enum, 1, &v, 1)); }

static Value bur_ok_str(const char *s, int64_t n) {
    bur_push(bur_obj((Obj *)bur_new_string_n(s, n)));
    Value r = bur_ok(bur_peek(0));
    bur_pop();
    return r;
}
static Value bur_err_str(const char *s) {
    bur_push(bur_obj((Obj *)bur_new_string(s)));
    Value r = bur_err(bur_peek(0));
    bur_pop();
    return r;
}

// ---- map internals ----------------------------------------------------

static uint64_t mapkey_hash(MapKey k) {
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
static bool mapkey_eq(MapKey a, MapKey b) {
    if (a.is_str != b.is_str) return false;
    if (a.is_str) return a.slen == b.slen && memcmp(a.s, b.s, (size_t)a.slen) == 0;
    return a.i == b.i;
}
static bool mapkey_of(Value v, MapKey *out) {
    if (v.t == VINT) { out->is_str = false; out->i = v.u.i; out->s = NULL; out->slen = 0; return true; }
    if (v.t == VOBJ && v.u.o->type == OBJ_STRING) {
        OString *s = (OString *)v.u.o;
        out->is_str = true; out->s = s->data; out->slen = s->len; out->i = 0;
        return true;
    }
    return false;
}
static void map_reindex(OMap *m) {
    for (int64_t i = 0; i < m->icap; i++) m->index[i] = -1;
    for (int64_t e = 0; e < m->len; e++) {
        uint64_t h = mapkey_hash(m->entries[e].k) & (uint64_t)(m->icap - 1);
        while (m->index[h] != -1) h = (h + 1) & (uint64_t)(m->icap - 1);
        m->index[h] = (int)e;
    }
}
static bool map_get(OMap *m, MapKey k, Value *out) {
    if (m->icap == 0) return false;
    uint64_t h = mapkey_hash(k) & (uint64_t)(m->icap - 1);
    while (m->index[h] != -1) {
        if (mapkey_eq(m->entries[m->index[h]].k, k)) { *out = m->entries[m->index[h]].val; return true; }
        h = (h + 1) & (uint64_t)(m->icap - 1);
    }
    return false;
}
static void map_set(OMap *m, MapKey k, Value key, Value val) {
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
static void map_ensure(OMap *m) {
    if ((m->len + 1) * 4 >= m->icap * 3 || m->icap == 0) {
        m->icap = m->icap == 0 ? 16 : m->icap * 2;
        m->index = (int *)realloc(m->index, sizeof(int) * (size_t)m->icap);
        map_reindex(m);
    }
}
static void map_del(OMap *m, MapKey k) {
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

static Value bur_add(Value a, Value b) {
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

static Value bur_arith(Value a, Value b, char op) {
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

static Value bur_neg(Value v) {
    if (v.t == VINT) {
        if (v.u.i == INT64_MIN) bur_trap("integer overflow: -(%" PRId64 ")", v.u.i);
        return bur_int(-v.u.i);
    }
    if (v.t == VFLOAT) return bur_float(-v.u.f);
    bur_trap("operand of '-' must be a number, got %s", bur_typename(v));
    return bur_unit();
}

static Value bur_not(Value v) {
    if (v.t != VBOOL) bur_trap("operand of '!' must be a bool, got %s", bur_typename(v));
    return bur_bool(!v.u.b);
}

// comparison operators: 0=Gt 1=GtEq 2=Lt 3=LtEq
static Value bur_compare(Value a, Value b, int kind) {
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

static Value bur_index_get(Value target, Value idx) {
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
    bur_trap("cannot index %s", bur_typename(target));
    return bur_unit();
}

static void bur_index_set(Value target, Value idx, Value val) {
    if (target.t != VOBJ || target.u.o->type != OBJ_LIST)
        bur_trap("cannot index-assign into %s", bur_typename(target));
    OList *l = (OList *)target.u.o;
    if (idx.t != VINT) bur_trap("list index must be an int, got %s", bur_typename(idx));
    if (idx.u.i < 0 || idx.u.i >= l->len)
        bur_trap("list index %" PRId64 " out of bounds (len %" PRId64 ")", idx.u.i, l->len);
    l->elems[idx.u.i] = val;
}

static int64_t bur_len(Value v) {
    if (v.t == VOBJ) {
        if (v.u.o->type == OBJ_LIST) return ((OList *)v.u.o)->len;
        if (v.u.o->type == OBJ_STRING) return ((OString *)v.u.o)->len;
        if (v.u.o->type == OBJ_MAP) return ((OMap *)v.u.o)->len;
    }
    bur_trap("len() needs a list, string, or map, got %s", bur_typename(v));
    return 0;
}

// ---- upvalues ---------------------------------------------------------

static Value upvalue_get(OUpvalue *u) { return u->open ? u->fiber->stack[u->slot] : u->closed; }
static void upvalue_set(OUpvalue *u, Value v) {
    if (u->open) u->fiber->stack[u->slot] = v; else u->closed = v;
}

static OUpvalue *bur_capture_upvalue(int slot) {
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

static void bur_close_upvalues(int from) {
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

static int bur_call_depth;

// Invoke the value at peek(argc). Closures run their compiled body via the
// C call stack; natives and variant constructors are handled inline. The
// callee and its argc arguments occupy the top argc+1 stack slots and are
// replaced by the single result.
static void bur_call(int argc) {
    Value callee = bur_peek(argc);
    if (callee.t != VOBJ) bur_trap("cannot call %s", bur_typename(callee));
    switch (callee.u.o->type) {
    case OBJ_CLOSURE: {
        OClosure *cl = (OClosure *)callee.u.o;
        if (argc != cl->fn->arity)
            bur_trap("%s expects %d argument(s), got %d", cl->fn->name, cl->fn->arity, argc);
        if (++bur_call_depth > 2048) bur_trap("stack overflow (call depth > 2048)");
        OClosure *prev = bur_cur_closure;
        bur_cur_closure = cl;
        cl->fn->code();
        bur_cur_closure = prev;
        bur_call_depth--;
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

// ---- runtime bootstrap helpers ----------------------------------------

static void bur_globals_put(const char *key, int64_t klen, Value v);

static OClosure *bur_new_closure(OFunc *fn) {
    OClosure *cl = (OClosure *)bur_alloc(sizeof(OClosure), OBJ_CLOSURE);
    cl->fn = fn;
    cl->nupvals = fn->numUpvals;
    if (fn->numUpvals > 0)
        cl->upvals = (OUpvalue **)calloc((size_t)fn->numUpvals, sizeof(OUpvalue *));
    return cl;
}

// ---- globals table ----------------------------------------------------

static uint64_t str_hash(const char *s, int64_t n) {
    uint64_t h = 1469598103934665603ULL;
    for (int64_t i = 0; i < n; i++) { h ^= (unsigned char)s[i]; h *= 1099511628211ULL; }
    return h;
}
static void bur_globals_grow(void) {
    int64_t oldcap = bur_globals_cap;
    GlobalSlot *old = bur_globals;
    bur_globals_cap = oldcap == 0 ? 64 : oldcap * 2;
    bur_globals = (GlobalSlot *)calloc((size_t)bur_globals_cap, sizeof(GlobalSlot));
    bur_globals_len = 0;
    for (int64_t i = 0; i < oldcap; i++)
        if (old[i].used) bur_globals_put(old[i].key, old[i].klen, old[i].val);
    free(old);
}
static GlobalSlot *bur_globals_slot(const char *key, int64_t klen) {
    uint64_t h = str_hash(key, klen) & (uint64_t)(bur_globals_cap - 1);
    while (bur_globals[h].used) {
        if (bur_globals[h].klen == klen && memcmp(bur_globals[h].key, key, (size_t)klen) == 0)
            return &bur_globals[h];
        h = (h + 1) & (uint64_t)(bur_globals_cap - 1);
    }
    return &bur_globals[h];
}
static void bur_globals_put(const char *key, int64_t klen, Value v) {
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
static bool bur_globals_get(const char *key, int64_t klen, Value *out) {
    if (bur_globals_cap == 0) return false;
    GlobalSlot *s = bur_globals_slot(key, klen);
    if (!s->used) return false;
    *out = s->val;
    return true;
}

// opcode helpers used directly by generated code
static Value bur_get_global(const char *name, int64_t n) {
    Value v;
    if (!bur_globals_get(name, n, &v)) bur_trap("undefined variable \"%.*s\"", (int)n, name);
    return v;
}
static void bur_set_global(const char *name, int64_t n, Value v) {
    Value old;
    if (!bur_globals_get(name, n, &old)) bur_trap("undefined variable \"%.*s\"", (int)n, name);
    bur_globals_put(name, n, v);
}

#include "burrt_natives.h"

// ---- boot -------------------------------------------------------------

static void bur_boot(int argc, char **argv) {
    bur_argc = argc;
    bur_argv = argv;
    clock_gettime(CLOCK_MONOTONIC, &bur_start_time);
    setvbuf(stdout, NULL, _IOFBF, 1 << 16);
    bur_cur = (Fiber *)calloc(1, sizeof(Fiber));
    bur_cur->cap = 256;
    bur_cur->stack = (Value *)malloc(sizeof(Value) * 256);
}

#endif // BURRT_H
