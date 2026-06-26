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
| `tag` | 版本标签 | ✅ |
| `reflog` | 操作历史 | ✅ |
| `undo` | 撤销操作 | ✅ |
| `wip` | 工作进度管理 | ✅ |
| `sync` | 远程同步（NAS/WebDAV） | ✅ |
| `clone` | 从远程克隆项目 | ✅ |
| `version` | 显示版本号 | ✅ |

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
- 用户信息保存到**全局配置**（`~/.drift/global.json`），所有项目共享
- 如已存在全局用户信息，则作为默认值显示（按 Enter 保留）
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
drift save --tag v1               # 保存并同时设置标签
drift save -m "first draft" --tag v1  # 带备注和标签
drift save --amend            # 修改最近一条版本
drift save --amend -m "新备注" # 修改版本并更新备注
drift save -a -m "备注"        # 自动暂存所有改动后保存
drift save --all              # 等同于 drift add . + drift save
```

`-m` / `--message` 可选。`--tag` 可选，用于在保存时直接为该版本设置标签（等效于保存后执行 `drift tag <版本> <标签>`）。`-a` / `--all` 可选，自动暂存工作区所有改动后再保存（类似 `git commit -a`），无需先执行 `drift add`。

**行为：**
- 从暂存区构建 Tree 对象
- 创建 Commit 对象，版本 ID 为提交哈希的前 8 位（如 `a1b2c3d4`），类似 Git 的短哈希
- 更新当前分支引用并清空暂存区
- 若文件内容与上一版本完全相同，拒绝保存
- 保存后列出本次实际变更的文件（与上一版本比较，非全部已跟踪文件）
- 若指定 `--tag`，保存成功后自动创建标签
- 若指定 `--all`，保存前自动暂存所有改动（含新增、修改、删除的已跟踪文件）

**--amend 行为：**
- 替换最近一条提交，生成新的版本 ID（新哈希前缀）
- 更新 Tree 和 Message
- 用于修正最近保存的版本

### `drift log` ✅

查看提交历史。

```bash
drift log                    # 当前分支完整历史
drift log <分支名>            # 指定分支历史
drift log --all              # 所有分支历史（去重，按时间倒序）
drift log --oneline          # 单行模式
drift log -n 5               # 显示最近 5 条
drift log main -n 10         # main 分支最近 10 条
drift log --all --oneline    # 所有分支单行模式
```

**参数：**

| 参数 | 说明 |
|------|------|
| `<分支名>` | 可选，指定要查看的分支（不存在时报错） |
| `--all` | 显示所有分支的提交（按 hash 去重，按时间倒序） |
| `--oneline` | 单行模式，简洁显示 |
| `-n` / `--number` | 限制显示的提交数量（0 = 全部） |
| `--porcelain` | 机器可读格式 |

**输出示例（完整模式，单分支）：**

```
  commit abc123def456...
  Version: a1b2c3d4
  Names:   v1
  Branch:  main
Date:    2024-06-15 10:30:00
Author:  张三 <zhangsan@example.com>

    完成前四章

commit def456abc123...
Version: e5f6a7b8
Branch:  main
Date:    2024-06-15 09:00:00

    修改配色方案
```

**输出示例（`--all` 模式）：**

```
Version history:

  a1b2c3d4 (v1)  [main]  完成前四章
  e5f6a7b8       [main]  修改配色方案
  c9d0e1f2       [feature]  方案A初稿
```

**输出示例（单行模式）：**

```
a1b2c3d4 (v1) [main] 完成前四章
e5f6a7b8 [main] 修改配色方案
c9d0e1f2 [main] 项目初始化
```

> **注意**：`drift log` 不带参数时默认查看当前分支历史；`drift log --all` 查看所有分支历史（去重）。前者适合专注当前工作线，后者适合总览全局。

### `drift export` ✅

导出指定版本到文件系统。

```bash
drift export <版本> -o <输出路径> [-f <格式>] [<路径>...]

# 基本用法
drift export a1b2c3d4 -o ./交付客户           # 导出到目录（默认）
drift export a1b2c3d4 -o ./交付.zip -f zip    # 导出为 zip
drift export a1b2c3d4 -o ./交付.tar.gz -f tar # 导出为 tar.gz
drift export main -o ./main-branch     # 导出分支最新版本
drift export v1 -o ./draft              # 使用标签导出

