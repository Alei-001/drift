# Drift - 技术设计文档

## 技术栈

| 项 | 选择 | 原因 |
|----|------|------|
| **语言** | Go 1.26+ | 编译为单一二进制、跨平台 |
| **哈希** | SHA-256 | 安全性高，纯摘要（不兼容 Git header） |
| **存储** | 内容寻址 + 二进制格式 | 去重、性能 |
| **CLI** | cobra | Go 标准 CLI 库 |
| **压缩** | zlib（tar.gz） | 仅 export 使用，存储不压缩 |

## 架构

### 对象模型

借鉴 Git 三种核心对象，做了适配创意工作者的简化：

```
Blob    — 文件内容（二进制 / 文本），SHA-256 寻址
Tree    — 目录结构，递归（每个子目录是独立 Tree 对象）
Commit  — 版本快照，指向根 Tree，单亲节点（线性历史）
```

```
Commit
  └── Tree（根目录）
        ├── Tree（子目录1）
        │     ├── Blob（文件1）
        │     └── Blob（文件2）
        ├── Tree（子目录2）
        │     └── Blob（文件3）
        └── Blob（文件4）
```

## 目录结构

```
.drift/
├── objects/
│   ├── blobs/          # SHA-256 前 2 位分片
│   │   ├── ab/
│   │   │   └── cdef... # 原始内容
│   │   └── 56/
│   │       └── 7890...
│   └── trees/          # *.dre（DREE 格式）
│       └── abcdef....dre
├── commits/            # *.dcm（DCMT 格式）
│   ├── v1.dcm
│   └── v2.dcm
├── refs/               # 分支引用（JSON）
│   └── main.json
├── index               # 暂存区（DRIX 格式）
├── config.json         # 项目配置
└── lock                # 文件锁标记
```

## 二进制格式

所有持久化数据使用二进制格式（`encoding/binary` + `encoding/hex`，little-endian）。

### DRIX — Index（暂存区）

```
Header:  magic[4]="DRIX" | version:uint32(1) | count:uint32
Entry:   path_len:uint16 | path:[]byte
         hash:[32]byte（SHA-256）
         modified_at:int64（Unix ms）
         size:int64
         mode:uint32
```

### DREE — Tree（目录）

```
Header:  magic[4]="DREE" | count:uint32
Entry:   name_len:uint16 | name:[]byte
         type:uint8（0=blob, 1=tree）
         hash:[32]byte
         mode:uint32
```

排序：子目录先于文件，同类型按名称字母序（保证确定性哈希）。

### DCMT — Commit（版本）

```
Header:  magic[4]="DCMT" | version:uint32(1)
Field:   id_len:uint16 | id:[]byte
         hash:[32]byte
         tree_hash:[32]byte
         parent_hash:[32]byte（全零 = 无父版本）
         timestamp:int64（Unix ms）
         branch_len:uint16 | branch:[]byte
         message_len:uint16 | message:[]byte
         author_name_len:uint16 | author_name:[]byte
         author_email_len:uint16 | author_email:[]byte
```

## 内容寻址

- 相同内容 → 相同 SHA-256 → 同一存储位置 → 自动去重
- 原子写入：先写 `.tmp` 文件，再 `os.Rename`
- Blob 分片存储：`hash[:2]` 子目录避免单目录文件过多

## 哈希算法

纯 SHA-256，不添加 Git 风格的 type+size header。

```
CalculateHash(data) = hex(sha256.Sum256(data))  → 64 字符 hex string
```

## 跨平台

### FileMode 规范化

已实现。存储时将 `os.FileMode` 归一化为标准值：

| 平台原始 | 归一化值 | 含义 |
|----------|----------|------|
| 各种 Regular | `0100644` | 普通文件 |
| 各种 Dir | `0040000` | 目录 |
| 有执行位的 Regular | `0100755` | 可执行文件 |

详见 `internal/core/filemode.go`。

### 换行符归一化

计划实现（当前未做）：

```
PutBlobFromFile  → 读取文件时 CRLF → LF 归一化
GetBlob          → 写入工作区时 LF → CRLF（Windows）/ 保持 LF（Unix）
```

确保同一文件在 Windows / macOS / Linux 上哈希一致。

### 文件锁

已实现 OS 级排他锁，防止多实例并发操作 `.drift/`：

- Windows: `LockFileEx`（`internal/storage/lock_windows.go`）
- Unix: `flock`（`internal/storage/lock_unix.go`）

详见 `internal/storage/lock.go`。

### 路径处理

- 内部统一使用 `/` 分隔符
- 读写文件系统时用 `filepath.FromSlash` / `filepath.ToSlash` 转换
- 路径长度上限：65535 字节（二进制格式 uint16 限制）

## 核心模块

### 对象存储（`internal/storage/store.go`）

- `PutBlob(data)` / `GetBlob(hash)` — 内容寻址 Blob 读写
- `PutBlobFromFile(path)` — 从文件系统流式写入，自动 CRLF→LF
- `PutTree(t)` / `GetTree(hash)` — DREE 格式读写
- `PutCommit(c)` / `GetCommit(id)` — DCMT 格式读写
- `ListCommits()` — 时间戳升序列表
- `SaveRef` / `GetRef` — 分支引用
- `SaveIndex` / `LoadIndex` — 暂存区

### 状态检测（`internal/core/change.go`）

三层比较模型（参考 go-git）：

```
暂存区有内容：Commit ↔ Index（staging 状态） + Index ↔ Workdir（worktree 状态）
暂存区为空：  Commit ↔ Workdir（worktree 状态）

Untracked：既不在 Commit 也不在 Index 中的文件
```

### 目录遍历（`internal/core/walker.go`）

基于 `filepath.Walk`，跳过 `.drift/` 和 `.git/`。支持 `.driftignore` 文件进行 glob 规则过滤。

### Tree 构建（`internal/core/tree_builder.go`）

从扁平 Index 构建递归 Tree：
1. 拆分路径为层级
2. 自底向上构建子树
3. 每个目录作为独立 Tree 对象存储
4. 父目录引用子目录的 Tree hash

## 性能考虑

### 大文件

- 流式计算哈希（`io.Copy` + `sha256.New()`），不一次性加载到内存
- 进度回调：大文件操作时通知用户进度（`internal/core/progress.go`）
- 内容对比用 `bytes.Equal`，避免 `string()` 内存复制

### Index 查找

已优化为 O(1) 查找。使用 `map[string]int` 路径索引（`internal/core/index.go`）。

## 安全

### 数据完整性

- 写入原子性：`tmp + Rename`
- 哈希校验：读取 Blob 和 Tree 时验证内容 hash（`internal/storage/store.go`）
- 暂存区保护：`restore` 检测非空暂存区，需 `--force`

### 并发安全

- OS 级文件锁：`flock`（Unix）/ `LockFileEx`（Windows）
- 详见 `internal/storage/lock.go`, `lock_windows.go`, `lock_unix.go`
