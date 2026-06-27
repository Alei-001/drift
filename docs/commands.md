# Drift - CLI 命令参考

## 设计原则

| 原则 | 说明 |
|------|------|
| **简洁性** | 比 Git 更少的命令 |
| **直观性** | 命令名即功能（save / history / back / export） |
| **非技术友好** | 面向创意工作者，学习成本低 |

## 命令列表

| 命令 | 说明 |
|------|------|
| `start` | 开始新项目 |
| `save` | 保存所有改动为新版本 |
| `history` | 查看版本历史 |
| `back` | 回退工作区到指定版本 |
| `diff` | 对比差异 |
| `export` | 导出版本 |
| `undo` | 撤销操作 |
| `branch` | 管理分支（create/list/switch/remove/rename） |
| `tag` | 管理标签（add/list/remove） |
| `move` | 移动/重命名文件 |
| `remove` | 删除文件 |
| `ignore` | 添加忽略规则 |
| `status` | 查看工作区状态 |
| `remote` | 远程备份配置（setup/show/remove） |
| `backup` | 远程备份操作（on/off/now/status/log） |
| `clone` | 从远程克隆项目 |
| `whoami` | 查看或设置身份 |
| `version` | 显示版本号 |

---

## 项目初始化

### `drift start`

开始一个新的 Drift 项目。

```bash
drift start
drift start --name "张三" --email "zhang@mail.com"
```

**行为：**
- 在当前目录创建 `.drift/` 文件夹和存储结构
- 创建 `main` 分支
- 若未提供 `--name`/`--email`，交互式提示输入用户身份
- 用户身份保存到**全局配置**（`~/.drift/global.json`），所有项目共享
- 显示下一步指引

---

## 版本命令

### `drift save`

保存当前工作区的所有改动为新版本。**无需先执行 `add` 命令。** 自动检测新增、修改和删除的文件。

```bash
drift save                    # 无备注
drift save -m "完成第三章"     # 带备注
drift save --tag v1           # 保存并同时设置标签
drift save -m "定稿" --tag v1 # 带备注和标签
```

**行为：**
- 自动扫描工作区所有文件，检测变更（修改、新增、删除）
- 遵从 `.driftignore` 规则，排除匹配的文件
- 文本文件自动归一化换行符（CRLF→LF），Windows/Mac 迁移无忧
- 创建 Commit 对象，版本 ID 为提交哈希前 8 位（如 `a1b2c3d4`）
- 更新当前分支引用并清空暂存区
- 若文件内容与上一版本完全相同，拒绝保存
- `--tag` 可选：保存成功后自动创建标签（标签冲突仅 warning，不影响 save）

**输出示例：**
```
Saved version a1b2c3d4: 完成第三章
  3 file(s) changed:
    M  章节/第三章.txt
    A  笔记/人物设定.txt
    D  素材/旧封面.psd
```

---

### `drift history`

查看版本历史。

```bash
drift history                    # 当前分支完整历史
drift history main               # 指定分支历史
drift history --all              # 所有分支历史（去重，按时间倒序）
drift history --brief            # 单行模式
drift history --all --brief      # 所有分支单行
drift history -n 5               # 最近 5 条
drift history --porcelain        # 机器可读格式
```

**参数：**

| 参数 | 说明 |
|------|------|
| `<分支名>` | 可选，指定要查看的分支 |
| `--all` | 显示所有分支的提交（按 hash 去重，按时间倒序） |
| `--brief` | 单行模式，简洁显示 |
| `-n` / `--number` | 限制显示的提交数量（0 = 全部） |
| `--porcelain` | 机器可读格式 |

**输出示例（完整模式）：**

```
Version:  a1b2c3d4
Tags:     v1, final
Branch:   main
Date:     2026-06-27 15:30:00
Author:   张三 <zhang@mail.com>

    完成第三章初稿
```

**输出示例（`--brief`）：**

```
VERSION     BRANCH    MESSAGE                         TAG
a1b2c3d4    main      完成第三章初稿                     v1, final
e5f6a7b8    main      补充大纲
```

列宽根据内容动态计算，中文等宽字符计为 2。

---

### `drift back`

恢复工作区到指定版本。

```bash
drift back                        # 回到当前分支最新版本（丢弃未保存改动）
drift back a1b2c3d4              # 回到指定版本
drift back v1                    # 使用标签回退
drift back main                  # 恢复到某分支最新版本
drift back a1b2c3d4 章节/        # 只恢复指定路径
drift back --force               # 强制（跳过未保存改动提示）
```

