# Drift

面向创意工作者的轻量级版本管理工具。

## 目标用户

插画师、设计师、小说作者、剧本创作者——需要管理多版本创意文件、但不愿学习复杂工具的人。

## 特性

- **简单易用** — 10 分钟上手，无需学习 Git
- **暂存预览** — 保存前预览即将提交的内容
- **分支探索** — 尝试不同创作方向（配色方案、剧情线）
- **版本导出** — 快速导出任意版本为目录 / zip / tar.gz
- **文本对比** — 查看版本之间的文字差异
- **跨平台** — Windows、macOS、Linux

## 快速开始

```bash
# 安装
install.bat    # Windows / PowerShell

# 初始化项目
drift init

# 添加文件到暂存区
drift add .

# 保存版本
drift save -m "初稿完成"

# 查看历史
drift list

# 回到旧版本
drift restore v1

# 导出交付
drift export v1 -o ./交付客户
```

## 技术栈

- **语言**: Go
- **哈希**: SHA-256（纯摘要，不兼容 Git）
- **存储**: 内容寻址 + 二进制格式（DRIX / DREE / DCMT）
- **CLI**: cobra

## 文档

| 文档 | 内容 |
|------|------|
| [产品需求](docs/PRD.md) | 目标用户、使用场景、功能取舍 |
| [技术设计](docs/technical.md) | 架构、数据格式、跨平台方案 |
| [命令参考](docs/commands.md) | 完整 CLI 命令说明 |
| [开发进度](docs/progress.md) | 已完成阶段与下一步计划 |
| [已知问题](docs/issues.md) | 缺陷分析与修复路线 |

## 项目结构

```
drift/
├── cmd/drift/          # CLI 入口
├── internal/
│   ├── core/           # 核心对象模型（Blob / Tree / Commit / Index）
│   ├── storage/        # 存储引擎（内容寻址、原子写入）
│   └── cli/            # CLI 命令实现
├── dist/               # 编译产物与安装脚本
└── docs/               # 文档
```

## 许可证

MIT
