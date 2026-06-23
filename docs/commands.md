# Drift - CLI 命令参考

## 设计原则

| 原则 | 说明 |
|------|------|
| **简洁性** | 比 Git 更少的命令 |
| **直观性** | 命令名即功能（save / list / export / restore） |
| **非技术友好** | 面向创意工作者，学习成本低 |

## 命令状态

| 标记 | 含义 |
|------|------|
| ✅ 已实现 | 功能完整可用 |
| 🔧 计划中 | 尚未实现 |

---

## 初始化命令

### `drift init` ✅

初始化一个新的 Drift 项目。

```bash
drift init
```

**行为：**
- 在当前目录创建 `.drift/` 文件夹
- 初始化存储结构
- 创建默认配置文件

---

## 暂存区命令

### `drift add` ✅

添加文件到暂存区。

```bash
drift add <路径>
drift add .        # 添加所有文件
drift add 章节/    # 添加整个目录
```

**行为：**
- 计算文件 SHA-256 哈希
- 将文件内容存入 `objects/blobs/`
- 更新暂存区（index）

### `drift status` ✅

查看工作区状态。

```bash
drift status
```

**输出示例：**

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

**状态标识：**
- `A` — Added（新增）
- `M` — Modified（修改）
- `D` — Deleted（删除）
- `?` — Untracked（未跟踪）

### `drift unstage` ✅

清空暂存区。

```bash
drift unstage
```

**行为：** 清空暂存区内容，不影响工作区文件。

---

## 版本命令

### `drift save` ✅

保存暂存区为新版本。

```bash
drift save                    # 无备注
drift save -m "备注信息"       # 带备注
```

`-m` / `--message` 可选。

**行为：**
- 从暂存区构建 Tree 对象
- 创建 Commit 对象（每个分支独立递增：main 的 v1, v2...；feature 的 v1, v2...）
- 更新当前分支引用并清空暂存区
- 若文件内容与上一版本完全相同，拒绝保存
- 保存后列出本次所有保存的文件

### `drift list` ✅

查看版本历史（最新在前）。

```bash
drift list              # 所有分支的版本历史
drift list <分支名>      # 仅查看指定分支的历史
```

**输出示例：**

```
Version history:

  v2  [main]     2024-06-15 10:30  完成前四章
  v1  [main]     2024-06-15 09:00  修改配色
  v1  [feature]  2024-06-14 15:00  方案A初稿
```

**格式说明：**
- `v1` — 版本号（每个分支独立递增）
- `[main]` — 提交所属分支
- `2024-06-15 10:30` — 提交时间
- `完成前四章` — 备注信息

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
- `<版本>` — 版本 ID（如 v1、v2）或分支名（如 main、feature）
- `-o` / `--output` — **必填**，输出路径
- `-f` / `--format` — 可选，`dir`（默认）/ `zip` / `tar`

### `drift restore` ✅

恢复工作区到指定版本。

```bash
drift restore <版本>
drift restore <版本> --force    # 强制恢复（丢弃暂存区改动）
drift restore main              # 恢复到分支最新版本
```

**行为：**
- 将工作区文件恢复到目标版本状态
- **不改变分支引用**（只改变工作树内容）
- 暂存区与当前版本不同时需 `--force`
- 未跟踪文件不受影响（保留）

> **注意**：`restore` 只改变工作树内容，分支引用保持不变。如果需要改变分支引用（类似 Git 的 reset），目前需要手动操作。

---

## 分支命令 ✅

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

### `drift switch` ✅

切换到指定分支。

```bash
drift switch <名称>
drift switch <名称> --force     # 强制切换（丢弃暂存区改动）
drift switch <名称> --create    # 分支不存在时自动创建并切换
drift switch <名称> -c          # --create 简写
```

**行为：**
- 将工作区切换到目标分支的最新版本
- 删除目标分支不存在的文件
- 暂存区非空时需 `--force`
- `--create`/`-c`：分支不存在时从当前分支创建，已存在时报错

> **设计原则**：分支是独立版本线，不做 merge。作家用分支写不同剧情线，设计师用分支试不同配色方案。

---

## 对比命令 ✅

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
drift diff v1 v2 --file 章节/第一章.txt    # 只看指定文件
drift diff v1 v2 --file 章节/ --file 笔记/  # 多个文件/目录
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
| `-o` / `--output <文件>` | 输出到文件而非命令行 |

**版本标识符格式：**

| 格式 | 示例 | 说明 |
|------|------|------|
| 版本 ID | `v1` | 当前分支的版本（如有歧义会提示） |
| 分支名 | `main` | 该分支的最新版本 |
| 分支/版本 | `main/v1` | 精确指定某分支的某版本 |

---

## 错误处理

| 错误信息 | 原因 | 解决方案 |
|----------|------|----------|
| `not a drift project (run 'drift init')` | 未运行 `drift init` | 运行 `drift init` |
| `file not found` | 路径错误 | 检查文件路径 |
| `nothing to save (use 'drift add' first)` | 暂存区为空 | 运行 `drift add` |
| `nothing changed since last version` | 文件内容未变化 | 修改文件后重新 `drift add` |
| `version not found` | 版本 ID 错误 | 运行 `drift list` |
| `staging area has pending changes` | 暂存区非空 | 运行 `drift unstage` 或使用 `--force` |
| `branch not found` | 分支名错误 | 运行 `drift branch list` |
| `branch "X" already exists` | 分支名重复 | 使用其他分支名或 `branch -m` 重命名 |
| `cannot delete the currently checked-out branch` | 试图删除当前分支 | 先 `drift switch` 到其他分支 |
| `could not acquire lock` | 另一个 drift 进程正在运行 | 等待或删除 `.drift/lock` |

---

## 配置命令 ✅

### `drift config` ✅

查看或设置配置选项。

```bash
drift config <key>           # 查看配置值
drift config <key> <value>   # 设置配置值
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
drift config core.default_branch dev  # 设置默认分支为 dev
drift config core.autocrlf true       # 开启 CRLF 归一化（add: CRLF→LF, checkout: LF→CRLF）
drift config core.autocrlf input      # 仅 add 时 CRLF→LF，checkout 保持 LF
```

---

## 日志命令 ✅

### `drift log` ✅

查看提交历史详情。

```bash
drift log                    # 当前分支完整历史
drift log <分支名>            # 指定分支历史
drift log --oneline          # 单行模式
drift log -n 5               # 只显示最近 5 条
drift log main -n 10         # main 分支最近 10 条
```

**参数：**

| 参数 | 说明 |
|------|------|
| `<分支名>` | 可选，指定要查看的分支 |
| `--oneline` | 单行模式，简洁显示 |
| `-n` / `--number` | 限制显示的提交数量 |

**输出示例（完整模式）：**

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

**输出示例（单行模式）：**

```
v3 [main] 完成前四章
v2 [main] 修改配色方案
v1 [main] 项目初始化
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
