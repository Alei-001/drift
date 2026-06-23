# Drift 测试规范文档

## 概述

本文档覆盖 Drift 所有已实现功能的测试用例。每个测试用例包含：

- **编号** — `TC-模块-序号`
- **前置条件** — 测试前需要的状态
- **操作步骤** — 具体执行的命令
- **预期输出** — 终端应显示的精确文本
- **预期行为** — 文件系统/存储应发生的变化

## 测试环境

| 项目 | 要求 |
|------|------|
| 操作系统 | Windows / macOS / Linux |
| 终端 | PowerShell / cmd / bash |
| 工作目录 | 任意可写目录（建议新建临时文件夹） |
| drift 二进制 | 已编译并加入 PATH（或使用 `go run ./cmd/drift/`） |

## 约定

- 下文中 `drift` 代表实际执行命令（`drift` 或 `go run ./cmd/drift/`）
- `{date}` 代表当前日期，格式 `YYYY-MM-DD`
- `{time}` 代表当前时间，格式 `HH:MM`
- `{hash}` 代表 64 位十六进制 SHA-256 哈希
- 输出中的空行用 `<空行>` 标注，实际输出无此标记

---

## 1. 初始化

### TC-INIT-001：首次初始化

**前置条件：** 当前目录无 `.drift/` 文件夹

**操作步骤：**
```bash
mkdir test-project && cd test-project
drift init
```

**预期输出：**
```
Drift project initialized
```

**预期行为：**
- 当前目录下创建 `.drift/` 文件夹
- `.drift/objects/blobs/` 目录存在
- `.drift/objects/trees/` 目录存在
- `.drift/commits/` 目录存在
- `.drift/refs/` 目录存在
- `.drift/index` 文件存在（空 index）

---

### TC-INIT-002：重复初始化

**前置条件：** 当前目录已有 `.drift/` 文件夹

**操作步骤：**
```bash
drift init
```

**预期输出：**
```
Drift project already exists
```

**预期行为：**
- 不报错
- 不覆盖已有数据
- 退出码为 0

---

### TC-INIT-003：未初始化时执行命令

**前置条件：** 当前目录无 `.drift/` 文件夹

**操作步骤：**
```bash
drift status
```

**预期输出：**
```
Error: not a drift project (run 'drift init')
```

**预期行为：**
- 退出码为 1
- 不创建任何文件

---

### TC-INIT-004：未初始化时查看帮助

**前置条件：** 当前目录无 `.drift/` 文件夹

**操作步骤：**
```bash
drift --help
drift add --help
drift save --help
```

**预期行为：**
- 显示帮助信息
- 不报 "not a drift project" 错误
- 退出码为 0

---

### TC-INIT-005：初始化后有未跟踪文件（无 commit）

**前置条件：** 刚初始化的项目

**操作步骤：**
```bash
echo "new" > note.txt
drift status
```

**预期输出：**
```
Untracked files:
  note.txt
```

**预期行为：**
- 无 commit 时未跟踪文件仍能正确显示

---

## 2. 暂存区命令

### TC-ADD-001：添加单个文件

**前置条件：** 已初始化项目，工作区有 `note.txt`

**操作步骤：**
```bash
echo "hello world" > note.txt
drift add note.txt
```

**预期输出：**
```
Added: note.txt
```

**预期行为：**
- `.drift/objects/blobs/{hash[:2]}/{hash[2:]}` 文件存在
- index 中包含 `note.txt` 的条目
- 工作区 `note.txt` 内容不变

---

### TC-ADD-002：添加多个文件

**前置条件：** 已初始化项目，工作区有 `a.txt` 和 `b.txt`

**操作步骤：**
```bash
echo "file a" > a.txt
echo "file b" > b.txt
drift add a.txt b.txt
```

**预期输出：**
```
Added 2 file(s)
```

**预期行为：**
- 两个 blob 对象分别存储
- index 中包含两个条目

---

### TC-ADD-003：添加目录

**前置条件：** 已初始化项目，工作区有 `chapter/` 目录含多个文件

**操作步骤：**
```bash
mkdir chapter
echo "chapter 1" > chapter/ch1.txt
echo "chapter 2" > chapter/ch2.txt
drift add chapter/
```

**预期输出：**
```
Added 2 file(s)
```

**预期行为：**
- 目录下所有文件的 blob 对象分别存储
- index 中包含 `chapter/ch1.txt` 和 `chapter/ch2.txt`

