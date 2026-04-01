// Package pathcheck implements path-level access control by querying provider APIs.
package pathcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Checker determines which files changed in a push and can revert refs.
type Checker interface {
	// GetChangedPaths returns the list of file paths modified between oldSHA and newSHA.
	GetChangedPaths(ctx context.Context, repo, oldSHA, newSHA string) ([]string, error)

	// RevertRef force-updates a ref back to the given SHA.
	RevertRef(ctx context.Context, repo, ref, sha string) error
}

// GitHubChecker implements Checker using the GitHub API.
type GitHubChecker struct {
	token  string
	client *http.Client
}

// NewGitHubChecker creates a GitHub-based path checker.
func NewGitHubChecker(token string) *GitHubChecker {
	return &GitHubChecker{
		token:  token,
		client: &http.Client{},
	}
}

// GetChangedPaths returns files changed between two commits using GitHub's compare API.
func (g *GitHubChecker) GetChangedPaths(ctx context.Context, repo, oldSHA, newSHA string) ([]string, error) {
	// repo format: "github.com/owner/repo.git" -> need "owner/repo"
	ownerRepo := extractOwnerRepo(repo)
	if ownerRepo == "" {
		return nil, fmt.Errorf("invalid repo format: %s", repo)
	}

	// For new branches, oldSHA is all zeros - can't compare
	if strings.HasPrefix(oldSHA, "000000") {
		// New branch - would need to compare against the default branch
		// For now, return empty (allow all) for new branches
		return []string{}, nil
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/compare/%s...%s", ownerRepo, oldSHA, newSHA)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github compare request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github compare failed: %s - %s", resp.Status, string(body))
	}

	var result struct {
		Files []struct {
			Filename string `json:"filename"`
		} `json:"files"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode github response: %w", err)
	}

	paths := make([]string, len(result.Files))
	for i, f := range result.Files {
		paths[i] = f.Filename
	}

	return paths, nil
}

// RevertRef force-updates a ref back to the given SHA using GitHub's refs API.
func (g *GitHubChecker) RevertRef(ctx context.Context, repo, ref, sha string) error {
	ownerRepo := extractOwnerRepo(repo)
	if ownerRepo == "" {
		return fmt.Errorf("invalid repo format: %s", repo)
	}

	// ref format: "refs/heads/branch" -> need "heads/branch"
	refPath := strings.TrimPrefix(ref, "refs/")

	url := fmt.Sprintf("https://api.github.com/repos/%s/git/refs/%s", ownerRepo, refPath)

	body := fmt.Sprintf(`{"sha":"%s","force":true}`, sha)
	req, err := http.NewRequestWithContext(ctx, "PATCH", url, strings.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("github ref update failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github ref update failed: %s - %s", resp.Status, string(respBody))
	}

	return nil
}

// extractOwnerRepo converts "github.com/owner/repo.git" to "owner/repo".
func extractOwnerRepo(repo string) string {
	// Remove host prefix
	repo = strings.TrimPrefix(repo, "github.com/")

	// Remove .git suffix
	repo = strings.TrimSuffix(repo, ".git")

	// Should have "owner/repo" format
	if !strings.Contains(repo, "/") {
		return ""
	}

	return repo
}

// NoOpChecker is a checker that allows all paths (for when path checking is disabled).
type NoOpChecker struct{}

func (n *NoOpChecker) GetChangedPaths(ctx context.Context, repo, oldSHA, newSHA string) ([]string, error) {
	return []string{}, nil
}

func (n *NoOpChecker) RevertRef(ctx context.Context, repo, ref, sha string) error {
	return nil
}
