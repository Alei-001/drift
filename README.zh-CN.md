<p align="center">
  <img src="assets/icon.png" alt="drift logo" width="180">
</p>

<h1 align="center">drift</h1>

<p align="center">
  <a href="README.md">English</a> ·
  <a href="https://github.com/Alei-001/drift">GitHub</a> ·
  <a href="docs/roadmap.md">路线图</a>
</p>

---

面向创作者的版本控制 —— 基于内容寻址、分块去重的版本控制系统，专为写作、绘画、设计等创意工作设计。与把所有文件当作不透明字节处理的 Git 不同，drift 能识别图片和视频的格式与元信息，并通过内容定义分块只存储真正发生变化的部分。

> 当前状态：阶段 1–3.5 已完成（本地核心 + 分支 + 文件类型引擎 + 远程同步）。GUI 桌面应用为后续规划，参见 [docs/roadmap.md](docs/roadmap.md)。

## 为什么需要 drift？

| Git 的痛点 | drift 的解决方案 |
|---|---|
| 200 MB 的文件只改了一小部分 → 整个文件重新存储 | FastCDC 内容定义分块，只存储变化的块 |
| 暂存区 / 提交 / 合并 —— 程序员的心智模型 | `save` 自动捕获所有变更；无暂存区，无合并 |
| 二进制文件的 diff 只显示原始字节 | 可插拔的文件类型引擎（text/image/video/binary），文本支持逐行 diff，图片/视频显示格式与尺寸变化 |
| 没有可视化时间线 | CLI 阶段以表格浏览历史；GUI 可视化时间线为阶段 4 规划 |
| 分支意味着合并冲突 | 分支是纯分叉用于实验，用户手动合并，永远不会有冲突 |

## 特性

- **内容寻址存储** —— BLAKE3 哈希校验完整性，跨快照与分支自动去重。
- **CDC 分块** —— FastCDC 变长内容定义分块，对超大文件（>100 MB）提供定长 fallback。上层使用 zstd 压缩。
- **无暂存区** —— `drift save` 原样捕获工作区。作者只需关注作品本身，无需关心索引。
- **无合并的分支** —— 创建实验性分支，自由切换，从任意位置恢复。永远不会有合并冲突。
- **文件类型引擎** —— 文本（逐行 unified diff）、图片（格式/尺寸/大小对比，支持 PNG/JPEG/GIF/WebP/BMP/TIFF）、视频（格式识别 + 尺寸解析，支持 MP4/MOV/AVI/MKV/WebM）、二进制（兜底）。新引擎通过注册表插拔。
- **自动监听** —— `drift watch on` 在文件变更时自动保存，auto-save 默认在 `log` 中隐藏。
- **远程同步** —— 通过 WebDAV 或 SMB 协议将本地仓库推送到远程存储，支持增量同步与分支级推送/拉取。
- **单一二进制** —— macOS / Windows / Linux 通用的 Go 静态二进制。无需运行时，无需安装守护进程。

## 安装

```powershell
go install github.com/Alei-001/drift/cmd/drift@latest
```

或从源码构建（通过 ldflags 注入版本信息）：

```powershell
git clone https://github.com/Alei-001/drift.git
cd drift
go build -ldflags "-X github.com/Alei-001/drift/internal/version.Version=v0.1.0" -o drift ./cmd/drift
```

需要 Go 1.25+。

### 升级

发布 GitHub release 后，可直接自升级到最新版：

```powershell
drift upgrade          # 下载并替换当前二进制
drift upgrade --check  # 仅检查是否有新版本
```

发布产物命名约定：`drift_<版本>_<系统>_<架构>.{zip|tar.gz}`，附
`drift_<版本>_checksums.txt`（SHA-256），存在时自动校验。

## 快速上手

```powershell
# 初始化项目
cd my-novel
drift init

# 保存快照
drift save -m "第一章草稿"

# 查看自上次保存以来的变更
drift status

# 浏览历史（当前分支链）
drift log

# 尝试一个实验方向
drift branch create rewrite-ending
drift switch rewrite-ending
# ... 编辑文件 ...
drift save -m "备选结局 v1"

# 切回主分支；实验内容保留在自己的分支上
drift switch main

# 查看某个快照的文件变更明细
drift log --detail id:12ab

# 将工作区恢复到指定快照（恢复前自动备份）
drift restore id:12ab

# 撤销最近一次保存
drift undo

# 配置远程仓库并同步
drift remote add origin webdav://example.com/dav/my-novel
drift push origin            # 推送本地数据到远程
drift pull origin            # 从远程拉取最新数据
drift clone webdav://example.com/dav/my-novel my-novel  # 克隆远程仓库
```

## 命令一览