---

### TC-ADD-004：添加当前目录所有文件

**前置条件：** 已初始化项目，工作区有多个文件

**操作步骤：**
```bash
drift add .
```

**预期输出：**
```
Added N file(s)
```
（N = 工作区文件总数）

**预期行为：**
- 所有文件的 blob 对象分别存储
- index 中包含所有文件条目

---

### TC-ADD-005：添加不存在的路径

**前置条件：** 已初始化项目

**操作步骤：**
```bash
drift add nonexistent.txt
```

**预期输出：**
```
Error: path not found: nonexistent.txt
```

**预期行为：**
- 退出码为 1
- index 不变
- 无 blob 对象写入

---

### TC-ADD-006：添加已暂存的文件（修改后重新添加）

**前置条件：** 已添加 `note.txt` 并修改其内容

**操作步骤：**
```bash
echo "original" > note.txt
drift add note.txt
echo "modified" > note.txt
drift add note.txt
```

**预期输出：**
第一次：
```
Added: note.txt
```
第二次：
```
Added: note.txt
```

**预期行为：**
- index 中 `note.txt` 的 hash 更新为新内容的 hash
- 两个 blob 对象都存在（内容不同，hash 不同）

---

### TC-STATUS-001：空工作区（无文件无暂存）

**前置条件：** 刚初始化的项目，无任何文件（也无 commit）

**操作步骤：**
```bash
drift status
```

**预期输出：**
```
Nothing to commit, working tree clean
```

**预期行为：**
- 无 commit 时 status 仍正常工作
- 不报错，退出码为 0

---

### TC-STATUS-002：暂存新文件

**前置条件：** 已添加 `note.txt` 到暂存区

**操作步骤：**
```bash
drift status
```

**预期输出：**
```
Staged changes:
  A note.txt
```

---

### TC-STATUS-003：暂存修改文件

**前置条件：** 已有版本 v1 包含 `note.txt`，修改后重新添加

**操作步骤：**
```bash
echo "modified" > note.txt
drift add note.txt
drift status
```

**预期输出：**
```
Staged changes:
  M note.txt
```

---

### TC-STATUS-004：暂存删除文件

**前置条件：** 已有版本 v1 包含 `note.txt`，文件已从工作区删除并暂存删除

**操作步骤：**
```bash
del note.txt        # Windows
# rm note.txt       # macOS/Linux
drift add note.txt
drift status
```

**预期输出：**
```
Staged changes:
  D note.txt
```

---

### TC-STATUS-005：未暂存的修改

**前置条件：** 已有版本 v1 包含 `note.txt`，修改了文件但未暂存

**操作步骤：**
```bash
echo "modified" > note.txt
drift status
```

**预期输出：**
```
Unstaged changes:
  M note.txt
```

---

### TC-STATUS-006：未跟踪的文件

**前置条件：** 已初始化项目，新建文件未添加

**操作步骤：**
```bash
echo "new file" > new.txt
drift status
```

**预期输出：**
```
Untracked files:
  new.txt
```

---

### TC-STATUS-007：混合状态

**前置条件：** 已有版本 v1 包含 `old.txt`，暂存了 `staged.txt`，修改了 `old.txt` 未暂存，新建了 `untracked.txt`

**操作步骤：**
```bash
drift status
```

**预期输出：**
```
Staged changes:
  A staged.txt

Unstaged changes:
  M old.txt

Untracked files:
  untracked.txt
```

---

### TC-RESET-001：清空暂存区

> **注意：** 该命令已重命名为 `drift unstage`，原 `drift reset` 不再存在。

**前置条件：** 暂存区有文件

**操作步骤：**
```bash
drift reset
```

**预期输出：**
```
Staging area cleared
```

**预期行为：**
- index 被清空
- blob 对象保留（不删除）
- 工作区文件不受影响

---

### TC-RESET-002：空暂存区执行 reset

**前置条件：** 暂存区已为空

**操作步骤：**
```bash
drift reset
```

**预期输出：**
```
Staging area cleared
```

**预期行为：**
- 不报错
- 退出码为 0

---

## 3. 版本命令

### TC-SAVE-001：无备注保存

**前置条件：** 暂存区有文件

**操作步骤：**
```bash
echo "content" > note.txt
drift add note.txt
drift save
```

**预期输出：**
```
Saved version v1
```