**行为：**
- 将工作区文件恢复到目标版本状态
- **不改变分支引用**（只改变工作树内容）
- 若工作区有未保存改动 → 拒绝并提示，需 `--force` 跳过
- `--force` 丢弃未保存改动，直接恢复
- 可指定路径，仅恢复匹配的文件
- 未跟踪文件不受影响

---

### `drift diff`

查看文件差异。

**默认显示摘要统计：**

```bash
drift diff                         # 工作区 vs 当前版本（摘要）
drift diff a1b2c3d4                # 工作区 vs 指定版本
drift diff a1b2c3d4 e5f6a7b8       # 两个版本对比
drift diff v1 v2                   # 两个标签对比
drift diff v1 main                 # 标签 vs 分支最新
drift diff main feature            # main vs feature
```

**摘要输出示例：**
```
Changed 2 file(s):
  M 章节/第一章.txt
  A 笔记/新笔记.txt
```

**其他参数：** `-p` 查看详细行级差异，`--file` 过滤文件，`-o` 输出到文件。

---

### `drift export`

导出指定版本到文件系统。

```bash
drift export a1b2c3d4 --to ./交付客户
drift export a1b2c3d4 --to ./交付.zip --format zip
drift export v1 --to ./draft 章节/
```

**参数：**
- `<版本>` — 版本 ID、分支名或标签
- `-o` / `--output` — **必填**，输出路径
- `-f` / `--format` — 可选，`dir`（默认）/ `zip` / `tar`
- `<路径>...` — 可选，指定要导出的文件或目录

---

### `drift undo`

撤销最近的操作。

```bash
drift undo                    # 撤销最近 1 次操作
drift undo -n 3               # 撤销最近 3 次操作
```

**可撤销的操作：** `save`（移除提交）、`branch remove`（恢复分支）、`branch rename`（恢复原名）、`tag add`（移除标签）、`tag remove`（恢复标签）。

---

## 分支命令

### `drift branch`

所有分支操作统一在 `branch` 命令下。

```bash
drift branch create <名称>     # 从当前版本创建分支并切换过去
drift branch list              # 列出所有分支
drift branch switch <名称>     # 切换到指定分支
drift branch remove <名称>     # 删除分支
drift branch rename <旧名> <新名> # 重命名分支
```

**输出示例：**
```
* main
  暗黑结局
  方案B
```

**创建：** 从当前最新版本创建新分支，HEAD 自动切换到新分支。若有未保存改动，自动暂存。

**删除约束：**
- 不能删除 HEAD
- 不能删除当前所在分支（需先切换到其他分支）

**重命名约束：**
- 不能重命名 HEAD
- 目标名称不能与已有分支重名

---

## 标签命令

### `drift tag`

标签用于给重要版本起一个易记的名称。支持中文。

```bash
drift tag add <版本> <名称>        # 给版本打标签
drift tag list                    # 列出所有标签
drift tag remove <名称>            # 删除标签
```

**示例：**
```bash
drift tag add a1b2c3d4 v1
drift tag add e5f6a7b8 final
drift tag add a1b2c3d4 定稿版本     # 支持中文
drift tag list
```

**输出示例：**
```
v1     → a1b2c3d4  完成第三章初稿
定稿版本 → f3c8a1b2  定稿版本
```

**行为：**
- 标签以 `refs/tags/<标签>.ref` 存储在 `.drift/` 中
- 同一版本可拥有多个标签
- 标签不能重复指向不同版本；先 `remove` 再重新 `add`

---

## 文件管理命令

### `drift move`

移动/重命名已跟踪的文件。

```bash
drift move <源> <目标>               # 重命名文件
drift move <文件> <目录>              # 移入目录
drift move <文件1> <文件2> <目录>     # 多个文件移入目录
```

---

### `drift remove`

删除已跟踪的文件。

```bash
drift remove <路径> [<路径>...]       # 删除一个或多个文件
drift remove --cached <路径>          # 仅停止跟踪，保留磁盘文件
drift remove -r 目录名                # 递归删除目录
drift remove --force *.tmp            # 跳过确认
drift remove --dry-run *.tmp          # 预览模式，不实际删除
```

---

### `drift ignore`

将路径添加到 `.driftignore`，该文件以后不再被 drift 自动跟踪。

```bash
drift ignore "*.psd"              # 忽略所有 PSD 文件
drift ignore "build/"             # 忽略 build 目录
drift ignore "temp/*.tmp"         # 忽略 temp 下的 tmp 文件
```

