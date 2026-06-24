<p align="center"><img src="../assets/icon.png" alt="Drift" width="96"></p>

# Drift

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![English README](https://img.shields.io/badge/README-English-blue.svg)](../README.md)

面向创意工作者的轻量级版本管理工具 —— 插画师、设计师、小说作者、剧本创作者。无需学习 Git 即可管理创意作品的多个版本。

> **English README:** [../README.md](../README.md)

## 为什么需要 Drift？

创意工作者目前靠手工管理版本 —— 文件夹命名为 `初稿`、`修改版1`、`终稿`、`终稿_最终版`、`终稿_最终版_改`、`终稿_最终版_改_客户选这个`。Drift 用一个简单的心智模型取代这种混乱：

```
保存版本  →  回到任意版本  →  导出交付
```

没有暂存区术语，没有合并冲突，不需要学习 Git 概念。

## 特性

- **简单易用** — 10 分钟上手，无需 Git 知识
- **暂存预览** — 保存前清楚看到即将提交的内容
- **分支探索** — 平行尝试不同配色方案、剧情线、布局（无 merge —— 分支是独立的创作线）
- **版本导出** — 导出任意版本为目录 / `.zip` / `.tar.gz`
- **文本对比** — 为写作者提供版本间行级差异对比
- **二进制友好** — 大文件（.psd、.blend、视频）流式处理 + 进度条，不会 OOM
- **跨平台** — Windows、macOS、Linux
- **WIP 自动保存** — 切换分支时有未保存改动会自动保存，一条命令恢复
- **版本别名** — 用 `初稿` / `终稿` 这样的名字代替 `v1` / `v2`

## 快速开始

### 安装

**Windows（安装程序）：** 从 [Releases](../../releases) 下载 `drift-setup-x.y.z.exe` 运行 —— 图形化安装向导，自动配置 PATH，带卸载程序。

**源码编译：**
```bash
go build -ldflags "-X github.com/drift/drift/internal/cli.version=0.1.0" -o drift ./cmd/drift/
```

验证安装：
```bash
drift version
```

### 使用

```bash
# 初始化项目（在当前目录创建 .drift/）
drift init

# 添加所有文件到暂存区
drift add .

# 保存版本
drift save -m "初稿完成"

# 查看历史
drift log --all

# 回到之前的版本
drift restore v1

# 导出版本用于交付
drift export v1 -o ./交付客户
```

### 常见工作流

**作家探索剧情分支：**
```bash
drift save -m "主线剧情 v1"
drift branch 另一个结局
drift switch 另一个结局
drift save -m "备选结局"
drift diff v1 v2 -p          # 逐行对比两个版本
```

**设计师迭代客户项目：**
```bash
drift save -a -m "修改第二版"   # -a 自动暂存所有改动，类似 git commit -a
drift export v2 -o ./客户v2.zip -f zip
drift restore v1 素材/封面.psd   # 只从 v1 恢复一个文件
```

## 命令一览

| 命令 | 说明 |
|------|------|
| `init` | 初始化新的 Drift 项目 |
| `add` | 添加文件到暂存区（支持 glob 通配符、多路径） |
| `status` | 查看工作区状态 |
| `save` | 保存暂存区为新版本（`-a` 自动暂存，`--amend` 修改最近版本，`--name` 设置别名） |
| `log` | 查看提交历史（`--all` 跨分支） |
| `restore` | 恢复工作区或指定文件到某个版本 |
| `export` | 导出版本为 dir / zip / tar.gz |
| `diff` | 查看版本间差异（`-p` 详细模式，`-f`/`--` 过滤文件） |
| `branch` | 创建 / 查看 / 删除 / 重命名分支 |
| `switch` | 切换分支（自动保存 WIP，`--create` 自动创建） |
| `name` | 管理版本别名 |
| `wip` / `restore-wip` | 查看 / 恢复自动保存的工作进度 |
| `rm` / `mv` | 删除 / 移动已跟踪文件 |
| `config` | 查看和设置配置（`user.name`、`core.autocrlf` 等） |
| `history` / `undo` | 查看 / 撤销最近操作 |
| `version` | 显示 drift 版本 |

完整参考：[commands.md](commands.md)

## 技术栈

- **语言：** Go
- **哈希：** SHA-256（纯摘要，不兼容 Git）
- **存储：** 内容寻址 + 二进制格式（DRIX / DREE / DCMT）+ zlib 压缩
- **CLI 框架：** cobra
- **文件锁：** OS 级（Windows 用 LockFileEx，Unix 用 flock）

## 项目结构

```
drift/
├── cmd/drift/          # CLI 入口
├── internal/
│   ├── core/           # 核心对象模型（Blob / Tree / Commit / Index）、哈希、编解码、diff
│   ├── storage/        # 内容寻址存储、原子写入、文件锁
│   ├── cli/            # 所有 cobra 命令
│   └── config/         # JSON 配置读写
├── installer/          # Inno Setup Windows 安装脚本
├── .github/workflows/  # CI/CD：发布工作流（tag 触发）
└── docs/               # 设计文档（中文）
```

## 文档

| 文档 | 内容 |
|------|------|
| [产品需求](PRD.md) | 目标用户、使用场景、功能取舍 |
| [技术设计](technical.md) | 架构、数据格式、跨平台方案 |
| [命令参考](commands.md) | 完整 CLI 命令文档 |
| [开发进度](progress.md) | 已完成阶段与下一步计划 |
| [测试计划](test-plan.md) | 测试用例与覆盖率 |

## 发布流程

发布完全通过 GitHub Actions 自动化。推送版本 tag 后工作流会：

1. 编译 Windows（amd64）、macOS（amd64 + arm64）、Linux（amd64 + arm64）二进制
2. 用 Inno Setup 编译 Windows `setup.exe`（图形化安装程序、PATH 管理、卸载程序）
3. 将所有产物发布到 GitHub Release

```bash
git tag v0.1.0
git push origin v0.1.0
```

## 致谢

本项目参考了 [go-git](https://github.com/go-git/go-git) 的实现思路，其使用 [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0) 开源协议。

## 许可证

MIT