| 命令 | 用途 |
|---|---|
| `drift init` | 创建 `.drift/` 仓库 |
| `drift save [-m <msg>] [--tag <name>]` | 保存所有变更为快照 |
| `drift status` | 显示新增 / 修改 / 删除的文件 |
| `drift log [--branch <name>] [--all] [--limit <n>]` | 浏览快照历史 |
| `drift show <version> [<file>]` | 列出快照中的文件，或显示某文件的内容 |
| `drift diff <v1> <v2>` | 对比两个快照（文件清单或 unified diff） |
| `drift restore <version>` | 将工作区恢复到指定快照（先备份） |
| `drift undo` | 撤销最近一次保存 |
| `drift branch {list,create,delete,rename}` | 管理分支 |
| `drift switch <branch>` | 切换分支（用 `-c` 可同时创建） |
| `drift tag {list,add,delete,rename}` | 管理标签 |
| `drift watch {on,off,status,pause,resume}` | 后台自动保存守护进程 |
| `drift ignore <pattern>` | 添加忽略规则到 `.driftignore` |
| `drift resolve <version>` | 解析版本引用为快照 ID |
| `drift remote {add,list,remove}` | 管理远程仓库配置 |
| `drift push <remote> [--branch <name>] [--dry-run]` | 推送本地数据到远程 |
| `drift pull <remote> [--branch <name>] [--dry-run]` | 从远程拉取数据到本地 |
| `drift clone <remote-url> <path>` | 克隆远程仓库到本地 |
| `drift ls-remote <remote>` | 列出远程仓库的分支与标签 |
| `drift check` | 校验 `.drift/` 存储完整性 |
| `drift gc [--dry-run]` | 清理不可达的快照与分块 |
| `drift config {get,set,list}` | 查看与修改配置 |
| `drift version` | 显示版本、提交与构建信息 |
| `drift upgrade [--check] [--force]` | 自升级到最新 GitHub release |

### 版本引用语法

接受 `<version>` 参数的命令支持以下写法：

- `head` —— 当前 HEAD 快照
- `id:<hash-prefix>` —— 按哈希前缀匹配（≥ 4 字符）
- `tag:<name>` —— 通过标签解析
- `branch:<name>` —— 通过分支头解析
- `<bare-name>` —— `branch:<bare-name>` 的简写

## 项目结构

```
cmd/                  CLI 入口（cobra 命令）—— 不含业务逻辑
  drift/              主二进制
internal/             业务实现（不可被外部项目导入）
  porcelain/          业务逻辑：snapshot、branch、restore、lock、watch、gc
  filetype/           可插拔类型引擎（text/image/video/binary）
  chunker/            FastCDC + 定长分块
  storage/            Storer 接口 + 共享辅助
    backends/         filesystem（生产）与 memory（测试）实现
    refname/          分支 / 标签名校验
    stream/           分块流式辅助
  remote/             远程同步：WebDAV/SMB 协议、push/pull 传输
  core/               领域类型：Hash、Chunk、Snapshot、FileEntry、Config 等
  util/               fsutil、glob、pathutil、format、cache
  version/            版本元数据 + GitHub Releases 自升级
docs/                 设计与参考文档
```

完整分层规则与约定见 [AGENTS.md](AGENTS.md)。

## 文档

- [docs/prd.md](docs/prd.md) —— 产品需求文档
- [docs/roadmap.md](docs/roadmap.md) —— 开发路线图
- [docs/cli-design.md](docs/cli-design.md) —— CLI 设计与输出格式
- [docs/architecture.md](docs/architecture.md) —— 分层架构与数据模型
- [docs/CODE_STANDARDS.md](docs/CODE_STANDARDS.md) —— 编码规范（权威）
- [docs/CODE_REVIEW.md](docs/CODE_REVIEW.md) —— 代码审查标准
- [docs/engine-plugin.md](docs/engine-plugin.md) —— 新增文件类型引擎指南

## 构建与测试

```powershell
go build ./...            # 构建全部包
go test ./...             # 运行全部测试
go test -run TestFoo ./internal/porcelain/   # 单个测试
```

无 Makefile、无 lint 配置 —— 纯 `go` 工具链。CI 通过 GitHub Actions
（见 `.github/workflows/`）。

### Protobuf 代码生成

生成文件位于 `internal/core/*.pb.go`。重新生成：

```powershell
protoc --proto_path=internal/core --go_out=internal/core --go_opt=paths=source_relative internal/core/snapshot.proto
protoc --proto_path=internal/core --go_out=internal/core --go_opt=paths=source_relative internal/core/index.proto
```

`--go_opt=paths=source_relative` 标志是**必需的**（详见 AGENTS.md）。

## 主要依赖

- [cobra](https://github.com/spf13/cobra) —— CLI 框架
- [zeebo/blake3](https://github.com/zeebo/blake3) —— 内容哈希
- [klauspost/compress](https://github.com/klauspost/compress) —— zstd 压缩
- [google.golang.org/protobuf](https://pkg.go.dev/google.golang.org/protobuf) —— 快照序列化格式
- [PlakarKorp/go-cdc-chunkers](https://github.com/PlakarKorp/go-cdc-chunkers) —— FastCDC 实现
- [hashicorp/golang-lru/v2](https://github.com/hashicorp/golang-lru) —— 分块与预览缓存

## 许可证

[MIT](LICENSE) —— 详见 [LICENSE](LICENSE) 文件。
