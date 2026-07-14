# drift CLI 命令设计

## 设计原则

- **无 Git 术语** — 用创作者的日常语言：保存、恢复、分支，而非 commit/rebase/reset
- **零暂存区** — `save` 自动捕获所有变更，不要求用户手动选择文件
- **命令精简** — 每个命令只做一件明确的事，避免像 git 那样一个命令有几十个 flag
- **不安全操作不暴露** — 没有 `reset --hard`，恢复永远可撤销；`save` 可 `undo`
- **GUI 为视觉主力** — 缩略图时间线在 GUI 中展现，CLI 专注快速、精确、可脚本化操作
- **UTF-8 全集** — 用户内容（message、tag、分支名）必须支持 UTF-8 全集，包括中文、日文等任意 Unicode 文本；CLI 输出的结构符号优先使用 ASCII 以保证跨终端兼容性

---

## 全局选项

所有命令共享以下选项，必须在子命令参数之前使用：

| 选项 | 说明 |
|------|------|
| `-C, --cwd <path>` | 在指定目录执行命令（而非当前工作目录）。对脚本和 GUI 调用至关重要 |
| `--json` | 以 JSON 格式输出。所有支持 `--json` 的命令输出结构一致（见"输出格式规范"） |
| `-q, --quiet` | 静默模式，只输出错误。退出码仍是判定成功的权威来源 |

示例：
```
drift -C ~/Documents/my-novel status
drift log --json
drift -q save -m "auto checkpoint"
```

---

## 输出格式规范

所有命令输出遵循统一的设计标准，确保可读性和脚本友好性。

### 结构层级

```
>>> <动作> [状态]              ← 状态行：命令做了什么 + 结果
                               ← 空行（核心内容前）
  <核心内容>                   ← 命令的主要输出，缩进 2 空格
                               ← 空行（总结前）
  <总结行>                     ← 一行统计信息
  <提示行>                     ← 可选，hint: 开头的操作建议
```

### 状态标识

| 标识 | 含义 | 示例 |
|------|------|------|
| `[ok]` | 成功 | `>>> Saved [12ab] [ok]` |
| `[failed]` | 失败 | `>>> Save [failed]` |
| `[warning]` | 部分成功 | `>>> Check [warning]` |
| `[active]` | 运行中 | `>>> Watching [active]` |

### 文件变更标记

| 符号 | 含义 |
|------|------|
| `+` | 新增 |
| `-` | 删除 |
| `~` | 修改 |

### 总结行格式

```
  N files: +A ~M -D
```

### 错误格式

```
>>> <动作> [failed]
Error: <错误描述>
  hint: <解决建议>
```

### ASCII 与 Unicode 使用规则

- **结构符号用 ASCII**：状态行 `>>>`、箭头 `->`、变更标记 `+/-/~`、状态标识 `[ok]`/`[failed]` 等。保证在任意终端（含老 CMD、PowerShell 默认编码、SSH 非 UTF-8 locale）下稳定显示
- **用户内容用 UTF-8 全集**：message、tag、分支名、文件路径支持任意 Unicode 文本（中文、日文、emoji 等）。程序在 Windows 上需主动设置 console output 为 UTF-8（CP_UTF8），保证中文等非 ASCII 内容正确显示

### 命令分类

| 类型 | 命令 | 输出特点 |
|------|------|---------|
| 执行 | init, save, restore, export, import, branch, switch, ignore, tag, undo, clone | 状态行 + 文件列表 + 总结 |
| 查询 | log, show, status, diff, check, config, version, ls-remote | 状态行 + 查询结果 + 总结 |
| 驻留 | watch | 状态行 + 实时日志流 + 结束总结 |
| 工具 | upgrade | 状态行 + 升级摘要 |

### JSON 输出规范

`--json` 模式下所有命令输出统一的信封结构：

```json
{
  "command": "save",
  "status": "ok",
  "data": { ... },
  "hint": null
}
```

- `command`：命令名
- `status`：`ok` / `failed` / `warning` / `active`
- `data`：命令特定的结构化数据（每个命令的 schema 见各命令章节）
- `hint`：可选的提示字符串，失败时通常非空

---

## 命令全景

```
drift
├── init            初始化项目
├── save            保存当前状态为快照
├── undo            撤销最后一次 save（保持"恢复永远可撤销"的对称性）
├── log             浏览历史快照
├── show            查看快照内容（文件列表或单文件）
├── status          查看变更情况
├── diff            比较差异
├── restore         恢复文件到指定快照（自动备份）
├── export          导出快照为 zip 归档
├── import          从其他分支导入单个文件到当前工作区
├── branch          分支管理（list/create/delete/rename 子命令）
├── switch          切换分支
├── tag             标签管理（list/add/delete/rename 子命令）
├── ignore          忽略规则管理（list/add/remove 子命令）
├── watch           后台监听（on/off/status/pause/resume 子命令）
├── check           校验数据完整性
├── gc              回收无引用的快照与块
├── config          配置管理（get/set/list）
├── remote          管理远程存储后端（add/remove/list/rename/set-url/show/test）
├── push            上传本地对象到远程（快照/块/refs）
├── pull            下载远程对象到本地（快照/块/refs）
├── clone           克隆远程仓库到本地新目录
├── ls-remote       列出远程仓库的 refs（不下载对象）
├── version         查看版本与构建信息
├── upgrade         自升级到最新发布版
└── help            帮助信息
```

> 远程同步原计划为单一 `sync` 命令，实际拆分为 `push` + `pull` 两个方向独立的命令，语义更清晰、错误路径更可控。详见"远程同步"章节。

---

## 版本引用语法

所有接受 `<snapshot-id>` 参数的命令（show、diff、restore、log 等）统一使用以下语法：

| 语法 | 含义 | 示例 |
|------|------|------|
| `id:<hash-prefix>` | 按快照哈希前缀定位（至少 4 位） | `id:12ab` |
| `tag:<name>` | 按 tag 定位 | `tag:submission` |
| `branch:<name>` | 按分支定位（取分支头快照） | `branch:main` |
| `head` | 当前 HEAD 指向的快照 | `head` |
| `<name>`（裸名） | 等价于 `branch:<name>`，见解析规则 2 | `main`、`dev` |

**解析规则**：
1. 带前缀（`id:`/`tag:`/`branch:`）的引用按前缀分派，无歧义
2. 裸名按 `branch:<name>` 解析。分支名是用户自定义的可读名称，不会与机器生成的 hash 冲突，因此裸写安全。Tag 和 hash 必须显式带前缀
3. 不支持"裸 hash 前缀"（如 `12ab`）—— 必须写 `id:12ab`，消除分支名与 hash 前缀冲突的歧义
4. `head` 是保留关键字（不能用作分支/tag 名），解析为当前 HEAD 快照

**NFC 规范化**：tag 名和分支名在存储前进行 Unicode NFC 规范化，避免同形异码问题（如 `é` 的两种编码视为同一个名字）。

---

## 各命令详细设计

### `drift init`

```
drift init [path]

在当前目录（或指定 path）初始化 drift 项目。

示例：
  drift init
  drift init ~/Documents/my-novel
  drift -C ~/Projects init
```

Output：

```
>>> Initialized [ok]
/Users/me/Documents/my-novel/.drift/
```

Error（已有仓库）：

```
>>> Init [failed]
Error: already a drift repository.
  hint: use 'drift status' to check current state.
```

- 创建 `.drift/` 目录
- `.driftignore` **不会**自动创建（与 git 一致）；用户通过 `drift ignore add <pattern>` 按需添加规则，文件在首次 add 时创建

---

### `drift save`

```
drift save [-m <message>] [--tag <name>...]

保存当前所有变更，创建一个新快照。

选项：
  -m, --message    快照消息（可选；省略时使用默认消息 "snapshot <timestamp>"）
  --tag            为这个快照起一个或多个固定别名，可重复：--tag "v1" --tag "交稿"

示例：
  drift save -m "Chapter 3 draft complete"
  drift save -m "Update cover" --tag "submission" --tag "v1"
  drift save                              # 快速存档，使用默认消息
  drift save -m "第三章初稿完成"            # 中文消息
```

Output — 带 message：

```
>>> Saved [12ab] [ok]
Chapter 3 draft complete

  +  chapter4.md      12.3 KB
  +  sketch.png        2.1 MB
  ~  chapter3.md      45.2 KB

  3 files: +2 ~1
```

Output — 无 message（快速存档）：

```
>>> Saved [12ab] [ok]
[no message] snapshot 2026-07-06 14:30

  +  chapter4.md      12.3 KB

  1 file: +1
```

Output — 带 tag：

