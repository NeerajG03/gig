# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest  | Yes       |

## Reporting a Vulnerability

If you discover a security vulnerability in gig, please report it responsibly:

1. **Do not** open a public GitHub issue for security vulnerabilities.
2. Email **REDACTED** with:
   - A description of the vulnerability
   - Steps to reproduce
   - Impact assessment
3. You should receive a response within 72 hours.
4. Once confirmed, a fix will be developed and released as a patch version.

## Scope

gig is a local-first task management tool. The primary attack surface is:

- **SQLite database**: Stored locally at `~/.gig/gig.db`. No network exposure by default.
- **Web UI** (`gig ui`): Binds to `localhost:9741` by default. **No authentication** — do not expose on untrusted networks without a reverse proxy with auth.
- **Shell hooks**: Commands in `gig.yaml` are executed as the current user. Treat `gig.yaml` as executable configuration — do not accept untrusted config files.
- **JSONL import**: `gig import` performs upserts. Malformed JSONL could corrupt task data but cannot execute code.

## Security Design

- **No CGO**: Pure Go binary with no native dependencies reduces supply chain risk.
- **No network by default**: The CLI operates entirely on local files. Only `gig ui` and `gig sync` involve network activity.
- **Parameterized SQL**: All database queries use parameterized statements (no string interpolation).
- **FK constraints**: Foreign key enforcement prevents orphaned data.
