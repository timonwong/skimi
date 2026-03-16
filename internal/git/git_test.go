package git

import "testing"

func TestRepoURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"github.com/foo/bar", "https://github.com/foo/bar"},
		{"https://github.com/foo/bar", "https://github.com/foo/bar"},
		{"http://github.com/foo/bar", "http://github.com/foo/bar"},
		{"git@github.com:foo/bar", "git@github.com:foo/bar"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := repoURL(tt.input)
			if got != tt.want {
				t.Errorf("repoURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
