---
name: auth
description: Check 115 login status and complete QR login if needed
version: 3.0
---

## Step 1: Check login status

```bash
cloud115 auth --check
```

Expected output if logged in:
```
✓ 115 已登录
```

If logged in → tell user "115 已登录", done.

## Step 2: If NOT logged in — generate QR (non-blocking)

```bash
cloud115 auth --get-qr
```

This prints `QR_URL=https://...` and exits immediately. Show the URL to the user:

> 请用 115 App 扫描这个二维码登录：
> {QR_URL}
>
> 扫码完成后告诉我。

## Step 3: After user confirms scan — complete login

```bash
cloud115 auth --wait-qr
cloud115 auth --check
```

`--wait-qr` blocks until the QR is scanned, then saves cookies to config.

## Other options

- `cloud115 auth --renew` — Auto-renew cookies without scanning (if cookies still valid)
- `cloud115 auth --force` — Force re-login even if already logged in
- `cloud115 auth --app tv` — Use TV device type (default); also: qandroid, web
