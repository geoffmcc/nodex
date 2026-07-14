# Residual Risk

- Live Proxmox semantics (HA locks, migration/backup races, task disappearance, permission edge cases) are documented for future authorized validation; no live target was used.
- Operator-confirmed destructive actions can still cause data loss by design; Nodex provides gates, deterministic target display, and typed confirmations but cannot prevent deliberate confirmation.
- A compromised local machine can read environment variables, keyrings, or credential files; Nodex minimizes output leakage but cannot protect against full local compromise.
- Gitleaks `dir --no-git` was not supported by the installed version; `gitleaks dir --redact --exit-code 0 .` and manual Python secret-pattern scan were used. One fake CSRF fixture was remediated to avoid scanner noise.
- Windows ACL equivalence for secret file permissions remains platform-specific and should be verified natively when release validation requires it.
