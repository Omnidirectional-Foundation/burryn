#!/bin/bash
# lsp-verify.sh — LSP 服务器行为验证：stdio JSON-RPC 驱动实测。
# LSP server behaviour tests: drives JSON-RPC over stdio.
# 断言：能力位、formatting 全文编辑与 fmt 引擎一致、已格式化空编辑、坏代码 null、
# completion 作用域感知（内层可见外层、块出即遮蔽、global fn 标 Function）、
# signature-help（activeParameter 按实参区间、参数在标签内定位、方法剔 receiver、
# 调用外 null）、documentHighlight（与 references 同一归一集合）、
# prepareRename/rename（光标落声明或使用处等价、WorkspaceEdit 覆盖全部位置）、
# native 内建在 references/rename/documentHighlight 上一律空结果（回归：native
# 全体共用 declare(..., Sp(0, 0))，未加判据会把毫不相关的 native 揉进同一组）。
# Asserts: capability bits, formatting full-document edit parity with the fmt
# engine, empty edits on already-clean input, null on broken code, scope-aware
# completion (inner sees outer, shadowing on block exit, global functions
# tagged Function), signature-help (activeParameter from argument spans,
# parameter labels within the label, method receiver excluded, null off-call),
# documentHighlight (same normalized set as references), prepareRename/rename
# (cursor on declaration or use is equivalent, WorkspaceEdit covers every
# site), and native builtins yielding empty results on references/rename/
# documentHighlight (regression: every native shares declare(..., Sp(0, 0)),
# and without a guard that lumps unrelated natives into one group).
# Usage: BUR=<compiler> ./scripts/lsp-verify.sh
set -u
cd "$(dirname "$0")/.."

if [ -z "${BUR:-}" ]; then
    echo "BUR is not set; invoke as BUR=<compiler> $0 ..." >&2
    exit 2
fi

if ! python3 - "$BUR" <<'PYEOF'
import json, os, subprocess, sys

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
      and caps.get("definitionProvider") is True
      and caps.get("signatureHelpProvider") is not None
      and caps.get("referencesProvider") is True
      and caps.get("documentSymbolProvider") is True
      and caps.get("documentHighlightProvider") is True
      and caps.get("renameProvider", {}).get("prepareProvider") is True
      and caps.get("completionProvider", {}).get("triggerCharacters") == [":"])
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

# completion：模块 fixture，作用域感知
mod_dir = "/tmp/burryn-lsp-verify-mod"
os.makedirs(mod_dir, exist_ok=True)
with open(mod_dir + "/bur.mod", "w") as f:
    f.write("module lspverify\n")
mod_src = (
    "import \"std/encoding\"\n"
    "\n"
    "fn helper(x) {\n"
    "    x + 1\n"
    "}\n"
    "\n"
    "fn add3(a, b, c) {\n"
    "    a + b + c\n"
    "}\n"
    "\n"
    "let topv = 5\n"
    "\n"
    "fn main() {\n"
    "    let inner = topv + helper(1)\n"
    "    let total = add3(1, topv, 3)\n"
    "    let sl = str_len(\"abc\")\n"
    "    let hx = encoding::hex_encode(\"hi\")\n"
    "    if inner > 0 {\n"
    "        let deep = 1\n"
    "        println(deep)\n"
    "    }\n"
    "    println(inner)\n"
    "    println(total + sl)\n"
    "    println(hx)\n"
    "}\n"
)
with open(mod_dir + "/main.bur", "w") as f:
    f.write(mod_src)
muri = "file://" + mod_dir + "/main.bur"
mlines = mod_src.split("\n")
send("textDocument/didOpen", {"textDocument": {"uri": muri, "languageId": "burryn", "version": 1, "text": mod_src}})

def comp(pat):
    ln = next(i for i, l in enumerate(mlines) if pat in l)
    ch = mlines[ln].index(pat.split("(")[0]) + 3
    r = req("textDocument/completion", {"textDocument": {"uri": muri},
                                        "position": {"line": ln, "character": ch}})
    return {i["label"]: i["kind"] for i in r["result"]}

