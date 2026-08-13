# Configuration reference

`skimi` reads `~/.config/skimi/skills.yaml` by default. The YAML decoder is
strict: unknown fields, invalid agent names, and invalid selectors are errors.
Installation stops before changing agent skill directories when validation
fails.

## Complete example

```yaml
agents:
  default:
    - claude
    - standard

packages:
  - repo: github.com/example/ai-skills
    skills:
      - code-review
      - path: workflows/wait
    agents:
      includes:
        - claude
        - codex
      excludes:
        - codex

  - repo: github.com/example/ai-skills/experimental
    agents:
      excludes:
        - openclaw

  - local_path: ~/my-local-skills
```

## Top-level fields

| Field | Type | Required | Default | Description |
|---|---|---:|---|---|
| `agents` | mapping | no | all supported agents | Global agent defaults. |
| `agents.default` | list of strings | no | all supported agents | Agents used by packages without an `agents` override. |
| `packages` | list | yes for installation | empty | Ordered skill sources. Later packages win name conflicts. |

Supported agent names and destinations:

| Agent | Destination |
|---|---|
| `claude` | `~/.claude/skills/` |
| `standard` | `~/.agents/skills/` |
| `codex` | `~/.codex/skills/` |
| `openclaw` | `~/.openclaw/skills/` |
| `pi` | `~/.pi/agent/skills/` |

All agents use directory symlinks. Every skill is installed flat as
`<destination>/<skill-name>`.

## Package fields

| Field | Type | Required | Default | Description |
|---|---|---:|---|---|
| `repo` | string | exactly one source | none | Remote Git repository, optionally followed by a subdirectory. |
| `local_path` | string | exactly one source | none | Local directory. `~/`, relative, and absolute paths are supported. |
| `skills` | list | no | every detected skill | Name or path selectors. |
| `agents` | mapping | no | global defaults | Package-specific agent filters. |
| `agents.includes` | list of strings | no | global defaults | Replaces the candidate agent set when non-empty. |
| `agents.excludes` | list of strings | no | empty | Removes agents from the candidate set after `includes`. |
| `target_dir` | string | no | ignored | Deprecated compatibility field; emits a warning and will be removed in the next major version. |

`repo` and `local_path` are mutually exclusive and exactly one is required.

## Remote sources and subdirectories

These forms identify the same GitHub repository:

```yaml
repo: owner/repo
repo: github.com/owner/repo
repo: https://github.com/owner/repo.git
repo: git@github.com:owner/repo.git
```

Additional path segments select a directory inside the repository:

```yaml
repo: github.com/owner/repo/team-skills
```

If the effective source directory contains a `skills/` directory, skimi scans
only that directory. Otherwise it scans the effective source directory. The
scan descends through grouping directories and stops descending when it finds a
`SKILL.md`.

## Skill selectors

A string selects every detected skill with that declared name:

```yaml
skills:
  - wait
```

A path selector chooses one directory exactly:

```yaml
skills:
  - path: workflows/wait
```

Paths are relative to the effective scan root. When a source has `skills/`, the
path is relative to `skills/`; otherwise it is relative to the source itself.
Paths must use `/`, be clean and relative, and cannot contain `..`. Every
selector must match. Repeated selectors install a skill only once.

Skill names must contain only lowercase ASCII letters, numbers, and single
hyphens, cannot start or end with a hyphen, and cannot exceed 64 characters.
Invalid names are errors and are not rewritten.

## Duplicate names

Skills are installed by declared name. When more than one selected skill has
the same name, the last one wins and skimi prints a warning that identifies the
loser and winner.

- Packages are evaluated in configuration order; a later package wins.
- Within a package, source-relative paths are sorted lexically; the later path
  wins.

Use a `path` selector when you want an earlier same-name skill explicitly.
Last-wins applies only to skills managed in the current skimi plan. An existing
destination that is not recorded in the lock file and does not already point to
the selected source causes a preflight error; skimi does not overwrite it.

## Paths and flags

| Purpose | Default | Override |
|---|---|---|
| Config | `~/.config/skimi/skills.yaml` | `--config` |
| Lock | `~/.config/skimi/skills-lock.yaml` | `--lock` |
| Store | `~/.local/share/skimi/skills/` | `--store` |

The global flags apply to every command. `install` and `update` also support:

| Flag | Default | Description |
|---|---|---|
| `--dry-run` | `false` | Show link and stale-link changes without applying them. |
| `--verbose`, `-v` | `false` | Enable additional installation detail. |

`update` additionally supports `--all` to update every installed remote skill.

## Lock file and migration

`skills-lock.yaml` is generated state. Version 2 records the selected source
and source-relative path so `skimi list` can show the actual winner of a name
collision. Do not edit the lock file manually.

Version 1 lock files remain readable. A successful `install` or `update`
migrates managed nested links or legacy hardlink trees to flat symlinks and
writes version 2 atomically. Only paths recorded in the old lock are removed;
empty legacy grouping directories are removed afterward. A failed operation
rolls link changes back and leaves the old lock usable. A lock version newer
than the running binary supports is rejected.
