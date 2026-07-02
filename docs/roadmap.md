# drift 开发计划

---

## 阶段一：核心存储引擎 + 基础命令

**目标**：能存、能看、能回——单机本地版本管理的最小可用版本。

### 模块任务

| 模块 | 任务 | 优先级 |
|------|------|--------|
| `go.mod`, `main.go` | 项目初始化，依赖引入 | P0 |
| `core/` | Hash (BLAKE3), Chunk, Snapshot, Reference, FileEntry, FileMode, Config, Index 类型定义 | P0 |
| `porcelain/project.go` | 项目初始化 / 打开逻辑 | P0 |
| `porcelain/snapshot.go` | 快照创建（扫描 → 分块 → 去重 → 压缩 → 存储）| P0 |
| `porcelain/restore.go` | 恢复逻辑（备份 → 获取快照 → 重建工作区）| P0 |
| `storage/storer.go` | Storer 接口组合 | P0 |
| `storage/filesystem/` | .drift/ 磁盘存储实现（chunks/, snapshots/, refs/, index, config）| P0 |
| `storage/filesystem/layout.go` | .drift/ 目录布局定义 | P0 |
| `storage/filesystem/config.go` | 配置读写 | P0 |
| `storage/memory/` | 内存存储（测试用）| P1 |
| `chunker/` | 封装 go-cdc-chunkers FastCDC + 定长 fallback + zstd 压缩 | P0 |
| `filetype/engine.go` | Engine / Detector / ChunkerSelector / Differ / Previewer 接口定义 | P0 |
| `filetype/registry.go` | 引擎注册表 + MIME 检测 | P0 |
| `filetype/binary/` | 通用二进制引擎（fallback）| P0 |
| `filetype/text/` | 文本引擎（ChunkerFor + Differ）| P0 |
| `util/cache/` | Chunk Cache + Preview Cache（hashicorp/golang-lru/v2）| P0 |
| `util/fsutil/` | 目录遍历 + 原子文件操作 | P0 |

### CLI 命令

| 命令 | 对应文件 | 功能 |
|------|---------|------|
| `drift init` | cmd/init.go | 创建 .drift/ 目录结构 |
| `drift save [-m <msg>]` | cmd/save.go | 保存快照 |
| `drift log [-l <n>]` | cmd/log.go | 表格展示历史快照 |
| `drift status` | cmd/status.go | 显示工作区变更 (A/M/D) |
| `drift show <id> <file>` | cmd/show.go | 查看历史版本中的文件内容 |
| `drift diff [<id1> <id2>]` | cmd/diff.go | 文件级汇总 / unified diff |
| `drift restore <id>` | cmd/restore.go | 恢复工作区到指定快照 |
| `drift check` | cmd/check.go | 校验 .drift/ 数据完整性 |
| `drift` | cmd/root.go | 主命令入口 |

### 阶段一交付物

- `drift` 单二进制（macOS / Windows / Linux）
- 支持 save / log / status / diff / show / restore / check 全部基础命令
- 所有文件通过 binary 引擎 + FastCDC 分块存储
- 测试覆盖率 > 70%

---

## 阶段二：分支系统 + 自动化

**目标**：能分叉、能自动保存。

### 模块任务

| 模块 | 任务 | 状态 |
|------|------|------|
| `porcelain/branch.go` | 分支创建 / 切换 / 删除 / 重命名逻辑 | ✅ 已完成 |
| `cmd/branch.go` | `drift branch` / `branch -d` / `branch -m` 命令 | ✅ 已完成 |
| `cmd/switch.go` | `drift switch` / `drift switch -c` 命令 | ✅ 已完成 |
| `cmd/watch.go` | `drift watch` 命令（fsnotify） | ✅ 已完成 |
| `cmd/ignore.go` | `drift ignore` 命令 | ✅ 已完成 |
| `porcelain/lock.go` | 工作区锁（watch/switch 协调） | ✅ 已完成 |
| `porcelain/gc.go` | 垃圾回收（不可达快照与块清理） | ✅ 已完成 |
| `cmd/gc.go` | `drift gc` 命令 | ✅ 已完成 |
| `util/glob/` | .driftignore 模式匹配 | ✅ 已完成 |

