# Security Policy

## Reporting a Vulnerability

If you find a security issue, please email newcoderlife@gmail.com instead of opening a public issue.

## Security Considerations

- **Cookie storage**: 115 cookies are stored in plaintext in `.env`. Keep this file private (`chmod 600 .env`) and never commit it.
- **strm-proxy**: The proxy has no authentication. It defaults to `127.0.0.1` (localhost only). If you bind to `0.0.0.0`, ensure proper network isolation.
- **Cookie mode**: This project uses 115's unofficial cookie-based API. Use at your own risk.
