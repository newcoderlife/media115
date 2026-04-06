---
name: scrape-fix
description: Correct a scraping result and record the decision for future agents to learn from
version: 3.0
---

修正一个错误的刮削结果，并记录决策过程。

## 何时使用

1. 用户说"这个结果不对" — 用户纠正
2. Agent 在 `/scrape` 过程中发现匹配不准确 — 主动纠正
3. Agent 解决了一个 `not_found` 文件 — 记录推理过程

## Input
$ARGUMENTS — 纠正指令，如 "xxx.mkv 是电影《满江红》" 或 "Youth.Periplous 是青春环游记"

## Steps

### 1. 理解纠正内容
提取：文件名、正确类型（movie/tv/av）、正确标题、数据源、source_id。

### 2. 搜索确认
```bash
media115 scrape "正确的标题"
media115 scrape "正确的标题" --source bangumi
```

从结果中确认 ID。

### 3. 记录决策

Case 记录有两个目的：
- **给 LLM agent 看**：未来遇到类似文件名时参考推理模式
- **给回归测试用**：确保代码的 analyzer 解析结果不退化

```bash
media115 -c "
from media115.scraper.analyzer import add_case, AnalysisResult
from pathlib import Path
r = AnalysisResult(
    filename='FILENAME', media_type='TYPE', title='TITLE',
    year=YEAR, source='SOURCE', source_id='SOURCE_ID',
)
add_case(Path('tests/scrape_cases.json'), r)
"
```

### 4. 验证回归测试
```bash
media115 -m pytest tests/test_scrape_regression.py -v
```

### 5. 报告

说明：
- 原始文件名
- 错误匹配（如有）
- 正确匹配 + 推理过程
- Case ID

**推理过程很重要** — 记录你为什么选择这个匹配。例如：
> "Youth.Periplous.2019 → 搜 Youth Periplous 无结果 → Periplous 可能是 Perilous 的变体或音译 → 搜中文'青春环游记' → 命中 TMDB 106938，年份 2019 吻合"

这个推理链条帮助未来的 agent 学习如何处理类似的非标准文件名。