**预期行为：**
- `.drift/commits/` 下生成 commit 对象（DCMT 格式）
- `.drift/objects/trees/` 下生成 tree 对象（DREE 格式）
- `.drift/refs/main.json` 更新为最新 commit hash
- 暂存区被清空
- commit 的 message 字段为空字符串

---

### TC-SAVE-002：带备注保存

**前置条件：** 暂存区有文件

**操作步骤：**
```bash
echo "chapter 1" > ch1.txt
drift add ch1.txt
drift save -m "first chapter"
```

**预期输出：**
```
Saved version v1: first chapter
```

**预期行为：**
- commit 的 message 字段为 `"first chapter"`

---

### TC-SAVE-003：空暂存区保存

**前置条件：** 暂存区为空

**操作步骤：**
```bash
drift save
```

**预期输出：**
```
Error: nothing to save (use 'drift add' first)
```

**预期行为：**
- 退出码为 1
- 无 commit 对象生成

---

### TC-SAVE-004：版本号自动递增

**前置条件：** 无

**操作步骤：**
```bash
echo "v1" > f1.txt && drift add f1.txt && drift save -m "v1"
echo "v2" > f2.txt && drift add f2.txt && drift save -m "v2"
echo "v3" > f3.txt && drift add f3.txt && drift save -m "v3"
```

**预期输出：**
```
Saved version v1: v1
Saved version v2: v2
Saved version v3: v3
```

**预期行为：**
- commit ID 依次为 v1, v2, v3
- 每个 commit 的 parent 指向前一个

---

### TC-SAVE-005：暂存区清空后 status 显示 clean

**前置条件：** 已保存一个版本

**操作步骤：**
```bash
drift status
```

**预期输出：**
```
Nothing to commit, working tree clean
```

---

### TC-LOG-001：查看版本历史（--all）

**前置条件：** 已保存 3 个版本

**操作步骤：**
```bash
drift log --all
```

**预期输出：**
```
Version history:

  v3  [main]  v3
  v2  [main]  v2
  v1  [main]  v1
```

**预期行为：**
- 最新版本在前
- 每行格式：`  {ID}  [{branch}]  {message}`

---

### TC-LOG-002：无版本时查看历史（--all）

**前置条件：** 刚初始化，无任何版本

**操作步骤：**
```bash
drift log --all
```

**预期输出：**
```
No versions yet
```

---

### TC-LOG-003：无备注的版本显示（--all）

**前置条件：** 已保存一个无备注的版本

**操作步骤：**
```bash
drift log --all
```

**预期输出：**
```
Version history:

  v1  [main]
```

**预期行为：**
- 无备注时不显示 message 部分

---

### TC-EXPORT-001：导出到目录

**前置条件：** 已保存版本 v1 包含 `note.txt`

**操作步骤：**
```bash
drift export v1 -o ./output
```

**预期输出：**
```
Exported 1 file(s) to ./output
```

**预期行为：**
- `./output/note.txt` 存在且内容与原文件一致
- `./output/` 下无 `.drift/` 目录

---

### TC-EXPORT-002：导出为 zip

**前置条件：** 已保存版本 v1

**操作步骤：**
```bash
drift export v1 -o ./output.zip -f zip
```

**预期输出：**
```
Exported N file(s) to ./output.zip
```

**预期行为：**
- `./output.zip` 文件存在
- 解压后文件内容与原文件一致

---

### TC-EXPORT-003：导出为 tar.gz

**前置条件：** 已保存版本 v1

**操作步骤：**
```bash
drift export v1 -o ./output.tar.gz -f tar
```

**预期输出：**
```
Exported N file(s) to ./output.tar.gz
```

**预期行为：**
- `./output.tar.gz` 文件存在
- 解压后文件内容与原文件一致

---

### TC-EXPORT-004：缺少 -o 参数

**前置条件：** 已保存版本 v1

**操作步骤：**
```bash
drift export v1
```

**预期输出：**
```
Error: output path is required (use -o flag)
```

**预期行为：**
- 退出码为 1

---

### TC-EXPORT-005：版本不存在

**前置条件：** 已初始化项目

**操作步骤：**
```bash
drift export v99 -o ./output
```

**预期输出：**
```
Error: version not found: v99
```

**预期行为：**
- 退出码为 1
- 不创建输出目录

---

### TC-EXPORT-006：输出目录已存在

**前置条件：** 已保存版本 v1，`./output` 目录已存在

**操作步骤：**
```bash
mkdir output
drift export v1 -o ./output
```

