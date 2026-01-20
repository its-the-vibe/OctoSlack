package main

import "github.com/slack-go/slack"

// PullRequestEvent represents a GitHub pull request event
type PullRequestEvent struct {
	Action      string `json:"action"`
	PullRequest struct {
		Number         int    `json:"number"`
		Title          string `json:"title"`
		HTMLURL        string `json:"html_url"`
		Merged         bool   `json:"merged"`
		MergeCommitSHA string `json:"merge_commit_sha"`
		User           struct {
			Login string `json:"login"`
		} `json:"user"`
		Head struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Repo struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"base"`
	} `json:"pull_request"`
}

// SlackMessage represents a Slack message payload for SlackLiner
type SlackMessage struct {
	Channel  string                 `json:"channel"`
	Text     string                 `json:"text"`
	ThreadTS string                 `json:"thread_ts,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// SlackReaction represents a Slack reaction payload
type SlackReaction struct {
	Reaction string `json:"reaction"`
	Channel  string `json:"channel"`
	TS       string `json:"ts"`
}

// SlackHistoryMessage represents a message from Slack history
type SlackHistoryMessage struct {
	TS       string
	ThreadTS string
	Metadata *slack.SlackMetadata
}

// PoppitCommandOutput represents a poppit command output event
type PoppitCommandOutput struct {
	Type     string                 `json:"type"`
	Command  string                 `json:"command"`
	Output   string                 `json:"output"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}
