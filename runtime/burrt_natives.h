// Burryn C runtime — sequential native functions.
//
// Included from burrt.h after the value model, GC, and helpers are defined.
// Each native mirrors its Go counterpart in builtins.go; allocation-heavy
// ones root fresh objects on the operand stack across collections exactly
// as the VM does. Channel/yield natives belong to the concurrency core and
// are added there.
#ifndef BURRT_NATIVES_H
#define BURRT_NATIVES_H

// ---- accessors --------------------------------------------------------

static bool nat_as_str(Value v, const char **s, int64_t *n) {
    if (v.t == VOBJ && v.u.o->type == OBJ_STRING) {
        OString *o = (OString *)v.u.o;
        *s = o->data; *n = o->len; return true;
    }
    return false;
}
static OList *nat_as_list(Value v) {
    return (v.t == VOBJ && v.u.o->type == OBJ_LIST) ? (OList *)v.u.o : NULL;
}
static OMap *nat_as_map(Value v) {
    return (v.t == VOBJ && v.u.o->type == OBJ_MAP) ? (OMap *)v.u.o : NULL;
}

// ---- io ---------------------------------------------------------------

static void nat_write_joined(Value *args, int argc) {
    for (int i = 0; i < argc; i++) {
        if (i > 0) fputc(' ', stdout);
        bur_write_display(args[i]);
    }
}
static Value nat_print(Value *args, int argc) { nat_write_joined(args, argc); return bur_unit(); }
static Value nat_println(Value *args, int argc) { nat_write_joined(args, argc); fputc('\n', stdout); return bur_unit(); }

// ---- collections ------------------------------------------------------

static Value nat_len(Value *args, int argc) { (void)argc; return bur_int(bur_len(args[0])); }

static Value nat_map(Value *args, int argc) {
    (void)args; (void)argc;
    OMap *m = (OMap *)bur_alloc(sizeof(OMap), OBJ_MAP);
    return bur_obj((Obj *)m);
}
static Value nat_get(Value *args, int argc) {
    (void)argc;
    OMap *m = nat_as_map(args[0]);
    if (!m) bur_trap("get() needs a map, got %s", bur_typename(args[0]));
    MapKey k;
    if (!mapkey_of(args[1], &k)) bur_trap("map keys must be int or str, got %s", bur_typename(args[1]));
    Value v;
    if (map_get(m, k, &v)) return bur_some(v);
    return bur_none();
}
static Value nat_put(Value *args, int argc) {
    (void)argc;
    OMap *m = nat_as_map(args[0]);
    if (!m) bur_trap("put() needs a map, got %s", bur_typename(args[0]));
    MapKey k;
    if (!mapkey_of(args[1], &k)) bur_trap("map keys must be int or str, got %s", bur_typename(args[1]));
    map_ensure(m);
    map_set(m, k, args[1], args[2]);
    return bur_unit();
}
static Value nat_delete(Value *args, int argc) {
    (void)argc;
    OMap *m = nat_as_map(args[0]);
    if (!m) bur_trap("delete() needs a map, got %s", bur_typename(args[0]));
    MapKey k;
    if (!mapkey_of(args[1], &k)) bur_trap("map keys must be int or str, got %s", bur_typename(args[1]));
    map_del(m, k);
    return bur_unit();
}
static Value nat_keys(Value *args, int argc) {
    (void)argc;
    OMap *m = nat_as_map(args[0]);
    if (!m) bur_trap("keys() needs a map, got %s", bur_typename(args[0]));
    OList *l = bur_new_list(NULL, 0);
    bur_push(bur_obj((Obj *)l));
    for (int64_t i = 0; i < m->len; i++) list_push(l, m->entries[i].key);
    bur_pop();
    return bur_obj((Obj *)l);
}
static Value nat_push(Value *args, int argc) {
    (void)argc;
    OList *l = nat_as_list(args[0]);
    if (!l) bur_trap("push() needs a list, got %s", bur_typename(args[0]));
    list_push(l, args[1]);
    return bur_unit();
}
static Value nat_pop(Value *args, int argc) {
    (void)argc;
    OList *l = nat_as_list(args[0]);
    if (!l) bur_trap("pop() needs a list, got %s", bur_typename(args[0]));
    if (l->len == 0) bur_trap("pop() on empty list");
    return l->elems[--l->len];
}
static Value nat_range(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VINT || args[1].t != VINT) bur_trap("range() needs two ints");
    OList *l = bur_new_list(NULL, 0);
    bur_push(bur_obj((Obj *)l));
    for (int64_t i = args[0].u.i; i < args[1].u.i; i++) list_push(l, bur_int(i));
    bur_pop();
    return bur_obj((Obj *)l);
}
static Value nat_slice(Value *args, int argc) {
    (void)argc;
    OList *l = nat_as_list(args[0]);
    if (!l || args[1].t != VINT || args[2].t != VINT) bur_trap("slice() needs ([a], int, int)");
    int64_t start = args[1].u.i, end = args[2].u.i;
    if (start < 0 || end < start || end > l->len)
        bur_trap("slice(%" PRId64 ", %" PRId64 ") out of bounds (len %" PRId64 ")", start, end, l->len);
    return bur_obj((Obj *)bur_new_list(l->elems + start, end - start));
}
static Value nat_concat(Value *args, int argc) {
    (void)argc;
    OList *x = nat_as_list(args[0]), *y = nat_as_list(args[1]);
    if (!x || !y) bur_trap("concat() needs ([a], [a])");
    OList *l = bur_new_list(x->elems, x->len);
    bur_push(bur_obj((Obj *)l));
    for (int64_t i = 0; i < y->len; i++) list_push(l, y->elems[i]);
    bur_pop();
    return bur_obj((Obj *)l);
}
static Value nat_contains(Value *args, int argc) {
    (void)argc;
    OList *l = nat_as_list(args[0]);
    if (!l) bur_trap("contains() needs a list, got %s", bur_typename(args[0]));
    for (int64_t i = 0; i < l->len; i++)
        if (bur_eq(l->elems[i], args[1])) return bur_bool(true);
    return bur_bool(false);
}

