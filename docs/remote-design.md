# 远程同步设计文档

> 状态：已实现（WebDAV + SMB 协议，push/pull/clone/ls-remote 命令，配置与凭据管理）
> 最后更新：2026-07-10

## 1. 目标与定位

drift 的远程功能定位为**纯备份与多机同步**：

- 本地 `.drift/` 始终是主存储，所有命令默认走本地，零延迟。
- 远程只存储对象副本，用于：① 跨设备同步工作进度；② 异地备份。
- 不追求"远程为主存储"或"在线协作"等复杂场景。

这一定位让架构大幅简化：无需处理在线编辑冲突、无需远程事务、无需细粒度权限。

## 2. 同步模型

### 2.1 对象级内容寻址同步

drift 的 chunks 和 snapshots 本身就是内容寻址的（文件名 = hash），天然适合对象级同步：

- **push**：扫描本地对象 → 对每个对象 `Stat` 远程 → 不存在则上传 → 更新远程 refs
- **pull**：列出远程对象 → 对每个本地不存在的对象下载 → 更新本地 refs（仅追加）

由于对象以 hash 命名，**同名对象内容必然相同**，所以同步过程无需任何冲突解决逻辑。

### 2.2 默认全仓库同步

- `drift push` / `drift pull` 默认同步整个仓库（所有分支、tag、快照、chunk）。
- `--branch <name>` 参数可限定单个分支：只同步该分支链上的快照及其引用的 chunk，以及该分支的 ref。

### 2.3 refs 合并策略

快照和 chunk 无冲突（hash 寻址），但 refs（分支/tag 指针）可能冲突：

- **同名 ref 指向相同 hash**：无操作，幂等。
- **本地有、远程无**：push 时上传 ref；pull 时保留本地不动。
- **远程有、本地无**：pull 时下载 ref；push 时不动远程。
- **同名 ref 指向不同 hash**（真正的分叉）：
  - **push**：拒绝覆盖远程 ref，报错提示用户先 pull。
  - **pull**：本地 ref 保留不动，远程版本另存为 `<name>.remote`，提示用户手动决定如何处理（例如新建分支接住远程历史）。

这一策略保证**永不丢失数据**，且行为可预测。

## 3. 协议与库选型

### 3.1 选型原则

- 纯 Go 实现，无 CGO 依赖（保持单二进制跨平台编译）。
- 活跃维护，社区认可。
- 提供文件级读写抽象，能适配到统一接口。

### 3.2 选定库

| 协议 | 库 | 许可证 | 维护状态 | 适用场景 |
|------|-----|--------|----------|----------|
| WebDAV | `github.com/studio-b12/gowebdav` | BSD-3 | 2026-01 仍有提交 | 网盘（坚果云、Nextcloud、ownCloud、群晖 WebDAV） |
| SMB | `github.com/hirochachacha/go-smb2` | BSD-2 | 社区维护 | NAS（群晖、威联通原生 SMB 共享） |

**WebDAV 为主力协议**：覆盖面最广，纯 HTTP，穿透防火墙友好，几乎所有网盘和 NAS 都支持。

**SMB 为补充**：Windows 共享和 NAS 原生协议，局域网内速度快，但公网友好度差。

### 3.3 不选的方案及理由

- **S3/GCS 等对象存储 SDK**：对象存储非标准文件协议，且 SDK 重，留待后续按需支持。
- **rsync**：需要 ssh/sshd，对网盘不适用。
- **自研协议 over HTTP**：重复造轮子，违反"优先用成熟库"原则。

## 4. 架构设计

### 4.1 新增包：`internal/remote/`

```
internal/remote/
    remote.go        RemoteFS 接口 + ProtocolFactory 注册表（init 注册模式）
    config.go        RemoteConfig 结构 + remotes.json 读写（仓库级）
    credentials.go   Credentials 结构 + credentials.json 读写（用户级）
    webdav.go        WebDAV 实现（init 中 Register("webdav", ...)）
    smb.go           SMB 实现（init 中 Register("smb", ...)）
    sync.go          Push/Pull 核心逻辑
    mock_remote.go   内存 mock RemoteFS（仅测试用）
    sync_test.go     同步逻辑测试
```

