# drift 编码规则

本文档定义 drift 项目的编码规范，旨在保证代码风格一致、可读性强、易于维护。

---

## 1. 通用 Go 规范

### 1.1 代码格式化

- 所有代码必须通过 `gofmt`（或 `goimports`）格式化，不得手动调整对齐
- 提交前运行 `go vet ./...`，确保无静态分析警告
- 导入分组：标准库 → 第三方库 → 项目内部库，组间空一行

```go
import (
    "context"
    "fmt"
    "os"

    "github.com/klauspost/compress/zstd"
    "github.com/your-org/drift/core"
    "github.com/your-org/drift/storage"
)
```

### 1.2 命名规范

| 元素 | 规范 | 示例 |
|------|------|------|
| 包名 | 小写单词，不使用下划线，简洁有意义 | `chunker`, `filesystem`, `fsutil` |
| 导出标识符 | PascalCase，不使用缩写（除非是业界通用缩写如 ID、URL） | `ChunkStore`, `SnapshotID`, `GetChunk` |
| 非导出标识符 | camelCase | `chunkCache`, `zstdDecoder`, `putChunk` |
| 常量 | PascalCase（导出）或 camelCase（非导出），不使用 ALL_CAPS | `ChunkFlagCompressed`, `binaryLargeThreshold` |
| 接口 | 单方法接口以 `-er` 结尾 | `Storer`, `Chunker`, `Previewer`, `Detector` |
| 接收器 | 短名称（1-2字符），同类型方法保持一致 | `func (fs *FSStorage) GetChunk(...)` |
| 布尔值 | `Is`/`Has`/`Can` 前缀 | `IsSymRef()`, `HasChunk()`, `compress bool` |

### 1.3 注释规范

- **所有导出标识符必须有注释**，以被注释的名称开头
- 注释使用完整句子，末尾加句号
- 非导出函数/类型在逻辑复杂处添加注释，简单逻辑不必赘述
- 不要添加无意义的注释（如 `i++ // increment i`）

```go
// ChunkFlag represents flags for a chunk.
type ChunkFlag uint8

// PutChunk stores a chunk in the content-addressed storage.
// If the chunk already exists, it returns nil without error.
func (fs *FSStorage) PutChunk(ctx context.Context, chunk *core.Chunk) error {
```

---

## 2. 项目分层规范

drift 采用严格的 7 层架构，依赖方向只能从上到下，禁止反向依赖：

```
cmd/ → porcelain/ → filetype/ → chunker/ → storage/ → core/
                                      ↘         ↙
                                       util/
```

### 2.1 各层职责

| 层 | 目录 | 职责 | 禁止事项 |
|----|------|------|---------|
| CLI | `cmd/` | 参数解析、输出格式化、命令路由 | 不包含业务逻辑，不直接操作 storage |
| Porcelain | `porcelain/` | 业务逻辑编排、工作区锁、高级操作 | 不实现分块算法，不直接操作磁盘文件路径 |
| Filetype | `filetype/` | 文件类型检测、类型特定 Chunker/Differ/Previewer | 不依赖 porcelain，不直接调用 storage |
| Chunker | `chunker/` | CDC/Fixed 分块算法 | 不依赖 filetype/porcelain/storage |
| Storage | `storage/` | 持久化接口与实现、缓存、序列化 | 不包含业务逻辑，不依赖 porcelain/filetype/chunker |
| Core | `core/` | 核心数据结构、接口、错误定义 | 不依赖任何其他项目层 |
| Util | `util/` | 通用工具（缓存、文件操作、日志等） | 不依赖 core 以上的任何层 |

### 2.2 接口定义位置

- 存储接口定义在 `storage/` 包中（如 `ChunkStorer`、`ReferenceStorer`）
- 文件类型引擎接口定义在 `filetype/` 包中（如 `Engine`、`Detector`）
- 分块器接口定义在 `chunker/` 包中（如 `Chunker`）
- 核心数据类型定义在 `core/` 包中

实现接口的具体类型（如 `filesystem.FSStorage`）在子包中，通过 `var _ storage.Storer = (*FSStorage)(nil)` 进行编译时接口检查。

---

## 3. 错误处理规范

### 3.1 错误包装

- 使用 `fmt.Errorf("context: %w", err)` 包装错误，保留原始错误链
- 每层添加上下文信息，从内到外：操作 → 对象 → 原因
- sentinel 错误定义在对应的包中，使用 `errors.New`

```go
// storage/errors.go
var (
    ErrNotFound   = errors.New("drift: not found")
    ErrCorrupted  = errors.New("drift: data corrupted")
    ErrLocked     = errors.New("drift: locked by another process")
    ErrInvalidRef = errors.New("drift: invalid reference")
)

// 正确：添加上下文
func (fs *FSStorage) GetChunk(hash core.Hash) (*core.Chunk, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, fmt.Errorf("get chunk %x: %w", hash[:8], storage.ErrNotFound)
        }
        return nil, fmt.Errorf("read chunk %x: %w", hash[:8], err)
    }
}
```

### 3.2 禁止事项