// ---- conversions & numbers --------------------------------------------

static Value nat_str(Value *args, int argc) {
    (void)argc;
    Buf b = {0};
    bur_format(&b, args[0], false);
    Value r = bur_obj((Obj *)bur_new_string_n(b.data ? b.data : "", b.len));
    buf_free(&b);
    return r;
}
static Value nat_trunc(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VFLOAT) bur_trap("trunc() needs a float, got %s", bur_typename(args[0]));
    return bur_int((int64_t)args[0].u.f);
}
static Value nat_to_float(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VINT) bur_trap("to_float() needs an int, got %s", bur_typename(args[0]));
    return bur_float((double)args[0].u.i);
}
static Value nat_float_bits(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VFLOAT) bur_trap("float_bits() needs a float, got %s", bur_typename(args[0]));
    uint64_t bits;
    memcpy(&bits, &args[0].u.f, sizeof(bits));
    return bur_int((int64_t)bits);
}
static Value nat_parse_int(Value *args, int argc) {
    (void)argc;
    const char *s; int64_t n;
    if (!nat_as_str(args[0], &s, &n)) bur_trap("parse_int() needs a str, got %s", bur_typename(args[0]));
    char buf[64];
    // trim leading/trailing ASCII space, mirroring strings.TrimSpace enough
    int64_t i = 0, j = n;
    while (i < j && isspace((unsigned char)s[i])) i++;
    while (j > i && isspace((unsigned char)s[j - 1])) j--;
    if (j - i == 0 || j - i >= (int64_t)sizeof buf) return bur_none();
    memcpy(buf, s + i, (size_t)(j - i));
    buf[j - i] = '\0';
    char *end;
    errno = 0;
    long long v = strtoll(buf, &end, 10);
    if (errno != 0 || *end != '\0') return bur_none();
    return bur_some(bur_int((int64_t)v));
}
static Value nat_parse_float(Value *args, int argc) {
    (void)argc;
    const char *s; int64_t n;
    if (!nat_as_str(args[0], &s, &n)) bur_trap("parse_float() needs a str, got %s", bur_typename(args[0]));
    char buf[128];
    int64_t i = 0, j = n;
    while (i < j && isspace((unsigned char)s[i])) i++;
    while (j > i && isspace((unsigned char)s[j - 1])) j--;
    if (j - i == 0 || j - i >= (int64_t)sizeof buf) return bur_none();
    memcpy(buf, s + i, (size_t)(j - i));
    buf[j - i] = '\0';
    char *end;
    errno = 0;
    double v = strtod(buf, &end);
    if (*end != '\0') return bur_none();
    return bur_some(bur_float(v));
}

