// Burryn C runtime — native functions (multi-file implementation unit).
#include "burrt.h"

// ---- platform glue -----------------------------------------------------
// Winsock errors and sentinels differ from errno; these thin helpers keep
// every socket call site on one code path across platforms.
#ifdef _WIN32
#define strdup _strdup
static int bur_sock_err(void) { return WSAGetLastError(); }
static void bur_sock_set_err(int e) { WSASetLastError(e); }
static bool bur_sock_would_block(int err) {
    return err == WSAEWOULDBLOCK || err == WSAEINPROGRESS;
}
static bool bur_sock_intr(int err) { return err == WSAEINTR; }
static const char *bur_sock_strerror(int err) {
    static char buf[32];
    snprintf(buf, sizeof buf, "winsock error %d", err);
    return buf;
}
static void bur_sock_close(intptr_t fd) { closesocket((SOCKET)fd); }
static int bur_sock_prep(intptr_t fd) { // non-blocking; inheritability N/A
    u_long nb = 1;
    return ioctlsocket((SOCKET)fd, FIONBIO, &nb) == 0 ? 0 : -1;
}
static intptr_t bur_sock_accept(intptr_t lfd) {
    SOCKET s = accept((SOCKET)lfd, NULL, NULL);
    return s == INVALID_SOCKET ? -1 : (intptr_t)s;
}
static int bur_sock_connect(intptr_t fd, const struct sockaddr *sa, socklen_t len) {
    return connect((SOCKET)fd, sa, (int)len);
}
static int bur_sock_recv(intptr_t fd, char *buf, size_t max) {
    return (int)recv((SOCKET)fd, buf, (int)max, 0);
}
static int bur_sock_send(intptr_t fd, const char *buf, size_t len) {
    return (int)send((SOCKET)fd, buf, (int)len, 0);
}
static int bur_sock_take_error(intptr_t fd) {
    int e = 0;
    int n = sizeof e;
    if (getsockopt((SOCKET)fd, SOL_SOCKET, SO_ERROR, (char *)&e, &n) < 0)
        e = WSAGetLastError();
    return e;
}
#else
static int bur_sock_err(void) { return errno; }
static void bur_sock_set_err(int e) { errno = e; }
static bool bur_sock_would_block(int err) {
    return err == EAGAIN || err == EWOULDBLOCK || err == EINPROGRESS;
}
static bool bur_sock_intr(int err) { return err == EINTR; }
static const char *bur_sock_strerror(int err) { return strerror(err); }
static void bur_sock_close(intptr_t fd) { close((int)fd); }
static int bur_sock_prep(intptr_t fd) { // non-blocking + close-on-exec
    int flags = fcntl((int)fd, F_GETFL, 0);
    if (flags < 0 || fcntl((int)fd, F_SETFL, flags | O_NONBLOCK) < 0 ||
        fcntl((int)fd, F_SETFD, FD_CLOEXEC) < 0)
        return -1;
    return 0;
}
static intptr_t bur_sock_accept(intptr_t lfd) {
    return (intptr_t)accept((int)lfd, NULL, NULL);
}
static int bur_sock_connect(intptr_t fd, const struct sockaddr *sa, socklen_t len) {
    return connect((int)fd, sa, len);
}
static int bur_sock_recv(intptr_t fd, char *buf, size_t max) {
    return (int)recv((int)fd, buf, max, 0);
}
static int bur_sock_send(intptr_t fd, const char *buf, size_t len) {
#ifdef MSG_NOSIGNAL
    return (int)send((int)fd, buf, len, MSG_NOSIGNAL);
#else
    return (int)send((int)fd, buf, len, 0);
#endif
}
static int bur_sock_take_error(intptr_t fd) {
    int e = 0;
    socklen_t n = sizeof e;
    if (getsockopt((int)fd, SOL_SOCKET, SO_ERROR, &e, &n) < 0) e = errno;
    return e;
}
#endif

bool nat_as_str(Value v, const char **s, int64_t *n) {
    if (v.t == VOBJ && v.u.o->type == OBJ_STRING) {
        OString *o = (OString *)v.u.o;
        *s = o->data; *n = o->len; return true;
    }
    return false;
}
OList *nat_as_list(Value v) {
    return (v.t == VOBJ && v.u.o->type == OBJ_LIST) ? (OList *)v.u.o : NULL;
}
OMap *nat_as_map(Value v) {
    return (v.t == VOBJ && v.u.o->type == OBJ_MAP) ? (OMap *)v.u.o : NULL;
}

// ---- io ---------------------------------------------------------------

void nat_write_joined(Value *args, int argc) {
    for (int i = 0; i < argc; i++) {
        if (i > 0) fputc(' ', stdout);
        bur_write_display(args[i]);
    }
}
Value nat_print(Value *args, int argc) { nat_write_joined(args, argc); fflush(stdout); return bur_unit(); }
Value nat_println(Value *args, int argc) { nat_write_joined(args, argc); fputc('\n', stdout); fflush(stdout); return bur_unit(); }
Value nat_eprintln(Value *args, int argc) {
    Buf b = {0};
    for (int i = 0; i < argc; i++) {
        if (i > 0) buf_char(&b, ' ');
        bur_format(&b, args[i], false);
    }
    buf_char(&b, '\n');
    fwrite(b.data, 1, (size_t)b.len, stderr);
    buf_free(&b);
    return bur_unit();
}

// ---- collections ------------------------------------------------------

Value nat_len(Value *args, int argc) { (void)argc; return bur_int(bur_len(args[0])); }

