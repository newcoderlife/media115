# media115

[![CI](https://github.com/newcoderlife/media115/actions/workflows/ci.yml/badge.svg)](https://github.com/newcoderlife/media115/actions/workflows/ci.yml)
[![Python 3.9+](https://img.shields.io/badge/python-3.9%2B-blue.svg)](https://www.python.org/downloads/)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![codecov](https://codecov.io/gh/newcoderlife/media115/graph/badge.svg)](https://codecov.io/gh/newcoderlife/media115)
[![Ruff](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/astral-sh/ruff/main/assets/badge/v2.json)](https://github.com/astral-sh/ruff)

115 网盘媒体库管理工具：LLM 驱动刮削、秒传上传、STRM 化、302 直链播放。

## 快速开始

```bash
# 1. 安装（推荐）
uv tool install media115

# 2. 初始化：生成默认配置、复制 skills 到 ~/.claude/skills/
media115 init

# 3. 编辑配置，填入 TMDB_READ_ACCESS_TOKEN（必需）
#    配置文件位于 ~/.config/media115/.env
chmod 600 ~/.config/media115/.env

# 4. 登录 115 网盘
media115 auth               # 浏览器打开链接，手机扫码，cookie 自动保存

# 5. 试用：列出网盘根目录
media115 ls /

# 6. 试用：搜索电影元数据
media115 scrape "The Matrix"

# 7. 试用：搜索动漫
media115 scrape "孤独摇滚" --source bangumi
```

**开发者安装：**

```bash
git clone <repo-url> && cd media115
python3 -m venv .venv && source .venv/bin/activate
pip install -e ".[dev]"
```

## 115 网盘认证

仅支持 Cookie 模式（扫码登录），无需申请官方 API。

> **安全提示**：Cookie 明文保存在 `.env` 文件中。请确保权限为 600（`chmod 600 ~/.config/media115/.env`），不要将其提交到版本控制。

```bash
# 扫码登录
media115 auth               # 打印 URL，浏览器打开扫码

# 登录成功后 cookie 自动保存到 .env 的 CLOUD_115_COOKIES 字段

# 检查是否已登录
media115 auth --check

# 自动续期
media115 auth --renew
```

## 配置文件

`media115 init` 会生成以下文件：

| 文件 | 用途 |
|------|------|
| `~/.config/media115/.env` | 密钥（TMDB token、115 cookie） |
| `~/.config/media115/config.yaml` | 分类配置、速率限制等 |

cwd 下的 `.env` / `config.yaml` 优先于 XDG 路径，适合项目本地覆盖。

## 使用方式

### 方式一：AI Agent（推荐）

在任意目录启动 AI 编程工具，让 agent 读 `AGENTS.md`：

```bash
# Claude Code
claude -p "读 AGENTS.md，然后刮削 /downloads/新番/"

# 交互模式
# 启动后告诉 agent："读 AGENTS.md，按里面的流程刮削 /path/to/folder"
```

刮削流程：

```
你: "刮削 /downloads/新番/"
  ↓
Agent: 检查 115 登录状态，未登录则引导扫码
  ↓
Agent: 扫描文件，分析类型，输出计划表
  ↓
你: 确认或纠正（"第 3 行是电影《满江红》"）
  ↓
Agent: 执行刮削，生成 NFO + 海报，输出结果表
  ↓
你: "第 1 行不对"
  ↓
Agent: 修正 → 写入回归测试 case → 跑 pytest 验证
```

每次纠正自动变成回归测试用例，保证历史纠正不被破坏。

### 方式二：CLI

```bash
# 搜索元数据
media115 scrape "The Matrix"                     # TMDB（默认）
media115 scrape "孤独摇滚" --source bangumi       # Bangumi
media115 scrape "ABC-123" --source javbus          # AV（JavBus）

# 115 网盘文件系统
media115 auth                                     # 扫码登录
media115 auth --check                             # 检查登录状态
media115 ls /影音                                 # 列目录
media115 ls -l /影音/电影                          # 详细格式
media115 stat /影音/电影/满江红.mkv                # 文件元信息
media115 find "满江红" /影音                       # 搜索
media115 mkdir -p /影音/电影/新目录                # 创建目录
media115 mv /影音/a.mkv /影音/电影/               # 移动
media115 rename /影音/old.mkv new.mkv             # 重命名
media115 rm /影音/垃圾.txt                        # 删除
media115 put ./local.nfo /影音/电影/              # 上传（支持秒传）
media115 rapid ./large.mkv /影音/电影/            # 秒传（按 SHA1 匹配）
media115 get /影音/电影/a.mkv ./                  # 下载
media115 sync /影音                               # 刷新目录树缓存
media115 cache status                             # 缓存状态
media115 cache clear                              # 清除缓存

# 批量操作（需要先 sync）
media115 scan-tree 电影                           # 分析缓存，输出刮削计划
media115 batch-scrape 电影                        # 批量刮削
media115 organize 电影                            # 重命名 + 移动 + 上传 NFO（dry-run）
media115 organize 电影 --execute                  # 执行

# 代理（可选依赖）
media115 serve --port 9000                        # 启动 strm-proxy
```

> **注意**：strm-proxy 默认只监听 127.0.0.1，没有认证层。如需局域网访问，请使用 `--host 0.0.0.0` 并自行做好网络隔离。

### 方式三：Python API

```python
from media115.scraper.tmdb import TMDBClient
from media115.scraper.nfo import generate_movie_nfo
from media115.scraper.artwork import save_poster
from pathlib import Path

client = TMDBClient(read_access_token="your_token")
detail = client.movie_detail(603)
images = client.movie_images(603)

generate_movie_nfo({
    "title": detail["title"],
    "year": int(detail["release_date"][:4]),
    "uniqueids": {"tmdb": str(detail["id"])},
}, Path("./movie.nfo"))

save_poster(images["posters"][0]["file_path"], Path("."))
```

## 项目结构

```
AGENTS.md                  # Agent 指令（任何 AI 工具读这个就能跑）
src/media115/
  cli.py                   # CLI 入口
  fs.py                    # 115 文件系统抽象（路径解析、目录树）
  fs_cli.py                # 文件系统 CLI 命令（ls/mv/rename/rm 等）
  client.py                # 115 API 客户端（cookie 认证）
  _ec115.py                # EC115 加密（秒传 SHA1/RSA 签名）
  cache.py                 # 缓存管理（XDG 路径）
  organizer.py             # organize 逻辑
  proxy.py                 # STRM 302 代理
  scraper/                 # TMDB / Bangumi / JavBus / jav321 刮削器
  _skills/                 # Agent Skills 定义
tests/                     # 单元测试 + 回归用例
```

## 测试

```bash
pytest tests/ -v                          # 全部测试
pytest tests/ -v -m "not live"            # 离线测试（不需要网络）
pytest tests/test_scrape_regression.py -v  # 刮削回归
```

> Live 测试（TMDB/Bangumi API）需要有效的 token 和网络。离线测试覆盖分析器、NFO 生成、缓存等核心逻辑。

## 数据源

| 类别 | 主源 | 认证 | 备注 |
|------|------|------|------|
| 电影/剧集 | TMDB | API Key (免费) | 中文支持完整 |
| 动漫 | Bangumi | Bearer Token (可选) | 原生中文 |
| AV | jav321 + javfree | 无需 | HTML 刮削，双源 fallback |

## 依赖

核心依赖：`httpx`、`lxml`、`click`、`pyyaml`、`pycryptodome`（EC115 加密）、`lz4`（缓存压缩）

可选依赖：`fastapi`、`uvicorn`（strm-proxy，`pip install media115[proxy]`）

## Credits

- [py115](https://github.com/deadblue/py115) — M115 加密实现（RSA + XOR）的主要参考
- [p115client](https://github.com/ChenyangGao/p115client) — 115 API 调用模式参考（cookie 认证、文件列表、目录导出）
- [TMDB](https://www.themoviedb.org/) — 电影/剧集元数据 API
- [Bangumi](https://bgm.tv/) — 动漫元数据 API
- [Kodi Wiki](https://kodi.wiki/view/NFO_files) — NFO 文件格式规范
- [pycryptodome](https://github.com/Legrandin/pycryptodome) — EC115 秒传加密
- [lz4](https://github.com/python-lz4/python-lz4) — 缓存压缩