// ---- strings ----------------------------------------------------------

static Value nat_str_len(Value *args, int argc) {
    (void)argc;
    const char *s; int64_t n;
    if (!nat_as_str(args[0], &s, &n)) bur_trap("str_len() needs a str, got %s", bur_typename(args[0]));
    return bur_int(n);
}
static Value nat_char_at(Value *args, int argc) {
    (void)argc;
    const char *s; int64_t n;
    if (!nat_as_str(args[0], &s, &n) || args[1].t != VINT) bur_trap("char_at() needs (str, int)");
    int64_t i = args[1].u.i;
    if (i < 0 || i >= n) bur_trap("char_at index %" PRId64 " out of bounds (len %" PRId64 ")", i, n);
    return bur_obj((Obj *)bur_new_string_n(s + i, 1));
}
static Value nat_split(Value *args, int argc) {
    (void)argc;
    const char *s, *sep; int64_t n, sn;
    if (!nat_as_str(args[0], &s, &n) || !nat_as_str(args[1], &sep, &sn)) bur_trap("split() needs (str, str)");
    OList *l = bur_new_list(NULL, 0);
    bur_push(bur_obj((Obj *)l));
    if (sn == 0) {
        // strings.Split on "" splits into UTF-8-agnostic single bytes here is
        // not what Go does; Go splits into runes. Sequential examples never
        // pass an empty separator, so mirror the common path: whole string.
        list_push(l, bur_obj((Obj *)bur_new_string_n(s, n)));
    } else {
        int64_t start = 0;
        for (int64_t i = 0; i + sn <= n;) {
            if (memcmp(s + i, sep, (size_t)sn) == 0) {
                list_push(l, bur_obj((Obj *)bur_new_string_n(s + start, i - start)));
                i += sn;
                start = i;
            } else {
                i++;
            }
        }
        list_push(l, bur_obj((Obj *)bur_new_string_n(s + start, n - start)));
    }
    bur_pop();
    return bur_obj((Obj *)l);
}
static Value nat_join(Value *args, int argc) {
    (void)argc;
    OList *l = nat_as_list(args[0]);
    const char *sep; int64_t sn;
    if (!l || !nat_as_str(args[1], &sep, &sn)) bur_trap("join() needs ([str], str)");
    Buf b = {0};
    for (int64_t i = 0; i < l->len; i++) {
        const char *p; int64_t pn;
        if (!nat_as_str(l->elems[i], &p, &pn)) { buf_free(&b); bur_trap("join() needs a list of str"); }
        if (i > 0) buf_bytes(&b, sep, sn);
        buf_bytes(&b, p, pn);
    }
    Value r = bur_obj((Obj *)bur_new_string_n(b.data ? b.data : "", b.len));
    buf_free(&b);
    return r;
}
static Value nat_substr(Value *args, int argc) {
    (void)argc;
    const char *s; int64_t n;
    if (!nat_as_str(args[0], &s, &n) || args[1].t != VINT || args[2].t != VINT) bur_trap("substr() needs (str, int, int)");
    int64_t start = args[1].u.i, cnt = args[2].u.i;
    if (start < 0 || cnt < 0 || start + cnt > n)
        bur_trap("substr(%" PRId64 ", %" PRId64 ") out of bounds (len %" PRId64 ")", start, cnt, n);
    return bur_obj((Obj *)bur_new_string_n(s + start, cnt));
}
static Value nat_str_contains(Value *args, int argc) {
    (void)argc;
    const char *s, *sub; int64_t n, sln;
    if (!nat_as_str(args[0], &s, &n) || !nat_as_str(args[1], &sub, &sln)) bur_trap("str_contains() needs (str, str)");
    if (sln == 0) return bur_bool(true);
    for (int64_t i = 0; i + sln <= n; i++)
        if (memcmp(s + i, sub, (size_t)sln) == 0) return bur_bool(true);
    return bur_bool(false);
}
static Value nat_str_index_of(Value *args, int argc) {
    (void)argc;
    const char *s, *sub; int64_t n, sln;
    if (!nat_as_str(args[0], &s, &n) || !nat_as_str(args[1], &sub, &sln)) bur_trap("str_index_of() needs (str, str)");
    if (sln == 0) return bur_some(bur_int(0));
    for (int64_t i = 0; i + sln <= n; i++)
        if (memcmp(s + i, sub, (size_t)sln) == 0) return bur_some(bur_int(i));
    return bur_none();
}
static Value nat_trim(Value *args, int argc) {
    (void)argc;
    const char *s; int64_t n;
    if (!nat_as_str(args[0], &s, &n)) bur_trap("trim() needs a str, got %s", bur_typename(args[0]));
    int64_t i = 0, j = n;
    while (i < j && isspace((unsigned char)s[i])) i++;
    while (j > i && isspace((unsigned char)s[j - 1])) j--;
    return bur_obj((Obj *)bur_new_string_n(s + i, j - i));
}
static Value nat_chr(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VINT || args[0].u.i < 0 || args[0].u.i > 0x10ffff) bur_trap("chr() needs an int code point");
    // encode the code point as UTF-8
    int64_t cp = args[0].u.i;
    char buf[4]; int len;
    if (cp < 0x80) { buf[0] = (char)cp; len = 1; }
    else if (cp < 0x800) { buf[0] = (char)(0xC0 | (cp >> 6)); buf[1] = (char)(0x80 | (cp & 0x3F)); len = 2; }
    else if (cp < 0x10000) { buf[0] = (char)(0xE0 | (cp >> 12)); buf[1] = (char)(0x80 | ((cp >> 6) & 0x3F)); buf[2] = (char)(0x80 | (cp & 0x3F)); len = 3; }
    else { buf[0] = (char)(0xF0 | (cp >> 18)); buf[1] = (char)(0x80 | ((cp >> 12) & 0x3F)); buf[2] = (char)(0x80 | ((cp >> 6) & 0x3F)); buf[3] = (char)(0x80 | (cp & 0x3F)); len = 4; }
    return bur_obj((Obj *)bur_new_string_n(buf, len));
}
static Value nat_ord(Value *args, int argc) {
    (void)argc;
    const char *s; int64_t n;
    if (!nat_as_str(args[0], &s, &n) || n == 0) bur_trap("ord() needs a non-empty string");
    // decode the first UTF-8 code point
    unsigned char c = (unsigned char)s[0];
    int64_t cp; int extra;
    if (c < 0x80) { cp = c; extra = 0; }
    else if ((c & 0xE0) == 0xC0) { cp = c & 0x1F; extra = 1; }
    else if ((c & 0xF0) == 0xE0) { cp = c & 0x0F; extra = 2; }
    else { cp = c & 0x07; extra = 3; }
    for (int k = 1; k <= extra && k < n; k++) cp = (cp << 6) | ((unsigned char)s[k] & 0x3F);
    return bur_int(cp);
}

