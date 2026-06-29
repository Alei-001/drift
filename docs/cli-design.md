# drift CLI 命令设计

## 设计原则

- **无 Git 术语** — 用创作者的日常语言：保存、恢复、分支，而非 commit/rebase/reset
- **零暂存区** — `save` 自动捕获所有变更，不要求用户手动选择文件
- **命令精简** — 每个命令只做一件明确的事，避免像 git 那样一个命令有几十个 flag
- **不安全操作不暴露** — 没有 `reset --hard`，恢复永远可撤销
- **GUI 为视觉主力** — 缩略图时间线在 GUI 中展现，CLI 专注快速、精确、可脚本化操作

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

### 命令分类

| 类型 | 命令 | 输出特点 |
|------|------|---------|
| 执行 | init, save, restore, branch, switch, ignore | 状态行 + 文件列表 + 总结 |
| 查询 | log, show, status, diff, check | 状态行 + 查询结果 + 总结 |
| 驻留 | watch | 状态行 + 实时日志流 + 结束总结 |

---

## 命令全景

```
drift
├── init         初始化项目，开始追踪
├── save         保存当前状态为快照
├── log          浏览历史快照
├── show         预览某个快照的内容
├── status       查看自上次保存后的变更
├── diff         比较两个快照的差异
├── restore      恢复文件到指定快照
├── branch       创建 / 列出 / 删除 / 重命名分支
├── switch       切换到主分支或其他分支
├── ignore       忽略文件或目录
├── watch        自动监听并保存（后台守护）
│   ├── on        启动监听
│   ├── off       停止监听
│   └── status    查看状态
├── check        校验数据完整性
├── gc           回收无引用的快照与块
├── remote       管理远程存储（后续版本）
├── sync         同步到远程（后续版本）
└── help         帮助信息
```

---

## 各命令详细设计

### `drift init`

```
drift init [path]

在当前目录（或指定 path）初始化 drift 项目。

示例：
  drift init
  drift init ~/Documents/my-novel
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
- 自动添加 `.driftignore` 默认模板（排除 .DS_Store / Thumbs.db / 系统临时文件）

---

### `drift save`

```
drift save -m <message> [--tag <name>]

保存当前所有变更，创建一个新快照。-m 为必填。

选项：
  -m, --message    快照消息（必填）
  --tag            为这个快照起一个固定别名，如 --tag "交稿v1"

示例：
  drift save -m "Chapter 3 draft complete"          # inline message
  drift save -m "Update cover" --tag "submission"   # message + tag
```

Output：

```
>>> Saved [12ab] [ok]
Chapter 3 draft complete

  +  chapter4.md      12.3 KB
  +  sketch.png        2.1 MB
  ~  chapter3.md      45.2 KB

  3 files: +2 ~1
```

Output — 带 tag：

```
>>> Saved [9f1e] [ok]
Submit to client  [submission]

  +  chapter4.md      12.3 KB
  ~  chapter3.md      45.2 KB

  2 files: +1 ~1
```

Error（缺少消息）：

```
>>> Save [failed]
Error: -m <message> is required.
  hint: use 'drift save -m "your message"' to describe this snapshot.
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

---

### `drift log`

```
drift log [-l <n>] [--json] [--all]
drift log -v <id>

浏览历史快照。默认只显示用户手动创建的快照，[auto] 快照隐藏。

选项：
  -l, --limit     显示最近 N 条记录（默认 10）
  -v, --verbose    查看某个快照的文件变更明细
  --json          以 JSON 格式输出，适合脚本处理
  --all           包括自动保存 (drift watch) 的快照

示例：
  drift log
  drift log -l 20
  drift log -v 12ab
  drift log --all
```

Output — 默认：

```
>>> History (3 snapshots)
12ab  2026-06-28 16:30  Chapter 3 draft complete                     +2 ~1
a3c2  2026-06-27 22:15  Update cover color scheme                     ~1
9f1e  2026-06-27 10:00  Submit to client                [submission]  +1 ~1
```