- **禁止** 吞掉错误（`err != nil { return nil }` 或 `_ = err`）
- **禁止** 使用 `panic` 处理可预期的错误（仅在不可恢复的编程错误时使用）
- **禁止** 在日志中记录错误后又返回同一错误（造成重复日志）

```go
// 错误：吞掉错误
if err != nil {
    log.Println(err)
    return nil  // 缺少错误返回
}

// 正确
if err != nil {
    return nil, fmt.Errorf("do something: %w", err)
}
```

### 3.3 错误判断

- 外部调用方使用 `errors.Is(err, storage.ErrNotFound)` 判断错误类型
- 使用 `errors.As(err, &target)` 获取自定义错误类型
- 不要通过字符串匹配判断错误

---

## 4. 函数与方法规范

### 4.1 函数签名

- 参数数量控制在 5 个以内，超过时考虑使用 Option 模式或配置结构体
- 同类参数放在一起（如 `ctx context.Context` 始终作为第一个参数）
- 返回值顺序：结果 → 错误（`(*Result, error)`）
- `context.Context` 作为第一个参数传递给所有可能涉及 I/O 或需要取消的函数

```go
// 好
func (fs *FSStorage) PutChunk(ctx context.Context, chunk *core.Chunk) error

// 避免：参数过多
func SaveProject(path, message, author string, compress, preview bool, chunkMin, chunkAvg, chunkMax int) error
```

### 4.2 接收器命名

- 使用 1-2 个字符的短名称，与类型名称相关
- 同一类型的所有方法使用相同的接收器名称
- 不要使用 `this` 或 `self`

```go
type FSStorage struct { ... }
func (fs *FSStorage) GetChunk(...) { ... }  // ✓ 一致
func (f *FSStorage) PutChunk(...) { ... }   // ✗ 与上面不一致
```

### 4.3 函数长度

- 单个函数不超过 50 行（不含注释和空行）
- 超过时拆分为多个小函数，每个函数只做一件事
- 复杂逻辑提取为命名良好的辅助函数

---

## 5. 并发与锁规范

### 5.1 锁原则

- drift 使用**单一层级的工作区锁**（`.drift/workspace.lock`），由 porcelain 层管理
- 存储层**不加锁**：内容寻址存储（CAS）天然无写竞争（同哈希同内容）
- 不要引入新的锁文件或锁机制，除非有充分理由

### 5.2 Goroutine 与 Channel

- 导出的同步函数在内部管理 goroutine 生命周期，不向调用方暴露 goroutine
- watch 守护进程是唯一的长期运行 goroutine，通过 PID 文件管理生命周期
- 使用 `context.Context` 控制 goroutine 取消
- 通道容量为 0（无缓冲）或 1（信号量），避免使用大缓冲通道

---

## 6. 存储 I/O 规范

### 6.1 文件操作

- 所有文件写入使用原子操作：写临时文件 → `Sync()` → `Rename()`
- 项目提供 `util/fsutil.WriteFileAtomic()` 工具函数，必须使用它
- 不要直接使用 `os.WriteFile` 写入关键数据文件

```go
// 正确
if err := fsutil.WriteFileAtomic(path, data, 0644); err != nil {
    return fmt.Errorf("write config: %w", err)
}

// 错误：非原子写入
if err := os.WriteFile(path, data, 0644); err != nil {
```

### 6.2 大文件处理

- **禁止** 将大文件（>1MB）完整读入内存
- 使用流式 `io.Reader`/`io.Writer` 逐块处理
- 分块读取、分块写入、分块计算哈希，内存占用不超过一个 chunk 的大小

```go
// 正确：流式写入
func writeFileChunks(path string, chunks []*core.Chunk, store storage.ChunkStorer) error {
    f, err := os.Create(path)
    if err != nil {
        return err
    }
    defer f.Close()
    for _, h := range chunks {
        ch, err := store.GetChunk(ctx, h)
        if err != nil {
            return err
        }
        if _, err := f.Write(ch.Data); err != nil {
            return err
        }
    }
    return nil
}
```

### 6.3 路径处理

- 使用 `filepath.Join()` 拼接路径，不要手动拼接 `/` 或 `\`
- 使用 `filepath.ToSlash()` 统一内部路径表示
- 路径比较使用 `filepath.Clean()` 规范化后再比较

---

## 7. 数据结构规范

### 7.1 Core 类型

- `core/` 中的类型是项目的"数据契约"，字段设计应稳定审慎
- 时间戳统一使用 `int64`（Unix 时间戳），不使用 `time.Time`（避免时区和序列化问题）
- 可选字段使用指针类型（如 `*SnapshotID`, `*FileMetadata`），nil 表示不存在
- 切片初始化为空切片（非 nil），避免 JSON 序列化产生 `null`

```go
// 正确
type Snapshot struct {
    ID        SnapshotID
    PrevID    *SnapshotID  // 可选，首个快照为 nil
    Timestamp int64        // Unix 时间戳
    Files     []FileEntry  // 非 nil 空切片
}

