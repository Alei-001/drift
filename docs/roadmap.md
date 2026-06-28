# drift 开发计划

---

## 阶段一：核心存储引擎 + 基础命令

**目标**：能存、能看、能回——单机本地版本管理的最小可用版本。

### 模块任务

| 模块 | 任务 | 优先级 |
|------|------|--------|
| `go.mod`, `main.go` | 项目初始化，依赖引入 | P0 |
| `core/` | Hash (BLAKE3), Chunk, Snapshot, Reference, FileEntry, FileMode, Config, Object 类型定义 | P0 |
| `porcelain/project.go` | 项目初始化 / 打开逻辑 | P0 |
| `porcelain/snapshot.go` | 快照创建（扫描 → 分块 → 去重 → 压缩 → 存储）| P0 |
| `porcelain/restore.go` | 恢复逻辑（备份 → 获取快照 → 重建工作区）| P0 |
| `storage/storer.go` | Storer 接口组合 | P0 |
| `storage/filesystem/` | .drift/ 磁盘存储实现（chunks/, snapshots/, refs/, index, config）| P0 |
| `storage/filesystem/layout.go` | .drift/ 目录布局定义 | P0 |
| `storage/filesystem/config.go` | 配置读写 | P0 |
| `storage/memory/` | 内存存储（测试用）| P1 |
| `chunker/` | 封装 go-cdc-chunkers FastCDC + 定长 fallback + zstd 压缩 | P0 |
| `filetype/engine.go` | Engine / Detector / Chunker / Differ / Previewer 接口定义 | P0 |
| `filetype/registry.go` | 引擎注册表 + MIME 检测 | P0 |
| `filetype/binary/` | 通用二进制引擎（fallback）| P0 |
| `filetype/text/` | 文本引擎（Chunker + Differ）| P0 |
| `util/cache/` | Chunk Cache + Preview Cache（hashicorp/golang-lru/v2）| P0 |
| `util/fsutil/` | 目录遍历 + 原子文件操作 | P0 |
| `util/logger/` | slog 封装 | P1 |

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

| 模块 | 任务 |
|------|------|
| `porcelain/branch.go` | 分支创建 / 切换逻辑 |
| `cmd/branch.go` | `drift branch` 命令 |
| `cmd/branch_switch.go` | `drift switch` / `drift switch -c` 命令 |
| `cmd/watch.go` | `drift watch` 命令（fsnotify） |
| `cmd/ignore.go` | `drift ignore` 命令 |
| `util/glob/` | .driftignore 模式匹配 |
| `util/event/` | 事件总线（watch 用） |

### CLI 命令

| 命令 | 对应文件 | 功能 |
|------|---------|------|
| `drift branch <name>` | cmd/branch.go | 创建分支（不切换）|
| `drift switch <name>` | cmd/branch_switch.go | 切换到已有分支 |
| `drift switch -c <name>` | cmd/branch_switch.go | 创建并切换到新分支 |
| `drift switch main` | cmd/branch_switch.go | 切回主线 |
| `drift watch` | cmd/watch.go | 文件监听，自动保存 |
| `drift ignore <pattern>` | cmd/ignore.go | 忽略文件/目录 |

### 阶段二交付物

- 多分支并行实验
- `drift watch` 后台守护，崩溃保护
- 分支间切换自动备份 + 工作区还原

---

## 阶段三：富文件类型引擎

**目标**：图片/PSD 有格式感知，真正区别于 git。

### 模块任务

| 模块 | 任务 |
|------|------|
| `filetype/image/chunker.go` | 图片分块器 |
| `filetype/image/preview.go` | 缩略图生成（govips） |
| `filetype/image/differ.go` | 像素级差异（为 GUI 储备） |
| `storage/filesystem/preview.go` | 预览缓存存储 |
| `filetype/psd/` | PSD 图层级分块（研究 + 实现） |

### 阶段三交付物

- 图片文件 CDC 去重，200MB PSD 修改后仅存变化块
- 缩略图自动生成并缓存（供 GUI 阶段使用）
- 大文件处理策略（>100MB 定长分块）

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
  │         分支+自动化   图片/PSD      GUI桌面      远程协同
  │                       格式引擎      应用
  │
 核心存储
 + 基础CLI

 目标：           目标：        目标：        目标：        目标：
 能存能看能回      能分叉自动存   富文件支持     图形界面      多人共享
```

## 当前状态

- [x] 项目架构设计
- [x] CLI 命令设计
- [x] 开源库选型与核查
- [ ] 阶段一开发
