package detect

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/timonwong/skimi/internal/types"
	"gopkg.in/yaml.v3"
)

const skillFile = "SKILL.md"

// Scan looks for SKILL.md files in subdirectories.
// Priority: if rootDir contains a skills/ subdirectory, scan only within it.
// Otherwise, scan subdirectories of rootDir directly.
// When a SKILL.md is found the walk does not descend further into that
// directory (matching skm behaviour). Returns one DetectedSkill per unique
// skill name found; when duplicates exist the shallowest path wins.
func Scan(rootDir string) ([]types.DetectedSkill, error) {
	// Priority: check for skills/ subdirectory first
	scanDir := rootDir
	if skillsDir := filepath.Join(rootDir, "skills"); isDir(skillsDir) {
		scanDir = skillsDir
	}

	var raw []types.DetectedSkill
	if err := walk(scanDir, scanDir, &raw); err != nil {
		return nil, err
	}
	return deduplicateSkills(raw, scanDir), nil
}

// isDir reports whether path is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// deduplicateSkills returns skills with duplicate names removed, keeping the
// entry whose SkillPath is shallowest relative to rootDir. A warning is printed
// to stderr for every duplicate that is dropped.
func deduplicateSkills(skills []types.DetectedSkill, rootDir string) []types.DetectedSkill {
	type entry struct {
		skill types.DetectedSkill
		depth int
		order int
	}
	best := make(map[string]entry, len(skills))
	for i, s := range skills {
		rel, _ := filepath.Rel(rootDir, s.SkillPath)
		depth := strings.Count(filepath.ToSlash(rel), "/")
		if prev, ok := best[s.Name]; !ok || depth < prev.depth {
			if ok {
				fmt.Fprintf(os.Stderr, "warning: duplicate skill %q: keeping %s, dropping %s\n",
					s.Name, s.SkillPath, prev.skill.SkillPath)
			}
			best[s.Name] = entry{skill: s, depth: depth, order: i}
		} else {
			fmt.Fprintf(os.Stderr, "warning: duplicate skill %q: keeping %s, dropping %s\n",
				s.Name, prev.skill.SkillPath, s.SkillPath)
		}
	}

	// Rebuild in original insertion order for deterministic output.
	out := make([]types.DetectedSkill, 0, len(best))
	for _, s := range skills {
		if e, ok := best[s.Name]; ok && e.skill.SkillPath == s.SkillPath {
			out = append(out, s)
			delete(best, s.Name)
		}
	}
	return out
}

// walk is the recursive helper for Scan.
func walk(rootDir, dir string, skills *[]types.DetectedSkill) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(dir, e.Name())
		skillMD := filepath.Join(sub, skillFile)

		if _, err := os.Stat(skillMD); err == nil {
			// Found a SKILL.md — parse it and stop descending.
			skill, err := parseSkillMD(skillMD, sub)
			if err != nil {
				return err
			}
			*skills = append(*skills, skill)
			continue // do not descend further
		}

		// No SKILL.md here — recurse.
		if err := walk(rootDir, sub, skills); err != nil {
			return err
		}
	}
	return nil
}

// skillFrontmatter mirrors the YAML frontmatter fields we care about.
type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// parseSkillMD reads a SKILL.md file, extracts its YAML frontmatter, and
// returns a DetectedSkill. The name defaults to the directory basename if the
// frontmatter does not set one.
func parseSkillMD(path, skillDir string) (types.DetectedSkill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return types.DetectedSkill{}, fmt.Errorf("read %s: %w", path, err)
	}

	fm, err := extractFrontmatter(data)
	if err != nil {
		// Non-fatal — just use defaults.
		fm = skillFrontmatter{}
	}

	name := fm.Name
	if name == "" {
		name = filepath.Base(skillDir)
	}

	return types.DetectedSkill{
		Name:        name,
		Description: fm.Description,
		SkillPath:   skillDir,
	}, nil
}

// extractFrontmatter parses YAML frontmatter delimited by "---" lines at the
// beginning of a Markdown file.
func extractFrontmatter(data []byte) (skillFrontmatter, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))

	// First non-empty line must be "---".
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line != "---" {
			return skillFrontmatter{}, fmt.Errorf("no frontmatter")
		}
		break
	}

	var sb strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
	}

	var fm skillFrontmatter
	if err := yaml.Unmarshal([]byte(sb.String()), &fm); err != nil {
		return skillFrontmatter{}, err
	}
	return fm, nil
}
