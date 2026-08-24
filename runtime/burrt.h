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

// macOS hides the deprecated ucontext routines unless _XOPEN_SOURCE is set.
#if defined(__APPLE__) && !defined(_XOPEN_SOURCE)
#define _XOPEN_SOURCE 600
#endif

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

#ifdef _WIN32
#ifndef _CRT_SECURE_NO_WARNINGS
#define _CRT_SECURE_NO_WARNINGS // strerror and friends are fine here
#endif
#ifndef _WIN32_WINNT
#define _WIN32_WINNT 0x0600 // WSAPoll and friends
#endif
#ifndef WIN32_LEAN_AND_MEAN
#define WIN32_LEAN_AND_MEAN
#endif
#ifndef NOMINMAX
#define NOMINMAX
#endif
#include <winsock2.h> // must precede windows.h
#include <ws2tcpip.h>
#include <windows.h>
#include <sys/stat.h> // _stat64 for file_exists
#else
#include <sys/stat.h>
#include <sys/types.h>
#include <dirent.h>
#include <unistd.h>
#include <sys/wait.h>
#include <fcntl.h>
#include <poll.h>
#include <sys/socket.h>
#include <netdb.h>
#include <ucontext.h>
#endif

// the scheduler context: ucontext on POSIX/macOS, a Win32 fiber handle on Windows
#ifdef _WIN32
typedef void *BurFiberCtx;
#else
typedef ucontext_t BurFiberCtx;
#endif

// CLOCK_MONOTONIC as nanoseconds; QueryPerformanceCounter-backed on Windows.
// The split multiply keeps full precision without overflowing int64 uptime.
static inline int64_t bur_mono_ns(void) {
#ifdef _WIN32
    LARGE_INTEGER f, c;
    QueryPerformanceFrequency(&f);
    QueryPerformanceCounter(&c);
    return (int64_t)((uint64_t)c.QuadPart / (uint64_t)f.QuadPart * 1000000000ull +
                     (uint64_t)c.QuadPart % (uint64_t)f.QuadPart * 1000000000ull / (uint64_t)f.QuadPart);
#else
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return ts.tv_sec * 1000000000 + ts.tv_nsec;
#endif
}

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
    OBJ_UPVALUE, OBJ_ENUMTYPE, OBJ_VARIANTCTOR, OBJ_ENUMINST, OBJ_NATIVE,
    OBJ_CHANNEL, OBJ_RECORD, OBJ_TUPLE
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

typedef struct {
    Obj obj;
    OString **names;
    Value *fields;
    int nfields;
} ORecord;

typedef struct {
    Obj obj;
    Value *elems;
    int64_t n;
} OTuple;

typedef Value (*NativeFn)(Value *args, int argc);

typedef struct {
    Obj obj;
    const char *name;
    int arity; // -1 variadic
    NativeFn fn;
} ONative;

// ---- fibers & channels (concurrency core) -----------------------------
//
// Each fiber owns a suspended native call stack of its own: swapcontext on
// POSIX/macOS, a Win32 fiber on Windows. Blocking a fiber swaps its whole
// native stack out to the scheduler, and resuming swaps it back. This
// mirrors the Go VM's cooperative, single-threaded scheduler (vm.go): a
// FIFO ready queue, park/wake on channels, and deterministic interleaving
// — never OS threads.

typedef struct OChannel OChannel;
typedef struct OClosure OClosure; // full struct is defined above; alias needed here

typedef enum {
    FREADY, FBLOCKED_SEND, FBLOCKED_RECV, FBLOCKED_SELECT, FDONE, FBLOCKED_TIMER,
    FBLOCKED_IO
} FiberStatus;

struct Fiber {
    Value *stack;
    int top, cap;
    OUpvalue **openUpvals;
    int nopen, opencap;

