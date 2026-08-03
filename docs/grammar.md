# Burryn 语法（正式 grammar）

> 语法冻结于 S8.2（v0.5 里程碑）。本文档是语言表层语法的唯一权威描述，与 `compiler/frontend/` 的实现逐条对应。
> 冻结后语法变更须走修订流程：改本文档 → 改 parser/lexer → 全仓 `bur fmt` → 自举 fixpoint + parity 全绿 → 更新 SPEC 里程碑。

## 记号约定

- EBNF：`A ::= B | C`；`[x]` 可选；`{x}` 重复零或多次；`x+` 重复一次或多次；`x,` 表示逗号分隔列表（尾逗号允许，除非注明）
- 终止符：`'fn'` 关键字/符号；`ident` / `int` / `float` / `str` 词法类
- 语义约束在语法规则下方以「约束」标注

## 1. 词法

### 1.1 注释

```
comment      ::= '//' 行尾之前任意字符
block_comment ::= '/*' ... '*/'     // 无嵌套
```

注释作为 trivia 收集，不进入 token 流；`bur fmt` 按 span 重插。

### 1.2 标识符与关键字

```
ident        ::= [a-zA-Z_][a-zA-Z0-9_]*
```

**关键字（23 个，保留字，不可作标识符）：**

```
let  mut  const  fn  if  else  while  for  in  return
true  false  enum  match  spawn  break  continue  pub
import  select  record  type  defer
```

**两个"像关键字但不是关键字"的标识符（词法层特判，语法层语义由上下文决定）：**

| 名字 | 角色 | 说明 |
|---|---|---|
| `default` | `select` 的 default 臂 | 仅 `select` 臂位置特判（`t_ident` + lexeme 检查）；其他位置可作普通标识符 |
| `chan` | 内建函数 | `chan(n)` / `chan()` 缓冲/无缓冲 channel 构造，是 native 函数调用而非关键字 |

### 1.3 数字字面量

```
int          ::= [0-9]+                       // i64，溢出 trap
float        ::= [0-9]+ '.' [0-9]+            // f64
```

无十六进制/二进制/下划线分隔符。`.` 两侧必须有数字（`3.` 与 `.5` 均非法，歧义留给字段访问）。

### 1.4 字符串与插值

```
str          ::= '"' { string_char | '{' expression '}' | '{{' } '"'
string_char  ::= 转义序列 | 非 `"`、非 `{` 字符
escape       ::= '\n' | '\t' | '\"' | '\\'      // 唯一的四个转义
```

- 插值 `{expr}` 内必须是 `str` 值（非 str 编译错，提示 `str()`）
- `{{` 转义为字面 `{`
- 插值串由 `t_interp_start` / `t_interp_mid` / `t_interp_end` 三个 token 表示，花括号是 token 边界

### 1.5 符号（token 全表）

```
(  )  {  }  [  ]  ,  .  -  +  *  /  %  ;  ?  :  ::  !  !=  =
==  >  >=  <  <=  &&  ||  =>  <-  ->  |>  |  "  \n(自动分号)
```

**符号语义分工（冻结定案）：**

| 符号 | 语义 |
|---|---|
| `::` | 路径限定：包函数 `pkg::fn`、enum 变体 `Shape::Circle`、`pkg::Enum::Variant`。**永远是路径** |
| `.` | **永远是 record 字段访问**（`r.x`）。不得用于路径限定 |
| `=>` | match 臂、select 臂分隔 |
| `->` | 函数签名返回类型标注（S7.8） |
| `<-` | channel 接收（表达式前缀）与 send 语句分隔符（`ch <- v`） |

**`<-` 消歧规则（词法层 longest-match，冻结定案）**：`<` 后紧跟 `-`（无空格）无条件合成 `<-` token。因此「小于负值」必须加空格写成 `x < -5`；`x<-5` 解析为 `<-`（send/recv 语义）而非比较。`--` 不是 token：`5--5` 是 `5 - -5`。
| `|>` | 管道 |
| `|` | record 更新基底（`record { r | x: 3 }`）与行变量（类型层 `record { x: int | r }`） |
| `?` | `Result`/`Option` 解包早退 |
| `\n` | 语句结束（见 §1.6） |

### 1.6 自动分号插入

- 换行在以下 token 之后插入分号：`ident`、`str`、`int`、`float`、`true`、`false`、`)`、`]`、`}`、`?`、`return`、`break`、`continue`
- 显式 `;` 等同换行
- `} else`、`} else if` 必须同行（`}` 是语句结束 token，换行会插入分号）

## 2. 程序结构

```
program      ::= { top_level_decl }
top_level_decl ::= import | fn_decl | enum_decl | let_decl | const_decl
                | type_decl | pub_decl
