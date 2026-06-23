# Drift - CLI 命令参考

## 设计原则

| 原则 | 说明 |
|------|------|
| **简洁性** | 比 Git 更少的命令 |
| **直观性** | 命令名即功能（save / log / export / restore） |
| **非技术友好** | 面向创意工作者，学习成本低 |

## 命令列表

| 命令 | 说明 | 状态 |
|------|------|------|
| `init` | 初始化项目 | ✅ |
| `add` | 添加文件到暂存区 | ✅ |
| `status` | 查看工作区状态 | ✅ |
| `unstage` | 清空或移除暂存区文件 | ✅ |
| `save` | 保存版本 | ✅ |
| `log` | 查看提交历史（`--all` 跨分支） | ✅ |
| `export` | 导出版本 | ✅ |
| `restore` | 恢复工作区 | ✅ |
| `diff` | 对比差异 | ✅ |
| `branch` | 管理分支 | ✅ |
| `switch` | 切换分支 | ✅ |
| `config` | 配置管理 | ✅ |
| `rm` | 删除文件 | ✅ |
| `mv` | 移动/重命名文件 | ✅ |
| `name` | 版本别名 | ✅ |
| `history` | 操作历史 | ✅ |
| `undo` | 撤销操作 | ✅ |
| `wip` | 工作进度管理 | ✅ |

---

## 初始化命令

### `drift init` ✅

初始化一个新的 Drift 项目。

```bash
drift init
```

**行为：**
- 在当前目录创建 `.drift/` 文件夹
- 初始化存储结构和默认配置
- 创建 `main` 分支并设置 HEAD
- 交互式提示输入 `user.name` 和 `user.email`（在终端中运行时）
- 显示下一步指引

---

## 暂存区命令

### `drift add` ✅

添加文件到暂存区。

```bash
drift add <路径> [<路径>...]   # 支持多个路径
drift add .                    # 添加所有文件
drift add 章节/                # 添加整个目录
drift add *.txt                # 支持 glob 通配符
drift add 素材/ 章节/第一章.txt # 混合添加目录和文件
```

**行为：**
- 支持多个路径参数，一次性添加多个文件/目录
- 支持 glob 模式（`*`、`?`、`[...]`）
- 重复添加相同内容自动跳过
- 计算文件 SHA-256 哈希，存入 `objects/blobs/`
- 更新暂存区（index）

### `drift status` ✅

查看工作区状态。

```bash
drift status                  # 人类可读格式（默认）
drift status --porcelain      # 机器可读格式
```

**输出示例（默认）：**

```
On branch main, version v2

Staged changes:
  A 章节/第一章.txt
  M 章节/第二章.txt

Unstaged changes:
  D 素材/旧图.png

Untracked files:
  新笔记.txt
```

**输出示例（--porcelain）：**

```
A 章节/第一章.txt
M 章节/第二章.txt
 D 素材/旧图.png
?? 新笔记.txt
```

**状态标识：**
- `A` — Added（新增）
- `M` — Modified（修改）
- `D` — Deleted（删除）
- `?` — Untracked（未跟踪）

**porcelain 格式说明：** 每行格式为 `XY <路径>`，其中 X 为暂存区状态，Y 为工作区状态。

### `drift unstage` ✅

从暂存区移除文件。

```bash
drift unstage                 # 清空整个暂存区
drift unstage <路径>           # 仅移除指定文件
```

**行为：**
- 不带参数时清空整个暂存区
- 带路径参数时只移除该文件
- 不影响工作区文件

---

## 版本命令

### `drift save` ✅

保存暂存区为新版本。

```bash
drift save                    # 无备注
drift save -m "备注信息"       # 带备注
drift save --name 初稿         # 保存并同时设置别名
drift save -m "初稿" --name 初稿  # 带备注和别名
drift save --amend            # 修改最近一条版本（保留版本号）
drift save --amend -m "新备注" # 修改版本并更新备注
drift save -a -m "备注"        # 自动暂存所有改动后保存
drift save --all              # 等同于 drift add . + drift save
```

