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
	"github.com/slack-go/slack"
)

// Config holds the application configuration
type Config struct {
	RedisHost      string
	RedisPort      string
	RedisChannel   string
	SlackBotToken  string
	SlackChannelID string
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

// SlackMessage represents a Slack message payload
type SlackMessage struct {
	Text string `json:"text"`
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
		Addr: fmt.Sprintf("%s:%s", config.RedisHost, config.RedisPort),
	})
	defer rdb.Close()

	// Test Redis connection
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("Connected to Redis successfully")

	// Create Slack client
	slackClient := slack.New(config.SlackBotToken)

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
			if err := handleMessage(msg.Payload, slackClient, config.SlackChannelID); err != nil {
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
		SlackBotToken:  getEnv("SLACK_BOT_TOKEN", ""),
		SlackChannelID: getEnv("SLACK_CHANNEL_ID", ""),
	}

	if config.SlackBotToken == "" {
		log.Fatal("SLACK_BOT_TOKEN environment variable is required")
	}

	if config.SlackChannelID == "" {
		log.Fatal("SLACK_CHANNEL_ID environment variable is required")
	}

	log.Printf("Configuration loaded: Redis=%s:%s, Channel=%s",
		config.RedisHost, config.RedisPort, config.RedisChannel)

	return config
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func handleMessage(payload string, slackClient *slack.Client, channelID string) error {
	var event PullRequestEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}

	// Only process ready_for_review events
	if event.Action != "ready_for_review" {
		log.Printf("Ignoring event with action: %s", event.Action)
		return nil
	}

	log.Printf("Processing ready_for_review event for PR #%d", event.PullRequest.Number)

	// Create Slack message
	message := fmt.Sprintf(
		"🎉 Pull Request Ready for Review!\n\n"+
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

	return postToSlack(slackClient, channelID, message)
}

func postToSlack(slackClient *slack.Client, channelID, message string) error {
	_, _, err := slackClient.PostMessage(
		channelID,
		slack.MsgOptionText(message, false),
	)
	if err != nil {
		return fmt.Errorf("failed to post to Slack: %w", err)
	}

	log.Printf("Successfully posted message to Slack")
	return nil
}
