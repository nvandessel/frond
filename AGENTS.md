# Agent Instructions

This project uses **bd** (beads) for issue tracking. Run `bd onboard` to get started.

## Essential Reading

1. `docs/GO_GUIDELINES.md` — Go coding standards (read before writing code)

## Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --status in_progress  # Claim work
bd close <id>         # Complete work
bd sync               # Sync with git
```

## Development

```bash
go build ./...        # Build
go test ./...         # Test
go test -race -coverprofile=coverage.out ./...  # Test with race + coverage
go vet ./...          # Vet
gofmt -l .            # Check formatting
golangci-lint run     # Lint
```

## Project Structure

```
tier/
├── main.go                 # Entry point
├── cmd/                    # Cobra commands (new, push, sync, status, track, untrack)
│   └── root.go             # Root command, --json global flag
└── internal/
    ├── state/              # tier.json types, read/write, lockfile
    ├── git/                # Thin git CLI wrapper
    ├── gh/                 # Thin gh CLI wrapper
    └── dag/                # Cycle detection, topo sort, readiness, tree rendering
```

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd sync
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
