---
name: media115-subscribe
description: M-Team torrent grabber — grab .torrent files matching a rule into a local watch folder
metadata:
  version: "1.0"
---

搜索 M-Team 站内种子，匹配规则就把 `.torrent` 原子写入本地 watch 文件夹，交给外部 BT 客户端（qBittorrent / Transmission）自动加载。**不涉及 115 网盘。**

## 核心原则

1. **规则由 agent 帮用户搭。** 用户只会给模糊诉求（“自动下载 4K 免费电影，不要原盘”）。你负责翻译成完整的 `media115 grab` 参数。
2. **一律从 `--dry-run` 开始。** 在用户确认规则匹配到预期的种子前，不要真的下载。
3. **外部调度，不要自己写守护进程。** `media115 grab` 是一次性的，让用户自己挂到 cron/launchd/systemd timer。

## Input

$ARGUMENTS — 用户的订阅意图，例如 "4K 免费电影，≤50 个文件"。如果用户没说清楚，追问以下信息：
- 关键字（或空=拉取全站最新）
- 类型：movie / tvshow / normal（默认 normal 覆盖面最广）
- 必须免费？(推荐 yes)
- 必须 4K？
- watch 文件夹路径

## Preconditions

- `[mteam].api_key` 已配置（见 `~/.config/media115/config.toml`）。没有就提示用户去 M-Team 个人中心生成。
- watch 文件夹必须在外部 BT 客户端的监控目录里。

## Step 1: 体检

```bash
media115 doctor
```

确认 `MTeam APIKey` 有值。

## Step 2: Dry-run 验证

根据用户诉求组装 `grab` 参数并先 dry-run：

```bash
media115 grab \
  --mode movie \
  --keyword "" \
  --watch-dir "/path/to/watch" \
  --require-free \
  --require-4k \
  --max-files 50 \
  --min-size 4G --max-size 80G \
  --min-seeders 3 \
  --labels-deny "原盘,BDMV" \
  --fresh-hours 48 \
  --dry-run
```

关键过滤器翻译：
- "不要肉酱盘 / 不要原盘" → `--no-junk`
- "只看最近的新种" → `--fresh-hours 48`
- "豆瓣 8 分以上" → `--min-rating 8.0`
- "只要日剧合集" → `--mode tvshow --tv-complete`
- "过滤掉抢先版" → `--exclude "CAM,TC,HDTS,HC"`

## Step 3: 确认后执行

确认 dry-run 输出符合预期后，去掉 `--dry-run` 执行真实下载：

```bash
media115 grab \
  --mode movie \
  --watch-dir "/path/to/watch" \
  --require-free --require-4k \
  --no-junk --fresh-hours 48
```

## Step 4: 挂定时任务

```bash
crontab -e
# 添加：
*/30 * * * * /usr/local/bin/media115 grab >> ~/.cache/media115/logs/grab.log 2>&1
```

## Step 5: 观察

检查 cron 日志输出（`~/.cache/media115/logs/grab.log`）确认 grab 正常运行。

如果用户反馈"抓了奇怪的东西"，先 `--dry-run` 复现问题，再调整过滤参数（加 `--exclude`、改 `--labels-deny` 等）。

## CLI Reference

| 命令 | 作用 |
|------|------|
| `media115 grab [--dry-run] [--max-pages N] [flags]` | 搜索 + 过滤 + 下载一次 |

## 过滤字段一览

| Flag | 意义 |
|------|------|
| `--require-free` | 仅 FREE / 2XFREE |
| `--require-4k` | labelsNew 含 4K/2160p/UHD，或种子名含 2160p/4K/UHD |
| `--require-hq` | 4K + HDR/DOVI |
| `--no-junk` | 排除原盘 + 肉酱盘（按 mode 自动调阈值：movie=20, tvshow=500, adult=跳过） |
| `--max-files N` | 种子文件数 ≤ N |
| `--min-size / --max-size` | 体积范围，例 `4G` / `80G` |
| `--min-seeders N` | 做种数下限 |
| `--exclude a,b` | 种子名含任一词则跳过（大小写不敏感） |
| `--labels-allow a,b` | labelsNew 至少含一个 |
| `--labels-deny a,b` | labelsNew 任一命中则跳过 |
| `--fresh-hours N` | 仅 N 小时内发布 |
| `--min-rating X` | 评分下限 (IMDB/豆瓣取高者) |
| `--tv-complete` | 仅剧集合集（EXX-EYY / 全X集 / Complete / 完结），且 `--mode tvshow` 时才生效 |

## Rules

**DO:**
- 先 `--dry-run` 验证规则，确认后再去掉。
- 读 `grab` 输出的 Rejections 计数，针对性调整规则。
- 把 watch 文件夹指向 BT 客户端的监控目录（qBittorrent: Settings → Downloads → Monitored Folder）。

**DO NOT:**
- 绕过 CLI 直接调 M-Team API。
- 在没有 min-interval 的前提下高频 grab（默认 30s/请求，保持不变）。
- 用 grab 下载 115 里已有的内容 — 这是站内抓取器，不是同步器。
