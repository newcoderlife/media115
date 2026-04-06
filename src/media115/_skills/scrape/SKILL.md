---
name: scrape
description: Scrape metadata for media files — agent drives matching, code only provides tools
version: 5.0
---

刮削媒体文件的元数据（NFO + 海报）。**你（agent）负责判断文件是什么，代码只负责搜索和生成。**

## 核心原则

1. **你来判断，代码只是工具。** `batch-scrape` 只处理高置信度匹配（标题+年份完全吻合）。所有不确定的，由你来判断。
2. **没刮到就是没刮到。** 不要强行匹配。宁可标记为待确认，也不要用错误的元数据。
3. **学习历史决策。** 先看 `tests/scrape_cases.json` 里的历史 case，理解之前的匹配模式。
4. **文件名是最重要的线索。** 从文件名推测标题、年份、季/集、类型。文件名里有中文就用中文搜。

## Input
$ARGUMENTS — category: `电影`, `AV`, or `剧目`. Ask the user if not provided.

## Preconditions
- 115 must be logged in (`/auth`)
- Tree cache must exist. If not, run `/sync` first.

## Steps

### 1. 扫描待处理文件
```bash
media115 scan-tree $CATEGORY
```

看 scan-tree 的输出表格。关注 "Action" 列：
- `scrape` — 需要刮削
- `skip` — 已有 NFO，跳过
- `known case` — 已有历史 case
- `unrecognized` — 文件名无法解析

### 2. 先跑 batch-scrape 处理简单 case
```bash
media115 batch-scrape $CATEGORY
```

batch-scrape 会自动处理文件名清晰、TMDB 精确匹配的文件。输出里注意：
- `→ 标题 (ID)` — 自动匹配成功
- `not_found` — 没搜到
- `error` — 网络/API 错误

### 3. 审查自动匹配结果（关键步骤）

**不要跳过这一步。** batch-scrape 的自动匹配可能是错的。检查输出中每一行：

- 中文标题是否合理？"Youth.Periplous.2019" 匹配到 "Youth" 而不是 "青春环游记" 就是错的。
- 年份是否吻合？文件名里有 2019 但匹配到一部没有年份的剧，说明匹配错了。
- 类型是否对？电影文件匹配到 TV show 说明错了。

**如果发现可疑匹配：**
1. 用 `media115 scrape "更合适的搜索词"` 手动搜索
2. 尝试不同的搜索策略：
   - 文件名中的中文部分
   - 英文标题的不同拼写/翻译
   - 只用年份 + 关键词
3. 确认正确结果后，用 `/scrape-fix` 修正

### 4. 处理 not_found 和 unrecognized

对每个失败文件，你来推理：

**推理方法：**
1. 看文件名全称，提取所有线索（标题、年份、季集号、制作组标签）
2. 猜测可能的中文名或英文名
3. `media115 scrape "猜测的名字"` 搜索
4. 如果搜到多个结果，用年份、类型、集数等信息交叉验证
5. 如果搜不到，尝试：
   - 换数据源：`--source bangumi`（动漫）
   - 缩短/变换搜索词
   - 搜索制作公司或导演名
6. 如果确实搜不到匹配，告诉用户这个文件需要手动处理

**不要做的事：**
- 不要盲取第一个搜索结果
- 不要在不确定时强行匹配
- 不要用文件名中的单个英文单词去搜（太宽泛，会匹配到不相关的）

### 5. 记录决策

用 `/scrape-fix` 保存每个决策。这些 case 会帮助未来的 agent 学习你的判断模式。

Case 应该包含：
- 文件名
- 正确的匹配结果（标题、来源、ID）
- 推理过程（为什么选这个而不是那个）

### 6. 报告

输出汇总表：

| 文件 | 匹配结果 | 置信度 | 备注 |
|------|---------|--------|------|
| xxx.mkv | 青春环游记 (tmdb:106938) | 高 | 年份+标题吻合 |
| yyy.mkv | ？ | — | 未找到匹配 |

问用户是否确认，然后执行 `/organize`。