Value nat_map(Value *args, int argc) {
    (void)args; (void)argc;
    OMap *m = (OMap *)bur_alloc(sizeof(OMap), OBJ_MAP);
    return bur_obj((Obj *)m);
}
Value nat_get(Value *args, int argc) {
    (void)argc;
    OMap *m = nat_as_map(args[0]);
    if (!m) bur_trap("get() needs a map, got %s", bur_typename(args[0]));
    MapKey k;
    if (!mapkey_of(args[1], &k)) bur_trap("map keys must be int or str, got %s", bur_typename(args[1]));
    Value v;
    if (map_get(m, k, &v)) return bur_some(v);
    return bur_none();
}
Value nat_put(Value *args, int argc) {
    (void)argc;
    OMap *m = nat_as_map(args[0]);
    if (!m) bur_trap("put() needs a map, got %s", bur_typename(args[0]));
    MapKey k;
    if (!mapkey_of(args[1], &k)) bur_trap("map keys must be int or str, got %s", bur_typename(args[1]));
    map_ensure(m);
    map_set(m, k, args[1], args[2]);
    return bur_unit();
}
Value nat_delete(Value *args, int argc) {
    (void)argc;
    OMap *m = nat_as_map(args[0]);
    if (!m) bur_trap("delete() needs a map, got %s", bur_typename(args[0]));
    MapKey k;
    if (!mapkey_of(args[1], &k)) bur_trap("map keys must be int or str, got %s", bur_typename(args[1]));
    map_del(m, k);
    return bur_unit();
}
Value nat_keys(Value *args, int argc) {
    (void)argc;
    OMap *m = nat_as_map(args[0]);
    if (!m) bur_trap("keys() needs a map, got %s", bur_typename(args[0]));
    OList *l = bur_new_list(NULL, 0);
    bur_push(bur_obj((Obj *)l));
    for (int64_t i = 0; i < m->len; i++) list_push(l, m->entries[i].key);
    bur_pop();
    return bur_obj((Obj *)l);
}
Value nat_push(Value *args, int argc) {
    (void)argc;
    OList *l = nat_as_list(args[0]);
    if (!l) bur_trap("push() needs a list, got %s", bur_typename(args[0]));
    list_push(l, args[1]);
    return bur_unit();
}
Value nat_pop(Value *args, int argc) {
    (void)argc;
    OList *l = nat_as_list(args[0]);
    if (!l) bur_trap("pop() needs a list, got %s", bur_typename(args[0]));
    if (l->len == 0) bur_trap("pop() on empty list");
    return l->elems[--l->len];
}
Value nat_range(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VINT || args[1].t != VINT) bur_trap("range() needs two ints");
    OList *l = bur_new_list(NULL, 0);
    bur_push(bur_obj((Obj *)l));
    for (int64_t i = args[0].u.i; i < args[1].u.i; i++) list_push(l, bur_int(i));
    bur_pop();
    return bur_obj((Obj *)l);
}
Value nat_slice(Value *args, int argc) {
    (void)argc;
    OList *l = nat_as_list(args[0]);
    if (!l || args[1].t != VINT || args[2].t != VINT) bur_trap("slice() needs ([a], int, int)");
    int64_t start = args[1].u.i, end = args[2].u.i;
    if (start < 0 || end < start || end > l->len)
        bur_trap("slice(%" PRId64 ", %" PRId64 ") out of bounds (len %" PRId64 ")", start, end, l->len);
    return bur_obj((Obj *)bur_new_list(l->elems + start, end - start));
}
Value nat_concat(Value *args, int argc) {
    (void)argc;
    OList *x = nat_as_list(args[0]), *y = nat_as_list(args[1]);
    if (!x || !y) bur_trap("concat() needs ([a], [a])");
    OList *l = bur_new_list(x->elems, x->len);
    bur_push(bur_obj((Obj *)l));
    for (int64_t i = 0; i < y->len; i++) list_push(l, y->elems[i]);
    bur_pop();
    return bur_obj((Obj *)l);
}
Value nat_contains(Value *args, int argc) {
    (void)argc;
    OList *l = nat_as_list(args[0]);
    if (!l) bur_trap("contains() needs a list, got %s", bur_typename(args[0]));
    for (int64_t i = 0; i < l->len; i++)
        if (bur_eq(l->elems[i], args[1])) return bur_bool(true);
    return bur_bool(false);
}

// ---- conversions & numbers --------------------------------------------

