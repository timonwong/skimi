# skimi — Claude Code Guide

## Build & Verify

```bash
go build ./cmd/skimi          # compile
go vet ./...                  # static analysis
golangci-lint run             # what CI's Lint step runs; vet alone misses it
go test -race -count=1 ./...  # run all tests
goreleaser build --snapshot   # verify release build
```

## Architecture

```
cmd/skimi/main.go              Entry point
internal/types/types.go        All shared data types (leaf, no internal deps)
internal/source/source.go      Source-string parser: shorthand/URL/local path → ParsedSource (leaf)
internal/ui/styles.go          Shared lipgloss styles (leaf)
internal/fileutil/fileutil.go  Shared AtomicWrite helper
internal/config/config.go      skills.yaml read/write + DefaultPaths
internal/lock/lock.go          skills-lock.yaml atomic read/write + FindByName
internal/git/git.go            git CLI wrapper (Clone/Pull/Fetch/ResetHardUpstream/IsRepoRoot/HeadCommit/RevParse/Log)
internal/detect/detect.go      SKILL.md scanner + frontmatter parser
internal/linker/linker.go      symlink management, agent directory mapping, IsManagedLink
internal/installer/installer.go Install/update orchestration + the applyPlan transaction
internal/cli/                  Cobra commands: root install list view edit check-updates update remove
```

**Dependency order** (bottom → top):
`types`, `source`, `ui` → `fileutil`, `config`, `lock`, `git`, `detect`, `linker` → `installer` → `cli` → `cmd/skimi`

## Default Paths

| Purpose | Path |
|---|---|
| Config | `~/.config/skimi/skills.yaml` |
| Lock | `~/.config/skimi/skills-lock.yaml` |
| Store | `~/.local/share/skimi/skills/` |

## Agent Skill Directories

| Agent constant | Directory |
|---|---|
| `AgentClaude` | `~/.claude/skills/` |
| `AgentCodex` | `~/.codex/skills/` |
| `AgentPi` | `~/.pi/agent/skills/` |
| `AgentStandard` | `~/.agents/skills/` |
| `AgentOpenClaw` | `~/.openclaw/skills/` |

## Code Conventions

- Error wrapping: `fmt.Errorf("context: %w", err)` — lowercase, verb+noun, always `%w`
- Non-fatal errors: `fmt.Fprintf(os.Stderr, "warning: ...")`; program continues
- File not found: `config.Load` and `lock.Load` return empty struct, not error
- Atomic writes: use `fileutil.AtomicWrite` (tmp file + rename, same filesystem)
- Tests: table-driven with `t.Run`, `t.TempDir()`, `go-cmp` for struct diffs; no mocks

## Key Design Decisions

- Skills install flat, as directory symlinks, into `<agentDir>/skills/<skill>` for every agent
- Config-driven `install` is a full sync (config is the desired state); interactive `install <source>` is additive via `Options.Additive`, preserving unrelated lock entries
- Deletion requires proof of ownership: paths failing `linker.IsManagedLink` (see its doc comment) are never deleted, only warned about
- `applyPlan` is transactional: existing paths are renamed to backups, restored on any failure, and deleted only after the lock file saves
- Duplicate names use deterministic last-wins resolution; `path` selectors disambiguate
- The store is a disposable cache: `installer.EnsureRepo` re-clones a store dir git cannot use and resets onto a force-pushed upstream, but only after a fetch proves the remote is reachable, and only for paths inside `StoreDir`
- SKILL.md scan: stops descending once SKILL.md is found in a directory (same as skm behaviour)
- `target_dir` is deprecated, ignored, and retained only for v1 config compatibility
