package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/redis/go-redis/v9"
	"github.com/slack-go/slack"
)

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
