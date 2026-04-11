---
name: doctor
description: Check environment, credentials, and cache status
version: 1.0
---

Run this first when something doesn't work. Also run at the start of any session to confirm everything is configured.

## Step 1: Run doctor

```bash
media115 doctor
```

Expected output (all green):
```
环境检查:
  Python:        3.x.x
  media115:      0.x.x

配置:
  .env:          /Users/you/.config/media115/.env ✓
  config.yaml:   /Users/you/.config/media115/config.yaml ✓

认证:
  115 Cookies:   ✓ 已配置
  TMDB Token:    ✓ 已配置
  Bangumi Token: ✓ 已配置

缓存:
  缓存根目录:    /Users/you/.cache/media115
  path_index:    104 条目
  dir listings:  42 目录, 359 条目
  db 大小:       804.0K
  tree_entry:    ✓ 2158 条目 (1452 视频, 706 NFO)
  scrape_output: 4 分类

Skills:
  /Users/you/.claude/skills/media115: ✓ 已安装
```

## Step 2: Fix issues

| Issue | Fix |
|-------|-----|
| `.env ✗ 不存在` | Create `~/.config/media115/.env` with credentials |
| `115 Cookies: ✗` | Run `/auth` |
| `TMDB Token: ✗` | Add `TMDB_READ_ACCESS_TOKEN=...` to `.env` |
| `Bangumi Token: ✗` | Add `BANGUMI_ACCESS_TOKEN=...` to `.env` (optional, for anime) |
| `tree_entry: 0 条目` | Run `media115 sync /影音` |
| `Skills: ✗ 未安装` | Run `media115 init` |

## Step 3: Check cache status separately (optional)

```bash
media115 cache status
```

Shows more detail: tree snapshot root path, entry count, rate limit state.