**预期输出：**
```
Error: directory already exists: ./output
```

**预期行为：**
- 退出码为 1
- 不覆盖已有目录

---

### TC-RESTORE-001：回退到指定版本

**前置条件：** 已保存 v1（含 `a.txt`）和 v2（含 `a.txt` 修改版 + `b.txt`）

**操作步骤：**
```bash
drift restore v1
```

**预期输出：**
```
Restored to v1: 0 added, 1 modified, 1 deleted
```

（具体数字取决于实际差异）

**预期行为：**
- 工作区 `a.txt` 恢复到 v1 的内容
- 工作区 `b.txt` 被删除（v1 中不存在）
- 不创建新版本

---

### TC-RESTORE-002：暂存区非空时回退（无 --force）

**前置条件：** 暂存区有文件

**操作步骤：**
```bash
drift restore v1
```

**预期输出：**
```
Error: staging area has pending changes (use --force to discard)
```

**预期行为：**
- 退出码为 1
- 工作区不变

---

### TC-RESTORE-003：暂存区非空时强制回退

**前置条件：** 暂存区有文件

**操作步骤：**
```bash
drift restore v1 --force
```

**预期输出：**
```
Restored to v1: N added, N modified, N deleted
```

**预期行为：**
- 工作区恢复到 v1 状态
- 暂存区被清空

---

### TC-RESTORE-004：保留未跟踪文件

**前置条件：** 已保存 v1（含 `a.txt`），工作区有未跟踪的 `untracked.txt`

**操作步骤：**
```bash
echo "keep me" > untracked.txt
drift restore v1
```

**预期输出：**
```
Restored to v1: 0 added, 0 modified, 0 deleted
```

**预期行为：**
- `untracked.txt` 保留不变
- `a.txt` 保持 v1 状态

---

## 4. 分支命令

### TC-BRANCH-001：创建分支

**前置条件：** 已保存版本 v1

**操作步骤：**
```bash
drift branch experiment
```

**预期输出：**
```
Created branch: experiment
```

**预期行为：**
- `.drift/refs/experiment.json` 存在
- 内容与 `main.json` 指向同一 commit hash
- HEAD 仍指向 main（不自动切换）

---

### TC-BRANCH-002：查看分支列表

**前置条件：** 已创建 `experiment` 分支

**操作步骤：**
```bash
drift branch list
```

**预期输出：**
```
* main
  experiment
```

**预期行为：**
- 当前分支前有 `*` 标记
- 每行一个分支

---

### TC-BRANCH-003：无分支时查看列表

**前置条件：** 刚初始化（只有 main）

**操作步骤：**
```bash
drift branch list
```

**预期输出：**
```
* main
```

---

### TC-SWITCH-001：切换到已有分支

**前置条件：** 已创建 `experiment` 分支，当前在 main

**操作步骤：**
```bash
drift switch experiment
```

**预期输出：**
```
Switched to branch: experiment
```

**预期行为：**
- HEAD 更新为指向 experiment
- 工作区文件更新为 experiment 分支的最新版本

---

### TC-SWITCH-002：切换分支时删除目标分支不存在的文件

**前置条件：** main 有 `main-only.txt`，experiment 没有该文件

**操作步骤：**
```bash
drift switch experiment
```

**预期输出：**
```
Switched to branch: experiment
```

**预期行为：**
- `main-only.txt` 从工作区删除

---

### TC-SWITCH-003：切换到不存在的分支

**前置条件：** 已初始化项目

**操作步骤：**
```bash
drift switch nonexistent
```

**预期输出：**
```
Error: branch not found: nonexistent
```

**预期行为：**
- 退出码为 1
- 工作区不变

---

### TC-SWITCH-004：暂存区非空时切换（无 --force）

**前置条件：** 暂存区有文件

**操作步骤：**
```bash
drift switch experiment
```

**预期输出：**
```
Error: staging area has pending changes (use --force to discard)
```

**预期行为：**
- 退出码为 1
- 工作区不变

---

### TC-SWITCH-005：暂存区非空时强制切换

**前置条件：** 暂存区有文件

**操作步骤：**
```bash
drift switch experiment --force
```

**预期输出：**
```
Switched to branch: experiment
```

**预期行为：**
- 暂存区被清空
- 工作区更新为 experiment 的内容

---

### TC-SWITCH-006：分支独立版本线

**前置条件：** main 有 v1，在 experiment 上保存 v2，main 上保存 v3

