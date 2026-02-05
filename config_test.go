package main

import (
	"encoding/json"
	"os"
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

func TestShouldBlacklistPR(t *testing.T) {
	// Initialize logger for tests
	initLogger("ERROR")
	
	tests := []struct {
		name      string
		eventJSON string
		patterns  []string
		expected  bool
	}{
		{
			name: "No patterns configured - should not blacklist",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 1,
					"head": {"ref": "dependabot/docker/golang-1.26rc3-alpine"}
				}
			}`,
			patterns: []string{},
			expected: false,
		},
		{
			name: "Exact match with dependabot rc version",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 2,
					"head": {"ref": "dependabot/docker/golang-1.26rc3-alpine"}
				}
			}`,
			patterns: []string{`dependabot/docker/golang-1\..*rc.*-alpine`},
			expected: true,
		},
		{
			name: "Non-matching branch",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 3,
					"head": {"ref": "feature/new-feature"}
				}
			}`,
			patterns: []string{`dependabot/docker/golang-1\..*rc.*-alpine`},
			expected: false,
		},
		{
			name: "Multiple patterns - match first",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 4,
					"head": {"ref": "dependabot/npm/react-19.0.0-rc.1"}
				}
			}`,
			patterns: []string{
				`dependabot/npm/.*-rc\..*`,
				`renovate/.*-beta`,
			},
			expected: true,
		},
		{
			name: "Multiple patterns - match second",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 5,
					"head": {"ref": "renovate/package-1.0.0-beta"}
				}
			}`,
			patterns: []string{
				`dependabot/npm/.*-rc\..*`,
				`renovate/.*-beta`,
			},
			expected: true,
		},
		{
			name: "Pattern with anchor - should match start of string",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 6,
					"head": {"ref": "dependabot/docker/node-20.0.0-rc1"}
				}
			}`,
			patterns: []string{`^dependabot/`},
			expected: true,
		},
		{
			name: "Pattern with anchor - should not match middle of string",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 7,
					"head": {"ref": "my-dependabot/docker/node"}
				}
			}`,
			patterns: []string{`^dependabot/`},
			expected: false,
		},
		{
			name: "Complex pattern for golang rc versions",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 8,
					"head": {"ref": "dependabot/docker/golang-1.27rc2-alpine"}
				}
			}`,
			patterns: []string{`^dependabot/docker/golang-\d+\.\d+rc\d+-alpine$`},
			expected: true,
		},
		{
			name: "Pattern should not match stable golang version",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 9,
					"head": {"ref": "dependabot/docker/golang-1.27.0-alpine"}
				}
			}`,
			patterns: []string{`^dependabot/docker/golang-\d+\.\d+rc\d+-alpine$`},
			expected: false,
		},
		{
			name: "Case-sensitive pattern matching",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 10,
					"head": {"ref": "DEPENDABOT/docker/golang-1.26rc3-alpine"}
				}
			}`,
			patterns: []string{`^dependabot/`},
			expected: false,
		},
		{
			name: "Case-insensitive pattern matching",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 11,
					"head": {"ref": "DEPENDABOT/docker/golang-1.26rc3-alpine"}
				}
			}`,
			patterns: []string{`(?i)^dependabot/`},
			expected: true,
		},
		{
			name: "General rc version pattern",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 12,
					"head": {"ref": "dependabot/pip/django-5.0rc1"}
				}
			}`,
			patterns: []string{`rc\d+`},
			expected: true,
		},
		{
			name: "Wildcard pattern for all dependabot branches",
			eventJSON: `{
				"action": "opened",
				"pull_request": {
					"number": 13,
					"head": {"ref": "dependabot/npm/lodash-4.17.21"}
				}
			}`,
			patterns: []string{`^dependabot/.*`},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var event PullRequestEvent
			if err := json.Unmarshal([]byte(tt.eventJSON), &event); err != nil {
				t.Fatalf("Failed to unmarshal test event: %v", err)
			}

			result := shouldBlacklistPR(event, tt.patterns)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for PR #%d (branch=%s, patterns=%v)",
					tt.expected, result, event.PullRequest.Number,
					event.PullRequest.Head.Ref, tt.patterns)
			}
		})
	}
}

func TestLoadYAMLConfig(t *testing.T) {
	// Test with non-existent file
	config := loadYAMLConfig("non-existent-file.yaml")
	if config.Redis.Host != "" {
		t.Errorf("Expected empty config for non-existent file")
	}

	// Test with valid YAML file
	yamlContent := `
redis:
  host: testhost
  port: "1234"
  channel: test-channel
slack:
  channel_id: C123456
  redis_list: test-list
  reactions_list: test-reactions
  search_limit: 50
poppit:
  channel: test-poppit
timebomb:
  channel: test-timebomb
logging:
  level: DEBUG
draft_pr_filter:
  enabled_repos: ["repo1", "repo2"]
  allowed_branch_prefixes: ["feature/", "hotfix/"]
`
	// Create temporary test file
	tmpfile, err := os.CreateTemp("", "config-test-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(yamlContent)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	config = loadYAMLConfig(tmpfile.Name())
	if config.Redis.Host != "testhost" {
		t.Errorf("Expected Redis.Host to be 'testhost', got %q", config.Redis.Host)
	}
	if config.Redis.Port != "1234" {
		t.Errorf("Expected Redis.Port to be '1234', got %q", config.Redis.Port)
	}
	if config.Slack.SearchLimit != 50 {
		t.Errorf("Expected Slack.SearchLimit to be 50, got %d", config.Slack.SearchLimit)
	}
	if len(config.DraftPRFilter.EnabledRepos) != 2 {
		t.Errorf("Expected 2 enabled repos, got %d", len(config.DraftPRFilter.EnabledRepos))
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		envKey       string
		envValue     string
		yamlValue    string
		defaultValue string
		expected     string
	}{
		{
			name:         "Env var set",
			envKey:       "TEST_VAR_1",
			envValue:     "env-value",
			yamlValue:    "yaml-value",
			defaultValue: "default-value",
			expected:     "env-value",
		},
		{
			name:         "Env var not set, YAML value available",
			envKey:       "TEST_VAR_2",
			envValue:     "",
			yamlValue:    "yaml-value",
			defaultValue: "default-value",
			expected:     "yaml-value",
		},
		{
			name:         "Neither env nor YAML set",
			envKey:       "TEST_VAR_3",
			envValue:     "",
			yamlValue:    "",
			defaultValue: "default-value",
			expected:     "default-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set env var if needed
			if tt.envValue != "" {
				os.Setenv(tt.envKey, tt.envValue)
				defer os.Unsetenv(tt.envKey)
			}

			result := getEnvOrDefault(tt.envKey, tt.yamlValue, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}
