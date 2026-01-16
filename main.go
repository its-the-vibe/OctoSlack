package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/redis/go-redis/v9"
)

// Config holds the application configuration
type Config struct {
	RedisHost             string
	RedisPort             string
	RedisChannel          string
	RedisPassword         string
	SlackRedisList        string
	SlackChannelID        string
	PoppitChannel         string
	SlackReactionsList    string
	SlackSearchLimit      int
	SlackConversationsAPI string
}

// PullRequestEvent represents a GitHub pull request event
type PullRequestEvent struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
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

// SlackConversationsRequest represents a request to search Slack conversations
type SlackConversationsRequest struct {
	Channel string `json:"channel"`
	Limit   int    `json:"limit"`
}

// SlackConversationsResponse represents the response from SlackLiner conversations API
type SlackConversationsResponse struct {
	Messages []SlackHistoryMessage `json:"messages"`
}

// SlackHistoryMessage represents a message from Slack history
type SlackHistoryMessage struct {
	TS       string                 `json:"ts"`
	ThreadTS string                 `json:"thread_ts,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// PoppitCommandOutput represents a poppit command output event
type PoppitCommandOutput struct {
	Type     string                 `json:"type"`
	Command  string                 `json:"command"`
	Output   string                 `json:"output"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

func main() {
	config := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", config.RedisHost, config.RedisPort),
		Password: config.RedisPassword,
	})
	defer rdb.Close()

	// Test Redis connection
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("Connected to Redis successfully")

	// Subscribe to Redis channels
	pubsub := rdb.Subscribe(ctx, config.RedisChannel, config.PoppitChannel)
	defer pubsub.Close()

	log.Printf("Subscribed to Redis channels: %s, %s", config.RedisChannel, config.PoppitChannel)
	log.Println("Waiting for pull request notifications and command output...")

	// Channel for receiving messages
	ch := pubsub.Channel()

	// Main loop
	for {
		select {
		case msg := <-ch:
			if msg.Channel == config.RedisChannel {
				if err := handlePullRequestEvent(ctx, msg.Payload, rdb, config); err != nil {
					log.Printf("Error handling pull request event: %v", err)
				}
			} else if msg.Channel == config.PoppitChannel {
				if err := handlePoppitCommandOutput(ctx, msg.Payload, rdb, config); err != nil {
					log.Printf("Error handling poppit command output: %v", err)
				}
			}
		case <-sigChan:
			log.Println("Shutting down gracefully...")
			return
		}
	}
}

func loadConfig() Config {
	config := Config{
		RedisHost:             getEnv("REDIS_HOST", "localhost"),
		RedisPort:             getEnv("REDIS_PORT", "6379"),
		RedisChannel:          getEnv("REDIS_CHANNEL", "github-events"),
		RedisPassword:         getEnv("REDIS_PASSWORD", ""),
		SlackRedisList:        getEnv("SLACK_REDIS_LIST", "slack_messages"),
		SlackChannelID:        getEnv("SLACK_CHANNEL_ID", ""),
		PoppitChannel:         getEnv("POPPIT_CHANNEL", "poppit:command-output"),
		SlackReactionsList:    getEnv("SLACK_REACTIONS_LIST", "slack_reactions"),
		SlackSearchLimit:      getEnvInt("SLACK_SEARCH_LIMIT", 100),
		SlackConversationsAPI: getEnv("SLACK_CONVERSATIONS_API", "slack_conversations"),
	}

	if config.SlackChannelID == "" {
		log.Fatal("SLACK_CHANNEL_ID environment variable is required")
	}

	log.Printf("Configuration loaded: Redis=%s:%s, Channel=%s, SlackList=%s",
		config.RedisHost, config.RedisPort, config.RedisChannel, config.SlackRedisList)

	return config
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func handlePullRequestEvent(ctx context.Context, payload string, rdb *redis.Client, config Config) error {
	var event PullRequestEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}

	// Process review_requested events
	if event.Action == "review_requested" {
		return handleReviewRequested(ctx, event, rdb, config)
	}

	// Process closed events where PR was merged
	if event.Action == "closed" && event.PullRequest.Merged {
		return handlePRMerged(ctx, event, rdb, config)
	}

	log.Printf("Ignoring event with action: %s (merged: %v)", event.Action, event.PullRequest.Merged)
	return nil
}