**协议注册采用 init 注册模式**（与 `internal/filetype/init.go` 引擎注册一致）：每个协议实现文件在 `init()` 中调用 `remote.Register(name, factory)`，sync.go 通过 `remote.NewRemoteFS(cfg)` 按名称查找已注册的 factory。新增协议 = 新增一个文件 + init 注册，无需修改 remote.go 的注册表或 sync.go。

### 4.2 RemoteFS 接口与协议注册

所有协议的库都提供文件读写能力，抽象成统一接口：

```go
// RemoteFS is the abstract filesystem interface that all remote protocols
// implement. push/pull logic operates solely against this interface, so
// adding a new protocol (e.g. S3) only requires implementing it here.
type RemoteFS interface {
    // Stat returns metadata for a remote path, or os.ErrNotExist.
    Stat(path string) (*RemoteInfo, error)
    // Read opens a remote file for reading. Caller must close the reader.
    Read(path string) (io.ReadCloser, error)
    // Write uploads a file. Path's parent directories are created if needed.
    Write(path string, r io.Reader) error
    // Remove deletes a remote file. NotFound is not an error.
    Remove(path string) error
    // List enumerates entries under a directory path.
    List(path string) ([]RemoteInfo, error)
    // MkdirAll creates a directory tree.
    MkdirAll(path string) error
    // Close releases protocol-level resources (connections, sessions).
    Close() error
}

// RemoteInfo is the metadata returned by Stat/List.
type RemoteInfo struct {
    Path     string
    Size     int64
    IsDir    bool
    ModTime  time.Time
}
```

**协议注册表**（init 注册模式，与 `internal/filetype/init.go` 一致）：

```go
// ProtocolFactory constructs a RemoteFS from a RemoteConfig. Each protocol
// implementation registers its factory in init().
type ProtocolFactory func(cfg RemoteConfig) (RemoteFS, error)

var protocols = map[string]ProtocolFactory{}

// Register adds a protocol factory under the given name. Called from each
// protocol implementation's init().
func Register(name string, f ProtocolFactory) {
    protocols[name] = f
}

// NewRemoteFS looks up the registered factory for cfg.Type and constructs
// a RemoteFS. Returns ErrUnsupported for unknown protocol names.
func NewRemoteFS(cfg RemoteConfig) (RemoteFS, error) {
    f, ok := protocols[cfg.Type]
    if !ok {
        return nil, fmt.Errorf("unknown protocol %q: %w", cfg.Type, ErrUnsupported)
    }
    return f(cfg)
}
```

```go
// webdav.go
func init() { Register("webdav", NewWebDAVFS) }

// smb.go
func init() { Register("smb", NewSMBFS) }

// 未来 s3.go — 新增协议只需这一个文件，零修改其他文件
// func init() { Register("s3", NewS3FS) }
```

**扩展点**：新增协议只需 ① 实现 RemoteFS 接口 ② 在 init() 中 Register。sync.go、注册表、cmd 层均无需修改。

`ErrUnsupported` 是 `internal/remote` 包的 sentinel error，用于未知协议名。与 `internal/storage` 的 `ErrUnsupported` 同名但属不同包，各自独立。

### 4.3 配置结构（两级分离）

配置遵循"**remote 定义是仓库属性，凭据是用户属性**"原则，物理分离：

| 文件 | 位置 | 内容 | 敏感度 | 是否随仓库传输 |
|------|------|------|--------|---------------|
| `remotes.json` | `<repo>/.drift/remotes.json` | remote 定义（name/url/type/user/options） | 中 | 可安全随仓库走 |
| `credentials.json` | `<UserConfigDir>/drift/credentials.json` | 凭据（host+user → password） | 高 | 永不随仓库走 |

`<UserConfigDir>` 由 Go 标准库 `os.UserConfigDir()` 返回：Linux/macOS 为 `~/.config`，Windows 为 `%APPDATA%`。

**分离的理由**：
- 同一台机器上多个仓库通常备份到同一 NAS，凭据应复用而非重复配置
- 仓库目录可能被打包/复制/发送给他人，凭据不应跟随
- remote 定义与仓库绑定（clone 别人仓库时 remote 已就位），属仓库级
- 凭据是机器/用户级属性，与具体仓库无关

**`<repo>/.drift/remotes.json`**（仓库级，可分享）：

