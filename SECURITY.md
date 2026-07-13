# Security Policy

## Supported versions

Nodex is in early development and has no stable release series yet. Security fixes are provided on the `main` branch until versioned releases exist.

## Reporting a vulnerability

Please report suspected vulnerabilities privately to the repository maintainer instead of opening a public issue with exploit details or secrets. This repository does not currently publish a dedicated security-reporting email address in its documentation; if no private channel is available to you, open a public GitHub issue with only a minimal, non-sensitive summary and ask for a private coordination channel.

Include the affected commit or version, a concise impact description, and safe reproduction steps using local test fixtures where possible.

Do not include live Proxmox tokens, passwords, private keys, or authorization headers in reports. If a credential may have been exposed, rotate it before sharing diagnostics.

## Scope

In scope: the Nodex CLI, local configuration and credential handling, Proxmox provider request handling, output redaction/sanitization, CI workflow configuration, and documentation that affects security decisions.

Out of scope unless explicitly authorized: testing against a live Proxmox server that you do not own, denial-of-service testing against third-party infrastructure, social engineering, and disclosure of unrelated secrets.

## Disclosure expectations

The maintainer will investigate reachability and impact, then coordinate a fix or documented mitigation when appropriate. Public disclosure should wait until a fix or mitigation is available, or until coordinated timing has been agreed.
