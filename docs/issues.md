# Drift 已知问题与修复计划

> 基于 go-git 参考实现的差距分析结果。

---

## Phase A：Bug 修复

### A1. `change.go` — commitFiles 只取了根级 entries，子目录文件全部不可见

**文件**: `internal/core/change.go` 第 12-18 行

```go
commitFiles := make(map[string]string)
if commitTree != nil {
    for _, e := range commitTree.Entries {
        if e.Type == BlobObject {
            commitFiles[e.Name] = e.Hash
        }
    }
}
```

**问题**: `commitTree.Entries` 是根级扁平列表。如果 commit 中有 `dir/file.txt`，它的 entry 存在 `dir/` 这个 subtree 的 Entries 中，根级 Tree 的 Entries 里只有 `{Name: "dir", Type: TreeObject}`。当前代码只取 `Type == BlobObject` 的根级 files，所有嵌套文件丢失 → 被误判为 Untracked。

**修复**: 用 `TreeReader.ListBlobs(commitTree, "")` 获取完整递归文件列表。

```go
commitFiles := make(map[string]string)
if commitTree != nil {
    reader := NewTreeReader(storeReader)
    blobs, err := reader.ListBlobs(commitTree, "")
    if err == nil {
        for _, be := range blobs {
            commitFiles[be.Path] = be.Hash
        }
    }
}
```

**注意**: 需要给 `ComputeStatus` 传入 `StoreReader` 参数，或创建一个不需要 store 的递归展平函数。

---

### A2. `add.go` — 静默吞掉 LoadIndex 错误

**文件**: `internal/cli/add.go`

```go
_ = store.LoadIndex(&idx)
```

**问题**: 如果 index 文件存在但损坏（DRIX magic 不匹配、version 不对、截断），`LoadIndex` 返回 error 但被丢弃。后续 `idx.Add(entry)` 只写入新 entry，然后 `SaveIndex` 覆盖整个文件 → 原有 staging 数据全部丢失。

**修复**:
```go
if err := store.LoadIndex(&idx); err != nil {
    // index 文件损坏或不存在，从空 index 开始
    idx = core.Index{}
}
```

至少处理已知的 "file not exist" 错误：
```go
if err := store.LoadIndex(&idx); err != nil {
    if !errors.Is(err, ErrObjectNotFound) && !os.IsNotExist(err) {
        return fmt.Errorf("failed to load index: %w", err)
    }
    idx = core.Index{}
}
```

---

### A3. `tree.go` — NewTree 中 Marshal 错误被静默吞掉

**文件**: `internal/core/tree.go`

```go
func NewTree(entries []TreeEntry) *Tree {
    t := &Tree{Entries: entries}
    data, err := t.Marshal()
    if err != nil {
        return &Tree{Entries: entries, Hash: ""}  // 静默空 hash
    }
    t.Hash = calculateHash(data)
    return t
}
```

**问题**: hash 计算失败时返回空 `""`，调用者无法区分 "root tree with zero entries" 和 "hash computation failed"。后续 if `t.Hash == ""` 判断会误判。

**修复**: 返回 error 而不是静默：
```go
func NewTree(entries []TreeEntry) (*Tree, error) {
    t := &Tree{Entries: entries}
    data, err := t.Marshal()
    if err != nil {
        return nil, fmt.Errorf("failed to marshal tree: %w", err)
    }
    t.Hash = calculateHash(data)
    return t, nil
}
```

**影响范围**: `tree_builder.go` 中的 `buildTree` 调用处需同步修改。

---

### A4. `commit_codec.go` — parent hash "0" 被当作 no-parent sentinel

**文件**: `internal/core/commit_codec.go` Marshal 方法

```go
if c.Parent != "" && c.Parent != "0" {
    parentBytes, err = hexDecode(c.Parent)
} else {
    parentBytes = make([]byte, 32) // 32 零字节
}
```