**行为：**
- 将模式追加到项目根目录的 `.driftignore` 文件
- 语法与 `.gitignore` 一致
- 模式已存在则提示

---

## 工作区状态

### `drift status`

查看工作区当前状态。

```bash
drift status                 # 人类可读
drift status --porcelain     # 机器可读
```

**输出示例：**
```
On branch main, version a1b2c3d4

Unsaved changes:
  M 章节/第三章.txt
  D 旧封面.psd

New files (not yet saved):
  新笔记.txt
```

**状态标识：**
- `A` — Added（新增）
- `M` — Modified（修改）
- `D` — Deleted（删除）
- `?` — Untracked（未跟踪）

---

## 远程备份配置

### `drift remote`

管理远程备份目标地址。支持四种协议：WebDAV（Nextcloud、群晖 NAS、坚果云等）、FTP/FTPS、SFTP、SMB/CIFS。

```bash
drift remote setup          # 交互式配置远程备份目标
drift remote show           # 显示当前远程配置
drift remote remove         # 清除远程配置
```

**`remote setup` 交互式流程：**
```
? Protocol (webdav/ftp/sftp/smb): webdav
? Host: cloud.example.com
? Port (0=default): 443
? Path: /dav/novels
? Username: zhang
? Password: ****
? TLS? (y/N): y
Remote saved: webdav://zhang@cloud.example.com/dav/novels
```

**密码存储：** 密码以 AES-256-GCM 加密存储在 `~/.drift/global.json`，加密密钥为 `~/.drift/.key`（权限 0600）。也支持 `DRIFT_REMOTE_PASSWORD` 环境变量。

**协议默认端口：**

| 协议 | 默认端口 | TLS 支持 |
|------|---------|---------|
| `webdav` | 80 (http) / 443 (https) | `--tls` |
| `ftp` | 21 | `--tls`（FTPS） |
| `sftp` | 22 | 内置 SSH 加密 |
| `smb` | 445 | — |

---

## 远程备份操作

### `drift backup`

启用、停用和触发性远程备份。需先通过 `drift remote setup` 配置远程地址。

```bash
drift backup on               # 启用自动备份（每次 save 后自动同步）
drift backup off              # 停用自动备份
drift backup now              # 立即手动备份
drift backup status           # 查看备份状态
drift backup log              # 查看备份历史
```

**`backup status` 输出示例：**
```
Auto-backup: ON
Remote:      webdav://cloud.example.com/dav/novels
Last backup: 2026-06-27 15:30:00
```

**冲突策略：** 本地版本优先（"最后保存胜出"），远程版本被覆盖。Drift 是个人备份工具，非多设备实时协作。

---

### `drift clone`

从远程备份克隆项目到本地。

```bash
drift clone my-novel                  # 克隆到 ./my-novel
drift clone my-novel --to ~/work/     # 克隆到指定目录
```

**前置条件：** 必须先通过 `drift remote setup` 配置远程地址。

**行为：**
- 完整复制项目：包括 `.drift/`（版本历史）和工作区文件
- 目标目录必须不存在或为空
- 克隆后立即可用

---

## 身份管理

### `drift whoami`

查看或设置作者身份。身份用于版本记录，保存在全局配置（所有项目共享）或项目级配置。

```bash
drift whoami                              # 显示当前身份
drift whoami set "张三" "zhang@mail.com"    # 设置全局身份
drift whoami set "李四" "li@mail.com" --local  # 设置项目级身份
```

---

## 帮助

```bash
drift --help
drift <命令> --help
```

### `drift version`

显示 drift 版本号。

```
drift dev
```

> 版本号通过构建时 ldflags 注入；开发环境显示为 `dev`。

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
| `not a drift repository (run 'drift start')` | 未运行 `drift start` | 运行 `drift start` |
| `file not found` / `path not found` | 路径错误 | 检查文件路径 |
| `nothing changed since last version` | 文件内容未变化 | 修改文件后重新 `drift save` |
| `version not found` | 版本 ID 错误 | 运行 `drift history --all` |
| `branch not found` | 分支名错误 | 运行 `drift branch list` |
| `branch "X" already exists` | 分支名重复 | 使用其他分支名或 `branch rename` |
| `cannot delete the currently checked-out branch` | 试图删除当前分支 | 先 `drift branch switch` 到其他分支 |
| `could not acquire lock` | 另一个 drift 进程正在运行 | 等待或检查 PID（见 `.drift/lock`） |
| `not a drift project` | 当前目录未初始化 | `drift start` |
| `unsafe symlink` | 符号链接指向仓库外 | 使用仓库内的路径 |