`-m` / `--message` 可选。`--name` 可选，用于在保存时直接为该版本设置别名（等效于保存后执行 `drift name <版本> <别名>`）。`-a` / `--all` 可选，自动暂存工作区所有改动后再保存（类似 `git commit -a`），无需先执行 `drift add`。

**行为：**
- 从暂存区构建 Tree 对象
- 创建 Commit 对象（每个分支独立递增：main 的 v1, v2...；feature 的 v1, v2...）
- 更新当前分支引用并清空暂存区
- 若文件内容与上一版本完全相同，拒绝保存
- 保存后列出本次所有保存的文件
- 若指定 `--name`，保存成功后自动创建别名
- 若指定 `--all`，保存前自动暂存所有改动（含新增、修改、删除的已跟踪文件）

**--amend 行为：**
- 替换最近一条提交，保留版本号（ID）和 parent
- 更新 Tree 和 Message
- 不会创建新的版本号
- 用于修正最近保存的版本

### `drift log` ✅

查看提交历史。

```bash
drift log                    # 当前分支完整历史
drift log <分支名>            # 指定分支历史
drift log --all              # 所有分支历史（去重，按时间倒序）
drift log --oneline          # 单行模式
drift log -n 5               # 只显示最近 5 条
drift log main -n 10         # main 分支最近 10 条
drift log --all --oneline    # 所有分支单行模式
```

**参数：**

| 参数 | 说明 |
|------|------|
| `<分支名>` | 可选，指定要查看的分支（不存在时报错） |
| `--all` | 显示所有分支的提交（按 hash 去重，按时间倒序） |
| `--oneline` | 单行模式，简洁显示 |
| `-n` / `--number` | 限制显示的提交数量 |

**输出示例（完整模式，单分支）：**

```
commit abc123def456...
Version: v3
Branch:  main
Date:    2024-06-15 10:30:00
Author:  张三 <zhangsan@example.com>

    完成前四章

commit def456abc123...
Version: v2
Branch:  main
Date:    2024-06-15 09:00:00

    修改配色方案
```

**输出示例（`--all` 模式）：**

```
Version history:

  v3  [main]  完成前四章
  v2  [main]  修改配色方案
  v1  [feature]  方案A初稿
```

**输出示例（单行模式）：**

```
v3 [main] 完成前四章
v2 [main] 修改配色方案
v1 [main] 项目初始化
```

> **注意**：`drift log` 不带参数时默认查看当前分支历史；`drift log --all` 查看所有分支历史（去重）。前者适合专注当前工作线，后者适合总览全局。

### `drift export` ✅

导出指定版本到文件系统。

```bash
drift export <版本> -o <输出路径> [-f <格式>]

drift export v1 -o ./交付客户           # 导出到目录（默认）
drift export v1 -o ./交付.zip -f zip    # 导出为 zip
drift export v1 -o ./交付.tar.gz -f tar # 导出为 tar.gz
drift export main -o ./main-branch     # 导出分支最新版本
```

**参数：**
- `<版本>` — 版本 ID（如 v1、v2）、分支名（如 main）或别名（如 初稿）
- `-o` / `--output` — **必填**，输出路径
- `-f` / `--format` — 可选，`dir`（默认）/ `zip` / `tar`

### `drift restore` ✅

恢复工作区到指定版本。

```bash
drift restore <版本>                    # 恢复整个工作区到指定版本
drift restore <版本> --force            # 强制恢复（丢弃暂存区改动）
drift restore main                      # 恢复到分支最新版本
drift restore v2 章节/第一章.txt         # 仅恢复指定文件到 v2
drift restore v2 章节/第一章.txt 笔记/   # 仅恢复指定文件/目录到 v2
```