```

- 包：目录即包，`bur.mod` 声明 `module <path>`；`require` 声明依赖
- `import` 只能在顶层：`import [ident] '"' import_path '"'`（可选别名，默认别名 = 路径末段）

## 3. 声明

```
fn_decl      ::= 'fn' ident '(' params ')' ['->' type_expr] block
params       ::= { ['mut'] ident [':' type_expr] },
let_decl     ::= ['pub'] 'let' ['mut'] ident '=' expression
const_decl   ::= ['pub'] 'const' ident '=' const_expr
enum_decl    ::= ['pub'] 'enum' ident ['(' type_param {',' type_param} ')'] '{' variants '}'
type_param   ::= ident
variants     ::= { ident ['(' type_expr {',' type_expr} ')'] },
type_decl    ::= ['pub'] 'type' ident '=' type_expr
pub_decl     ::= 'pub' ( fn_decl | enum_decl | let_decl | const_decl | type_decl )
```

- **零标注**：参数/返回类型可省略（S7.8）；`interface` 文件（S6.1）要求显式标注
- **`const` 初始化式**：编译期可折叠（字面量、const 引用、算术/比较/bool、str 拼接），折叠溢出 = 编译错；不可 `mut`
- 顶层 `let` 可 `pub`；`pub` 导出标记，不用首字母大写

## 4. 语句

```
statement    ::= let_decl | const_decl | fn_decl | enum_decl | type_decl
               | pub_decl | import_decl | while_stmt | for_stmt | spawn_stmt
               | select_stmt | defer_stmt | return_stmt | break_stmt
               | continue_stmt | expr_stmt | assign_stmt | send_stmt
while_stmt   ::= 'while' expression block
for_stmt     ::= 'for' ident 'in' expression block       // list 或 channel 迭代
spawn_stmt   ::= 'spawn' expression                     // 起纤程
defer_stmt   ::= 'defer' expression                     // 挂包围函数，退出 LIFO
return_stmt  ::= 'return' [expression]
break_stmt   ::= 'break'
continue_stmt::= 'continue'
assign_stmt  ::= ( ident | index ) '=' expression      // 目标必须是变量或下标
send_stmt    ::= expression '<-' expression            // ch <- v
select_stmt   ::= 'select' '{' { select_arm } '}'
select_arm   ::= ( '<-' expression | ident '=' '<-' expression
                 | expression '<-' expression | 'default' ) '=>' expression,
```

**select 细节（冻结定案）：**
- 臂形态：`<-ch => ...`（接收丢弃）、`v = <-ch => ...`（接收绑定，左侧必须是裸名）、`ch <- v => ...`（发送）、`default => ...`（非阻塞；至多一个）
- `default` 是特判标识符，非关键字
- 臂体是表达式（block 是表达式的一种），`=>` 之后可有逗号

**for 迭代 channel**：`for x in ch` 排空 channel，关闭后结束。

## 5. 表达式（优先级从低到高）

```
expression   ::= pipe_expr
pipe_expr    ::= or_expr { '|>' pipe_target }
pipe_target  ::= path ['(' args ')']                  // path = ident {'::' ident}，至多三段
or_expr      ::= and_expr { '||' and_expr }
and_expr     ::= equality { '&&' equality }
equality     ::= comparison { ('==' | '!=') comparison }
comparison   ::= term { ('<' | '<=' | '>' | '>=') term }
term         ::= factor { ('+' | '-') factor }
factor       ::= unary { ('*' | '/' | '%') unary }
unary        ::= ('!' | '-') unary | postfix
postfix      ::= primary { '::' ident | '.' ident | '[' expression ']' | '(' args ')' | '?' }
primary      ::= int | float | str | interp_str | 'true' | 'false'
               | ident | path | record_literal | 'record' type_expr
               | 'if' expression block ['else' (block | if_expr)]
               | 'match' expression '{' match_arms '}'
               | '(' expression ')' | lambda
