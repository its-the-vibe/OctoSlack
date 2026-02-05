package main

import (
	"os"
	"strconv"
	"strings"
)

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
	TimeBombChannel    string
	DraftPRFilter      DraftPRFilterConfig
}

// DraftPRFilterConfig controls which draft PRs should send notifications
type DraftPRFilterConfig struct {
	EnabledRepoNames     []string
	AllowedBranchStarts  []string
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
		TimeBombChannel:    getEnv("TIMEBOMB_CHANNEL", "timebomb-messages"),
		DraftPRFilter:      buildDraftFilterConfig(),
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

func buildDraftFilterConfig() DraftPRFilterConfig {
	reposCSV := getEnv("DRAFT_NOTIFY_REPOS", "")
	prefixesCSV := getEnv("DRAFT_NOTIFY_BRANCH_PREFIXES", "")
	
	return DraftPRFilterConfig{
		EnabledRepoNames:    splitAndTrim(reposCSV),
		AllowedBranchStarts: splitAndTrim(prefixesCSV),
	}
}

func splitAndTrim(csvInput string) []string {
	if csvInput == "" {
		return []string{}
	}
	
	parts := strings.Split(csvInput, ",")
	result := make([]string, 0, len(parts))
	
	for _, item := range parts {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	
	return result
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