**参数：**
- `<版本>` — 版本 ID（如 v1、v2）、分支名（如 main）或别名（如 初稿）
- `<路径>...` — 可选，指定要恢复的文件或目录。不指定则恢复整个工作区。

**行为：**
- 将工作区文件恢复到目标版本状态
- **不改变分支引用**（只改变工作树内容）
- 暂存区与当前版本不同时需 `--force`
- 未跟踪文件不受影响（保留）
- 指定路径时，仅恢复匹配的文件（支持目录前缀匹配），其他文件保持不变

> **注意**：`restore` 只改变工作树内容，分支引用保持不变。指定路径时可只回退部分文件，适合工作区文件较多、只需恢复个别文件的场景。

---

## 文件管理命令

### `drift rm` ✅

删除文件并从暂存区移除。

```bash
drift rm <路径> [<路径>...]       # 删除一个或多个文件
drift rm --cached <路径>          # 仅从暂存区移除，保留工作区文件
drift rm -r 目录名                # 递归删除目录
drift rm *.tmp                    # 支持 glob 通配符
```

**参数：**

| 参数 | 说明 |
|------|------|
| `--cached` | 仅从暂存区移除，保留磁盘文件 |
| `-r` / `--recursive` | 递归删除目录（必需参数用于目录） |

**行为：**
- 只允许操作已跟踪的文件（暂存区或已提交）
- 默认同时删除工作区文件和暂存区条目
- `--cached` 保留磁盘文件，仅更新暂存区
- 自动清理因此而变空的父目录

### `drift mv` ✅

移动或重命名已跟踪的文件。

```bash
drift mv <源> <目标>               # 重命名文件
drift mv <文件> <目录>              # 移入目录
drift mv <文件1> <文件2> <目录>     # 多个文件移入目录
```

**行为：**
- 操作已跟踪的文件（暂存区或已提交均可）
- 同时更新工作区和暂存区
- 目标为已存在的目录时，自动将源移入该目录
- 自动清理空的源目录

---

## 分支命令

### `drift branch` ✅

创建、查看、删除或重命名分支。

```bash
drift branch <名称>      # 基于当前版本创建分支
drift branch list        # 查看所有分支
drift branch -d <名称>   # 删除分支
drift branch -m <新名> <旧名>  # 重命名分支
```

**输出示例：**

```
* main
  方案A
  方案B
```

**删除约束：**
- 不能删除 HEAD
- 不能删除当前所在分支（需先切换到其他分支）

**重命名约束：**
- 不能重命名 HEAD
- 目标名称不能与已有分支重名
- 若当前在该分支上，HEAD 自动更新为新名

**操作记录：** 删除和重命名操作会被记录到操作历史中，可通过 `drift undo` 恢复。

### `drift switch` ✅

切换到指定分支。

```bash
drift switch <名称>
drift switch <名称> --force     # 强制切换（丢弃未保存改动）
drift switch <名称> --create    # 分支不存在时自动创建并切换
drift switch <名称> -c          # --create 简写
```

**行为：**
- 将工作区切换到目标分支的最新版本
- 删除目标分支不存在的文件
- **暂存区和工作区有改动时自动保存到 WIP**（不再拒绝切换）
- `--force` 忽略所有未保存改动直接切换
- `--create`/`-c`：分支不存在时从当前分支创建，已存在时报错
- 切换后可执行 `drift wip` 查看或恢复保存的工作进度

> **设计原则**：分支是独立版本线，不做 merge。作家用分支写不同剧情线，设计师用分支试不同配色方案。

---

## 对比命令

### `drift diff` ✅

查看文件差异。

**默认显示摘要统计：**

```bash
drift diff                # 工作区 vs 当前分支最新版本（摘要）
drift diff v1             # 工作区 vs v1（摘要）
drift diff v1 v2          # v1 vs v2（摘要）
drift diff main feature   # main 最新 vs feature 最新（摘要）
drift diff main/v1 feature/v1  # 跨分支精确比较（摘要）
```

**摘要输出示例：**