// ---- filesystem & process ---------------------------------------------

static Value nat_read_file(Value *args, int argc) {
    (void)argc;
    const char *path; int64_t pn;
    if (!nat_as_str(args[0], &path, &pn)) bur_trap("read_file() needs a str, got %s", bur_typename(args[0]));
    FILE *fp = fopen(path, "rb");
    if (!fp) return bur_err_str(strerror(errno));
    Buf b = {0};
    char chunk[8192]; size_t r;
    while ((r = fread(chunk, 1, sizeof chunk, fp)) > 0) buf_bytes(&b, chunk, (int64_t)r);
    fclose(fp);
    Value res = bur_ok_str(b.data ? b.data : "", b.len);
    buf_free(&b);
    return res;
}
static Value nat_write_file(Value *args, int argc) {
    (void)argc;
    const char *path, *contents; int64_t pn, cn;
    if (!nat_as_str(args[0], &path, &pn) || !nat_as_str(args[1], &contents, &cn)) bur_trap("write_file() needs (str, str)");
    FILE *fp = fopen(path, "wb");
    if (!fp) return bur_err_str(strerror(errno));
    if (cn > 0 && fwrite(contents, 1, (size_t)cn, fp) != (size_t)cn) { fclose(fp); return bur_err_str(strerror(errno)); }
    fclose(fp);
    return bur_ok(bur_unit());
}
static Value nat_file_exists(Value *args, int argc) {
    (void)argc;
    const char *path; int64_t pn;
    if (!nat_as_str(args[0], &path, &pn)) bur_trap("file_exists() needs a str, got %s", bur_typename(args[0]));
    struct stat st;
    return bur_bool(stat(path, &st) == 0);
}
static int nat_strcmp_qsort(const void *a, const void *b) {
    return strcmp(*(const char *const *)a, *(const char *const *)b);
}
static Value nat_read_dir(Value *args, int argc) {
    (void)argc;
    const char *path; int64_t pn;
    if (!nat_as_str(args[0], &path, &pn)) bur_trap("read_dir() needs a str, got %s", bur_typename(args[0]));
    DIR *d = opendir(path);
    if (!d) return bur_err_str(strerror(errno));
    char **names = NULL; int64_t count = 0, cap = 0;
    struct dirent *e;
    while ((e = readdir(d))) {
        if (strcmp(e->d_name, ".") == 0 || strcmp(e->d_name, "..") == 0) continue;
        if (count == cap) { cap = cap * 2 + 16; names = (char **)realloc(names, sizeof(char *) * (size_t)cap); }
        names[count++] = strdup(e->d_name);
    }
    closedir(d);
    qsort(names, (size_t)count, sizeof(char *), nat_strcmp_qsort); // os.ReadDir sorts by name
    OList *l = bur_new_list(NULL, 0);
    bur_push(bur_obj((Obj *)l));
    for (int64_t i = 0; i < count; i++) { list_push(l, bur_obj((Obj *)bur_new_string(names[i]))); free(names[i]); }
    free(names);
    Value res = bur_ok(bur_peek(0));
    bur_pop();
    return res;
}