```
>>> Saved [9f1e] [ok]
Submit to client  [submission] [v1]

  +  chapter4.md      12.3 KB
  ~  chapter3.md      45.2 KB

  2 files: +1 ~1
```

Error（无变更）：

```
>>> Save [failed]
Error: nothing to save.
  hint: modify some files first to create a meaningful checkpoint.
```

- 自动扫描所有变更的文件（新增、修改、删除）
- 对新增的大文件进行 CDC 分块，只存储变化的块
- 对图片类文件自动生成缩略图并缓存（供 GUI 使用）
- 与 `drift watch` 自动保存不同：手动 save 代表有意义的检查点
- `-m` 可选：省略时使用默认消息，便于快速存档；用户可后续用 `drift tag` 或未来的 `drift log --edit-message` 补充
- `--tag` 可重复，支持一次打多个 tag

---

### `drift undo`

```
drift undo

撤销最后一次 save。HEAD 回退到 PrevID，被撤销的快照标记为不可达（下次 gc 清理）。

这是 save 的逆操作，保证"用户主动操作也可撤销"——与 restore 自动备份的"恢复可撤销"精神一致。

示例：
  drift undo
```

Output — 撤销成功：

```
>>> Undone [ok]
Removed snapshot 12ab ("Chapter 3 draft complete").
HEAD now at a3c2 ("Update cover color scheme").

  hint: the undone snapshot is now unreachable. It will be removed by 'drift gc'.
```

Output — 连续撤销（撤销倒数第二次）：

```
>>> Undone [ok]
Removed snapshot a3c2 ("Update cover color scheme").
HEAD now at 9f1e ("Submit to client").
```

Error — 没有可撤销的快照：

```
>>> Undo [failed]
Error: no snapshot to undo.
  hint: HEAD is already at the initial snapshot.
```

Error — 工作区有未保存变更：

```
>>> Undo [failed]
Error: uncommitted changes would be lost.
  hint: use 'drift save' or 'drift restore' first.
```

- 撤销的是 HEAD 的前移，不影响工作区文件
- 如果工作区有未保存变更，拒绝执行（避免丢失）
- 被撤销的快照在 gc 前仍可通过 `id:<hash>` 访问（恢复误撤销）

---

### `drift log`

```
drift log [--limit <n>] [--detail <id>] [--all] [--branch <name>] [--json]

浏览历史快照。默认只显示当前分支的可达历史（沿 PrevID 链回溯，包含从
父分支继承的提交）。自动保存 (`drift watch`) 的 [auto] 快照默认隐藏。

选项：
  -l, --limit      显示最近 N 条记录（默认 30）
  --detail <id>    查看某个快照的文件变更明细（替代旧的 -v）
  --all            显示所有分支的全部快照（含自动保存）
  --branch <name>  显示指定分支的可达历史（默认：当前分支）
  --json           JSON 格式输出

示例：
  drift log
  drift log -l 20
  drift log --detail id:12ab
  drift log --all
  drift log --branch feature
  drift log --json
```

Output — 默认（当前分支）：

```
>>> History (4 snapshots on 'dev')
fc83  2026-07-07 20:34  dev          commit 4 on dev                            ~1
d536  2026-07-07 20:34               commit 3 on dev                            ~1
aa89  2026-07-07 20:34  main         commit 2 on main                           ~1
37f4  2026-07-07 20:34               commit 1 on main                           +1
```

> 第三列为分支名，**仅在该快照是某分支头（tip）时才显示**（类似 git
> `--decorate=short`）。继承自父分支的提交该列为空，用户一眼就能看出分支
> 在哪里切出——上例中 `main` 标在 `aa89`，说明 dev 是从 main 的 `aa89`
> 处切出的；之后两条是 dev 独有。

多个分支头指向同一快照时用逗号分隔，超长截断为 `name1,name2,+N`：

```
b4e1  2026-07-07 22:15  main,dev     Shared edit                                ~1
```

消息或标签过长时自动截断，末尾加 `...`：

```
>>> History (3 snapshots on 'main')
12ab  2026-07-07 16:30  main         Chapter 3 draft complete, revised by editor...  +2 ~1
b4e1  2026-07-07 22:15  dev          Fix typo                        [typo-fix-...]  ~1
```

> 被截断的完整内容可通过 `drift log --detail id:<id>` 查看。

**Tag 列来源**：tag 列只反映 `tags/<name>` 引用——`drift save --tag` 在保存后创建
对应 ref，`drift tag add` 事后追加 ref，二者写入同一处。`tag delete` / `tag rename`
直接修改 ref，因此对 log 视图立即生效。多个 tag 同指一个快照时，列宽足够显示
`[v1.0,v2.0]`，超长截断为 `[v1.0,+N]`；`--json` 输出始终返回完整 tag 数组。

```
7cd8  2026-07-07 20:45  main         release                                [release,+2]  +1
```

> 兼容性：早期版本会把 tag 写进快照内嵌字段，导致 `tag delete`/`rename` 无法改写
> 历史。新版本不再写入内嵌字段，tag 仅以 ref 形式存在；log 层通过 `mergeTags`
> 合并旧快照内嵌字段与当前 refs，保证历史数据仍可读。

Output — `--branch main`（指定分支）：

```
>>> History (2 snapshots on 'main')
aa89  2026-07-07 20:34  main          commit 2 on main                           ~1
37f4  2026-07-07 20:34               commit 1 on main                           +1
```

> 只显示 main 分支可达的提交，dev 独有提交不会出现。

Output — `--all`（全部分支）：

```
>>> History (4 snapshots, all branches)
fc83  2026-07-07 20:34  dev           commit 4 on dev                            ~1
d536  2026-07-07 20:34               commit 3 on dev                            ~1
aa89  2026-07-07 20:34  main          commit 2 on main                           ~1
37f4  2026-07-07 20:34               commit 1 on main                           +1
```

> 显示所有快照（含 auto-saves），分支列同样只标注 tip。

Output — `--detail id:<id>`：

```
>>> Snapshot 12ab
2026-06-28 16:30  Chapter 3 draft complete

  +  chapter4.md      12.3 KB
  +  sketch.png        2.1 MB
  ~  chapter3.md      45.2 KB  (42 lines)

  3 files: +2 ~1
```

Output — `--json`：

```json
{
  "command": "log",
  "status": "ok",
  "data": {
    "snapshots": [
      {"id":"12ab","time":"2026-06-28T16:30:00","message":"Chapter 3 draft complete","tags":[],"branch":"main","changes":"+2 ~1 -0"},
      {"id":"a3c2","time":"2026-06-27T22:15:00","message":"Update cover color scheme","tags":[],"branch":"main","changes":"+0 ~1 -0"},
      {"id":"9f1e","time":"2026-06-27T10:00:00","message":"Submit to client","tags":["submission"],"branch":"dev","changes":"+1 ~1 -0"}
    ]
  },
  "hint": null
}
```

Error：

```
>>> Log [failed]
Error: no snapshots yet.
  hint: use 'drift save -m "message"' to create your first snapshot.
```

> 缩略图仅供 GUI 时间线视图使用，CLI 中不展示。

---

### `drift show`

```
drift show [<snapshot-id>] [<file>] [--open]

查看指定快照的内容。
- 无参数：显示帮助
- 仅 snapshot-id：列出该快照包含的文件清单
- snapshot-id + file：显示文件内容（文本）或元信息（二进制/图片）
- 单个非版本引用参数：作为文件路径，隐式取 `head` 版本，等价于 `drift show head <file>`

> 单文件参数不会与“裸名按分支解析”规则冲突：文件路径必含 `.` 或 `/`，而分支名不允许包含这些字符。

选项：
  --open    用系统默认程序打开文件

示例：
  drift show id:12ab                         # 列出快照文件清单
  drift show id:12ab chapter1.md             # 查看文本文件内容
  drift show tag:submission cover.psd        # 查看二进制文件元信息
  drift show tag:submission cover.psd --open # 用系统程序打开
  drift show README.md                       # 单文件参数隐式 head：等价于 drift show head README.md
  drift show main README.md                   # 裸名按分支解析（等价于 branch:main）
```

Output — 仅 snapshot-id（文件清单）：

```
>>> Snapshot id:12ab (3 files)

  chapter1.md       4.2 KB   text
  chapter4.md      12.3 KB   text
  sketch.png        2.1 MB   image (4200x3150)

  3 files
```

Output — 文本文件：

```
>>> File id:12ab:chapter1.md

# Chapter 1: The Beginning
The sun rose over the quiet village...
```

Output — 二进制文件：