**操作步骤：**
```bash
drift branch experiment
# 在 experiment 上工作
drift switch experiment
echo "exp work" > exp.txt
drift add exp.txt
drift save -m "experiment work"
# 回到 main
drift switch main
echo "main work" > main.txt
drift add main.txt
drift save -m "main work"
# 查看各分支
drift log
drift switch experiment
drift log
```

**预期行为：**
- main 上 `drift log` 显示 v1, v3
- experiment 上 `drift log` 显示 v1, v2
- 两个分支的版本线独立

---

### TC-SWITCH-007：使用 --create 创建并切换分支

**前置条件：** 已初始化项目，已保存 v1

**操作步骤：**
```bash
drift switch newbranch --create
```

**预期输出：**
```
Created branch: newbranch
Switched to branch: newbranch
```

**预期行为：**
- HEAD 指向 newbranch
- newbranch 与之前的 main 指向同一 commit

---

### TC-BRANCH-004：删除分支

**前置条件：** 已创建 `experiment` 分支，当前不在该分支上

**操作步骤：**
```bash
drift branch -d experiment
```

**预期输出：**
```
Deleted branch: experiment
```

**预期行为：**
- `.drift/refs/experiment.json` 被删除
- 当前分支不受影响

---

### TC-BRANCH-005：删除当前分支（应失败）

**前置条件：** 当前在 `main` 分支

**操作步骤：**
```bash
drift branch -d main
```

**预期输出：**
```
Error: cannot delete the currently checked-out branch "main" (switch to another branch first)
```

---

### TC-BRANCH-006：重命名分支

**前置条件：** 已创建 `old` 分支

**操作步骤：**
```bash
drift branch -m new old
```

**预期输出：**
```
Renamed branch: old → new
```

**预期行为：**
- `old` 分支引用被删除
- `new` 分支引用存在，指向原 `old` 的 commit
- 若当前在 `old` 上，HEAD 自动更新为 `new`

---

## 5. 日志命令

### TC-LOG-001：查看完整日志

**前置条件：** 已保存两个版本

**操作步骤：**
```bash
drift log
```

**预期输出：**
```
commit {hash}
Version: v2
Branch:  main
Date:    {date} {time}
Author:  {name} <{email}>

    second commit

commit {hash}
Version: v1
Branch:  main
Date:    {date} {time}

    first commit
```

---

### TC-LOG-002：单行模式

**前置条件：** 已保存版本

**操作步骤：**
```bash
drift log --oneline
```

**预期输出：**
```
v2 [main] second commit
v1 [main] first commit
```

---

### TC-LOG-003：限制数量

**前置条件：** 已保存 5 个版本

**操作步骤：**
```bash
drift log -n 3
```

**预期输出：**
显示最近 3 条记录

---

### TC-LOG-004：查看指定分支日志

**前置条件：** 有 `main` 和 `feature` 两个分支各有提交

**操作步骤：**
```bash
drift log feature
```

**预期输出：**
仅显示 `feature` 分支的提交历史

---

## 6. 对比命令

### TC-DIFF-001：工作区 vs 最新版本（无差异）

**前置条件：** 已保存 v1，工作区文件未修改

**操作步骤：**
```bash
drift diff
```

**预期输出：**
```
No differences
```

---

### TC-DIFF-002：工作区 vs 最新版本（文本差异）

**前置条件：** 已保存 v1 包含 3 行文本，修改了第 2 行

**操作步骤：**
```bash
# v1 内容：
# line 1
# line 2
# line 3
# 修改后：
# line 1
# line two
# line 3
drift diff
```

**预期输出：**
```
--- note.txt
+++ note.txt
 line 1
-line 2
+line two
 line 3
```

**预期行为：**
- 使用 LCS 算法，只显示变更行及其上下文
- 不变行前缀空格，删除行前缀 `-`，新增行前缀 `+`

---

### TC-DIFF-003：工作区 vs 最新版本（新增文件）

**前置条件：** 已保存 v1，工作区新增了 `new.txt`

**操作步骤：**
```bash
drift diff
```

**预期输出：**
```
--- /dev/null
+++ new.txt
(new file)

```

---

### TC-DIFF-004：工作区 vs 最新版本（删除文件）

**前置条件：** 已保存 v1 包含 `old.txt`，从工作区删除了该文件

**操作步骤：**
```bash
drift diff
```

