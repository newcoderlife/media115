# 115-media

115 网盘媒体库管理工具：LLM 驱动刮削、秒传上传、STRM 化、302 直链播放。

## 安装

```bash
git clone <repo-url> && cd 115-media
python3 -m venv .venv && source .venv/bin/activate
pip install -e .
cp .env.example .env && chmod 600 .env
# 编辑 .env，填入 TMDB / Bangumi / 115 的密钥
```

## 使用

### AI Agent（推荐）

本项目设计为 AI agent 驱动。让 agent 读 `AGENTS.md` 即可开始工作。

```
读一下 AGENTS.md，然后刮削 /downloads/新番/ 这个文件夹
```

不同工具的启动方式：

```bash
# Claude Code
claude -p "读 AGENTS.md，然后执行 Scrape workflow，目标目录 /downloads/新番/"

# OpenAI Codex
codex "读 AGENTS.md，然后执行 Scrape workflow，目标目录 /downloads/新番/"

# 交互模式（Claude Code / Cursor / 任何 AI 工具）
# 进入项目目录，启动 agent，告诉它：
#   "读 AGENTS.md，按里面的流程刮削 /downloads/新番/"
```

#### 刮削流程

```
你: "刮削 /downloads/新番/"
  ↓
Agent: 扫描文件，分析类型，输出计划表
  ↓
你: 确认或纠正（"第 3 行是电影《满江红》"）
  ↓
Agent: 执行刮削，生成 NFO + 海报，输出结果表
  ↓
你: "第 1 行不对"
  ↓
Agent: 修正 → 写入回归 case → 跑 pytest → 报告
```

每次纠正自动变成回归测试，保证以后改代码不会破坏已有纠正。

### CLI（不依赖 AI）

```bash
115-media scrape "The Matrix"                     # 搜索 TMDB
115-media scrape "孤独摇滚" --source bangumi       # 搜索 Bangumi
115-media scrape "ABC-123" --source javbus         # 搜索 JavBus
115-media ls 0                                     # 列出 115 网盘根目录
115-media upload movie.mkv --remote-dir 12345      # 秒传上传
115-media serve --port 9000                        # 启动 strm-proxy
```

### Python API

```python
from media115.scraper.tmdb import TMDBClient
from media115.scraper.nfo import generate_movie_nfo
from pathlib import Path

client = TMDBClient(read_access_token="your_token")
detail = client.movie_detail(603)
generate_movie_nfo({
    "title": detail["title"],
    "year": int(detail["release_date"][:4]),
    "uniqueids": {"tmdb": str(detail["id"])},
}, Path("./movie.nfo"))
```

## 项目结构

```
AGENTS.md              # Agent 指令文件（所有 AI 工具读这个）
skills/                # Agent Skills（可选，供支持 skill 的工具加载）
src/media115/          # Python 源码
tests/                 # 93 个测试 + 回归用例
```

## 测试

```bash
.venv/bin/pytest tests/ -v                          # 全部（93 tests）
.venv/bin/pytest tests/test_scrape_regression.py -v  # 刮削回归
```

## 数据源

| 类别 | 主源 | 认证 |
|------|------|------|
| 电影/剧集 | TMDB | API Key (免费) |
| 动漫 | Bangumi | Bearer Token (可选) |
| AV | JavBus | 无需 |