消息或标签过长时自动截断，末尾加 `…`：

```
>>> History (3 snapshots)
12ab  2026-06-28 16:30  Chapter 3 draft complete, revised by editor…  +2 ~1
b4e1  2026-06-27 22:15  Fix typo                        [typo-fix-…]  ~1
```

> 被截断的完整内容可通过 `drift log -v <id>` 查看。`-v` 模式不限宽度，完整展示消息和文件列表。

Output — `--all`：

```
>>> History (5 snapshots, including auto-saves)
12ab  2026-06-28 16:30  Chapter 3 draft complete                     +2 ~1
f3e2  2026-06-28 16:25  [auto] 2026-06-28 16:25                     ~1    · dimmed
a3c2  2026-06-27 22:15  Update cover color scheme                     ~1
f1a0  2026-06-27 22:10  [auto] 2026-06-27 22:10                     +1    · dimmed
9f1e  2026-06-27 10:00  Submit to client                [submission]  +1 ~1
```

Output — `-v <id>`：

```
>>> Snapshot 12ab
2026-06-28 16:30  Chapter 3 draft complete

  +  chapter4.md      12.3 KB
  +  sketch.png        2.1 MB
  ~  chapter3.md      45.2 KB  (42 lines)

  3 files: +2 ~1
```

Output — `--json`：

```
>>> History (3)
[
  {"id":"12ab","time":"2026-06-28T16:30:00","message":"Chapter 3 draft complete","tag":null,"changes":"+2 ~1 -0"},
  {"id":"a3c2","time":"2026-06-27T22:15:00","message":"Update cover color scheme","tag":null,"changes":"+0 ~1 -0"},
  {"id":"9f1e","time":"2026-06-27T10:00:00","message":"Submit to client","tag":"submission","changes":"+1 ~1 -0"}
]
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
drift show <version> <file> [--open]

查看指定快照中某个文件的内容。文本文件用分页器展示；非文本文件默认显示元信息。

选项：
  --open    用系统默认程序打开文件

示例：
  drift show 12ab chapter1.md
  drift show @tag:submission cover.psd
  drift show @tag:submission cover.psd --open
  drift show main README.md
```

Output — 文本文件：

```
>>> File 12ab:chapter1.md

# Chapter 1: The Beginning
The sun rose over the quiet village...

(↑↓ scroll  ·  q quit  ·  / search)
```

Output — 二进制文件：

```
>>> File @tag:submission:cover.psd
  Size:       23.4 MB
  Modified:   06-28 16:30

  hint: use --open to view with system program.
```

Output — 图片文件（额外显示尺寸）：

```
>>> File 12ab:cover.png
  Size:       2.1 MB
  Dimensions: 4200×3150
  Modified:   06-28 16:30

  hint: use --open to view with system program.
```

Output — `--open`：

```
>>> Opening [ok]
Launched system viewer for @tag:submission:cover.psd.
```

Error：

```
>>> Show [failed]
Error: 'cover.psd' not found in snapshot 12ab.
  hint: use 'drift log -v 12ab' to list files in this snapshot.
```

---

### `drift status`

```
drift status [-s]

查看自上次 save 以来的变更情况。列出所有新增、修改、删除的文件。

选项：
  -s, --short    仅显示文件路径

示例：
  drift status
  drift status -s
```

Output：

```
>>> Status (3 files changed since last save)

  +  chapter4.md
  +  assets/sketch.png
  ~  chapter3.md

  3 files: +2 ~1
```

Output — `--short`：

```
>>> Status (3 files)
chapter4.md
assets/sketch.png
chapter3.md
```

Output — 无变更：

```
>>> Status [ok]
Nothing changed since last save.
```

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
drift diff                    对比工作区 vs 上次快照
drift diff <id>               对比工作区 vs 指定快照
drift diff <id1> <id2>        对比两个快照
drift diff <id1> <id2> <file> 对比某文件在两个快照间的差异