func handleReviewRequested(ctx context.Context, event PullRequestEvent, rdb *redis.Client, config Config) error {
	log.Printf("Processing review_requested event for PR #%d", event.PullRequest.Number)

	// Create Slack message text
	messageText := fmt.Sprintf(
		"👀 Review Requested for Pull Request!\n\n"+
			"*Repository:* %s\n"+
			"*PR #%d:* %s\n"+
			"*Author:* %s\n"+
			"*Branch:* %s\n"+
			"*Link:* <%s|View PR>",
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

func handlePRMerged(ctx context.Context, event PullRequestEvent, rdb *redis.Client, config Config) error {
	log.Printf("Processing closed (merged) event for PR #%d with merge commit %s", 
		event.PullRequest.Number, event.PullRequest.MergeCommitSHA)

	// Search for the original review message in Slack
	matchedMessage, err := findMessageByMetadata(ctx, rdb, config, "pr_url", event.PullRequest.HTMLURL)
	if err != nil {
		return fmt.Errorf("failed to search Slack messages: %w", err)
	}

	if matchedMessage == nil {
		log.Printf("No matching Slack message found for PR URL: %s", event.PullRequest.HTMLURL)
		return nil
	}

	log.Printf("Found matching message with ts: %s", matchedMessage.TS)

	// Reply to the message in a thread
	replyText := fmt.Sprintf("✅ Pull Request merged! Commit: %s", event.PullRequest.MergeCommitSHA[:7])

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

	log.Printf("Successfully pushed message to Redis list '%s'", listKey)
	return nil
}

// findMessageByMetadata searches for a message in Slack channel by metadata field
func findMessageByMetadata(ctx context.Context, rdb *redis.Client, config Config, metadataKey string, metadataValue string) (*SlackHistoryMessage, error) {
	// Request conversation history from SlackLiner
	request := SlackConversationsRequest{
		Channel: config.SlackChannelID,
		Limit:   config.SlackSearchLimit,
	}

	requestJSON, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Push request to slack_conversations list and wait for response
	if err := rdb.RPush(ctx, config.SlackConversationsAPI, requestJSON).Err(); err != nil {
		return nil, fmt.Errorf("failed to push request to Redis: %w", err)
	}

	// For now, we'll retrieve messages from a response list
	// This is a simplified implementation - in production, you might use a request/response pattern
	responseKey := fmt.Sprintf("%s:response", config.SlackConversationsAPI)
	
	// Get the response (blocking pop with timeout)
	result, err := rdb.BLPop(ctx, 5*1000000000, responseKey).Result() // 5 second timeout
	if err != nil {
		// If no response, log and return nil (not found)
		log.Printf("No response from SlackLiner conversations API (timeout or no data)")
		return nil, nil
	}

	if len(result) < 2 {
		return nil, fmt.Errorf("invalid response format from Redis")
	}

	var response SlackConversationsResponse
	if err := json.Unmarshal([]byte(result[1]), &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Search through messages for matching metadata
	for _, msg := range response.Messages {
		if msg.Metadata != nil {
			if eventPayload, ok := msg.Metadata["event_payload"].(map[string]interface{}); ok {
				if value, ok := eventPayload[metadataKey].(string); ok && value == metadataValue {
					return &msg, nil
				}
			}
		}
	}

	return nil, nil
}

// findMessageByMergeCommitSHA searches for a message in Slack by merge_commit_sha in metadata
func findMessageByMergeCommitSHA(ctx context.Context, rdb *redis.Client, config Config, mergeCommitSHA string) (*SlackHistoryMessage, error) {
	// Similar to findMessageByMetadata but searches for merge_commit_sha
	request := SlackConversationsRequest{
		Channel: config.SlackChannelID,
		Limit:   config.SlackSearchLimit,
	}

	requestJSON, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if err := rdb.RPush(ctx, config.SlackConversationsAPI, requestJSON).Err(); err != nil {
		return nil, fmt.Errorf("failed to push request to Redis: %w", err)
	}

	responseKey := fmt.Sprintf("%s:response", config.SlackConversationsAPI)
	result, err := rdb.BLPop(ctx, 5*1000000000, responseKey).Result()
	if err != nil {
		log.Printf("No response from SlackLiner conversations API (timeout or no data)")
		return nil, nil
	}

	if len(result) < 2 {
		return nil, fmt.Errorf("invalid response format from Redis")
	}

	var response SlackConversationsResponse
	if err := json.Unmarshal([]byte(result[1]), &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Search through messages for matching merge_commit_sha
	for _, msg := range response.Messages {
		if msg.Metadata != nil {
			if eventPayload, ok := msg.Metadata["event_payload"].(map[string]interface{}); ok {
				if sha, ok := eventPayload["merge_commit_sha"].(string); ok && sha == mergeCommitSHA {
					return &msg, nil
				}
			}
		}
	}

	return nil, nil
}

// handlePoppitCommandOutput processes poppit command output events
func handlePoppitCommandOutput(ctx context.Context, payload string, rdb *redis.Client, config Config) error {
	var event PoppitCommandOutput
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return fmt.Errorf("failed to unmarshal poppit event: %w", err)
	}

	// Only process git-webhook type events with specific command
	if event.Type != "git-webhook" {
		log.Printf("Ignoring poppit event with type: %s", event.Type)
		return nil
	}

	if event.Command != "docker compose up --build -d" {
		log.Printf("Ignoring poppit command: %s", event.Command)
		return nil
	}

	// Extract git_commit_sha from metadata
	if event.Metadata == nil {
		log.Printf("Poppit event has no metadata")
		return nil
	}

	gitCommitSHA, ok := event.Metadata["git_commit_sha"].(string)
	if !ok || gitCommitSHA == "" {
		log.Printf("Poppit event missing git_commit_sha in metadata")
		return nil
	}

	log.Printf("Processing poppit command output for commit: %s", gitCommitSHA)

	// Search for message with matching merge_commit_sha
	matchedMessage, err := findMessageByMergeCommitSHA(ctx, rdb, config, gitCommitSHA)
	if err != nil {
		return fmt.Errorf("failed to search Slack messages: %w", err)
	}

	if matchedMessage == nil {
		log.Printf("No matching Slack message found for commit SHA: %s", gitCommitSHA)
		return nil
	}

	log.Printf("Found matching message with ts: %s, thread_ts: %s", matchedMessage.TS, matchedMessage.ThreadTS)

	// Determine the parent message timestamp
	// If the message is in a thread, thread_ts points to the parent
	parentTS := matchedMessage.ThreadTS
	if parentTS == "" {
		// If thread_ts is empty, this is the parent message
		parentTS = matchedMessage.TS
	}

	// Create reaction
	reaction := SlackReaction{
		Reaction: "package",
		Channel:  config.SlackChannelID,
		TS:       parentTS,
	}

	// Marshal and push to slack_reactions list
	reactionJSON, err := json.Marshal(reaction)
	if err != nil {
		return fmt.Errorf("failed to marshal reaction: %w", err)
	}

	if err := rdb.RPush(ctx, config.SlackReactionsList, reactionJSON).Err(); err != nil {
		return fmt.Errorf("failed to push reaction to Redis list: %w", err)
	}

	log.Printf("Successfully pushed reaction to Redis list '%s' for ts: %s", config.SlackReactionsList, parentTS)
	return nil
}
