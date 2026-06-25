# Phase 2: App 方法实现审计与修正

## 一、目标与背景
**目标**：对照 `internal/repo/` 原始实现和 `internal/cli/` 原始逻辑，审计 `internal/app/` 包的方法实现，找出并修正所有逻辑遗漏、功能缺失、静默吞错（违反 C1 规则）的问题。
**背景**：`internal/app/` 已是完整实现（P1+P2 合并产物）。大部分方法已正确移植自 `repo/` 包，但存在若干功能缺失和错误处理问题。本计划采用"审计并修正"模式。
**约束**：
- 不修改 `internal/repo/`、`internal/cli/`、`internal/core/` 等其他包
- 修正后必须保持 `go build ./...` 和 `go test ./...` 通过
- 遵循 `docs/refactoring/README.md` 中的错误处理约定 C1-C5
- 如果修正涉及类型签名变更，需同步更新 P1 文档

## 二、审计发现的偏差
### 严重问题
#### S1：`Restore()` 签名和功能不完整
**现状**（`app/snapshot.go:23`）：
```go
func (a *App) Restore(version string) error
```
**repo 原版**（`repo/restore.go:22`）：
```go
func (r *Repository) Restore(version string, filters []string, force bool) (*RestoreResult, error)
```
**缺失的功能**：
1. `filters []string` — 路径过滤（CLI 支持 `drift restore <version> <path>...`）
2. `force bool` — 安全检查（CLI 支持 `--force` 标志，非 force 时检查 pending staged changes 和 worktree modifications）
3. `RestoreResult` 返回类型 — CLI 打印 "Restored to %s: %d added, %d modified, %d deleted"
4. 过滤模式下的索引保留逻辑（非匹配路径的索引条目应保留）
5. dirty 检查逻辑（非 force 时拒绝有未提交变更的恢复）
**影响**：P4 重写 CLI restore 命令时，`app.Restore()` 无法提供 CLI 所需功能。
**CLI 调用证据**（`cli/restore.go:34`）：
```go
result, err := sharedRepo.Restore(version, filters, force)
```

#### S2：`WIPSave()` 缺少 hasPending 检查
**现状**（`app/wip.go:33-53`）：
```go
func (a *App) WIPSave(branch string) error {
    // 加载索引 → StageWorktreeChanges → SaveWIP → 清空索引
    // 缺少 hasPending 检查
}
```
**repo 原版**（`repo/wip.go:16-51`）：
```go
func (r *Repository) WIPSave() error {
    // 加载索引 → StageWorktreeChanges → 检查 hasPending → 无变更则 no-op → SaveWIP → 清空索引
}
```
**影响**：即使没有任何变更，也会保存空 WIP 并清空索引。
**注意**：app 版本的 `WIPSave(branch string)` 签名是 P1 规格定义的增强（允许为非当前分支保存 WIP），签名本身正确。只是缺少 hasPending 检查逻辑。

### 中等问题
#### M1：`Restore()` 未记录操作日志
**现状**：`OpRestore` 常量已定义（`undo.go:22`）但从未使用。`Restore()` 成功后未调用 `recordOperation`。
**性质**：repo 原版也有此问题（原有 bug）。P1 文档已记录为 P2 待办。应在 S1 修正 Restore 时一并修复。

#### M2：`diff.go:221` 静默吞错（违反 C1）
**现状**：
```go
_ = a.store.LoadIndex(&idx)  // 索引加载失败被忽略
```
**影响**：索引加载失败时，untracked 文件检测失效。

#### M3：`commit.go:109,142` NameAdd 静默吞错缺少注释
**现状**：
```go
_ = a.NameAdd(commit.ID, opts.Name)  // 无注释
```
**repo 原版**：
```go
if err := r.AddName(commit.ID, opts.Name); err != nil {
    // Non-fatal: name assignment failure shouldn't block the save.
}
```
**影响**：违反 C1 规则（需有 `// Non-fatal:` 注释）。

#### M4：`file.go:97` WalkWorkingDirWithIgnore 静默吞错（违反 C1）
**现状**：
```go
_ = core.WalkWorkingDirWithIgnore(fullDir, a.dir, func(path string, info os.FileInfo) error {
    // 收集目录内文件路径
})
```
**影响**：walk 错误被忽略，路径收集可能不完整，导致 `drift rm -r` 漏删文件。

### 低优先级（可接受，建议加注释）
#### L1：`branch.go:69,84,95-96` 的 `, _ := a.store.GetRef(...)`
**性质**：用于获取 undo 信息的"尽力而为"读取。repo 原版也有相同模式。如果 GetRef 失败，后续操作（DeleteRef 等）也会失败并报错。
**处理**：保留，但建议加注释说明"尽力获取 hash 用于 undo 记录"。

#### L2：`snapshot.go:56` 的 `, _ := reader.ListBlobs(t, "")`
**性质**：获取前一个版本 blob 列表的"尽力而为"读取。repo 原版也有（`restore.go:85`）。
**处理**：保留，加注释。