```go
// RemoteConfig describes a single configured remote. Password is NOT stored
// here — it lives in the user-level credentials.json, matched by host+user.
// Protocol-specific fields (SMB domain, S3 region/bucket, SFTP key path, etc.)
// go in Options so adding a new protocol never changes this struct.
type RemoteConfig struct {
    Name    string            `json:"name"`
    Type    string            `json:"type"`             // "webdav" | "smb" | 未来扩展
    URL     string            `json:"url"`              // webdav: https://host[:port]/path; smb: smb://host[:port]/share[/path]
    User    string            `json:"user"`
    Options map[string]string `json:"options,omitempty"` // 协议专属字段
}

// RemotesFile is the on-disk format of .drift/remotes.json.
type RemotesFile struct {
    Remotes []RemoteConfig `json:"remotes"`
}
```

**Options 字段按协议约定**（各协议实现自行解析）：
- SMB：`Options["domain"] = "WORKGROUP"`
- 未来 S3：`Options["region"] = "us-east-1"`，`Options["bucket"] = "my-bucket"`
- 未来 SFTP：`Options["key_path"] = "~/.ssh/id_rsa"`，`Options["port"] = "22"`

**新增协议无需修改 RemoteConfig 结构**，只在协议实现文件中约定并解析自己的 Options 键。这与 §4.2 的 init 注册模式配合，实现"开闭原则"——对扩展开放，对修改关闭。

**`<UserConfigDir>/drift/credentials.json`**（用户级，敏感）：

```go
// Credential is a single (host, user) → password entry. The match key is
// host+user so the same NAS can have multiple accounts.
type Credential struct {
    Host     string `json:"host"`
    User     string `json:"user"`
    Password string `json:"password"`
}

// CredentialsFile is the on-disk format of credentials.json.
type CredentialsFile struct {
    Credentials []Credential `json:"credentials"`
}
```

文件权限强制 0600（仅用户可读），首次创建即设置。Go 标准库 `os.OpenFile` 配合显式 perm 即可，无需引入新依赖。

**凭据解析流程**（push/pull 时）：
1. 读 `<repo>/.drift/remotes.json` 找到 remote 定义（含 url + user）
2. 解析 url 得到 host
3. 读 `<UserConfigDir>/drift/credentials.json` 按 `host + user` 匹配 password
4. 找不到 → 交互式询问密码（`golang.org/x/term` 不回显），询问是否保存
5. 用户拒绝保存 → 本次会话内存暂存，退出后丢失

**SMB URL 格式**：`smb://host[:port]/share[/path]`。库 `go-smb2` 的 API 是 `net.Dial("tcp", "host:port")` + `s.Logon(share)`，`RemoteConfig` 解析时拆分 URL 为 host/port/share 三部分传给库。端口缺省 445。

**remotes.json 不被同步**：远程仓库不存 remotes.json，避免 remote 配置泄露。push/pull 只同步仓库对象（chunks/snapshots/manifests/refs/HEAD/config），不同步 remotes.json 本身。credentials.json 在用户目录下，天然不在 `.drift/` 内，更不会被同步。

**drift remote remove 不删凭据**：删除 remotes.json 条目时，**不删除** credentials.json 对应条目（凭据可能被其他仓库复用）。命令输出提示"凭据保留在用户级配置，如需删除请手动编辑 `<path>`"。

**drift remote set-url 的 host 变化**：若新 url 的 host 与旧 url 不同，原凭据可能失效，命令输出 warning 提示"host 已变更，可能需要重新配置密码"。

### 4.4 远程仓库布局

远程目录镜像本地 `.drift/` 结构，复用 `filesystem/layout.go` 的常量。**所有对象文件名均使用完整 hash（64 位十六进制），两级目录分片**，与本地存储格式完全一致：

```
<remote-root>/
  chunks/
    <hash[:2]>/<hash[2:]>           内容寻址 chunk（1字节header+payload，可能压缩）
  snapshots/
    <hash[:2]>/<hash[2:]>           完整 snapshot（protobuf 编码）
  manifests/
    <hash[:2]>/<hash[2:]>           轻量清单（protobuf，加速 ListSnapshots）
  refs/
    heads/
      main                          分支 ref（内容为 hex hash）
      dev
    tags/
      v1                            tag ref（内容为 hex hash）
```

**不同步 HEAD**：HEAD 是工作区状态（当前分支），每台机器独立，不同步。

