---
name: auth
description: Check 115 login status and complete QR login if needed
version: 2.2
---

## Step 1: Check login status

```bash
media115 auth --check
```

Expected output if logged in:
```
✓ 115 已登录
```

If logged in → tell user "115 已登录", done.

## Step 2: If NOT logged in — generate QR (non-blocking)

```bash
media115 auth --get-qr
```

This prints `QR_URL=https://...` and exits immediately. Show the URL to the user:

> 请用 115 App 扫描这个二维码登录：
> {QR_URL}
>
> 扫码完成后告诉我。

## Step 3: After user confirms scan — complete login

```bash
media115 auth --wait-qr
media115 auth --check
```

`--wait-qr` blocks until the QR is scanned, then saves cookies to `.env`.

## Other options

- `auth --renew` — Auto-renew cookies without scanning (if cookies still valid)
- `auth --force` — Force re-login even if already logged in
- `auth --app tv` — Use TV device type (default); also: qandroid, web
