# skimi — Claude Code Guide

## Build & Verify

```bash
go build ./cmd/skimi          # compile
go vet ./...                  # static analysis
go test -race -count=1 ./...  # run all tests
goreleaser build --snapshot   # verify release build
```

## Architecture

```
cmd/skimi/main.go              Entry point (13 lines, calls cli.Execute)
internal/types/types.go        All shared data types (leaf node, no internal deps)
internal/fileutil/fileutil.go  Shared AtomicWrite helper
internal/config/config.go      skills.yaml read/write + DefaultPaths
internal/lock/lock.go          skills-lock.yaml atomic read/write + FindByName
internal/git/git.go            git CLI wrapper (Clone/Pull/Fetch/HeadCommit/RevParse/Log)
internal/detect/detect.go      SKILL.md scanner + frontmatter parser
internal/linker/linker.go      symlink/hardlink management, agent directory mapping
internal/installer/installer.go Core install orchestration: Run, RepoStorePath, ExpandPath
internal/cli/                  Cobra commands: root install list view check-updates update remove
```

**Dependency order** (bottom → top):
`types` → `fileutil`, `config`, `lock`, `git`, `detect`, `linker` → `installer` → `cli` → `cmd/skimi`

No circular dependencies. `types` is the pure leaf; all packages flow up to `cli`.

## Default Paths

| Purpose | Path |
|---|---|
| Config | `~/.config/skimi/skills.yaml` |
| Lock | `~/.config/skimi/skills-lock.yaml` |
| Store | `~/.local/share/skimi/skills/` |

## Agent Skill Directories

| Agent constant | Directory | Link type |
|---|---|---|
| `AgentClaude` | `~/.claude/skills/` | symlink |
| `AgentCodex` | `~/.codex/skills/` | symlink |
| `AgentPi` | `~/.pi/agent/skills/` | symlink |
| `AgentStandard` | `~/.agents/skills/` | hardlink |
| `AgentOpenClaw` | `~/.openclaw/skills/` | hardlink |

## Code Conventions

- Error wrapping: `fmt.Errorf("context: %w", err)` — lowercase, verb+noun, always `%w`
- Non-fatal errors: `fmt.Fprintf(os.Stderr, "warning: ...")`; program continues
- File not found: `config.Load` and `lock.Load` return empty struct, not error
- Atomic writes: use `fileutil.AtomicWrite` (tmp file + rename, same filesystem)
- Tests: table-driven with `t.Run`, `t.TempDir()`, `go-cmp` for struct diffs; no mocks

## Key Design Decisions

- `target_dir` is a package-level field; skills install into `<agentDir>/skills/<targetDir>/<skill>`
- SKILL.md scan: stops descending once SKILL.md is found in a directory (same as skm behaviour)
- `standard` and `openclaw` agents use hardlinks; all others use symlinks
- Lock file written atomically; never partially updated
- `installer.RepoStorePath` and `installer.ExpandPath` are exported for reuse by the CLI layer

