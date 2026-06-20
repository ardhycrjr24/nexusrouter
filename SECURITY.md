# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in NexusRouter, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please report via one of these methods:

1. **GitHub Private Advisory** (preferred): Use [GitHub's private vulnerability reporting](https://github.com/ardhycrjr24/NexusRouter/security/advisories/new)
2. **Email**: Send details to the maintainers via the contact information in the repository

Please include:
- A description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

## Response

We aim to acknowledge reports within 72 hours and provide a fix or mitigation plan within 7 days for confirmed vulnerabilities.

## Supported Versions

| Version | Supported |
|---|---|
| Latest `main` | Yes |
| Older commits | Best effort |

As NexusRouter is in active development and has not yet reached a stable release, we recommend always running the latest version from `main`.

## Scope

NexusRouter handles encrypted credentials and API keys. Areas of particular security interest include:

- Envelope encryption (`internal/crypto/`, `internal/vault/`)
- API key hashing and authentication (`internal/identity/`, `internal/auth/`)
- OAuth token storage and refresh (`internal/oauth/`)
- Admin API access controls (`internal/gateway/`)
- Proxy credential forwarding (`internal/proxy/`)

We appreciate your help in keeping NexusRouter secure.
