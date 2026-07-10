# Drift — 产品需求文档

## 1. 项目概述

Drift 是一款面向**普通创作者**的通用版本控制软件 —— 面向写作、绘画、设计等创意工作场景。与专为程序员和纯文本设计的 Git 不同，Drift 强调简单易用，支持图片/PSD 等富媒体文件的格式感知存储和可视化浏览。

## 2. 要解决的问题

| 痛点 | 影响 |
|------|------|
| Git 将所有文件视为不透明二进制 | 200MB 的 PSD 只改了一个图层，就得存储完整副本 |
| Git 的心智模型面向开发者 | 暂存区、提交、合并 —— 对非程序员没有意义 |
| 没有可视化时间线 | 无法用缩略图浏览历史；diff 只显示原始字节 |
| 合并冲突 | 没有安全、直观的方式同时试验多个方向 |
| CLI 优先的设计 | 需要记住大量命令和参数 |

## 3. 目标用户

- **写作者**：小说、剧本、学术论文作者
- **画师**：插画师、概念设计师、漫画作者
- **设计师**：平面设计师、UI/UX 设计师（PSD/AI/Sketch）
- **独立创作者**：管理自有素材和迭代版本的个体创作者
- **小型创意团队**（第五阶段）：协作设计迭代

## 4. 产品目标

### 阶段一（当前） —— 本地核心

- 高效存储任意文件的每个版本（CDC 分块 + BLAKE3 去重）
- 通过简洁的表格浏览历史
- 文本文件 unified diff 对比
- 恢复到任意历史版本，恢复前自动备份

### 阶段二 —— 分支与自动化

- 创建和切换实验性分支
- 文件监听自动保存
- 基于模式的文件忽略

### 阶段三 —— 富文件类型引擎

- 图片格式感知分块（go-vips）
- PSD 图层级分块
- 缩略图生成为可视化时间线准备
- 大文件优化（>100MB 定长分块）

### 阶段四 —— 桌面 GUI

- 带缩略图的可视化时间线
- 图片并排/叠加视觉对比
- 拖拽恢复
- 零命令行操作

### 阶段五 —— 远程同步

- 块级增量同步到远程存储
- 可选的端到端加密
- S3 / IPFS / 自建服务端支持

## 5. 核心设计决策

| 决策 | 理由 |
|------|------|
| **无暂存区** | `save` 自动捕获所有变更，降低认知负担 |
| **无 merge / rebase** | 分支是纯分叉用于实验，用户手动合并 |
| **内容寻址存储（BLAKE3）** | 自动去重；通过哈希验证完整性 |
| **CDC 分块（FastCDC）** | 大文件只存储变化的块，二进制文件也能去重 |
| **zstd 压缩** | 混合文件类型下速度快、压缩比好 |
| **Protobuf 序列化** | 高效的快照和索引存储，schema 可演进 |
| **先 CLI，后 GUI** | 先验证存储引擎，CLI 用于自动化；GUI 提供可视化 |
| **主分支名：main** | 简单通用的约定 |

## 6. 功能需求

### FR-1：项目初始化

- `drift init [path]` 创建 `.drift/` 目录结构
- 自动生成默认配置文件
- 创建 `.driftignore` 默认忽略系统临时文件

### FR-2：快照创建（save）

- 扫描工作区变更（新增/修改/删除）
- 使用 FastCDC 对文件进行内容定义分块
- BLAKE3 哈希去重后存储
- zstd 压缩每个块
- 存储快照元数据：消息、时间戳、作者、标签
- 更新分支引用和工作区索引

### FR-3：历史浏览（log）

- 表格格式：ID | 时间 | 消息 | 变更数
- `-c` 紧凑模式，适合脚本处理
- `-v <id>` 详细模式，显示单个快照的文件变更明细（A/M/D）
- 默认过滤自动保存的快照，`--all` 包含全部

### FR-4：状态查看（status）

- 对比工作区与上次索引
- 以 A（新增）、M（修改）、D（删除）标记变更
- 显示距上次保存的时间

### FR-5：文件查看（show）

- 文本文件直接输出内容
- 二进制文件显示类型和大小信息

### FR-6：差异对比（diff）

- 无参：工作区 vs 最近快照
- 一个快照 ID：工作区 vs 指定快照
- 两个快照 ID：快照间对比
- 三个参数：两个快照间单文件对比
- 文本文件：unified diff 格式
- 二进制文件：显示大小变化摘要

### FR-7：版本恢复（restore）

- 恢复整个工作区或单个文件到任意快照
- 恢复前自动备份（消息格式：`backup: restore to <id>`）
- `--no-backup` 跳过自动备份

### FR-8：完整性校验（check）

- 遍历所有块，重新计算 BLAKE3 哈希与文件名比对
- 报告损坏或缺失的块
- `--fix` 标志预留（未来冗余修复）

### FR-9：分支管理（阶段二）

- 创建分支（不切换）
- 切换分支时自动保存当前状态
- `-c` 标志创建并切换

