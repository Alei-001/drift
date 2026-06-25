# Phase 1: App 骨架规格

本文档定义 `internal/app/` 包的完整类型契约，作为 P3-P7 阶段的可靠类型基础。

## 文件清单（15 个文件）
| 文件 | 职责 |
|------|------|
| `app.go` | `App` 结构体、`New()`、`Author()`、`ResolveCommit()`、`ReadOperations()`、内部 helper |
| `init.go` | `Init()`、`IsInitialized()`、`Chdir()` |
| `stage.go` | `Add()`、`Unstage()`、`ClearStaging()` |
| `commit.go` | `SaveOptions`、`SaveResult`、`Save()`、`computeChangedPaths()` |
| `query.go` | `HistoryOptions`、`History()`、`Log()`、`Status()` |
| `diff.go` | `DiffOptions`、`DiffEntry`、`DiffResult`、`Diff()` |
| `snapshot.go` | `ExportFormat`、`Export()`、`Restore()` |
| `switch.go` | `SwitchOptions`、`SwitchResult`、`Switch()`、`RestoreWIP()` |
| `branch.go` | `BranchList/Create/Delete/Rename()`、`CurrentBranch()` |
| `name.go` | `NameEntry`、`NameAdd/Delete/List()`、`NamesByHash()` |
| `wip.go` | `WIPEntry`、`WIPList/Save/Restore/Drop()` |
| `file.go` | `RemoveOptions`、`MoveOptions`、`CleanOptions`、`Remove/Move/Clean()` |
| `undo.go` | `OpType`、`OperationEntry`、`RefChange`、`UndoResult`、`Undo()` |
| `config.go` | `ConfigScope`、`ConfigEntry`、`ConfigGet/Set/Unset/List()` |
| `sync.go` | `SyncStatus`、`SyncRemoteOptions`、`Sync*()`、`Clone()` |

## `app.go` — App 结构体与核心方法
```go
package app
type App struct {
	store  *storage.Store
	wt     *worktree.Worktree
	config *config.Config
	dir    string
}
func New(store *storage.Store, cfg *config.Config, dir string) *App
func (a *App) Author() core.Signature
// 返回项目级 config.User，回退到 driftsync.LoadGlobalConfig().User
func (a *App) ResolveCommit(version string) (*core.Commit, error)
// 解析版本标识：name → branch/ID → branch → hash 前缀
func (a *App) ReadOperations() ([]OperationEntry, error)
// 读取操作日志，返回条目按时间倒序（最新在前）
// 是导出方法，供 Log() 和外部消费者使用
```
**内部 helper（非导出，支持实现但不属于骨架契约）**：
```go
func (a *App) currentCommit() (*core.Commit, error)
func (a *App) findCommitByHash(hash string) (*core.Commit, error)
func (a *App) resolveName(version string) string
func matchCommitID(c *core.Commit, version string) bool
func (a *App) hasPendingStagedChanges(idx *core.Index, filters []string) (bool, error)
```
**字段说明**：
- `store`：存储层，内容寻址的文件系统操作
- `wt`：工作树操作层
- `config`：项目级配置，可能为 nil（未初始化时）
- `dir`：工作目录绝对路径

## `init.go` — 生命周期
```go
func (a *App) Init() error
// 创建 .drift/ 目录结构，初始化 main 分支和 HEAD
func (a *App) IsInitialized() bool
// 检查 .drift/ 是否存在
func (a *App) Chdir(dir string) error
// 切换工作目录，重建 store/config/wt
```

## `commit.go` — 提交
```go
type SaveOptions struct {
	Amend bool   // 修订上一个版本
	All   bool   // 自动暂存所有变更
	Name  string // 保存后为此版本分配名称
}
type SaveResult struct {
	ID          string   // 提交 ID（hash 前 8 字符）
	Message     string   // 提交消息
	Branch      string   // 分支名
	StagedPaths []string // 实际变更的路径列表
	Amended     bool     // 是否为修订
}
func (a *App) Save(msg string, opts SaveOptions) (*SaveResult, error)
// msg 是独立参数，不是 opts 字段
```
**注意**：`SaveOptions` **不含** `Message` 字段。message 通过 `msg` 参数传入。`SaveResult.Message` 字段是正确的，应保留。

## `query.go` — 查询
```go
type HistoryOptions struct {
	Branch string // 指定分支，空则用当前分支
	All    bool   // 列出所有分支的提交
	Limit  int    // 限制数量，<=0 表示不限
}
func (a *App) History(opts HistoryOptions) ([]*core.Commit, error)
func (a *App) Log(limit int) ([]OperationEntry, error)
// 包装 ReadOperations()，limit <=0 不限
func (a *App) Status() (*core.Status, error)
// 委托 core.ComputeStatus，返回指针
```
**返回类型约定**：App 层统一返回指针（`*core.Status`、`[]*core.Commit`），core 层可能返回值（`core.ComputeStatus` 返回 `core.Status` 值）。这是有意的层级不对称。

## `diff.go` — 差异
```go
type DiffOptions struct {
	V1    string   // 版本标识，空表示工作树
	V2    string   // 版本标识，空表示工作树
	Paths []string // 文件路径过滤
}
type DiffEntry struct {
	Path     string          // 相对路径
	Status   string          // "added" | "deleted" | "modified"
	IsBinary bool
	OldSize  int64
	NewSize  int64
	Edits    []core.DiffEdit // 行级编辑脚本
}
type DiffResult struct {
	Entries []DiffEntry
}
func (a *App) Diff(opts DiffOptions) (*DiffResult, error)
// V1==""&&V2=="" → 工作树 vs 最新提交
// V2=="" → 工作树 vs V1
// 否则 → V1 vs V2
```