### CLI 命令

| 命令 | 对应文件 | 功能 |
|------|---------|------|
| `drift branch <name>` | cmd/branch.go | 创建分支（不切换）|
| `drift branch -d <name>` | cmd/branch.go | 删除分支 |
| `drift branch -m <new>` | cmd/branch.go | 重命名当前分支 |
| `drift branch -m <old> <new>` | cmd/branch.go | 重命名指定分支 |
| `drift switch <name>` | cmd/switch.go | 切换到已有分支 |
| `drift switch -c <name>` | cmd/switch.go | 创建并切换到新分支 |
| `drift switch main` | cmd/switch.go | 切回主线 |
| `drift watch on/off/status` | cmd/watch.go | 文件监听，自动保存 |
| `drift ignore <pattern>` | cmd/ignore.go | 忽略文件/目录 |
| `drift gc [--dry-run]` | cmd/gc.go | 回收不可达快照与块 |

### 阶段二交付物

- 多分支并行实验
- 分支删除 / 重命名（含当前分支 HEAD 同步）
- `drift watch` 后台守护，崩溃保护，watch/switch 工作区锁协调
- 分支间切换自动备份 + 工作区还原
- 垃圾回收：删除分支后回收孤立快照与块，`--dry-run` 预览

---

## 阶段三：富文件类型引擎

**目标**：图片/视频有格式感知，大文件分块策略优化。

### 模块任务

| 模块 | 任务 | 状态 |
|------|------|------|
| `filetype/engine.go` | Detector 接口重构：拆为 `DetectByMagic` / `DetectByExtension` / `DetectByHeuristic` 三方法 | ✅ 已完成 |
| `filetype/registry.go` | `Registry.Detect` 改为三轮分层匹配（magic → extension → heuristic） | ✅ 已完成 |
| `filetype/init.go` | 引擎注册顺序：text → image → video → binary | ✅ 已完成 |
| `filetype/image/engine.go` | ImageEngine：分层 Detect（png/jpg/gif/webp/bmp/tiff）、Name | ✅ 已完成 |
| `filetype/image/differ.go` | 文件级元信息 diff（尺寸/格式/大小变化，不做像素 diff） | ✅ 已完成 |
| `filetype/image/preview.go` | 输出图片摘要信息（尺寸 + 格式 + 大小） | ✅ 已完成 |
| `filetype/video/engine.go` | VideoEngine：分层 Detect（mp4/mov/avi/mkv/webm） | ✅ 已完成 |
| `filetype/video/differ.go` | 文件级 diff（文件大小变化） | ✅ 已完成 |
| `filetype/video/preview.go` | 输出视频基本信息（含 MP4 tkhd 尺寸解析） | ✅ 已完成 |
| `chunker/fastcdc.go` | FastCDC 参数可配置化：`NewFastCDCChunkerWithParams(min, avg, max int)` | ✅ 已完成 |
| `porcelain/snapshot.go` | 按引擎类型 + 文件大小 5 档选择分块策略（各引擎 `ChunkerFor`） | ✅ 已完成 |
| `porcelain/snapshot.go` | CreateSnapshot 与 ComputeFileHash 共用 `chunkFile()` 保证哈希一致 | ✅ 已完成 |
| `filetype/psd/` | PSD 图层解析（研究性，非硬交付） | ⏸ 降级为研究项（P2） |

### 分块策略分档（5 档自适应）

各引擎 `ChunkerFor(fileSize)` 实现的分块策略（text 2 档 + binary 3 档 = 5 档效果）：