// build Ok(Output(code, stdout, stderr)), rooting each fresh string
static Value nat_output(int code, const char *out, int64_t outn, const char *err, int64_t errn) {
    bur_push(bur_obj((Obj *)bur_new_string_n(out, outn))); // peek(1)
    bur_push(bur_obj((Obj *)bur_new_string_n(err, errn))); // peek(0)
    Value fields[3] = { bur_int(code), bur_peek(1), bur_peek(0) };
    OEnumInst *o = bur_new_inst(bur_out_enum, 0, fields, 3);
    bur_pop();                            // drop stderr root (kept via o)
    bur_cur->stack[bur_cur->top - 1] = bur_obj((Obj *)o); // replace stdout root
    Value res = bur_ok(bur_peek(0));
    bur_pop();
    return res;
}
static Value nat_exec(Value *args, int argc) {
    (void)argc;
    const char *cmd; int64_t cmdn;
    OList *al = nat_as_list(args[1]);
    if (!nat_as_str(args[0], &cmd, &cmdn) || !al) bur_trap("exec() needs (str, [str])");
    // build argv (NUL-terminated copies)
    int n = (int)al->len;
    char **cargv = (char **)malloc(sizeof(char *) * (size_t)(n + 2));
    cargv[0] = strdup(cmd);
    for (int i = 0; i < n; i++) {
        const char *s; int64_t sn;
        if (!nat_as_str(al->elems[i], &s, &sn)) { for (int j = 0; j <= i; j++) free(cargv[j]); free(cargv); bur_trap("exec() args must be str"); }
        cargv[i + 1] = (char *)malloc((size_t)sn + 1);
        memcpy(cargv[i + 1], s, (size_t)sn); cargv[i + 1][sn] = '\0';
    }
    cargv[n + 1] = NULL;

    int outp[2], errp[2], failp[2];
    if (pipe(outp) || pipe(errp) || pipe(failp)) { for (int i = 0; i <= n; i++) free(cargv[i]); free(cargv); return bur_err_str(strerror(errno)); }
    fcntl(failp[1], F_SETFD, FD_CLOEXEC);
    pid_t pid = fork();
    if (pid == 0) {
        dup2(outp[1], 1); dup2(errp[1], 2);
        close(outp[0]); close(outp[1]); close(errp[0]); close(errp[1]); close(failp[0]);
        execvp(cargv[0], cargv);
        int e = errno; ssize_t wr = write(failp[1], &e, sizeof e); (void)wr;
        _exit(127);
    }
    close(outp[1]); close(errp[1]); close(failp[1]);
    for (int i = 0; i <= n; i++) free(cargv[i]);
    free(cargv);
    if (pid < 0) { close(outp[0]); close(errp[0]); close(failp[0]); return bur_err_str(strerror(errno)); }

    int childErr = 0; ssize_t fr = read(failp[0], &childErr, sizeof childErr); close(failp[0]);
    Buf ob = {0}, eb = {0}; char chunk[8192]; ssize_t r;
    while ((r = read(outp[0], chunk, sizeof chunk)) > 0) buf_bytes(&ob, chunk, (int64_t)r);
    while ((r = read(errp[0], chunk, sizeof chunk)) > 0) buf_bytes(&eb, chunk, (int64_t)r);
    close(outp[0]); close(errp[0]);
    int status = 0; waitpid(pid, &status, 0);
    if (fr == (ssize_t)sizeof childErr) { buf_free(&ob); buf_free(&eb); return bur_err_str(strerror(childErr)); }
    int code = WIFEXITED(status) ? WEXITSTATUS(status) : 128 + (WIFSIGNALED(status) ? WTERMSIG(status) : 0);
    Value res = nat_output(code, ob.data ? ob.data : "", ob.len, eb.data ? eb.data : "", eb.len);
    buf_free(&ob); buf_free(&eb);
    return res;
}

