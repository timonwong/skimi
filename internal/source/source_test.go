package source

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   ParsedSource
	}{
		// Local paths
		{
			name:   "absolute path",
			source: "/home/user/skills",
			want:   ParsedSource{Kind: SourceLocal, LocalPath: "/home/user/skills"},
		},
		{
			name:   "home relative path",
			source: "~/projects/skills",
			want:   ParsedSource{Kind: SourceLocal, LocalPath: "~/projects/skills"},
		},
		{
			name:   "current dir relative path",
			source: "./local-skills",
			want:   ParsedSource{Kind: SourceLocal, LocalPath: "./local-skills"},
		},
		{
			name:   "parent dir relative path",
			source: "../other-skills",
			want:   ParsedSource{Kind: SourceLocal, LocalPath: "../other-skills"},
		},

		// Shorthand format (owner/repo)
		{
			name:   "shorthand owner/repo",
			source: "owner/repo",
			want:   ParsedSource{Kind: SourceRemote, Repo: "github.com/owner/repo"},
		},
		{
			name:   "shorthand owner/repo/subdir",
			source: "owner/repo/skills/foo",
			want:   ParsedSource{Kind: SourceRemote, Repo: "github.com/owner/repo", Subdir: "skills/foo"},
		},
		{
			name:   "shorthand nowledge-co/community/nowledge-mem-npx-skills",
			source: "nowledge-co/community/nowledge-mem-npx-skills",
			want:   ParsedSource{Kind: SourceRemote, Repo: "github.com/nowledge-co/community", Subdir: "nowledge-mem-npx-skills"},
		},
		{
			name:   "shorthand single segment",
			source: "owner",
			want:   ParsedSource{Kind: SourceRemote, Repo: "github.com/owner"},
		},

		// Domain format (domain/owner/repo)
		{
			name:   "domain format github.com/owner/repo",
			source: "github.com/owner/repo",
			want:   ParsedSource{Kind: SourceRemote, Repo: "github.com/owner/repo"},
		},
		{
			name:   "domain format github.com/owner/repo/subdir",
			source: "github.com/owner/repo/subdir",
			want:   ParsedSource{Kind: SourceRemote, Repo: "github.com/owner/repo", Subdir: "subdir"},
		},
		{
			name:   "domain format gitlab.com/owner/repo/deep/path",
			source: "gitlab.com/owner/repo/deep/path",
			want:   ParsedSource{Kind: SourceRemote, Repo: "gitlab.com/owner/repo", Subdir: "deep/path"},
		},
		{
			name:   "domain format incomplete",
			source: "github.com/owner",
			want:   ParsedSource{Kind: SourceRemote, Repo: "github.com/owner"},
		},

		// HTTPS URLs
		{
			name:   "https URL basic",
			source: "https://github.com/owner/repo",
			want:   ParsedSource{Kind: SourceRemote, Repo: "github.com/owner/repo"},
		},
		{
			name:   "https URL with .git",
			source: "https://github.com/owner/repo.git",
			want:   ParsedSource{Kind: SourceRemote, Repo: "github.com/owner/repo"},
		},
		{
			name:   "https URL with subdir",
			source: "https://github.com/owner/repo/subdir/path",
			want:   ParsedSource{Kind: SourceRemote, Repo: "github.com/owner/repo", Subdir: "subdir/path"},
		},
		{
			name:   "http URL",
			source: "http://github.com/owner/repo",
			want:   ParsedSource{Kind: SourceRemote, Repo: "github.com/owner/repo"},
		},

		// SSH URLs
		{
			name:   "git SSH basic",
			source: "git@github.com:owner/repo",
			want:   ParsedSource{Kind: SourceRemote, Repo: "github.com/owner/repo"},
		},
		{
			name:   "git SSH with .git",
			source: "git@github.com:owner/repo.git",
			want:   ParsedSource{Kind: SourceRemote, Repo: "github.com/owner/repo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.source)
			if err != nil {
				t.Fatalf("Parse() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Parse() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestIsLocalPath(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{"/absolute/path", true},
		{"~/home/path", true},
		{"./relative/path", true},
		{"../parent/path", true},
		{"owner/repo", false},
		{"github.com/owner/repo", false},
		{"https://github.com/owner/repo", false},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			got := isLocalPath(tt.source)
			if got != tt.want {
				t.Errorf("isLocalPath(%q) = %v, want %v", tt.source, got, tt.want)
			}
		})
	}
}