| 文件特征 | 分块策略 | 理由 |
|----------|---------|------|
| 文本 < 64KB | 整文件一块（nil chunker） | 太小，分块增加索引开销无收益 |
| 文本 64K-50MB | FastCDC 4K-8K-16K | 小块，行级修改去重好 |
| 二进制 < 50MB | FastCDC 128K-256K-512K（默认） | 当前策略，已验证 |
| 二进制 50M-500M | FastCDC 1M-2M-4M | 减少块数，降低索引膨胀 |
| 二进制 > 500M | Fixed 8M | 大文件 CDC 找切点 CPU 开销大，定长更高效 |

### 不做的事（明确排除）

- 像素级 diff（为 Phase 4 GUI 储备，CLI 阶段不做）
- 缩略图图片生成（需 govips/CGO，暂不引入重依赖）
- PSD 图层级分块作为硬交付（降级为研究项 P2，未纳入 Phase 3 交付）

### 阶段三交付物

- **ImageEngine**：6 种图片格式检测（magic + 扩展名双层），元信息 diff（格式/尺寸/大小），preview 摘要
- **VideoEngine**：5 种视频格式检测，大小 diff，preview（含 MP4 tkhd 尺寸解析）
- **分层检测架构重构**：Detector 接口拆为 `DetectByMagic` / `DetectByExtension` / `DetectByHeuristic`，Registry 三轮分层匹配
- **FastCDC 参数可配置化**：`NewFastCDCChunkerWithParams(min, avg, max int)`，默认 `NewFastCDCChunker()` 保持 128K-256K-512K
- **5 档自适应分块策略**：按引擎类型 + 文件大小选择最优分块（含整文件单块优化）
- **CreateSnapshot 与 ComputeFileHash 分块策略一致性保证**：共用 `chunkFile()` → 同一文件两路计算得到相同哈希
- 图片/视频文件 diff 输出有意义的元信息对比（尺寸/格式/大小），而非 "binary files differ"
- 图片/视频文件 show/preview 输出摘要信息，而非 `[binary file]`

---

## 阶段四：GUI 桌面应用

**目标**：创作者通过图形界面操作，零命令行。

### 技术栈

| 层 | 选型 |
|----|------|
| 框架 | Wails v3 |
| 前端 | React + 组件库 |
| 后端 | 嵌入 drift 库，直接调用 porcelain 层 |

### 核心界面

| 界面 | 功能 |
|------|------|
| 时间线视图 | 所有快照的缩略图并列展示，一键切换/恢复 |
| 视觉 Diff | 图片叠加 / 并排 / 像素热力图 |
| 文件浏览器 | 按快照浏览历史文件内容 |
| 分支管理 | 可视化创建/切换分支 |
| 设置面板 | 忽略规则、自动保存间隔、用户信息 |

### 阶段四交付物

- macOS / Windows 安装包
- 双击打开项目，拖动时间线浏览历史
- CLI 命令完全可用，GUI 为附加交互方式

---

## 阶段五：远程协同（远期）

**目标**：多人共享版本库。

### 功能规划

| 功能 | 说明 |
|------|------|
| `drift remote add <name> <url>` | 配置远程存储 |
| `drift sync` | 块级增量同步（只传新块） |
| 后端支持 | S3 / IPFS / 自建服务器 |
| 端到端加密 | 可选的远程数据加密 |

---

## 项目里程碑

```
Phase 1 ──── Phase 2 ──── Phase 3 ──── Phase 4 ──── Phase 5
  ▲             ▲            ▲            ▲            ▲
  │             │            │            │            │
  │         分支+自动化   图片/视频     GUI桌面      远程协同
  │                       +分层检测     应用
  │                       +5档分块
 核心存储
 + 基础CLI

 目标：           目标：        目标：        目标：        目标：
 能存能看能回      能分叉自动存   富文件支持     图形界面      多人共享
```

## 当前状态

- [x] 项目架构设计
- [x] CLI 命令设计
- [x] 开源库选型与核查
- [x] 阶段一开发
- [x] 阶段二开发（分支系统 + 自动化 + GC）
- [x] 阶段三开发（富文件类型引擎：Image/Video + 分层检测 + 5 档分块）
