# Changelog

## v1.0.0 (2026-06-26)

### Object-Level Sync Engine (Internal)

- Complete redesign from file-level rsync to object-level DAG sync.
  Push uploads objects reachable from local commits; pull downloads missing
  objects via fetchChain; clone does a full fetch. Incremental bound is a
  tracking ref (`remotes/origin/<branch>`) — no manifest.json.
- New `Transport` interface: `Get` / `Put` / `Exists` / `GetRef` / `PutRef` /
  `ListRefs` / `Close`. Remote stores `.drift/` structure directly
  (`objects/blobs/`, `objects/trees/`, `commits/`, `refs/`).
- Rewrote WebDAV, FTP, SFTP, and SMB transports to the new interface.
- Removed `local` protocol (NAS mounts go through SMB; cloud-drive folders
  double-layer with drift sync).
- New `core/dag.go` — `ReachableObjects` walks the commit DAG and collects
  all blob/tree/commit hashes for push/pull.
- Added `HasObject` and `OpenObject` to `storage.Store`.
- Pull restores the working directory after downloading objects (writes
  blobs, updates index, cleans deleted files, restores WIP).
- Clone saves an index file after downloading so `drift status` works
  immediately.
- Sync and clone commands are **disabled** in this release (hidden from
  CLI) pending integration testing with real remote servers. All sync
  code and tests remain in the codebase.

### CLI Consistency & Refactoring

- `drift config` (bare), `drift tag` (bare), and `drift branch` (bare) all
  list content by default. `--list` flag removed across all three commands.
- `drift config remote` replaces `drift sync remote`; remote configuration
  now lives under `config`. `drift sync` keeps only `enable` / `disable` /
  `status` / `now`.
- `drift config` output is section-grouped (`[core]` / `[sync]` / `[remote]`
  / `[user]`) and shows all keys including defaults.
- `drift reflog` redesigned: header row, OP column widened to 14 chars,
  description truncated at 20–60 width, parenthesised-message-first
  truncation, null-hash padding in porcelain, `-` placeholder in verbose.
- `drift log` — oneline mode shows VERSION / MESSAGE / TAG columns; all
  tags for a commit are comma-joined.
- `drift diff <tag1> <tag2>` examples added to docs.
- `drift wip` docs updated to match actual subcommand structure
  (`list` / `save` / `restore` / `drop`).

### Init & User Config

- `drift init` skips the interactive prompt when global `user.name` and
  `user.email` are already set.
- Email format validation (`xxx@xxx.xxx`); re-prompts on invalid input.

### Config Package Restructure

- `GlobalConfig` moved from `sync/` to `config/`.
- `GlobalUserConfig` merged into `UserConfig` — one struct for both
  project-level and global user identity.
- `config.go` renamed to `project.go`.
- Removed unused `SetGlobalConfigPathForTest`.

### Bug Fixes

- `collectTreeObjects` in the DAG walker now receives the tree hash as a
  parameter because `tree.Hash` is empty after Unmarshal (DREE format does
  not store self-hashes).
- Tracking ref naming consistency: `remotes/origin/heads/<branch>` used
  everywhere (Push, Fetch, Clone, fetchRef).
- `SyncNow` returns both push and pull counts.
- Branch list sorted by last commit time instead of alphabetical.
- `drift branch list` check moved before create to handle explicit
  positional `list`.
- `drift unstage` with no arguments now clears the entire staging area.

### Documentation

- `docs/commands.md` — sync and remote configuration sections merged and
  rewritten. WIP section updated. Tag diff examples added. Reflog output
  examples updated. Removed stale `sync.auto_after_save` key.
- `AGENTS.md` — build command updated to use `.exe` suffix on Windows.

### Tests

- 7 sync engine tests covering Push (empty / incremental / divergence),
  Fetch (full / up-to-date), Pull, and Clone.
- `memoryStore` mock in core tests now includes `GetCommit` (required by
  the expanded `StoreReader` interface).

---

# 更新日志

## v1.0.0 (2026-06-26)

### 对象级同步引擎（内部）

