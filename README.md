# skimi

A Go implementation of a skill manager for AI agents — inspired by [reorx/skm](https://github.com/reorx/skm).

## Overview

`skimi` manages AI agent skills across multiple agent platforms. It reads a declarative
configuration file (`skills.yaml`) and installs skills from git repositories or local paths
into agent-specific skill directories using flat symlinks. On Windows, where creating a
symlink requires Developer Mode or an elevated shell, skimi falls back to a directory
junction, which needs no such privilege.

**Credit**: This project is based on the design and configuration format of
[skm](https://github.com/reorx/skm) by [reorx](https://github.com/reorx).
The core concepts (SKILL.md detection, lock file, agent directory conventions) are
preserved for compatibility. skimi adds Go performance, repository subdirectory support,
path selectors, and an interactive TUI for ad-hoc installs.

## Supported Agents

| Agent      | Skills Directory            |
|------------|-----------------------------|
| claude     | `~/.claude/skills/`         |
| standard   | `~/.agents/skills/`         |
| codex      | `~/.codex/skills/`          |
| openclaw   | `~/.openclaw/skills/`       |
| pi         | `~/.pi/agent/skills/`       |

## Configuration

**Config file**: `~/.config/skimi/skills.yaml`
**Lock file**: `~/.config/skimi/skills-lock.yaml`
**Skill store**: `~/.local/share/skimi/skills/`

See the [complete configuration reference](docs/configuration.md) for every
field, default, constraint, selector form, collision rule, and migration detail.

### Example `skills.yaml`

```yaml
agents:
  default:
    - claude
    - standard

packages:
  - repo: github.com/example/ai-skills
    agents:
      includes:
        - claude

  - repo: github.com/myorg/shared-skills
    skills:
      - coding-assistant
      - path: review/code-review

  - local_path: ~/my-local-skills
```

## Installation

### Homebrew

On macOS or Linux:

```sh
brew install timonwong/tap/skimi
```

### GitHub Release

Download a pre-built archive from
[GitHub Releases](https://github.com/timonwong/skimi/releases) (Linux, macOS,
Windows; amd64 and arm64). On Linux and macOS:

```sh
tar -xzf skimi_0.2.0_darwin_arm64.tar.gz
chmod +x skimi
sudo mv skimi /usr/local/bin/skimi
skimi --help
```

Replace `0.2.0` and `darwin_arm64` with your release version and platform.
Each release includes a SHA-256 checksum file for verification.

### Manual

Install the latest version from source with Go 1.23 or later:

```sh
go install github.com/timonwong/skimi/cmd/skimi@latest
```

Source installations report the development version rather than a stamped
release version.

## Usage

```bash
# Install all skills from skills.yaml (full sync: removes skills not declared there)
skimi install

# Interactively install from a source (additive: other installed skills stay)
skimi install github.com/example/ai-skills

# List installed skills
skimi list

# Edit skills.yaml
skimi edit

# Preview skills in a source without installing
skimi view github.com/example/ai-skills

# Check for updates
skimi check-updates

# Update selected skills
skimi update <skill-name> [skill-name...]

# Update all remote skills
skimi update --all

# Remove a skill
skimi remove <skill-name>
```

## Global Flags

| Flag          | Default                              | Description                     |
|---------------|--------------------------------------|---------------------------------|
| `--config`    | `~/.config/skimi/skills.yaml`        | Config file path                |
| `--lock`      | `~/.config/skimi/skills-lock.yaml`   | Lock file path                  |
| `--store`     | `~/.local/share/skimi/skills/`       | Skill store directory           |

## License

MIT — see [LICENSE](LICENSE).