```
Changed files between v1 and v2:

  M 章节/第一章.txt    +15 -3   (text)
  M 章节/第二章.txt    +8 -2    (text)
  M 素材/封面.psd      1.2MB -> 1.5MB  (binary)
  D 笔记/旧笔记.txt    1024 bytes  (text)
  A 笔记/新笔记.txt    2048 bytes  (text)

Summary: 5 files changed (3 text, 1 binary), 23 insertions(+), 5 deletions(-)
```

**查看详细差异：**

```bash
drift diff -p             # 工作区 vs 当前分支（详细）
drift diff v1 v2 -p       # v1 vs v2（详细）
```

**详细输出示例（文本文件）：**

```
--- v1/章节/第一章.txt
+++ v2/章节/第一章.txt
@@ -1,5 +1,8 @@
 第一章 开始
 
-这是一个故事的开始。
+这是一个关于冒险的故事。
+
+主角是一个年轻的旅行者。
 
 天气晴朗。
```

**二进制文件输出：**

```
Binary file 素材/封面.psd changed (1234567 -> 1567890 bytes)
```

**指定文件：**

```bash
# 三种等价的文件过滤方式（可混用）：
drift diff v1 v2 --file 章节/第一章.txt         # --file 标志
drift diff v1 v2 -f 章节/第一章.txt              # -f 简写
drift diff v1 v2 -- 章节/第一章.txt              # -- 分隔符后直接跟路径

# 多个文件/目录（每种方式都支持重复）：
drift diff v1 v2 --file 章节/ --file 笔记/
drift diff v1 v2 -f 章节/ -f 笔记/
drift diff v1 v2 -- 章节/ 笔记/
```

**输出到文件：**

```bash
drift diff v1 v2 -p --output diff.txt      # 保存差异到文件
```

**参数说明：**

| 参数 | 说明 |
|------|------|
| `-p` / `--patch` | 显示详细行级差异 |
| `--file <路径>` | 只比较指定文件（可重复使用） |
| `-f <路径>` | `--file` 的简写（可重复使用） |
| `-- <路径>...` | 在 `--` 分隔符后直接跟路径，无需标志 |
| `-o` / `--output <文件>` | 输出到文件而非命令行 |

**版本标识符格式：**

| 格式 | 示例 | 说明 |
|------|------|------|
| 版本 ID | `v1` | 当前分支的版本（如有歧义会提示） |
| 分支名 | `main` | 该分支的最新版本 |
| 分支/版本 | `main/v1` | 精确指定某分支的某版本 |
| 别名 | `初稿` | 通过 `drift name` 设置的别名 |

---

## 版本别名命令

### `drift name` ✅

为版本设置友好别名（类似 Git 的 tag）。

```bash
drift name <版本> <标签名>    # 为版本设置别名
drift name --list             # 查看所有别名
drift name --delete <标签名>  # 删除别名
```

**示例：**

```bash
drift name v1 初稿
drift name v3 终稿
drift name --list
drift name --delete 初稿
```

**行为：**
- 别名为版本提供一个易记的名称
- 别名可在所有需要版本号的命令中使用（diff、export、restore 等）
- 别名以 `refs/names/<标签>.ref` 存储在 `.drift/` 中

---

## 操作历史命令

### `drift history` ✅

查看操作历史记录。

```bash
drift history                 # 查看所有操作记录
drift history -n 10           # 只显示最近 10 条
```

**输出示例：**

```
2024-06-23 10:30:00  save v3  (修改配色) on main
2024-06-23 10:25:00  save v2  on main
2024-06-23 10:20:00  branch | create experiment
2024-06-23 10:15:00  save v1  on main
```

**记录的操作类型：**
- `save` — 保存版本
- `branch` — 创建/删除/重命名分支
- `switch` — 切换分支（HEAD 变更）

### `drift undo` ✅

撤销最近的操作。

