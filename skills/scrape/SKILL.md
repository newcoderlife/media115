---
name: scrape
description: Scrape metadata for media files — agent drives matching, code only provides tools
version: 7.0
---

刮削媒体文件的元数据（NFO + 海报）。**你（agent）负责判断文件是什么，代码只负责搜索和生成。**

## 核心原则

1. **你来判断，代码只是工具。** `media115 scrape` 只处理高置信度匹配（标题+年份完全吻合）。所有不确定的，由你来判断。
2. **没刮到就是没刮到。** 不要强行匹配。宁可标记为待确认，也不要用错误的元数据。
3. **文件名是最重要的线索。** 从文件名推测标题、年份、季/集、类型。文件名里有中文就用中文搜。

## Input

$ARGUMENTS — category: `电影`, `AV`, or `剧目`. Ask the user if not provided.

## Preconditions

- 115 must be logged in (`/auth`)
- SQLite cache must exist. If not, run `/sync` first.

## Step 1: 扫描待处理文件

```bash
media115 scan $CATEGORY
```

看 scan 的输出表格。关注 `Action` 列：
- `scrape` — 需要刮削
- `skip` — 已有 NFO，跳过
- `unrecognized` — 文件名无法解析，需要手动处理

## Step 2: 先跑 scrape 处理简单 case

```bash
media115 scrape $CATEGORY
```

scrape 自动处理文件名清晰、数据源精确匹配的文件。

输出里注意：
- `→ 标题 (ID)` — 自动匹配成功
- `not_found` — 没搜到
- `error` — 网络/API 错误

数据源按类型自动选择：
- `电影`/`剧目` → TMDB
- `AV` (日本) → jav321 / javfree
- `AV` (欧美, `av_west`) → ThePornDB / StashDB
- `anime` → TMDB / Bangumi

## Step 3: 审查自动匹配结果（关键步骤，不要跳过）

检查输出中每一行：

- 中文标题是否合理？"Youth.Periplous.2019" 匹配到 "Youth" 而不是 "青春环游记" 就是错的。
- 年份是否吻合？文件名里有 2019 但匹配到一部没有年份的内容说明匹配错了。
- 类型是否对？电影文件匹配到 TV show 说明错了。

**如果发现可疑匹配，立即用 `/scrape-fix` 修正。**

## Step 4: 处理 not_found 和 unrecognized

对每个失败文件，你来推理：

1. 看文件名全称，提取所有线索（标题、年份、季集号、制作组标签）
2. 猜测可能的中文名或英文名
3. 用 `scrape-fix` 指定正确的匹配：

```bash
media115 scrape-fix "猜测的文件.mkv" --search "猜测的名字"
media115 scrape-fix "猜测的文件.mkv" --search "猜测的名字" --category 剧目 # 动漫/剧目
media115 scrape-fix "SONE-001.mkv" --number "SONE-001"                    # 日本AV
```

4. 如果搜到多个结果，用年份、类型、集数等信息交叉验证
5. 如果确实搜不到匹配，告诉用户这个文件需要手动处理

**不要做的事：**
- 不要盲取第一个搜索结果
- 不要在不确定时强行匹配
- 不要用文件名中的单个英文单词去搜（太宽泛）

## Step 5: 修正错误匹配

对每个需要修正的文件，用 `/scrape-fix`：

```bash
media115 scrape-fix "Restart.2026.mkv" --tmdb-id 1664596
media115 scrape-fix "Restart.2026.mkv" --search "守护游戏"
media115 scrape-fix "T-3800040.mkv" --number "T28-003"
```

## Step 6: 报告并执行 organize

输出汇总表：

| 文件 | 匹配结果 | 置信度 | 备注 |
|------|---------|--------|------|
| xxx.mkv | 青春环游记 (tmdb:106938) | 高 | 年份+标题吻合 |
| yyy.mkv | ？ | — | 未找到匹配，需用户确认 |

确认后，执行 `/organize`。
