# drift 架构评审与重设计方案

---

## 目录

1. [前言](#1-前言)
2. [项目概述](#2-项目概述)
3. [架构合理性评估](#3-架构合理性评估)
4. [技术方案与算法评估](#4-技术方案与算法评估)
5. [与 Git 及 Git LFS 的全面对比](#5-与-git-及-git-lfs-的全面对比)
6. [重设计方案](#6-重设计方案)
7. [变更优先级与迁移路径](#7-变更优先级与迁移路径)
8. [总结](#8-总结)

---

## 1. 前言

本文档从三个维度对 drift 项目进行专业评审：

- **架构合理性**：分层设计、接口抽象、关注点分离
- **技术选型**：哈希算法、分块策略、压缩方案、序列化格式等的正确性
- **与 Git/Git LFS 的差异**：数据模型、存储策略、二进制文件处理等核心设计对比

基于以上评审，提出一份面向未来的架构重设计方案。

---

## 2. 项目概述

drift 是一个面向创作者（作家、插画师、设计师）的版本控制系统。与 Git 不同，它：

- **无暂存区**：`drift save` 直接捕捉工作树全部变更
- **无合并冲突**：分支是纯分叉，用户自行选择合并方式
- **内容定义分块（CDC）**：FastCDC 算法将文件切分为内容寻址的 chunk，修改 1% 的内容只重存 1% 的数据
- **可插拔文件类型引擎**：文本（统一 diff）、图片（格式/尺寸/色彩对比）、视频（容器格式检测）、二进制（fallback）
- **单二进制零服务端**：WebDAV/SMB 协议即可作为远程存储，无需 Git 服务端或 LFS 服务器

---

## 3. 架构合理性评估

### 3.1 分层架构 — 8/10

**现状**：

```
cmd/                  CLI（cobra 命令，无业务逻辑）
internal/
  porcelain/          业务逻辑：snapshot、branch、restore、lock、watch、gc
  filetype/           可插拔类型引擎：text、image、video、binary
  chunker/            FastCDC + FixedSize 分块
  storage/            Storer 接口 + 共享常量
    backends/
      filesystem/     磁盘 .drift/ 实现
      memory/         内存实现（测试）
    refname/          引用名称校验
    stream/           chunk 流式读写工具
  remote/             远程同步（WebDAV/SMB）
  core/               领域类型：Hash、Chunk、Snapshot、FileEntry、Config
  util/               工具：缓存、文件系统、路径处理
  version/            版本元数据 + 自升级
```

**正确之处**：

1. `cmd/` 与 `internal/` 在物理层面严格隔离。`internal/` 下所有包不可被外部项目 import，CLI 是唯一的公共 API 表面。
2. 依赖方向单向：`cmd → porcelain → {filetype, chunker, storage, remote} → core → util`。不存在循环依赖。
3. `remote/` 包设计了与存储后端的平级关系——它依赖 `storage/` 的**共享常量**（`layout.go`、`chunk_format.go`），但**绝不** import `storage/backends/filesystem`，保证了协议无关性。
4. 文件类型引擎的注册顺序显式声明在 `init.go` 中（text → image → video → binary），避免隐式顺序带来的未定义行为。

**结构性问题**：

#### 问题 1：Storer 上帝接口（严重）

`internal/storage/storer.go` 定义了一个组合 6 个子接口 + `io.Closer` 的"上帝接口"：

```go
type Storer interface {
    ChunkStorer      // 5 methods
    SnapshotStorer   // 4 methods
    ReferenceStorer  // 4 methods
    IndexStorer      // 2 methods
    PreviewStorer    // 2 methods
    ConfigStorer     // 2 methods
    io.Closer        // 1 method
}
// 总计约 20 个方法
```

**影响**：

| 方面 | 影响 |
|------|------|
| 接口隔离原则 | 违反。调用 `pushChunk` 只需 `ChunkStorer`，却被强制依赖全部 20 个方法 |
| 新增后端 | 实现门槛极高。即使是测试 mock，也必须提供 20 个方法骨架 |
| 单元测试 | 无法按需 mock。试想测试 `isAncestor`（只需要 `SnapshotStorer`），需要构造完整的 `Storer` |
| 编译检查 | 编译器无法提示"此函数只需要 chunk 操作"，失去接口驱动的架构约束 |

**已有证据**：子接口 `ChunkStorer`、`SnapshotStorer`、`ReferenceStorer` 等已存在且定义清晰。porcelain 和 remote 层的函数签名完全可以改为接受具体子接口，而非捆绑传递 `Storer`。

#### 问题 2：PreviewStorer 是空实现

两个存储后端均实现了空的 `GetPreview` / `PutPreview`：

```go
// filesystem/preview.go
func (fs *FSStorage) GetPreview(ctx context.Context, hash core.Hash, size int) ([]byte, error) {
    return nil, storage.ErrNotFound  // 始终返回未找到
}
func (fs *FSStorage) PutPreview(ctx context.Context, hash core.Hash, size int, data []byte) error {
    return nil  // 空操作
}
```

代码注释标注为"Phase 1"，但该接口已出现在 `Storer` 中。**要么实现预览缓存，要么从接口体系中移除，直到就绪。**

#### 问题 3：ChunkCompactor 游离于接口体系外

`ChunkCompactor` 是一个独立接口（`internal/storage/compactor.go`），**不在 `Storer` 中**：

```go
type ChunkCompactor interface {
    CompactChunks(ctx context.Context, reachable map[core.Hash]bool, dryRun bool) (CompactReport, error)
}
```

GC 代码（`porcelain/gc.go:258`）通过类型断言获取它：

```go
if compactor, ok := store.(storage.ChunkCompactor); ok {
    cr, err := compactor.CompactChunks(ctx, reachableChunks, dryRun)
    // ...
} else {
    // fallback: per-chunk DeleteChunk
}
```

**这是一个有意为之的可选接口模式，类似于 `io.WriterTo` 是 `io.Writer` 的可选扩展。** 但问题是：类型断言绑定在具体 `Storer` 上，而非独立注入。如果引入缓存中间件或装饰器，类型断言可能穿透装饰层失败。更稳健的做法是在构建阶段显式注入 Compactor。

#### 问题 4：缓存逻辑嵌入存储后端

`FSStorage` 在构造函数中硬编码了 256 条 LRU chunk 缓存：

```go
func NewFSStorage(root string) (*FSStorage, error) {
    chunkCache, _ := cache.NewCache[core.Hash, *core.Chunk](256)
    // ...
    return &FSStorage{chunkCache: chunkCache, ...}, nil
}
```

**影响**：

- Memory 后端无法受益于缓存
- 缓存大小不可配置（低资源设备 64 条、大服务器 1024 条）
- 无法禁用缓存用于特定场景（如 CI 中的一次性 restore）
- 缓存策略与持久化存储混在同一个类型中，违反单一职责

#### 问题 5：io.Closer 嵌入接口

```go
type Storer interface {
    // ...
    io.Closer
}
```

Memory 后端的 `Close()` 是空操作。更 Go 惯用的做法是调用方单独断言：

```go
if closer, ok := store.(io.Closer); ok {
    defer closer.Close()
}
```

### 3.2 数据模型 — 7/10

**Snapshot 模型**（`internal/core/snapshot.go`）：

```go
type Snapshot struct {
    ID        SnapshotID
    PrevID    *SnapshotID   // nil = 初始快照
    Message   string
    Author    string
    Timestamp int64
    Files     []FileEntry   // 直接嵌入文件列表
    Tags      []string
    TotalSize int64
}
```

**优点**：

- 单向链表（`PrevID`）而非 DAG，强制线性历史，消除了 merge commit 和 merge conflict
- 扁平化路径：`FileEntry.Path` 存储字符串路径，不存在树结构。对于文件数通常有限的创作者项目是合理的简化

**可改进之处**：

当前 `Snapshot` 直接包含 `[]FileEntry`（全部文件路径+chunk 引用）。一个含 10,000 个文件的快照，元数据可能有数百 KB。为了 `ListSnapshots` 时不必加载完整文件列表，项目额外引入了 **manifest** 机制：

```
snapshot → 大文件（全部 FileEntry）
manifest → 小文件（message + author + timestamp + file_count，不含文件列表）
```

这是**两份数据维护同一个快照**，引入了多余的写入和一致性风险。更干净的设计见 [§6.4](#64-snapshottree-分离)。

### 3.3 并发与锁 — 8/10

**Workspace Lock**（`internal/porcelain/lock.go`）实现了两阶段获取：

1. **快速路径**：`O_CREATE | O_EXCL` 原子创建锁文件
2. **回收路径**：通过 claim file + 原子 rename 回收过期锁，关闭 TOCTOU 窗口

这是跨进程锁的正确实现方式。代码注释详细记录了为何简单的"检查 mtime + 删除 + 创建"存在竞态，以及 claim file 如何消除它。

**Watch Daemon**（`internal/porcelain/watch_daemon.go`）采用子进程+文件 IPC：

```
drift watch on
    └─ exec(drift, "_watch_daemon")  → 子进程
         ├─ PID 文件：watch.pid
         └─ JSON 状态文件：watch.state（轮询 pause/resume 标志）
```

**优点**：简单、无额外依赖、`os.Executable()` 获取当前二进制路径（不信任 `$PATH`）。

**可改进之处**：

- pause/resume 的响应延迟最多为 `interval`（默认 300 秒），因为是轮询
- 无心跳机制：PID 存在 ≠ 进程功能正常（daemon hang 住但不崩溃时无法检测）
- 唯一的 IPC 是信号（SIGTERM 停止）和文件轮询，缺乏即时命令通道

### 3.4 测试 — 8/10

| 目录 | 测试文件 | 测试代码行数 |
|------|:-------:|:----------:|
| `internal/porcelain/` | 25 | 6,515 |
| `internal/storage/backends/filesystem/` | 6 | 1,709 |
| `internal/remote/` | 7 | 1,623 |
| **合计** | **38** | **9,847** |

**优点**：

- 核心路径（save、restore、GC、push/pull）均有覆盖
- 使用标准库 `testing`，无第三方断言库
- Memory 后端作为测试双，避免临时目录
- 快照回滚测试和 restore 回滚测试确保了关键操作的安全性

---

## 4. 技术方案与算法评估

### 4.1 BLAKE3 — 10/10

| 维度 | 评估 |
|------|------|
| 速度 | x86-64 上比 SHA-256 快 10–20 倍（SIMD 并行树哈希） |
| 输出大小 | 32 字节（256 bits），碰撞抵抗充分 |
| 跨平台 | `zeebo/blake3` 是纯 Go 实现，无 CGO，编译零摩擦 |
| 替代方案 | SHA-256 太慢；XXH3 不是密码学哈希（碰撞风险不可接受）；BLAKE2b 是老一代 |

**结论**：在内容寻址存储中，这是没有争议的最佳选择。

### 4.2 FastCDC — 9/10

| 参数 | 值 | 评价 |
|------|-----|------|
| min size | 128 KB | 合理。太小→chunk 过多、元数据开销大；太大→去重粒度粗 |
| avg size | 256 KB | 合理。2 的幂满足 FastCDC 库约束。1GB 文件约 4,000 个 chunk |
| max size | 512 KB | 合理。防止单 chunk 过大导致 restore 内存峰值 |

`go-cdc-chunkers` 的 `fastcdc-v1.0.0` 变体动态计算掩码（而非旧版硬编码 8KB 假设），切割点准确。

**分层策略**（`chunker/strategy.go`）：

| 文件大小 | 策略 | 评价 |
|----------|------|------|
| < 50 MB | FastCDC 128K/256K/512K | ✓ 标准 |
| 50 MB – 500 MB | FastCDC ×4（512K/1M/2M） | ✓ 减少 chunk 数量 |
| ≥ 500 MB | **FixedChunker** 2MB | ⚠️ 失去 CDC 去重 |

**关键问题：≥500MB 的文件使用 `FixedChunker`，失去内容寻址去重能力。**

`FixedChunker` 按固定偏移切分，chunk 边界不依赖于内容。如果你有一个 600 MB 的 `.psd` 文件，在开头插入一个图层再保存，**所有 chunk 的偏移都会改变、所有 chunk 的哈希都会改变**。drift 会存储一份全新的 600 MB 数据，而非只存变化的几个 chunk。

**替代方案**：将 FastCDC 参数进一步放大（例如 avg=2MB、max=4MB），保留变长分块的去重能力。4MB avg 对 10GB 文件仅产生约 2,500 个 chunk，元数据开销约 110KB，完全可接受。代价是 chunk 粒度过粗，增量传输（push/pull）的粒度变大。

### 4.3 zstd — 9/10

| 对比维度 | zstd level 3 | gzip level 6 | 差异 |
|----------|:-----------:|:-----------:|------|
| 文本压缩比 | ~2.5× | ~2.7× | gzip 略高 8%，但 zstd 快 3–5 倍 |
| 二进制压缩比 | ~1.3× | ~1.2× | zstd 略优 |
| 压缩速度 | ~200 MB/s | ~25 MB/s | zstd 快 ~8 倍 |
| 解压速度 | ~500 MB/s | ~100 MB/s | zstd 快 ~5 倍 |

**额外亮点**：

- `buildChunkPayload` 比较压缩前后大小，**如果压缩没有变小就存储原始数据**。防止已压缩文件（JPEG、MP4、zip）反而膨胀。
- `maxDecompressedChunkSize = 64MB` 防止压缩炸弹攻击。
- `MarshalOptions{Deterministic: true}` 确保同一输入产生相同的序列化输出，保证哈希稳定性。

**性能瓶颈**：zstd encoder 用 `sync.Mutex` 保护而非 `sync.Pool`。虽然 `klauspost/compress/zstd.Encoder.EncodeAll` 是并发安全的，但当前设计下所有 worker 排队等锁。批量 save 时，CPU 有 8 个核但压缩吞吐只有 1 核。

### 4.4 protobuf — 8/10

**为什么是 protobuf**：

| 替代方案 | 拒绝理由 |
|----------|----------|
| JSON | map 键序非确定 → 相同内容两次 marshal 可能产生不同的字节 → 内容寻址哈希不稳定 |
| MessagePack | 无 schema、同样有 map 键序问题 |
| 自定义二进制 | 需手写编解码器、易出 bug、无 schema 演进 |

**正确用法**：

- `MarshalOptions{Deterministic: true}` 强制 map 字段按 key 排序，保证哈希可复现
- `SnapshotToProto(s, withIDHash=false)` 在计算快照 ID 时排除 `IdHash` 字段，避免循环依赖（ID 需要哈希，哈希的计算需要排除 ID）
- `proto.Clone()` 而非值拷贝，防止 copylocks（protobuf 内部有 `sync.Mutex`）

**已知限制**（代码自行注释承认）：Go protobuf 库不支持流式解码，`GetSnapshot` 必须 `io.ReadAll` 全部字节再 `Unmarshal`。超大快照的峰值内存是 raw + decoded 两倍。

### 4.5 Myers 差分算法（文本 diff）— 8/10

Myers 是 `git diff` 使用的同款算法，复杂度 O(ND)，D 是编辑距离。

**做得好**：

- `maxDiffLines` 阈值：超大文件回落到 "files differ" 而非跑 O(N×M) DP
- `linesEqual` 快速路径：相同内容不进 Myers
- `ErrLineTooLong`：单行超过 1MB 视为非文本（likely binary/minified）
- `scanLines` 自定义 split 支持 `\r\n`、`\n`、`\r` 三种换行符

### 4.6 Pack 格式 — 8/10

当 loose chunk 累积到 ≥512 个时，compact 将之打包：

```
.pack 文件：[chunk1 载荷][chunk2 载荷]...
.idx 文件： DPID(4B) | version(1B) | count(4B) | [hash(32B) | offset(8B) | length(4B) | flags(1B)]×count
                                            └────────── 45 bytes each ──────────┘
```

**优点**：

- 固定宽度 index entry（45 bytes），可随机访问
- 读取时需 BLAKE3 重哈希校验 + 文件边界校验
- OOM 防护：`maxPackEntryLength=64MB`、`maxPackIndexEntries=1M`
- 包重写阈值：dead ratio ≥ 50%（`packRewriteRatio=0.5`）才触发

---

## 5. 与 Git 及 Git LFS 的全面对比

### 5.1 数据模型

```
Git                          drift

commit                     snapshot
  ├─ tree                    ├─ file_entry[]  (扁平路径字符串)
  │   ├─ blob (file A)       │   ├─ chunk_hash[]
  │   └─ blob (file B)       │   └─ chunk_hash[]
  └─ parent commit(s)         └─ prev_id  (单向链表)
```

| 维度 | Git | drift |
|------|-----|-------|
| 内容单元 | blob = 整个文件 | chunk = 文件片段（128KB–2MB） |
| 目录结构 | tree 对象显式建模层级 | FileEntry.Path 扁平字符串 |
| 版本历史 | DAG（每个 commit 可有多个 parent） | 单向链表（单个 PrevID） |
| 元数据 | commit 含 author/committer/date/message/parents | snapshot 含 author/message/timestamp/PrevID |
| 去重粒度 | 文件级 | chunk 级 |

drift 的扁平路径是**合理的简化**：Git 的 tree 对象面向源码仓库（数十万文件、目录级权限和重命名跟踪），drift 面向创作者项目（通常几十到几千个文件）。

drift 的单向链表是**刻意的设计选择**：不支持 merge 即不支持 merge conflict。代价是丧失了 Git 的分布式协作能力（pull request、分支合并、cherry-pick）。

### 5.2 存储与去重

```
Git：                           drift：

blob（完整文件）                  chunk（128KB-2MB 片段）
  ↓ zlib 压缩                     ↓ zstd 压缩
  ↓ delta 压缩（repack 时）        ↓ 内容寻址自动去重（实时）
loose object → pack file        loose chunk → pack file
```

**去重对比**：

```
文件 A v1："hello world"        文件 A v1："hello"" worl""d\x00"
文件 A v2："hello WORLD"        文件 A v2："hello"" WORL""D\x00"
                                 相同的 chunk "hello" 只存一份

Git：两个独立 blob，repack 时做 delta 压缩    drift：CDC 实时自动跨版本去重
```

| 维度 | Git | drift | 优劣 |
|------|-----|-------|------|
| 跨版本去重 | repack 时的 delta 压缩（事后、批处理） | CDC 实时（保存时就自动发生） | drift 优 — 不需要手动 `git gc` |
| 跨文件去重 | **无**（每个 blob 独立） | 自动（不同文件中相同片段共享 chunk） | drift 优 — 复制的图片/段落自动共享存储 |
| 存小文件 | 1 blob = 1 个 loose 文件 | 可能 1 chunk = 1 个文件 | 相当 |
| 改大文件 1% | 存整个文件（delta 压缩在 repack 后才生效） | 只存 1% 的变化 chunk | drift 显著优 |
| 压缩算法 | zlib（1995 年标准） | zstd（2016 年标准） | drift 优 — 速度 5–8 倍，压缩比接近 |

### 5.3 大文件处理 — drift vs Git LFS

这是**最具架构差异的对比维度**。

```
Git LFS 方案：                         drift 方案：

photo.psd (实际文件)                   photo.psd (实际文件)
    │                                      │
    ├─ 指针文件 → Git (.git/objects)        └─ FastCDC 分块
    │   内容："version https://...         → chunk1, chunk2, chunk3
    │          oid sha256:abc123                  │
    │          size 52428800"                     ▼
    │                                    .drift/chunks/（内容寻址 + zstd）
    └─ 真实文件 → LFS Server
        .git/lfs/objects/（本地缓存）
```

| 维度 | Git LFS | drift |
|------|---------|-------|
| **安装门槛** | 需安装 git-lfs 扩展 + 配置 LFS 服务端 | **单二进制，零额外依赖** |
| **指针文件** | 仓库中有 130 bytes 的指针文件 | **无指针文件**，元数据在 snapshot 中 |
| **增量存储** | ❌ 整个文件级别。改 1 字节 → 重新上传整个文件 | ✅ chunk 级别。改 1 字节 → 只存变化的 1-2 个 chunk |
| **跨文件去重** | ❌ 两个路径的相同文件存两份 | ✅ 相同内容 = 相同 chunk 哈希 = 存一份 |
| **服务端依赖** | **必须有 LFS 服务器**（GitHub LFS / GitLab LFS / 自建） | **零服务端**。WebDAV/SMB 即可。NAS、Nextcloud 都能做 remote |
| **锁定机制** | LFS file locking API（防止多人同时编辑二进制文件） | 只有 workspace lock（防止并发 drift 命令冲突），无文件级锁 |
| **离线工作** | 需先 `git lfs fetch` 下载大文件才能看到内容 | **本地优先**，`.drift/` 包含所有内容 |
| **clone 速度** | `git clone` 只拉指针（快）+ `git lfs pull` 拉大文件（慢） | `drift clone` 拉全部内容（首次慢，有去重） |
| **历史清理** | 困难。`git filter-branch` / BFG 操作复杂 | GC 自动回收不可达 chunk |
| **成熟度** | 工业级，被 GitHub/GitLab/Bitbucket 原生支持 | 新项目，社区为零 |

**Git LFS 的根本问题是它是"外挂"**：

1. **指针文件污染历史**：将 100MB 文件迁移到 LFS 后，历史中仍存在原始 blob，需要用 `git filter-branch` 或 BFG Repo-Cleaner 清理
2. **两份存储、两个生命周期**：Git 对象在 `.git/objects/`，LFS 对象在 `.git/lfs/objects/`（本地缓存）+ LFS 服务端。GC 只回收 Git 对象，LFS 有独立的清理逻辑
3. **外挂意味着永久的零钱**：每台机器需安装 `git-lfs` 二进制，每个仓库需 `.gitattributes` 配置追踪规则

drift 是**内建**方案：大小文件走相同的存储路径，没有指针文件，没有外挂，没有第二套对象生命周期。

### 5.4 适用场景总结

| 场景 | 推荐 | 原因 |
|------|:----:|------|
| 个人创作者（作家/画师/设计师） | **drift** | 零配置、无 merge、二进制友好 |
| 大文件频繁修改 | **drift** | CDC chunk 级增量 |
| 多个文件有重复内容 | **drift** | CDC 跨文件自动去重 |
| 用 NAS 做版本控制 | **drift** | WebDAV 即可，无需 Git 服务端 |
| 团队协作源码 | **Git** | 分布式协作、merge/rebase、CI/CD |
| 需要文件锁 | **Git LFS** | LFS lock API |
| 现有 Git 项目 | **Git** | 迁移成本极高 |
| 稀疏检出 / partial clone | **Git** | `--filter=blob:none` |

---

## 6. 重设计方案

### 设计原则

1. **接口即契约**：调用者依赖最小接口，而非全能对象
2. **去重是核心承诺**：无论文件多大，CDC 不能退化
3. **关注点分离到物理层**：缓存、压缩、IPC 各自独立
4. **单二进制零服务端**：保持核心差异化优势

---

### 6.1 StoreSet 替代 Storer 上帝接口

**现状问题**（见 [§3.1 问题 1](#问题-1storer-上帝接口严重)）：`Storer` 是组合 7 个子接口的上帝接口。

**重设计**：

```go
// === 独立接口（保留，不变） ===
type ChunkStore interface {
    HasChunk(context.Context, Hash) (bool, error)
    GetChunk(context.Context, Hash) (*Chunk, error)
    PutChunk(context.Context, *Chunk) error
    DeleteChunk(context.Context, Hash) error
    ListChunks(context.Context) ([]Hash, error)
}

type SnapshotStore interface {
    GetSnapshot(context.Context, SnapshotID) (*Snapshot, error)
    PutSnapshot(context.Context, *Snapshot) error
    DeleteSnapshot(context.Context, SnapshotID) error
    ListSnapshots(context.Context, *ListOptions) ([]*SnapshotSummary, error)
}

type RefStore interface { /* GetRef, SetRef, ListRefs, DeleteRef */ }
type IndexStore interface { /* GetIndex, SetIndex */ }
type ConfigStore interface { /* GetConfig, SetConfig, SetCompressionConfig */ }

// === Compactor 作为独立可选接口 ===
type Compactor interface {
    CompactChunks(ctx context.Context, reachable map[Hash]bool, dryRun bool) (CompactReport, error)
}

// === StoreSet：传递便利，不强制实现 ===
type StoreSet struct {
    Chunks    ChunkStore
    Snapshots SnapshotStore
    Refs      RefStore
    Index     IndexStore
    Config    ConfigStore
    Compactor Compactor  // nil 表示不支持压缩（如 Memory 后端）
}

// 工厂函数：自动检测 Compactor 支持
func NewStoreSet(backend interface{}) *StoreSet {
    s := &StoreSet{
        Chunks:    backend.(ChunkStore),
        Snapshots: backend.(SnapshotStore),
        Refs:      backend.(RefStore),
        Index:     backend.(IndexStore),
        Config:    backend.(ConfigStore),
    }
    if c, ok := backend.(Compactor); ok {
        s.Compactor = c
    }
    return s
}
```

**调用方变更**：

```go
// 之前（上帝接口）
func Push(ctx context.Context, store Storer, rfs RemoteFS, ...) (*SyncStats, error)

// 之后（按需依赖）
func Push(ctx context.Context, chunks ChunkStore, snapshots SnapshotStore,
          refs RefStore, rfs RemoteFS, ...) (*SyncStats, error)
```

```go
// 之前（类型断言获取 Compactor）
func CollectGarbage(ctx context.Context, store Storer, ...) (GCReport, error) {
    if compactor, ok := store.(storage.ChunkCompactor); ok {
        // ...
    }
}

// 之后（显式注入，nil 安全）
func CollectGarbage(ctx context.Context, s *StoreSet, ...) (GCReport, error) {
    if s.Compactor != nil {
        cr, err := s.Compactor.CompactChunks(ctx, reachable, dryRun)
        // ...
    } else {
        // fallback: per-chunk deletion
    }
}
```

**收益**：

- 新增 S3 后端只需实现 5 个独立接口（不需要 Compactor 就不实现，字段留 nil）
- Mock 测试：mock `ChunkStore`（5 个方法）而非整个 `Storer`（20 个方法）
- `Compactor` 是显式 nil 检查，编译器不会遗漏
- 后续可独立演进每个子接口

---

### 6.2 CDC 全覆盖 — 永不退化到固定分块

**现状问题**（见 [§4.2](#42-fastcdc--910)）：≥500MB 文件使用 `FixedChunker`，失去内容寻址去重。

**重设计**：

```
文件大小               当前方案                    重设计方案
─────────────────────────────────────────────────────────────────
< 50 MB              FastCDC 128K/256K/512K     FastCDC 128K/256K/512K     ✓ 不变
50 MB – 200 MB       FastCDC ×4                 FastCDC 256K/512K/1M       ✓ CDC 保留
200 MB – 1 GB        FastCDC ×4                 FastCDC 512K/1M/2M         ✓ CDC 保留
≥ 1 GB               FixedChunker 2MB ❌         FastCDC 1M/2M/4M           ✓ CDC 保留
```

```go
func ChunkerFor(fileSize int64) Chunker {
    switch {
    case fileSize < 50<<20:
        return NewFastCDC(128<<10, 256<<10, 512<<10)
    case fileSize < 200<<20:
        return NewFastCDC(256<<10, 512<<10, 1<<20)
    case fileSize < 1<<30:
        return NewFastCDC(512<<10, 1<<20, 2<<20)
    default:
        return NewFastCDC(1<<20, 2<<20, 4<<20)
        // 10GB 文件 → ~2,500 个 chunk → ~110KB 元数据 → 可接受
    }
}
```

**trade-off**：4MB avg chunk，10GB 文件约 2,500 个 chunk，push/pull 时每条 chunk 都是一个 HTTP 请求。如果未来这成为瓶颈，可引入**两级 chunk**：

```
文件 → CDC(128KB) → 小 chunk 序列 → 按 4MB 分批次 → 批次哈希 = BLAKE3(c1 || c2 || ...)
```

外层大块用于 I/O（push/pull），内层小块用于去重。复杂度显著增加，保留为未来优化项。

---

### 6.3 缓存中间件

**现状问题**（见 [§3.1 问题 4](#问题-4缓存逻辑嵌入存储后端)）：LRU 缓存硬编码在 `FSStorage` 中。

**重设计**：

```go
// cache/chunk_cache.go — 独立的缓存装饰器
type CachedChunkStore struct {
    inner ChunkStore
    cache *lru.Cache[Hash, *Chunk]
}

func NewCachedChunkStore(inner ChunkStore, size int) *CachedChunkStore {
    cache, _ := lru.New[Hash, *Chunk](size)
    return &CachedChunkStore{inner: inner, cache: cache}
}

func (c *CachedChunkStore) GetChunk(ctx context.Context, h Hash) (*Chunk, error) {
    if ch, ok := c.cache.Get(h); ok {
        return CloneChunk(ch), nil
    }
    ch, err := c.inner.GetChunk(ctx, h)
    if err != nil {
        return nil, err
    }
    c.cache.Add(h, ch)
    return CloneChunk(ch), nil
}

func (c *CachedChunkStore) PutChunk(ctx context.Context, ch *Chunk) error {
    if err := c.inner.PutChunk(ctx, ch); err != nil {
        return err
    }
    c.cache.Add(ch.Hash, ch)
    return nil
}
// HasChunk, DeleteChunk, ListChunks 透传
```

**使用**：

```go
// 生产环境
fs := filesystem.New(root)
s.Chunks = cache.NewCachedChunkStore(fs, 256)

// 测试环境（可选择无缓存或更小的缓存）
mem := memory.New()
s.Chunks = cache.NewCachedChunkStore(mem, 64)

// CI 环境（无缓存，减少内存占用）
s.Chunks = fs
```

**收益**：缓存大小可配置、可独立测试、可应用于任意后端。

---

### 6.4 Snapshot/Tree 分离

**现状问题**：`Snapshot` 直接包含 `[]FileEntry`，导致 `ListSnapshots` 不得不引入额外的 manifest 文件来避免加载完整文件列表。

**重设计**：借鉴 Git 的 commit/tree 分离，但保留扁平化路径形式。

```go
// FileTree 独立存储，有独立哈希
type FileTree struct {
    ID      Hash          // BLAKE3(entries 的确定性序列化)
    Entries []FileEntry
}

// Snapshot 引用 FileTree
type Snapshot struct {
    ID        Hash
    PrevID    *Hash
    TreeID    Hash      // 指向 FileTree
    Message   string
    Author    string
    Timestamp int64
}
```

```
存储布局：
.drift/
  snapshots/      → 快照元数据（< 1KB：message + author + tree_id）
  trees/          → 文件列表（数十 KB：所有文件路径 + chunk 引用）
  chunks/         → 文件内容（GB 级：实际数据）
```

**收益**：

- `ListSnapshots` 不再需要 manifest backfill —— 快照文件本身就小
- 同一文件列表被多个快照共享（如 auto-save 只改一个文件）时，`FileTree` 也自动去重
- 消除了 `manifest.go` 的全部冗余逻辑
- Diff 两个快照：比较 `TreeID`，不同则加载两个 `FileTree`。不需加载 chunk 数据

#### 为什么保留扁平路径

Git 使用树形对象（commit → tree → subtree → ... → blob），每个目录一个 tree 对象。drift 的 `FileEntry.Path` 是完整路径字符串（如 `"images/photos/sunset.png"`），而非拆分为层级 tree。这个选择是刻意的：

**目标用户的项目规模不同**。创作者的仓库通常是几百到两三千个文件（文档、图片、视频），而非代码仓库的数万源码文件。在这个规模下：

```
项目规模              扁平 FileTree                     Git 式 tree 层级
─────────────────────────────────────────────────────────────────────────
100 个文件           1 个 20KB FileTree                ~10 个 tree 对象 + 1 commit
                     改 1 文件 → 重写 20KB             改 1 文件 → 重写 ~1 个 tree + commit ✓

1,000 个文件         1 个 200KB FileTree               ~50 个 tree 对象
                     改 1 文件 → 重写 200KB            改 1 文件 → 重写 ~1-2 个 tree ✓
                                                       但多了 50 个小对象的存储开销

10,000 个文件        1 个 2MB FileTree ❌               ~200 个 tree 对象
                     改 1 文件 → 重写 2MB              改 1 文件 → 重写 ~5 个 tree ✓
```

**分界点约在 5,000 个文件**。低于此数，扁平 FileTree 的简单性（无递归遍历、无 tree diff 逻辑、无深层空目录对象爆炸）胜出。超过此数，树形结构的增量共享收益开始超越复杂度成本。

#### 未来扩展路径

如果 drift 的用户出现 10,000+ 文件的项目，可以按以下路径逐步演进，**无需破坏现有存储格式**：

**方案 A：分页 FileTree（低复杂度）**

```go
// 大型 FileTree 内部自动分组，每组 1000 个条目
type FileTree struct {
    Pages []Hash  // 每页是 FilePage{Entries: [1000]FileEntry} 的哈希，按路径排序
}
```

改 1 个文件只需重建其所属的 1 页（~200KB → ~200 bytes）。其余 9 页哈希不变，跨快照共享。

**方案 B：路径 Merkle Trie（中复杂度）**

```go
// 每个 trie 节点按路径前缀分组
type TrieNode struct {
    Children map[string]Hash  // 子路径 → 子 TrieNode 的哈希
    File     *FileEntry        // 路径结束于此节点的文件
}
```

```
路径：["a/b/c.txt", "a/b/d.txt", "a/e.txt", "f.txt"]

    TrieNode_Root  ← 快照引用
    ├─ "a" → TrieNode_A
    │    ├─ "b" → TrieNode_AB
    │    │    ├─ "c.txt" → FileEntry
    │    │    └─ "d.txt" → FileEntry
    │    └─ "e.txt" → FileEntry
    └─ "f.txt" → FileEntry
```

改 1 个深层文件只需重建 O(depth) 个 trie 节点。比 Git tree 更优的地方在于：它按前缀分组而非按目录分组，一个大而浅的目录（`/` 下 5000 个文件）会被自动拆分为前缀子节点，每个只管理约 200 个条目。

**方案 C：Git 式层级 Tree（高复杂度）**

完全模仿 Git 的 tree/blob 模型。对 drift 来说是过度设计——Git 需要 tree 对象是因为它要支持 `git log -- <path>` 的目录级过滤和 `git checkout <subtree>`。drift 无此需求。

**推荐路径**：当前用扁平 FileTree（简单，覆盖 95% 实际场景）。当出现 5,000+ 文件的项目时，将 FileTree 内部实现替换为分页式（方案 A），对外接口不变——`Snapshot.TreeID` 和 `FileTree` 的组合对调用方透明，纯内部优化。

---

### 6.5 zstd Pool 替代 Mutex

**现状问题**（见 [§4.3](#43-zstd--910)）：单一 `sync.Mutex` 保护 zstd encoder，并发 save 时所有 worker 排队。

**重设计**：

```go
var zstdEncoderPool = sync.Pool{
    New: func() interface{} {
        enc, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
        return enc
    },
}

func compressChunk(data []byte) ([]byte, bool) {
    enc := zstdEncoderPool.Get().(*zstd.Encoder)
    defer zstdEncoderPool.Put(enc)
    compressed := enc.EncodeAll(data, nil)
    if len(compressed) >= len(data) {
        return nil, false  // 压缩没变小，存原始数据
    }
    return compressed, true
}
```

`EncodeAll` 是并发安全的（每个 goroutine 用自己的 encoder），不需要任何锁。N 个 worker → N 路并行压缩。

---

### 6.6 Watch Daemon — Socket IPC 替代文件轮询

**现状问题**（见 [§3.3](#33-并发与锁--810)）：pause/resume 轮询延迟 300s，无心跳检测，无即时命令通道。

**重设计**：Unix Domain Socket IPC。

```
drift watch on ────────────────────────────────────────────┐
                                                           │
  exec(drift, "_watch_daemon")                              │
       │                                                   ▼
       │                                   ┌──────────────────────────┐
       │                                   │  Daemon 进程              │
       │    Unix Socket                    │                          │
       │  /tmp/drift-watch-<hash>.sock     │  ticker → save loop      │
       │                                   │                          │
       │                                   │  goroutine:              │
       ▼                                   │   listen on socket       │
  drift watch status ────► socket ──────► │   ┌──────────────────┐   │
  drift watch pause  ────► socket ──────► │   │ status_handler   │   │
  drift watch resume ────► socket ──────► │   │ pause_handler    │   │
  drift watch off    ────► socket ──────► │   │ resume_handler   │   │
                                          │   │ shutdown_handler │   │
                                          │   └──────────────────┘   │
                                          └──────────────────────────┘
```

```go
// Daemon 侧
type Daemon struct {
    mu       sync.Mutex
    paused   bool
    listener net.Listener
    saves    int
    lastBeat atomic.Int64   // Unix timestamp
}

func (d *Daemon) Serve(ctx context.Context) {
    for {
        conn, err := d.listener.Accept()
        if err != nil {
            return
        }
        go d.handleConn(conn)
    }
}

func (d *Daemon) handleConn(conn net.Conn) {
    defer conn.Close()
    var req Request
    json.NewDecoder(conn).Decode(&req)
    switch req.Command {
    case "status":
        resp := Response{
            Running: true, Paused: d.paused,
            Saves: d.saves, LastBeat: d.lastBeat.Load(),
        }
        json.NewEncoder(conn).Encode(resp)
    case "pause":
        d.mu.Lock(); d.paused = true; d.mu.Unlock()
        json.NewEncoder(conn).Encode(Response{OK: true})
    case "resume":
        d.mu.Lock(); d.paused = false; d.mu.Unlock()
        json.NewEncoder(conn).Encode(Response{OK: true})
    case "shutdown":
        json.NewEncoder(conn).Encode(Response{OK: true})
        d.cancel()  // 触发主循环退出
    }
}
```

```go
// CLI 侧
func PauseDaemon(cwd string) error {
    conn, _ := net.Dial("unix", socketPath(cwd))
    defer conn.Close()
    json.NewEncoder(conn).Encode(Request{Command: "pause"})
    var resp Response
    json.NewDecoder(conn).Decode(&resp)
    if !resp.OK {
        return fmt.Errorf("daemon refused pause")
    }
    return nil
}
```

**收益**：

- pause/resume **微秒级响应**（不再有 300s 延迟）
- 自带心跳：`lastBeat` 每次 tick 更新，status 检查心跳新鲜度
- 优雅关闭：发 shutdown 命令而非 `kill`
- 可扩展：未来加 `force-save`、`reload-config` 等命令只需加 case 分支

---

### 6.7 远程同步索引协商

**现状问题**：push/pull 前通过多次 `Stat` 或 `List` 检查 chunk 是否存在。

**重设计**：

```
Push 流程：
  local                           remote (WebDAV/SMB)
    │                                │
    │  GET .drift/remote.idx         │
    │ ◄───────────────────────────── │  （所有已有 chunk hash + 分支 tip）
    │                                │
    │  差集 = 本地 - remote           │
    │                                │
    │  PUT 差集 chunks（批量并发）      │
    │ ──────────────────────────────►│
    │                                │
    │  PUT .drift/remote.idx（更新）   │
    │ ──────────────────────────────►│
```

```go
type RemoteIndex struct {
    Version  int
    Updated  int64
    Chunks   []Hash              // 所有已有 chunk 的哈希
    Branches map[string]Hash     // 分支名 → tip hash
}
```

**收益**：一次 `GET` 取代 N 次 `Stat`/`List`，对大仓库效果显著。

---

### 6.8 引擎特定 CDC 参数

**现状问题**：所有二进制文件使用同一套 CDC 参数，未区分格式特性。

**重设计**：

```go
// 图片引擎：细粒度，捕获小范围修改
func (e *ImageEngine) ChunkerFor(fileSize int64) chunker.Chunker {
    if fileSize < 50<<20 {
        return chunker.NewFastCDC(64<<10, 64<<10, 128<<10)
    }
    return chunker.NewFastCDC(256<<10, 512<<10, 1<<20)
}

// 视频引擎：粗粒度，H.264 已过帧间压缩
func (e *VideoEngine) ChunkerFor(fileSize int64) chunker.Chunker {
    return chunker.NewFastCDC(512<<10, 1<<20, 2<<20)
}
```

---

### 6.9 目录结构

```
cmd/drift/                   CLI 入口

internal/
  core/                      领域类型（Hash, Chunk, Snapshot, FileTree, ...）

  store/                     存储接口定义
    chunk.go                 ChunkStore 接口
    snapshot.go              SnapshotStore 接口
    ref.go                   RefStore 接口
    index.go                 IndexStore 接口
    config.go                ConfigStore 接口
    compact.go               Compactor 接口
    storeset.go              StoreSet 聚合结构体

  store/filesystem/          FS 后端实现
  store/memory/              Memory 后端实现
  store/cache/               缓存中间件（装饰任意 ChunkStore）

  chunker/                   CDC 策略（无 FixedChunker）

  engine/                    文件类型引擎
    engine.go                Engine 接口
    registry.go              注册表
    text/                    文本引擎
    image/                   图片引擎
    video/                   视频引擎
    binary/                  二进制引擎（fallback）

  snapshot/                  快照业务逻辑
    save.go                  CreateSnapshot
    restore.go               RestoreSnapshot
    diff.go                  DiffSnapshots
    undo.go                  Undo

  branch/                    分支管理
  sync/                      远程同步
  gc/                        垃圾回收
  watch/                     daemon（含 socket server）

  remote/                    远程协议实现
    remotefs.go              RemoteFS 接口
    webdav/                  WebDAV 实现
    smb/                     SMB 实现

  util/                      工具（不变）
```

**核心变化**：

- `porcelain/` 拆分为 `snapshot/`、`branch/`、`sync/`、`gc/`、`watch/` 5 个子包，减少单包大小
- `store/` 只定义接口，实现放 `store/filesystem/` 和 `store/memory/`
- `store/cache/` 是独立中间件
- `engine/` 取代 `filetype/`
- PreviewStorer 在未实现前暂不定义

---

## 7. 变更优先级与迁移路径

| 优先级 | 变更 | 影响范围 | 向后兼容 | 建议时机 |
|:------:|------|----------|:--------:|----------|
| **P0** | StoreSet 替代 Storer 上帝接口 | 所有包 | ❌ 破坏性接口变更 | 尽快，v0.x 阶段 |
| **P0** | Compactor 显式注入，消除类型断言 | GC、存储后端 | ❌ 签名变化 | 同上 |
| **P1** | 移除 FixedChunker，CDC 全覆盖 | `chunker/strategy.go` | ✅ 内部实现 | 随时 |
| **P1** | Snapshot/Tree 分离 | 核心数据模型 | ❌ 存储格式不兼容 | 有迁移计划时 |
| **P1** | zstd Pool 替代 Mutex | FS 后端 | ✅ 内部实现 | 随时 |
| **P1** | 缓存中间件抽离 | FS 后端 + 新 `cache/` 包 | ✅ 可选增强 | 随时 |
| **P2** | Watch socket IPC | daemon + CLI | ❌ IPC 协议变化 | 功能成熟后 |
| **P2** | 远程索引协商 | `remote/` | ✅ 可选增强 | 大仓库场景驱动 |
| **P2** | 引擎特定 CDC 参数 | 各引擎 `ChunkerFor` | ✅ 内部实现 | 按需 |
| **P2** | porcelain 子包拆分 | 所有依赖方 | ❌ 包路径变化 | v0.x 阶段 |
| **P3** | Preview 实现或移除 | 接口 + 后端 | ❌ 接口变化 | 决策后 |

---

## 8. 总结

### 8.1 项目优势

1. **技术选型合理**：BLAKE3（速度+安全最佳平衡）、FastCDC（正确的 CDC 算法）、zstd（现代压缩标杆）、protobuf（schema 演进稳妥）
2. **分层架构清晰**：CLI → business → engine → chunker → storage → core → util，依赖方向单向
3. **安全防护充分**：写入前哈希校验、解压炸弹保护（64MB）、原子写入（temp + rename）、TOCTOU 安全的跨进程锁
4. **测试覆盖可观**：9,847 行测试代码，核心路径均有覆盖
5. **内存和磁盘都做到了正确的哨兵错误 + `errors.Is` 分类**
6. **代码注释质量高**：每个导出符号都有 `doc comment`，关键设计决策有详尽注释

### 8.2 需改进的架构问题

| 问题 | 严重程度 | 影响 |
|------|:--------:|------|
| Storer 上帝接口 | **高** | 新增后端门槛高、测试 mock 困难、违反接口隔离 |
| ≥500MB FixedChunker 失去 CDC | **中** | 违背"内容去重"核心承诺 |
| PreviewStorer 空实现 | **中** | 接口承诺未兑现的技术债 |
| zstd Mutex 串行压缩 | **中** | 并发 save 吞吐瓶颈 |
| Watch daemon 轮询 IPC | **低** | 功能可用但延迟高、缺心跳 |
| 缓存嵌入后端 | **低** | 可配置性差，但功能正常 |

### 8.3 与 Git 的定位差异

drift 不是 Git 的替代品，而是为**不同用户群体**设计的版本控制工具：

- Git = 分布式团队代码协作（DAG 历史、merge、pull request、CI/CD 集成）
- drift = 个人创作者二进制文件管理（CDC 去重、零 staging area、单二进制零服务端）
- Git LFS = 大文件的外挂解决方案（指针文件、独立服务端、文件级粒度）

**在 drift 的目标场景中（个人创作者 + 大二进制文件），其架构设计优于 Git，特别是 CDC 跨文件去重和单二进制零服务端这两点是 Git 无法提供的核心竞争力。**

---