```
>>> File tag:submission:cover.psd
  Size:       23.4 MB
  Modified:   06-28 16:30

  hint: use --open to view with system program.
```

Output — 图片文件（额外显示尺寸）：

```
>>> File id:12ab:cover.png
  Size:       2.1 MB
  Dimensions: 4200x3150
  Modified:   06-28 16:30

  hint: use --open to view with system program.
```

Output — `--open`：

```
>>> Opening [ok]
Launched system viewer for tag:submission:cover.psd.
```

Error：

```
>>> Show [failed]
Error: 'cover.psd' not found in snapshot id:12ab.
  hint: use 'drift show id:12ab' to list files in this snapshot.
```

- `show <snapshot-id>` 列出文件清单，不再需要 `log --detail`
- `show <snapshot-id> <file>` 显示文件内容

---

### `drift status`

```
drift status [--short]

查看自上次 save 以来的变更情况。列出所有新增、修改、删除的文件。

选项：
  -s, --short    仅显示文件路径

示例：
  drift status
  drift status -s
  drift --json status
```

Output：

```
>>> Status (3 files changed since last save)
On branch: main

  +  chapter4.md
  +  assets/sketch.png
  ~  chapter3.md

  3 files: +2 ~1
```

Output — `--short`（仅文件路径，供脚本解析）：

```
>>> Status (3 files)
chapter4.md
assets/sketch.png
chapter3.md
```

Output — 无变更：

```
>>> Status [ok]
On branch: main
Nothing changed since last save.
```

Output — 分离头指针（detached HEAD）：

```
>>> Status [ok]
HEAD detached
Nothing changed since last save.
```

- 默认输出第二行始终展示当前分支（`On branch: <name>`）或分离头状态（`HEAD detached`），方便用户随时确认所在分支。
- `--short` 模式保持纯路径输出，不显示分支行；分支信息可通过 `--json` 的 `branch` 字段获取。

Error：

```
>>> Status [failed]
Error: not a drift repository.
  hint: use 'drift init' to create one.
```

- 状态字母含义：`+` Added  `-` Deleted  `~` Modified
- 重命名目前显示为删除 + 新增对，后续版本支持 R 标记

---

### `drift diff`

```
drift diff [--stat] [<base>] [<target>] [-- <file>]

显示差异。
- 无参数：工作区 vs HEAD
- 1 个参数：工作区 vs 指定快照
- 2 个参数：两个快照之间
- `-- <file>`：限定单文件，文本输出 unified diff，二进制输出元信息变化

`--` 分隔符明确区分快照参数与文件参数，消除歧义。

选项：
  --stat    只显示文件级摘要（不显示行级 diff）

示例：
  drift diff                                       # 工作区 vs HEAD
  drift diff id:12ab                              # 工作区 vs 12ab
  drift diff id:9f1e id:12ab                     # 两快照之间
  drift diff id:9f1e id:12ab -- chapter3.md      # 单文件行级 diff
  drift diff --stat id:9f1e id:12ab              # 仅文件级摘要
```

Output — 文件级：

```
>>> Diff id:9f1e -> id:12ab

  ~  chapter4.md
  +  assets/sketch.png

  2 files: +1 ~1
```

Output — 含删除的场景：

```
>>> Diff id:12ab -> id:9f1e

  -  assets/sketch.png
  ~  chapter4.md

  2 files: ~1 -1
```

> `+` 在目标快照新增，`-` 在目标快照删除，`~` 两边都有但内容不同。`->` 左边为基准，右边为对比目标。

Output — 单文件文本差异：

```
>>> Diff id:9f1e -> id:12ab chapter3.md
--- id:9f1e/chapter3.md  (旧版)
+++ id:12ab/chapter3.md  (新版)

@@ -12,5 +12,5 @@
 The old man sat by the window,
-staring at the rain.
+gazing at the falling rain.
 His tea had long gone cold.
-Outside, a car passed.
+Outside, a black car rumbled past.
 The clock struck noon.
```

**如何读取这段输出：**

| 符号 | 含义 |
|------|------|
| `---` | 旧版文件（`id:9f1e/chapter3.md`） |
| `+++` | 新版文件（`id:12ab/chapter3.md`） |
| `@@ -12,5 +12,5 @@` | 旧版第 12 行起 5 行 -> 新版第 12 行起 5 行 |
| 无前缀 | 上下文行，两边一样，没改过 |
| `-` | 旧版有，新版没有 — **被删掉的内容** |
| `+` | 新版有，旧版没有 — **新写的内容** |

> 此格式与 `git diff` 完全兼容。

Output — 二进制文件差异：

```
>>> Diff id:9f1e -> id:12ab cover.psd
  Size:       22.1 MB -> 23.4 MB (+1.3 MB)

  (binary file — metadata only)
```

Output — 图片文件差异（额外显示尺寸变化）：

```
>>> Diff id:9f1e -> id:12ab cover.png
  Size:       22.1 MB -> 23.4 MB (+1.3 MB)
  Dimensions: 4000x3000 -> 4200x3150

  (binary file — metadata only)
```

Output — `--stat`：

```
>>> Diff id:9f1e -> id:12ab (stat)

  chapter4.md       | 12 ++++++----
  assets/sketch.png | Bin 0 -> 2.1 MB

  2 files changed, 8 insertions(+), 4 deletions(-)
```

> 无 `-- <file>` 时所有文件一视同仁（只比较哈希）。指定 `-- <file>` 时，文本文件输出 unified diff；二进制文件显示元信息变化（图片额外显示尺寸）。

---

### `drift restore`

```
drift restore <snapshot-id> [<file>]

恢复项目（或单个文件）到指定快照的状态。

⚠ 恢复前会自动备份当前状态，避免误操作丢失。

选项：
  --no-backup     跳过自动备份（仅单文件恢复时允许；整快照恢复强制备份）

示例：
  drift restore id:12ab
  drift restore id:12ab chapter3.md
  drift restore tag:submission
  drift restore id:12ab chapter3.md --no-backup   # 单文件可跳过备份
```

Output — 整快照恢复：

```
>>> Restored to id:12ab [ok]

  +  chapter4.md
  +  sketch.png
  ~  chapter3.md

  3 files: +2 ~1
  backup: [a4f1]
```

> `backup: [a4f1]` 是恢复前自动保存的快照，保存了**被覆盖前的状态**。如果恢复错了，用 `drift restore id:a4f1` 即可撤销回去。

Output — 单文件：

```
>>> Restored id:12ab:chapter3.md [ok]

  ~  chapter3.md

  1 file: ~1
  backup: [b2e3]
```

Error — 整快照恢复尝试用 `--no-backup`：

```
>>> Restore [failed]
Error: --no-backup is only allowed for single-file restore.
  hint: full restore always creates a backup for safety.
```

> **未保存变更的处理**：当工作区有未提交修改时，整快照恢复**不会拒绝执行**，而是**先强制创建备份快照**再执行恢复。这样既不阻断用户工作流，又保证了"恢复永远可撤销"的核心承诺——若恢复错了，用 `drift restore id:<backup>` 即可撤销回去。

- 整快照恢复**强制备份**，`--no-backup` 仅对单文件恢复有效（影响范围小，可接受跳过）
- 这保证了"恢复永远可撤销"的核心承诺不被破坏

---

### `drift export`

```
drift export <version> [-o <path>]
```

将指定快照的所有文件导出为 zip 归档，便于把某个版本分享给未安装 drift 的人。

`<version>` 接受与其他命令一致的版本引用语法：`head`、`id:<前缀>`、`tag:<名称>`、`branch:<名称>`、`<裸名>`。

选项：

| 选项 | 默认 | 说明 |
|------|------|------|
| `-o, --output` | `drift-export-<short-id>.zip` | 输出 zip 文件路径 |

示例：

```
drift export head
drift export id:12ab
drift export tag:submission -o release.zip
drift export head -o /tmp/snapshot-12ab.zip
```

Output：

```
>>> Exported [ok]

  /home/me/release.zip

  3 files, 67.6 KB
```

Error — 快照不存在：

```
>>> Export [failed]
Error: snapshot 'id:9f1e' not found.
  hint: use 'drift log' to list available snapshots.
```

- 文件按 chunk 流式写入 zip，峰值内存取决于最大分块而非最大文件
- 输出路径的父目录不存在时自动创建；目标文件已存在时覆盖
- 目录条目保留，文件权限（Unix mode）一并写入 zip

---

### `drift import`

```
drift import <branch> <file>
```

从其他分支的最新快照中导入单个文件到当前工作区。这是一种非合并的文件级 cherry-pick：不影响其他工作区文件、不移动 HEAD、不创建快照。适用于把实验分支中的某个文件拿到当前分支而无需切换分支。

