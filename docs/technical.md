# Drift - 技术设计文档

## 技术栈

| 项目 | 选择 | 原因 |
|------|------|------|
| **语言** | Go | 开发效率高、跨平台、编译为单一二进制 |
| **版本** | Go 1.26.4 | 最新稳定版 |
| **哈希算法** | SHA-256 | 安全性高，避免碰撞 |
| **存储方式** | 内容寻址 + JSON元数据 | 简单可靠 |
| **CLI框架** | cobra | Go标准CLI库 |
| **压缩** | 可选gzip | 节省存储空间 |

## 架构设计

### 对象模型

Drift借鉴Git的对象模型，但进行简化：

```
对象类型：
├── Blob  - 存储文件内容（二进制/文本）
├── Tree  - 存储目录结构（递归）
└── Commit - 存储版本信息
```

### 对象关系

```
Commit
  └── Tree (根目录)
        ├── Tree (子目录1)
        │     ├── Blob (文件1)
        │     └── Blob (文件2)
        ├── Tree (子目录2)
        │     └── Blob (文件3)
        └── Blob (文件4)
```

## 目录结构

### 项目目录

```
.drift/
├── objects/              # 内容寻址存储
│   ├── blobs/           # 文件内容
│   │   ├── ab/cdef1234...
│   │   └── 56/7890abcd...
│   └── trees/           # 目录结构
│       ├── 12/34567890...
│       └── ef/ghijklmn...
├── commits/             # 版本记录
│   ├── v1.json
│   └── v2.json
├── refs/                # 分支引用
│   ├── main.json
│   └── 方案A.json
├── index                # 暂存区
└── config.json          # 项目配置
```

## 数据结构

### Blob对象

存储文件的实际内容，使用SHA-256哈希作为文件名。

```
objects/blobs/ab/cdef1234567890...
```

### Tree对象

```json
{
  "hash": "tree_xyz789",
  "entries": [
    {
      "name": "章节",
      "type": "tree",
      "hash": "tree_abc111"
    },
    {
      "name": "素材",
      "type": "tree",
      "hash": "tree_def222"
    },
    {
      "name": "大纲.txt",
      "type": "blob",
      "hash": "blob_ghi333"
    }
  ]
}
```

### Commit对象

```json
{
  "hash": "commit_v1",
  "id": "v1",
  "message": "完成前四章",
  "timestamp": "2024-01-15T10:30:00Z",
  "parent": null,
  "branch": "main",
  "tree": "tree_xyz789"
}
```

### 暂存区（index）

```json
{
  "staged": [
    {
      "path": "章节/第一章.txt",
      "hash": "blob_abc123",
      "status": "modified"
    },
    {
      "path": "章节/第四章.txt",
      "hash": "blob_def456",
      "status": "added"
    }
  ]
}
```

### 分支引用

```json
{
  "name": "main",
  "current_commit": "commit_v3",
  "created_at": "2024-01-15T10:00:00Z"
}
```

## 存储方案

### 内容寻址

- 文件内容使用SHA-256哈希作为标识
- 相同内容的文件只存储一次
- 自动去重，节省空间

### 硬链接优化

- 未修改的文件使用硬链接
- 避免重复存储
- 节省磁盘空间

### 文件组织

```
objects/blobs/
├── ab/
│   └── cdef1234...   # 前2位作为子目录
└── 56/
    └── 7890abcd...
```

## 核心模块

### 1. 对象存储引擎（internal/storage）

负责：
- Blob对象的读写
- Tree对象的构建和解析
- 内容哈希计算
- 硬链接管理

### 2. 暂存区管理（internal/core）

负责：
- 文件变更检测
- 暂存区状态管理
- 文件添加/移除

### 3. 版本管理（internal/core）

负责：
- Commit对象创建
- 版本历史管理
- 分支管理

### 4. CLI命令（internal/cli）

负责：
- 命令解析
- 用户交互
- 输出格式化

## 哈希算法

### SHA-256

- 输出长度：256位（32字节）
- 十六进制表示：64字符
- 安全性高，避免碰撞
- 适合文件内容标识

### 哈希计算

```go
func CalculateHash(data []byte) string {
    hash := sha256.Sum256(data)
    return hex.EncodeToString(hash[:])
}
```

## 跨平台支持

### 编译目标

```bash
# Windows
GOOS=windows GOARCH=amd64 go build -o drift.exe

# macOS
GOOS=darwin GOARCH=amd64 go build -o drift

# Linux
GOOS=linux GOARCH=amd64 go build -o drift
```

### 文件路径处理

- 使用`filepath.Join`处理路径
- 支持Windows/Linux/macOS路径分隔符
- 统一使用`/`作为内部路径分隔符

## 性能考虑

### 大文件处理

- 流式计算哈希，避免一次性加载到内存
- 分块读取，支持GB级文件
- 进度条显示

### 目录遍历

- 并发遍历目录
- 忽略.drift目录本身
- 支持.gitignore类似的忽略规则（后续版本）

## 安全考虑

### 数据完整性

- 哈希校验确保文件完整
- 写入原子性（先写临时文件，再重命名）

### 并发安全

- 文件锁保护.drift目录
- 防止多实例同时操作

## 扩展性

### 插件机制（后续版本）

- 支持自定义文件预览
- 支持自定义导出格式
- 支持远程存储后端

### 配置文件

```json
{
  "version": "1.0.0",
  "hash_algorithm": "sha256",
  "compression": false,
  "auto_save_interval": 0
}
```
