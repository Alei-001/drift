# Drift - 开发进度

## Phase 1：基础框架 ✅

**目标：** 搭建项目骨架，实现 Blob / Tree / Commit 对象存储引擎和 CLI 入口。

### 设计决策

| 方面 | Drift | Git / go-git | 理由 |
|------|-------|-------------|------|
| 哈希 | 纯 SHA-256 | Git header + SHA-1/SHA-256 | 不兼容 Git |
| 存储 | 内容寻址 + 二进制格式 | Packfile + Delta | MVP 简洁 |
| 文件系统 | `os` 标准库 | billy VFS | 减少依赖 |

### 已完成

| 任务 | 文件 |
|------|------|
| SHA-256 哈希计算 | `internal/core/hash.go` |
| 对象类型定义 | `internal/core/object.go` |
| Blob / Tree / Commit 对象 | `internal/core/blob.go, tree.go, commit.go` |
| 对象存储引擎 | `internal/storage/store.go` |
| CLI 根命令 + `drift init` | `internal/cli/root.go` |

---

## Phase 2：暂存区 ✅

**目标：** 实现文件变更检测和暂存区管理。

### 设计决策

| 方面 | Drift | go-git |
|------|-------|--------|
| Index 格式 | 二进制 DRIX | 二进制 DIRC |
| Status 存储 | 动态计算（不存储） | 同 |
| 变更检测 | mtime+size 快速判断 → hash 确认 | 同 |

### 已完成

| 任务 | 文件 |
|------|------|
| Index 结构 + DRIX 编解码 | `internal/core/index.go, index_codec.go` |
| Status 类型 | `internal/core/status.go` |
| 目录遍历 | `internal/core/walker.go` |
| 变更检测 + 状态计算 | `internal/core/change.go` |
| `drift add` / `status` / `unstage` | `internal/cli/add.go, status.go, unstage.go` |

---

## Phase 3：版本管理 ✅

**目标：** 实现版本提交和历史管理。

### 设计决策

| 方面 | Drift | go-git |
|------|-------|--------|
| Tree 格式 | 二进制 DREE | 文本（mode name\0hash） |
| Commit 格式 | 二进制 DCMT | headers + message |
| Tree 结构 | 递归（每个目录独立对象） | 同 |
| Commit 哈希 | 含时间戳（同一输入不同时间 → 不同哈希） | 含作者时间 |

### 已完成

| 任务 | 文件 |
|------|------|
| Tree 编解码 (DREE) | `internal/core/tree_codec.go` |
| Commit 编解码 (DCMT) | `internal/core/commit_codec.go` |
| Tree 构建器（递归） | `internal/core/tree_builder.go` |
| Tree 遍历（递归展平 + Diff） | `internal/core/tree_walker.go` |
| `drift save` / `log` | `internal/cli/save.go, log.go` |

---

## Phase 4：导出与回退 ✅

**目标：** 实现版本导出和回退功能。

### 设计决策

| 方面 | Drift | go-git |
|------|-------|--------|
| 导出格式 | dir / zip / tar.gz | tar / zip |
| 回退方式 | index ↔ worktree 对比（增量） | tree diff + index diff |
| 冲突检测 | 暂存区非空需 `--force` | check unstaged changes |

### 已完成

| 任务 | 文件 |
|------|------|
| `drift export`（dir/zip/tar.gz） | `internal/cli/export.go` |
| `drift restore`（增量 + conflict） | `internal/cli/restore.go` |

---

## Phase A：Bug 修复 ✅

基于 go-git 差距分析，优先修复已知缺陷。详见 [issues.md](issues.md)。

| 编号 | 任务 | 文件 | 状态 |
|------|------|------|------|
| A1 | commitFiles 递归展平（子目录文件误判） | `change.go` | ✅ 已完成 |
| A2 | LoadIndex 错误处理（静默吞错） | `add.go` | ✅ 已完成 |
| A3 | NewTree 返回 error（静默空 hash） | `tree.go, tree_builder.go` | ✅ 已完成 |
| A4 | parent hash sentinel 健壮化 | `commit_codec.go, commit.go` | ✅ 已完成 |
| A5 | bytes.Equal 替代 string() 比较 | `restore.go` | ✅ 已完成 |

---

## Phase B：架构加固 ✅

| 编号 | 任务 | 影响范围 | 状态 |
|------|------|----------|------|
| B1 | Storage interface | 新增 `storage.go` | ✅ 已完成 |
| B2 | Index byPath map 索引 | `index.go` | ✅ 已完成 |
| B3 | FileMode 规范化 | 新增 `filemode.go` | ✅ 已完成 |
| B4 | Object 完整性校验 | `store.go` | ✅ 已完成 |
| B5 | OS 级文件锁 | 新增 `lock.go`, `lock_windows.go`, `lock_unix.go` | ✅ 已完成 |

---

## Phase C：功能补全 ✅

| 编号 | 任务 | 影响范围 | 状态 |
|------|------|----------|------|
| C1 | Config 系统 | 新增 `config/config.go` | ✅ 已完成 |
| C2 | .driftignore | 新增 `driftignore.go`, 更新 `walker.go` | ✅ 已完成 |
| C3 | 进度回调 | `progress.go`, `export.go` | ✅ 已完成 |
| C4 | Branch / Switch | 新增 `branch.go`, `switch.go` | ✅ 已完成 |
| C5 | Diff 命令 | 新增 `diff.go` | ✅ 已完成 |
| C6 | Signature 模型 | `commit.go`, `commit_codec.go` | ✅ 已完成 |

---

## Phase D：性能与规模 🔧

| 编号 | 任务 | 触发条件 | 状态 |
|------|------|----------|------|
| D1 | 迭代器模式 | 提交>1000时 | ⏸ 推迟 |
| D2 | 进度回调 | 所有大文件操作 | ✅ 已完成 |
| D3 | 并行 diff | 性能瓶颈时 | ⏸ 推迟 |
| D4 | Mmap 大文件 | 文件>1GB时 | ⏸ 推迟 |

---

## 后续阶段（原计划，已整合到以上 Phase）

| 原阶段 | 内容 | 整合到 |
|--------|------|--------|
| Phase 5：分支功能 | branch / switch | Phase C |
| Phase 6：对比功能 | diff | Phase C |
| Phase 7：优化完善 | 忽略规则、性能 | Phase B / C / D |
