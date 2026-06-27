# 更新日志

## v2.0.0 (2026-06-27)

### 破坏性变更

- **`drift init` → `drift start`**。原命令不再可用。
- **`drift log` → `drift history`**。`--oneline` → `--brief`。
- **`drift restore` → `drift back`**。支持无参形式（恢复到最新版本）。
- **`drift mv` → `drift move`**。`drift rm` → `drift remove`。
- **删除 `drift switch`**。合并到 `drift branch switch <名称>`。
- **`drift branch` 标志删除**。`-d` → `branch remove`，`-m` → `branch rename`。
- **`drift tag --delete` → `drift tag remove`**。标签列表改为 `drift tag list`。
- **删除 `drift config`**。拆分到 `whoami` / `remote` / `backup` / `ignore`。
- **`drift sync` → `drift backup`**。子命令：`on`/`off`/`now`/`status`/`log`。
- **删除 `drift add` / `drift unstage`**。无暂存区——`drift save` 自动检测所有改动。
- **`drift wip` / `drift reflog` 移出 CLI**。WIP 完全自动化；reflog 内部使用。

### 新增命令

- `drift ignore <模式>` — 添加 .driftignore 规则。
- `drift whoami` / `drift whoami set <姓名> <邮箱>` — 身份管理。
- `drift remote {setup|show|remove}` — 交互式远程配置。
- `drift backup {on|off|now|status|log}` — 远程备份管理。

### 新功能

- **取消暂存区。** `drift save` 自动检测所有工作区变更（修改、新增、删除）。`.driftignore` 规则自动生效。
- **CRLF→LF 自动处理。** 所有文本文件保存时自动归一化为 LF 后计算哈希，二进制文件原样存储。无需 `core.autocrlf` 配置。
- **`save --tag` 标志。** 保存时直接打标签。标签冲突仅 warning，不影响 save。
- **密码加密。** `~/.drift/global.json` 中的密码以 AES-256-GCM 加密存储，密钥在 `~/.drift/.key`（权限 0600）。也支持 `DRIFT_REMOTE_PASSWORD` 环境变量。
- **中文 tag/分支名。** 标签和分支名支持 Unicode（中文、日文、emoji）。
- **`drift remote setup` 交互式配置。** 分步向导，不再需要记 `--protocol` 等 10 个标志。
- **`drift backup log` 备份历史。** 展示最近备份记录。

### Bug 修复

- **新 save 流程缺少 blob 存储。** 旧流程通过 staging 自动存储 blob；新流程无 staging，需显式存储。在 `Save()` 中为所有变更文件增加 blob 持久化循环，使用 CRLF 归一化内容匹配哈希计算。
- **BuildChangedIndex 遍历中修改 slice。** `range idx.Entries` 内调用了 `idx.Remove`，导致元素跳过。修复为先收集删除列表，循环结束后再移除。
- **符号链接被跳过。** `BuildChangedIndex` 此前跳过了非普通文件，符号链接全部丢失。现通过 `os.Readlink` 检测并正确跟踪。
- **文件类型变更时 mode 未更新。** 当文件类型变化（普通↔符号链接、普通↔可执行），索引中 mode 保持旧值。修复为在差异遍历时同步更新 mode。

---

### Bug 修复

- **Save 不再丢失子目录文件。** `GetTree` 返回的 Tree 对象 Hash 字段未设置，导致
  `BuildFromIndexWithBase` 失败后静默回退，使新 commit 丢失所有子目录条目。
- **子树复用优化恢复。** 树构建器的 `BuildFromIndexWithBase` 快速路径在比较新旧
  子树条目时，新条目未排序而旧条目已排序，导致比较永久不匹配，每次 save 都从
  零重建全部子树。
- **`--no-color` 标志可正常工作。** `BuildRootCmd` 内的局部变量遮蔽了包级变量，
  导致 `--no-color` 标志绑定到局部变量，`useColor()` 永远读不到用户设置。
- **Windows autocrlf 状态下 status 不再误报修改。** `HasModifications` 在
  Windows 且 `autocrlf=true` 时现用 LF 归一化 hash 与 blob 存储格式比较。
- **工作树中的符号链接现可正确检测。** `drift status` 此前对所有符号链接都
  标记为 Modified，因为 `CalculateHashFromFile` 跟随了符号链接而非比较目标路径
  字符串的 hash。
- **路径穿越漏洞修复。** `ExpandAddPaths`、`NormalizePathFilters` 和 `Clone`
  现均调用 `ValidateTreePath` 验证路径是否安全，防止目录逃逸攻击。