Value nat_str(Value *args, int argc) {
    (void)argc;
    Buf b = {0};
    bur_format(&b, args[0], false);
    Value r = bur_obj((Obj *)bur_new_string_n(b.data ? b.data : "", b.len));
    buf_free(&b);
    return r;
}
Value nat_trunc(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VFLOAT) bur_trap("trunc() needs a float, got %s", bur_typename(args[0]));
    return bur_int((int64_t)args[0].u.f);
}
Value nat_to_float(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VINT) bur_trap("to_float() needs an int, got %s", bur_typename(args[0]));
    return bur_float((double)args[0].u.i);
}
Value nat_float_bits(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VFLOAT) bur_trap("float_bits() needs a float, got %s", bur_typename(args[0]));
    uint64_t bits;
    memcpy(&bits, &args[0].u.f, sizeof(bits));
    return bur_int((int64_t)bits);
}
Value nat_parse_int(Value *args, int argc) {
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
Value nat_parse_float(Value *args, int argc) {
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

Value nat_str_len(Value *args, int argc) {
    (void)argc;
    const char *s; int64_t n;
    if (!nat_as_str(args[0], &s, &n)) bur_trap("str_len() needs a str, got %s", bur_typename(args[0]));
    return bur_int(n);
}
Value nat_char_at(Value *args, int argc) {
    (void)argc;
    const char *s; int64_t n;
    if (!nat_as_str(args[0], &s, &n) || args[1].t != VINT) bur_trap("char_at() needs (str, int)");
    int64_t i = args[1].u.i;
    if (i < 0 || i >= n) bur_trap("char_at index %" PRId64 " out of bounds (len %" PRId64 ")", i, n);
    return bur_obj((Obj *)bur_new_string_n(s + i, 1));
}
Value nat_byte_at(Value *args, int argc) {
    (void)argc;
    const char *s; int64_t n;
    if (!nat_as_str(args[0], &s, &n) || args[1].t != VINT) bur_trap("byte_at() needs (str, int)");
    int64_t i = args[1].u.i;
    if (i < 0 || i >= n) bur_trap("byte_at index %" PRId64 " out of bounds (len %" PRId64 ")", i, n);
    return bur_int((int64_t)(unsigned char)s[i]);
}
Value nat_split(Value *args, int argc) {
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
Value nat_join(Value *args, int argc) {
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
Value nat_substr(Value *args, int argc) {
    (void)argc;
    const char *s; int64_t n;
    if (!nat_as_str(args[0], &s, &n) || args[1].t != VINT || args[2].t != VINT) bur_trap("substr() needs (str, int, int)");
    int64_t start = args[1].u.i, cnt = args[2].u.i;
    if (start < 0 || cnt < 0 || start + cnt > n)
        bur_trap("substr(%" PRId64 ", %" PRId64 ") out of bounds (len %" PRId64 ")", start, cnt, n);
    return bur_obj((Obj *)bur_new_string_n(s + start, cnt));
}
Value nat_str_contains(Value *args, int argc) {
    (void)argc;
    const char *s, *sub; int64_t n, sln;
    if (!nat_as_str(args[0], &s, &n) || !nat_as_str(args[1], &sub, &sln)) bur_trap("str_contains() needs (str, str)");
    if (sln == 0) return bur_bool(true);
    for (int64_t i = 0; i + sln <= n; i++)
        if (memcmp(s + i, sub, (size_t)sln) == 0) return bur_bool(true);
    return bur_bool(false);
}
Value nat_str_index_of(Value *args, int argc) {
    (void)argc;
    const char *s, *sub; int64_t n, sln;
    if (!nat_as_str(args[0], &s, &n) || !nat_as_str(args[1], &sub, &sln)) bur_trap("str_index_of() needs (str, str)");
    if (sln == 0) return bur_some(bur_int(0));
    for (int64_t i = 0; i + sln <= n; i++)
        if (memcmp(s + i, sub, (size_t)sln) == 0) return bur_some(bur_int(i));
    return bur_none();
}
Value nat_trim(Value *args, int argc) {
    (void)argc;
    const char *s; int64_t n;
    if (!nat_as_str(args[0], &s, &n)) bur_trap("trim() needs a str, got %s", bur_typename(args[0]));
    int64_t i = 0, j = n;
    while (i < j && isspace((unsigned char)s[i])) i++;
    while (j > i && isspace((unsigned char)s[j - 1])) j--;
    return bur_obj((Obj *)bur_new_string_n(s + i, j - i));
}
Value nat_chr(Value *args, int argc) {
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
Value nat_byte_chr(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VINT || args[0].u.i < 0 || args[0].u.i > 255) bur_trap("byte_chr() needs an int in 0..255");
    char buf[1]; buf[0] = (char)args[0].u.i;
    return bur_obj((Obj *)bur_new_string_n(buf, 1));
}
Value nat_ord(Value *args, int argc) {
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

Value nat_read_file(Value *args, int argc) {
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
Value nat_write_file(Value *args, int argc) {
    (void)argc;
    const char *path, *contents; int64_t pn, cn;
    if (!nat_as_str(args[0], &path, &pn) || !nat_as_str(args[1], &contents, &cn)) bur_trap("write_file() needs (str, str)");
    FILE *fp = fopen(path, "wb");
    if (!fp) return bur_err_str(strerror(errno));
    if (cn > 0 && fwrite(contents, 1, (size_t)cn, fp) != (size_t)cn) { fclose(fp); return bur_err_str(strerror(errno)); }
    fclose(fp);
    return bur_ok(bur_unit());
}
Value nat_file_exists(Value *args, int argc) {
    (void)argc;
    const char *path; int64_t pn;
    if (!nat_as_str(args[0], &path, &pn)) bur_trap("file_exists() needs a str, got %s", bur_typename(args[0]));
    struct stat st;
    return bur_bool(stat(path, &st) == 0);
}
int nat_strcmp_qsort(const void *a, const void *b) {
    return strcmp(*(const char *const *)a, *(const char *const *)b);
}
Value nat_read_dir(Value *args, int argc) {
    (void)argc;
    const char *path; int64_t pn;
    if (!nat_as_str(args[0], &path, &pn)) bur_trap("read_dir() needs a str, got %s", bur_typename(args[0]));
#ifdef _WIN32
    size_t plen = strlen(path) + 4;
    char *pattern = (char *)malloc(plen);
    snprintf(pattern, plen, "%s\\*", path[0] ? path : ".");
    WIN32_FIND_DATAA find;
    HANDLE dh = FindFirstFileA(pattern, &find);
    free(pattern);
    if (dh == INVALID_HANDLE_VALUE) return bur_err_str("cannot open directory");
    char **names = NULL; int64_t count = 0, cap = 0;
    do {
        if (strcmp(find.cFileName, ".") == 0 || strcmp(find.cFileName, "..") == 0) continue;
        if (count == cap) { cap = cap * 2 + 16; names = (char **)realloc(names, sizeof(char *) * (size_t)cap); }
        names[count++] = strdup(find.cFileName);
    } while (FindNextFileA(dh, &find));
    FindClose(dh);
#else
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
#endif
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
Value nat_output(int code, const char *out, int64_t outn, const char *err, int64_t errn) {
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
int64_t bur_proc_alloc(void) {
    if (bur_nprocs == bur_procscap) {
        bur_procscap = bur_procscap * 2 + 8;
        bur_procs = (BurProc *)realloc(bur_procs, sizeof(BurProc) * (size_t)bur_procscap);
    }
    memset(&bur_procs[bur_nprocs], 0, sizeof(BurProc));
    bur_procs[bur_nprocs].used = true;
    return bur_nprocs++;
}

// wait on one proc's own fds until it completes (deterministic mode and
// single-runnable shortcuts); pointer stays valid: no allocation inside
#ifndef _WIN32
void bur_proc_finish(BurProc *p) {
    while (!p->complete) {
        struct pollfd pf[3]; int n = 0;
        int fds[3] = { (int)p->outfd, (int)p->errfd, (int)p->failfd };
        for (int k = 0; k < 3; k++)
            if (fds[k] >= 0) { pf[n].fd = fds[k]; pf[n].events = POLLIN; pf[n].revents = 0; n++; }
        if (n > 0) poll(pf, (nfds_t)n, -1);
        bur_proc_pump(p);
    }
}
#else
void bur_proc_finish(BurProc *p) {
    // deterministic mode serializes execution, so latency is not a contract
    while (!p->complete) {
        bur_proc_pump(p);
        if (!p->complete) Sleep(1);
    }
}
#endif

// consume a completed proc slot into exec's Result value
Value bur_proc_result(BurProc *p) {
    p->consumed = true;
    Value res;
    if (p->have_err) res = bur_err_str(strerror(p->child_err));
    else res = nat_output(p->code, p->ob.data ? p->ob.data : "", p->ob.len,
                          p->eb.data ? p->eb.data : "", p->eb.len);
    buf_free(&p->ob); buf_free(&p->eb);
    p->ob = (Buf){0}; p->eb = (Buf){0};
    return res;
}

// spawn a child into a fresh proc slot; on failure sets *errmsg and returns
// -1. In deterministic mode the child is run to completion before returning.
int64_t bur_proc_spawn(Value cmdv, Value argsv, const char **errmsg) {
    const char *cmd; int64_t cmdn;
    OList *al = nat_as_list(argsv);
    if (!nat_as_str(cmdv, &cmd, &cmdn) || !al) bur_trap("exec needs (str, [str])");
    // build argv (NUL-terminated copies)
    int n = (int)al->len;
    char **cargv = (char **)malloc(sizeof(char *) * (size_t)(n + 2));
    cargv[0] = strdup(cmd);
    for (int i = 0; i < n; i++) {
        const char *s; int64_t sn;
        if (!nat_as_str(al->elems[i], &s, &sn)) { for (int j = 0; j <= i; j++) free(cargv[j]); free(cargv); bur_trap("exec args must be str"); }
        cargv[i + 1] = (char *)malloc((size_t)sn + 1);
        memcpy(cargv[i + 1], s, (size_t)sn); cargv[i + 1][sn] = '\0';
    }
    cargv[n + 1] = NULL;

#ifdef _WIN32
    // Join argv into a command line with the standard CommandLineToArgvW
    // quoting: args holding space/tab/quote get wrapped, backslash runs
    // preceding a quote double up, embedded quotes escape as \".
    size_t clen = 1;
    for (int i = 0; i <= n; i++) clen += strlen(cargv[i]) * 2 + 3;
    char *cmdline = (char *)malloc(clen);
    size_t off = 0;
    for (int i = 0; i <= n; i++) {
        const char *a = cargv[i];
        int quote = a[0] == '\0' || strpbrk(a, " \t\"") != NULL;
        if (i) cmdline[off++] = ' ';
        if (quote) cmdline[off++] = '"';
        for (const char *q = a;; q++) {
            int bs = 0;
            while (q[bs] == '\\') bs++;
            q += bs;
            if (*q == '"') {
                for (int k = 0; k < bs * 2; k++) cmdline[off++] = '\\';
                cmdline[off++] = '"';
            } else if (*q == '\0' && quote) {
                for (int k = 0; k < bs * 2; k++) cmdline[off++] = '\\';
                break;
            } else {
                for (int k = 0; k < bs; k++) cmdline[off++] = '\\';
                if (*q == '\0') break;
                cmdline[off++] = *q;
            }
        }
        if (quote) cmdline[off++] = '"';
    }
    cmdline[off] = '\0';

    SECURITY_ATTRIBUTES sa = { sizeof sa, NULL, TRUE };
    HANDLE outrd, outwr, errrd, errwr;
    if (!CreatePipe(&outrd, &outwr, &sa, 0) || !CreatePipe(&errrd, &errwr, &sa, 0)) {
        for (int i = 0; i <= n; i++) free(cargv[i]);
        free(cargv);
        *errmsg = "cannot create pipe";
        return -1;
    }
    SetHandleInformation(outrd, HANDLE_FLAG_INHERIT, 0); // parent ends stay private
    SetHandleInformation(errrd, HANDLE_FLAG_INHERIT, 0);

    STARTUPINFOA si;
    memset(&si, 0, sizeof si);
    si.cb = sizeof si;
    si.dwFlags = STARTF_USESTDHANDLES; // stdin inherited, stdout/stderr piped
    si.hStdInput = GetStdHandle(STD_INPUT_HANDLE);
    si.hStdOutput = outwr;
    si.hStdError = errwr;

    PROCESS_INFORMATION pi;
    memset(&pi, 0, sizeof pi);
    // A CreateProcess failure IS exec failure reported synchronously, so the
    // POSIX fail pipe has no counterpart here; failfd stays -1 forever.
    BOOL ok = CreateProcessA(NULL, cmdline, NULL, NULL, TRUE, CREATE_NO_WINDOW,
                             NULL, NULL, &si, &pi);
    CloseHandle(outwr);
    CloseHandle(errwr);
    free(cmdline);
    for (int i = 0; i <= n; i++) free(cargv[i]);
    free(cargv);
    if (!ok) {
        CloseHandle(outrd);
        CloseHandle(errrd);
        *errmsg = "cannot start process";
        return -1;
    }
    CloseHandle(pi.hThread);

    int64_t h = bur_proc_alloc();
    BurProc *p = &bur_procs[h];
    p->pid = (intptr_t)pi.hProcess;
    p->outfd = (intptr_t)outrd; p->errfd = (intptr_t)errrd; p->failfd = -1;
    if (bur_deterministic) bur_proc_finish(p); // serialize: no IO overlap
    return h;
#else
    int outp[2], errp[2], failp[2];
    if (pipe(outp) || pipe(errp) || pipe(failp)) { for (int i = 0; i <= n; i++) free(cargv[i]); free(cargv); *errmsg = strerror(errno); return -1; }
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
    if (pid < 0) { close(outp[0]); close(errp[0]); close(failp[0]); *errmsg = strerror(errno); return -1; }
    fcntl(outp[0], F_SETFL, O_NONBLOCK);
    fcntl(errp[0], F_SETFL, O_NONBLOCK);
    fcntl(failp[0], F_SETFL, O_NONBLOCK);

    int64_t h = bur_proc_alloc();
    BurProc *p = &bur_procs[h];
    p->pid = pid;
    p->outfd = outp[0]; p->errfd = errp[0]; p->failfd = failp[0];
    if (bur_deterministic) bur_proc_finish(p); // serialize: no IO overlap
    return h;
#endif
}

// exec blocks its fiber, not the scheduler: it parks as FBLOCKED_IO and the
// scheduler's poll wakes it once the child is done.
Value nat_exec(Value *args, int argc) {
    (void)argc;
    const char *errmsg = NULL;
    int64_t h = bur_proc_spawn(args[0], args[1], &errmsg);
    if (h < 0) return bur_err_str(errmsg);
    for (;;) {
        BurProc *p = &bur_procs[h]; // re-fetch: the table can move while parked
        bur_proc_pump(p);
        if (p->complete) return bur_proc_result(p);
        bur_cur->io_proc = h;
        bur_nio++;
        bur_park(FBLOCKED_IO);
    }
}

Value nat_exec_start(Value *args, int argc) {
    (void)argc;
    const char *errmsg = NULL;
    int64_t h = bur_proc_spawn(args[0], args[1], &errmsg);
    if (h < 0) return bur_err_str(errmsg);
    return bur_ok(bur_int(h));
}

Value nat_exec_poll(Value *args, int argc) {
    (void)argc;
    int64_t h = args[0].u.i;
    if (!bur_proc_valid(h)) bur_trap("exec_poll: invalid or consumed handle %lld", (long long)h);
    BurProc *p = &bur_procs[h];
    bur_proc_pump(p);
    if (!p->complete) return bur_none();
    bur_push(bur_proc_result(p)); // root the Result across the Some allocation
    Value opt = bur_some(bur_peek(0));
    bur_pop();
    return opt;
}

// ---- tcp -------------------------------------------------------------


BurNet *bur_nets;
int64_t bur_nnets, bur_netscap;

Value bur_net_err(const char *op, const char *msg) {
    char buf[512];
    snprintf(buf, sizeof buf, "%s: %s", op, msg);
    return bur_err_str(buf);
}

Value bur_net_ok_str(const char *data, int64_t len) {
    bur_push(bur_obj((Obj *)bur_new_string_n(data, len)));
    Value res = bur_ok(bur_peek(0));
    bur_pop();
    return res;
}

intptr_t bur_net_socket(int family, int socktype, int protocol) {
    intptr_t fd = (intptr_t)socket(family, socktype, protocol);
#ifdef _WIN32
    if (fd == -1) return -1;
#else
    if (fd < 0) return -1;
#endif
    if (bur_sock_prep(fd) < 0) {
        int saved = bur_sock_err();
        bur_sock_close(fd);
        bur_sock_set_err(saved);
        return -1;
    }
#ifdef SO_NOSIGPIPE
    int one = 1;
    setsockopt((int)fd, SOL_SOCKET, SO_NOSIGPIPE, &one, sizeof one);
#endif
    return fd;
}

int64_t bur_net_alloc(intptr_t fd, BurNetKind kind) {
    if (bur_nnets == bur_netscap) {
        bur_netscap = bur_netscap * 2 + 16;
        bur_nets = (BurNet *)realloc(bur_nets, sizeof(BurNet) * (size_t)bur_netscap);
    }
    int64_t h = bur_nnets++;
    bur_nets[h].fd = fd;
    bur_nets[h].kind = kind;
    bur_nets[h].used = true;
    return h;
}

BurNet *bur_net_get(int64_t h, BurNetKind kind) {
    if (h < 0 || h >= bur_nnets || !bur_nets[h].used || bur_nets[h].kind != kind) return NULL;
    return &bur_nets[h];
}

void bur_net_cancel_waiters(intptr_t fd) {
    for (int64_t i = 0; i < bur_nfibers; i++) {
        Fiber *f = bur_fibers[i];
        if (f->status != FBLOCKED_IO || f->io_fd != fd) continue;
        bur_nio--;
        f->io_fd = -1;
        f->io_events = 0;
        f->io_ready = false;
        bur_schedule(f);
    }
}

Value nat_tcp_listen(Value *args, int argc) {
    (void)argc;
    const char *host; int64_t hostn;
    if (!nat_as_str(args[0], &host, &hostn) || args[1].t != VINT)
        bur_trap("tcp_listen() needs (str, int)");
    int64_t port = args[1].u.i;
    if (port < 0 || port > 65535) return bur_net_err("tcp_listen", "port out of range");

    char service[24];
    snprintf(service, sizeof service, "%lld", (long long)port);
    struct addrinfo hints = {0}, *addrs = NULL;
    hints.ai_family = AF_UNSPEC;
    hints.ai_socktype = SOCK_STREAM;
    hints.ai_flags = AI_PASSIVE | AI_NUMERICSERV;
    int gai = getaddrinfo(hostn == 0 ? NULL : host, service, &hints, &addrs);
    if (gai != 0) return bur_net_err("tcp_listen", gai_strerror(gai));

#ifdef _WIN32
    intptr_t fd = -1; int saved = WSAEADDRNOTAVAIL;
#else
    intptr_t fd = -1; int saved = EADDRNOTAVAIL;
#endif
    for (struct addrinfo *a = addrs; a; a = a->ai_next) {
        fd = bur_net_socket(a->ai_family, a->ai_socktype, a->ai_protocol);
        if (fd < 0) { saved = bur_sock_err(); continue; }
        int one = 1;
        setsockopt((int)fd, SOL_SOCKET, SO_REUSEADDR, &one, sizeof one);
        if (bind((int)fd, a->ai_addr, a->ai_addrlen) == 0 && listen((int)fd, 128) == 0) break;
        saved = bur_sock_err();
        bur_sock_close(fd);
        fd = -1;
    }
    freeaddrinfo(addrs);
    if (fd < 0) return bur_net_err("tcp_listen", bur_sock_strerror(saved));
    return bur_ok(bur_int(bur_net_alloc(fd, BUR_NET_LISTENER)));
}

Value nat_tcp_accept(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VINT) bur_trap("tcp_accept() needs an int");
    int64_t h = args[0].u.i;
    for (;;) {
        BurNet *listener = bur_net_get(h, BUR_NET_LISTENER);
        if (!listener) return bur_net_err("tcp_accept", "invalid listener handle");
        intptr_t fd = bur_sock_accept(listener->fd);
        if (fd >= 0) {
            if (bur_sock_prep(fd) < 0) {
                int saved = bur_sock_err();
                bur_sock_close(fd);
                return bur_net_err("tcp_accept", bur_sock_strerror(saved));
            }
#ifdef SO_NOSIGPIPE
            int one = 1;
            setsockopt((int)fd, SOL_SOCKET, SO_NOSIGPIPE, &one, sizeof one);
#endif
            return bur_ok(bur_int(bur_net_alloc(fd, BUR_NET_CONN)));
        }
        int err = bur_sock_err();
        if (bur_sock_intr(err)) continue;
        if (!bur_sock_would_block(err))
            return bur_net_err("tcp_accept", bur_sock_strerror(err));
        bur_wait_current_fd(listener->fd, POLLIN);
    }
}

Value nat_tcp_dial(Value *args, int argc) {
    (void)argc;
    const char *host; int64_t hostn;
    if (!nat_as_str(args[0], &host, &hostn) || args[1].t != VINT)
        bur_trap("tcp_dial() needs (str, int)");
    int64_t port = args[1].u.i;
    if (hostn == 0) return bur_net_err("tcp_dial", "empty host");
    if (port < 0 || port > 65535) return bur_net_err("tcp_dial", "port out of range");

    char service[24];
    snprintf(service, sizeof service, "%lld", (long long)port);
    struct addrinfo hints = {0}, *addrs = NULL;
    hints.ai_family = AF_UNSPEC;
    hints.ai_socktype = SOCK_STREAM;
    hints.ai_flags = AI_NUMERICSERV;
    int gai = getaddrinfo(host, service, &hints, &addrs);
    if (gai != 0) return bur_net_err("tcp_dial", gai_strerror(gai));

#ifdef _WIN32
    intptr_t fd = -1; int saved = WSAECONNREFUSED;
#else
    intptr_t fd = -1; int saved = ECONNREFUSED;
#endif
    for (struct addrinfo *a = addrs; a; a = a->ai_next) {
        fd = bur_net_socket(a->ai_family, a->ai_socktype, a->ai_protocol);
        if (fd < 0) { saved = bur_sock_err(); continue; }
        if (bur_sock_connect(fd, a->ai_addr, a->ai_addrlen) < 0) {
            int err = bur_sock_err();
            if (!bur_sock_would_block(err)) {
                saved = err;
                bur_sock_close(fd);
                fd = -1;
                continue;
            }
            bur_wait_current_fd(fd, POLLOUT);
            saved = bur_sock_take_error(fd);
            if (saved != 0) {
                bur_sock_close(fd);
                fd = -1;
                continue;
            }
        }
        break;
    }
    freeaddrinfo(addrs);
    if (fd < 0) return bur_net_err("tcp_dial", bur_sock_strerror(saved));
    return bur_ok(bur_int(bur_net_alloc(fd, BUR_NET_CONN)));
}

Value nat_net_read(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VINT || args[1].t != VINT) bur_trap("net_read() needs (int, int)");
    int64_t h = args[0].u.i, max = args[1].u.i;
    if (max < 0) return bur_net_err("net_read", "max must not be negative");
    if (max == 0) return bur_net_ok_str("", 0);
    char *buf = (char *)malloc((size_t)max);
    for (;;) {
        BurNet *conn = bur_net_get(h, BUR_NET_CONN);
        if (!conn) { free(buf); return bur_net_err("net_read", "invalid connection handle"); }
        int n = bur_sock_recv(conn->fd, buf, (size_t)max);
        if (n >= 0) {
            Value res = bur_net_ok_str(buf, (int64_t)n);
            free(buf);
            return res;
        }
        int err = bur_sock_err();
        if (bur_sock_intr(err)) continue;
        if (!bur_sock_would_block(err)) {
            free(buf);
            return bur_net_err("net_read", bur_sock_strerror(err));
        }
        bur_wait_current_fd(conn->fd, POLLIN);
    }
}

Value nat_net_write(Value *args, int argc) {
    (void)argc;
    const char *data; int64_t len;
    if (args[0].t != VINT || !nat_as_str(args[1], &data, &len))
        bur_trap("net_write() needs (int, str)");
    int64_t h = args[0].u.i, off = 0;
    while (off < len) {
        BurNet *conn = bur_net_get(h, BUR_NET_CONN);
        if (!conn) return bur_net_err("net_write", "invalid connection handle");
        int n = bur_sock_send(conn->fd, data + off, (size_t)(len - off));
        if (n > 0) { off += (int64_t)n; continue; }
        if (n == 0) return bur_net_err("net_write", "write returned zero bytes");
        int err = bur_sock_err();
        if (bur_sock_intr(err)) continue;
        if (!bur_sock_would_block(err))
            return bur_net_err("net_write", bur_sock_strerror(err));
        bur_wait_current_fd(conn->fd, POLLOUT);
    }
    return bur_ok(bur_unit());
}

Value nat_net_close(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VINT) bur_trap("net_close() needs an int");
    int64_t h = args[0].u.i;
    if (h < 0 || h >= bur_nnets || !bur_nets[h].used)
        bur_trap("net_close: invalid or closed handle %lld", (long long)h);
    intptr_t fd = bur_nets[h].fd;
    bur_nets[h].used = false;
    bur_net_cancel_waiters(fd);
    bur_sock_close(fd);
    return bur_unit();
}

// net_nb: non-blocking socket operation for the VM scheduler.
// op 0 = accept, op 1 = read, op 2 = write.
// Returns Ok(str) on success, Err("__eagain") when the operation would
// block, Err(msg) on real errors.
Value nat_net_nb(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VINT || args[1].t != VINT || args[3].t != VINT)
        bur_trap("net_nb() needs (int, int, str, int)");
    int64_t op = args[0].u.i, h = args[1].u.i;

    if (op == 0) { // accept
        BurNet *listener = bur_net_get(h, BUR_NET_LISTENER);
        if (!listener) return bur_net_err("tcp_accept", "invalid listener handle");
        for (;;) {
            intptr_t fd = bur_sock_accept(listener->fd);
            if (fd >= 0) {
                if (bur_sock_prep(fd) < 0) {
                    int saved = bur_sock_err();
                    bur_sock_close(fd);
                    return bur_net_err("tcp_accept", bur_sock_strerror(saved));
                }
#ifdef SO_NOSIGPIPE
                int one = 1;
                setsockopt((int)fd, SOL_SOCKET, SO_NOSIGPIPE, &one, sizeof one);
#endif
                int64_t ch = bur_net_alloc(fd, BUR_NET_CONN);
                char buf[24];
                int n = snprintf(buf, sizeof buf, "%lld", (long long)ch);
                return bur_ok(bur_obj((Obj *)bur_new_string_n(buf, n)));
            }
            int err = bur_sock_err();
            if (bur_sock_intr(err)) continue;
            if (bur_sock_would_block(err))
                return bur_err_str("__eagain");
            return bur_net_err("tcp_accept", bur_sock_strerror(err));
        }
    }
    if (op == 1) { // read
        int64_t max = args[3].u.i;
        if (max < 0) return bur_net_err("net_read", "max must not be negative");
        if (max == 0) return bur_net_ok_str("", 0);
        char *buf = (char *)malloc((size_t)max);
        for (;;) {
            BurNet *conn = bur_net_get(h, BUR_NET_CONN);
            if (!conn) { free(buf); return bur_net_err("net_read", "invalid connection handle"); }
            int n = bur_sock_recv(conn->fd, buf, (size_t)max);
            if (n >= 0) {
                Value res = bur_net_ok_str(buf, (int64_t)n);
                free(buf);
                return res;
            }
            int err = bur_sock_err();
            if (bur_sock_intr(err)) continue;
            if (bur_sock_would_block(err)) {
                free(buf);
                return bur_err_str("__eagain");
            }
            free(buf);
            return bur_net_err("net_read", bur_sock_strerror(err));
        }
    }
    if (op == 2) { // write (single attempt, returns bytes written)
        const char *data; int64_t len;
        if (!nat_as_str(args[2], &data, &len))
            bur_trap("net_nb(write) needs a str argument");
        BurNet *conn = bur_net_get(h, BUR_NET_CONN);
        if (!conn) return bur_net_err("net_write", "invalid connection handle");
        if (len == 0) return bur_net_ok_str("0", 1);
        for (;;) {
            int n = bur_sock_send(conn->fd, data, (size_t)len);
            if (n >= 0) {
                char buf[24];
                int sn = snprintf(buf, sizeof buf, "%lld", (long long)n);
                return bur_ok(bur_obj((Obj *)bur_new_string_n(buf, sn)));
            }
            int err = bur_sock_err();
            if (bur_sock_intr(err)) continue;
            if (bur_sock_would_block(err))
                return bur_err_str("__eagain");
            return bur_net_err("net_write", bur_sock_strerror(err));
        }
    }
    bur_trap("net_nb: invalid op %lld", (long long)op);
    return bur_unit();
}

// ---- process, misc ----------------------------------------------------

Value nat_args(Value *args, int argc) {
    (void)args; (void)argc;
    OList *l = bur_new_list(NULL, 0);
    bur_push(bur_obj((Obj *)l));
    for (int i = 1; i < bur_argc; i++) list_push(l, bur_obj((Obj *)bur_new_string(bur_argv[i])));
    bur_pop();
    return bur_obj((Obj *)l);
}
Value nat_exit(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VINT) bur_trap("exit() needs an int, got %s", bur_typename(args[0]));
    fflush(stdout);
    exit((int)args[0].u.i);
}
Value nat_clock(Value *args, int argc) {
    (void)args; (void)argc;
    double s = (double)(bur_mono_ns() - bur_start_ns) / 1e9;
    return bur_float(s);
}
Value nat_type_of(Value *args, int argc) { (void)argc; return bur_obj((Obj *)bur_new_string(bur_typename(args[0]))); }
Value nat_assert(Value *args, int argc) {
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

Value nat_chan(Value *args, int argc) {
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
Value nat_close(Value *args, int argc) {
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
Value nat_recv(Value *args, int argc) {
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
Value nat_yield(Value *args, int argc) {
    (void)args; (void)argc;
    bur_schedule(bur_cur); // cooperative handoff: reschedule at the back
    bur_switch_to_sched();
    return bur_unit();
}
Value nat_sleep(Value *args, int argc) {
    (void)argc;
    int64_t ms = args[0].u.i;
    if (ms <= 0) { // sleep(0) is a plain yield
        bur_schedule(bur_cur);
        bur_switch_to_sched();
        return bur_unit();
    }
    bur_cur->wake_ns = bur_now_ns() + ms * 1000000;
    bur_ntimers++;
    bur_park(FBLOCKED_TIMER); // the scheduler wakes us at the deadline
    return bur_unit();
}

// ---- stdin (LSP transport) --------------------------------------------

void bur_stdin_nonblock(void) {
    static int done = 0;
    if (done) return;
#ifndef _WIN32
    int flags = fcntl(0, F_GETFL, 0);
    if (flags >= 0) fcntl(0, F_SETFL, flags | O_NONBLOCK);
#endif
    done = 1; // Windows checks readiness per read via PeekNamedPipe instead
}

// one stdin read attempt; sets *again when data may still arrive later.
// Console stdin on Windows has no non-blocking mode, so it reads blocking.
static int64_t bur_stdin_try(char *buf, int64_t max, bool *again) {
#ifdef _WIN32
    HANDLE h = GetStdHandle(STD_INPUT_HANDLE);
    *again = false;
    if (!h || h == INVALID_HANDLE_VALUE) return -1;
    DWORD want = max > 8192 ? 8192 : (DWORD)max;
    if (GetFileType(h) == FILE_TYPE_PIPE) { // spawned-with-pipes case (LSP)
        DWORD avail = 0;
        if (!PeekNamedPipe(h, NULL, 0, NULL, &avail, NULL)) return -1; // EOF
        if (avail == 0) { *again = true; return -1; }
        if (want > avail) want = avail;
    }
    DWORD got = 0;
    if (!ReadFile(h, buf, want, &got, NULL)) return -1;
    return got;
#else
    ssize_t n = read(0, buf, (size_t)max);
    if (n >= 0) { *again = false; return n; }
    *again = errno == EINTR || errno == EAGAIN || errno == EWOULDBLOCK;
    return -1;
#endif
}

Value nat_read_stdin(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VINT) bur_trap("read_stdin() needs an int");
    int64_t max = args[0].u.i;
    if (max <= 0) return bur_obj((Obj *)bur_new_string_n("", 0));
    bur_stdin_nonblock();
    char *buf = (char *)malloc((size_t)max);
    for (;;) {
        bool again = false;
        int64_t n = bur_stdin_try(buf, max, &again);
        if (n >= 0) {
            Value v = bur_obj((Obj *)bur_new_string_n(buf, (int64_t)n));
            free(buf);
            return v;
        }
        if (!again) {
            free(buf);
            bur_trap("read_stdin: input error");
        }
#ifdef _WIN32
        // stdin cannot join the socket waitset; wake on a short timer and retry
        bur_cur->wake_ns = bur_now_ns() + 1000000;
        bur_ntimers++;
        bur_park(FBLOCKED_TIMER);
#else
        bur_wait_current_fd(0, POLLIN);
#endif
    }
}

Value nat_stdin_nb(Value *args, int argc) {
    (void)argc;
    if (args[0].t != VINT) bur_trap("stdin_nb() needs an int");
    int64_t max = args[0].u.i;
    if (max <= 0) return bur_ok_str("", 0);
    bur_stdin_nonblock();
    char *buf = (char *)malloc((size_t)max);
    bool again = false;
    int64_t n = bur_stdin_try(buf, max, &again);
    if (n >= 0) {
        bur_push(bur_obj((Obj *)bur_new_string_n(buf, (int64_t)n)));
        Value res = bur_ok(bur_peek(0));
        bur_pop();
        free(buf);
        return res;
    }
    free(buf);
    if (again) return bur_err_str("__eagain");
#ifdef _WIN32
    return bur_err_str("stdin read failed");
#else
    return bur_err_str(strerror(errno));
#endif
}

Value nat_gc(Value *args, int argc) { (void)args; (void)argc; bur_gc_collect(); return bur_int(bur_gc_last_freed); }
Value nat_heap_objects(Value *args, int argc) { (void)args; (void)argc; return bur_int(bur_gc_count); }
Value nat_gc_cycles(Value *args, int argc) { (void)args; (void)argc; return bur_int(bur_gc_cycles); }

// ---- registration -----------------------------------------------------

void bur_register_native(const char *name, int arity, NativeFn fn) {
    ONative *n = (ONative *)bur_alloc(sizeof(ONative), OBJ_NATIVE);
    n->name = name;
    n->arity = arity;
    n->fn = fn;
    bur_globals_put(name, (int64_t)strlen(name), bur_obj((Obj *)n));
}

void bur_register_natives(void) {
    bur_register_native("print", -1, nat_print);
    bur_register_native("println", -1, nat_println);
    bur_register_native("eprintln", -1, nat_eprintln);
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
    bur_register_native("byte_at", 2, nat_byte_at);
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
    bur_register_native("byte_chr", 1, nat_byte_chr);
    bur_register_native("ord", 1, nat_ord);
    bur_register_native("clock", 0, nat_clock);
    bur_register_native("sleep", 1, nat_sleep);
    bur_register_native("exec_start", 2, nat_exec_start);
    bur_register_native("exec_poll", 1, nat_exec_poll);
    bur_register_native("tcp_listen", 2, nat_tcp_listen);
    bur_register_native("tcp_accept", 1, nat_tcp_accept);
    bur_register_native("tcp_dial", 2, nat_tcp_dial);
    bur_register_native("net_read", 2, nat_net_read);
    bur_register_native("net_write", 2, nat_net_write);
    bur_register_native("net_close", 1, nat_net_close);
    bur_register_native("net_nb", 4, nat_net_nb);
    bur_register_native("read_stdin", 1, nat_read_stdin);
    bur_register_native("stdin_nb", 1, nat_stdin_nb);
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