显示两个快照之间的文件级变更摘要。指定文件时显示行级差异。

示例：
  drift diff
  drift diff 12ab
  drift diff 12ab 9f1e
  drift diff 12ab 9f1e chapter3.md
```

Output — 文件级：

```
>>> Diff 9f1e → 12ab

  ~  chapter4.md
  +  assets/sketch.png

  2 files: +1 ~1
```

Output — 含删除的场景：

```
>>> Diff 12ab → 9f1e

  -  assets/sketch.png
  ~  chapter4.md

  2 files: ~1 -1
```

> `+` 在目标快照新增，`-` 在目标快照删除，`~` 两边都有但内容不同。`→` 左边为基准，右边为对比目标。

Output — 单文件文本差异：

```
>>> Diff 9f1e → 12ab chapter3.md
--- 9f1e/chapter3.md  (旧版)
+++ 12ab/chapter3.md  (新版)

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
| `---` | 旧版文件（`9f1e/chapter3.md`） |
| `+++` | 新版文件（`12ab/chapter3.md`） |
| `@@ -12,5 +12,5 @@` | 旧版第 12 行起 5 行 → 新版第 12 行起 5 行 |
| 无前缀 | 上下文行，两边一样，没改过 |
| `-` | 旧版有，新版没有 — **被删掉的内容** |
| `+` | 新版有，旧版没有 — **新写的内容** |

在上面这个例子中，作者把 "staring at the rain" 润色成 "gazing at the falling rain"，把 "a car passed" 改成了 "a black car rumbled past"。

> 此格式与 `git diff` 完全兼容。

Output — 二进制文件差异：

```
>>> Diff 9f1e → 12ab cover.psd
  Size:       22.1 MB → 23.4 MB (+1.3 MB)

  (binary file — metadata only)
```

Output — 图片文件差异（额外显示尺寸变化）：

```
>>> Diff 9f1e → 12ab cover.png
  Size:       22.1 MB → 23.4 MB (+1.3 MB)
  Dimensions: 4000×3000 → 4200×3150

  (binary file — metadata only)
```

> 无 file 参数时所有文件一视同仁（只比较哈希）。指定 file 参数时，文本文件输出 unified diff；二进制文件显示元信息变化（图片额外显示尺寸）。

---

### `drift restore`

```
drift restore <version> [<file>]

恢复项目（或单个文件）到指定快照的状态。

⚠ 恢复前会自动备份当前状态，避免误操作丢失。

选项：
  --no-backup     跳过自动备份

示例：
  drift restore 12ab
  drift restore 12ab chapter3.md
  drift restore @tag:submission
```

Output：

```
>>> Restored to 12ab [ok]

  +  chapter4.md
  +  sketch.png
  ~  chapter3.md

  3 files: +2 ~1
  backup: [a4f1]
```

> `backup: [a4f1]` 是恢复前自动保存的快照，保存了**被覆盖前的状态**。如果恢复错了，用 `drift restore a4f1` 即可撤销回去。

Output — 单文件：

```
>>> Restored 12ab:chapter3.md [ok]

  ~  chapter3.md

  1 file: ~1
  backup: [b2e3]
```

Error：

```
>>> Restore [failed]
Error: uncommitted changes would be overwritten.
  hint: use 'drift save' first, or restore a single file.
```

---

### `drift branch`

```
drift branch [<name>]
drift branch -d <name>
drift branch -m <new-name>
drift branch -m <old-name> <new-name>

不带参数时列出所有分支。带 name 时创建新分支（不切换）。
使用 -d 删除指定分支。
使用 -m 重命名分支：单参数时重命名当前分支，双参数时重命名指定分支。
重命名当前分支会同步更新 HEAD 指向新分支名。

选项：
  -d    删除分支
  -m    重命名分支

示例：
  drift branch                      # list branches
  drift branch new-color-scheme     # create branch
  drift branch -d old-experiment    # delete branch
  drift branch -m dev               # rename current branch to 'dev'
  drift branch -m feature dev       # rename 'feature' to 'dev'
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

Error：

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
  hint: use 'drift branch' to list existing branches.
```