**不同步 config**：config 是仓库级行为配置，每台机器独立，不同步。

**不同步 index**：`index` 文件是工作区缓存，可从当前分支 tip 快照重建。pull 后由 porcelain 层调用 `RebuildIndexFromSnapshot` 重新生成 index。

**不同步 previews**：当前 preview 是 stub（未实现），无需同步。

**chunk 文件格式**（与本地一致，见 `filesystem/chunk.go`）：
```
[1字节 header][payload]
header bit 0: 0=未压缩, 1=zstd压缩
payload: 原始数据 或 zstd压缩数据
```

**snapshot/manifest 文件格式**（与本地一致）：protobuf 编码，无额外 header。

**不维护额外的 manifest.json**：每次 push/pull 直接 `List` 远程目录判断差异。WebDAV 的 PROPFIND 和 SMB 的 List 都能高效列出目录。后续若大仓库性能不足再加协调清单。

### 4.5 与现有架构的集成

```
cmd/
  remote.go          新增：remote add/remove/list/set-url/test 命令
  remote_add.go      新增：交互式配置向导
  push.go            新增：push 命令
  pull.go            新增：pull 命令
internal/
  remote/            新增包（见 4.1）
  porcelain/
    sync.go          新增：Push/Pull 业务逻辑封装
    sync_test.go
    workspace.go     修改：提取 RebuildIndexFromSnapshot 复用函数
```

**分层不变**：cmd 只做 CLI 解析和输出，业务逻辑在 porcelain，协议实现在 remote。

**porcelain/sync.go 的职责**：
- 调用 `internal/remote` 构造 RemoteFS 实例
- 通过现有 `storage.Storer` 接口读写本地对象（见下方"对象读写方案"）
- 协调 push/pull 流程：对象枚举、差异判断、传输、refs 合并
- pull 后触发 index 重建（调用 `RebuildIndexFromSnapshot`）
- 输出统计信息给 cmd 层

sync.go 只依赖 `storage.Storer` 接口和 `remote.RemoteFS` 接口，不依赖具体后端实现，便于用 memory backend + mock RemoteFS 做单元测试。

**对象读写方案**（解决 Storer 接口无法直接读写原始存储字节的问题）：

push/pull 通过 Storer 接口的高层方法读写对象，不直接操作文件系统：

| 操作 | Storer 方法 | 说明 |
|------|------------|------|
| push snapshot | `GetSnapshot(id)` → `proto.Marshal(SnapshotToProto)` → `remote.Write` | 重新序列化后上传，protobuf 序列化确定性保证字节稳定 |
| push manifest | `GetSnapshot(id)` → `core.SnapshotToManifest` → `proto.Marshal` → `remote.Write` | manifest 从 snapshot 派生，不单独存储 |
| push chunk | `GetChunk(hash)` → 按 `Chunk.Flags` 重新压缩+加 header → `remote.Write` | 重新编码后上传，hash 不变（hash 是对未压缩 data 的 hash） |
| pull snapshot | `remote.Read` → `proto.Unmarshal` → `PutSnapshot(snap)` | PutSnapshot 自动写 manifest |
| pull chunk | `remote.Read` → 解析 header → `PutChunk(&Chunk{Data, Flags})` | PutChunk 按本地压缩配置存储 |

**关键性质**：chunk hash 是对**未压缩 data** 的 BLAKE3 hash，不是对存储字节的 hash。所以同一 hash 的 chunk 可以有不同的存储字节（不同压缩 level），但内容相同。push 时重新压缩不影响正确性，远程已存在该 hash 的文件（不管压缩 level）就跳过。

**RebuildIndexFromSnapshot**（P2-C 解决方案）：

当前 index 重建逻辑内联在 restore/switch/save 三处。sync.go 实现前先提取一个可复用函数：

```go
// RebuildIndexFromSnapshot regenerates the staging index from a snapshot's
// file entries. Used by restore, switch, and pull (after syncing new snapshots).
func RebuildIndexFromSnapshot(ctx context.Context, store storage.Storer, snapID core.SnapshotID) error {
    snap, err := store.GetSnapshot(ctx, snapID)
    if err != nil {
        return fmt.Errorf("get snapshot: %w", err)
    }
    newIndex := &core.Index{UpdatedAt: time.Now().Unix()}
    for _, entry := range snap.Files {
        if err := ctx.Err(); err != nil {
            return err
        }
        newIndex.Entries = append(newIndex.Entries, core.IndexEntry{
            Path:    entry.Path,
            Size:    entry.Size,
            ModTime: entry.ModTime,
            Chunks:  entry.Chunks,
            Hash:    entry.Hash,
        })
    }
    return store.SetIndex(ctx, newIndex)
}
```

