---
name: auth
description: Check 115 login status and guide QR login if needed
version: 1.0
---

## Step 1: Check login status

```bash
.venv/bin/python -m media115.cli auth --check
```

## Step 2: If already logged in

Tell the user "115 已登录" and proceed.

## Step 3: If NOT logged in

**DO NOT run `115-media auth` via Bash.** It is interactive and will block forever.

Tell the user to run it themselves in a separate terminal:

> 115 还没登录。请在终端运行：
>
> `.venv/bin/115-media auth`
>
> 浏览器打开输出的链接，用 115 App 扫码。完成后告诉我。

## Step 4: After user confirms login

Verify:

```bash
.venv/bin/python -m media115.cli auth --check
```