#### L3：`file.go:193-194,282-283` 的 `filepath.Abs/Rel` 的 `, _ :=`
**性质**：这些函数在正常路径下极少失败。
**处理**：保留。

#### L4：`switch.go:234` DeleteWIP 静默吞错
**现状**：已有 `// Non-fatal:` 注释（line 232-233）。
**处理**：符合 C1 规则，无需修改。

### 已正确改进的部分（无需修改）
- `recordOperation()` 返回 error 并被检查（repo 版本不返回 error）
- `Undo()` 支持多次 undo（count 参数），修复了 repo 版本的 `_ = r.Store.DeleteRef()` 静默吞错
- `validateNameLabel` 改为小写（内部 helper 不需导出）
- `branch.go` 的 `recordOperation` 调用检查返回值（repo 版本不检查）

## 三、修正任务
### 修正任务 M1：补全 `Restore()` 签名和功能（严重）
**涉及文件**：
- `internal/app/snapshot.go` — 修改 Restore 实现
- `docs/refactoring/01-phase1-app-skeleton.md` — 同步更新 P1 规格中的 Restore 签名和 RestoreResult 类型
**操作**：
1. 在 `snapshot.go` 中添加 `RestoreResult` 类型：
```go
type RestoreResult struct {
    Version  string
    Added    int
    Modified int
    Deleted  int
}
```
2. 修改 `Restore` 签名和实现，移植自 `repo/restore.go:22-174`：
```go
func (a *App) Restore(version string, filters []string, force bool) (*RestoreResult, error)
```
**必须包含的逻辑**：
- `hasFilter := len(filters) > 0`
- 非 force 时：检查 pending staged changes（`hasPendingStagedChanges`）和 worktree modifications（`a.wt.HasModifications`），有变更则拒绝
- 解析版本 → 加载目标 tree → 列出 blob
- 有 filter 时：过滤 blob，无匹配则报错
- 获取前一个版本的 blob 列表（用于统计 added/modified/deleted）
- 有 filter 时：保留非匹配路径的索引条目
- 写入目标 blob 到工作树，构建新索引
- 删除不在目标版本中的文件
- 统计 added/modified/deleted
- 清理空目录
- 保存索引
- **记录操作日志**（`recordOperation(OpRestore, ...)`）—— 这是修复 M1
3. 更新 P1 文档中 `snapshot.go` 部分：
```go
type RestoreResult struct {
    Version  string
    Added    int
    Modified int
    Deleted  int
}
func (a *App) Restore(version string, filters []string, force bool) (*RestoreResult, error)
```
**验收**：
- `go build ./...` 通过
- Restore 支持 filters、force、返回 RestoreResult
- Restore 成功后调用 `recordOperation(OpRestore, ...)`
- P1 文档与代码一致

### 修正任务 M2：补全 `WIPSave()` 的 hasPending 检查（严重）
**涉及文件**：`internal/app/wip.go`
**操作**：在 `WIPSave` 的 `StageWorktreeChanges` 之后、`SaveWIP` 之前，添加 hasPending 检查：
```go
func (a *App) WIPSave(branch string) error {
    var idx core.Index
    if err := a.store.LoadIndex(&idx); err != nil {
        return err
    }
    if err := a.wt.StageWorktreeChanges(&idx); err != nil {
        return err
    }
    // 检查是否有实际变更，无变更则 no-op
    hasPending, err := a.hasPendingStagedChanges(&idx, nil)
    if err != nil {
        return fmt.Errorf("failed to check pending staged changes: %w", err)
    }
    if !hasPending {
        return nil
    }
    if err := a.wt.SaveWIP(branch, &idx); err != nil {
        return err
    }
    emptyIdx := &core.Index{}
    if err := a.store.SaveIndex(emptyIdx); err != nil {
        return err
    }
    return nil
}
```
**验收**：
- `go build ./...` 通过
- 无变更时 `WIPSave` 返回 nil 且不创建 WIP 文件

### 修正任务 M3：修正 `diff.go:221` 静默吞错（中）
**涉及文件**：`internal/app/diff.go`
**操作**：将 `_ = a.store.LoadIndex(&idx)` 改为加注释说明（而非返回错误，因为全新仓库中索引文件不存在）：
```go
// Non-fatal: index may not exist in a fresh repo; empty index means all files are untracked, which is correct.
_ = a.store.LoadIndex(&idx)
```
**验收**：`go build ./...` 通过；diff.go 中 `_ = a.store.LoadIndex` 有注释。

### 修正任务 M4：为 `commit.go:109,142` 的 NameAdd 加 Non-fatal 注释（中）
**涉及文件**：`internal/app/commit.go`
**操作**：将两处 `_ = a.NameAdd(commit.ID, opts.Name)` 改为有注释的形式：
```go
if err := a.NameAdd(commit.ID, opts.Name); err != nil {
    // Non-fatal: name assignment failure shouldn't block the save.
}
```
**验收**：`go build ./...` 通过；commit.go 中 `_ = a.NameAdd` 不存在。

