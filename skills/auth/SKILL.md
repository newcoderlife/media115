---
name: auth
description: Check 115 login status and complete QR login if needed
version: 2.0
---

## Step 1: Check login status

```bash
.venv/bin/python -m media115.cli auth --check
```

## Step 2: If already logged in

Tell the user "115 已登录" and proceed with their request.

## Step 3: If NOT logged in — generate QR (non-blocking)

```bash
.venv/bin/python -m media115.cli auth --get-qr
```

This prints `QR_URL=https://...` and exits immediately. Show the URL to the user:

> 请用 115 App 扫描这个二维码登录：
> {QR_URL}
>
> 扫码完成后告诉我。

## Step 4: After user confirms scan — complete login

```bash
.venv/bin/python -m media115.cli auth --wait-qr
```

This polls for scan result and saves cookies to .env. Then verify:

```bash
.venv/bin/python -m media115.cli auth --check
```
