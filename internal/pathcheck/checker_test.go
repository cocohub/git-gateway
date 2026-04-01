package pathcheck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubChecker_GetChangedPaths(t *testing.T) {
	// Mock GitHub API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Return mock compare response
		response := map[string]interface{}{
			"files": []map[string]string{
				{"filename": "src/main.go"},
				{"filename": "src/handler.go"},
				{"filename": ".github/workflows/ci.yml"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	checker := &GitHubChecker{
		token:  "test-token",
		client: server.Client(),
	}

	// Override the URL construction for testing
	// In real code, this would need a URL override mechanism
	// For now, we test the extraction logic separately

	t.Run("extractOwnerRepo", func(t *testing.T) {
		tests := []struct {
			input string
			want  string
		}{
			{"github.com/owner/repo.git", "owner/repo"},
			{"github.com/owner/repo", "owner/repo"},
			{"owner/repo.git", "owner/repo"},
			{"invalid", ""},
		}

		for _, tt := range tests {
			got := extractOwnerRepo(tt.input)
			if got != tt.want {
				t.Errorf("extractOwnerRepo(%q) = %q, want %q", tt.input, got, tt.want)
			}
		}
	})

	t.Run("new branch returns empty paths", func(t *testing.T) {
		paths, err := checker.GetChangedPaths(context.Background(),
			"github.com/owner/repo.git",
			"0000000000000000000000000000000000000000",
			"abc123")

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(paths) != 0 {
			t.Errorf("expected empty paths for new branch, got %v", paths)
		}
	})
}

func TestNoOpChecker(t *testing.T) {
	checker := &NoOpChecker{}

	paths, err := checker.GetChangedPaths(context.Background(), "repo", "old", "new")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected empty paths, got %v", paths)
	}

	err = checker.RevertRef(context.Background(), "repo", "ref", "sha")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