**问题**: hex decode `"0"`（奇数 hex 字符串）失败，走 else 分支生成 32 零字节，与 "no parent" sentinel 相同。如果某人真的有一个 parent hash 中包含 `"0"` 字符（不是 hash 值本身），行为不可预测。

更脆弱的是：
```go
if err != nil {
    return nil, err
}
parentBytes = paddingHash(hashBytes, make([]byte, 32)) // padding 全零
```

hex decode 失败后 hashBytes 未初始化就传给 padding → 不确定行为。

**修复**: 使用明确的 isRoot 判断：
```go
func (c *Commit) isRoot() bool {
    return c.Parent == "" || c.Parent == "0000000000000000000000000000000000000000000000000000000000000000"
}
```

Sentinel 使用 64 个 `'0'` 字符的完整 hex 字符串，不用缩写 `"0"`。

---

### A5. `restore.go` — 文件内容比较用 string() 转换

**文件**: `internal/cli/restore.go`

```go
if string(existing) == string(data) {
```

**问题**: 对大文件（几百 MB 的 .psd / .blend 文件），这会：
1. 把两个 []byte 都转成 string → 各复制一份（Go 的 string 不可变，编译器可能优化但不可依赖）
2. 对 binary 大文件浪费大量内存
3. 如果两个 string 不相等，但实际上只是恢复操作，不需要比较，直接覆盖即可

**修复**: 用 `bytes.Equal(existing, data)` 替代。更进一步——如果已有 file 的 hash 和目标 blob hash 一致就跳过，不需要读内容：
```go
existingHash, _ := core.CalculateHashFromFile(targetPath)
if existingHash == entry.Hash {
    // skip, content unchanged
    continue
}
```

---

## Phase B：架构加固

### B1. 缺失 Storage 抽象层

**问题**: `store.go` 是唯一 concrete 实现，不利于测试和未来扩展。go-git 用 `EncodedObjectStorer` interface 抽象存储，支持 filesystem 和 memory 实现。

**建议**:

```
internal/storage/
  store.go       → Storage interface (SetBlob, GetBlob, SetTree, GetTree, ...)
  local.go       → LocalStorage 实现（当前代码）
  memory.go      → MemoryStorage 实现（测试用）
```

**接口设计**（最小集合）:
```go
type Storage interface {
    PutBlob(data []byte) (hash string, err error)
    PutBlobFromFile(path string) (hash string, err error)
    GetBlob(hash string) ([]byte, error)
    PutTree(t *core.Tree) error
    GetTree(hash string) (*core.Tree, error)
    PutCommit(c *core.Commit) error
    GetCommit(id string) (*core.Commit, error)
    ListCommits() ([]*core.Commit, error)
    SaveRef(name string, hash string) error
    GetRef(name string) (string, error)
    SaveIndex(idx *core.Index) error
    LoadIndex(idx *core.Index) error
}
```

**参考**: go-git `plumbing/storer/storer.go` 的三层 composable 接口模式。

---

### B2. Index 查找是 O(n) 线性搜索

**问题**: `Index.Entry()` 和 `Index.Has()` 遍历全部 entries。10 万条目时每次调用都是 10 万次比较。

**修复**: 增加 `map[string]int` 路径索引：
```go
type Index struct {
    Entries []IndexEntry
    byPath  map[string]int // path → entries index
}

func (idx *Index) Entry(path string) (IndexEntry, error) {
    if idx.byPath == nil {
        idx.buildIndex()
    }
    i, ok := idx.byPath[path]
    if !ok {
        return IndexEntry{}, ErrEntryNotFound
    }
    return idx.Entries[i], nil
}
```

或者参考 go-git 的 TREE extension，在 index 中维护目录结构以支持高效路径查找。

---

### B3. FileMode 未规范化

**问题**: 当前存储 `os.FileMode` 原始值。同一文件在 Windows/Linux/macOS 上 mode 不同 → 不同 hash → 重复存储。Windows 上的可执行位无意义。