// ---- process, misc ----------------------------------------------------

static Value nat_args(Value *args, int argc) {
    (void)args; (void)argc;
    OList *l = bur_new_list(NULL, 0);
    bur_push(bur_obj((Obj *)l));
    for (int i = 1; i < bur_argc; i++) list_push(l, bur_obj((Obj *)bur_new_string(bur_argv[i])));
    bur_pop();
    return bur_obj((Obj *)l);
}
static Value nat_exit(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VINT) bur_trap("exit() needs an int, got %s", bur_typename(args[0]));
    fflush(stdout);
    exit((int)args[0].u.i);
}
static Value nat_clock(Value *args, int argc) {
    (void)args; (void)argc;
    struct timespec now;
    clock_gettime(CLOCK_MONOTONIC, &now);
    double s = (double)(now.tv_sec - bur_start_time.tv_sec) + (double)(now.tv_nsec - bur_start_time.tv_nsec) / 1e9;
    return bur_float(s);
}
static Value nat_type_of(Value *args, int argc) { (void)argc; return bur_obj((Obj *)bur_new_string(bur_typename(args[0]))); }
static Value nat_assert(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VBOOL) bur_trap("assert() needs a bool, got %s", bur_typename(args[0]));
    if (!args[0].u.b) {
        Buf b = {0};
        bur_format(&b, args[1], false);
        buf_char(&b, '\0');
        bur_trap("assertion failed: %s", b.data ? b.data : "");
    }
    return bur_unit();
}
// ---- concurrency ------------------------------------------------------

static Value nat_chan(Value *args, int argc) {
    int cap = 0;
    if (argc > 1) bur_trap("chan() takes at most one argument");
    if (argc == 1) {
        if (args[0].t != VINT || args[0].u.i < 0) bur_trap("chan() capacity must be a non-negative int");
        cap = (int)args[0].u.i;
    }
    OChannel *ch = (OChannel *)bur_alloc(sizeof(OChannel), OBJ_CHANNEL);
    ch->cap = cap; // bur_alloc zeroes the rest (empty buffer/queues, not closed)
    return bur_obj((Obj *)ch);
}
static Value nat_close(Value *args, int argc) {
    (void)argc;
    OChannel *ch = as_channel_opt(args[0]);
    if (!ch) bur_trap("close() needs a channel, got %s", bur_typename(args[0]));
    if (ch->closed) bur_trap("close of closed channel");
    ch->closed = true;
    // wake every blocked receiver: each re-runs its receive, drains any
    // buffered values, then observes closure
    for (int i = 0; i < ch->nrecvq; i++) bur_schedule(ch->recvq[i]);
    ch->nrecvq = 0;
    bur_wake_waiters(ch); // select arms on this channel are now ready
    return bur_unit();
}
// recv(ch): blocking receive exposed as an Option (None means closed+drained).
// Unlike the VM it can park directly, since natives run on the fiber's C stack.
static Value nat_recv(Value *args, int argc) {
    (void)argc;
    OChannel *ch = as_channel_opt(args[0]);
    if (!ch) bur_trap("recv() needs a channel, got %s", bur_typename(args[0]));
    for (;;) {
        Value v;
        if (chan_try_recv(ch, &v)) {
            bur_push(v); // root v across the Some allocation
            Value opt = bur_some(bur_peek(0));
            bur_pop();
            bur_wake_waiters(ch);
            return opt;
        }
        if (ch->closed) return bur_none();
        fq_push(&ch->recvq, &ch->nrecvq, &ch->recvqcap, bur_cur);
        bur_wake_waiters(ch);
        bur_park(FBLOCKED_RECV);
    }
}
static Value nat_yield(Value *args, int argc) {
    (void)args; (void)argc;
    bur_schedule(bur_cur); // cooperative handoff: reschedule at the back
    bur_switch_to_sched();
    return bur_unit();
}

