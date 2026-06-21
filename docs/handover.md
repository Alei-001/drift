# Drift - 交接文档

## 项目概述

**项目名称：** Drift

**项目定位：** 面向创意工作者的轻量级版本管理工具

**技术栈：** Go 1.26.4

**项目路径：** E:\Projects\drift

## 目标用户

- 插画师/画画博主
- 平面设计师
- 视频创作者
- 摄影师
- 小说作者
- 剧本创作

## 已完成工作

### 1. 项目初始化

- [x] 创建项目目录结构
- [x] 初始化Go模块（go.mod）
- [x] 初始化Git仓库
- [x] 创建.gitignore（排除reference/）
- [x] 初始提交（commit: 0b22663）

### 2. 文档编写

- [x] README.md - 项目概述
- [x] PRD.md - 产品需求文档
- [x] technical.md - 技术设计文档
- [x] commands.md - CLI命令设计
- [x] development-plan.md - 开发计划

### 3. 参考代码

- [x] 克隆go-git到reference/go-git/

## 项目结构

```
E:\Projects\drift\
├── .git/                # Git仓库
├── .gitignore           # 排除reference/
├── cmd/drift/           # CLI入口（待开发）
├── docs/                # 文档
│   ├── README.md
│   ├── PRD.md
│   ├── technical.md
│   ├── commands.md
│   ├── development-plan.md
│   └── handover.md      # 本文件
├── internal/            # 核心代码（待开发）
│   ├── core/            # 对象模型
│   ├── storage/         # 存储引擎
│   └── cli/             # CLI命令
├── reference/           # 参考代码（已git忽略）
│   └── go-git/          # Go实现的Git
├── test/                # 测试（待开发）
└── go.mod               # Go模块文件
```

## 核心设计决策

### 1. 对象模型（借鉴Git）

```
Blob  - 存储文件内容
Tree  - 存储目录结构（递归）
Commit - 存储版本信息
```

### 2. 存储结构

```
.drift/
├── objects/blobs/       # 文件内容（内容寻址）
├── objects/trees/       # 目录结构
├── commits/             # 版本记录
├── refs/                # 分支引用
├── index                # 暂存区
└── config.json          # 项目配置
```

### 3. CLI命令设计

**暂存区命令：**
- `drift add <path>` - 添加文件到暂存区
- `drift status` - 查看工作区状态
- `drift reset` - 清空暂存区

**版本命令：**
- `drift save -m "备注"` - 提交版本
- `drift list` - 查看版本历史
- `drift show <id>` - 查看版本详情
- `drift export <id>` - 导出版本
- `drift restore <id>` - 回退版本

**分支命令：**
- `drift branch <name>` - 创建分支
- `drift branch list` - 查看分支
- `drift switch <name>` - 切换分支

**对比命令：**
- `drift diff` - 查看差异

### 4. 关键决策

| 决策 | 选择 | 原因 |
|------|------|------|
| 语言 | Go | 跨平台、编译为单一二进制 |
| 哈希 | SHA-256 | 安全性高 |
| 暂存区 | 支持 | 用户可选择性保存文件 |
| 分支合并 | 不做 | 二进制文件无法自动合并 |
| 存储方式 | 快照+硬链接 | 简单可靠 |

## 开发计划

```
Phase 1：基础框架（2周）      - Blob/Tree存储
Phase 2：暂存区（2周）        - drift add/status/reset
Phase 3：版本管理（2周）      - drift save/list/show
Phase 4：导出回退（2周）      - drift export/restore
Phase 5：分支功能（2周）      - drift branch/switch
Phase 6：对比功能（1周）      - drift diff
Phase 7：优化完善（2周）      - 性能优化、错误处理
```

**总计：** 约13周

## 待开发模块

### Phase 1：基础框架

```
internal/
├── core/
│   ├── blob.go          # Blob对象
│   ├── tree.go          # Tree对象
│   └── commit.go        # Commit对象
├── storage/
│   ├── object_store.go  # 对象存储引擎
│   └── hash.go          # 哈希计算
└── cli/
    └── root.go          # CLI根命令
```

## 参考资源

**go-git仓库：** `reference/go-git/`

| 目录 | 参考内容 |
|------|----------|
| `plumbing/` | 底层对象存储实现 |
| `storage/` | 存储接口设计 |
| `worktree.go` | 工作区管理 |
| `repository.go` | 仓库操作 |

## 环境信息

- **Go版本：** 1.26.4
- **Go路径：** C:\Program Files\Go\bin\go.exe
- **Git：** 已配置，已清除代理设置

## 下一步工作

1. 创建Phase 1的基础代码结构
2. 实现Blob对象存储
3. 实现Tree对象存储
4. 实现Commit对象存储
5. 实现基本的对象存储引擎

## 注意事项

1. reference文件夹已被.gitignore排除
2. go-git是参考代码，不要直接复制
3. 代码风格保持简洁，不添加不必要的注释
4. 优先实现核心功能，后续再优化
