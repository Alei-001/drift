# Drift

面向创意工作者的轻量级版本管理工具

## 简介

Drift 是一个专为创意工作者设计的版本管理工具。它让插画师、设计师、作家等创意工作者能够像程序员管理代码一样管理自己的创意作品。

## 特性

- **简单易用** - 10分钟上手，无需学习复杂概念
- **暂存功能** - 选择性保存需要的文件
- **分支探索** - 尝试不同创作方向
- **版本导出** - 快速导出任意版本
- **跨平台** - 支持 Windows、macOS、Linux

## 目标用户

- 插画师/画画博主
- 平面设计师
- 视频创作者
- 摄影师
- 小说作者
- 剧本创作

## 快速开始

```bash
# 初始化项目
drift init

# 添加文件到暂存区
drift add .

# 保存版本
drift save -m "初稿完成"

# 查看版本历史
drift list

# 导出版本
drift export v1 -o ./交付客户
```

## 命令参考

| 命令 | 功能 |
|------|------|
| `drift init` | 初始化项目 |
| `drift add <path>` | 添加文件到暂存区 |
| `drift status` | 查看工作区状态 |
| `drift save -m "备注"` | 保存版本 |
| `drift list` | 查看版本历史 |
| `drift show <id>` | 查看版本详情 |
| `drift export <id>` | 导出版本 |
| `drift restore <id>` | 回退到指定版本 |
| `drift branch <name>` | 创建分支 |
| `drift switch <name>` | 切换分支 |
| `drift diff` | 查看差异 |

## 文档

- [产品需求文档](docs/PRD.md)
- [技术设计文档](docs/technical.md)
- [CLI命令设计](docs/commands.md)
- [开发计划](docs/development-plan.md)

## 技术栈

- **语言**: Go
- **存储**: 内容寻址 + JSON元数据
- **哈希**: SHA-256

## 项目结构

```
drift/
├── cmd/drift/        # CLI入口
├── internal/
│   ├── core/         # 核心对象模型
│   ├── storage/      # 存储引擎
│   └── cli/          # CLI命令实现
├── docs/             # 文档
└── test/             # 测试
```

## 许可证

MIT License
