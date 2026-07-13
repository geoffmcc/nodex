# Support

Use the GitHub repository for Nodex support and project discussion.

## Ask a question or report a bug

Open an issue at `https://github.com/geoffmcc/nodex/issues` for:

- installation or build problems;
- unexpected CLI output;
- configuration or credential-resolution issues;
- Proxmox provider errors;
- documentation corrections.

Include:

- the Nodex command you ran;
- your operating system;
- `nodex version` output;
- whether stdout was a terminal or redirected, if output formatting is relevant;
- sanitized configuration snippets when configuration is relevant;
- redacted error messages.

Do not include live Proxmox tokens, passwords, private keys, authorization headers, private hostnames, or public IP addresses that should remain private.

## Security issues

For suspected vulnerabilities, follow the [security policy](SECURITY.md). Do not open a public issue containing exploit details or secrets.

## Current support scope

The current implementation supports local CLI use and the built-in read-only Proxmox provider. Resource mutation workflows, daemon operation, remote agents, and third-party provider plugins are outside the current implemented scope.
