# Changelog

## v1.0.0 (2026-06-26)

### New Features

- **Object-level sync engine** — push/pull/clone now operate on the commit
  DAG instead of scanning every file on disk. Incremental bound is a tracking
  ref; no manifest file needed. Supports WebDAV, FTP, SFTP, and SMB.
  *(Sync and clone commands are hidden in this release pending integration
  testing.)*
- `drift config` (bare), `drift tag` (bare), and `drift branch` (bare) all
  list content by default. The `--list` flag has been removed.
- `drift config` output is now section-grouped (`[core]`, `[sync]`, `[remote]`,
  `[user]`) and displays all keys including their defaults.
- `drift init` skips the interactive prompt when global `user.name` and
  `user.email` are already configured.
- Email format validation on `drift init` — re-prompts on invalid input.
- `drift log` oneline mode shows a table with VERSION / MESSAGE / TAG columns.
  Multiple tags on a commit are comma-joined.
- `drift branch` list is now sorted by last commit time (was alphabetical).

### Improvements

- `drift reflog` redesigned: header row, wider OP column, description truncated
  by width, parenthesised-message-first truncation for readability.
- `drift config remote` replaces `drift sync remote`. Use `drift config remote
  --protocol <type> ...` to set up a remote. `drift sync` now only manages
  enable / disable / status / now.
- `drift wip` subcommands: `list`, `save`, `restore`, `drop`.
- `drift unstage` with no arguments now clears the entire staging area.

### Bug Fixes

- Pull now restores the working directory after downloading objects.
- Clone now saves an index file so `drift status` works immediately.
- Branch list check now correctly handles explicit `drift branch list`
  before create.
- `drift diff <tag1> <tag2>` now works correctly with tag-based comparison.

---

# 更新日志

## v1.0.0 (2026-06-26)

### 新功能

- **对象级同步引擎** — push/pull/clone 改为在 commit DAG 上操作，不再扫描
  全部文件。增量边界为 tracking ref，无需 manifest 文件。支持 WebDAV、FTP、
  SFTP、SMB 四种协议。（*同步和克隆命令在本版本中隐藏，待集成测试后启用。*）
- `drift config` / `drift tag` / `drift branch` 裸命令默认列出内容，`--list`
  flag 已移除。
- `drift config` 输出按组显示（`[core]` / `[sync]` / `[remote]` / `[user]`），
  展示所有配置项及其默认值。
- `drift init` 在全局 `user.name` 和 `user.email` 已配置时跳过交互式输入。
- `drift init` 增加邮箱格式校验，输入不合法则循环重试。
- `drift log` oneline 模式显示 VERSION / MESSAGE / TAG 三列表格，commit 对应
  的多个 tag 以逗号拼接。
- `drift branch` 列表改为按最后提交时间排序（原为字母序）。

### 改进

- `drift reflog` 重新设计：增加表头、OP 列加宽、描述按宽度截断、括号内
  message 优先截断以保持结构可读。
- `drift config remote` 取代 `drift sync remote`。使用 `drift config remote
  --protocol <类型> ...` 配置远程地址。`drift sync` 仅管理 enable / disable /
  status / now。
- `drift wip` 子命令：`list` / `save` / `restore` / `drop`。
- `drift unstage` 无参数时清空整个暂存区。

### Bug 修复

- Pull 下载对象后恢复工作目录。
- Clone 下载后保存索引文件，`drift status` 立即可用。
- `drift branch list` 正确处理显式 `list` 位置参数。
- `drift diff <tag1> <tag2>` 支持标签间对比。
