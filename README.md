# skimi

A Go implementation of a skill manager for AI agents — inspired by [reorx/skm](https://github.com/reorx/skm).

## Overview

`skimi` manages AI agent skills across multiple agent platforms. It reads a declarative
configuration file (`skills.yaml`) and installs skills from git repositories or local paths
into agent-specific skill directories, creating symlinks or hardlinks as appropriate.

**Credit**: This project is based on the design and configuration format of
[skm](https://github.com/reorx/skm) by [reorx](https://github.com/reorx).
The core concepts (SKILL.md detection, lock file, agent directory conventions) are
preserved for compatibility. skimi adds Go performance, subdirectory installation support
(`target_dir`), and an interactive TUI for ad-hoc installs.

## Supported Agents

| Agent      | Skills Directory            | Link Type  |
|------------|-----------------------------|------------|
| claude     | `~/.claude/skills/`         | symlink    |
| standard   | `~/.agents/skills/`         | hardlink   |
| codex      | `~/.codex/skills/`          | symlink    |
| openclaw   | `~/.openclaw/skills/`       | hardlink   |
| pi         | `~/.pi/agent/skills/`       | symlink    |

## Configuration

**Config file**: `~/.config/skimi/skills.yaml`
**Lock file**: `~/.config/skimi/skills-lock.yaml`
**Skill store**: `~/.local/share/skimi/skills/`

### Example `skills.yaml`

```yaml
agents:
  default:
    - claude
    - standard

packages:
  - repo: github.com/example/ai-skills
    target_dir: example          # installs into <agent_skills_dir>/example/<skill_name>
    agents:
      includes:
        - claude

  - repo: github.com/myorg/shared-skills
    skills:
      - coding-assistant
      - code-review

  - local_path: ~/my-local-skills
    target_dir: local
```

## Installation

```bash
go install github.com/timonwong/skimi/cmd/skimi@latest
```

## Usage

```bash
# Install all skills from skills.yaml
skimi install

# Interactively install from a source
skimi install github.com/example/ai-skills

# List installed skills
skimi list

# Preview skills in a source without installing
skimi view github.com/example/ai-skills

# Check for updates
skimi check-updates

# Update all skills
skimi update

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