**预期输出：**
```
--- old.txt
+++ /dev/null
(deleted)

```

---

### TC-DIFF-005：工作区 vs 指定版本

**前置条件：** 已保存 v1 和 v2

**操作步骤：**
```bash
drift diff v1
```

**预期输出：**
显示工作区与 v1 之间的差异

---

### TC-DIFF-006：两个版本之间对比

**前置条件：** 已保存 v1 和 v2

**操作步骤：**
```bash
drift diff v1 v2
```

**预期输出：**
显示 v1 与 v2 之间的差异（格式同 TC-DIFF-002）

---

### TC-DIFF-007：二进制文件差异

**前置条件：** 已保存 v1 包含二进制文件，修改了该文件

**操作步骤：**
```bash
drift diff
```

**预期输出：**
```
--- image.png
+++ image.png
Binary file changed (1234 -> 5678 bytes)

```

**预期行为：**
- 不尝试文本 diff
- 显示文件大小变化

---

### TC-DIFF-008：无版本时执行 diff

**前置条件：** 刚初始化，无任何版本

**操作步骤：**
```bash
drift diff
```

**预期输出：**
```
Error: no versions to compare against
```

**预期行为：**
- 退出码为 1

---

## 6. 配置与忽略

### TC-CONFIG-001：默认配置

**前置条件：** 无

**操作步骤：**
```bash
drift init
cat .drift/config.json
```

**预期输出：**
```json
{
  "user": {
    "name": "",
    "email": ""
  },
  "core": {
    "default_branch": "main"
  }
}
```

**预期行为：**
- `.drift/config.json` 文件存在
- 字段值为空字符串或默认值

---

### TC-CONFIG-002：查看配置值

**前置条件：** 已初始化项目

**操作步骤：**
```bash
drift config user.name
```

**预期输出：**
```
（空行或默认值）
```

---

### TC-CONFIG-003：设置配置值

**前置条件：** 已初始化项目

**操作步骤：**
```bash
drift config user.name "Test User"
drift config user.name
```

**预期输出：**
```
Test User
```

---

### TC-CONFIG-004：查看不存在的配置项

**前置条件：** 已初始化项目

**操作步骤：**
```bash
drift config unknown.key
```

**预期输出：**
```
Error: unknown config key: unknown.key (supported: user.name, user.email, core.default_branch)
```

---

### TC-IGNORE-001：.driftignore 忽略特定文件

**前置条件：** 已初始化项目，创建 `.driftignore`

**操作步骤：**
```bash
echo "*.log" > .driftignore
echo "test data" > debug.log
echo "real data" > note.txt
drift add .
drift status
```

**预期行为：**
- `debug.log` 不出现在暂存区
- `note.txt` 正常暂存

**预期输出（status）：**
```
Staged changes:
  A note.txt
```

---

### TC-IGNORE-002：.driftignore 忽略目录

**前置条件：** 已初始化项目

**操作步骤：**
```bash
echo "build/" > .driftignore
mkdir build
echo "output" > build/out.txt
echo "source" > src.txt
drift add .
```

**预期行为：**
- `build/out.txt` 不被暂存
- `src.txt` 正常暂存

---

### TC-IGNORE-003：.driftignore 使用 ** 通配符

**前置条件：** 已初始化项目

**操作步骤：**
```bash
echo "**/node_modules/**" > .driftignore
mkdir -p node_modules/pkg
echo "deps" > node_modules/pkg/index.js
echo "source" > app.js
drift add .
drift status
```

**预期行为：**
- `node_modules/pkg/index.js` 不被暂存
- `app.js` 正常暂存

**预期输出（status）：**
```
Staged changes:
  A app.js
```

**注意：** `node_modules/**` 只匹配 node_modules 下的直接子文件，不匹配子目录。要递归忽略需用 `**/node_modules/**`。

---

### TC-IGNORE-004：硬编码忽略 .drift/ 和 .git/

**前置条件：** 已初始化项目

**操作步骤：**
```bash
echo "test" > note.txt
drift add .
drift status
```

**预期行为：**
- `.drift/` 下的文件永远不出现在暂存区
- `.git/` 下的文件永远不出现在暂存区
- 不需要 `.driftignore` 配置

---

## 7. 错误处理

### TC-ERR-001：未初始化执行 add

**前置条件：** 当前目录无 `.drift/`

**操作步骤：**
```bash
drift add note.txt
```

**预期输出：**
```
Error: not a drift project (run 'drift init')
```