- 从文件级 rsync 完全重构为对象级 DAG 同步。Push 上传从本地 commit 可达的对象；
  Pull 通过 fetchChain 下载缺失对象；Clone 做全量拉取。增量边界使用 tracking ref
  （`remotes/origin/<branch>`），不再使用 manifest.json。
- 新 `Transport` 接口：`Get` / `Put` / `Exists` / `GetRef` / `PutRef` / `ListRefs`
  / `Close`。远端直接存储 `.drift/` 目录结构（`objects/blobs/`、`objects/trees/`、
  `commits/`、`refs/`）。
- 重写了 WebDAV、FTP、SFTP、SMB 四种传输层协议。
- 移除了 `local` 协议（NAS 挂载走 SMB；云同步目录与 drift sync 双层叠加）。
- 新增 `core/dag.go` — `ReachableObjects` 沿 commit DAG 回溯收集所有
  blob/tree/commit 哈希。
- `storage.Store` 新增 `HasObject` 和 `OpenObject`。
- Pull 下载对象后恢复工作目录（写入文件、更新索引、清理已删除文件、恢复 WIP）。
- Clone 下载完成后保存索引文件，使 `drift status` 立即可用。
- **同步和克隆命令在本版本中已禁用**（CLI 中隐藏），待远程服务器集成测试后再
  启用。所有同步代码和测试保留在代码库中。

### CLI 一致性 & 重构

- `drift config`（裸命令）、`drift tag`（裸命令）、`drift branch`（裸命令）均
  默认列出内容。三者均已移除 `--list` flag。
- `drift config remote` 取代 `drift sync remote`；远程配置归入 `config`。
  `drift sync` 仅保留 `enable` / `disable` / `status` / `now`。
- `drift config` 输出按组显示（`[core]` / `[sync]` / `[remote]` / `[user]`），
  展示所有 key 含默认值。
- `drift reflog` 重新设计：增加表头、OP 列扩至 14 字符、desc 宽度 20-60 动态
  截断、括号内 message 优先截断、porcelain 模式空字段用 null hash 占位、
  verbose 模式 `-` 占位。
- `drift log` — oneline 模式显示 VERSION / MESSAGE / TAG 三列；commit 对应
  的多个 tag 逗号拼接。
- 文档中补充了 `drift diff <tag1> <tag2>` 对比示例。
- `drift wip` 文档更新为实际子命令结构（`list` / `save` / `restore` / `drop`）。

### 初始化 & 用户配置

- `drift init` 在全局 `user.name` 和 `user.email` 均已配置时跳过交互式输入。
- 邮箱格式校验（`xxx@xxx.xxx`）；输入不合法则循环重试。

### Config 包重构

- `GlobalConfig` 从 `sync/` 移至 `config/`。
- `GlobalUserConfig` 合并为 `UserConfig` — 项目和全局用户身份共用一个 struct。
- `config.go` 重命名为 `project.go`。
- 移除未使用的 `SetGlobalConfigPathForTest`。

### Bug 修复

- DAG walker 中 `collectTreeObjects` 的树哈希改为参数传入（`tree.Hash` 在
  Unmarshal 后为空，DREE 格式不存 self-hash）。
- Tracking ref 命名一致性：Push、Fetch、Clone、fetchRef 统一使用
  `remotes/origin/heads/<branch>`。
- `SyncNow` 同时返回 push 和 pull 计数。
- 分支列表按最后提交时间排序（原为字母序）。
- `drift branch list` 检查移至 create 之前，正确处理显式 `list` 位置参数。
- `drift unstage` 无参数时清空整个暂存区。

### 文档

- `docs/commands.md` — 配置与远程同步章节合并重构。WIP 章节更新。补全 tag 对比
  示例。更新 reflog 示例。删除虚假的 `sync.auto_after_save` 配置项。
- `AGENTS.md` — 构建命令补全 Windows `.exe` 后缀。

### 测试

- 7 个同步引擎测试，覆盖 Push（空远端 / 增量 / 分歧拒绝）、Fetch（全量 /
  已是最新）、Pull、Clone。
- Core 测试中的 `memoryStore` mock 补全 `GetCommit`（`StoreReader` 接口扩展后
  所需）。
