# pikpak

`pikpak` 是一个用 Go 编写的 PikPak 命令行客户端。支持多账号、缓存 session token 并自动刷新、配额查询、离线任务管理、远程文件浏览与删除、回收站清理，以及一个可断点续传的多线程下载器。

## 特性

- 单个 TOML 配置文件管理多个账号。
- 登录后缓存 session token 并自动刷新（`access_token` / `refresh_token`）。
- 一条命令查询单个或全部账号的配额。
- 离线任务：从单个 URL、多个 URL 或批量文件（每行一个 URL，`#` 开头视为注释）创建任务，可指定目标文件夹（按 ID 或路径）。
- 离线任务列表查询。
- 按路径或文件夹 ID 浏览远程文件。
- 按文件 ID 或远程路径下载文件（支持单个或多个）：
  - 实时进度条（百分比、已传输字节、EMA 平滑速度）；
  - 单连接模式下基于已有本地文件大小自动续传；
  - 多线程 HTTP `Range` 并发下载（默认 4 路）；
  - **跨进程断点续传**：并发下载中断后再次运行同一命令会自动恢复；
  - 服务器不支持 Range 时自动回退到单连接模式；
  - 文件名按 Windows 非法字符（`<>:"/\|?*` 及控制字符）自动清理；
  - **批量下载**：一次命令下载多个文件，按顺序串行处理。
- 文件删除（回收站或永久）。
- 一键清空账号根目录（保留特殊容器）。
- 清空回收站。

## 系统要求

- Go 1.26 或更高版本。
- 一个 PikPak 账号（用户名 + 密码）。

## 安装

```sh
go install github.com/lnzx/pikpak@latest
```

或从源码构建：

```sh
git clone https://github.com/lnzx/pikpak.git
cd pikpak
go build -ldflags="-s -w" -trimpath
```

产物：`pikpak`（Windows 上为 `pikpak.exe`）。

## 配置

`pikpak` 从以下位置读取配置：

- Linux / macOS: `~/.config/pikpak/config.toml`
- Windows: `%USERPROFILE%\.config\pikpak\config.toml`

> 配置目录需要事先存在，加载器不会替你创建。

示例（也可参考仓库根目录的 `config.example.toml`）：

```toml
[accounts]
main   = { username = "user@example.com",   password = "pass1" }
backup = { username = "backup@example.com", password = "pass2" }
```

未指定 `--account` 时，按 alias 字典序选第一个账号。也可以每次显式指定：

```sh
pikpak --account backup quota
pikpak -a backup quota          # 短选项
```

Session token 缓存在 `~/.config/pikpak/sessions/` 下，每个账号一个 JSON 文件，文件名为 `session_<md5(username)>.json`，权限 `0600`；缓存目录在首次写入时以 `0700` 创建。Token 过期或因任何原因重新登录后，新 token 会立即落盘，下次启动无需重新登录。

> **并发限制**：session 写入采用"写临时文件 + `rename` 原子替换"，临时文件名固定为 `<session>.tmp`。请不要对**同一账号**同时运行多个 `pikpak` 进程（例如脚本里并发 `quota`/`task add`），两个进程会争同一 `.tmp` 路径，可能互相覆盖或导致 `rename` 失败。不同账号并发是安全的。

## 安全说明

- 配置文件中的密码是**明文存储**。
- Session token（access/refresh）保存在 sessions 目录，等价于"已登录的浏览器 cookie"。整个 `~/.config/pikpak/` 目录都应当作凭据材料对待，自行确保权限私有。
- 这是非官方客户端：调用的是 PikPak 安卓客户端的 API endpoint，二进制中嵌入了安卓端的 `client_id` / `client_secret`。如果 PikPak 修改 API，客户端可能会失效。

## 命令

```text
pikpak --version

pikpak accounts                                   # 别名: acc

pikpak quota                                      # 别名: q     — 全部账号
pikpak quota -a backup                            #                单个账号

pikpak task add "https://example.com/file.iso"    # 别名: t a
pikpak task add -i urls.txt
pikpak task add --folder /Movies "https://..."
pikpak task list                                  # 别名: t ls
pikpak task delete <task-id> [<task-id> ...]      # 别名: t del, t rm
pikpak task delete <task-id> --delete-files       #                同时删除远端文件
pikpak task clear                                 #                清空所有离线任务
pikpak task clear --delete-files                  #                同时删除远端文件

pikpak file list                                  # 别名: f ls, f list
pikpak file list /Movies
pikpak file list <folder-id>
pikpak file download <file-id>                    # 别名: f d
pikpak file download "/remote/path/file.iso" -o ./downloads
pikpak file download <file-id> <file-id> ...      #                批量下载多个文件
pikpak file download <file-id> --parallel 8 --chunk-min 64MB
pikpak file download <file-id> --force
pikpak file delete <id> [<id> ...]                # 别名: f rm, f del
pikpak file delete <id> --force                   #                永久删除（不可恢复）
pikpak file clear                                 #                把根目录所有内容移入回收站
pikpak file clear --force                         #                永久删除

pikpak trash empty
```