inside = comp("println(deep)")
check("completion inside block sees all",
      inside.get("helper") == 3 and inside.get("topv") == 6
      and inside.get("inner") == 6 and inside.get("deep") == 6)
outside = comp("println(inner)")
check("completion outside block shadows deep",
      "inner" in outside and "helper" in outside and "topv" in outside
      and "deep" not in outside)

# signature-help：activeParameter、标签内参数定位、方法剔 receiver、调用外 null
def sigat(uri, ln, ch):
    r = req("textDocument/signatureHelp", {"textDocument": {"uri": uri},
                                           "position": {"line": ln, "character": ch}})
    return r["result"]

ln_add3 = next(i for i, l in enumerate(mlines) if "add3(1, topv" in l)
sig1 = sigat(muri, ln_add3, mlines[ln_add3].index("topv"))
check("signatureHelp 2nd arg -> active 1 with param span",
      sig1 is not None and sig1["activeParameter"] == 1
      and sig1["signatures"][0]["label"] == "add3(a, b, c)"
      and sig1["signatures"][0]["parameters"][1]["label"] == [8, 9])
sig0 = sigat(muri, ln_add3, mlines[ln_add3].index("(1,") + 1)
check("signatureHelp 1st arg -> active 0",
      sig0 is not None and sig0["activeParameter"] == 0)
ln_top = next(i for i, l in enumerate(mlines) if l.strip() == "let topv = 5")
check("signatureHelp outside call -> null", sigat(muri, ln_top, 4) is None)

# pkg:: 成员补全：import std/encoding 后 :: 处列出 pub 成员
ln_hex = next(i for i, l in enumerate(mlines) if "encoding::hex_encode" in l)
pk = req("textDocument/completion", {"textDocument": {"uri": muri},
                                     "position": {"line": ln_hex,
                                                  "character": mlines[ln_hex].index("encoding::") + len("encoding::")}})
pklabels = {i["label"] for i in (pk["result"] or [])}
check("completion pkg:: lists pub members",
      "hex_encode" in pklabels and "hex_decode" in pklabels
      and "base64_encode" in pklabels)

# native 内建签名：str_len 取类型标签；println 变参特例无签名
ln_sl = next(i for i, l in enumerate(mlines) if "str_len(" in l)
sigs = sigat(muri, ln_sl, mlines[ln_sl].index("str_len("))
check("signatureHelp native builtin type label",
      sigs is not None and sigs["signatures"][0]["label"] == "str_len(str) -> int"
      and sigs["activeParameter"] == 0)
ln_pl = next(i for i, l in enumerate(mlines) if l.strip() == "println(inner)")
sigp = sigat(muri, ln_pl, mlines[ln_pl].index("println(") + 8)
check("signatureHelp variadic native skipped", sigp is None)

# references：声明与使用互查，includeDeclaration 开关
def refs(ln, chv, inc):
    r = req("textDocument/references", {"textDocument": {"uri": muri},
                                        "position": {"line": ln, "character": chv},
                                        "context": {"includeDeclaration": inc}})
    return r["result"]

r_all = refs(ln_top, 4, True)
check("references from declaration = decl + 2 uses",
      r_all is not None and len(r_all) == 3)
ln_use1 = next(i for i, l in enumerate(mlines) if "topv + helper(1)" in l)
r_use = refs(ln_use1, mlines[ln_use1].index("topv"), True)
check("references from use finds the same set",
      r_use is not None and len(r_use) == 3)
r_exc = refs(ln_top, 4, False)
check("references excludes declaration", r_exc is not None and len(r_exc) == 2)

# documentSymbol：顶层声明清单与 SymbolKind
ds = req("textDocument/documentSymbol", {"textDocument": {"uri": muri}})
dsl = ds["result"] or []
dmap = {s["name"]: s["kind"] for s in dsl}
check("documentSymbol lists top-level decls",
      dmap.get("helper") == 12 and dmap.get("add3") == 12
      and dmap.get("main") == 12 and dmap.get("topv") == 14
      and len(dsl) == 4)

