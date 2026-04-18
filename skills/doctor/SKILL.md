---
name: doctor
description: Check environment, credentials, and cache status
version: 2.0
---

Run this first when something doesn't work. Also run at the start of any session to confirm everything is configured.

## Step 1: Run doctor

```bash
cloud115 doctor
```

Expected output (all green):
```
环境检查:
  cloud115:      0.x.x
  media115:      0.x.x

配置:
  config.toml:   /Users/you/.config/media115/config.toml ✓

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
```

## Step 2: Fix issues

| Issue | Fix |
|-------|-----|
| `config.toml ✗ 不存在` | Create `~/.config/media115/config.toml` with credentials |
| `115 Cookies: ✗` | Run `/auth` |
| `TMDB Token: ✗` | Add `token` under `[auth.tmdb]` in config.toml |
| `Bangumi Token: ✗` | Add `token` under `[auth.bangumi]` in config.toml (optional, for anime) |
| `tree_entry: 0 条目` | Run `cloud115 sync /影音` |

## Step 3: Check cache status separately (optional)

```bash
cloud115 cache status
```

Shows more detail: tree snapshot root path, entry count, rate limit state.