## `snapshot.go` — 导出与恢复
```go
type ExportFormat string
const (
	ExportDir ExportFormat = "dir"
	ExportZip ExportFormat = "zip"
	ExportTar ExportFormat = "tar"
)
type RestoreResult struct {
	Version  string
	Added    int
	Modified int
	Deleted  int
}
func (a *App) Export(version, output string, format ExportFormat) error
func (a *App) Restore(version string, filters []string, force bool) (*RestoreResult, error)
```

## `switch.go` — 分支切换
```go
type SwitchOptions struct {
	Force  bool // 强制切换，跳过 WIP 保存
	Create bool // 不存在则创建
}
type SwitchResult struct {
	Branch      string
	Created     bool
	WIPSaved    bool
	EmptyBranch bool
}
func (a *App) Switch(branch string, opts SwitchOptions) (*SwitchResult, error)
func (a *App) RestoreWIP(branch string) (int, error)
// 恢复 WIP 到索引和工作树，返回恢复条目数
```

## `branch.go` — 分支管理
```go
func (a *App) CurrentBranch() string
// 读 HEAD，默认 "main"
func (a *App) BranchList() ([]string, error)
func (a *App) BranchCreate(name string) error
func (a *App) BranchDelete(name string) error
func (a *App) BranchRename(oldName, newName string) error
```

## `name.go` — 版本命名
```go
type NameEntry struct {
	Label string
	Hash  string
}
func (a *App) NameAdd(version, label string) error
func (a *App) NameDelete(label string) error
func (a *App) NameList() ([]NameEntry, error)
func (a *App) NamesByHash() map[string][]string
```

## `wip.go` — 工作进度
```go
type WIPEntry struct {
	Path string
	Hash string
	Mode uint32
}
func (a *App) WIPList(branch string) ([]WIPEntry, error)
func (a *App) WIPSave(branch string) error
func (a *App) WIPRestore(branch string) (int, error)
func (a *App) WIPDrop(branch string) error
```

## `file.go` — 文件操作
```go
type RemoveOptions struct {
	Cached    bool // 仅从索引移除，不删工作树文件
	Recursive bool // 递归移除目录
}
type MoveOptions struct {
	Force bool // 覆盖已存在目标
}
type CleanOptions struct {
	DryRun bool // 只列出不移除
	Dirs   bool // 包含目录
}
func (a *App) Remove(paths []string, opts RemoveOptions) error
func (a *App) Move(sources []string, dest string, opts MoveOptions) error
func (a *App) Clean(opts CleanOptions) ([]string, error)
```

## `undo.go` — 操作日志与撤销
```go
type OpType string
const (
	OpSave         OpType = "save"
	OpSwitch       OpType = "switch"
	OpBranchDelete OpType = "branch-delete"
	OpBranchRename OpType = "branch-rename"
	OpRestore      OpType = "restore"
	OpNameAdd      OpType = "name-add"
	OpNameDelete   OpType = "name-delete"
)
type OperationEntry struct {
	Timestamp  time.Time   `json:"timestamp"`
	Op         OpType      `json:"op"`
	Desc       string      `json:"desc"`
	RefChanges []RefChange `json:"ref_changes"`
}
type RefChange struct {
	Ref    string `json:"ref"`
	Before string `json:"before"`
	After  string `json:"after"`
}
type UndoResult struct {
	Entry          OperationEntry
	RemainingCount int
}
func (a *App) Undo(count int) (*UndoResult, error)
```
**内部 helper（非导出）**：
```go
func (a *App) recordOperation(op OpType, desc string, changes []RefChange) error
func (a *App) removeLastOperation() error
func (a *App) undoOne() (*OperationEntry, int, error)
```
**关键**：`OperationEntry` 和 `RefChange` 的 JSON tag **必须**与 `repo/history.go` 完全一致，否则破坏日志兼容性。`ReadOperations()` 是导出方法，定义在 `app.go`（见上文）。

## `config.go` — 配置
```go
type ConfigScope string
const (
	LocalScope  ConfigScope = "local"
	GlobalScope ConfigScope = "global"
)
type ConfigEntry struct {
	Key   string
	Value string
}
func (a *App) ConfigGet(scope ConfigScope, key string) (string, error)
func (a *App) ConfigSet(scope ConfigScope, key, value string) error
func (a *App) ConfigUnset(scope ConfigScope, key string) error
func (a *App) ConfigList(scope ConfigScope) ([]ConfigEntry, error)
```
**已知债务**：`config.go` 导入 `internal/sync`（`driftsync.LoadGlobalConfig`），导致 app→sync 依赖方向倒置。已在 `architecture-refactor.md` §3.4 记录，P1 不修。

## `sync.go` — 同步
```go
type SyncStatus struct {
	Enabled    bool
	RemoteName string
	LastSync   string
}
type SyncRemoteOptions struct {
	Host               string
	Port               int
	Path               string
	Username           string
	Password           string
	TLS                bool
	InsecureSkipVerify bool
	Share              string
	KeyPath            string
}
func (a *App) SyncEnable() error
func (a *App) SyncDisable() error
func (a *App) SyncNow() error
func (a *App) SyncStatus() (*SyncStatus, error)
func (a *App) SyncEnabled() bool
func (a *App) SyncRemoteSet(protocol string, opts SyncRemoteOptions) error
func (a *App) Clone(remoteName, destDir string) error
```