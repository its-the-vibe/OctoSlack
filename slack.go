package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/slack-go/slack"
)

func pushToSlackList(ctx context.Context, rdb *redis.Client, listKey string, message SlackMessage) error {
	// Marshal the message to JSON
	messageJSON, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Push message to Redis list
	if err := rdb.RPush(ctx, listKey, messageJSON).Err(); err != nil {
		return fmt.Errorf("failed to push message to Redis list: %w", err)
	}

	logger.Info("Successfully pushed message to Redis list '%s'", listKey)
	return nil
}

// findMessageByMetadata searches for a message in Slack channel by metadata field
func findMessageByMetadata(ctx context.Context, slackClient *slack.Client, config Config, metadataKey string, metadataValue string) (*SlackHistoryMessage, error) {
	// Use Slack SDK to fetch conversation history
	historyParams := &slack.GetConversationHistoryParameters{
		ChannelID:          config.SlackChannelID,
		Limit:              config.SlackSearchLimit,
		IncludeAllMetadata: true,
	}

	history, err := slackClient.GetConversationHistoryContext(ctx, historyParams)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation history: %w", err)
	}

	// Search through messages for matching metadata
	for _, msg := range history.Messages {
		// Check if metadata exists and has the event type
		if msg.Msg.Metadata.EventType != "" && msg.Msg.Metadata.EventPayload != nil {
			// Check if the metadata field matches
			if value, ok := msg.Msg.Metadata.EventPayload[metadataKey].(string); ok && value == metadataValue {
				return &SlackHistoryMessage{
					TS:       msg.Msg.Timestamp,
					ThreadTS: msg.Msg.ThreadTimestamp,
					Metadata: &msg.Msg.Metadata,
				}, nil
			}
		}
	}

	return nil, nil
}

// findMessageByMergeCommitSHA searches for a message in Slack by merge_commit_sha in thread replies
// It searches for messages with event_type "review_requested", then searches their replies for
// event_type "closed" with the matching merge_commit_sha
func findMessageByMergeCommitSHA(ctx context.Context, slackClient *slack.Client, config Config, mergeCommitSHA string) (*SlackHistoryMessage, error) {
	// First, search for messages with event_type "review_requested"
	historyParams := &slack.GetConversationHistoryParameters{
		ChannelID:          config.SlackChannelID,
		Limit:              config.SlackSearchLimit,
		IncludeAllMetadata: true,
	}

	history, err := slackClient.GetConversationHistoryContext(ctx, historyParams)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation history: %w", err)
	}

	// Search through messages for those with event_type "review_requested"
	for _, msg := range history.Messages {
		if msg.Msg.Metadata.EventType != "review_requested" {
			continue
		}

		// For each review_requested message, search its thread replies
		// Note: We use SlackSearchLimit and don't paginate for simplicity per issue requirements
		repliesParams := &slack.GetConversationRepliesParameters{
			ChannelID:          config.SlackChannelID,
			Timestamp:          msg.Msg.Timestamp,
			Limit:              config.SlackSearchLimit,
			IncludeAllMetadata: true,
		}

		replies, _, _, err := slackClient.GetConversationRepliesContext(ctx, repliesParams)
		if err != nil {
			logger.Warn("Failed to get replies for message %s: %v", msg.Msg.Timestamp, err)
			continue
		}

		// Search through replies for event_type "closed" with matching merge_commit_sha
		for _, reply := range replies {
			if reply.Msg.Metadata.EventType != "closed" {
				continue
			}

			if reply.Msg.Metadata.EventPayload == nil {
				continue
			}

			// Check if merge_commit_sha matches
			if sha, ok := reply.Msg.Metadata.EventPayload["merge_commit_sha"].(string); ok && sha == mergeCommitSHA {
				// Return the parent message (not the reply)
				return &SlackHistoryMessage{
					TS:       msg.Msg.Timestamp,
					ThreadTS: msg.Msg.ThreadTimestamp,
					Metadata: &msg.Msg.Metadata,
				}, nil
			}
		}
	}

	return nil, nil
}
