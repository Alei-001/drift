# 更新日志

## v1.1.0 (2026-06-27)

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