# 路径过滤（只导出指定文件/目录）
drift export a1b2c3d4 -o ./部分 章节/         # 只导出 章节/ 目录
drift export a1b2c3d4 -o ./部分 章节/ 笔记/   # 导出多个目录
drift export a1b2c3d4 -o ./单文件 章节/第一章.txt  # 只导出单个文件
```

**参数：**
- `<版本>` — 版本 ID（如 `a1b2c3d4`，支持缩写前缀）、分支名（如 main）或标签（如 `v1`）
- `-o` / `--output` — **必填**，输出路径
- `-f` / `--format` — 可选，`dir`（默认）/ `zip` / `tar`
- `<路径>...` — 可选，指定要导出的文件或目录（支持目录前缀匹配）。不指定则导出整个版本。

> **注意**：`-f` 在 `export` 命令中表示 `--format`（导出格式），而在 `diff` 命令中表示 `--file`（过滤文件）。两个命令的标志空间相互独立，但使用时请注意上下文。

### `drift restore` ✅

恢复工作区到指定版本。

```bash
drift restore <版本>                    # 恢复整个工作区到指定版本
drift restore <版本> --force            # 强制恢复（丢弃暂存区改动）
drift restore main                      # 恢复到分支最新版本
drift restore a1b2c3d4 章节/第一章.txt   # 仅恢复指定文件到该版本
drift restore a1b2c3d4 章节/第一章.txt 笔记/   # 仅恢复指定文件/目录
drift restore v1                        # 使用标签恢复
```

**参数：**
- `<版本>` — 版本 ID（如 `a1b2c3d4`，支持缩写前缀）、分支名（如 main）或标签（如 `v1`）
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
drift rm -f *.tmp                 # 跳过确认
drift rm --dry-run *.tmp          # 预览模式，不实际删除
```

**参数：**

| 参数 | 说明 |
|------|------|
| `--cached` | 仅从暂存区移除，保留磁盘文件 |
| `-r` / `--recursive` | 递归删除目录（必需） |
| `-f` / `--force` | 跳过确认提示，直接删除 |
| `--dry-run` | 预览模式，只显示会删除的文件，不实际执行 |

**行为：**
- 只删除已被 Drift 跟踪的文件（在暂存区或已提交的版本中）；未跟踪文件会被静默跳过
- 默认同时删除工作区文件和暂存区条目
- 删除前会弹出交互确认，列出将被删除的文件，需要确认后才会执行；使用 `-f` 跳过
- `--cached` 保留磁盘文件，仅从暂存区移除
- `--dry-run` 预览将要删除的文件，不修改磁盘或暂存区
- 自动清理因此而变空的父目录

**示例：**

```bash
# 删除单个文件
drift rm 章节/旧稿.txt

# 删除多个文件
drift rm 章节/旧稿.txt 笔记/废弃.txt

# 递归删除整个目录（必须加 -r）
drift rm -r 旧素材/

# 使用 glob 通配符批量删除
drift rm *.tmp
drift rm *.bak *.old

# 仅从暂存区移除，保留磁盘文件（停止跟踪该文件）
drift rm --cached 大文件.psd

# 删除后查看状态
drift rm 章节/旧稿.txt
drift status          # 会显示该文件为 D（删除）
drift save -m "删除旧稿"  # 提交删除
```

> **注意**：`drift rm` 只会删除已被 Drift 跟踪的文件。未跟踪的文件会被静默跳过（不影响操作），这种情况请用系统命令（如资源管理器或 `del`）直接删除。删除后建议执行 `drift save` 提交修改。

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

**示例：**

```bash
# 重命名单个文件
drift mv 章节/第一章.txt 章节/序章.txt

# 将文件移入已存在的目录
drift mv 封面.png 素材/

# 多个文件移入同一目录（最后一个参数必须是目录）
drift mv 封面.png 插图.png 素材/

# 重命名目录（目录名即新文件名）
drift mv 旧章节/ 新章节/

# 移动后查看状态并提交
drift mv 章节/第一章.txt 章节/序章.txt
drift status          # 旧路径显示 D（删除），新路径显示 M（修改）或 A（新增）
drift save -m "重命名第一章为序章"
```