导入后请执行 `drift save` 将变更记录为新快照。

示例：

```
drift import dev chapter3.md
drift import feature assets/cover.png
```

Output：

```
>>> Imported [ok]

  chapter3.md  (from branch dev, 45.2 KB)

  1 file imported. Use 'drift save' to record this change.
```

Error — 分支不存在：

```
>>> Import [failed]
Error: import failed.
  hint: use 'drift branch list' to see available branches.
```

Error — 文件不在分支中：

```
>>> Import [failed]
Error: import failed.
  hint: use 'drift show branch:dev' to list files in this branch.
```

- 写入工作区后同步更新本地索引，使下次 `save` 将其视为未变更（除非用户再次修改）
- 工作区锁在导入期间持有，避免与 `save`/`restore` 并发冲突

---

### `drift branch`

```
drift branch list                              列出所有分支
drift branch create <name>                     创建新分支（不切换）
drift branch delete <name>                     删除分支
drift branch rename [<old-name>] <new-name>    重命名分支

重命名单参数时重命名当前分支，双参数时重命名指定分支。
重命名当前分支会同步更新 HEAD 指向新分支名。

示例：
  drift branch list
  drift branch create new-color-scheme
  drift branch create feature/foo              # 层级分支名（Git 语义）
  drift branch delete old-experiment
  drift branch rename dev                       # 重命名当前分支为 dev
  drift branch rename feature dev               # 重命名 feature 为 dev
```

Output — 创建：

```
>>> Branch created [ok]
'new-color-scheme' at snapshot 12ab.
```

Output — 列表：

```
>>> Branches (3)
* main
  new-color-scheme
  third-person-pov
```

Output — 删除：

```
>>> Branch deleted [ok]
'old-experiment' has been removed.
```

Output — 重命名：

```
>>> Branch renamed [ok]
'feature' has been renamed to 'dev'.
```

Error — 分支已存在：

```
>>> Branch [failed]
Error: 'new-color-scheme' already exists.
  hint: use 'drift switch new-color-scheme' to switch to it.
```

Error — 删除当前分支：

```
>>> Branch [failed]
Error: cannot delete the current branch 'main'.
  hint: switch to another branch first with 'drift switch'.
```

Error — 删除不存在的分支：

```
>>> Branch [failed]
Error: branch 'old-experiment' not found.
  hint: use 'drift branch list' to list existing branches.
```

Error — 删除 main 分支：

```
>>> Branch [failed]
Error: cannot delete 'main'.
  hint: 'main' is the default branch and cannot be removed.
```

Error — 重命名 main 分支：

```
>>> Branch [failed]
Error: cannot rename 'main'.
  hint: 'main' is the default branch and cannot be renamed.
```

- 分支名支持层级（如 `feature/foo`、`release/v1`），与 Git 语义一致
- 分支名经 NFC 规范化后存储

---

### `drift switch`

```
drift switch <name>            切换到已有分支
drift switch -c <name>         创建并切换到新分支
drift switch main              切换到主线

选项：
  -c, --create    创建新分支并切换
  --no-autosave   跳过切换前的自动保存（要求工作区干净）

示例：
  drift switch main
  drift switch new-color-scheme
  drift switch -c experimental
  drift switch main --no-autosave
```

Output — 自动保存当前工作区后切换：

```
>>> Switched to 'experimental' [ok]

  0 files differ from main.
  autosave: [b72d]
```

Output — 工作区干净 + `--no-autosave`：

```
>>> Switched to 'main' [ok]

  3 files differ from experimental.
```

Output — 切换回 main（有差异）：

```
>>> Switched to 'main' [ok]

  3 files differ from experimental.
  autosave: [c91e]
```

Error — 分支不存在：

```
>>> Switch [failed]
Error: branch 'typo-branch' not found.
  hint: use 'drift branch list' to list existing branches.
```

Error — `--no-autosave` 但工作区有变更：

```
>>> Switch [failed]
Error: --no-autosave requires a clean working tree.
  hint: use 'drift save' first, or drop --no-autosave to auto-save.
```

- 切换前自动保存当前工作区（创建 [auto] 快照），保证未提交变更不丢失
- `--no-autosave` 用于用户已手动 save 后切换、不想产生额外 [auto] 快照的场景，要求工作区干净
- `autosave:` 行在未产生自动保存时不显示

---

### `drift tag`

```
drift tag list                                 列出所有 tag
drift tag add <name> [<version>]               给已有快照打 tag（默认 head）
drift tag delete <name>                        删除 tag
drift tag rename <old-name> <new-name>         重命名 tag

示例：
  drift tag list
  drift tag add submission id:9f1e
  drift tag add 交稿v1 id:12ab                 # 中文 tag 名
  drift tag add latest                          # 省略 version，默认打在 head
  drift tag delete submission
  drift tag rename v1 final-v1
```

Output — 列表：

```
>>> Tags (3)
  submission   -> 9f1e  Submit to client
  v1           -> 12ab  Chapter 3 draft complete
  交稿v1        -> 12ab  Chapter 3 draft complete
```

Output — 添加：

```
>>> Tag added [ok]
'submission' -> 9f1e
```

Output — 删除：

```
>>> Tag deleted [ok]
'submission' has been removed.
```

Output — 重命名：

```
>>> Tag renamed [ok]
'v1' has been renamed to 'final-v1'.
```

Error — tag 已存在：

```
>>> Tag [failed]
Error: tag 'submission' already exists.
  hint: use 'drift tag delete submission' first, or pick another name.
```

Error — tag 不存在：

```
>>> Tag [failed]
Error: tag 'submission' not found.
  hint: use 'drift tag list' to see existing tags.
```

Error — 快照不存在：

```
>>> Tag [failed]
Error: snapshot 'id:9f1e' not found.
  hint: use 'drift log' to list available snapshots.
```

- tag 名经 NFC 规范化后存储
- `save --tag` 仍可用（等价于 `save` 后 `tag add`），但 `tag` 命令族提供完整管理能力

---

### `drift ignore`

```
drift ignore list                              列出当前忽略规则
drift ignore add <pattern>...                  添加忽略规则
drift ignore remove <pattern>                  移除某条规则

示例：
  drift ignore list
  drift ignore add "*.tmp" "*.psd"
  drift ignore remove "*.tmp"
```

Output — 添加：

```
>>> Ignore updated [ok]
+ *.tmp
+ *.psd

  2 rules added.
```

Output — 列表：

```
>>> Ignore rules (3)
*.tmp
*.psd
backup/
```

Output — 移除：

```
>>> Ignore updated [ok]
- *.tmp

  1 rule removed.
```

Error — 规则不存在：

```
>>> Ignore [failed]
Error: pattern '*.tmp' not found.
  hint: use 'drift ignore list' to see current rules.
```

`.driftignore` 不会在 `drift init` 时自动创建，首次 `drift ignore add` 时生成。支持的 glob 模式：`*`（单层通配）、`**`（递归通配）、`?`（单字符）、`[...]`（字符类）、`/` 前缀（锚定根目录）、`#`（注释）。不含 `/` 的模式按 basename 匹配（任意层级生效），含 `/` 的模式按相对路径匹配。完整语法见 `docs/architecture.md` §7.3。

---

### `drift watch`

```
drift watch on [--interval <seconds>] [--keep <n>]   启动后台监听
drift watch off                                      停止后台监听
drift watch status                                   查看监听状态
drift watch pause                                    暂停监听（保留配置）
drift watch resume                                   恢复监听

后台守护进程，检测到文件变更后自动保存。仅在文件变化时才创建快照，无变更则跳过该轮。启动后不阻塞终端，可正常执行其他命令。

选项（仅 on 模式）：
  --interval   检测间隔（默认 300 秒 = 5 分钟）。注意：这是检测频率，不是保存频率——无变更不保存
  --keep       最多保留 N 个自动保存（默认 50）。超出后自动清理最旧的

示例：
  drift watch on
  drift watch on --interval 600
  drift watch on --keep 30
  drift watch off
  drift watch status
  drift watch pause
  drift watch resume
```

Output — 启动：

```
>>> Watching [active]
Daemon started (PID 4821). Auto-save every 300s.
Keep last 50 auto-saves (older ones auto-pruned).
Use 'drift watch off' to stop, 'drift watch status' to check.
```

Output — 查看状态：

```
>>> Watching [active]
Running since 14:22 (47 min ago).
Auto-saves: 9 (50 max)
Last save: 16:30  +2 ~1
```

Output — 暂停：

```
>>> Watch [paused]
Daemon paused. Configuration retained.
Use 'drift watch resume' to continue.
```

