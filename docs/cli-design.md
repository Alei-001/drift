# drift CLI 命令设计

## 设计原则

- **无 Git 术语** — 用创作者的日常语言：保存、恢复、分支，而非 commit/rebase/reset
- **零暂存区** — `save` 自动捕获所有变更，不要求用户手动选择文件
- **命令精简** — 每个命令只做一件明确的事，避免像 git 那样一个命令有几十个 flag
- **不安全操作不暴露** — 没有 `reset --hard`，恢复永远可撤销
- **GUI 为视觉主力** — 缩略图时间线在 GUI 中展现，CLI 专注快速、精确、可脚本化操作

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
├── branch       创建新的分支
├── switch       切换到主分支或其他分支
├── ignore       忽略文件或目录
├── watch        自动监听并保存
├── check        校验数据完整性
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
效果：创建 .drift/ 目录，开始追踪文件。

示例：
  drift init
  drift init ~/Documents/my-novel
```

- 创建 `.drift/` 目录（类似 `.git/`）
- 自动添加 `.driftignore` 默认模板（排除 .DS_Store / Thumbs.db / 系统临时文件）
- 可选：交互式初始化（选择要追踪的文件类型）

---

### `drift save`

```
drift save [-m <message>] [--tag <name>]

保存当前所有变更，创建一个新快照。

选项：
  -m, --message    快照消息（不提供时打开 $EDITOR 或记事本让用户编写）
  --tag            为这个快照起一个固定别名，如 --tag "交稿v1"

示例：
  drift save                                # 打开编辑器
  drift save -m "第三章初稿完成"              # 直接提供消息
  drift save -m "修改封面配色" --tag "交稿版"  # 消息 + 标签
```

- 自动扫描所有变更的文件（新增、修改、删除）
- 对新增的大文件进行 CDC 分块，只存储变化的块
- 对图片类文件自动生成缩略图并缓存（供 GUI 使用）
- 消息即版本标识：log 中展示，restore 用哈希或 tag 定位
- 与 `drift watch` 自动保存不同：手动 save 代表有意义的检查点

---

### `drift log`

```
drift log [--limit <n>] [--compact] [--all]
drift log -v <id>                  # 查看指定快照的文件变更明细

浏览历史快照。默认只显示用户手动创建的快照，[auto] 快照隐藏。

选项：
  -l, --limit     显示最近 N 条记录（默认 10）
  -v, --verbose    查看某个快照的文件变更明细（需指定 ID）
  -c, --compact   纯文本模式，适合脚本处理
  --all           包括自动保存 (drift watch) 的快照

示例：
  drift log
  drift log -l 20
  drift log -v 12ab               # 查看 12ab 的变更文件列表
  drift log --all                 # 包括自动保存
```

默认输出：
```
ID    时间           消息                   变更
────  ────────────  ────────────────────  ──────
12ab  06-28 16:30   第三章初稿完成           +2 Δ1
a3c2  06-27 22:15   修改封面配色              Δ1
9f1e  06-27 10:00   提交甲方第一版 [交稿v1]   +1 Δ1
```

`-v <id>` 模式（展开单个快照文件列表）：
```
drift log -v 12ab

快照 12ab  06-28 16:30  第三章初稿完成

  新增:
    chapter4.md     12.3 KB
    sketch.png       2.1 MB
  修改:
    chapter3.md     45.2 KB  (42行)
```

`--all` 模式（含自动保存）：
```
ID    时间           消息                   变更
────  ────────────  ────────────────────  ──────
12ab  06-28 16:30   第三章初稿完成           +2 Δ1
f3e2  06-28 16:25   auto - 06-28 16:25    Δ1          ← 灰色
9f1e  06-27 10:00   提交甲方第一版 [交稿v1]   +1 Δ1
```

> **关于缩略图**：PreviewStore 生成的缩略图仅供 GUI 时间线视图使用，CLI 中不展示。如需查看图片内容，用 `drift show <hash>` 调用系统默认查看器。

---

### `drift show`

```
drift show <version> <file>

查看指定快照中某个文件的内容。文本文件用分页器展示，非文本文件调用系统默认程序打开。

示例：
  drift show 12ab chapter1.md        # 分页查看老版本文本
  drift show @tag:交稿v1 cover.psd    # 用系统程序打开旧版图片
  drift show main README.md           # 查看当前分支的文件
```

- 文本文件：stdout 输出或通过 less/pager 分页
- 图片/二进制：调用 `open`(macOS) / `start`(Win) / `xdg-open`(Linux) 打开
- 不影响工作区，不做 restore

---

### `drift status`

```
drift status

查看自上次 save 以来的变更情况。

输出示例：
  自上次保存 (3分钟前)，3 个文件有变更：

   A  chapter4.md
   A  assets/sketch.png
   M  chapter3.md

状态字母：
  A  Added     新文件
  D  Deleted   已删除
  M  Modified  已修改

> 重命名目前显示为删除（D） + 新增（A）对，后续版本支持 R 标记合并显示。
```

---

### `drift diff`

```
drift diff                    对比工作区 vs 上次快照
drift diff <id>               对比工作区 vs 指定快照
drift diff <id1> <id2>        对比两个快照
drift diff <id1> <id2> <file> 对比某文件在两个快照间的差异

示例：
  drift diff                           # 刚刚改了哪些文件？
  drift diff 12ab                      # 和 v12ab 比改了什么？
  drift diff 12ab 9f1e                 # 两个历史版本之间的差异
  drift diff 12ab 9f1e chapter3.md     # 某文件的历史差异
