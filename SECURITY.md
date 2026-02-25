# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |

## Reporting a Vulnerability

Please report security vulnerabilities through GitHub's **private vulnerability reporting**.

1. Go to the [Security tab](https://github.com/nvandessel/frond/security) of this repository
2. Click **"Report a vulnerability"**
3. Fill in the details

**Please do not open public issues for security vulnerabilities.**

## Response Timeline

- **Acknowledgment**: Within 48 hours
- **Initial assessment**: Within 1 week
- **Fix or mitigation**: Dependent on severity, targeting 30 days for critical issues

## Security Features

frond is a thin CLI wrapper around `git` and `gh`. Its security posture includes:

- **No credential storage** — frond does not store or handle credentials; authentication is delegated entirely to `gh` (GitHub CLI)
- **Input validation** — Branch names and other user inputs are validated before being passed to git commands
- **Local-only state** — The `frond.json` state file is local to each repository with no network transmission
- **Dependency scanning** — CI runs `govulncheck` on every build

## Scope

This policy covers the frond CLI tool. The underlying `git` and `gh` tools are governed by their own security policies.