go-git 的 `filemode.FileMode`: `Empty(0)`, `Dir(0040000)`, `Regular(0100644)`, `Executable(0100755)`, `Symlink(0120000)`, `Submodule(0160000)`

**修复**:
1. 在 `internal/core/filemode.go` 定义规范化的 mode 类型
2. `WalkWorkingDir` 中对每个文件做 `osMode → DriftMode` 转换
3. TreeEntry 存储规范化 mode

```go
func NormalizeMode(osMode os.FileMode) uint32 {
    if osMode.IsDir()     { return 0040000 }
    if osMode.IsRegular() { return 0100644 }
    return 0100644  // default to regular
}
```

---

### B4. Object 完整性无验证

**问题**: 读取 blobs/trees/commits 后没有校验 hash。磁盘位翻转/文件损坏不会被检测。

**修复**:
1. `GetBlob` — 读完后 `sha256.Sum256(data)` 与预期 hash 比较
2. `GetTree` — 读完后重算 hash 与文件名 hash 比较
3. `GetCommit` — 读完后 `calculateHash()` 与存的 `Hash` 字段比较
4. 校验失败返回 `ErrObjectCorrupted` 而非静默返回坏数据

---

### B5. 跨进程不安全

**问题**: `store.go` 用 `sync.Mutex` + sentinel 文件做锁，只在进程内有效。两个 drift 进程同时操作 `.drift/` 会竞态。

**修复**: 用 OS 级文件锁：
- Windows: `syscall.LockFileEx` 排他锁 `.drift/lock` 文件
- Unix: `syscall.Flock`

```go
func (s *Store) lock() error {
    f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
    if err != nil { return err }
    if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
        f.Close()
        return fmt.Errorf("another drift process is running: %w", err)
    }
    s.lockFile = f
    return nil
}
```

---

## Phase C：功能补全

### C1. Signature 模型（替代裸 string author）

go-git: `Signature{Name, Email, When time.Time}` — Commit 和 Tag 共用。

**实现**:
```go
type Signature struct {
    Name  string
    Email string
    When  time.Time
}
```

Commit 从 `Author string` 升级到 `Author Signature`。DCMT 格式需向后兼容处理（version bump 或 optional field）。

---

### C2. .driftignore 替代硬编码 dotfile skip

**问题**: `WalkWorkingDir` 无条件跳过所有 dotfile（包括 `.editorconfig`、`.gitignore` 等合理文件）。

go-git: `gitignore.NewMatcher(patterns)` + `collectIgnorePatterns()`。

**实现**:
1. 解析 `.driftignore` 文件（glob 规则，每行一条）
2. `WalkWorkingDir` 增加 ignore matcher 参数
3. 保留 `.drift/` 自身的硬跳过（安全）
4. 不再默认跳过 dotfile（由 `.driftignore` 显式配置）

---

### C3. Config 系统

**问题**: 没有配置读取能力（Init 写了 config.json 但不读）。

**需要配置项**:
- `user.name` / `user.email` — 用于 commit signature
- `core.ignorePatterns` — 替代硬编码 dotfile skip（或指向 `.driftignore`）
- `core.defaultBranch` — 默认分支名

**实现**: JSON config，与当前 Init 写入的 config.json 兼容。

---

### C4. Config Storer + 跨命令共享

**问题**: 每个 cli 命令都独立调用 `NewStore(dir)` 和 `IsInitialized()`。Config 和其他常用状态应该跨命令共享。

**实现**: RootCommand 的 `PersistentPreRunE` 中初始化 Store 和 Config，存入 context 或全局变量。

---

### C5. Branch / Switch 命令

当前 `save.go` 中的 branch 固定为 "main"。没有真正的分支管理。