### FR-10：自动监听（阶段二）

- 通过轮询（time.Ticker）监听工作区文件变更
- 可配置间隔自动保存
- 自动保存的快照以 `auto -` 前缀标记

## 7. 非功能性需求

| 需求 | 目标 |
|------|------|
| 性能 | 1GB 混合文件保存 < 30 秒（SSD） |
| 存储效率 | 大文件小修改的增量保存去重率 > 50% |
| 跨平台 | Windows / macOS / Linux 单二进制 |
| 可靠性 | 所有持久化操作使用原子写入 |
| 可测试性 | 内存存储后端支持快速集成测试 |
| 可扩展性 | 通过 `Engine` 接口按需添加新文件类型引擎 |

## 8. 项目结构

```
drift/
├── main.go                  # 入口
├── go.mod / go.sum
├── docs/                    # 设计文档
│   ├── prd.md               # 本文档
│   ├── architecture.md      # 系统架构
│   ├── cli-design.md        # CLI 命令设计
│   └── roadmap.md           # 开发计划
├── core/                    # 核心数据类型（零外部依赖）
│   ├── hash.go              # BLAKE3 Hash 类型
│   ├── chunk.go             # Chunk 类型
│   ├── snapshot.go          # Snapshot 类型
│   ├── snapshot.proto       # Snapshot Protobuf Schema
│   ├── file_entry.go        # FileEntry 类型
│   ├── index.go             # Index 类型
│   ├── index.proto          # Index Protobuf Schema
│   ├── ref.go               # Reference 类型
│   ├── config.go            # Config 类型
│   ├── file_mode.go         # FileMode 类型
│   └── object.go            # Object 接口（预留）
├── util/                    # 工具包
│   ├── cache/               # LRU 缓存
│   ├── fsutil/              # 文件系统工具（遍历、原子写入）
│   └── logger/              # 结构化日志
├── chunker/                 # 分块算法
│   ├── chunker.go           # Chunker 接口（Chunk(r io.Reader)）
│   ├── strategy.go          # 二进制类共享分块策略（BinaryChunkerFor + 阈值常量）
│   ├── fastcdc.go           # FastCDC 实现
│   └── fixed.go             # 定长分块器
├── storage/                 # 存储层
│   ├── chunk_store.go       # ChunkStorer 接口
│   ├── snapshot_store.go    # SnapshotStorer 接口
│   ├── ref_store.go         # ReferenceStorer 接口
│   ├── index_store.go       # IndexStorer 接口
│   ├── config_store.go      # ConfigStorer 接口
│   ├── preview_store.go     # PreviewStorer 接口
│   ├── storer.go            # Storer 组合接口
│   ├── filesystem/          # 磁盘存储实现
│   └── memory/              # 内存存储实现（测试用）
├── filetype/                # 文件类型引擎
│   ├── engine.go            # 引擎接口定义
│   ├── registry.go          # 引擎注册表
│   ├── init.go              # 自动注册
│   ├── binary/              # 二进制引擎（兜底）
│   └── text/                # 文本引擎（分块+差异+预览）
├── porcelain/               # 业务逻辑层
│   ├── project.go           # 项目初始化/打开
│   ├── snapshot.go          # 快照创建
│   └── restore.go           # 快照恢复
└── cmd/                     # CLI 命令层
    ├── root.go              # 根命令
    ├── init.go              # drift init
    ├── save.go              # drift save
    ├── log.go               # drift log
    ├── status.go            # drift status
    ├── show.go              # drift show
    ├── diff.go              # drift diff
    ├── restore.go           # drift restore
    └── check.go             # drift check
```

## 9. 技术栈

| 组件 | 技术选型 | 用途 |
|------|---------|------|
| 语言 | Go 1.22+ | 单二进制跨平台发布 |
| CLI 框架 | cobra | 命令解析与帮助生成 |
| 哈希 | BLAKE3 (zeebo/blake3) | 内容寻址存储、去重、完整性校验 |
| CDC 分块 | FastCDC (go-cdc-chunkers) | 内容定义分块，二进制文件去重 |
| 压缩 | zstd (klauspost/compress) | 快速高效的分块压缩 |
| 序列化 | Protobuf (google.golang.org/protobuf) | 快照和索引持久化 |
| 缓存 | LRU (hashicorp/golang-lru/v2) | 块缓存，避免重复解压 |
| 文件监听 | 轮询（time.Ticker） | 阶段二自动保存 |
| 图片处理 | 纯 Go（magic bytes 解析） | 阶段三图片元信息 |
| GUI 框架 | Wails v3 + React | 阶段四桌面应用 |

## 10. 当前进度

- [x] 架构设计完成
- [x] CLI 命令设计完成
- [x] 开源依赖选型与核查
- [x] 阶段一实现完成（8 个 CLI 命令，7 层架构，约 40 个 Go 文件）
- [ ] 阶段二：分支与自动化
- [ ] 阶段三：富文件类型引擎
- [ ] 阶段四：桌面 GUI
- [ ] 阶段五：远程同步