restore/switch/pull 复用此函数，消除重复代码。

## 5. 命令设计

### 5.1 remote 命令组

```
drift remote add <name> <url> [flags]    添加远程（url 为位置参数）
drift remote remove <name>              删除远程
drift remote list                       列出所有远程
drift remote set-url <name> <url>       修改远程 URL
drift remote test <name>                测试连接
```

**`drift remote add` flags**：
- `--type <webdav|smb>`：协议类型，默认 `webdav`
- `--url <u>`：远程 URL
- `--user <u>`：用户名
- `--password <p>`：密码（若提供则保存到用户级 credentials.json；若不提供则交互式询问）
- `--option <key=value>`：协议专属字段，可重复（如 `--option domain=WORKGROUP`），写入 RemoteConfig.Options
- `--no-save-password`：不把密码保存到 credentials.json（仅本次会话内存暂存）

端口通过 URL 指定（`smb://host:445/share` 或 `https://host:8443/dav`），不单独设 flag。协议专属字段统一通过 `--option` 传入，避免为每个协议增加专属 flag（扩展性考虑，见 §4.3 Options 设计）。

**交互式配置**：当 `drift remote add` 未提供 `--url` 或 `--user` 时，进入交互式向导（`--type` 有默认值 webdav，不作为触发条件）。交互式会依次询问协议、URL、用户名、密码（不回显），按协议询问专属字段（SMB 询问 domain），询问是否保存密码到用户级 credentials.json，并在末尾询问是否测试连接：

```
$ drift remote add mynas
Protocol (webdav/smb) [webdav]: webdav
URL: https://nas.example.com/dav/drift-backup
Username: alice
Password: ****
Save password to ~/.config/drift/credentials.json? [Y/n]: y
Test connection now? [Y/n]: y
✓ Connected.
Remote 'mynas' added (credentials saved to user-level config).
```

```
$ drift remote add mynas --type smb --url smb://nas/share/drift --user alice --password secret --option domain=WORKGROUP
（非交互式，remote 定义写入 .drift/remotes.json，密码写入用户级 credentials.json）
```

```
$ drift remote add mynas --type webdav --url https://nas/dav --user alice
（部分参数缺失，交互式只询问密码）
Password: ****
Save password to ~/.config/drift/credentials.json? [Y/n]: y
Remote 'mynas' added.
```

交互式输入使用 `golang.org/x/term` 读取密码（不回显），避免引入额外依赖。命令行参数与交互式可混用：提供了 `--type` 但没提供 `--password`，则只交互式询问密码。

**写入两个文件**：
- `--password` 提供 → remote 定义写 `<repo>/.drift/remotes.json`，密码写 `<UserConfigDir>/drift/credentials.json`（按 host+user 匹配键）
- `--no-save-password` → remote 定义写 remotes.json，密码仅本次会话内存暂存
- 交互式拒绝保存 → 同 `--no-save-password`

### 5.2 push / pull 命令

```
drift push [remote-name] [--branch <b>] [--dry-run] [--quiet|-q] [--json]
drift pull [remote-name] [--branch <b>] [--dry-run] [--quiet|-q] [--json]
```

**参数说明**：
- `remote-name`：可选，默认用配置中的第一个远程（若只有一个）或报错提示指定
- `--branch <b>`：只同步指定分支链（默认全仓库）
- `--dry-run`：只显示会同步什么，不实际执行
- `--quiet`：成功时无输出
- `--json`：JSON 输出

**push 输出示例**：
```
>>> Pushing to 'mynas' [ok]
  snapshots:  3 uploaded, 2 already present
  manifests:  3 uploaded, 2 already present
  chunks:     12 uploaded, 45 already present
  refs:       2 branches, 1 tag updated
```

