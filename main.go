package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/redis/go-redis/v9"
)

// Config holds the application configuration
type Config struct {
	RedisHost      string
	RedisPort      string
	RedisChannel   string
	RedisPassword  string
	SlackRedisList string
	SlackChannel   string
}

// PullRequestEvent represents a GitHub pull request event
type PullRequestEvent struct {
	Action      string `json:"action"`
	PullRequest struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		HTMLURL string `json:"html_url"`
		User    struct {
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

	// Subscribe to Redis channel
	pubsub := rdb.Subscribe(ctx, config.RedisChannel)
	defer pubsub.Close()

	log.Printf("Subscribed to Redis channel: %s", config.RedisChannel)
	log.Println("Waiting for pull request notifications...")

	// Channel for receiving messages
	ch := pubsub.Channel()

	// Main loop
	for {
		select {
		case msg := <-ch:
			if err := handleMessage(ctx, msg.Payload, rdb, config); err != nil {
				log.Printf("Error handling message: %v", err)
			}
		case <-sigChan:
			log.Println("Shutting down gracefully...")
			return
		}
	}
}

func loadConfig() Config {
	config := Config{
		RedisHost:      getEnv("REDIS_HOST", "localhost"),
		RedisPort:      getEnv("REDIS_PORT", "6379"),
		RedisChannel:   getEnv("REDIS_CHANNEL", "github-events"),
		RedisPassword:  getEnv("REDIS_PASSWORD", ""),
		SlackRedisList: getEnv("SLACK_REDIS_LIST", "slack_messages"),
		SlackChannel:   getEnv("SLACK_CHANNEL", ""),
	}

	if config.SlackChannel == "" {
		log.Fatal("SLACK_CHANNEL environment variable is required")
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

func handleMessage(ctx context.Context, payload string, rdb *redis.Client, config Config) error {
	var event PullRequestEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}

	// Only process review_requested events
	if event.Action != "review_requested" {
		log.Printf("Ignoring event with action: %s", event.Action)
		return nil
	}

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
		Channel: config.SlackChannel,
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