```bash
drift undo                    # 撤销最近一次操作
drift undo -n 3               # 撤销最近 3 次操作
```

**行为：**
- 恢复被操作影响的分支引用到操作前的状态
- 例如：撤销 `save` 会移除该提交并恢复分支指向
- 撤销 `branch -d` 会恢复被删除的分支
- 撤销 `branch -m` 会恢复分支原名
- 操作历史本身也会记录撤销操作

---

## 工作进度管理

### `drift wip` ✅

查看已保存的工作进度（Work-In-Progress）。

```bash
drift wip                     # 查看所有分支的 WIP
```

**输出示例：**

```
  main       3 file(s)
  feature    1 file(s)
```

**说明：**
- 切换分支时，如果有未保存的改动，会自动保存到 WIP
- WIP 以 `.drift/wip/<分支名>.json` 存储
- 不会自动恢复，需要手动执行 `drift restore-wip`

### `drift restore-wip` ✅

恢复之前保存的工作进度。

```bash
drift restore-wip             # 恢复当前分支的 WIP
drift restore-wip <分支名>     # 恢复指定分支的 WIP
```

**行为：**
- 将 WIP 中的文件恢复到工作区和暂存区
- 恢复后自动删除 WIP 文件

---

## 配置命令

### `drift config` ✅

查看或设置配置选项。

```bash
drift config <key>              # 查看配置值
drift config <key> <value>      # 设置配置值
drift config --list             # 列出所有配置
drift config --unset <key>      # 清除配置值
```

**支持的配置项：**

| 配置键 | 说明 | 默认值 |
|--------|------|--------|
| `user.name` | 用户姓名（用于版本记录） | 空 |
| `user.email` | 用户邮箱（用于版本记录） | 空 |
| `core.default_branch` | 默认分支名称 | `main` |
| `core.autocrlf` | CRLF 换行符归一化策略 | `""`（不做转换） |

**示例：**

```bash
drift config user.name "张三"
drift config user.email "zhangsan@example.com"
drift config user.name        # 查看当前用户名
drift config --list            # 列出所有配置
drift config --unset user.email  # 清除邮箱配置
drift config core.default_branch dev  # 设置默认分支为 dev
drift config core.autocrlf true       # 开启 CRLF 归一化
drift config core.autocrlf input      # 仅 add 时 CRLF→LF
```

---

## 帮助

```bash
drift --help
drift <命令> --help
```

## 退出码

| 码 | 含义 |
|----|------|
| 0 | 成功 |
| 1 | 一般错误 |

---

## 错误处理

| 错误信息 | 原因 | 解决方案 |
|----------|------|----------|
| `not a drift project (run 'drift init')` | 未运行 `drift init` | 运行 `drift init` |
| `file not found` / `path not found` | 路径错误 | 检查文件路径 |
| `nothing to save (use 'drift add' first)` | 暂存区为空 | 运行 `drift add` |
| `nothing changed since last version` | 文件内容未变化 | 修改文件后重新 `drift add` |
| `version not found` | 版本 ID 错误 | 运行 `drift log --all` |
| `staging area has pending changes` | 暂存区非空 | 运行 `drift unstage` 或使用 `--force` |
| `branch not found` | 分支名错误 | 运行 `drift branch list` |
| `branch "X" already exists` | 分支名重复 | 使用其他分支名或 `branch -m` 重命名 |
| `cannot delete the currently checked-out branch` | 试图删除当前分支 | 先 `drift switch` 到其他分支 |
| `could not acquire lock` | 另一个 drift 进程正在运行 | 等待或检查 PID（见 `.drift/lock`） |
| `pathspec 'X' did not match any tracked files` | 路径不是已跟踪文件 | 先 `drift add` |
| `not a drift project` | 当前目录未初始化 | `drift init` |
| `no version to amend` | 还没有版本可修改 | 先 `drift save` 创建版本 |
| `unsafe symlink` | 符号链接指向仓库外 | 使用仓库内的路径 |
