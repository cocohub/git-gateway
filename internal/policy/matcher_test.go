package policy

import "testing"

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		want    bool
	}{
		// Simple wildcards
		{"*.go", "main.go", true},
		{"*.go", "main.rs", false},
		{"test_*", "test_foo", true},
		{"test_*", "foo_test", false},

		// Double star patterns
		{"**/*.go", "main.go", true},
		{"**/*.go", "src/main.go", true},
		{"**/*.go", "src/pkg/main.go", true},
		{"**/*.go", "main.rs", false},

		// Path patterns
		{"github.com/**", "github.com/owner/repo", true},
		{"github.com/acme/**", "github.com/acme/repo", true},
		{"github.com/acme/**", "github.com/other/repo", false},

		// Ref patterns
		{"refs/heads/main", "refs/heads/main", true},
		{"refs/heads/main", "refs/heads/master", false},
		{"refs/heads/**", "refs/heads/feature/test", true},
		{"refs/heads/feature/**", "refs/heads/feature/my-feature", true},
		{"refs/heads/feature/**", "refs/heads/main", false},
		{"refs/tags/**", "refs/tags/v1.0.0", true},

		// Edge cases
		{"", "", true},
		{"*", "anything", true},
		{"**", "a/b/c", true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.s, func(t *testing.T) {
			got := MatchGlob(tt.pattern, tt.s)
			if got != tt.want {
				t.Errorf("MatchGlob(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.want)
			}
		})
	}
}

func TestMatchAny(t *testing.T) {
	patterns := []string{
		"refs/heads/main",
		"refs/heads/master",
		"refs/tags/**",
	}

	tests := []struct {
		s    string
		want bool
	}{
		{"refs/heads/main", true},
		{"refs/heads/master", true},
		{"refs/tags/v1.0.0", true},
		{"refs/heads/feature/test", false},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := MatchAny(tt.s, patterns)
			if got != tt.want {
				t.Errorf("MatchAny(%q, patterns) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}