    int id;
    FiberStatus status;
    Value sendVal;          // pending value while blocked on send
    OClosure *entry;        // closure the fiber runs on first resume
    int call_depth;         // per-fiber call depth, for stack-overflow trapping
    OFunc **trace_fn;       // per-depth function, for trap stack traces
    int *trace_ln;          // per-depth live source line (BUR_LN stores)
    int trace_cap;
    int budget;             // instructions remaining before a forced yield
    int64_t wake_ns;        // absolute CLOCK_MONOTONIC deadline while FBLOCKED_TIMER
    int64_t io_proc;        // process handle awaited while FBLOCKED_IO
    intptr_t io_fd;         // descriptor awaited while FBLOCKED_IO
    short io_events;        // poll events requested for io_fd
    bool io_ready;          // scheduler observed readiness for io_fd

    Value *defers;          // registered defer closures; frames delimit their
    int ndefers, defercap;  // slice by watermark and pop it LIFO on exit

    BurFiberCtx ctx;        // suspended fiber state (ucontext / Win32 fiber)
#ifndef _WIN32
    char *cstack;           // heap-allocated native stack backing ctx
#endif

    OChannel **selectChans; // channels this fiber waits on while parked in select
    int nselect, selectcap;
};

// a channel: a bounded FIFO buffer plus queues of fibers blocked sending,
// blocked receiving, or parked in a select waiting for any state change
struct OChannel {
    Obj obj;
    int cap;
    Value *buf;
    int buflen, bufcap;
    Fiber **sendq; int nsendq, sendqcap;
    Fiber **recvq; int nrecvq, recvqcap;
    Fiber **waiters; int nwait, waitcap;
    bool closed;
};


// ---- cross-unit declarations (multi-file runtime) ----

#define BUR_TIMESLICE 10000            // instructions per fiber turn (matches vm.go)
#define BUR_STACK_SIZE (1 << 20)       // 1 MiB per spawned fiber
#define BUR_MAIN_STACK_SIZE (8 << 20)  // 8 MiB for the main fiber (deep recursion)
// BUR_LN: generated code records the live source line of the running
// frame before each instruction; bur_trap walks these for the trace.
#define BUR_LN(n) (bur_cur->trace_ln[bur_cur->call_depth] = (n))
typedef struct Buf Buf;
struct Buf { char *data; int64_t len, cap; };
typedef struct { char *key; int64_t klen; Value val; bool used; } GlobalSlot;
typedef struct {
    intptr_t pid;                  // child pid (POSIX) / process handle (Windows)
    intptr_t outfd, errfd, failfd; // parent read ends, -1 once closed
    Buf ob, eb;                    // collected stdout/stderr
    int child_err;                 // errno from a failed exec, via failfd (POSIX)
    bool have_err;
    int code;                   // exit code once reaped
    bool complete;              // pipes drained + child reaped
    bool consumed;              // result already handed out
    bool used;                  // slot allocated
} BurProc;
typedef void (*BurWaitReadyFn)(int64_t owner, short revents);
typedef struct {
#ifdef _WIN32
    WSAPOLLFD *fds;
#else
    struct pollfd *fds;
#endif
    int64_t *owners;
    BurWaitReadyFn *ready;
    int n, cap;
    int64_t deadline_ns;
} BurWaitSet;

