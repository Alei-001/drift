# Drift - 开发进度

## Phase 1：基础框架

**状态：** 已完成

**目标：** 搭建项目骨架，实现 Blob/Tree/Commit 对象存储引擎和 CLI 入口

### 设计决策

| 方面 | 简化方案 | 原版 Git | 理由 |
|------|----------|----------|------|
| 存储格式 | 直接文件 + JSON 元数据 | Packfile + Delta | 实现简单，MVP 够用 |
| 哈希算法 | 纯 SHA-256 | Git Header + Hash | 不需要兼容 Git |
| 文件系统 | os 标准库 | billy (VFS) | 减少依赖 |
| 缓存 | 无 | LRU 缓存 | MVP 阶段性能可接受 |

### 已保证的特性

- [x] 内容寻址去重（相同文件只存一次）
- [x] 原子写入（先写临时文件，再重命名）
- [x] 文件锁（防止并发损坏）

### 任务清单

| 任务 | 文件 | 状态 |
|------|------|------|
| SHA-256 哈希计算 | `internal/core/hash.go` | 已完成 |
| 对象类型定义 | `internal/core/object.go` | 已完成 |
| Blob 对象 | `internal/core/blob.go` | 已完成 |
| Tree 对象 | `internal/core/tree.go` | 已完成 |
| Commit 对象 | `internal/core/commit.go` | 已完成 |
| 对象存储引擎 | `internal/storage/store.go` | 已完成 |
| CLI 根命令 | `internal/cli/root.go` | 已完成 |
| 程序入口 | `cmd/drift/main.go` | 已完成 |

### 验证结果

- [x] `go build ./...` 编译通过
- [x] `go vet ./...` 无警告
- [x] `drift init` 能创建 `.drift/` 目录结构
- [x] 可以存储和读取 Blob/Tree/Commit 对象（API 已实现）

### 测试命令

```bash
# 编译
go build ./...

# 测试 init
drift init
```

---

## Phase 2：暂存区

**状态：** 已完成

**目标：** 实现文件变更检测和暂存区管理

### 设计决策

| 方面 | Drift | go-git | 理由 |
|------|-------|--------|------|
| Index 格式 | 二进制 (DRIX) | 二进制 (DIRC) | 性能优先，支持大量文件 |
| 元数据 | path, hash, mtime, size, mode | hash, name, mtime, size, mode, uid/gid | 去掉不需要的字段 |
| Status 存储 | 动态计算（不存储） | 动态计算 | Status 是派生状态 |
| 变更检测 | mtime+size 快速判断，必要时计算 hash | 同左 | 学习 go-git 的正确做法 |

### 二进制格式 (DRIX)

```
Header:
  - Magic: "DRIX" (4 bytes)
  - Version: uint32 (4 bytes)
  - Entry count: uint32 (4 bytes)

Entry:
  - Path length: uint16 (2 bytes)
  - Path: []byte (variable)
  - Hash: [32]byte (SHA-256)
  - ModifiedAt: int64 (Unix ms)
  - Size: int64
  - Mode: uint32
```

### 核心架构

```
Status = diffCommitWithStaging + diffStagingWithWorktree

commit tree ↔ index  →  Staging 状态（已暂存）
index ↔ working dir  →  Worktree 状态（未暂存）
```

### 任务清单

| 任务 | 文件 | 状态 |
|------|------|------|
| Index 结构 | `internal/core/index.go` | 已完成 |
| Status 类型 | `internal/core/status.go` | 已完成 |
| 目录遍历 | `internal/core/walker.go` | 已完成 |
| 变更检测 | `internal/core/change.go` | 已完成 |
| drift add | `internal/cli/add.go` | 已完成 |
| drift status | `internal/cli/status.go` | 已完成 |
| drift reset | `internal/cli/reset.go` | 已完成 |

### 验证结果

- [x] `go build ./...` 编译通过
- [x] `go vet ./...` 无警告
- [x] `drift add file.txt` 能将文件添加到暂存区
- [x] `drift add .` 能递归添加目录下所有文件
- [x] `drift status` 能正确显示新增/修改/删除/未跟踪状态
- [x] `drift reset` 能清空暂存区

---

## 后续阶段

| 阶段 | 状态 | 预计时间 |
|------|------|----------|
| Phase 3：版本管理 | 待开始 | 2 周 |
| Phase 4：导出和回退 | 待开始 | 2 周 |
| Phase 5：分支功能 | 待开始 | 2 周 |
| Phase 6：对比功能 | 待开始 | 1 周 |
| Phase 7：优化完善 | 待开始 | 2 周 |