Error — 删除 main 分支：

```
>>> Branch [failed]
Error: cannot delete 'main'.
  hint: 'main' is the default branch and cannot be removed.
```

Error — 重命名到已存在的分支名：

```
>>> Branch [failed]
Error: branch 'dev' already exists.
  hint: use 'drift branch' to list existing branches.
```

Error — 重命名不存在的分支：

```
>>> Branch [failed]
Error: branch 'old-name' not found.
  hint: use 'drift branch' to list existing branches.
```

Error — 重命名当前分支时未指定新名字：

```
>>> Branch [failed]
Error: new branch name required with -m.
```

Error — 重命名 main 分支：

```
>>> Branch [failed]
Error: cannot rename 'main'.
  hint: 'main' is the default branch and cannot be renamed.
```

---

### `drift switch`

```
drift switch <name>            切换到已有分支
drift switch -c <name>         创建并切换到新分支
drift switch main              切换到主线

选项：
  -c    创建新分支并切换

示例：
  drift switch main
  drift switch new-color-scheme
  drift switch -c experimental
```

Output：

```
>>> Switched to 'experimental' [ok]

  0 files differ from main.
  autosave: [b72d]
```

Output — 切换回 main（有差异）：

```
>>> Switched to 'main' [ok]

  3 files differ from experimental.
  autosave: [c91e]
```

Error：

```
>>> Switch [failed]
Error: branch 'typo-branch' not found.
  hint: use 'drift branch' to list existing branches.
```

---

### `drift ignore`

```
drift ignore [--list | --remove <pattern> | <pattern...>]

管理忽略规则。

选项：
  --list            列出当前忽略规则
  --remove <p>      移除某条规则

示例：
  drift ignore "*.tmp" "*.psd"
  drift ignore --list
  drift ignore --remove "*.tmp"
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

Error：

```
>>> Ignore [failed]
Error: pattern '*.tmp' not found.
  hint: use 'drift ignore --list' to see current rules.
```

---

### `drift watch`

```
drift watch on  [--interval <seconds>]    启动后台监听
drift watch off                           停止后台监听
drift watch status                        查看监听状态

后台守护进程，检测到文件变更后自动保存。仅在文件变化时才创建快照，无变更则跳过该轮。启动后不阻塞终端，可正常执行其他命令。

选项（仅 on 模式）：
  --interval   检测间隔（默认 300 秒 = 5 分钟）。注意：这是检测频率，不是保存频率——无变更不保存
  --keep       最多保留 N 个自动保存（默认 50）。超出后自动清理最旧的，防止存储膨胀

示例：
  drift watch on
  drift watch on --interval 600
  drift watch on --keep 30
  drift watch off
  drift watch status
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

> 如果两次检测之间文件没变化，不会创建快照。`Auto-saves` 只统计实际保存的次数。

Output — 手动停止：

```
>>> Stopped [ok]
Daemon stopped. 9 auto-saves created.
18 older auto-saves pruned during this session.
```

- 守护进程在后台运行，不影响其他命令
- 默认只保留最近 50 个自动保存，超出的自动删除（`--keep` 可调整）
- 手动保存 (`drift save`) 不受影响，只清理 `[auto]` 快照
- 自动快照是"安全网"，建议配合手动 `drift save -m "关键节点"` 使用
- 关闭终端时守护进程自动退出

Output — 状态（未运行）：

```
>>> Watch [inactive]
No watch daemon running.
Start with 'drift watch on'.
```

Error：

```
>>> Watch [failed]
Error: a watch daemon is already running (PID 4821).
  hint: use 'drift watch off' to stop it first.
```

- 守护进程在后台运行，不影响其他命令
- 自动快照是"安全网"，建议配合手动 `drift save -m "关键节点"` 使用
- 关闭终端时守护进程自动退出

---

### `drift check`