extern Fiber *bur_cur;
extern Fiber *bur_main_fiber;
extern BurFiberCtx bur_sched_ctx;
extern Fiber **bur_fibers;
extern int64_t bur_nfibers, bur_fiberscap;
extern Fiber **bur_ready;
extern int64_t bur_ready_head, bur_ready_len, bur_ready_cap;
extern int bur_next_fiber_id;
extern int64_t bur_ntimers;
extern int64_t bur_nio;
extern bool bur_deterministic;
extern Obj *bur_gc_head;
extern int64_t bur_gc_count, bur_gc_threshold, bur_gc_cycles, bur_gc_last_freed;
extern bool bur_gc_ready;
extern OClosure *bur_cur_closure;
extern OEnumType *bur_opt_enum, *bur_res_enum, *bur_out_enum;
extern int64_t bur_start_ns; // CLOCK_MONOTONIC at boot
extern int bur_argc;
extern char **bur_argv;
extern void bur_trap(const char *fmt, ...) ;
extern void bur_gc_collect(void);
extern void bur_format(Buf *b, Value v, bool quote);
extern void buf_reserve(Buf *b, int64_t extra) ;
extern void buf_bytes(Buf *b, const char *s, int64_t n) ;
extern void buf_str(Buf *b, const char *s) ;
extern void buf_char(Buf *b, char c) ;
extern void buf_free(Buf *b) ;
extern Obj *bur_alloc(size_t size, ObjType type) ;
extern OString *bur_new_string_n(const char *s, int64_t n) ;
extern OString *bur_new_string(const char *s) ;
extern OList *bur_new_list(Value *elems, int64_t n) ;
extern void list_push(OList *l, Value v) ;
extern OEnumInst *bur_new_inst(OEnumType *e, int variant, Value *fields, int nfields) ;
extern ORecord *bur_new_record(OString **names, Value *fields, int nfields) ;
extern OTuple *bur_new_tuple(Value *elems, int64_t n) ;
extern void bur_push(Value v);
extern Value bur_pop(void);
extern void bur_trap(const char *fmt, ...);
extern void bur_record_make(int n, const char *names_enc) ;
extern void bur_tuple_make(int n) ;
extern void bur_record_get(const char *fname) ;
extern void bur_record_update(int n, const char *names_enc) ;
extern void bur_push(Value v);
extern Value bur_pop(void);
extern void bur_mark_value(Value v);
extern Obj **bur_gray;
extern int64_t bur_gray_len, bur_gray_cap;
extern void bur_gray_push(Obj *o) ;
extern void bur_mark_value(Value v) ;
extern void bur_gc_trace(Obj *o) ;
extern GlobalSlot *bur_globals;
extern int64_t bur_globals_cap, bur_globals_len;
extern Obj **bur_roots;
extern int64_t bur_nroots, bur_rootcap;
extern void bur_add_root(Obj *o) ;
extern void bur_gc_collect(void) ;
extern void bur_push(Value v) ;
extern Value bur_pop(void) ;
extern Value bur_peek(int n) ;
extern const char *bur_typename(Value v) ;
extern bool bur_obj_eq(Obj *a, Obj *b);
extern bool bur_eq(Value a, Value b) ;
extern bool map_get(OMap *m, MapKey k, Value *out);
extern bool mapkey_of(Value v, MapKey *out);
extern bool bur_obj_eq(Obj *a, Obj *b) ;
extern void bur_format_float(Buf *b, double f) ;
extern void bur_quote(Buf *b, const char *s, int64_t n) ;
extern void bur_format(Buf *b, Value v, bool quote) ;
extern void bur_write_display(Value v) ;
extern Value bur_some(Value v) ;
extern Value bur_none(void)    ;
extern Value bur_ok(Value v)   ;
extern Value bur_err(Value v)  ;
extern Value bur_ok_str(const char *s, int64_t n) ;
extern Value bur_err_str(const char *s) ;
extern uint64_t mapkey_hash(MapKey k) ;
extern bool mapkey_eq(MapKey a, MapKey b) ;
extern bool mapkey_of(Value v, MapKey *out) ;
extern void map_reindex(OMap *m) ;
extern bool map_get(OMap *m, MapKey k, Value *out) ;
extern void map_set(OMap *m, MapKey k, Value key, Value val) ;
extern void map_ensure(OMap *m) ;
extern void map_del(OMap *m, MapKey k) ;
extern Value bur_add(Value a, Value b) ;
extern Value bur_arith(Value a, Value b, char op) ;
extern Value bur_neg(Value v) ;
extern Value bur_not(Value v) ;
extern Value bur_compare(Value a, Value b, int kind) ;
extern Value bur_index_get(Value target, Value idx) ;
extern void bur_index_set(Value target, Value idx, Value val) ;
extern int64_t bur_len(Value v) ;
extern Value upvalue_get(OUpvalue *u) ;
extern void upvalue_set(OUpvalue *u, Value v) ;
extern OUpvalue *bur_capture_upvalue(int slot) ;
extern OUpvalue *bur_new_closed_cell(Value v) ;
extern OUpvalue *bur_capture_value(int slot) ;
extern void bur_close_upvalues(int from) ;
extern void bur_call(int argc) ;
extern void bur_defer_push(Value cl) ;
extern void bur_run_defers(int dbase) ;
extern void bur_globals_put(const char *key, int64_t klen, Value v);
extern OClosure *bur_new_closure(OFunc *fn) ;
extern uint64_t str_hash(const char *s, int64_t n) ;
extern void bur_globals_grow(void) ;
extern GlobalSlot *bur_globals_slot(const char *key, int64_t klen) ;
extern void bur_globals_put(const char *key, int64_t klen, Value v) ;
extern bool bur_globals_get(const char *key, int64_t klen, Value *out) ;
extern Value bur_get_global(const char *name, int64_t n) ;
extern void bur_set_global(const char *name, int64_t n, Value v) ;
extern void bur_fiber_entry(void);
extern void fq_push(Fiber ***a, int *n, int *cap, Fiber *f) ;
extern Fiber *fq_pop(Fiber ***a, int *n) ;
extern void chan_buf_push(OChannel *ch, Value v) ;
extern Value chan_buf_pop(OChannel *ch) ;
extern void bur_ready_push(Fiber *f) ;
extern Fiber *bur_ready_pop(void) ;
extern void bur_fibers_push(Fiber *f) ;
extern void bur_schedule(Fiber *f) ;
extern void bur_wake_waiters(OChannel *ch) ;
extern void bur_remove_waiter(OChannel *ch, Fiber *f) ;
extern void bur_clear_select(Fiber *f) ;
extern void bur_select_add(Fiber *f, OChannel *ch) ;
extern bool chan_recv_ready(OChannel *ch) ;
extern bool chan_send_ready(OChannel *ch) ;
extern bool chan_try_recv(OChannel *ch, Value *out) ;
extern void bur_park(FiberStatus st) ;
extern void bur_wait_current_fd(intptr_t fd, short events) ;
extern void bur_switch_to_sched(void) ;
extern void bur_preempt(void) ;
extern OChannel *as_channel_opt(Value v) ;
extern Fiber *bur_new_fiber(OClosure *cl, Value *args, int argc, size_t stacksize) ;
extern void bur_fiber_entry(void) ;
extern int64_t bur_now_ns(void) ;
extern BurProc *bur_procs;
extern int64_t bur_nprocs, bur_procscap;
extern BurWaitSet bur_waitset;
extern void bur_wait_reset(void) ;
extern void bur_wait_fd(intptr_t fd, short events, int64_t owner, BurWaitReadyFn ready) ;
extern void bur_wait_timer(int64_t deadline_ns) ;
extern bool bur_proc_valid(int64_t h) ;
extern void bur_proc_pump(BurProc *p) ;
extern void bur_proc_ready(int64_t owner, short revents) ;
extern void bur_fiber_fd_ready(int64_t owner, short revents) ;
extern void bur_wait_poll(bool block) ;
extern void bur_wake_due_timers(void) ;
extern void bur_scheduler(void) ;
extern void bur_spawn(int argc) ;
extern void bur_send(Value chv, Value val) ;
extern Value bur_recv(Value chv) ;
extern bool bur_chan_next(Value chv, Value *out) ;
extern int bur_select(const unsigned char *kinds, int nArms, bool hasDefault) ;
extern void bur_boot(int argc, char **argv) ;

#include "burrt_natives.h"

#endif // BURRT_H
