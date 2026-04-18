---
name: wishlist
description: Batch search M-Team for a movie wishlist, deduplicate against 115 library, and download best torrents
version: 1.0
---

根据用户提供的电影片单（如"豆瓣 Top 250"），与 115 影视库去重后，批量搜索 M-Team 免费高质量种子并下载。BT 客户端下载完成后逐个秒传到 115，可选后续刮削整理。

## 核心原则

1. **Agent 提供名单，Agent 逐个执行。** 用户给出片单意图（"豆瓣 Top 250"、"奥斯卡最佳影片"），你通过搜索获取具体电影列表，然后逐个调用 CLI 命令。
2. **先去重再搜索。** 使用 `cloud115 find` 检查 115 库已有电影，跳过已有的，避免浪费 M-Team API 配额。
3. **分批 dry-run，用户确认后下载。** 每批 25 部，先展示搜索结果表格，确认后才真正下载。
4. **`--pick-best` 只下最优种子。** 每部电影只下载一个最佳种子（完成数 ≥ 20 按完成数排序，< 20 按时间排序）。

## Input

$ARGUMENTS — 用户的片单意图，例如 "豆瓣 Top 250"、"2024 年豆瓣高分电影"、"IMDB Top 100"。如果用户没说清楚，追问：
- 片单来源（豆瓣/IMDB/自定义列表）
- 过滤偏好（是否要求 4K、是否要求免费、体积限制）
- watch 文件夹路径
- 115 影视库路径（默认 `/影音/电影`）

## Preconditions

- `[mteam].api_key` 已配置。没有就提示用户去 M-Team 个人中心生成。
- 115 已登录：`cloud115 auth --check`。
- watch 文件夹在 BT 客户端监控目录中。

## Step 1: 体检

```bash
media115 doctor
cloud115 auth --check
```

确认 MTeam APIKey 有值、115 已登录。

## Step 2: 同步 115 影视库缓存

```bash
cloud115 sync /影音
```

构建本地树缓存，后续 `find` 查询走缓存。

## Step 3: 获取电影列表

通过搜索获取用户指定的电影列表。整理为结构化名单：序号、电影名、年份。

示例（豆瓣 Top 250 前 5）：
```
1. 肖申克的救赎 (1994)
2. 霸王别姬 (1993)
3. 泰坦尼克号 (1997)
4. 阿甘正传 (1994)
5. 千与千寻 (2001)
```

## Step 4: 去重 — 检查 115 已有

对名单中每部电影执行：

```bash
cloud115 find "{电影名}" /影音/电影
```

- **有输出** → 115 已有，标记跳过
- **无输出** → 缺失，加入待搜索列表

汇总去重结果：
```
名单总数: 250
115 已有: 82
待搜索: 168
```

## Step 5: 分批 dry-run 搜索

将待搜索列表分批（每批 25 部），对当前批次逐个执行：

```bash
media115 grab --keyword "{电影名}" --mode movie \
  --require-free --no-junk --pick-best \
  --watch-dir /path/to/watch --dry-run
```

解析输出：
- 含 `[dry-run]` 行 → 提取种子名、体积、做种数、折扣状态
- `匹配=0` → M-Team 无符合条件的资源

## Step 6: 展示批次结果

汇总为表格展示给用户：

| # | 电影 | 115 | M-Team | 种子名 | 体积 | 完成数 |
|---|------|-----|--------|--------|------|--------|
| 1 | 肖申克的救赎 | ✅ 已有 | — | — | — | — |
| 2 | 霸王别姬 | ❌ | ✅ | Farewell.My.Concubine.2160p | 65G | 128 |
| 3 | 控方证人 | ❌ | ❌ 无免费 | — | — | — |

## Step 7: 用户确认后下载

用户确认后，对本批已找到的种子去掉 `--dry-run` 执行：

```bash
media115 grab --keyword "{电影名}" --mode movie \
  --require-free --no-junk --pick-best \
  --watch-dir /path/to/watch
```

确认 `下载=1` 后继续下一部。全部完成后处理下一批（回到 Step 5）。

## Step 8: 等待 BT 下载完成

提示用户等待 BT 客户端完成下载。用户确认后进入上传步骤。

## Step 9: 秒传上传到 115

遍历 BT 下载目录中已完成的电影，逐个上传：

```bash
cloud115 put /path/to/downloads/{电影目录}/{文件名}.mkv /影音/电影/{电影目录}/
```

`put` 会自动先尝试秒传，失败再 fallback 到普通上传。

## Step 10: 整理（可选）

```bash
cloud115 sync /影音
media115 scrape 电影
media115 organize 电影 --execute
```

参考 `/scrape` 和 `/organize` skill 执行刮削和整理。

## CLI Reference

| 命令 | 作用 |
|------|------|
| `cloud115 sync /影音` | 同步目录树到本地缓存 |
| `cloud115 find "{名}" /影音/电影` | 搜索 115 已有文件 |
| `media115 grab --pick-best [flags]` | 搜索 M-Team，只下载最优种子 |
| `cloud115 put <local> <remote>` | 秒传上传到 115 |

## Rules

**DO:**
- 先 `cloud115 sync` 再 `cloud115 find` 去重，避免浪费 M-Team API 调用。
- 每批 25 部，展示表格让用户确认后再下载。
- 用 `--pick-best` 确保每部电影只下一个最优种子。
- 读 `grab` 输出的 Rejections 计数，针对性调整过滤参数。
- 汇总最终报告：成功/已有/未找到/出错。

**DO NOT:**
- 不要跳过 dry-run 直接下载。
- 不要跳过去重步骤——浪费 API 配额且可能重复下载。
- 不要一次处理全部 250 部——分批确认，便于用户审核和中断。
