package main

import (
	"encoding/json"
	"testing"
)

func TestShouldNotifyDraftPR(t *testing.T) {
	// Initialize logger for tests
	initLogger("ERROR")
	
	tests := []struct {
		name           string
		eventJSON      string
		filterRepos    []string
		filterPrefixes []string
		expected       bool
	}{
		{
			name: "Draft PR with matching repo and branch prefix",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 1,
					"draft": true,
					"head": {"ref": "feature/new-feature"},
					"base": {"repo": {"full_name": "owner/repo"}}
				}
			}`,
			filterRepos:    []string{"owner/repo"},
			filterPrefixes: []string{"feature/"},
			expected:       true,
		},
		{
			name: "Draft PR with non-matching repo",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 2,
					"draft": true,
					"head": {"ref": "feature/test"},
					"base": {"repo": {"full_name": "different/repo"}}
				}
			}`,
			filterRepos:    []string{"owner/repo"},
			filterPrefixes: []string{"feature/"},
			expected:       false,
		},
		{
			name: "Draft PR with non-matching branch prefix",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 3,
					"draft": true,
					"head": {"ref": "bugfix/test"},
					"base": {"repo": {"full_name": "owner/repo"}}
				}
			}`,
			filterRepos:    []string{"owner/repo"},
			filterPrefixes: []string{"feature/", "release/"},
			expected:       false,
		},
		{
			name: "Draft PR with empty filter config",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 4,
					"draft": true,
					"head": {"ref": "feature/test"},
					"base": {"repo": {"full_name": "owner/repo"}}
				}
			}`,
			filterRepos:    []string{},
			filterPrefixes: []string{},
			expected:       false,
		},
		{
			name: "Draft PR matching one of multiple repos",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 5,
					"draft": true,
					"head": {"ref": "release/v1.0"},
					"base": {"repo": {"full_name": "team/service2"}}
				}
			}`,
			filterRepos:    []string{"team/service1", "team/service2", "team/service3"},
			filterPrefixes: []string{"release/", "hotfix/"},
			expected:       true,
		},
		{
			name: "Draft PR matching one of multiple prefixes",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 6,
					"draft": true,
					"head": {"ref": "hotfix/urgent-fix"},
					"base": {"repo": {"full_name": "owner/repo"}}
				}
			}`,
			filterRepos:    []string{"owner/repo"},
			filterPrefixes: []string{"feature/", "release/", "hotfix/"},
			expected:       true,
		},
		{
			name: "Draft PR with prefix-like branch but not exact match",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 7,
					"draft": true,
					"head": {"ref": "feat/test"},
					"base": {"repo": {"full_name": "owner/repo"}}
				}
			}`,
			filterRepos:    []string{"owner/repo"},
			filterPrefixes: []string{"feature/"},
			expected:       false,
		},
		{
			name: "Draft PR with only repos configured (no prefixes)",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 8,
					"draft": true,
					"head": {"ref": "any-branch"},
					"base": {"repo": {"full_name": "owner/repo"}}
				}
			}`,
			filterRepos:    []string{"owner/repo"},
			filterPrefixes: []string{},
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var event PullRequestEvent
			if err := json.Unmarshal([]byte(tt.eventJSON), &event); err != nil {
				t.Fatalf("Failed to unmarshal test event: %v", err)
			}

			filter := DraftPRFilterConfig{
				EnabledRepoNames:    tt.filterRepos,
				AllowedBranchStarts: tt.filterPrefixes,
			}

			result := shouldNotifyDraftPR(event, filter)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for PR #%d (repo=%s, branch=%s)",
					tt.expected, result, event.PullRequest.Number,
					event.PullRequest.Base.Repo.FullName,
					event.PullRequest.Head.Ref)
			}
		})
	}
}

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "Single item",
			input:    "owner/repo",
			expected: []string{"owner/repo"},
		},
		{
			name:     "Multiple items",
			input:    "repo1,repo2,repo3",
			expected: []string{"repo1", "repo2", "repo3"},
		},
		{
			name:     "Items with spaces",
			input:    "repo1 , repo2 ,  repo3",
			expected: []string{"repo1", "repo2", "repo3"},
		},
		{
			name:     "Items with empty entries",
			input:    "repo1,,repo2,,,repo3",
			expected: []string{"repo1", "repo2", "repo3"},
		},
		{
			name:     "Branch prefixes",
			input:    "feature/,release/,hotfix/",
			expected: []string{"feature/", "release/", "hotfix/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitAndTrim(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected length %d, got %d", len(tt.expected), len(result))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("At index %d: expected %q, got %q", i, tt.expected[i], result[i])
				}
			}
		})
	}
}
