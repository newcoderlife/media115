---
name: auth
description: Check 115 login status and initiate QR login if needed
version: 1.0
---

Check if the user is logged into 115 cloud. If not, guide them through QR login.

## Steps

1. Check login status:

```bash
.venv/bin/python -m media115.cli auth --check
```

2. If already logged in, tell the user and proceed with their request.

3. If not logged in, run the auth command:

```bash
.venv/bin/python -m media115.cli auth
```

This will print a URL. Tell the user:
- Open the URL in a browser
- Scan the QR code with the 115 mobile app
- Wait for confirmation

4. After login succeeds, cookies are saved to `.env`. Verify by running `--check` again.

## When to use

Call this skill automatically before any command that requires 115 access (`ls`, `scan`, `upload`, `serve`). If the command fails with a credentials error, invoke this skill.