```
drift check [--fix]

校验 .drift/ 目录中所有块的数据完整性，验证 BLAKE3 哈希。

选项：
  --fix  尝试修复损坏的块

示例：
  drift check
  drift check --fix
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

Output — 有损坏（未修复）：

```
>>> Check [warning]
  blocks:  142 total, 140 passed
  corrupt: 2
  missing: 0

  hint: use --fix to attempt repair.
```

Output — 已修复：

```
>>> Check [ok]
  blocks:  142 total, 140 passed → 142 passed
  repaired: 2 blocks.
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
drift gc [--dry-run]

回收不再被任何分支或标签引用的快照与块，释放存储空间。
删除分支后留下的孤立快照、以及这些快照独占的块，都会被清理。

回收算法：
  1. 从所有 refs（heads/*、tags/*）的 target 出发，沿快照 PrevID 链
     遍历，标记所有可达快照。
  2. 删除未被标记的快照（即"不可达快照"）。
  3. 扫描所有可达快照引用的块哈希，删除未被引用的块。
  4. 先删快照、后删块，保证中途任何时刻都不会出现快照引用已删块的情况。

选项：
  --dry-run  只统计将要回收的数量，不实际删除

示例：
  drift gc --dry-run    # 预览将回收多少快照与块
  drift gc              # 执行回收
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

---

## 版本引用语法

```
<N-hash>       快照哈希前缀（至少 4 位），如 12ab
@tag:<name>    通过 tag 定位，如 @tag:submission
main           主线分支名
<branch>       分支名
```

---

## Git concept mapping

| User sees | Git equivalent | Difference |
|---------|-----------|------|
| `save` | `add + commit` | Auto-includes all changes, no staging area |
| `snap` / snapshot | `commit` | - |
| `log` | `log --oneline --graph` | With thumbnail preview |
| `restore` | `reset` / `checkout` | Auto-backup before restore |
| `branch` | `branch` | Create and list; no merge |
| `switch` | `checkout` / `switch` | Auto-save before switch |
| `tag` | `tag` | Tag via `save --tag` |
| `main` | `main` / `master` | - |
| `diff` | `diff` | Supports images, visual diff |
| - | `merge`, `rebase`, `stash`, `cherry-pick`, `bisect` | Intentionally omitted |

---

## 分阶段实现计划

### 第一阶段：本地核心

```
init    save    log    show    status    restore    check
```

这 7 个命令跑通，就是一个完整可用的本地版本管理工具。

### 第二阶段：分支 + 自动化

```
branch    switch    ignore    watch    gc
```

### 第三阶段：远程

```
remote    sync
```

---

## 命令速查卡

```
drift init                             initialize a project
drift save -m "msg"                    save with message
drift save -m "msg" --tag "v1"         save with tag
drift log -l 10                        show last 10 entries
drift log -v 12ab                      show file change details
drift show 12ab chapter.md             view old version of a file
drift show @tag:v1 cover.psd           show metadata for image
drift show @tag:v1 cover.psd --open    open image with system app
drift status                           show changes since last save
drift status -s                        short format, paths only
drift diff                             working tree vs last save
drift diff 12ab 9f1e                   diff between two snapshots
drift restore 12ab                     restore to a snapshot
drift restore 12ab chapter.md          restore a single file
drift branch                           list all branches
drift branch "new-direction"           create a branch
drift branch -d "old-experiment"       delete a branch
drift branch -m "dev"                  rename current branch
drift branch -m "feature" "dev"        rename a specific branch
drift switch -c "experiment"           create and switch
drift switch main                      switch back to main
drift ignore "*.psd"                   ignore PSD files
drift ignore --list                    list ignore rules
drift ignore --remove "*.psd"          remove ignore rule
drift watch on                          start auto-watch daemon
drift watch off                         stop auto-watch daemon
drift watch status                      check daemon status
drift check                            verify data integrity
drift gc --dry-run                     preview reclaimable snapshots and chunks
drift gc                               reclaim unreachable snapshots and chunks
```
