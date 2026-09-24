// errtext-dump.c — 打印本平台 C 运行时的错误文本，供 x86 后端逐平台对齐 Err 文本。
// Prints this platform's C runtime error texts so the x86 backend can match
// its Err texts per platform: strerror over the errno range, gai_strerror for
// every EAI_* code the platform defines, and the errno fopen actually sets for
// the file-error shapes the runtime natives hit.
// Usage: cc -o errtext-dump scripts/errtext-dump.c && ./errtext-dump
#ifdef _WIN32
#define _CRT_SECURE_NO_WARNINGS
#include <winsock2.h>
#include <ws2tcpip.h>
#pragma comment(lib, "ws2_32.lib")
#else
#include <netdb.h>
#endif
#include <errno.h>
#include <stdio.h>
#include <string.h>

static void try_open(const char *label, const char *path, const char *mode) {
    errno = 0;
    FILE *fp = fopen(path, mode);
    if (fp) {
        printf("fopen\t%s\tok\n", label);
        fclose(fp);
    } else {
        printf("fopen\t%s\t%d\t%s\n", label, errno, strerror(errno));
    }
}

int main(void) {
    for (int i = 0; i <= 160; i++) printf("strerror\t%d\t%s\n", i, strerror(i));
#define GAI(name) printf("gai\t%s\t%d\t%s\n", #name, (int)(name), gai_strerror(name));
#ifdef EAI_AGAIN
    GAI(EAI_AGAIN)
#endif
#ifdef EAI_BADFLAGS
    GAI(EAI_BADFLAGS)
#endif
#ifdef EAI_FAIL
    GAI(EAI_FAIL)
#endif
#ifdef EAI_FAMILY
    GAI(EAI_FAMILY)
#endif
#ifdef EAI_MEMORY
    GAI(EAI_MEMORY)
#endif
#ifdef EAI_NONAME
    GAI(EAI_NONAME)
#endif
#ifdef EAI_SERVICE
    GAI(EAI_SERVICE)
#endif
#ifdef EAI_SOCKTYPE
    GAI(EAI_SOCKTYPE)
#endif
#ifdef EAI_NODATA
    GAI(EAI_NODATA)
#endif
#ifdef EAI_ADDRFAMILY
    GAI(EAI_ADDRFAMILY)
#endif
#ifdef EAI_SYSTEM
    GAI(EAI_SYSTEM)
#endif
#ifdef EAI_OVERFLOW
    GAI(EAI_OVERFLOW)
#endif
    try_open("read_missing", "no_such_dir_xyz/no_such_file", "rb");
    try_open("read_dir", ".", "rb");
    try_open("write_missing_dir", "no_such_dir_xyz/f", "wb");
    try_open("write_dir", ".", "wb");
    return 0;
}