Output — 恢复：

```
>>> Watching [active]
Daemon resumed. Auto-save every 300s.
```

> 如果两次检测之间文件没变化，不会创建快照。`Auto-saves` 只统计实际保存的次数。

Output — 手动停止：

```
>>> Stopped [ok]
Daemon stopped. 9 auto-saves created.
18 older auto-saves pruned during this session.
```

Output — 状态（未运行）：

```
>>> Watch [inactive]
No watch daemon running.
Start with 'drift watch on'.
```

Error — 已在运行：

```
>>> Watch [failed]
Error: a watch daemon is already running (PID 4821).
  hint: use 'drift watch off' to stop it first.
```

Error — 暂停时尝试暂停：

```
>>> Watch [failed]
Error: daemon is not running (or already paused).
  hint: use 'drift watch on' to start watching.
```

- 守护进程在后台运行，不影响其他命令
- 默认只保留最近 50 个自动保存，超出的自动删除（`--keep` 可调整）
- 手动保存 (`drift save`) 不受影响，只清理 `[auto]` 快照
- 自动快照是"安全网"，建议配合手动 `drift save -m "关键节点"` 使用
- 关闭终端时守护进程自动退出
- `pause`/`resume` 保留 `--interval`/`--keep` 配置，无需重新指定

> `watch` 系列命令不支持 `--json`：守护进程为实时日志流，不适合一次性 JSON 信封结构。程序化访问可通过 PID 文件或 `drift watch status` 的文本输出解析。

---

### `drift check`

```
drift check [--verbose] [--filter <pattern>]

校验 .drift/ 目录中所有块的数据完整性，验证 BLAKE3 哈希。

选项：
  --verbose       显示每个块的校验结果（默认只显示汇总）
  --filter <p>    只校验匹配 pattern 的文件

示例：
  drift check
  drift check --verbose
  drift check --filter "chapter*.md"
```

Output — 全部正常：

```
>>> Check [ok]
142 blocks passed.
```

Output — 全部正常但有不可达快照：

```
>>> Check [ok]
142 blocks passed.
  hint: 3 unreachable snapshots detected. use 'drift gc --dry-run' to review.
```

Output — 有损坏：

```
>>> Check [warning]
  blocks:  142 total, 140 passed
  corrupt: 2
  missing: 0

  hint: corrupt chunks cannot be auto-repaired. Restore affected files from a known-good snapshot using 'drift restore <snapshot-id>'.
```

Output — `--verbose`：

```
>>> Check [warning]
  12ab:chapter3.md  chunk 0  OK
  12ab:chapter3.md  chunk 1  OK
  12ab:chapter4.md  chunk 0  CORRUPT (hash mismatch)
  ...

  blocks:  142 total, 140 passed
  corrupt: 2
  missing: 0
```

Error：

```
>>> Check [failed]
Error: .drift/ directory not found.
  hint: run 'drift init' first.
```

---

### `drift gc`

```
drift gc [--dry-run] [--keep-auto <n>]

回收不再被任何分支或标签引用的快照与块，释放存储空间。
删除分支后留下的孤立快照、以及这些快照独占的块，都会被清理。

回收算法：
  1. 从所有 refs（heads/*、tags/*）的 target 出发，沿快照 PrevID 链
     遍历，标记所有可达快照。
  2. 删除未被标记的快照（即"不可达快照"）。
  3. 扫描所有可达快照引用的块哈希，删除未被引用的块。
  4. 先删快照、后删块，保证中途任何时刻都不会出现快照引用已删块的情况。

选项：
  --dry-run     只统计将要回收的数量，不实际删除
  --keep-auto   保留最近 N 个不可达的 [auto] 快照作为安全网（默认 0）

示例：
  drift gc --dry-run                   # 预览将回收多少快照与块
  drift gc                             # 执行回收
  drift gc --keep-auto 5               # 保留最近 5 个 auto 快照
```

Output — 正常回收：

```
>>> GC [ok]
  snapshots:  3 removed
  chunks:     27 removed
  freed:      12.4 MB
```

Output — 预览模式：

```
>>> GC [dry-run]
  snapshots:  3 would be removed
  chunks:     27 would be removed
  freed:      ~12.4 MB
```

Output — 保留 auto 快照：

```
>>> GC [ok]
  snapshots:  3 removed (2 auto-saves kept by --keep-auto=5)
  chunks:     27 removed
  freed:      12.4 MB
```

Output — 无可回收：

```
>>> GC [ok]
  nothing to reclaim.
```

Error：

```
>>> GC [failed]
Error: .drift/ directory not found.
  hint: run 'drift init' first.
```

- `--keep-auto` 保留最近的 N 个 [auto] 快照（即使不可达），作为误操作的安全网
- 手动 save 的快照不享受此保护（它们要么被分支/tag 引用，要么被回收）

---

### `drift config`

```
drift config list                              列出所有配置
drift config get <key>                         查看某项配置
drift config set <key> <value>                 修改配置

配置项：
  user.name              作者名（用于 snapshot.author）
  user.email             作者邮箱（可选，用于身份标识）

> 算法调优参数（chunk 分块尺寸、compression 压缩级别等）**不暴露给用户**，
> 它们硬编码在 `core.DefaultConfig` 中，面向创作者场景做了优化（128K/256K/512K
> 分块、zstd level 3）。用户只需配置身份信息；远程仓库参数（remote.*）将在
> 协作功能落地后加入。

示例：
  drift config list
  drift config get user.name
  drift config set user.name "张三"
  drift config set user.email "zhangsan@example.com"
```

Output — 列表：

```
>>> Config
  user.name  = 张三
  user.email = zhangsan@example.com
```

Output — 获取：

```
>>> Config: user.name
张三
```

Output — 设置：

```
>>> Config updated [ok]
  user.name = "张三"
```

Error — 未知配置项（含已移除的算法参数）：

```
>>> Config [failed]
Error: unknown config key 'chunk.min_size'.
  hint: use 'drift config list' to see available keys.
```

---

## 远程同步

### 设计要点

- **本地优先**：`.drift/` 始终是主存储，所有命令默认走本地、零延迟；远程仅存对象副本。
- **对象级内容寻址同步**：chunks 和 snapshots 文件名即 hash，同名对象内容必然相同，同步过程无需冲突解决。
- **refs 策略**：同名 ref 指向相同 hash 时幂等无操作；指向不同 hash 时——push 拒绝覆盖（提示先 pull），pull 保留本地、远程版本另存为 `<name>.remote`。
- **HEAD 与 config 不同步**：HEAD 是本机工作区状态，config 是本机行为配置，二者均不参与远程同步。
- **两级配置分离**：`remotes.json`（仓库级，无密码，可分享）+ `credentials.json`（用户级 `~/.config/drift/` 或 `%APPDATA%\drift\`，0600 权限，按 host+user 匹配）。
- **协议注册**：`ProtocolFactory` 通过 `init()` 注册，`NewRemoteFS` 按 `cfg.Type` 查找。当前支持 `webdav`（主力，覆盖网盘/NAS）和 `smb`（补充，Windows 共享/NAS）。

> 完整设计见 [docs/remote-design.md](remote-design.md)。

### `drift remote`

```
drift remote add <name> [--type webdav|smb] [--url <u>] [--user <u>] [--password <p>]
                         [--password-stdin] [--no-save-password] [--option key=value]...