### 全局选项

- `-a, --account <alias>` — 指定账号。省略时使用 alias 字典序第一个账号。如果没有配置任何账号，命令以错误退出。

### `accounts`

列出配置中的账号及其 session 缓存状态。`SESSION` 列显示 `cached` 或 `-`。本命令不发起任何网络请求。

### `quota`

为每个账号打印两个数值：

- `CLOUD_DOWNLOAD(REMAINING/TOTAL)` — 当日离线下载配额，输出为 `剩余/总额` 两个整数（如 `5/5`）。
- `STORAGE` — 已用存储 / 总容量（人类可读）。

未指定 `--account` 时，按顺序查询所有账号；某个账号失败时该行显示 `ERROR`，其余账号继续。

### `task add`

- `[url ...]` — 位置参数，多个 URL 直接列在命令后。
- `-i, --input <file>` — 从文件读取 URL（每行一个，`#` 开头与空行忽略）。
- `-f, --folder <id-or-path>` — 指定目标文件夹。带 `/` 的视为远程路径并解析；否则视为文件夹 ID。

位置参数 URL 与 `--input` 文件的内容会合并提交，每个 URL 独立处理：单个 URL 失败不会中止整批；只要有任何失败，进程以非零码退出，并输出 `submitted N task(s), failed M task(s)`。

每个 URL 输出一行，字段以制表符 (`\t`) 分隔：

```text
submitted	account=main	task_id=...	phase=...	name=...
failed	account=main	url=...	error=...
```

### `task list`

列出选定账号的离线任务，包含 `PENDING`、`RUNNING`、`ERROR`、`COMPLETE` 四个阶段。列：`TASK_ID PHASE PROGRESS FILE_ID NAME MESSAGE`。

### `task delete`

```sh
pikpak task delete <task-id> [<task-id> ...]
pikpak task delete <task-id> --delete-files
pikpak task delete <task-id> -d
```

删除指定的离线任务。加 `-d, --delete-files` 时**同时删除已下载到云盘的远端文件**。

### `task clear`

```sh
pikpak task clear
pikpak task clear --delete-files
pikpak task clear -d
```

清空选定账号的全部离线任务（涵盖 `PENDING`/`RUNNING`/`ERROR`/`COMPLETE` 四个阶段）。加 `-d, --delete-files` 时同时删除远端文件。

> **警告**：本命令不会询问确认。

### `file list` / `file ls`

```sh
pikpak file list /                  # 根目录
pikpak file ls /Movies/Action       # 绝对远程路径
pikpak file ls <folder-id>          # 文件夹 ID（不带前导 /）
```

列：`TYPE SIZE MODIFIED ID NAME`。文件大小以人类可读形式输出，目录显示为空。

**路径解析规则**：参数中包含 `/`（含前导 `/`）视为远程路径，逐层调 `file ls` 解析；否则视为文件夹 ID 原样传给服务器。`file download` 和 `task add --folder` 沿用同样规则。

### `file download`

```sh
pikpak file download <file-id>
pikpak file download "/path/to/file.iso"             # 路径，带前导 /
pikpak file download "subdir/file.iso"               # 含 / 也视为路径
pikpak file download <file-id> -o ./downloads/
pikpak file download <file-id> <file-id> ...         # 批量下载多个文件
pikpak file download <file-id> --parallel 8 --chunk-min 64MB
pikpak file download <file-id> --force
```

**批量下载**：可以一次指定多个文件 ID 或路径，客户端会按顺序串行下载（不并行），每个文件内部仍使用多线程并发连接。某个文件下载失败不会中断后续文件，最后汇总报告失败数量。

选项：

- `-o, --output <path>` — 输出文件或目录。
  - `<path>` 是已存在的目录或以路径分隔符结尾时，自动在末尾拼上远程文件名。
  - 否则 `<path>` 就是最终的输出文件名。
  - 省略时，文件写到当前目录，文件名为远程文件名。
- `-p, --parallel <n>` — 并发 Range 连接数，默认 `4`。传 `1` 强制单连接。
- `-c, --chunk-min <size>` — 启用并发模式所需的最小文件尺寸，默认 `32MB`。接受 `B`、`K`、`M`、`G`、`T`、`P`（可选 `B` 后缀，大小写不敏感）。小于该值的文件始终走单连接。
- `-f, --force` — 删除已有的输出文件及临时文件（`.download` 与 `.download.meta`）后重新下载。

**目标解析**：参数中含 `/` 视为远程路径解析，否则视为文件 ID。