**预期行为：**
- 退出码为 1

---

### TC-ERR-002：未初始化执行 save

**前置条件：** 当前目录无 `.drift/`

**操作步骤：**
```bash
drift save
```

**预期输出：**
```
Error: not a drift project (run 'drift init')
```

---

### TC-ERR-003：未初始化执行 log --all

**前置条件：** 当前目录无 `.drift/`

**操作步骤：**
```bash
drift log --all
```

**预期输出：**
```
Error: not a drift project (run 'drift init')
```

---

### TC-ERR-004：版本不存在执行 restore/export/diff

**前置条件：** 已初始化项目

**操作步骤：**
```bash
drift restore v99
drift export v99 -o ./output
drift diff v99
```

**预期输出（每条命令）：**
```
Error: version not found: v99
```

**预期行为：**
- 退出码为 1
- 三个命令共用 `findCommitByPrefix` 函数，错误消息一致

## 8. 边界场景

### TC-EDGE-001：空文件

**前置条件：** 已初始化项目

**操作步骤：**
```bash
# Windows (PowerShell)
New-Item empty.txt -ItemType File
# macOS / Linux
# touch empty.txt

drift add empty.txt
drift save -m "empty file"
drift log --all
```

**预期行为：**
- 空文件可以正常暂存和保存
- blob 对象存在（0 字节内容的 hash）
- restore 后文件仍为空

---

### TC-EDGE-002：文件名含空格

**前置条件：** 已初始化项目

**操作步骤：**
```bash
echo "content" > "my file.txt"
drift add "my file.txt"
drift save -m "spaces"
drift status
```

**预期行为：**
- 文件正常暂存和保存
- status 输出中文件名正确显示

---

### TC-EDGE-003：文件名含中文

**前置条件：** 已初始化项目

**操作步骤：**
```bash
echo "content" > "笔记.txt"
drift add "笔记.txt"
drift save -m "chinese"
```

**预期行为：**
- 文件正常暂存和保存
- 所有命令输出中文件名正确显示

---

### TC-EDGE-004：深层嵌套目录

**前置条件：** 已初始化项目

**操作步骤：**
```bash
mkdir -p a/b/c/d
echo "deep" > a/b/c/d/file.txt
drift add a/
drift save -m "deep nesting"
drift export v1 -o ./output
```

**预期行为：**
- 深层目录结构正确保存
- export 后目录结构完整：`./output/a/b/c/d/file.txt`

---

### TC-EDGE-005：大量文件

**前置条件：** 已初始化项目

**操作步骤：**
```bash
# 创建 100 个文件
for i in $(seq 1 100); do echo "file $i" > "file_$i.txt"; done
drift add .
drift save -m "100 files"
drift log --all
drift export v1 -o ./output
```

**预期行为：**
- 所有文件正确暂存（显示进度）
- 保存成功
- export 显示进度

---

### TC-EDGE-006：同内容不同文件名

**前置条件：** 已初始化项目

**操作步骤：**
```bash
echo "same content" > a.txt
echo "same content" > b.txt
drift add a.txt b.txt
drift save -m "same content"
```

**预期行为：**
- 两个文件指向同一个 blob（内容寻址）
- `objects/blobs/` 下只有一个 blob 对象

---

### TC-EDGE-007：修改文件后 hash 变化

**前置条件：** 已保存 v1 包含 `note.txt`

**操作步骤：**
```bash
echo "original" > note.txt
drift add note.txt
drift save -m "v1"
# 记录 v1 的 blob hash
echo "modified" > note.txt
drift add note.txt
drift save -m "v2"
```

**预期行为：**
- v1 和 v2 的 blob hash 不同
- `objects/blobs/` 下有两个 blob 对象
- v1 的 blob 保留不被覆盖

---

## 9. 完整工作流测试

### TC-FLOW-001：典型创作工作流