drift remote remove <name>
drift remote list
drift remote rename <old-name> <new-name>
drift remote set-url <name> <new-url>
drift remote show <name>
drift remote test <name>
```

管理远程存储后端。`add` 在缺少 `--url` 或 `--user` 时进入交互模式（TTY 提示输入，密码隐式输入，SMB 额外询问 domain）。密码默认保存到用户级 `credentials.json`，`--no-save-password` 跳过保存（每次 push/pull 时提示输入）。`--password-stdin` 从标准输入读取密码，优先级高于 `--password`，适用于自动化脚本与管道。

选项（仅 `add`）：

| 选项 | 默认 | 说明 |
|------|------|------|
| `--type` | `webdav` | 协议类型（`webdav` / `smb`） |
| `--url` | （必填） | 远程 URL |
| `--user` | （必填） | 用户名 |
| `--password` | `""` | 密码；为空时交互提示 |
| `--password-stdin` | `false` | 从 stdin 读取密码（用于自动化/脚本） |
| `--no-save-password` | `false` | 不保存密码到 credentials.json |
| `--option` | — | 协议特定字段，`key=value`，可重复（如 SMB 的 `domain=WORKGROUP`） |

示例：

```
drift remote add origin --type webdav --url https://dav.example.com/dav --user me
drift remote add nas --type smb --url smb://nas.local/share --user me --option domain=WORKGROUP
drift remote list
drift remote rename old-backup archive
drift remote set-url origin https://new.dav.example.com/dav
drift remote show origin
drift remote test origin
drift remote remove old-backup
```

Output — `list`：

```
origin  webdav  https://dav.example.com/dav
nas     smb     smb://nas.local/share
```

Output — `list`（空）：

```
(no remotes configured)
```

Output — `add`：

```
Remote "origin" added (credentials saved to user-level config).
```

Output — `add`（未保存密码）：

```
Remote "origin" added (password NOT saved, will prompt on next push/pull).
```

Output — `remove`：

```
Remote "old-backup" removed. Credentials preserved in user-level config.
```

Output — `set-url`（host 变化时额外告警）：

```
Remote "origin" URL updated to https://new.dav.example.com/dav
warning: host changed (dav.example.com → new.dav.example.com); password may need reconfiguring.
```

Output — `test`（成功）：

```
>>> Remote "origin" reachable [ok]
```

Output — `rename`：

```
Remote "old-backup" renamed to "archive"
```

Output — `show`：

```
  name:     origin
  type:     webdav
  url:      https://dav.example.com/dav
  user:     me
  option:   domain=WORKGROUP
```

Error — 远程不存在：

```
remote "origin" not found
```

Error — 凭据缺失：

```
no credential for me@dav.example.com: run 'drift remote add' to configure
```

Error — 远程已存在：

```
remote "origin" already exists; use 'drift remote set-url' to update
```

Error — `rename` 时新名已存在：

```
remote "archive" already exists
  hint: use a different name or remove the existing remote first.
```

- `remove` 不删除凭据（凭据可能被其他仓库复用）
- `set-url` 检测 host 变化并告警，提示用户可能需要重新配置密码
- `test` 通过 `List(".")` 验证连通性，覆盖 URL/凭据/网络三个失败维度
- `rename` 仅改 remote 名，不移动凭据（凭据按 host+user 匹配，与 remote 名无关）
- `show` 输出单个 remote 的完整配置（含协议特定 options），但隐藏 `_password` 等内部字段

---

### `drift push`

```
drift push <remote> [--branch <name>] [--all] [--dry-run]
```

上传本地对象（snapshots / manifests / chunks / refs）到远程。默认只推送当前分支；`--all` 推送所有分支。已存在于远程的对象跳过；refs 分叉（同名不同目标）时报错，提示先 pull。HEAD 和 config 不上传。

选项：

| 选项 | 默认 | 说明 |
|------|------|------|
| `--branch` / `-b` | `""`（当前分支） | 只推送指定分支链上的快照、chunk 及该分支 ref |
| `--all` | `false` | 推送所有分支（不只是当前分支） |
| `--dry-run` | `false` | 预览将推送的内容（不实际上传） |

示例：

```
drift push origin
drift push origin --branch main
drift push origin --all
drift push origin --dry-run
```

Output — 正常：

```
>>> Pushing to 'origin' [ok]
  snapshots:  3 uploaded, 1 already present
  manifests:  3 uploaded
  chunks:     27 uploaded, 5 already present
  refs:       2 updated
```

Output — `--dry-run`：

```
>>> Push (dry run) [ok]
  snapshots:  3 would upload, 1 already present
  chunks:     27 would upload, 5 already present
  refs:       2 would update
```

Error — 分叉（需先 pull）：

```
>>> Push [failed]
Error: ...
  hint: check remote configuration and network connectivity
```

- **refs 快进判定**：`isAncestor()` 沿 PrevID 链判断目标差异是快进（祖先关系）还是真正分叉。快进允许覆盖远程 ref；零 hash 目标（新仓库）始终快进。
- **幂等计数**：`pushRef` 返回 `(bool, error)`，只有真正写入时才计入 `RefsUpdated`，幂等推送显示 `refs: 0 updated`。

---

### `drift pull`

```
drift pull <remote> [--branch <name>] [--all] [--dry-run] [--restore]
```

下载远程对象到本地。默认只拉取当前分支；`--all` 拉取所有分支。已存在本地对象跳过；分叉 refs 保留本地、远程版本另存为 `<name>.remote`；若当前分支 tip 前进，本地索引重建。HEAD 和 config 不下载。默认不修改工作区文件——如需更新文件，pull 后执行 `drift restore head`，或使用 `--restore` 让 pull 在分支 tip 前进后自动恢复工作区到最新快照。

选项：

| 选项 | 默认 | 说明 |
|------|------|------|
| `--branch` / `-b` | `""`（当前分支） | 只拉取指定分支链 |
| `--all` | `false` | 拉取所有分支（不只是当前分支） |
| `--dry-run` | `false` | 预览将拉取的内容（不实际下载） |
| `--restore` | `false` | pull 后自动恢复工作区文件（仅当分支 tip 前进时生效） |

示例：

```
drift pull origin
drift pull origin --branch main
drift pull origin --all
drift pull origin --restore
drift pull origin --dry-run
```

Output — 正常（无 tip 前进）：

```
>>> Pulling from 'origin' [ok]
  snapshots:  3 downloaded, 1 already present
  chunks:     27 downloaded, 5 already present
  refs:       2 updated, 0 diverged (saved as .remote)
```

Output — 当前分支 tip 前进：

```
>>> Pulling from 'origin' [ok]
  snapshots:  2 downloaded, 0 already present
  chunks:     18 downloaded, 0 already present
  refs:       1 updated, 0 diverged (saved as .remote)
  index:      rebuilt (branch 'main' tip advanced)
  hint: branch 'main' tip advanced. Working directory is out of sync.
        run 'drift restore head' to update your files.
```

Output — `--restore`（tip 前进后自动恢复工作区）：

```
>>> Pulling from 'origin' [ok]
  snapshots:  2 downloaded, 0 already present
  chunks:     18 downloaded, 0 already present
  refs:       1 updated, 0 diverged (saved as .remote)
  index:      rebuilt (branch 'main' tip advanced)
>>> Working directory restored [ok]
```

Output — `--dry-run`：

```
>>> Pull (dry run) [ok]
  snapshots:  3 would download, 1 already present
  chunks:     27 would download, 5 already present
  refs:       2 would update, 0 diverged
```

- 分叉 refs 显示 `N diverged (saved as .remote)`，用户可检查 `.remote` 版本后决定如何处理（如新建分支接住远程历史）
- `BranchTipChanged` 存储完整 ref 名（含 `heads/` 前缀），输出时剥离前缀以提升可读性
- `--restore` 仅在当前分支 tip 前进（索引重建）时触发恢复；恢复失败时输出 warning 并提示手动 `drift restore head`

---

### `drift clone`

```
drift clone <url> [<directory>] [--type webdav|smb] [--user <u>] [--password <p>] [--password-stdin]
```

从远程仓库克隆到本地新目录，等价于：`drift init <dir> && drift remote add origin <url> && drift pull origin --all`。

未指定 `<directory>` 时，取 URL 路径的最后一段作为目录名。克隆完成后当前分支为远程的 HEAD 分支。

选项：

| 选项 | 默认 | 说明 |
|------|------|------|
| `--type` | `webdav` | 协议类型（`webdav` / `smb`） |
| `--user` | `""` | 远程用户名 |
| `--password` | `""` | 远程密码；建议改用 `--password-stdin` |
| `--password-stdin` | `false` | 从 stdin 读取密码（用于自动化/脚本） |

示例：

```
drift clone https://dav.example.com/dav/my-novel
drift clone https://dav.example.com/dav/my-novel ~/Documents/my-novel
drift clone smb://nas.local/share --type smb --user me
echo "$PASS" | drift clone https://dav.example.com/dav/repo --user me --password-stdin
```

Output：

```
>>> Cloned into 'my-novel' [ok]
  snapshots:  3
  branches:   2
  tags:       1
  branch:     main
```

Output — 未保存密码（warning 输出到 stderr）：

```
>>> Cloned into 'my-novel' [ok]
  snapshots:  3
  branches:   2
  tags:       1
  branch:     main
  warning: password was not saved; provide credentials on next push/pull.
```

Error — 克隆失败：

```
>>> Clone [failed]
Error: clone failed.
  hint: check the remote URL and network connectivity