**需要实现**:
- `drift branch` — 列出分支
- `drift branch <name>` — 创建新分支（基于当前 commit）
- `drift switch <name>` — 切换分支（类似 resetWorktreeToTree）
- `drift delete <name>` — 删除分支

### C6. Diff 命令

- `drift diff` — 工作区未暂存的改动 vs index（index 为空时 vs commit）
- `drift diff --staged` — index vs HEAD
- `drift diff <v1> <v2>` — 两个版本之间

---

## Phase D：性能与极端规模

### D1. 迭代器模式替代一次性全量加载

**问题**: `ListCommits()` 一次性加载所有 commit 到 `[]*Commit`。大历史项目会 OOM。

go-git 模式：
```go
type CommitIter interface {
    Next() (*Commit, error)
    ForEach(func(*Commit) error) error
    Close()
}
```

**实现**: 先在 `ListCommits` 上按需实现，再渐进迁移调用方。

### D2. 大文件操作的进度回调

**问题**: 大文件 export/restore/save 无进度反馈。用户不知道操作是否在进行。

**实现**: 增加 `ProgressReporter` callback：
```go
type ProgressReporter func(total, current int64, description string)
```

对 `PutBlobFromFile`、`export`、`restore` 等操作增加进度回调。

### D3. 树比较并行化

`DiffTrees` 当前是串行递归。大文件树下可以并行遍历子树。

### D4. Mmap 大文件读取

对非常大的文件（>100MB），用 mmap 替代 `os.ReadFile` 避免全量复制。

---

## 修复执行计划

| Phase | 编号 | 任务 | 优先级 | 状态 | 预计影响范围 |
|-------|------|------|--------|------|-------------|
| A | A1 | commitFiles 递归展平 | **P0** | ✅ 已完成 | `change.go` |
| A | A2 | LoadIndex 错误处理 | **P0** | ✅ 已完成 | `add.go` |
| A | A3 | NewTree 返回 error | **P1** | ✅ 已完成 | `tree.go`, `tree_builder.go` |
| A | A4 | parent hash sentinel 健壮化 | **P1** | ✅ 已完成 | `commit_codec.go`, `commit.go` |
| A | A5 | bytes.Equal 替代 string() | **P2** | ✅ 已完成 | `restore.go` |
| B | B1 | Storage interface | P1 | ✅ 已完成 | 新增 `storage.go` |
| B | B2 | Index byPath map 索引 | P1 | ✅ 已完成 | `index.go` |
| B | B3 | FileMode 规范化 | P1 | ✅ 已完成 | 新增 `filemode.go`, `walker.go` |
| B | B4 | Object 完整性校验 | P2 | ✅ 已完成 | `store.go` |
| B | B5 | OS 级文件锁 | P2 | ✅ 已完成 | 新增 `lock.go`, `lock_windows.go`, `lock_unix.go` |
| C | C1 | Signature 模型 | P2 | ✅ 已完成 | `commit.go`, `commit_codec.go` |
| C | C2 | .driftignore | P2 | ✅ 已完成 | 新增 `driftignore.go`, 更新 `walker.go` |
| C | C3 | Config 系统 | P2 | ✅ 已完成 | 新增 `config/config.go` |
| C | C4 | Config Storer 跨命令 | P2 | ✅ 已完成 | `root.go` + 所有 CLI 文件 |
| C | C5 | Branch/Switch | P2 | ✅ 已完成 | 新增 `branch.go`, `switch.go` |
| C | C6 | Diff 命令 | P2 | ✅ 已完成 | 新增 `diff.go` |
| D | D1 | 迭代器模式 | P3 | ⏸ 推迟 | 提交>1000时再做 |
| D | D2 | 进度回调 | P3 | ✅ 已完成 | `progress.go`, `export.go` |
| D | D3 | 并行 diff | P3 | ⏸ 推迟 | 性能瓶颈时再做 |
| D | D4 | Mmap 大文件 | P3 | ⏸ 推迟 | 文件>1GB时再做 |