**pull 输出示例**：
```
>>> Pulling from 'mynas' [ok]
  snapshots:  4 downloaded, 1 already present
  chunks:     8 downloaded, 49 already present
  refs:       1 branch updated, 1 new tag
  index:      rebuilt (branch 'main' tip advanced)
  hint: branch 'main' tip advanced. Working directory is out of sync.
        run 'drift restore' to update your files.
```

**refs 分叉提示**：
```
>>> Pulling from 'mynas' [warning]
  ...
  hint: branch 'dev' diverged. Local kept; remote saved as 'dev.remote'.
        inspect with 'drift log --branch dev.remote', or rename with
        'drift branch rename dev.remote dev-remote'.
```

## 6. push/pull 算法详述

### 6.1 push 流程

```
1. 解析 remote-name，加载 RemoteConfig
2. 构造 RemoteFS 实例，Test 连通性
3. 确定同步范围：
   - 若 --branch：walk 该分支 tip 的 PrevID 链，收集 snapshot hashes
     + 这些 snapshot 引用的所有 chunk hashes
   - 若全仓库：ListSnapshots() 收集所有 snapshot hashes，
     再从每个 snapshot 读 FileEntry.Chunks 收集 chunk hashes
     （不直接 List chunks/，避免上传孤儿 chunk）
4. 上传 snapshots + manifests：
   for each snapshot hash:
       snap_remote = snapshots/<hash[:2]>/<hash[2:]>
       if remote.Stat(snap_remote) == ErrNotExist:
           snap = store.GetSnapshot(id)
           上传 proto.Marshal(SnapshotToProto(snap, true)) 到 snap_remote
           manifest_remote = manifests/<hash[:2]>/<hash[2:]>
           上传 proto.Marshal(SnapshotToManifest(snap)) 到 manifest_remote
5. 上传 chunks：
   for each chunk hash:
       chunk_remote = chunks/<hash[:2]>/<hash[2:]>
       if remote.Stat(chunk_remote) == ErrNotExist:
           chunk = store.GetChunk(hash)
           按 chunk.Flags 重新压缩+加 1 字节 header
           上传到 chunk_remote
6. 上传 refs（仅当 ref 指向的 snapshot 已存在于远程）：
   for each local ref (heads/*, tags/*):
       target = ref.Target
       if remote.Stat(snapshots/<target[:2]>/<target[2:]>) 存在:
           remote_ref = refs/<heads|tags>/<name>
           existing = remote.Read(remote_ref)  // 可能不存在
           if existing 不存在: 写入本地 ref
           elif existing == local ref.Target: 跳过
           else: 报错 "ref 分叉，请先 pull"
7. 输出统计
```

**不同步 HEAD**（P2-A）：HEAD 是工作区状态（"当前在哪个分支"），多机协作时不同机器的当前分支本应不同。同步 HEAD 会把 A 的当前分支覆盖到 B，破坏 B 的工作上下文。

**不同步 config**（P2-B）：config 是仓库级行为配置（compression/auto-save 等），每台机器可能有自己的偏好。push 覆盖远程 + pull 不覆盖本地 的不对称会导致 config 丢失。改为**完全不同步 config**，每个仓库独立配置。

### 6.2 pull 流程

```
1. 解析 remote-name，加载 RemoteConfig
2. 构造 RemoteFS 实例，Test 连通性
3. 确定同步范围：
   - 若 --branch：先拉远程 refs/heads/<branch> 得到 tip，walk 远程 PrevID 链
   - 若全仓库：List 远程 manifests/ 目录收集所有 snapshot hashes，
     再从每个 snapshot 读 chunk hashes
4. 下载 snapshots + manifests：
   for each remote snapshot hash:
       if 本地 HasSnapshot(id) == false:
           data = remote.Read(snapshots/<hash[:2]>/<hash[2:]>)
           snap = proto.Unmarshal(data)
           store.PutSnapshot(snap)  // 自动写 manifest
5. 下载 chunks：
   for each remote chunk hash:
       if 本地 HasChunk(hash) == false:
           data = remote.Read(chunks/<hash[:2]>/<hash[2:]>)
           解析 1 字节 header → flags
           解压 payload（若 flags 标记压缩）
           store.PutChunk(&Chunk{Hash, Data, Flags})
6. 合并 refs（仅追加，不覆盖）：
   for each remote ref (heads/*, tags/*):
       local_ref = 读本地对应 ref
       if local_ref 不存在: 直接写入
       elif local_ref.Target == remote_ref.Target: 跳过
       else: 本地保留不动，远程版本写为 refs/heads/<name>.remote
             （或 refs/tags/<name>.remote），提示分叉
7. 重建 index：
   若 pull 了新快照且当前分支 tip 发生变化，调用
   RebuildIndexFromSnapshot(ctx, store, currentBranchTip) 重建 index
8. 输出统计
```

