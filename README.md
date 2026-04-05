# media115

[![CI](https://github.com/newcoderlife/media115/actions/workflows/ci.yml/badge.svg)](https://github.com/newcoderlife/media115/actions/workflows/ci.yml)
[![Python 3.12+](https://img.shields.io/badge/python-3.12%2B-blue.svg)](https://www.python.org/downloads/)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Ruff](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/astral-sh/ruff/main/assets/badge/v2.json)](https://github.com/astral-sh/ruff)

115 网盘媒体库管理工具：LLM 驱动刮削、秒传上传、STRM 化、302 直链播放。

## 快速开始

```bash
# 1. 安装
git clone <repo-url> && cd media115
python3 -m venv .venv && source .venv/bin/activate
pip install -e ".[dev]"

# 2. 配置密钥
cp .env.example .env && chmod 600 .env
# 编辑 .env，填入 TMDB_READ_ACCESS_TOKEN（必需）和 BANGUMI_ACCESS_TOKEN（可选）

# 3. 验证安装
.venv/bin/pytest tests/ -v   # 离线测试应全部通过（TMDB/Bangumi live 测试需要网络+token）

# 4. 试用：搜索电影元数据
media115 scrape "The Matrix"

# 5. 试用：搜索动漫
media115 scrape "孤独摇滚" --source bangumi

# 6.（可选）登录 115 网盘
media115 auth               # 浏览器打开链接，手机扫码，cookie 自动保存到 .env
media115 ls /               # 列出网盘根目录
media115 scan /影音          # 扫描文件夹，输出刮削计划表
```

## 115 网盘认证

支持两种模式，优先使用 cookie 模式。

> **安全提示**：Cookie 模式使用非官方接口，cookie 明文保存在 `.env` 文件中。请确保 `.env` 权限为 600（`chmod 600 .env`），不要将其提交到版本控制。

| 模式 | 认证方式 | 是否需要审批 | 适用场景 |
|------|---------|------------|---------|
| **Cookie（推荐）** | `media115 auth` 扫码登录 | 不需要 | 立即可用 |
| OpenAPI | App ID + Secret | 需要在 open.115.com 审批 | 官方方式 |

```bash
# Cookie 模式：扫码登录
media115 auth                    # 打印 URL，浏览器打开扫码
# 登录成功后 cookie 自动保存到 .env 的 CLOUD_115_COOKIES 字段

# 检查是否已登录
media115 auth --check

# OpenAPI 模式：在 .env 中配置
CLOUD_115_APP_ID=your_app_id
CLOUD_115_APP_SECRET=your_secret
```

## 使用方式

### 方式一：AI Agent（推荐）

在项目目录下启动任意 AI 编程工具，让 agent 读 `AGENTS.md`：

```bash
# Claude Code
claude -p "读 AGENTS.md，然后刮削 /downloads/新番/"

# OpenAI Codex
codex "读 AGENTS.md，然后刮削 /downloads/新番/"

# 交互模式（Claude Code / Cursor / 任意 AI）
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

# 115 网盘操作（需要先 auth）
media115 auth                                     # 扫码登录
media115 auth --check                             # 检查登录状态
media115 ls /                                     # 列出根目录
media115 ls /影音/电影                             # 列出子目录
media115 scan /影音                               # 扫描并输出刮削计划表
media115 upload movie.mkv --remote-dir 12345      # 上传文件（cookie 模式）
media115 serve --port 9000                        # 启动 strm-proxy（仅监听 localhost）
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
skills/                    # Agent Skills 定义（可选）
src/media115/              # Python 源码（~1600 行，零额外依赖的 115 加密）
tests/                     # 单元测试 + 回归用例
```

## 测试

```bash
.venv/bin/pytest tests/ -v                          # 全部测试
.venv/bin/pytest tests/ -v -m "not live"            # 离线测试（不需要网络）
.venv/bin/pytest tests/test_scrape_regression.py -v  # 刮削回归
```

> Live 测试（TMDB/Bangumi API）需要有效的 token 和网络。离线测试覆盖分析器、NFO 生成、缓存等核心逻辑。

## 数据源

| 类别 | 主源 | 认证 | 备注 |
|------|------|------|------|
| 电影/剧集 | TMDB | API Key (免费) | 中文支持完整 |
| 动漫 | Bangumi | Bearer Token (可选) | 原生中文 |
| AV | jav321 + javfree | 无需 | HTML 刮削，双源 fallback |

> **注意**：CLI 必须在项目根目录运行（依赖 `.env` 和 `.cache/`）。

## Credits

- [py115](https://github.com/deadblue/py115) — M115 加密实现（RSA + XOR）的主要参考
- [p115client](https://github.com/ChenyangGao/p115client) — 115 API 调用模式参考（cookie 认证、文件列表、目录导出）
- [TMDB](https://www.themoviedb.org/) — 电影/剧集元数据 API
- [Bangumi](https://bgm.tv/) — 动漫元数据 API
- [Kodi Wiki](https://kodi.wiki/view/NFO_files) — NFO 文件格式规范