```

输出（无 file 参数时，文件级汇总）：
  M  chapter4.md        ← 12ab 增, 9f1e 删 → M(modified)
  A  assets/sketch.png

输出（有 file 参数时，行级 unified diff）：
  --- a/chapter3.md
  +++ b/chapter3.md
  @@ -12,3 +12,3 @@
   ... 标准的 unified diff ...

> **文件级 vs 单文件差异**：无 file 参数时所有文件一视同仁（只比较哈希是否变更）。
> 指定 file 参数时，文本文件输出 unified diff；图片/二进制文件仅显示元信息和大小变化，
> "文件已变更"（不展开差异）。

---

### `drift restore`

```
drift restore <version> [<file>]

恢复项目（或单个文件）到指定快照的状态。

⚠ 恢复前会自动备份当前状态（消息格式：backup: restore to <id>），避免误操作丢失。

选项：
  --no-backup     跳过自动备份

示例：
  drift restore 12ab              # 恢复到指定快照
  drift restore 12ab chapter3.md  # 只恢复单个文件
  drift restore @tag:交稿v1       # 恢复到标记版本
```

- 恢复前自动备份当前工作区到新快照
- 备份快照在 `drift log --all` 中可见（灰色显示）

---

### `drift branch`

```
drift branch <name>

从当前快照创建一个新的分支（不切换）。

示例：
  drift branch 新配色方案
  drift branch 第三人称版本
```

- 只创建分支引用，不切换当前分支
- 如需创建并切换，用 `drift switch -c <name>`

---

### `drift switch`

```
drift switch <name>            切换到已有分支
drift switch -c <name>         创建并切换到新分支
drift switch main              切换到主线

选项：
  -c    创建新分支并切换

示例：
  drift switch main              # 回到主线
  drift switch 新配色方案         # 切换到已有分支
  drift switch -c 实验性构图      # 创建并切换
```

- 切换前自动 save 当前状态
- 打印切换前后的快照差异摘要

---

### `drift ignore`

```
drift ignore <pattern...>

将文件或 glob 模式加入忽略列表。

示例：
  drift ignore "*.tmp" "*.psd"
  drift ignore "backup/" "Thumbs.db"
  drift ignore --list            # 列出当前忽略规则
```

---

### `drift watch`

```
drift watch [--interval <seconds>]

进入监听模式，检测到文件变更后自动保存。
自动保存的快照消息格式为 `auto - 2026-06-28 16:30`，在 log 中以灰色显示。

选项：
  --interval  检测间隔（默认 300 秒 = 5 分钟）

示例：
  drift watch
  drift watch --interval 600     # 每 10 分钟自动保存
```

- 静默运行于后台，不弹出编辑器
- 自动快照是"安全网"，建议配合手动 `drift save -m "关键节点"` 使用
- Ctrl+C 退出

---

### `drift check`

```
drift check [--fix]

校验 .drift/ 目录中所有块的数据完整性，验证 BLAKE3 哈希。

选项：
  --fix  尝试修复损坏的块（从冗余信息中恢复）

示例：
  drift check
  drift check --fix
```

- 遍历所有块文件，重新计算 BLAKE3 哈希并与文件名比对
- 遍历所有快照，检查引用的块是否存在
- 输出检查报告：总块数 / 通过 / 损坏 / 缺失
- `--fix` 模式下尝试从副本恢复（如存在 pack 冗余）
- 本命令可定期运行或 CI 中集成

---

## 版本引用语法

```
<N-hash>       快照哈希前缀（至少 4 位），如 12ab
@tag:<name>    通过 tag 定位，如 @tag:交稿v1
main           主线分支名
<branch>       分支名
```

---

## 与 Git 概念映射（仅供参考，不对用户暴露）

| 用户看到 | Git 等价物 | 差异 |
|---------|-----------|------|
| `save` | `add + commit` | 自动包含所有变更，无暂存区 |
| `snap` / 快照 | `commit` | - |
| `log` | `log --oneline --graph` | 带缩略图预览 |
| `restore` | `reset` / `checkout` | 恢复前自动备份 |
| `branch` | `branch` | 创建新分支 |
| `switch` | `checkout` / `switch` | 切换前自动保存 |
| `tag` | `tag` | 不变 |
| `main` | `main` / `master` | - |
| `branch` | `branch` | 不可合并，纯分叉 |
| `diff` | `diff` | 支持图片、视觉差异 |
| - | `merge`, `rebase`, `stash`, `cherry-pick`, `bisect` | 故意不做 |

---

## 分阶段实现计划

### 第一阶段：本地核心（目标：能存能看能回）

```
init    save    log    show    status    restore    check
```

这 6 个命令跑通，就是一个完整可用的本地版本管理工具。

### 第二阶段：分支 + 自动化

```
new    switch    ignore    watch
```

### 第三阶段：远程

```
remote    sync
```

---

## 命令速查卡

```
drift init                    初始化项目
drift save                    打开编辑器保存快照
drift save -m "说明"          带消息保存快照
drift log -l 10               查看最近 10 条记录
drift log -v 12ab              查看变更文件列表
drift show 12ab chapter.md      查看老版本文本
drift show @tag:xxx cover.psd   用系统程序打开旧版文件
drift status                  查看变更状态
drift diff                       工作区变更 vs 上次保存
drift diff 12ab 9f1e              两个快照间的差异
drift restore 12ab                  恢复到指定快照
drift restore 12ab chapter.md       恢复单个文件
drift branch "新方向"             创建分支（不切换）
drift switch -c "实验"           创建并切换
drift switch main             切回主线
drift ignore "*.psd"          忽略 PSD 文件
drift watch                   开始自动监听
drift check                   校验数据完整性
```
