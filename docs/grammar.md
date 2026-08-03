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
int          ::= '0x' hex_digit+ | '0b' bin_digit+ | digit+        // i64，溢出 trap
float        ::= digit+ '.' digit+ [ ('e'|'E') ['+'|'-'] digit+ ]  // f64
digit_sep    ::= 数字之间允许 '_'（lexer 剥离，`1_000_000` = 1000000）
```

- hex/二进制前缀 `0x`/`0b`（大写 `0X`/`0B` 亦可），parser 端转换；`0x`/`0b` 后必须有数字
- 指数 `e`/`E` 后可选 `+`/`-`；前瞻判断避免吞掉标识符（`1e`、`1ex` 不是指数）
- `_` 必须在数字之间（`1_`、`_5`、`1__2` 均报 E1005）
- `.` 两侧必须有数字（`3.` 与 `.5` 均非法，歧义留给字段访问）

### 1.4 字符串与插值

```
str          ::= '"' { string_char | '{' expression '}' | '{{' } '"'
ml_str       ::= '"""' '\n' { ml_line } '"""'          // 多行字符串
ml_line      ::= 行首空白 { '\' 内容到行尾 | 空白行 }
string_char  ::= 转义序列 | 非 `"`、非 `{` 字符
escape       ::= '\n' | '\t' | '\"' | '\\' | '\r'      // 唯一的五个转义
```

- 插值 `{expr}` 内必须是 `str` 值（非 str 编译错，提示 `str()`）
- `{{` 转义为字面 `{`
- 插值串由 `t_interp_start` / `t_interp_mid` / `t_interp_end` 三个 token 表示，花括号是 token 边界
- **多行字符串（冻结定案）**：`"""` 后必须换行；每行行首空白忽略后**第一个非空白字符必须是 `\`**（内容标记），`\` 后到行尾为该行内容；全空白行 = 空行；内容 = 各行以 `\n` 连接、最后一行无尾换行；行内转义与插值照常；`"""` 结束行前导空白忽略

### 1.5 符号（token 全表）

```
(  )  {  }  [  ]  ,  .  ..  -  +  *  /  %  ;  ?  :  ::  !  !=  =
==  >  >=  <  <=  &&  ||  =>  <-  ->  |>  |  +=  -=  *=  /=  %=  "  \n(自动分号)
```

**符号语义分工（冻结定案）：**

| 符号 | 语义 |
|---|---|
| `::` | 路径限定：包函数 `pkg::fn`、enum 变体 `Shape::Circle`、`pkg::Enum::Variant`。**永远是路径** |
| `.` | 成员访问：record 字段（`r.x`）或方法调用（`p.area()`，checker 按类型解析）。不得用于路径限定 |
| `..` | range 字面量 `1..10`（半开区间），脱糖为 `range(1, 10)` 调用 |
| `=>` | match 臂、select 臂分隔 |
| `->` | 函数签名返回类型标注（S7.8） |
| `<-` | channel 接收（表达式前缀）与 send 语句分隔符（`ch <- v`） |
| `\|>` | 管道 |
| `\|` | or-pattern（match 臂 `A \| B =>`）、record 更新基底、行变量（类型层） |
| `?` | `Result`/`Option` 解包早退 |
| `+= -= *= /= %=` | 复合赋值：`x += v` 脱糖为 `x = x + v`（目标须为变量或下标） |
| `\n` | 语句结束（见 §1.6） |

**`<-` 消歧规则（词法层 longest-match，冻结定案）**：`<` 后紧跟 `-`（无空格）无条件合成 `<-` token。因此「小于负值」必须加空格写成 `x < -5`；`x<-5` 解析为 `<-`（send/recv 语义）而非比较。`--` 不是 token：`5--5` 是 `5 - -5`。

### 1.6 自动分号插入

- 换行在以下 token 之后插入分号：`ident`、`str`、`int`、`float`、`true`、`false`、`)`、`]`、`}`、`?`、`return`、`break`、`continue`
- 显式 `;` 等同换行
- `} else`、`} else if` 必须同行（`}` 是语句结束 token，换行会插入分号）

## 2. 程序结构

```
program      ::= { top_level_decl }
top_level_decl ::= import | fn_decl | method_decl | enum_decl | let_decl | const_decl
                | type_decl | pub_decl
