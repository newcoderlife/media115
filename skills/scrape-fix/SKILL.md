---
name: scrape-fix
description: Fix a wrong scrape result using the CLI command
version: 4.0
---

修正一个错误的刮削结果。

## 何时使用

1. 用户说"这个结果不对"
2. Agent 在 `/scrape` 过程中发现匹配不准确
3. 一个文件显示 `not_found` 但你知道它是什么

## Step 1: 确认正确的内容

对于电影/剧目，先搜索确认 TMDB ID：

```bash
media115 scrape "守护游戏"
media115 scrape "守护游戏" --source bangumi
```

从搜索结果中找到正确的 ID。

## Step 2: 执行修正

### 方法 A: 按 TMDB ID（最精确，推荐）

```bash
media115 scrape-fix "Restart.2026.mkv" --tmdb-id 1664596
```

### 方法 B: 按搜索词

```bash
media115 scrape-fix "Restart.2026.mkv" --search "守护游戏"
```

### 方法 C: AV 番号错误

```bash
media115 scrape-fix "T-3800040.mkv" --number "T28-003"
```

### 方法 D: 指定分类（当自动检测错误时）

```bash
media115 scrape-fix "SomeFile.mkv" --search "标题" --category 电影
```

### 方法 E: 指定剧集信息

```bash
media115 scrape-fix "SomeShow.S02E03.mkv" --tmdb-id 12345 --season 2 --episode 3
```

## Step 3: 运行 organize 应用修正

```bash
media115 organize $CATEGORY --execute
```

## 错误处理

如果 `scrape-fix` 报 `not found`:
1. 换一个搜索词再试
2. 尝试不同数据源：`--search "X" --category 剧目`（Bangumi 会自动尝试）
3. 如果确实找不到，告诉用户需要手动处理

如果修正后 organize 仍然显示 "0 to rename":
1. 运行 `media115 scan-tree $CATEGORY` 检查文件状态
2. 可能需要 `batch-scrape --force` 重新生成 NFO
