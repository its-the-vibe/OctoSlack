package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/slack-go/slack"
)

func handlePullRequestEvent(ctx context.Context, payload string, rdb *redis.Client, slackClient *slack.Client, config Config) error {
	var event PullRequestEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}

	// Process review_requested events
	if event.Action == "review_requested" {
		// Apply blacklist filter
		if shouldBlacklistPR(event, config.BranchBlacklist) {
			logger.Debug("PR #%d ignored - branch blacklisted", event.PullRequest.Number)
			return nil
		}
		return handlePRNotification(ctx, event, rdb, config)
	}

	// Process opened events for non-draft PRs
	if event.Action == "opened" && !event.PullRequest.Draft {
		// Apply blacklist filter
		if shouldBlacklistPR(event, config.BranchBlacklist) {
			logger.Debug("PR #%d ignored - branch blacklisted", event.PullRequest.Number)
			return nil
		}
		return handlePRNotification(ctx, event, rdb, config)
	}

	// Process opened events for draft PRs if they match the filter criteria
	if event.Action == "opened" && event.PullRequest.Draft {
		if shouldNotifyDraftPR(event, config.DraftPRFilter) {
			return handlePRNotification(ctx, event, rdb, config)
		}
		logger.Debug("Draft PR #%d ignored - does not match filter criteria", event.PullRequest.Number)
		return nil
	}

	// Process closed events where PR was merged
	if event.Action == "closed" && event.PullRequest.Merged {
		return handlePRMerged(ctx, event, rdb, slackClient, config)
	}

	// Process closed events where PR was NOT merged (rejected)
	if event.Action == "closed" && !event.PullRequest.Merged {
		return handlePRClosed(ctx, event, rdb, slackClient, config)
	}

	logger.Debug("Ignoring event with action: %s (merged: %v, draft: %v)", event.Action, event.PullRequest.Merged, event.PullRequest.Draft)
	return nil
}

func handlePRNotification(ctx context.Context, event PullRequestEvent, rdb *redis.Client, config Config) error {
	logger.Info("Processing %s event for PR #%d", event.Action, event.PullRequest.Number)

	// Create header based on event type
	var header string
	switch event.Action {
	case "review_requested":
		header = "👀 Review Requested for Pull Request!"
	case "opened":
		header = "🚀 New Pull Request Opened!"
	default:
		logger.Warn("Unexpected action '%s' in handlePRNotification", event.Action)
		header = "📢 Pull Request Notification"
	}

	// Create Slack message text
	messageText := fmt.Sprintf(
		"%s\n\n"+
			"*Repository:* %s\n"+
			"*PR #%d:* %s\n"+
			"*Author:* %s\n"+
			"*Branch:* %s\n"+
			"*Link:* <%s|View PR>",
		header,
		event.PullRequest.Base.Repo.FullName,
		event.PullRequest.Number,
		event.PullRequest.Title,
		event.PullRequest.User.Login,
		event.PullRequest.Head.Ref,
		event.PullRequest.HTMLURL,
	)

	// Create message with metadata for future automation
	slackMessage := SlackMessage{
		Channel: config.SlackChannelID,
		Text:    messageText,
		Metadata: map[string]interface{}{
			"event_type": event.Action,
			"event_payload": map[string]interface{}{
				"pr_number":  event.PullRequest.Number,
				"repository": event.PullRequest.Base.Repo.FullName,
				"pr_url":     event.PullRequest.HTMLURL,
				"author":     event.PullRequest.User.Login,
				"branch":     event.PullRequest.Head.Ref,
			},
		},
	}

	return pushToSlackList(ctx, rdb, config.SlackRedisList, slackMessage)
}

