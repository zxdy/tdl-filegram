<div align="center">

# tdl-filegram

### 给 [tdl](https://github.com/iyear/tdl) 装上 Web 可视化管理 —— 粘贴 `t.me` 链接，视频 / 图片 / 文件自动落到本地。

基于 tdl 核心下载引擎，用 Vue 3 + Ant Design Vue 打造的 Web 可视化管理界面。告别命令行，扫码登录、粘贴链接、实时进度、在线预览，全部在浏览器里完成。前端产物通过 `go:embed` 打包进单个二进制文件，**零外部依赖部署**。

[![Stars](https://img.shields.io/github/stars/weilaifeng/tdl-filegram?style=flat-square&logo=github&color=yellow)](https://github.com/weilaifeng/tdl-filegram/stargazers)
[![Forks](https://img.shields.io/github/forks/weilaifeng/tdl-filegram?style=flat-square&logo=github&color=blue)](https://github.com/weilaifeng/tdl-filegram/network/members)
[![Issues](https://img.shields.io/github/issues/weilaifeng/tdl-filegram?style=flat-square&logo=github)](https://github.com/weilaifeng/tdl-filegram/issues)
[![License](https://img.shields.io/github/license/weilaifeng/tdl-filegram?style=flat-square&color=green)](./LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![Vue](https://img.shields.io/badge/Vue-3-4FC08D?style=flat-square&logo=vue.js&logoColor=white)](https://vuejs.org)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=flat-square&logo=docker&logoColor=white)](./Dockerfile)

[💡 项目亮点](#-项目亮点) · [📥 快速开始](#-快速开始) · [🎬 使用流程](#-使用流程) · [🌐 技术栈](#-技术栈) · [📊 API 接口](#-api-接口) · [⚠️ 风险告知](#-风险告知与免责声明) · [❓ FAQ](#-faq) · [🗺️ Roadmap](#-roadmap)

🌐 **中文** · [English](./README.en.md)

</div>

---

## 💡 项目亮点

本项目最大的亮点是：**为 [tdl](https://github.com/iyear/tdl) 这个强大的 Telegram 命令行下载工具，提供了一套开箱即用的 Web 可视化管理界面**。

tdl 本身是个出色的 CLI 工具，但命令行参数多、登录交互在终端里完成、看不到任务历史和实时进度。tdl-filegram 在不改动 tdl 核心能力的前提下，给它套上了一层 Web UI：

| 对比 | tdl（命令行） | tdl-filegram（本项目） |
| --- | --- | --- |
| 交互方式 | 命令行参数 + 配置文件 | 浏览器图形界面，鼠标点点点 |
| 登录方式 | 终端输入手机号 / 验证码 | 手机扫码登录，支持 2FA |
| 任务进度 | 终端文本输出，关掉就没了 | 实时进度条 + 任务列表，历史可查 |
| 文件预览 | 需要本地播放器 | 浏览器在线播放 / 查看 |
| 部署形态 | 单个二进制 | 单个二进制（前端 go:embed）/ Docker |

底层复用 tdl 的核心模块（`core/downloader` 多线程下载引擎、`core/tmedia` 媒体解析、`core/tclient` 客户端封装、`core/storage` 会话存储、`core/dcpool` 连接池），在其之上封装出 HTTP API 与 Web 前端，把 tdl 的下载能力以更直观、更易用的方式呈现出来 —— **命令行的能力，图形界面的体验**。

## ✨ 功能特性

- **二维码登录** — 手机扫码即可，支持两步验证（2FA），session 持久化在 BoltDB，重启免重登
- **多线程下载** — 复用 [tdl](https://github.com/iyear/tdl) 下载引擎，可配置线程数与并发任务数
- **任务管理** — 创建 / 查询 / 列表 / 进度跟踪，支持分页，实时显示下载进度
- **在线预览** — 下载完成的文件可通过浏览器直接预览（视频在线播放、图片查看）
- **代理支持** — http / https / socks5 代理，应对网络受限环境
- **单文件部署** — 前端嵌入二进制，只需一个可执行文件 + SQLite，免 CGO
- **Docker 部署** — 多阶段构建，环境变量配置，`docker compose up` 开箱即用

## 🎯 谁会想用

| 你是 | 你能用它做什么 |
| --- | --- |
| **Telegram 频道 / 群组归档者** | 粘贴消息链接，批量下载频道里的视频、图片、文件，本地留存 |
| **受限网络环境用户** | 内置 http / socks5 代理支持，在网络受限的环境下照样连 Telegram |
| **想要在线预览的人** | 下载完的视频 / 图片直接浏览器打开播放，不用再装本地播放器 |
| **NAS / 家庭服务器玩家** | Docker 单容器部署，挂载下载目录，长期跑着当 Telegram 离线下载器 |
| **不想装一堆依赖的人** | 前端 `go:embed` 进二进制，纯 Go SQLite 驱动免 CGO，一个文件搞定 |
| **觉得 tdl 命令行麻烦的人** | 想用 tdl 的下载能力但不想记命令行参数，Web 界面扫码登录、粘贴链接就能下载 |

## ⚡ 快速开始

### 方式一：拉取镜像部署（推荐）

镜像已发布到 GitHub Container Registry，按你的 CPU 架构选择对应标签，免去本地构建：

```bash
# ARM64（Apple Silicon / 树莓派等）
docker pull ghcr.io/studynoweekend/tdl-filegram:1.0-arm64

# AMD64（Intel / AMD）
docker pull ghcr.io/studynoweekend/tdl-filegram:1.0-amd64
```

启动容器（以 amd64 为例，arm64 把镜像标签换成 `1.0-arm64` 即可）：

```bash
docker run -d --name tdl-filegram \
  --restart unless-stopped \
  -p 8744:8744 \
  -p 8743:8743 \
  -v ./data:/data \
  -v ./downloads:/downloads \
  -e DB_PATH=/data/tdl-filegram.db \
  -e DOWNLOAD_DIR=/downloads \
  -e DOWNLOAD_THREADS=4 \
  -e DOWNLOAD_LIMIT=2 \
  -e TG_APP_ID=你的appid \
  -e TG_APP_HASH=你的apphash \
  -e TG_DATA_DIR=/data/.tdl \
  -e TG_NAMESPACE=default \
  -e TG_POOL_SIZE=8 \
  -e TG_RECONNECT_TIMEOUT=5m \
  -e TG_PROXY=http://192.168.1.100:7890 \
  ghcr.io/studynoweekend/tdl-filegram:1.0-amd64
```

### 方式二：本地构建镜像部署

```bash
# 构建镜像
docker build -t tdl-filegram .

# 启动容器
docker run -d --name tdl-filegram \
  --restart unless-stopped \
  -p 8744:8744 \
  -p 8743:8743 \
  -v ./data:/data \
  -v ./downloads:/downloads \
  -e DB_PATH=/data/tdl-filegram.db \
  -e DOWNLOAD_DIR=/downloads \
  -e DOWNLOAD_THREADS=4 \
  -e DOWNLOAD_LIMIT=2 \
  -e TG_APP_ID=你的appid \
  -e TG_APP_HASH=你的apphash \
  -e TG_DATA_DIR=/data/.tdl \
  -e TG_NAMESPACE=default \
  -e TG_POOL_SIZE=8 \
  -e TG_RECONNECT_TIMEOUT=5m \
  -e TG_PROXY=http://192.168.1.100:7890 \
  tdl-filegram
```

### 方式三：本地开发部署

**前置要求：** Go 1.25+、Node.js 18+

```bash
# 构建前端
cd web && npm install && npm run build && cd ..

# 拉取依赖并运行后端
go mod tidy && go run cmd/api/main.go
```

开发模式下可分别启动前端 dev server 和后端：

```bash
# 终端 1：后端（监听 :8743）
go mod tidy && go run cmd/api/main.go

# 终端 2：前端 dev server（监听 :5173，自动代理 /api → :8743）
cd web && npm run dev
# 访问 http://localhost:5173
```

**端口说明：**

| 端口 | 服务 | 说明 |
| --- | --- | --- |
| `8744` | nginx（前端） | 前端页面 + 反向代理 `/api` 到后端 |
| `8743` | Go 后端 | API 服务，也可直接访问 |

- 浏览器访问前端：`http://localhost:8744`
- 直接调用 API：`http://localhost:8743`

## 🎬 使用流程

1. 启动服务后浏览器访问 `http://localhost:8744`
2. 点击「扫码登录」，用 Telegram 手机端扫描二维码
3. 如开启了两步验证，输入密码
4. 登录成功后，粘贴 `t.me` 消息链接，点击下载
5. 任务列表实时显示下载进度，完成后可在线预览或下载文件

## 🌐 技术栈

| 层 | 技术 | 说明 |
| --- | --- | --- |
| Web 框架 | [Gin](https://github.com/gin-gonic/gin) | HTTP 路由与中间件 |
| Telegram MTProto | [gotd/td](https://github.com/gotd/td) + [tdl core](https://github.com/iyear/tdl) | 底层 Telegram 通信 + 下载引擎 |
| 数据库 | SQLite（[glebarez/sqlite](https://github.com/glebarez/sqlite)） | 纯 Go 驱动，免 CGO |
| 日志 | [zap](https://github.com/uber-go/zap) + lumberjack | 结构化日志 + 滚动归档 |
| 前端 | Vue 3 + Ant Design Vue 4 + Vite | 前端嵌入二进制（go:embed） |
| 会话存储 | BoltDB（[bbolt](https://github.com/etcd-io/bbolt)） | Telegram session 持久化 |

## 🔧 配置

配置优先级：**环境变量 > config.yaml > 代码默认值**。

<details>
<summary><b>配置文件（config/config.yaml）</b></summary>

```yaml
app:
  name: tdl-filegram
  port: "8743"
  env: debug              # debug / release

log:
  level: info             # info / debug
  dir: logs

database:
  path: data/tdl-filegram.db # SQLite 数据库文件路径

download:
  dir: downloads          # 下载内容输出目录
  threads: 4              # 单文件下载线程数
  limit: 2                # 最大并发任务数

telegram:
  app_id:             # Telegram API ID
  app_hash: ""
  data_dir: .tdl          # BoltDB session 存储目录
  namespace: default      # session 命名空间
  pool_size: 8            # DC 连接池大小
  reconnect_timeout: 5m   # 重连退避超时
  proxy: ""               # 代理地址，如 http://127.0.0.1:7890
```

</details>

<details>
<summary><b>环境变量（Docker 部署用）</b></summary>

所有配置项均可通过环境变量覆盖，对应关系如下：

| 环境变量 | 配置项 | Docker 默认值 |
| --- | --- | --- |
| `APP_PORT` | `app.port` | `8743` |
| `APP_HOST` | `app.host` | `0.0.0.0` |
| `DB_PATH` | `database.path` | `/data/tdl-filegram.db` |
| `DOWNLOAD_DIR` | `download.dir` | `/downloads` |
| `DOWNLOAD_THREADS` | `download.threads` | `4` |
| `DOWNLOAD_LIMIT` | `download.limit` | `2` |
| `TG_APP_ID` | `telegram.app_id` | `` |
| `TG_APP_HASH` | `telegram.app_hash` | `` |
| `TG_DATA_DIR` | `telegram.data_dir` | `/data/.tdl` |
| `TG_NAMESPACE` | `telegram.namespace` | `default` |
| `TG_POOL_SIZE` | `telegram.pool_size` | `8` |
| `TG_RECONNECT_TIMEOUT` | `telegram.reconnect_timeout` | `5m` |
| `TG_PROXY` | `telegram.proxy` | 空（不使用代理） |
| `BOT_TOKEN` | `bot.token` | 空（不启用 Bot） |
| `BOT_ALLOWED_USER_IDS` | `bot.allowed_user_ids` | 空（必须配置，逗号分隔） |

</details>

<details>
<summary><b>Docker 卷挂载点</b></summary>

| 容器路径 | 用途 |
| --- | --- |
| `/data` | SQLite 数据库 + Telegram session 持久化 |
| `/downloads` | 下载的媒体文件输出目录 |

</details>

<details>
<summary><b>Telegram API 凭证怎么获取？</b></summary>

`app_id` / `app_hash` 是 Telegram MTProto API 的应用凭证，获取方式：

1. **自己申请（推荐）** — 登录 [my.telegram.org](https://my.telegram.org) → "API development tools"，填写应用信息后获取专属凭证。
2. **使用公开凭证** — 你也可使用 Telegram Desktop 的公开凭证（`app_id: 2040`）作为临时方案，但这属于伪装成官方桌面客户端，高频使用有风控风险，生产环境建议替换为自己申请的凭证。

</details>

## 📊 API 接口

所有接口返回统一结构：

```json
{
  "code": 0,
  "msg": "success",
  "data": {},
  "trace_id": "xxx"
}
```

<details>
<summary><b>登录接口</b></summary>

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/login/status` | 查询登录状态（是否就绪、是否已登录、二维码 URL、2FA 状态） |
| `POST` | `/api/login/qr/start` | 启动二维码登录流程 |
| `POST` | `/api/login/qr/2fa` | 提交两步验证密码 |

**登录状态** `login_status` 取值：

| 值 | 含义 |
| --- | --- |
| `""`（空） | 空闲，未发起登录 |
| `pending` | 二维码已生成，等待扫码确认 |
| `need_2fa` | 需要两步验证密码 |
| `success` | 登录成功 |
| `error` | 登录失败 |

</details>

<details>
<summary><b>下载接口</b></summary>

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/download` | 创建下载任务 |

请求体：

```json
{
  "url": "https://t.me/channel/123"
}
```

响应：

```json
{
  "job_id": "uuid"
}
```

</details>

<details>
<summary><b>任务接口</b></summary>

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/jobs?page=1&page_size=20` | 分页查询任务列表 |
| `GET` | `/api/jobs/:id` | 查询单个任务详情（含实时下载进度） |
| `GET` | `/api/jobs/:id/file` | 下载 / 在线预览已完成任务的文件 |

**任务状态** `status` 取值：`pending` / `downloading` / `success` / `failed`

</details>

<details>
<summary><b>其他</b></summary>

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/health` | 健康检查 |

</details>

## 📁 项目结构

```
tdl-filegram/
├── cmd/api/              # 程序入口
├── bootstrap/            # 启动初始化（配置、日志、数据库）
├── config/               # 配置文件
├── enum/                 # 业务错误码与常量
├── internal/
│   ├── controller/       # HTTP 控制器
│   ├── dto/              # 请求 / 响应结构体
│   ├── logic/            # 业务逻辑编排
│   ├── middleware/       # Gin 中间件（CORS、Recovery、Trace）
│   ├── model/            # 数据模型与 GORM 操作
│   └── router/           # 路由注册
├── pkg/telegram/         # Telegram 引擎封装（登录、下载、存储）
├── utils/                # 工具函数
├── web/                  # 前端源码 + 嵌入产物
├── Dockerfile            # 多阶段构建
├── docker-compose.yml    # Docker Compose 编排
└── Makefile              # 构建/运行快捷命令
```

## 🛡️ 注意事项

- 首次启动需要配置代理（如果网络无法直连 Telegram）
- session 持久化在 `data_dir`（BoltDB），重启后无需重新登录
- 复用 Telegram Desktop 凭证属于伪装客户端，高频使用有封号风险，建议替换为自己的 API 凭证
- 下载的文件保存在 `download.dir` 配置的目录中

## ⚠️ 风险告知与免责声明

**使用本项目可能导致你的 Telegram 账号被限制或封禁。一旦使用，即视为你已知晓并自愿承担全部风险，作者不对任何账号损失、数据丢失或其他后果承担责任。**

本项目与 Telegram 的交互并非官方推荐方式，主要体现在以下三点：

1. **非官方 SDK** — Telegram 官方仅提供 TDLib（C++）和 Bot API（HTTP），没有 Go SDK。本项目使用社区第三方库 [gotd/td](https://github.com/gotd/td) 自行实现的 MTProto 协议，而非官方 TDLib。该库作者也在文档中提醒使用者阅读《How To Not Get Banned》指南。
2. **用户账号登录** — 本项目以用户账号（而非 Bot）登录，目的是下载受保护频道的媒体内容（官方 Bot API 做不到），但也意味着你以"第三方用户客户端"身份活动，更易触发风控。
3. **应用凭证伪装** — 若复用 Telegram Desktop 的公开凭证（`app_id: 2040`），等同于向服务端伪装成官方桌面客户端，高频使用封号风险显著上升，建议替换为自己申请的凭证。

以上行为均可能触发 Telegram 的风控机制。请在使用前充分了解风险。

### 如何减小封号风险？

- 使用官方客户端会话登录
- 尽可能使用默认的下载和上传选项，**不要设置过大的 `threads` 和 `size`**
- 不要同时在多台设备上使用相同的帐户登录
- 不要同时下载或上传太多文件
- 成为 Telegram 大会员 😅

## ❓ FAQ

<details>
<summary><b>登录后重启服务还要重新扫码吗？</b></summary>

不用。Telegram session 持久化在 `data_dir`（BoltDB），只要挂载了 `/data` 卷，重启后自动恢复登录态。

</details>

<details>
<summary><b>网络连不上 Telegram 怎么办？</b></summary>

配置 `TG_PROXY` 环境变量，支持 http / https / socks5 代理，例如 `http://127.0.0.1:7890` 或 `socks5://127.0.0.1:1080`。

</details>

<details>
<summary><b>下载的文件在哪？</b></summary>

在 `download.dir` 配置的目录（Docker 默认 `/downloads`），挂载到宿主机即可直接访问。

</details>

<details>
<summary><b>支持下载哪些类型的内容？</b></summary>

支持 `t.me` 消息链接里的视频、图片、文件，复用 tdl 的多线程下载引擎。

</details>

## 🗺️ Roadmap

- ✅ 二维码登录 + 2FA
- ✅ 多线程下载 + 任务管理 + 在线预览
- ✅ Docker 部署 + 代理支持
- 🔲 文件夹 / 频道批量下载
- 🔲 下载速度限制
- 🔲 更多文件格式预览

## 💌 致谢

本项目站在巨人的肩膀上，特别感谢以下开源项目及其作者：

- **[tdl](https://github.com/iyear/tdl)** — by [@iyear](https://github.com/iyear)，Telegram 下载利器。tdl-filegram 复用了 tdl 的核心模块（`core/downloader` 多线程下载引擎、`core/tmedia` 媒体解析、`core/tclient` 客户端封装、`core/storage` 会话存储、`core/dcpool` 连接池），没有 tdl 的工作就没有本项目。
- **[gotd/td](https://github.com/gotd/td)** — 完整的 Telegram MTProto API Go 实现，本项目底层的全部 Telegram 通信能力均基于此。

## ⭐ Star history

[![Stargazers over time](https://api.star-history.com/svg?repos=weilaifeng/tdl-filegram&type=Date)](https://star-history.com/#weilaifeng/tdl-filegram&Date)

## 📜 协议

[AGPL-3.0](./LICENSE) —— 本项目复用了 [tdl](https://github.com/iyear/tdl)（AGPL-3.0）的核心模块，按其许可证要求，本项目同样采用 AGPL-3.0 开源。任何基于本项目的衍生作品须以相同协议开源。

---

<div align="center">

**用过觉得有用？给个 ⭐ 是对作者最大的鼓励。**

[⬆ 回到顶部](#tdl-filegram) · [📥 快速开始](#-快速开始) · [💬 提 Issue](https://github.com/weilaifeng/tdl-filegram/issues)

</div>
