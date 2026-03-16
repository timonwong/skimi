package source

import (
	"path/filepath"
	"strings"
)

// SourceKind indicates whether a source is local or remote.
type SourceKind int

const (
	SourceLocal SourceKind = iota
	SourceRemote
)

// ParsedSource holds the parsed components of a source specification.
type ParsedSource struct {
	Kind      SourceKind
	Repo      string // normalized repository identifier (e.g., "github.com/owner/repo")
	Subdir    string // optional sub-directory within the repo
	CloneURL  string // URL to use for cloning (preserves original protocol); empty means use Repo
	LocalPath string // absolute or relative local path (only set for SourceLocal)
}

// GetCloneURL returns the URL to use for git clone operations.
// If CloneURL is set (for explicit URLs like git@ or https://), it returns that.
// Otherwise, it returns Repo which will be converted to HTTPS by git.Clone.
func (p ParsedSource) GetCloneURL() string {
	if p.CloneURL != "" {
		return p.CloneURL
	}
	return p.Repo
}

// Parse parses a source string into its components.
//
// Parsing rules:
//   - Starts with /, ~/, ./, ../ → local path
//   - Starts with http://, https://, git@ → full URL, may have extra path as subdir
//   - Otherwise split by /:
//   - First segment contains '.' → domain format (e.g., github.com/owner/repo)
//     First 3 segments are repo, rest is subdir
//   - First segment has no '.' → shorthand format (e.g., owner/repo)
//     First 2 segments are owner/repo, rest is subdir
//     Auto-expands to github.com/owner/repo
func Parse(source string) (ParsedSource, error) {
	// Local path detection
	if isLocalPath(source) {
		return ParsedSource{
			Kind:      SourceLocal,
			LocalPath: source,
		}, nil
	}

	// Full URL detection
	if strings.HasPrefix(source, "http://") ||
		strings.HasPrefix(source, "https://") ||
		strings.HasPrefix(source, "git@") {
		return parseURL(source), nil
	}

	// Segment-based parsing
	return parseSegments(source), nil
}

// isLocalPath returns true if the source looks like a local filesystem path.
func isLocalPath(source string) bool {
	return strings.HasPrefix(source, "/") ||
		strings.HasPrefix(source, "~/") ||
		strings.HasPrefix(source, "./") ||
		strings.HasPrefix(source, "../")
}

// parseURL parses a full URL (http/https/git@) and extracts repo and subdir.
func parseURL(source string) ParsedSource {
	// For URLs like https://github.com/owner/repo/tree/main/subdir
	// or git@github.com:owner/repo.git
	// We need to extract the base repo URL and any subdir path

	// Handle git@ SSH URLs
	if strings.HasPrefix(source, "git@") {
		return parseGitSSH(source)
	}

	// Handle https:// or http://
	return parseHTTPURL(source)
}

// parseGitSSH parses git@host:owner/repo.git format.
func parseGitSSH(source string) ParsedSource {
	// git@github.com:owner/repo.git → github.com/owner/repo
	// git@github.com:owner/repo → github.com/owner/repo
	s := strings.TrimPrefix(source, "git@")
	s = strings.TrimSuffix(s, ".git")

	// Replace : with /
	s = strings.Replace(s, ":", "/", 1)

	// Split and take first 3 segments as repo (host/owner/repo)
	parts := strings.Split(s, "/")
	if len(parts) < 3 {
		return ParsedSource{
			Kind:     SourceRemote,
			Repo:     s,
			CloneURL: source, // preserve original SSH URL
		}
	}

	repo := strings.Join(parts[:3], "/")
	subdir := ""
	if len(parts) > 3 {
		subdir = strings.Join(parts[3:], "/")
	}

	return ParsedSource{
		Kind:     SourceRemote,
		Repo:     repo,
		Subdir:   subdir,
		CloneURL: source, // preserve original SSH URL
	}
}

// parseHTTPURL parses http(s)://host/owner/repo paths.
func parseHTTPURL(source string) ParsedSource {
	// Remove protocol
	s := source
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimSuffix(s, ".git")

	// Split and take first 3 segments (host/owner/repo)
	parts := strings.Split(s, "/")
	if len(parts) < 3 {
		return ParsedSource{
			Kind:     SourceRemote,
			Repo:     s,
			CloneURL: source, // preserve original URL with protocol
		}
	}

	repo := strings.Join(parts[:3], "/")
	subdir := ""
	if len(parts) > 3 {
		subdir = strings.Join(parts[3:], "/")
	}

	return ParsedSource{
		Kind:     SourceRemote,
		Repo:     repo,
		Subdir:   subdir,
		CloneURL: source, // preserve original URL with protocol
	}
}

// parseSegments parses source by splitting on / and applying heuristics.
func parseSegments(source string) ParsedSource {
	parts := strings.Split(source, "/")

	// Check if first segment looks like a domain (contains .)
	if len(parts) > 0 && strings.Contains(parts[0], ".") {
		// Domain format: domain/owner/repo[/subdir...]
		// First 3 segments are the repo
		if len(parts) < 3 {
			return ParsedSource{
				Kind: SourceRemote,
				Repo: source,
			}
		}

		repo := strings.Join(parts[:3], "/")
		subdir := ""
		if len(parts) > 3 {
			subdir = strings.Join(parts[3:], "/")
		}

		return ParsedSource{
			Kind:   SourceRemote,
			Repo:   repo,
			Subdir: subdir,
		}
	}

	// Shorthand format: owner/repo[/subdir...]
	// First 2 segments are owner/repo, auto-expand to github.com/owner/repo
	if len(parts) < 2 {
		// Just owner without repo - treat as github.com/owner
		return ParsedSource{
			Kind: SourceRemote,
			Repo: "github.com/" + source,
		}
	}

	repo := "github.com/" + parts[0] + "/" + parts[1]
	subdir := ""
	if len(parts) > 2 {
		subdir = filepath.Join(parts[2:]...)
	}

	return ParsedSource{
		Kind:   SourceRemote,
		Repo:   repo,
		Subdir: subdir,
	}
}
