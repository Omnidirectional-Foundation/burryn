#!/bin/bash
# lsp-verify.sh — LSP 服务器行为验证：stdio JSON-RPC 驱动实测。
# LSP server behaviour tests: drives JSON-RPC over stdio.
# 断言：能力位、formatting 全文编辑与 fmt 引擎一致、已格式化空编辑、坏代码返回 null。
# Asserts: capability bits, formatting full-document edit parity with the fmt
# engine, empty edits on already-clean input, null on broken code.
# Usage: BUR=<compiler> ./scripts/lsp-verify.sh
set -u
cd "$(dirname "$0")/.."

if [ -z "${BUR:-}" ]; then
    echo "BUR is not set; invoke as BUR=<compiler> $0 ..." >&2
    exit 2
fi

if ! python3 - "$BUR" <<'PYEOF'
import json, subprocess, sys

bur = sys.argv[1]
p = subprocess.Popen([bur, "lsp"], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                     stderr=subprocess.PIPE)
rid = 0

def send(method, params=None):
    global rid
    rid += 1
    msg = {"jsonrpc": "2.0", "id": rid, "method": method}
    if params is not None:
        msg["params"] = params
    d = json.dumps(msg).encode()
    p.stdin.write(b"Content-Length: %d\r\n\r\n" % len(d) + d)
    p.stdin.flush()

def recv_any():
    hdr = {}
    while True:
        line = p.stdout.readline()
        if line in (b"\r\n", b"\n"):
            break
        k, _, v = line.decode().partition(":")
        hdr[k.strip().lower()] = v.strip()
    return json.loads(p.stdout.read(int(hdr["content-length"])))

def req(m, prm):
    send(m, prm)
    while True:
        r = recv_any()
        if "id" in r:
            return r

def check(name, ok):
    print(("PASS " if ok else "FAIL ") + name)
    if not ok:
        p.kill()
        sys.exit(1)

init = req("initialize", {"processId": None, "rootUri": "file:///tmp",
                          "capabilities": {}})
caps = init["result"]["capabilities"]
check("capabilities", caps.get("documentFormattingProvider") is True
      and caps.get("hoverProvider") is True
      and caps.get("definitionProvider") is True)
send("initialized", {})

uri = "file:///tmp/burryn-lsp-verify.bur"

def fmt(text):
    send("textDocument/didOpen", {"textDocument": {"uri": uri, "languageId": "burryn",
                                                   "version": 1, "text": text}})
    r = req("textDocument/formatting", {"textDocument": {"uri": uri},
                                        "options": {"tabSize": 4}})
    return r["result"]

dirty = 'fn add(a, b) {\n        a + b\n}\nlet  x   =  add(1,2)\nprintln("x:",x)\n'
r1 = fmt(dirty)
ok = isinstance(r1, list) and len(r1) == 1 \
    and r1[0]["range"]["start"] == {"line": 0, "character": 0} \
    and r1[0]["range"]["end"]["line"] == dirty.count("\n") \
    and r1[0]["newText"] != dirty
check("formatting dirty -> one full-range edit", ok)
if ok:
    out = subprocess.run([bur, "fmt", "-"], input=dirty.encode(),
                         capture_output=True)
    check("edit matches fmt engine", r1[0]["newText"].encode() == out.stdout)

check("formatting clean -> empty edits", fmt(r1[0]["newText"]) == [])
check("formatting broken -> null", fmt("fn broken( {\n") is None)

send("shutdown", None)
req_wait = recv_any()
check("shutdown responds", req_wait.get("id") is not None)
send("exit", None)
check("exit code 0 after shutdown", p.wait(timeout=10) == 0)
print("ALL PASS")
PYEOF
then
    exit 1
fi