**pull 不触碰工作区文件**（P1-A）：pull 只更新 `.drift/` 内部状态（snapshots/chunks/refs/index），不修改工作区文件。若当前分支 tip 变化，pull 输出提示：

```
hint: branch 'main' tip advanced. Working directory is out of sync.
      run 'drift restore' to update your files.
```

用户自行决定是否运行 `drift restore` 同步工作区。pull 前不强制检测未保存变更（纯同步语义），但 pull 后 `drift status` 会显示工作区与新 index 的差异。

### 6.3 refs 分叉的完整处理流程

push 拒绝 + pull 另存为 `.remote` 的组合，形成可预测的分叉解决路径：

```
场景：本地 dev -> hashA，远程 dev -> hashB（分叉）

1. 本地 push dev → 远程 dev 已存在且 != hashA → 报错 "请先 pull"
2. 本地 pull dev → 本地 dev 保留 hashA，远程 hashB 写入 dev.remote
3. 用户检查 dev.remote 历史，决定如何处理：
   a. 若想要远程版本：drift branch delete dev && drift branch create dev
      （然后 branch 的 tip 会是 dev.remote 指向的 hashB）
   b. 若想保留本地版本：忽略 dev.remote，继续在 dev 上工作
   c. 若想合并：手动 cherry-pick（drift 暂不支持自动 merge）
4. 再次 push dev → 远程 dev.remote 已存在（不会再次报错），
   但 dev 仍指向 hashA，远程 dev 仍是 hashB → 仍然报错
   → 用户需先在远程处理 dev，或用 --force（后续版本）
```

第一版不提供 `--force`，强制用户理解分叉后再处理。

### 6.4 性能考量

- **Stat 去重**：每个对象上传前先 Stat，避免重复传输。WebDAV 的 HEAD/PROPFIND 和 SMB 的 Stat 都是单次请求，开销小。
- **并发上传**：chunk 上传/下载已实现并发（`concurrency = 8`，goroutine + semaphore 有界并发），RemoteFS 实现需线程安全。后续可加 `--jobs N` 参数让用户自定义并发度。
- **断点续传**：对象级同步天然支持断点——中断后重跑，已存在的对象会被跳过。
- **增量**：由于内容寻址，只传新对象。第二次 push 只传新增快照和 chunk。

## 7. 安全性

### 7.1 凭据存储

**两级分离**（见 §4.3）：
- remote 定义（name/url/type/user/options）明文存 `<repo>/.drift/remotes.json`，类比 git 的 `.git/config` 存 remote URL。不含密码，可安全随仓库传输。options 收纳协议专属字段（如 SMB 的 domain）。
- 凭据（password）明文存 `<UserConfigDir>/drift/credentials.json`，文件权限 0600。类比 git 早期明文存 `~/.git-credentials`。后续可加 `--credential-helper` 调用系统钥匙串（macOS Keychain / Windows Credential Manager / Linux Secret Service）。

**仓库级 remotes.json 的安全说明**：`.drift/` 目录在 `fsutil.Walk` 中始终被跳过（见 `internal/util/fsutil/walk.go` 第 74 行 `isDriftDir` 检查），所以 `remotes.json` 存在 `.drift/` 下天然不会被 `drift save` 跟踪，无需额外配置 `.driftignore`。

**用户级 credentials.json 的安全说明**：位于 `<UserConfigDir>/drift/` 下，不在任何仓库目录内，天然不会被任何仓库的 `drift save` 跟踪。0600 权限确保仅当前用户可读。

**演进路径**：v1 明文 JSON → v2 `--credential-helper` 调用系统钥匙串。remotes.json 和 credentials.json 的格式无需 breaking change，钥匙串作为凭据的另一种存储后端，按 host+user 同样匹配。

### 7.2 传输安全

- WebDAV：强制建议 HTTPS，HTTP 时给出警告。
- SMB：局域网使用，公网不推荐。