> **提示**：`drift mv` 同时移动磁盘文件和更新暂存区，移动后执行 `drift status` 会看到旧路径为 `D`（删除）、新路径为 `M`（修改）或 `A`（新增）。确认后执行 `drift save` 提交改动。

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
drift diff                         # 工作区 vs 当前分支最新版本（摘要）
drift diff a1b2c3d4                # 工作区 vs 指定版本（摘要）
drift diff a1b2c3d4 e5f6a7b8       # 两个版本对比（摘要）
drift diff v1 v2                   # 两个标签对比（跨分支也支持，标签全局唯一）
drift diff v1 main                 # 标签 vs 分支最新
drift diff main feature            # main 最新 vs feature 最新（摘要）
drift diff main/a1b2c3d4 feature/a1b2c3d4  # 跨分支精确比较（摘要）
```

**摘要输出示例：**

```
Changed 2 file(s):
  M 章节/第一章.txt
  A 笔记/新笔记.txt
```

如果无变化则显示 `No changes`。

**查看详细差异：**

```bash
drift diff -p                      # 工作区 vs 当前分支（详细）
drift diff a1b2c3d4 e5f6a7b8 -p    # 两个版本对比（详细）
drift diff v1 v2 -p                # 两个标签对比（详细）
```

**详细输出示例（文本文件）：**

```
--- 章节/第一章.txt
+++ 章节/第一章.txt
 第一章 开始

-这是一个故事的开始。
+这是一个关于冒险的故事。
+
+主角是一个年轻的旅行者。

 天气晴朗。
```

**二进制文件输出：**

```
Binary files differ: 素材/封面.psd
```

**指定文件：**

```bash
# 三种等价的文件过滤方式（可混用）：
drift diff a1b2c3d4 e5f6a7b8 --file 章节/第一章.txt   # --file 标志
drift diff a1b2c3d4 e5f6a7b8 -f 章节/第一章.txt        # -f 简写
drift diff a1b2c3d4 e5f6a7b8 -- 章节/第一章.txt        # -- 分隔符后直接跟路径
drift diff v1 v2 -f 章节/                                # 两个标签对比并过滤目录

# 多个文件/目录（每种方式都支持重复）：
drift diff a1b2c3d4 e5f6a7b8 --file 章节/ --file 笔记/
drift diff a1b2c3d4 e5f6a7b8 -f 章节/ -f 笔记/
drift diff a1b2c3d4 e5f6a7b8 -- 章节/ 笔记/
```

**输出到文件：**

```bash
drift diff a1b2c3d4 e5f6a7b8 -p --output diff.txt   # 保存差异到文件
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
| 版本 ID | `a1b2c3d4` | 提交哈希前 8 位，支持缩写前缀（如 `a1b2`） |
| 分支名 | `main` | 该分支的最新版本 |
| 分支/版本 | `main/a1b2c3d4` | 精确指定某分支的某版本 |
| 标签 | `v1` | 通过 `drift tag` 设置的标签 |

> **说明**：`drift diff` 支持同分支内不同版本对比（最常用），也支持跨分支对比。由于 Drift 不做合并，分支是独立的创作线，跨分支对比主要用于查看两条创作线之间的差异。

---

## 版本标签命令

### `drift tag` ✅

为版本设置友好标签（类似 Git 的 tag）。

```bash
drift tag <版本> <标签名>       # 为版本设置标签
drift tag --list                # 查看所有标签
drift tag --delete <标签名>     # 删除标签
```

**示例：**

```bash
drift tag a1b2c3d4 v1                # 为版本 a1b2c3d4 设置标签 "v1"
drift tag e5f6a7b8 final             # 为版本 e5f6a7b8 设置标签 "final"
drift tag a1b2 v1                    # 支持缩写前缀（等价于上一条）
drift tag --list                     # 查看所有标签
drift tag --delete v1                # 删除标签
```

**行为：**
	- 标签为版本提供一个易记的名称
	- 标签可在所有需要版本号的命令中使用（diff、export、restore 等）
	- 标签以 `refs/tags/<标签>.ref` 存储在 `.drift/` 中
	- 同一版本可拥有多个标签
	- 标签显示在 `drift log` 输出中（如 `a1b2c3d4 (v1) [main] ...`）

