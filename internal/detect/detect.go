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

// Scan recursively walks rootDir looking for directories that contain a
// SKILL.md file. When a SKILL.md is found the walk does not descend further
// into that directory (matching skm behaviour). Returns one DetectedSkill per
// SKILL.md found.
func Scan(rootDir string) ([]types.DetectedSkill, error) {
	var skills []types.DetectedSkill

	err := walk(rootDir, rootDir, &skills)
	if err != nil {
		return nil, err
	}
	return skills, nil
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