static Value nat_gc(Value *args, int argc) { (void)args; (void)argc; bur_gc_collect(); return bur_int(bur_gc_last_freed); }
static Value nat_heap_objects(Value *args, int argc) { (void)args; (void)argc; return bur_int(bur_gc_count); }
static Value nat_gc_cycles(Value *args, int argc) { (void)args; (void)argc; return bur_int(bur_gc_cycles); }

// ---- registration -----------------------------------------------------

static void bur_register_native(const char *name, int arity, NativeFn fn) {
    ONative *n = (ONative *)bur_alloc(sizeof(ONative), OBJ_NATIVE);
    n->name = name;
    n->arity = arity;
    n->fn = fn;
    bur_globals_put(name, (int64_t)strlen(name), bur_obj((Obj *)n));
}

static void bur_register_natives(void) {
    bur_register_native("print", -1, nat_print);
    bur_register_native("println", -1, nat_println);
    bur_register_native("len", 1, nat_len);
    bur_register_native("map", 0, nat_map);
    bur_register_native("get", 2, nat_get);
    bur_register_native("put", 3, nat_put);
    bur_register_native("delete", 2, nat_delete);
    bur_register_native("keys", 1, nat_keys);
    bur_register_native("push", 2, nat_push);
    bur_register_native("pop", 1, nat_pop);
    bur_register_native("str", 1, nat_str);
    bur_register_native("trunc", 1, nat_trunc);
    bur_register_native("to_float", 1, nat_to_float);
    bur_register_native("float_bits", 1, nat_float_bits);
    bur_register_native("parse_int", 1, nat_parse_int);
    bur_register_native("parse_float", 1, nat_parse_float);
    bur_register_native("str_len", 1, nat_str_len);
    bur_register_native("char_at", 2, nat_char_at);
    bur_register_native("range", 2, nat_range);
    bur_register_native("split", 2, nat_split);
    bur_register_native("join", 2, nat_join);
    bur_register_native("substr", 3, nat_substr);
    bur_register_native("str_contains", 2, nat_str_contains);
    bur_register_native("str_index_of", 2, nat_str_index_of);
    bur_register_native("trim", 1, nat_trim);
    bur_register_native("slice", 3, nat_slice);
    bur_register_native("concat", 2, nat_concat);
    bur_register_native("contains", 2, nat_contains);
    bur_register_native("read_file", 1, nat_read_file);
    bur_register_native("write_file", 2, nat_write_file);
    bur_register_native("file_exists", 1, nat_file_exists);
    bur_register_native("read_dir", 1, nat_read_dir);
    bur_register_native("exec", 2, nat_exec);
    bur_register_native("args", 0, nat_args);
    bur_register_native("exit", 1, nat_exit);
    bur_register_native("chr", 1, nat_chr);
    bur_register_native("ord", 1, nat_ord);
    bur_register_native("clock", 0, nat_clock);
    bur_register_native("type_of", 1, nat_type_of);
    bur_register_native("assert", 2, nat_assert);
    bur_register_native("gc", 0, nat_gc);
    bur_register_native("heap_objects", 0, nat_heap_objects);
    bur_register_native("gc_cycles", 0, nat_gc_cycles);
    bur_register_native("chan", -1, nat_chan);
    bur_register_native("close", 1, nat_close);
    bur_register_native("recv", 1, nat_recv);
    bur_register_native("yield", 0, nat_yield);

    // built-in enum types are also visible as globals (mirrors newVM)
    if (bur_opt_enum) bur_globals_put("Option", 6, bur_obj((Obj *)bur_opt_enum));
    if (bur_res_enum) bur_globals_put("Result", 6, bur_obj((Obj *)bur_res_enum));
    if (bur_out_enum) bur_globals_put("Output", 6, bur_obj((Obj *)bur_out_enum));
}

#endif // BURRT_NATIVES_H