path         ::= ident {'::' ident}                    // 至多三段
lambda       ::= 'fn' '(' params ')' block            // 体必须是 block
match_arms   ::= { pattern ['if' expression] '=>' expression },
record_literal ::= 'record' '{' [fields | update] '}'
```

**postfix 语义分工（冻结定案）：**

| postfix | 语义 |
|---|---|
| `x :: y` | 路径：`pkg::fn`（包函数）、`Shape::Circle`（变体）、`pkg::Enum::Variant`（三段）；左侧必须可解析为包别名或 enum 名，否则编译错 |
| `x . y` | **字段访问**：`x` 必须是 record 类型的表达式 |
| `x [ i ]` | list/str 下标（str 按字节）；`i` 越界 trap |
| `x ( args )` | 函数/闭包调用、enum 构造器调用 |
| `x ?` | 解包 `Result`/`Option`：`Err`/`None` 立即返回 |

**约束：**
- `|>` 左结合、最低优先级；RHS 是路径 + 可选参数列表（`lhs |> pkg::fn(a)`）；`?` 绑定紧于 `|>`
- 管道目标路径至多三段；括号表达式不能作管道目标
- 赋值/`+=` 等复合赋值不存在（只有 `=`）

## 6. 模式（match 臂）

```
pattern      ::= '_' | ident | int | float | str | path ['(' {pattern} ')']
path         ::= ident {'::' ident}
```

- `_` 通配；裸 `ident` 绑定；`Shape::Circle(r)` 变体解构；`pkg::Enum::Variant(...)` 跨包
- 变体模式字段数必须与声明一致（E0023）
- 穷尽性：缺臂 = E0004 编译错

## 7. 类型表达式

```
type_expr    ::= ident [':' constraint] | path ['(' type_args ')'] | '[' type_expr ']'
               | '(' {type_expr} ')' '->' type_expr | record_type
constraint   ::= 'num' | 'addord' | 'key' | 'int' | 'float' | 'str'
record_type  ::= 'record' '{' { ident ':' type_expr } ',' [ '|' ident ] '}'
```

- 类型参数：`[a]`（list）；`fn` 类型 `(T1, T2) -> T3`
- 约束变量（S7.8）：`fn f(a: addord)` 小写类型参数可带约束
- **record 类型**：字段表 `{ x: int, y: str }`；行变量 `| r`（S8.3 row poly 预留，值层无行变量）
- enum 类型引用：`Shape`（本包）、`pkg::Shape`（跨包）

## 8. record（冻结定稿）

```
record_literal  ::= 'record' '{' [ field {',' field} [','] ] '}'
                  | 'record' '{' ident '|' field {',' field} [','] '}'   // 更新
field           ::= ident ':' expression
record_type     ::= 'record' '{' [ type_field {',' type_field} [','] [ '|' ident ] ] '}'
type_field      ::= ident ':' type_expr
access          ::= postfix '.' ident               // r.x
```

**冻结定案（S8.2）：**
- 字面量必须带 `record` 关键字（与 block 区分，SPEC §2 定案）
- 更新 `record { r | x: 3 }`：以 `r` 为基底复制、覆写字段，生成新 record；**基底必须是裸 ident**（表达式先绑定到变量再更新）
- 空 record `record {}` 合法（字面量与类型皆可）
- 字段名重复（`record { x: 1, x: 2 }`）：语法层合法，行为由类型检查实现决定（冻结时记录为已知行为，无专门错误码）

## 9. 变更流程（冻结后）

1. 语法变更 = 改本文件 + parser/lexer + 全仓 `bur fmt` + 自举 fixpoint + parity 全绿
2. 语义变更（不改表层语法）不触碰本文件，但须更新 SPEC
3. LSP（S9）以本文件为语法契约，语法变更须同步 TextMate grammar 与 LSP 词法