**文件名清理**：远程文件名中的 Windows 非法字符（`<>:"/\|?*` 及 ASCII 控制字符）会被替换成 `_`；末尾的 `.` 与空格会被去掉，避免 Windows 静默丢弃字符导致文件无法定位。

进度输出每 250 ms 刷新一次：

```text
progress:  43.21%  712.34MB/1.61GB  84.12MB/s
```

#### 断点续传行为

**单连接模式**：

若本地文件已存在，客户端会发起 `Range: bytes=<size>-` 续传。如果服务器返回 `200 OK`（忽略了 Range 头），文件会被截断并重新开始。续传基于 `<dest>` 文件本身。

**并发模式（含跨进程续传）**：

- 启动时预分配 `<dest>.download` 临时文件至完整大小，每个 worker 通过 `WriteAt` 直接写到对应偏移。
- 与 `<dest>.download` **同时**维护 `<dest>.download.meta` 元数据文件，记录每个 chunk 当前的 `done` 字节数。
- **下载成功**：原子重命名 `<dest>.download → <dest>`，删除 `.meta`。
- **本次进程内重试**：3 次内部重试共享同一份 in-memory part 状态，瞬时网络错误不会从零开始。
- **跨进程续传**：进程被 Ctrl+C / 崩溃 / 关机后，`<dest>.download` 与 `<dest>.download.meta` 都保留在磁盘上。**下次再次运行同一条 `file download` 命令时**，客户端会自动校验 meta（`expected` 是否匹配当前远程文件大小、各 chunk 区间是否连续覆盖），通过后从上次中断处继续，并打印：

  ```text
  resuming parallel download: 1.50GB / 4.00GB already on disk
  ```

  校验失败、远程文件大小变化、或仅有 `.download` 而 meta 缺失等情况下，会重置重新下载。

- **`--force`**：强制清掉 `<dest>`、`<dest>.download`、`<dest>.download.meta`，从零开始。
- **服务器不支持 Range**：当 Range 请求被回应 `200`，清理 `.download` 和 `.meta`，回退到单连接模式（单连接本来就基于 `<dest>` 续传）。

每个阶段最多重试 3 次，每次失败之间有渐增的短暂 backoff。

> 注意：当前没有内容哈希校验。如果服务端文件本身被替换（同一 file_id 但内容不同），跨进程续传可能在新旧字节之间拼接出损坏文件。这种情况下手动删除 `.download` 和 `.download.meta` 即可重新下载。

### `file delete`

```sh
pikpak file delete <id> [<id> ...]
pikpak file delete <id> --force
```

不加 `--force` 时移入回收站（可恢复），加 `--force` 永久删除。只接受文件或文件夹 ID，**不接受路径**。

### `file clear`

```sh
pikpak file clear
pikpak file clear --force
```

把账号根目录下所有条目移入回收站（或加 `--force` 永久删除）。名为 `My Pack` 且为文件夹的特殊容器会被"展开"——其直接子项被删除，但 `My Pack` 本身保留（这是 PikPak 的默认容器）。

> **警告**：本命令不会询问确认。配合 `--force` 时数据不可恢复。

### `trash empty`

```sh
pikpak trash empty
```

清空选定账号的回收站。

## 项目结构

```
.
├── main.go                     CLI 入口与命令注册
├── cmd/                        urfave/cli 命令定义
│   ├── account.go              `accounts`
│   ├── quota.go                `quota`
│   ├── task.go                 `task add`, `task list`, `task delete`, `task clear`
│   ├── file.go                 `file ls`, `file download`, `file delete`, `file clear`
│   ├── download.go             `file download` 命令定义与 size 解析
│   └── trash.go                `trash empty`
└── internal/
    ├── config/                 TOML 配置加载、context 注入
    ├── session/                Session 落盘缓存（原子写入、0600 权限）
    └── pikpak/                 HTTP 客户端、类型、下载器
        ├── client.go           认证（密码 + captcha + refresh）、API 方法
        ├── download.go         单连接 + 并发 Range 下载器、进度条、跨进程续传
        └── types.go            JSON DTO
```

## 当前限制与已知问题

- 不支持目录下载（只能下载文件）。
- 需要打开浏览器完成的 captcha 校验仅作为错误信息返回校验 URL，客户端无法自动解决。
- 没有下载后的内容哈希校验：本地文件大小等于远程大小即视为完成。强制重下载请用 `--force`。
- 批量提交任务时遇到单个 URL 失败会继续提交其余 URL，但只要有失败，进程整体非零退出。
- `file clear` 没有确认提示，配合 `--force` 时需谨慎。

## 测试

```sh
go test ./...
```

目前单元测试覆盖配置加载和 session 落盘格式。针对 PikPak API 的网络路径未做 mock。

## 许可证

[MIT License](LICENSE)。