func handlePRMerged(ctx context.Context, event PullRequestEvent, rdb *redis.Client, slackClient *slack.Client, config Config) error {
	logger.Info("Processing closed (merged) event for PR #%d with merge commit %s",
		event.PullRequest.Number, event.PullRequest.MergeCommitSHA)

	// Search for the original review message in Slack
	matchedMessage, err := findMessageByMetadata(ctx, slackClient, config, "pr_url", event.PullRequest.HTMLURL)
	if err != nil {
		return fmt.Errorf("failed to search Slack messages: %w", err)
	}

	if matchedMessage == nil {
		logger.Warn("No matching Slack message found for PR URL: %s", event.PullRequest.HTMLURL)
		return nil
	}

	logger.Debug("Found matching message with ts: %s", matchedMessage.TS)

	// Reply to the message in a thread
	shortCommitSHA := event.PullRequest.MergeCommitSHA
	if len(shortCommitSHA) > 7 {
		shortCommitSHA = shortCommitSHA[:7]
	}
	replyText := fmt.Sprintf("✅ Pull Request merged! Commit: %s", shortCommitSHA)

	slackMessage := SlackMessage{
		Channel:  config.SlackChannelID,
		Text:     replyText,
		ThreadTS: matchedMessage.TS, // Reply in thread
		Metadata: map[string]interface{}{
			"event_type": "closed",
			"event_payload": map[string]interface{}{
				"merge_commit_sha": event.PullRequest.MergeCommitSHA,
			},
		},
	}

	return pushToSlackList(ctx, rdb, config.SlackRedisList, slackMessage)
}

// handlePRClosed processes closed events where PR was NOT merged (rejected)
func handlePRClosed(ctx context.Context, event PullRequestEvent, rdb *redis.Client, slackClient *slack.Client, config Config) error {
	logger.Info("Processing closed (rejected) event for PR #%d", event.PullRequest.Number)

	// Search for the original review message in Slack
	matchedMessage, err := findMessageByMetadata(ctx, slackClient, config, "pr_url", event.PullRequest.HTMLURL)
	if err != nil {
		return fmt.Errorf("failed to search Slack messages: %w", err)
	}

	if matchedMessage == nil {
		logger.Warn("No matching Slack message found for PR URL: %s", event.PullRequest.HTMLURL)
		return nil
	}

	logger.Debug("Found matching message with ts: %s", matchedMessage.TS)

	// Add ❌ emoji reaction to the message
	reaction := SlackReaction{
		Reaction: "x",
		Channel:  config.SlackChannelID,
		TS:       matchedMessage.TS,
	}

	// Marshal and push to slack_reactions list
	reactionJSON, err := json.Marshal(reaction)
	if err != nil {
		return fmt.Errorf("failed to marshal reaction: %w", err)
	}

	if err := rdb.RPush(ctx, config.SlackReactionsList, reactionJSON).Err(); err != nil {
		return fmt.Errorf("failed to push reaction to Redis list: %w", err)
	}

	logger.Info("Successfully pushed ❌ reaction to Redis list '%s' for ts: %s", config.SlackReactionsList, matchedMessage.TS)

	// Schedule the parent message for deletion after 1 hour (3600 seconds)
	timeBombMessage := TimeBombMessage{
		Channel: config.SlackChannelID,
		TS:      matchedMessage.TS,
		TTL:     3600, // 1 hour
	}

	timeBombJSON, err := json.Marshal(timeBombMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal timebomb message: %w", err)
	}

	if err := rdb.Publish(ctx, config.TimeBombChannel, timeBombJSON).Err(); err != nil {
		logger.Error("Failed to publish timebomb message to Redis channel '%s': %v", config.TimeBombChannel, err)
		return fmt.Errorf("failed to publish timebomb message to Redis: %w", err)
	}

	logger.Info("Successfully scheduled message deletion for ts: %s (TTL: 3600s)", matchedMessage.TS)
	return nil
}