```

- 克隆自动注册名为 `origin` 的 remote；密码非空时保存到用户级 `credentials.json`
- `--password-stdin` 优先级高于 `--password`；使用 `--password` 时会输出安全警告（明文密码在进程列表可见）

---

### `drift ls-remote`

```
drift ls-remote <remote>
```

列出远程仓库的所有 refs（分支与标签），不下载任何对象。用于在 pull/clone 前查看远程有哪些分支与标签。

示例：

```
drift ls-remote origin
```

Output：

```
3a7f9c1b2d4e6f8a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a  heads/main
9f1e0a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f  heads/dev
a3c20b4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b  tags/submission
```

Output — 远程无 refs：

```
(no refs on remote)
```

Error — 远程不可达：

```
>>> Ls-remote [failed]
Error: could not list remote refs.
  hint: check remote configuration and network connectivity
```

- 输出格式为 `<完整 target hash>\t<ref name>`，便于脚本解析
- 与 `pull` 不同：只读取远程 refs，不传输 snapshots/chunks

---

## 版本与自升级

### 设计要点

- **版本来源**：构建时通过 `-ldflags` 注入 `Version`/`Commit`/`BuildDate`；未注入时回退到 `runtime/debug.ReadBuildInfo()`，`go install` 构建的二进制会自动得到 module version 与 VCS revision
- **无需项目**：`version` 与 `upgrade` 不依赖 `.drift/` 仓库，可在任意目录运行，便于排查"装了什么版本"
- **`--version` 内置标志**：cobra 的内置 `--version` 走 `rootCmd.Version`，输出单行精简版本号；`drift version` 子命令输出完整的 commit/构建时间/平台信息
- **自升级机制**：`upgrade` 查询 GitHub Releases API，按 `GOOS/GOARCH` 匹配预构建二进制，下载后原子替换运行中的可执行文件
- **发布产物命名约定**：`drift_<version>_<os>_<arch>.{zip|tar.gz}`（windows 用 zip，其余用 tar.gz），附 `drift_<version>_checksums.txt`（SHA-256）
- **原子替换**：两步 rename（当前二进制 → `.old`，新二进制 → 原路径），Windows 下运行中二进制不能覆盖但可重命名；失败时回滚 `.old`
- **校验**：当 release 附带 `checksums.txt` 时，下载后校验 SHA-256；校验文件缺失或格式错误时跳过校验（HTTPS 已保护传输完整性），不阻断升级
- **dev 构建**：当前版本为 `(devel)` 或 `v0.0.0-*` 伪版本时，视为比任何正式 release 旧，`upgrade` 总会提示升级

### `drift version`

查看当前二进制的版本与构建信息。无需 drift 仓库，任意目录可运行。

**用法**

```
drift version
```

**输出（人类可读）**

```
drift v0.1.0 (commit: a1b2c3d, built: 2026-07-08T12:00:00Z)
  go1.25  windows/amd64
```

**输出（`--json`）**

```json
{
  "command": "version",
  "status": "ok",
  "data": {
    "version": "v0.1.0",
    "commit": "a1b2c3d",
    "built": "2026-07-08T12:00:00Z",
    "go_version": "go1.25",
    "os": "windows",
    "arch": "amd64"
  },
  "hint": null
}
```

**`-q` 模式**：只输出纯版本号（如 `v0.1.0`），便于脚本管道使用。

**`drift --version`**：cobra 内置标志，输出 `drift version v0.1.0`（单行精简）。

### `drift upgrade`

检查 GitHub 上的最新 release，并在有新版本时下载对应平台的预构建二进制原子替换当前可执行文件。无需 drift 仓库。

**用法**

```
drift upgrade [--check] [--force]
```

**选项**

| 选项 | 说明 |
|------|------|
| `--check` | 只检查是否有新版本，不执行升级 |
| `--force` | 即使已是最新版也强制重新安装 |
| `--json` | JSON 输出（全局选项） |
| `-q` | 静默模式，成功无输出（全局选项） |

**输出（已是最新）**

```
>>> Upgrade [ok]
  already up to date (v0.2.0)
```

**输出（成功升级）**

```
>>> Upgrade [ok]
  v0.1.0 -> v0.2.0
  restart drift to use the new version.
```

**输出（`--check` 发现有更新）**

```
>>> Upgrade [ok]
  update available: v0.1.0 -> v0.2.0
```

**输出（dev 构建，总是提示升级）**

```
>>> Running a development build (v0.0.0-20260708...); latest release will be offered. [warning]
>>> Upgrade [ok]
  update available: (devel) -> v0.1.0
```

**JSON 输出**

```json
{
  "command": "upgrade",
  "status": "ok",
  "data": {
    "from": "v0.1.0",
    "to": "v0.2.0",
    "upgraded": true,
    "message": "v0.1.0 -> v0.2.0"
  },
  "hint": null
}
```

**错误路径**

| 场景 | 输出 |
|------|------|
| 网络不可达 | `>>> Upgrade [failed]` + hint: `check your network connection and GitHub availability.` |
| 无 release（404） | `>>> Upgrade [failed]` + hint: `no GitHub release has been published yet; see .../releases.` |
| 无匹配平台资产 | `>>> Upgrade [failed]` + hint: `no prebuilt binary for this platform; build from source with 'go install ...'.` |
| 校验和不匹配 | `>>> Upgrade [failed]` + hint: `see .../releases for manual download.` |
| 下载/解压/替换失败 | `>>> Upgrade [failed]` + hint: `see .../releases for manual download.` |

**升级流程**

1. `version.GetInfo()` 取当前版本
2. `GET https://api.github.com/repos/Alei-001/drift/releases/latest` → `tag_name` + `assets[]`
3. semver 比较（手写实现，不引入新依赖；支持 pre-release）
4. 已最新且非 `--force` → 输出 `already up to date`
5. `--check` → 输出 `update available` 后退出
6. 按 `runtime.GOOS`/`runtime.GOARCH` 匹配资产（windows→`.zip`，其余→`.tar.gz`）
7. 若存在 `checksums.txt` 资产 → 下载并校验 SHA-256
8. 下载归档到临时文件
9. 从 zip/tar.gz 解压出 `drift`/`drift.exe`（stdlib，无新依赖）
10. 两步 rename 替换当前可执行文件，成功后清理 `.old`

---

## Git concept mapping

| User sees | Git equivalent | Difference |
|---------|-----------|------|
| `save` | `add + commit` | Auto-includes all changes, no staging area; -m optional |
| `undo` | `reset HEAD~1` | Undo last save; snapshot becomes unreachable |
| `snap` / snapshot | `commit` | - |
| `log` | `log --oneline --graph` | With thumbnail preview |
| `show` | `show` / `ls-tree` | Lists files or shows content |
| `restore` | `reset` / `checkout` | Auto-backup before restore (forced for full restore) |
| `branch` | `branch` | Create and list; no merge; subcommands |
| `switch` | `checkout` / `switch` | Auto-save before switch; --no-autosave |
| `tag` | `tag` | Full tag management via subcommands |
| `ignore` | `.gitignore` + `git ignore` (proposed) | Subcommands for add/list/remove |
| `main` | `main` / `master` | - |
| `diff` | `diff` | Supports images, visual diff; `--` separator |
| `config` | `config` | - |
| `remote` | `remote` | Subcommands: add/remove/list/rename/set-url/show/test; two-level config (repo + user creds) |
| `push` | `push` | Object-level content-addressed; no merge; diverged refs error |
| `pull` | `pull` / `fetch` | Diverged refs saved as `.remote`; `--restore` updates working tree |
| - | `merge`, `rebase`, `stash`, `cherry-pick`, `bisect` | Intentionally omitted |

---

## 分阶段实现计划

### 第一阶段：本地核心

```
init    save    undo    log    show    status    restore    check
```

这 8 个命令跑通，就是一个完整可用的本地版本管理工具。

### 第二阶段：分支 + 自动化

```
branch    switch    tag    ignore    watch    gc
```

### 第三阶段：远程

```
remote    push    pull
```

已完成：`remote`（add/remove/list/rename/set-url/show/test）、`push`、`pull` 支持 WebDAV 与 SMB 协议，refs 快进判定与分叉保护，两级凭据分离；`clone`/`ls-remote` 远程查询，`export`/`import` 内容导入导出。后续优化（Stage 6）：并发上传、进度条、credential helper。

---

## 命令速查卡