# documentHighlight：与 references 同一归一集合（决定用其中一个正确，
# 两者内容也该一致），一律带 kind
hl = req("textDocument/documentHighlight", {"textDocument": {"uri": muri},
                                            "position": {"line": ln_top, "character": 4}})
hll = hl["result"] or []
check("documentHighlight matches references count and carries kind",
      len(hll) == 3 and all(h["kind"] == 1 for h in hll)
      and {(h["range"]["start"]["line"], h["range"]["start"]["character"]) for h in hll}
      == {(r["range"]["start"]["line"], r["range"]["start"]["character"]) for r in r_all})

# prepareRename：光标落声明或使用处都给出同一枚符号的区间
def prep(ln, chv):
    r = req("textDocument/prepareRename", {"textDocument": {"uri": muri},
                                           "position": {"line": ln, "character": chv}})
    return r["result"]

prep_decl = prep(ln_top, 4)
check("prepareRename on declaration returns the `topv` span",
      prep_decl is not None and prep_decl["start"] == {"line": ln_top, "character": 4}
      and prep_decl["end"]["character"] - prep_decl["start"]["character"] == len("topv"))
prep_use = prep(ln_use1, mlines[ln_use1].index("topv"))
check("prepareRename on a use returns the same span shape",
      prep_use is not None
      and prep_use["end"]["character"] - prep_use["start"]["character"] == len("topv"))

# rename：WorkspaceEdit 覆盖声明与全部使用位置，newText 一致
ren = req("textDocument/rename", {"textDocument": {"uri": muri},
                                  "position": {"line": ln_use1, "character": mlines[ln_use1].index("topv")},
                                  "newName": "topv2"})
changes = (ren["result"] or {}).get("changes", {})
check("rename returns a WorkspaceEdit covering declaration and every use",
      list(changes.keys()) == [muri] and len(changes[muri]) == 3
      and all(e["newText"] == "topv2" for e in changes[muri]))

# native 内建：references/documentHighlight/prepareRename/rename 一律空结果
# （回归：native 全体共用 declare(..., Sp(0, 0))，未加判据会互相牵连）
ln_native = ln_pl
ch_native = mlines[ln_native].index("println") + 2
check("references on a native builtin is empty",
      refs(ln_native, ch_native, True) == [])
hl_native = req("textDocument/documentHighlight", {"textDocument": {"uri": muri},
                                                    "position": {"line": ln_native, "character": ch_native}})
check("documentHighlight on a native builtin is empty", hl_native["result"] == [])
check("prepareRename on a native builtin is null", prep(ln_native, ch_native) is None)
ren_native = req("textDocument/rename", {"textDocument": {"uri": muri},
                                         "position": {"line": ln_native, "character": ch_native},
                                         "newName": "whatever"})
check("rename on a native builtin is null", ren_native["result"] is None)

# 方法签名（receiver 剔除）：方法仅脚本模式可用，用单文件脚本 fixture；
# 每次检查只保留最后一份文档的录制，故脚本用例必须放最后
suri = "file:///tmp/burryn-lsp-verify-sig.bur"
ssrc = (
    "enum Point { Point(float, float) }\n"
    "\n"
    "fn (p: Point) dist() {\n"
    "    match p {\n"
    "        Point(x, y) => x * x + y * y,\n"
    "    }\n"
    "}\n"
    "\n"
    "let p0 = Point::Point(3.0, 4.0)\n"
    "println(p0.dist())\n"
)
send("textDocument/didOpen", {"textDocument": {"uri": suri, "languageId": "burryn",
                                               "version": 1, "text": ssrc}})
slines = ssrc.split("\n")
ln_dist = next(i for i, l in enumerate(slines) if "p0.dist()" in l)
sigm = sigat(suri, ln_dist, slines[ln_dist].index("p0.dist()"))
check("signatureHelp method script excludes receiver",
      sigm is not None and sigm["activeParameter"] == 0
      and sigm["signatures"][0]["label"] == "Point.dist()"
      and len(sigm["signatures"][0]["parameters"]) == 0)

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