---

## 操作历史命令

### `drift reflog` ✅

查看操作历史记录。

```bash
drift reflog                  # 默认显示最近 20 条
drift reflog -n 10            # 只显示最近 10 条
drift reflog -n 0             # 显示全部操作记录
drift reflog -v               # 详细模式（显示 ref 变更详情）
drift reflog --porcelain      # 机器可读格式
```

**参数：**

| 参数 | 说明 |
|------|------|
| `-n` / `--number` | 显示的条目数量，默认 20；`0` 表示显示全部 |
| `-v` / `--verbose` | 详细模式，显示 ref 变更详情 |
| `--porcelain` | 机器可读格式 |

**输出示例：**

```
Recent operations (newest first):

  1. 2024-06-23 10:30:00  save  修改配色 on main
  2. 2024-06-23 10:25:00  save  on main
  3. 2024-06-23 10:20:00  branch-create  experiment
  4. 2024-06-23 10:15:00  save  on main

To undo the most recent operation: drift undo
```

**记录的操作类型：**
- `save` — 保存版本
- `switch` — 切换分支（HEAD 变更）
- `branch-delete` — 删除分支
- `branch-rename` — 重命名分支
- `restore` — 恢复工作区
- `tag-add` / `tag-delete` — 标签增删

### `drift undo` ✅

撤销最近的操作。

```bash
drift undo                    # 撤销最近 1 次操作
drift undo -n 3               # 撤销最近 3 次操作
```

**参数：**

| 参数 | 说明 |
|------|------|
| `-n` / `--number` | 撤销的操作数量，默认 1；必须为正整数 |

**行为：**
- 恢复被操作影响的分支引用到操作前的状态
- 例如：撤销 `save` 会移除该提交并恢复分支指向
- 撤销 `branch -d` 会恢复被删除的分支
- 撤销 `branch -m` 会恢复分支原名
- 使用 `-n` 批量撤销时，按从新到旧的顺序依次回滚；若中途某次撤销失败（如操作日志已空），会停止并报告已撤销的数量
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

查看或设置配置选项。支持项目级配置（`.drift/config.json`）和全局配置（`~/.drift/global.json`）。

```bash
drift config <key>              # 查看配置值
drift config <key> <value>      # 设置项目配置值
drift config --global <key> <value>  # 设置全局配置值
drift config --list             # 列出所有配置（含全局、项目、同步、远程）
drift config --global --list    # 仅列出全局配置
drift config --unset <key>      # 清除项目配置值
drift config --global --unset <key>  # 清除全局配置值
```

**配置层级：**

| 层级 | 文件 | 说明 |
|------|------|------|
| 全局 | `~/.drift/global.json` | 所有项目共享，存储默认用户身份和远程同步根配置 |
| 项目 | `.drift/config.json` | 当前项目专用，覆盖全局配置 |

> **说明**：`user.name` 和 `user.email` 优先使用项目配置，若项目未设置则回退到全局配置。`drift init` 时输入的用户信息默认保存到全局配置，所有项目共享。

**支持的配置项：**

| 配置键 | 作用域 | 说明 | 默认值 |
|--------|--------|------|--------|
| `user.name` | 全局/项目 | 用户姓名（用于版本记录） | 空 |
| `user.email` | 全局/项目 | 用户邮箱（用于版本记录） | 空 |
| `core.default_branch` | 项目 | 默认分支名称 | `main` |
| `core.autocrlf` | 项目 | CRLF 换行符归一化策略 | `""`（不做转换） |
| `sync.enabled` | 项目 | 是否启用自动同步 | `false` |
| `sync.auto_after_save` | 项目 | 保存后是否自动触发同步 | `true` |
| `remote.protocol` | 全局 | 远程协议（local/webdav/ftp/sftp/smb） | 空 |
| `remote.host` | 全局 | 远程主机地址 | 空 |
| `remote.port` | 全局 | 远程主机端口 | 空 |
| `remote.path` | 全局 | 远程根路径 | 空 |
| `remote.username` | 全局 | 远程登录用户名 | 空 |
| `remote.tls` | 全局 | 是否启用 TLS | `false` |