**操作步骤：**
```bash
# 1. 初始化
mkdir my-novel && cd my-novel
drift init

# 2. 创建初稿
echo "第一章" > chapter1.txt
echo "第二章" > chapter2.txt
drift add .
drift save -m "初稿"

# 3. 修改第一章
echo "第一章 修改版" > chapter1.txt
drift add chapter1.txt
drift save -m "修改第一章"

# 4. 创建分支试写结局A
drift branch ending-a
drift switch ending-a
echo "结局A" > ending.txt
drift add ending.txt
drift save -m "结局A"
# ending-a: v1 → v2 → v3（v3是结局A）

# 5. 创建分支试写结局B（从 main 分出，和 ending-a 同一起点）
drift switch main
drift branch ending-b
drift switch ending-b
echo "结局B" > ending.txt
drift add ending.txt
drift save -m "结局B"
# ending-b: v1 → v2 → v4（v4是结局B）

# 6. 查看历史
drift log             # ending-b 上：显示 v1, v2, v4
drift switch ending-a
drift log             # ending-a 上：显示 v1, v2, v3
drift branch list      # 显示 * ending-a, ending-b, main

# 7. 导出给编辑
drift export v3 -o ./交付编辑 -f zip

# 8. 对比两个结局
drift diff v3 v4       # ending-a 的结局A vs ending-b 的结局B
```

**预期行为：**
- 每一步命令正常执行
- 分支线独立：ending-a 有 v3，ending-b 有 v4
- 导出的 zip 包含正确文件
- diff 显示结局差异

---

### TC-FLOW-002：配色方案对比（设计师场景）

**操作步骤：**
```bash
drift init
echo "red theme" > theme.css
echo "logo v1" > logo.png
drift add .
drift save -m "红色主题"

drift branch blue-theme
drift switch blue-theme
echo "blue theme" > theme.css
drift add theme.css
drift save -m "蓝色主题"

drift diff v1 v2
```

**预期行为：**
- diff 显示 theme.css 的文本差异
- diff 显示 logo.png 为二进制（无变化时不出现在 diff 中）

---

## 附录：测试检查清单

### 快速验收清单

| # | 功能 | 通过 |
|---|------|------|
| 1 | `drift init` 创建 .drift/ | ☐ |
| 2 | 重复 init 不报错 | ☐ |
| 3 | 未初始化时命令报错 | ☐ |
| 4 | 未初始化时 --help 不报错 | ☐ |
| 5 | 无 commit 时 status 正常 | ☐ |
| 6 | 无 commit 时 untracked 显示 | ☐ |
| 7 | `drift add` 单文件 | ☐ |
| 8 | `drift add` 目录 | ☐ |
| 9 | `drift add` 不存在路径报错 | ☐ |
| 10 | `drift status` 各状态正确 | ☐ |
| 11 | `drift reset` 清空暂存区 | ☐ |
| 12 | `drift save` 无备注 | ☐ |
| 13 | `drift save -m` 带备注 | ☐ |
| 14 | 空暂存区 save 报错 | ☐ |
| 15 | 版本号自动递增 | ☐ |
| 16 | `drift log --all` 显示历史 | ☐ |
| 17 | `drift export` 到目录 | ☐ |
| 18 | `drift export` 到 zip | ☐ |
| 19 | `drift export` 到 tar.gz | ☐ |
| 20 | `drift restore` 回退 | ☐ |
| 21 | restore 保留未跟踪文件 | ☐ |
| 22 | restore --force 强制回退 | ☐ |
| 23 | `drift branch` 创建分支 | ☐ |
| 24 | `drift branch list` 显示列表 | ☐ |
| 25 | `drift switch` 切换分支 | ☐ |
| 26 | switch 删除多余文件 | ☐ |
| 27 | switch --force 强制切换 | ☐ |
| 28 | `drift diff` 文本差异 | ☐ |
| 29 | `drift diff v1` 指定版本 | ☐ |
| 30 | `drift diff v1 v2` 两版本 | ☐ |
| 31 | diff 二进制文件显示大小 | ☐ |
| 32 | .driftignore 单文件忽略 | ☐ |
| 33 | .driftignore 目录忽略 | ☐ |
| 34 | .driftignore ** 通配符 | ☐ |
| 35 | 默认 config.json 创建 | ☐ |
| 36 | 空文件正常处理 | ☐ |
| 37 | 中文文件名正常处理 | ☐ |
| 38 | 同内容文件只存一份 blob | ☐ |

---

## 版本历史

| 版本 | 日期 | 变更 |
|------|------|------|
| 1.0 | 2026-06-22 | 初始版本 |
| 1.1 | 2026-06-22 | 修正 TC-IGNORE-003 模式为 `**/node_modules/**`；合并重复用例；补充无 commit 时 status 测试 |
| 1.2 | 2026-06-23 | `reset` → `unstage` 重命名；修正 config schema（`core.default_branch`）；新增 branch delete/rename、switch --create、log 命令测试用例 |
