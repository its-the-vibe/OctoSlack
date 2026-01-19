package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/redis/go-redis/v9"
	"github.com/slack-go/slack"
)

// LogLevel represents the logging level
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

// Logger holds the current log level
type Logger struct {
	level LogLevel
}

var logger *Logger

// initLogger initializes the global logger with the configured log level
func initLogger(levelStr string) {
	level := INFO // default
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		level = DEBUG
	case "INFO":
		level = INFO
	case "WARN":
		level = WARN
	case "ERROR":
		level = ERROR
	}
	logger = &Logger{level: level}
}

// Debug logs debug messages
func (l *Logger) Debug(format string, v ...interface{}) {
	if l.level <= DEBUG {
		log.Printf("[DEBUG] "+format, v...)
	}
}

// Info logs informational messages
func (l *Logger) Info(format string, v ...interface{}) {
	if l.level <= INFO {
		log.Printf("[INFO] "+format, v...)
	}
}

// Warn logs warning messages
func (l *Logger) Warn(format string, v ...interface{}) {
	if l.level <= WARN {
		log.Printf("[WARN] "+format, v...)
	}
}

// Error logs error messages
func (l *Logger) Error(format string, v ...interface{}) {
	if l.level <= ERROR {
		log.Printf("[ERROR] "+format, v...)
	}
}

// Fatal logs fatal messages and exits
func (l *Logger) Fatal(format string, v ...interface{}) {
	log.Fatalf("[FATAL] "+format, v...)
}

// Config holds the application configuration
type Config struct {
	RedisHost          string
	RedisPort          string
	RedisChannel       string
	RedisPassword      string
	SlackRedisList     string
	SlackChannelID     string
	PoppitChannel      string
	SlackReactionsList string
	SlackSearchLimit   int
	SlackBotToken      string
}

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

func main() {
	// Initialize logger first
	initLogger(getEnv("LOG_LEVEL", "INFO"))

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
		logger.Fatal("Failed to connect to Redis: %v", err)
	}
	logger.Info("Connected to Redis successfully")

	// Create Slack client
	slackClient := slack.New(config.SlackBotToken)
	logger.Info("Slack client initialized")

	// Subscribe to Redis channels
	pubsub := rdb.Subscribe(ctx, config.RedisChannel, config.PoppitChannel)
	defer pubsub.Close()

	logger.Info("Subscribed to Redis channels: %s, %s", config.RedisChannel, config.PoppitChannel)
	logger.Info("Waiting for pull request notifications and command output...")

	// Channel for receiving messages
	ch := pubsub.Channel()

	// Main loop
	for {
		select {
		case msg := <-ch:
			if msg == nil {
				logger.Debug("Received nil message from channel")
				continue
			}
			if msg.Channel == config.RedisChannel {
				if err := handlePullRequestEvent(ctx, msg.Payload, rdb, slackClient, config); err != nil {
					logger.Warn("Error handling pull request event: %v", err)
				}
			} else if msg.Channel == config.PoppitChannel {
				if err := handlePoppitCommandOutput(ctx, msg.Payload, rdb, slackClient, config); err != nil {
					logger.Warn("Error handling poppit command output: %v", err)
				}
			}
		case <-sigChan:
			logger.Info("Shutting down gracefully...")
			return
		}
	}
}

func loadConfig() Config {
	config := Config{
		RedisHost:          getEnv("REDIS_HOST", "localhost"),
		RedisPort:          getEnv("REDIS_PORT", "6379"),
		RedisChannel:       getEnv("REDIS_CHANNEL", "github-events"),
		RedisPassword:      getEnv("REDIS_PASSWORD", ""),
		SlackRedisList:     getEnv("SLACK_REDIS_LIST", "slack_messages"),
		SlackChannelID:     getEnv("SLACK_CHANNEL_ID", ""),
		PoppitChannel:      getEnv("POPPIT_CHANNEL", "poppit:command-output"),
		SlackReactionsList: getEnv("SLACK_REACTIONS_LIST", "slack_reactions"),
		SlackSearchLimit:   getEnvInt("SLACK_SEARCH_LIMIT", 100),
		SlackBotToken:      getEnv("SLACK_BOT_TOKEN", ""),
	}

	if config.SlackChannelID == "" {
		logger.Fatal("SLACK_CHANNEL_ID environment variable is required")
	}

	if config.SlackBotToken == "" {
		logger.Fatal("SLACK_BOT_TOKEN environment variable is required")
	}

	logger.Info("Configuration loaded: Redis=%s:%s, Channel=%s, SlackList=%s",
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

func handlePullRequestEvent(ctx context.Context, payload string, rdb *redis.Client, slackClient *slack.Client, config Config) error {
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
		return handlePRMerged(ctx, event, rdb, slackClient, config)
	}

	logger.Debug("Ignoring event with action: %s (merged: %v)", event.Action, event.PullRequest.Merged)
	return nil
}

func handleReviewRequested(ctx context.Context, event PullRequestEvent, rdb *redis.Client, config Config) error {
	logger.Info("Processing review_requested event for PR #%d", event.PullRequest.Number)

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

// handlePoppitCommandOutput processes poppit command output events
func handlePoppitCommandOutput(ctx context.Context, payload string, rdb *redis.Client, slackClient *slack.Client, config Config) error {
	var event PoppitCommandOutput
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return fmt.Errorf("failed to unmarshal poppit event: %w", err)
	}

	// Only process git-webhook type events with specific command
	if event.Type != "git-webhook" {
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