> **注意**：`remote.*` 配置项通常通过 `drift sync remote` 命令设置，直接用 `drift config` 设置时会提示使用 `drift sync remote`。

**示例：**

```bash
# 用户身份（推荐在 drift init 时设置到全局）
drift config --global user.name "张三"
drift config --global user.email "zhangsan@example.com"
drift config user.name "项目专用名"     # 项目级覆盖全局
drift config user.name                  # 查看当前用户名（项目优先，回退全局）

# 列出配置
drift config --list                     # 列出所有配置（全局+项目+同步+远程）
drift config --global --list            # 仅列出全局配置

# 清除配置
drift config --unset user.email         # 清除项目级邮箱（回退到全局）
drift config --global --unset user.email  # 清除全局邮箱

# 核心配置
drift config core.default_branch dev    # 设置默认分支为 dev
drift config core.autocrlf true         # 开启 CRLF 归一化
drift config core.autocrlf input        # 仅 add 时 CRLF→LF

# 同步配置
drift config sync.enabled true          # 启用同步
drift config sync.enabled               # 查看同步是否启用
drift config --unset sync.enabled       # 关闭同步
```

---

## 远程同步命令

### `drift sync` ✅

管理远程同步，支持五种协议：本地路径（NAS 挂载、网盘同步文件夹）、WebDAV（Nextcloud、ownCloud、群晖 NAS、坚果云等）、FTP/FTPS、SFTP（SSH 文件传输）、SMB/CIFS（Windows 共享、NAS）。

#### `drift sync remote` — 配置远程根路径

支持两种配置方式：**显式协议模式**（推荐，字段统一清晰）和**简写自动检测模式**（兼容旧用法）。

```bash
# ===== 显式协议模式（推荐）=====

# 本地路径（NAS 挂载、网盘文件夹）
drift sync remote --protocol local --path /mnt/nas

# WebDAV 服务器
drift sync remote --protocol webdav --host cloud.example.com --path /dav \
  --tls --user alice --pass secret

# FTP/FTPS 服务器
drift sync remote --protocol ftp --host nas.local --path /backups \
  --user alice --pass secret
drift sync remote --protocol ftp --host nas.local --tls --insecure   # FTPS, 自签证书

# SFTP 服务器（密码或密钥认证）
drift sync remote --protocol sftp --host nas.local --path /backups --user alice
drift sync remote --protocol sftp --host nas.local --user alice --key-path ~/.ssh/id_rsa

# SMB/CIFS 共享（Windows 共享、NAS）
drift sync remote --protocol smb --host nas.local --share photos --user alice

# ===== 简写自动检测模式（兼容）=====

# 本地路径
drift sync remote /mnt/nas

# WebDAV URL
drift sync remote https://cloud.example.com/dav --user alice --pass secret

# ===== 管理远程配置 =====

drift sync remote --show    # 查看当前远程
drift sync remote --unset   # 移除远程配置
```

**参数：**

| 参数 | 说明 |
|------|------|
| `--protocol` | 协议类型：`local` / `webdav` / `ftp` / `sftp` / `smb` |
| `--host` | 远程服务器主机名或 IP（网络协议必填） |
| `--port` | 远程服务器端口（0 = 协议默认值） |
| `--path` | 远程基础路径（网络协议）或本地文件系统路径（local 协议，必填） |
| `--user` | 认证用户名（可选，未提供则交互式输入） |
| `--pass` | 认证密码（可选，未提供则交互式输入） |
| `--tls` | 启用 TLS（FTPS、HTTPS） |
| `--insecure` | 跳过 TLS 证书验证（自签证书场景） |
| `--share` | SMB 共享名（smb 协议必填） |
| `--key-path` | SFTP 私钥路径（密钥认证，提供后可免输密码） |
| `--show` | 显示当前远程配置 |
| `--unset` | 清除远程配置 |

**协议默认端口：**

| 协议 | 默认端口 | TLS 支持 |
|------|---------|---------|
| `local` | — | — |
| `webdav` | 80 (http) / 443 (https) | `--tls` |
| `ftp` | 21 | `--tls`（FTPS） |
| `sftp` | 22 | 内置 SSH 加密 |
| `smb` | 445 | — |

