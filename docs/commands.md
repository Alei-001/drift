# Drift - CLI命令设计

## 设计原则

| 原则 | 说明 |
|------|------|
| **简洁性** | 比Git更少的命令 |
| **直观性** | 命令名即功能（save/list/export） |
| **中文友好** | 支持中文备注和分支名 |
| **非技术友好** | 面向创意工作者，学习成本低 |

## 命令总览

### 暂存区命令

| 命令 | 功能 | 示例 |
|------|------|------|
| `drift add <path>` | 添加文件到暂存区 | `drift add 章节/第一章.txt` |
| `drift add .` | 添加所有变更 | `drift add .` |
| `drift status` | 查看工作区状态 | `drift status` |
| `drift reset` | 清空暂存区 | `drift reset` |

### 版本命令

| 命令 | 功能 | 示例 |
|------|------|------|
| `drift save` | 提交暂存区内容 | `drift save -m "完成前三章"` |
| `drift list` | 查看版本历史 | `drift list` |
| `drift show <id>` | 查看版本详情 | `drift show v1` |
| `drift export <id>` | 导出版本 | `drift export v1 -o ./交付` |
| `drift restore <id>` | 回退到指定版本 | `drift restore v1` |

### 分支命令

| 命令 | 功能 | 示例 |
|------|------|------|
| `drift branch <name>` | 创建分支 | `drift branch 方案A` |
| `drift branch list` | 查看所有分支 | `drift branch list` |
| `drift switch <name>` | 切换分支 | `drift switch 方案A` |

### 对比命令

| 命令 | 功能 | 示例 |
|------|------|------|
| `drift diff` | 查看工作区与暂存区差异 | `drift diff` |
| `drift diff <v1> <v2>` | 对比两个版本 | `drift diff v1 v3` |

## 命令详细说明

### drift init

初始化一个新的Drift项目。

```bash
drift init
```

**行为：**
- 在当前目录创建`.drift`文件夹
- 初始化存储结构
- 创建默认配置文件

**输出：**
```
Drift项目已初始化
```

### drift add

添加文件到暂存区。

```bash
# 添加单个文件
drift add 章节/第一章.txt

# 添加目录
drift add 章节/

# 添加所有变更
drift add .
```

**行为：**
- 计算文件哈希
- 将文件内容存储到objects/blobs/
- 更新暂存区（index）

**输出：**
```
已添加：章节/第一章.txt
已添加：章节/第四章.txt
```

### drift status

查看工作区状态。

```bash
drift status
```

**输出示例：**
```
分支：main

暂存区：
  修改: 章节/第一章.txt
  新增: 章节/第四章.txt

未暂存：
  新增: 素材/新角色.txt
  修改: 大纲.txt
```

### drift reset

清空暂存区。

```bash
drift reset
```

**行为：**
- 清空暂存区内容
- 不影响工作区文件

**输出：**
```
暂存区已清空
```

### drift save

提交暂存区内容为新版本。

```bash
# 带备注提交
drift save -m "完成前三章"

# 使用默认备注
drift save
```

**行为：**
- 从暂存区读取文件列表
- 构建Tree对象
- 创建Commit对象
- 更新分支引用
- 清空暂存区

**输出：**
```
已保存版本：v3
备注：完成前三章
```

### drift list

查看版本历史。

```bash
drift list
```

**输出示例：**
```
版本历史：

v3  2024-01-15 10:30  完成前四章
v2  2024-01-15 09:00  修改配色
v1  2024-01-14 15:00  初稿完成
```

### drift show

查看版本详情。

```bash
drift show v1
```

**输出示例：**
```
版本：v1
备注：初稿完成
时间：2024-01-14 15:00:00
分支：main
父版本：无

文件列表：
  章节/第一章.txt (1.2KB)
  章节/第二章.txt (1.5KB)
  素材/人物设定.txt (0.8KB)
  大纲.txt (2.1KB)
```

### drift export

导出指定版本到指定位置。

```bash
# 导出到指定目录
drift export v1 -o ./交付客户

# 导出到当前目录
drift export v1
```

**行为：**
- 读取版本的Tree对象
- 递归还原目录结构
- 复制文件到目标位置

**输出：**
```
已导出版本v1到：./交付客户
```

### drift restore

回退到指定版本。

```bash
drift restore v1
```

**行为：**
- 将工作区文件恢复到指定版本状态
- 创建一个新版本记录这次回退

**输出：**
```
已回退到版本v1
注意：当前工作区文件已更新
```

### drift branch

创建或查看分支。

```bash
# 创建分支
drift branch 方案A

# 查看所有分支
drift branch list
```

**创建分支输出：**
```
已创建分支：方案A
```

**查看分支输出：**
```
分支列表：

* main    (当前)  v3
  方案A           v1
  方案B           v2
```

### drift switch

切换到指定分支。

```bash
drift switch 方案A
```

**行为：**
- 更新当前分支引用
- 将工作区文件切换到分支版本

**输出：**
```
已切换到分支：方案A
```

### drift diff

对比文件差异。

```bash
# 查看工作区与暂存区差异
drift diff

# 对比两个版本
drift diff v1 v3
```

**输出示例（文本文件）：**
```
--- v1/章节/第一章.txt
+++ v3/章节/第一章.txt
@@ -1,5 +1,8 @@
 第一章 开始
 
-这是一个故事的开始。
+这是一个关于冒险的故事。
+
+主角是一个年轻的旅行者。
 
 天气晴朗。
```

**输出示例（二进制文件）：**
```
二进制文件：素材/封面.psd
  v1: 1.2MB
  v3: 1.5MB
  状态：已修改
```

## 版本ID命名规则

| 格式 | 示例 | 说明 |
|------|------|------|
| 自动递增 | v1, v2, v3 | 默认命名 |
| 自定义 | v1-初稿 | 带备注的命名 |
| 分支前缀 | 方案A/v1 | 分支版本 |

## 错误处理

### 常见错误

| 错误 | 原因 | 解决方案 |
|------|------|----------|
| `drift: 未初始化项目` | 未运行drift init | 运行drift init |
| `drift: 文件不存在` | 路径错误 | 检查文件路径 |
| `drift: 暂存区为空` | 未添加文件 | 运行drift add |
| `drift: 版本不存在` | 版本ID错误 | 运行drift list查看 |

### 帮助信息

```bash
drift --help
drift <command> --help
```

## 退出码

| 退出码 | 含义 |
|--------|------|
| 0 | 成功 |
| 1 | 一般错误 |
| 2 | 参数错误 |
| 3 | 文件系统错误 |