### 修正任务 M5：修正 `file.go:97` WalkWorkingDirWithIgnore 静默吞错（中）
**涉及文件**：`internal/app/file.go`
**操作**：将 `collectDir` 改为返回 `error`，并在两处调用点检查返回值：
```go
collectDir := func(dirRel string) error {
    fullDir := filepath.Join(a.dir, filepath.FromSlash(dirRel))
    if err := core.WalkWorkingDirWithIgnore(fullDir, a.dir, func(path string, info os.FileInfo) error {
        relPath := filepath.ToSlash(filepath.Join(dirRel, path))
        addPath(relPath)
        return nil
    }); err != nil {
        return fmt.Errorf("failed to walk directory %s: %w", dirRel, err)
    }
    return nil
}
```
并在调用点：
```go
if err := collectDir(rel); err != nil {
    return nil, err
}
```
**验收**：`go build ./...` 通过；file.go 中 `_ = core.WalkWorkingDirWithIgnore` 不存在。

### 修正任务 M6：为低优先级 `, _ :=` 加注释说明（低）
**涉及文件**：`internal/app/branch.go`、`internal/app/snapshot.go`、`internal/app/switch.go`、`internal/app/file.go`
**操作**：在以下位置加注释说明"尽力而为"意图：
- `branch.go:69`：`branchHash, _ := a.store.GetRef(name)` → 加 `// Best-effort: hash for undo record; DeleteRef will fail if branch doesn't exist.`
- `branch.go:84`：`headBefore, _ := a.store.GetRef("HEAD")` → 加 `// Best-effort: for undo record.`
- `branch.go:95-96`：同样加注释
- `snapshot.go:56`：`prevBlobsList, _ := reader.ListBlobs(t, "")` → 加 `// Best-effort: for added/modified/deleted statistics.`
- `switch.go:76`：`parentHash, _ := a.store.GetRef(currentBranch)` → 加 `// Best-effort: parent hash for new branch creation.`
- `file.go:193-194,282-283`：`filepath.Abs/Rel` 的 `, _ :=` → 加 `// Best-effort: these functions rarely fail for normal paths.`
**验收**：上述每处 `, _ :=` 都有注释说明意图。

### 修正任务 M7：全量验证
**操作**：
```bash
go build ./internal/app/...
go build ./...
go test ./...
```
**验收**：三个命令退出码均为 0。

### 修正任务 M8：更新 P2 文档
**操作**：用本计划内容重写 `docs/refactoring/02-phase2-app-implementation.md`，反映"审计并修正"模式。
**验收**：文档与实际代码一致。

## 四、验收标准（整体）
| 编号 | 验收项 | 验证方式 |
|------|--------|----------|
| AC1 | `go build ./internal/app/...` | 退出码 0 |
| AC2 | `go build ./...` | 退出码 0 |
| AC3 | `go test ./...` | 退出码 0 |
| AC4 | `Restore` 签名为 `(version string, filters []string, force bool) (*RestoreResult, error)` | 检查 snapshot.go |
| AC5 | `RestoreResult` 类型存在且含 Version/Added/Modified/Deleted 字段 | 检查 snapshot.go |
| AC6 | `Restore` 成功后调用 `recordOperation(OpRestore, ...)` | grep 确认 |
| AC7 | `WIPSave` 有 hasPending 检查，无变更时返回 nil | 检查 wip.go |
| AC8 | `diff.go` 中 `_ = a.store.LoadIndex` 有注释 | 检查 diff.go |
| AC9 | `commit.go` 中无 `_ = a.NameAdd` | grep 确认 |
| AC10 | `file.go` 中无 `_ = core.WalkWorkingDirWithIgnore` | grep 确认 |
| AC11 | app 包中所有 `_ =` 和 `, _ :=` 要么有 `// Non-fatal:` 注释，要么有 `// Best-effort:` 注释 | grep 确认 |
| AC12 | P1 文档中 Restore 签名已更新 | 检查 01-phase1 文档 |
| AC13 | P2 文档已重写为审计修正模式 | 检查 02-phase2 文档 |

## 五、执行顺序
1. **M1**（补全 Restore）→ `go build ./...` — 最严重，涉及签名变更
2. **M2**（补全 WIPSave hasPending）→ `go build ./...` — 严重逻辑缺失
3. **M3**（diff.go 静默吞错）→ `go build ./...`
4. **M4**（commit.go Non-fatal 注释）→ `go build ./...`
5. **M5**（file.go 静默吞错）→ `go build ./...`
6. **M6**（低优先级注释）→ `go build ./...`
7. **M7**（全量验证）→ `go build && go test`
8. **M8**（更新文档）
理由：先做涉及签名变更的 M1（影响面最大），再做逻辑缺失的 M2，然后按严重程度依次处理静默吞错，最后做文档。

## 六、不在 P2 范围内的事项
- `app→sync` 依赖方向倒置（架构债务，已在 architecture-refactor.md 记录）
- CLI 层重写（P4 职责）
- 旧 repo 包迁移到 .wastebasket（P5 职责）
- 测试迁移（P6 职责）