**行为：**
- 远程根路径是全局配置，存在 `~/.drift/global.json`，所有项目共享
- 本地路径模式：验证目录存在，远程项目以子目录形式存储（可直接浏览文件）
- 网络协议模式：项目以子目录形式存储在远程服务器上
- 凭据通过参数或交互式输入提供；SFTP 支持密钥认证（`--key-path`）
- SFTP 使用 TOFU（Trust On First Use）主机密钥验证，记录在 `~/.drift/known_hosts`
- WebDAV/FTP 的 `--insecure` 标志用于自签证书场景

#### `drift sync enable` — 为当前项目启用同步

```bash
drift sync enable
```

**行为：**
- 使用当前目录名作为远程项目名
- 生成项目唯一 ID（UUID），存在 `.drift/config.json`
- 本地远程模式会自动创建远程项目目录
- 启用后，`drift save` 会自动触发同步

#### `drift sync disable` — 禁用同步

```bash
drift sync disable
```

#### `drift sync status` — 查看同步状态

```bash
drift sync status
```

**输出示例：**

```
Project:  my-novel
Remote:   /mnt/nas/my-novel
Protocol: local
Enabled:  yes
Last sync: 2026-06-24T10:30:00Z
```

#### `drift sync now` — 立即同步

```bash
drift sync now
```

**行为：**
- 双向同步：推送本地变更，拉取远程变更
- 增量同步：基于文件内容哈希（SHA-256），只传输变更的文件
- 删除追踪：通过远程 manifest 文件记录文件状态，正确同步删除操作
- 跳过 `.drift/lock` 和 `.drift/sync/` 目录（机器本地文件）
- 冲突策略：本地版本优先（"最后保存胜出"），远程版本被覆盖

**输出示例：**

```
Syncing to /mnt/nas/my-novel...
  Pushed 2 file(s):
    章节/第三章.txt
    .drift/refs/main
  Pulled 1 file(s):
    章节/第四章.txt
Sync complete
```

### `drift clone` ✅

从远程克隆项目到本地。

```bash
drift clone <项目名>              # 克隆到 ./<项目名>
drift clone <项目名> <目标目录>    # 克隆到指定目录
```

**示例：**

```bash
# 首次配置远程根路径（全局，一次性）
drift sync remote /mnt/nas

# 在另一台设备上克隆项目
drift clone my-novel

# 克隆到自定义目录名
drift clone my-novel my-book
```

**行为：**
- 完整复制项目：包括 `.drift/`（版本历史）和工作区文件
- 克隆后立即可用：`cd my-novel && drift status`
- 目标目录必须不存在或为空
- 本地文件夹名可随意修改，不影响同步（项目通过 UUID 识别）

**同步架构说明：**

| 组件 | 说明 |
|------|------|
| **Transport 接口** | 抽象传输层，统一 `Get`/`Put`/`Stat`/`Delete`/`List`/`Close` 接口 |
| **LocalTransport** | 本地文件系统传输（NAS 挂载、网盘文件夹） |
| **WebDAVTransport** | WebDAV 协议传输（Nextcloud、ownCloud、群晖、坚果云等） |
| **FTPTransport** | FTP/FTPS 协议传输 |
| **SFTPTransport** | SFTP 协议传输（SSH 文件传输，支持密码/密钥认证） |
| **SMBTransport** | SMB/CIFS 协议传输（Windows 共享、NAS） |
| **Engine** | 同步引擎，处理增量同步、删除追踪和冲突检测 |
| **Manifest** | 远程清单文件（`.drift/sync/manifest.json`），记录文件状态 |
| **项目 ID** | UUID，在 `drift init` 时生成，用于跨设备识别同一项目 |

**自动同步：** 启用同步后，每次 `drift save` 会自动触发后台同步。如果同步失败（如 NAS 未连接），会重试 2 次后显示警告，但不影响 save 操作，下次 save 时自动重试。

---

## 帮助

```bash
drift --help
drift <命令> --help
```

### `drift version` ✅

显示 drift 版本号。

```bash
drift version
```

**输出示例：**

```
drift dev
```

> 版本号通过构建时 ldflags 注入（如 `0.1.0`）；开发环境（`go run` / `go test`）显示为 `dev`。

---

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