### 7.3 远程隔离

不同远程仓库独立，push/pull 只操作指定远程，不会交叉污染。

## 8. 测试策略

### 8.1 单元测试

- `RemoteFS` 接口的内存 mock 实现（`mock_remote.go`），用于测试 sync 逻辑：
  - push 新对象、push 已存在对象、push refs 分叉
  - pull 新对象、pull 已存在对象、pull refs 分叉
  - `--branch` 限定范围
- 配置读写测试（两级分离）：
  - remotes.json 序列化/反序列化、缺字段容错
  - credentials.json 序列化/反序列化、0600 权限验证
  - 按 host+user 匹配凭据、找不到时返回 not-found
  - 多 remote 共享同一 host+user 凭据的场景
  - `drift remote remove` 不删 credentials.json 条目的验证

### 8.2 集成测试

- WebDAV：用 `golang.org/x/net/webdav` 起本地 WebDAV server，跑完整 push/pull 循环。
- SMB：难以在 CI 起真实 SMB server，集成测试手动执行或 skip。

### 8.3 手工测试清单

- 坚果云 WebDAV push/pull
- 群晖 NAS WebDAV 和 SMB 各测一次
- 本地起 Nextcloud Docker，测 WebDAV
- 多端分叉场景：A push → B pull → B push → A pull

## 9. 实现路线图

按可独立验证的增量推进，每步可单独提交：

1. **RemoteFS 接口 + 协议注册表 + mock 实现 + sync 逻辑**（不含真实协议）
   - `internal/remote/remote.go`（含 `ProtocolFactory` 注册表 + `Register`/`NewRemoteFS`）、`sync.go`、`sync_test.go`
   - mock RemoteFS + memory Storer 测试 push/pull 全流程
   - 覆盖：新对象同步、已存在跳过、refs 合并、refs 分叉（push 拒绝 + pull 另存 `.remote`）、index 重建、config 合并提示
   - 全部用 mock 测试通过

2. **WebDAV 实现**
   - `internal/remote/webdav.go`（init 中 `Register("webdav", ...)`）
   - 引入 `github.com/studio-b12/gowebdav`
   - 用 `golang.org/x/net/webdav` 起本地 server 做集成测试
   - 验证大文件上传、断点续传、并发安全

3. **配置管理 + remote 命令组**
   - `internal/remote/config.go`（仓库级 remotes.json，含 Options 字段）、`internal/remote/credentials.go`（用户级 credentials.json）
   - `cmd/remote.go`、`cmd/remote_add.go`
   - 交互式配置向导（`golang.org/x/term` 读密码），分别询问 remote 字段和密码保存
   - `--option key=value` 解析协议专属字段
   - `drift remote add/remove/list/set-url/test` 全部可用
   - 两级配置读写测试（含 0600 权限、host+user 匹配、remove 不删凭据）

4. **push/pull 命令**
   - `internal/porcelain/sync.go`、`cmd/push.go`、`cmd/pull.go`
   - 端到端 push/pull 可用
   - pull 后 index 重建逻辑
   - config 差异 warning 输出

5. **SMB 实现**
   - `internal/remote/smb.go`（init 中 `Register("smb", ...)`）
   - 引入 `github.com/hirochachacha/go-smb2`
   - SMB URL 解析（`smb://host[:port]/share[/path]`）
   - 解析 `Options["domain"]` 传给 go-smb2
   - 手工测试 NAS

6. **优化（按需）**
   - 并发上传 `--jobs N`
   - 进度条
   - credential helper（系统钥匙串）

**扩展协议示例**：未来加 S3 时，只需新增 `internal/remote/s3.go`（实现 RemoteFS 接口 + init 注册），无需改 remote.go / sync.go / config.go / cmd 任何文件。

## 10. 开放问题（待后续决策）

1. **远程 GC**：远程仓库的不可达对象如何清理？是否提供 `drift push --prune`？
2. **大文件分块上传**：WebDAV 对大文件可能超时，是否需要分块上传？drift 的 chunk 已分片（默认 128KB-512KB），单个 chunk 体积小，WebDAV 单次 PUT 足够，无需额外分块。
3. **带宽限制**：`--bandwidth` 参数是否需要？第一版不加。
4. **远程只读模式**：是否支持配置远程为只读（只能 pull 不能 push）？后续按需。