```
# 全局选项
drift -C <path> <command>              run in specified directory
drift --json <command>                 JSON output
drift -q <command>                     quiet (errors only)

# 初始化与配置
drift init                             initialize a project
drift init ~/Documents/my-novel
drift config list                      list all config
drift config set user.name "张三"

# 保存与撤销
drift save -m "msg"                    save with message
drift save                             quick save (default message)
drift save -m "msg" --tag v1 --tag v2  save with multiple tags
drift undo                             undo last save

# 浏览历史
drift log                              show last 30 entries
drift log -l 20                        show last 20
drift log --detail id:12ab            show file change details
drift log --all                        include auto-saves
drift log --branch feature             filter by branch
drift show id:12ab                    list files in snapshot
drift show id:12ab chapter.md         view old snapshot-id of a file
drift show tag:v1 cover.psd --open    open image with system app
drift status                           show changes since last save
drift status -s                        short format, paths only

# 差异对比
drift diff                             working tree vs HEAD
drift diff id:12ab                    working tree vs snapshot
drift diff id:9f1e id:12ab           diff between two snapshots
drift diff id:9f1e id:12ab -- chapter3.md   single file diff
drift diff --stat id:9f1e id:12ab    stat only

# 恢复 / 导出 / 导入
drift restore id:12ab                 restore to a snapshot (auto backup)
drift restore id:12ab chapter.md      restore a single file
drift restore id:12ab chapter.md --no-backup   single file, no backup
drift export head                      export snapshot as zip
drift export head -o release.zip       export to specific path
drift import dev chapter.md           import a file from another branch

# 分支
drift branch list                      list all branches
drift branch create new-direction      create a branch
drift branch create feature/foo        hierarchical name
drift branch delete old-experiment     delete a branch
drift branch rename dev                rename current branch
drift branch rename feature dev        rename a specific branch
drift switch main                      switch to main
drift switch -c experiment             create and switch
drift switch main --no-autosave        switch without auto-save

# 标签
drift tag list                         list all tags
drift tag add submission id:9f1e      tag an existing snapshot
drift tag add 交稿v1 id:12ab           Chinese tag name
drift tag delete submission            delete a tag
drift tag rename v1 final-v1           rename a tag

# 忽略规则
drift ignore list                      list ignore rules
drift ignore add "*.psd" "*.tmp"       add ignore rules
drift ignore remove "*.psd"            remove ignore rule

# 自动监听
drift watch on                         start auto-watch daemon
drift watch on --interval 600          custom interval
drift watch on --keep 30               custom retention
drift watch status                     check daemon status
drift watch pause                      pause (keep config)
drift watch resume                     resume
drift watch off                        stop daemon

# 维护
drift check                            verify data integrity
drift check --verbose                  per-chunk results
drift gc --dry-run                     preview reclaimable data
drift gc                               reclaim unreachable data
drift gc --keep-auto 5                 keep recent 5 auto-saves

# 远程同步
drift clone <url>                     clone a remote repo into a new directory
drift clone <url> mydir               clone into a specific directory
drift ls-remote origin                list remote refs without downloading
drift remote add origin --type webdav --url <u> --user <u>   add a remote
drift remote add nas --type smb --url <u> --user <u> --option domain=WORKGROUP
drift remote list                      list all remotes
drift remote rename old new           rename a remote
drift remote show origin               show a remote's full config
drift remote set-url origin <new-url>  update a remote's URL
drift remote test origin               test connectivity
drift remote remove old-backup         remove a remote (creds preserved)
drift push origin                      upload current branch to remote
drift push origin --all                push all branches
drift push origin --dry-run            preview what would be pushed
drift pull origin                      download remote objects (current branch)
drift pull origin --all                pull all branches
drift pull origin --restore            auto-restore working dir after pull
drift pull origin --dry-run            preview what would be pulled

# 版本与自升级
drift version                          show version and build info
drift version --json                   machine-readable version
drift --version                        one-line version (cobra built-in)
drift upgrade --check                  check for newer release
drift upgrade                          upgrade to latest release
drift upgrade --force                  reinstall even when up to date
```

---

## 设计变更摘要（相对旧版）

| 变更 | 原因 |
|------|------|
| 新增 `undo` 命令 | 保证 save 可撤销，与"恢复永远可撤销"原则对称 |
| 新增 `tag` 命令族 | 原 `save --tag` 无法 list/delete/rename/补打 tag |
| 新增 `config` 命令 | 用户需要 CLI 修改压缩级别、作者名等配置 |
| 新增全局选项 `-C`/`--json`/`--no-pager`/`-q`/`-v` | 脚本友好、GUI 调用、可观测性 |
| `save -m` 改为可选 | 零摩擦原则，支持快速存档；用户事后可补充 |
| `save --tag` 支持多 tag | 一次保存多标签是常见需求 |
| `restore --no-backup` 限制为单文件 | 整快照恢复强制备份，遵守"恢复永远可撤销"原则 |
| `branch`/`ignore`/`watch` 统一子命令风格 | 扩展性、一致性；flag 风格在命令多了后会撞车 |
| `watch` 新增 `pause`/`resume` | 临时禁用监听无需重新配置 |
| `log -v` 改为 `--detail` | `-v` 习惯上是 boolean verbose，带参数易混淆 |
| `log` 默认 limit 10 -> 30 | 活跃项目一屏看不全 |
| `log` 新增 `--branch` | 按分支过滤历史 |
| `show` 无 file 参数时列出文件清单 | 与 `log --detail` 功能合并，更符合"show 查看快照"的语义 |
| `show` 单文件参数隐式 `head` | 便利性：`drift show README.md` 比 `drift show head README.md` 更自然；文件路径含 `.`/`/`，与裸分支名不冲突 |
| 版本引用统一为 `id:`/`tag:`/`branch:`/`head` 前缀 | 消除分支名与 hash 前缀冲突的歧义 |
| `diff` 用 `--` 分隔符区分快照参数与文件参数 | 三参数模式无法表达"工作区 vs 快照的单文件 diff" |
| `diff` 新增 `--stat` | 只看文件级摘要，不看行级 diff |
| `gc` 新增 `--keep-auto` | 保留最近 N 个 auto 快照作为误操作安全网 |
| `check` 新增 `--verbose`/`--filter` | 详细校验结果、按文件过滤 |
| `switch` 新增 `--no-autosave` | 用户已手动 save 后切换，不想产生 [auto] 快照 |
| 结构符号统一 ASCII（`->` 替代 `→`，`...` 替代 `…`） | 跨终端兼容性（老 CMD、SSH 非 UTF-8 locale） |
| 用户内容（message/tag/分支名）支持 UTF-8 全集 | 项目面向创作者，需支持中文等多语言内容 |
| tag/分支名 NFC 规范化 | 避免 Unicode 同形异码问题 |
| `--json` 升级为全局选项，统一信封结构 | 脚本友好，所有命令输出结构一致 |
| 速查卡去除无空格参数的多余引号 | 避免误导新手 |
| 移除全局 `--no-pager` | 当前未实现分页器，flag 为空操作；移除避免承诺未兑现 |
| 移除全局 `-v`/`--verbose` | 与 `check --verbose` 命令级 flag 语义冲突且无读取处；移除避免混淆 |
| `watch` 不支持 `--json` | 守护进程为实时日志流，不适合 JSON 信封；程序化访问可解析 `watch status` 文本输出 |
| `sync` 拆分为 `push` + `pull` | 方向独立、错误路径更可控；单一 `sync` 难以表达"只上传不下载"等场景 |
| 新增 `remote`/`push`/`pull` 命令族 | 第三阶段远程同步落地：WebDAV/SMB 协议、两级凭据分离、refs 快进判定与分叉保护 |
| `push`/`pull` 的 `--dry-run` 已实现 | 预览将推送/拉取的对象数量，不实际执行上传/下载 |
| `restore` 备份 ID 显示恢复前 HEAD | 工作区干净时无备份快照产生，fallback 显示恢复前的 HEAD ID（而非恢复目标），确保用户能据此撤销 |
| `watch on` 预校验项目 | 原实现先 spawn 子进程再在子进程中打开项目，初始化失败时静默退出留下 stale PID 文件；预校验将错误前置到父进程 |
| 文本检测增加控制字节比例阈值 | 原 NUL-only 启发式会将无 NUL 的二进制数据误判为 text；新增 10% 控制字节阈值作为二次防线 |
| `remote` 命令族错误统一 `[failed]` 格式 | 原 `remove`/`set-url`/`add` 用裸 `fmt.Errorf`，与 `test`/`push`/`pull` 的格式化错误块不一致 |
| 新增 `version`/`upgrade` 命令 | 用户需要查看已安装版本并自升级到最新 release；`upgrade` 走 GitHub Releases 二进制自更新，无需 Go 工具链 |
| `cmd/version.go` 中 `resolveSnapshot` 移到 `cmd/resolve.go` | 原文件同时包含快照版本引用解析（`resolveSnapshot`）与 CLI 自身版本命令，职责混淆；拆分后 `resolve.go` 专管引用解析，`version.go` 仍存在并实现 `drift version` 命令 |