- **SFTP 主机密钥验证。** SFTP 连接默认通过 `~/.ssh/known_hosts` 验证主机
  密钥。如需跳过，设置 `insecure_skip_verify: true`。
- **FTP 同步目录创建修复。** `mkdirAll` 错误地重复了 basePath 前缀，导致
  对象存储到错误的远程路径。
- **首次 Push 不再 panic。** `trackingHash[:8]` 在 tracking ref 不存在时
  panic，现所有 hash 切片均使用 `shortHash()` 保护。
- **Commit hash 完整性。** `NewCommit` 现验证 message、author name 和 email
  不含 NUL 字节（会破坏 hash 计算），并返回 `error`。
- **Diff 现可检测 mode 变更。** `DiffTrees` 和 `LazyDiffTrees` 在比较 hash
  之外同时比较 mode，`chmod +x` 变更会在 `drift diff` 中可见。
- **Commit 序列化增加校验。** `Commit.Marshal` 在编码前拒绝空 Hash/TreeHash，
  防止写出损坏的 commit 文件。
- **错误处理全面强化。** `BranchCreate`、`Switch`、`Save`、`Restore`、`Push`、
  `Pull`、`TagAdd`、`Move`、`ResolveCommit`、`ListRefs` 不再静默丢弃
  `GetRef`、`GetTree`、`GetCommit`、`currentCommit` 和 `filepath` 操作
  返回的 I/O 错误，明确区分 `ErrObjectNotFound` 与真实故障。
- **僵死锁检测。** 锁轮询循环现检查记录的 PID 是否存活，发现进程已死时立即
  清理锁文件并重试，不再等待完整的 5 秒超时。
- **`Chdir` 原子性。** `App.Chdir` 不再在验证 config 前修改 `a.store`，
  防止 config 加载失败时 App 进入半切换状态。
- **`SyncEnable` nil 配置检查。** `SyncEnable` 和 `SyncDisable` 现在
  访问 sync 设置前检查 `a.config` 是否为 nil。
- **`StageWorktreeChanges` 处理已删除文件。** WIP save 现会从 index 中
  移除工作树中被删除的文件条目，防止产生过时的 WIP 快照。
- **`DeleteRef` 更新 HEAD。** 删除当前检出的分支时，现于锁内清除 HEAD，
  避免悬空引用。
- **`lock()` 失败时返回 nil unlocker。** 此前返回空函数可能被误用，现改为
  `nil` 使误用立即可见。
- **统一错误哨兵值。** `ErrCorruptedObject`（解压错误）与 `ErrObjectCorrupted`
  （hash 不一致）合并为单一哨兵值，`errors.Is` 可覆盖两种情况。

### 改进

- **CLI 全命令着色。** 21 个命令统一使用 ANSI 颜色：成功/新增=绿色，警告/
  修改=黄色，错误/删除=红色，标题/分支名=青色，空状态=灰色。
- **log 和 reflog 表格对齐。** 列宽格式化现先计算纯文本宽度再应用颜色，
  表头与数据列对齐一致，列间距加宽。
- **Clone 正确写出符号链接。** Clone 现通过 `os.Symlink` 创建符号链接，
  不再将目标路径写为普通文件。
- **`GetBlobSize` 错误类型统一。** 与其他对象获取器一致，使用
  `ErrObjectCorrupted`。
- **`PutCommit` 存在性检查。** 增加与 `PutBlob`、`PutTree` 相同的
  `os.Stat` 提前返回机制，避免不必要的重复写入。

---

### 新功能

- `drift gc` 移除不可达对象以回收磁盘空间。从任何分支、tag、HEAD 或
  reflog 条目可达的对象均会被保留。
- `drift upgrade [<version>]` 从 GitHub Releases 下载指定版本（或最新版）
  的自升级命令。`--check` 仅预览不下载。
- `gc.auto` 配置项控制自动 GC 阈值（默认 1000 个松散对象，设为 0 禁用）。
  自动 GC 在 `drift save` 和 `drift branch delete` 之后触发。
- `gc.reflogExpire` 配置项（默认 90 天）限制基于 reflog 的对象保留期：
  超过期限的条目在 GC 时被忽略，使旧 amend 提交的对象可被回收。

### Bug 修复

- `drift diff` 默认摘要输出现使用 A / D（而非 + / -）标记新增和删除的
  文件，与 `drift status` 的标记风格一致。
- `drift diff --patch` 现可正确显示空文件和新文件的内容变更（此前输出
  空 diff 体）。

### 改进

- Diff 性能：LazyDiffTrees 集成到所有 diff 路径，index mtime 快径避免
  重复读取未修改的文件，树构建器复用基树中的子树以加速新 commit 计算。

---

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