```

- 包：目录即包，`bur.mod` 声明 `module <path>`；`require` 声明依赖
- `import` 只能在顶层：`import [ident] '"' import_path '"'`（可选别名，默认别名 = 路径末段）

## 3. 声明

```
fn_decl      ::= 'fn' ident '(' params ')' ['->' type_expr] block
method_decl  ::= 'fn' '(' ident ':' type_expr ')' ident '(' params ')' ['->' type_expr] block
params       ::= { ['mut'] ident [':' type_expr] },
let_decl     ::= ['pub'] 'let' ['mut'] ident [':' type_expr] '=' expression
destruct_let ::= 'let' ['mut'] '(' { ident } ',' ')' '=' expression
const_decl   ::= ['pub'] 'const' ident [':' type_expr] '=' const_expr
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
for_stmt     ::= 'for' [ident ','] ident 'in' expression block   // 可选索引变量；channel 禁用索引
label_stmt   ::= ident ':' ( while_stmt | for_stmt )             // 标签循环（Go 式）
spawn_stmt   ::= 'spawn' expression                     // 起纤程
defer_stmt   ::= 'defer' expression                     // 挂包围函数，退出 LIFO
return_stmt  ::= 'return' [expression]
break_stmt   ::= 'break' [ident]                        // 可选标签
continue_stmt::= 'continue' [ident]                     // 可选标签
assign_stmt  ::= ( ident | index ) ('=' | '+=' | '-=' | '*=' | '/=' | '%=') expression
send_stmt    ::= expression '<-' expression            // ch <- v
select_stmt   ::= 'select' '{' { select_arm } '}'
select_arm   ::= ( '<-' expression | ident '=' '<-' expression
                 | expression '<-' expression | 'default' ) '=>' expression,
```

**select 细节（冻结定案）：**
- 臂形态：`<-ch => ...`（接收丢弃）、`v = <-ch => ...`（接收绑定，左侧必须是裸名）、`ch <- v => ...`（发送）、`default => ...`（非阻塞；至多一个）
- `default` 是特判标识符，非关键字
- 臂体是表达式（block 是表达式的一种），`=>` 之后可有逗号

**for 迭代 channel**：`for x in ch` 排空 channel，关闭后结束；索引形式（`for i, x in ch`）报 E0599。

**标签循环（冻结定案）**：`label: while/for` 起标签；`break label` / `continue label` 跳转到该标签循环（可跨嵌套层）；无标签 `break`/`continue` 作用于最近循环；标签不可跨函数；未定义标签 = E2018。

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
               | '(' expression ')' | tuple_lit | lambda
tuple_lit    ::= '(' expression ',' { expression ',' } [expression] ')'   // ≥2 元素
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
- **range**：`a..b`（半开 [a, b)）优先级低于 `||`，脱糖为 `range(a, b)`；`1..2+3` = `range(1, 2+3)`
- **方法调用**：`obj.method(args)` —— `.` 后是方法名 + 调用参数；checker 按 obj 类型查方法表，receiver 作首参编译为全局函数调用；非方法名回退字段访问

## 6. 模式（match 臂）

```
pattern      ::= or_pattern
or_pattern   ::= simple { '|' simple }              // 展开为多个臂（同一 guard + body）
simple       ::= '_' | ident | int | float | str | path ['(' {pattern} ')'] | record_pattern
record_pattern ::= 'record' '{' { ident } ',' '}'  // 解构字段到同名绑定
path         ::= ident {'::' ident}
```

- `_` 通配；裸 `ident` 绑定；`Shape::Circle(r)` 变体解构；`pkg::Enum::Variant(...)` 跨包
- **or-pattern**：`A | B => body` 在 parser 展开为两个臂；绑定一致性由 checker 保证（None 臂引用 Some 的绑定会报错）
- **record 模式**：`record { x, y }` 解构字段到同名绑定（字段子集合法）；非 record 值 = E0609；字段不存在 = E0609
- 变体模式字段数必须与声明一致（E0023）
- 穷尽性：缺臂 = E0004 编译错

## 7. 类型表达式

```
type_expr    ::= ident [':' constraint] | path ['(' type_args ')'] | '[' type_expr ']'
               | '(' {type_expr} ')' '->' type_expr | tuple_type | record_type
constraint   ::= 'num' | 'addord' | 'key' | 'int' | 'float' | 'str'
tuple_type   ::= '(' type_expr ',' { type_expr ',' } ')'       // ≥2 元素
record_type  ::= 'record' '{' { ident ':' type_expr } ',' [ '|' ident ] '}'
```

- 类型参数：`[a]`（list）；`fn` 类型必须带 `fn` 关键字（`fn (T1, T2) -> T3`）；裸括号 `(T1, T2)` 是 tuple 类型
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

## 9. interface 文件语法（S6.1 工具链格式）

```
interface    ::= { import_decl | enum_decl | pub_interface_fn }
pub_interface_fn ::= 'pub' 'fn' ident '(' { ident ':' type_expr } ',' ')' '->' type_expr
```

- interface 文件是包导出签名（checker 缓存格式），**参数与返回类型必须显式标注**（E1101）
- 无函数体；`pub` 标记导出

## 10. 变更流程（冻结后）

1. 语法变更 = 改本文件 + parser/lexer + 全仓 `bur fmt` + 自举 fixpoint + parity 全绿
2. 语义变更（不改表层语法）不触碰本文件，但须更新 SPEC
3. LSP（S9）以本文件为语法契约，语法变更须同步 TextMate grammar 与 LSP 词法