index := &core.Index{
    Entries: []core.IndexEntry{},  // 不是 nil
}
```

### 7.2 标志位与枚举

- 标志位使用 `uint8` 类型的位标志，常量显式赋值
- 枚举类型使用自定义类型 + 常量，不使用 `iota` 表示需要序列化的枚举（避免重排导致值变化）

```go
// 标志位：显式值
type ChunkFlag uint8
const (
    ChunkFlagNone       ChunkFlag = 0
    ChunkFlagCompressed ChunkFlag = 1
)

// 枚举类型：string 类型，JSON 友好
type RefType string
const (
    RefTypeBranch RefType = "branch"
    RefTypeTag    RefType = "tag"
    RefTypeHead   RefType = "HEAD"
)
```

---

## 8. 配置规范

### 8.1 配置传递

- 配置从 `core.Config`/`core.CoreConfig` 结构读取，不要使用全局变量
- `OpenProject()` 返回完整配置对象，向下传递给需要的函数
- `chunker.BinaryChunkerFor(fileSize, cfg)` 接受配置参数，使用默认值处理 nil
- 文件类型引擎的 `ChunkerFor(fileSize, cfg)` 同样接受配置

### 8.2 默认值

- 默认值统一定义在 `core.DefaultConfig()` 中
- 各层在使用配置时，对零值/负值进行防御性处理，回退到默认值

```go
func (c *CoreConfig) ZstdLevel() int {
    if c.CompressionLevel < 1 {
        return 3
    }
    if c.CompressionLevel > 19 {
        return 19
    }
    return c.CompressionLevel
}
```

---

## 9. 测试规范

### 9.1 测试组织

- 测试文件与源文件同目录，命名为 `*_test.go`
- 单元测试使用 `testing` 标准库 + `github.com/stretchr/testify/assert`
- 表驱动测试优先，测试用例覆盖边界条件

```go
func TestBinaryChunkerFor(t *testing.T) {
    tests := []struct {
        name     string
        fileSize int64
        cfg      *core.CoreConfig
        wantType reflect.Type
    }{
        {"small file", 10 * 1024, nil, reflect.TypeOf(&FastCDCChunker{})},
        {"large file", 100 * 1024 * 1024, nil, reflect.TypeOf(&FastCDCChunker{})},
        {"huge file", 600 * 1024 * 1024, nil, reflect.TypeOf(&FixedChunker{})},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            c := BinaryChunkerFor(tt.fileSize, tt.cfg)
            assert.IsType(t, tt.wantType, c)
        })
    }
}
```

### 9.2 测试存储

- 单元测试使用 `storage/memory` 内存存储，不依赖磁盘
- 需要测试文件系统实现时使用 `t.TempDir()` 创建临时目录

```go
func TestFSStorage_PutGet(t *testing.T) {
    dir := t.TempDir()
    store, err := filesystem.NewFSStorage(dir)
    assert.NoError(t, err)
    defer store.Close()
    // ... 测试逻辑
}
```

### 9.3 测试覆盖

- 核心路径（分块、存储、快照创建/恢复）必须有测试
- Bug 修复必须添加对应的回归测试
- 运行 `go test ./...` 必须全部通过

---

## 10. 跨平台规范

### 10.1 平台特定代码

- 使用文件名后缀区分平台特定代码：`*_windows.go`, `*_unix.go`
- 平台特定实现提供相同的函数签名，通过构建标签选择
- 不要在共享代码中使用 `runtime.GOOS` 判断平台

```go
// watch_proc_windows.go
//go:build windows

func processExists(pid int) bool {
    handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
    // ...
}

// watch_proc_unix.go
//go:build !windows

func processExists(pid int) bool {
    process, err := os.FindProcess(pid)
    // ...
}
```

### 10.2 路径与换行

- 内部路径统一使用 `/` 分隔（`filepath.ToSlash()`），仅在与 OS API 交互时使用 `filepath.Join()`
- 文本文件写入时不强制换行符，遵循 Go 标准库的默认行为

---

## 11. 日志规范

- 使用标准库 `log/slog` 进行结构化日志
- 日志级别：`Debug`（开发诊断）、`Info`（关键操作）、`Warn`（可恢复问题）、`Error`（失败）
- 日志消息简短明确，使用键值对附加上下文，不使用字符串格式化拼接消息

```go
slog.Info("snapshot created",
    "id", snap.ShortID(),
    "files", len(snap.Files),
    "duration", time.Since(start),
)

slog.Error("failed to read chunk",
    "hash", hash.FullString(),
    "error", err,
)
```

---

## 12. 提交规范

- 提交信息使用英文，符合 Conventional Commits 规范
- 格式：`<type>(<scope>): <subject>`
- Type 类型：
  - `feat`: 新功能
  - `fix`: 修复 bug
  - `refactor`: 重构（不改变功能）
  - `perf`: 性能优化
  - `test`: 测试相关
  - `docs`: 文档更新
  - `chore`: 构建/工具/依赖更新

```
feat(chunker): add configurable FastCDC parameters from CoreConfig
fix(storage): set ChunkFlagCompressed when writing compressed chunks
refactor(porcelain): remove redundant storage lock, use workspace lock only
docs(architecture): update disk layout and HEAD symref format
```