// shouldNotifyDraftPR determines if a draft PR should trigger a notification
// based on the configured repository and branch prefix filters
func shouldNotifyDraftPR(event PullRequestEvent, filter DraftPRFilterConfig) bool {
	// If no filters are configured, don't notify any draft PRs
	if len(filter.EnabledRepoNames) == 0 || len(filter.AllowedBranchStarts) == 0 {
		return false
	}
	
	repoFullName := event.PullRequest.Base.Repo.FullName
	branchName := event.PullRequest.Head.Ref
	
	// Check if repository matches
	repoMatches := false
	for _, allowedRepo := range filter.EnabledRepoNames {
		if allowedRepo == repoFullName {
			repoMatches = true
			break
		}
	}
	
	if !repoMatches {
		return false
	}
	
	// Check if branch prefix matches
	for _, allowedPrefix := range filter.AllowedBranchStarts {
		if strings.HasPrefix(branchName, allowedPrefix) {
			logger.Info("Draft PR #%d matches filter: repo=%s, branch=%s (prefix=%s)", 
				event.PullRequest.Number, repoFullName, branchName, allowedPrefix)
			return true
		}
	}
	
	return false
}

// shouldBlacklistPR determines if a PR should be blacklisted based on branch name patterns
func shouldBlacklistPR(event PullRequestEvent, blacklistPatterns []*regexp.Regexp) bool {
	// If no patterns configured, don't blacklist anything
	if len(blacklistPatterns) == 0 {
		return false
	}
	
	branchName := event.PullRequest.Head.Ref
	
	// Check if branch matches any blacklist pattern
	for _, pattern := range blacklistPatterns {
		if pattern.MatchString(branchName) {
			logger.Info("PR #%d blacklisted: branch '%s' matches pattern '%s'", 
				event.PullRequest.Number, branchName, pattern.String())
			return true
		}
	}
	
	return false
}

// handlePoppitCommandOutput processes poppit command output events
func handlePoppitCommandOutput(ctx context.Context, payload string, rdb *redis.Client, slackClient *slack.Client, config Config) error {
	var event PoppitCommandOutput
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return fmt.Errorf("failed to unmarshal poppit event: %w", err)
	}

	// Only process github-dispatcher type events with specific command
	if event.Type != "github-dispatcher" {
		logger.Debug("Ignoring poppit event with type: %s", event.Type)
		return nil
	}

	if event.Command != "docker compose up -d" {
		logger.Debug("Ignoring poppit command: %s", event.Command)
		return nil
	}

	// Extract git_commit_sha from metadata
	if event.Metadata == nil {
		logger.Debug("Poppit event has no metadata")
		return nil
	}

	gitCommitSHA, ok := event.Metadata["git_commit_sha"].(string)
	if !ok || gitCommitSHA == "" {
		logger.Debug("Poppit event missing git_commit_sha in metadata")
		return nil
	}

	logger.Info("Processing poppit command output for commit: %s", gitCommitSHA)

	// Search for message with matching merge_commit_sha
	matchedMessage, err := findMessageByMergeCommitSHA(ctx, slackClient, config, gitCommitSHA)
	if err != nil {
		return fmt.Errorf("failed to search Slack messages: %w", err)
	}

	if matchedMessage == nil {
		logger.Warn("No matching Slack message found for commit SHA: %s", gitCommitSHA)
		return nil
	}

	logger.Debug("Found matching parent message with ts: %s", matchedMessage.TS)

	// Create reaction for the parent message
	reaction := SlackReaction{
		Reaction: "package",
		Channel:  config.SlackChannelID,
		TS:       matchedMessage.TS,
	}

	// Marshal and push to slack_reactions list
	reactionJSON, err := json.Marshal(reaction)
	if err != nil {
		return fmt.Errorf("failed to marshal reaction: %w", err)
	}

	if err := rdb.RPush(ctx, config.SlackReactionsList, reactionJSON).Err(); err != nil {
		return fmt.Errorf("failed to push reaction to Redis list: %w", err)
	}

	logger.Info("Successfully pushed reaction to Redis list '%s' for ts: %s", config.SlackReactionsList, matchedMessage.TS)
	return nil
}
