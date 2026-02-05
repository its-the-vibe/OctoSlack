package main

import (
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
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

// YAMLConfig represents the structure of the YAML config file
type YAMLConfig struct {
	Redis struct {
		Host    string `yaml:"host"`
		Port    string `yaml:"port"`
		Channel string `yaml:"channel"`
	} `yaml:"redis"`
	Slack struct {
		ChannelID     string `yaml:"channel_id"`
		RedisList     string `yaml:"redis_list"`
		ReactionsList string `yaml:"reactions_list"`
		SearchLimit   int    `yaml:"search_limit"`
	} `yaml:"slack"`
	Poppit struct {
		Channel string `yaml:"channel"`
	} `yaml:"poppit"`
	TimeBomb struct {
		Channel string `yaml:"channel"`
	} `yaml:"timebomb"`
	Logging struct {
		Level string `yaml:"level"`
	} `yaml:"logging"`
	DraftPRFilter struct {
		EnabledRepos          []string `yaml:"enabled_repos"`
		AllowedBranchPrefixes []string `yaml:"allowed_branch_prefixes"`
	} `yaml:"draft_pr_filter"`
}

func loadConfig() Config {
	// Load defaults from YAML file if it exists
	yamlConfig := loadYAMLConfig("config.yaml")

	// Build config with YAML values as defaults, allow env vars to override
	config := Config{
		RedisHost:          getEnvOrDefault("REDIS_HOST", yamlConfig.Redis.Host, "localhost"),
		RedisPort:          getEnvOrDefault("REDIS_PORT", yamlConfig.Redis.Port, "6379"),
		RedisChannel:       getEnvOrDefault("REDIS_CHANNEL", yamlConfig.Redis.Channel, "github-events"),
		RedisPassword:      getEnv("REDIS_PASSWORD", ""),
		SlackRedisList:     getEnvOrDefault("SLACK_REDIS_LIST", yamlConfig.Slack.RedisList, "slack_messages"),
		SlackChannelID:     getEnvOrDefault("SLACK_CHANNEL_ID", yamlConfig.Slack.ChannelID, ""),
		PoppitChannel:      getEnvOrDefault("POPPIT_CHANNEL", yamlConfig.Poppit.Channel, "poppit:command-output"),
		SlackReactionsList: getEnvOrDefault("SLACK_REACTIONS_LIST", yamlConfig.Slack.ReactionsList, "slack_reactions"),
		SlackSearchLimit:   getEnvIntOrDefault("SLACK_SEARCH_LIMIT", yamlConfig.Slack.SearchLimit, 100),
		SlackBotToken:      getEnv("SLACK_BOT_TOKEN", ""),
		TimeBombChannel:    getEnvOrDefault("TIMEBOMB_CHANNEL", yamlConfig.TimeBomb.Channel, "timebomb-messages"),
		DraftPRFilter:      buildDraftFilterConfigWithYAML(yamlConfig),
	}

	if config.SlackChannelID == "" {
		logger.Fatal("SLACK_CHANNEL_ID must be set in config.yaml or SLACK_CHANNEL_ID environment variable")
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

func buildDraftFilterConfigWithYAML(yamlConfig YAMLConfig) DraftPRFilterConfig {
	// Check for environment variables first (they override YAML)
	reposCSV := os.Getenv("DRAFT_NOTIFY_REPOS")
	prefixesCSV := os.Getenv("DRAFT_NOTIFY_BRANCH_PREFIXES")
	
	// Use env vars if set, otherwise use YAML values
	repos := yamlConfig.DraftPRFilter.EnabledRepos
	if reposCSV != "" {
		repos = splitAndTrim(reposCSV)
	}
	
	prefixes := yamlConfig.DraftPRFilter.AllowedBranchPrefixes
	if prefixesCSV != "" {
		prefixes = splitAndTrim(prefixesCSV)
	}
	
	return DraftPRFilterConfig{
		EnabledRepoNames:    repos,
		AllowedBranchStarts: prefixes,
	}
}

func loadYAMLConfig(filename string) YAMLConfig {
	var yamlConfig YAMLConfig
	
	// Try to read the config file
	data, err := os.ReadFile(filename)
	if err != nil {
		// Config file is optional - just use defaults if it doesn't exist
		// Note: logger may not be initialized yet, so we can't log here
		return yamlConfig
	}
	
	// Parse YAML
	if err := yaml.Unmarshal(data, &yamlConfig); err != nil {
		// Log warning only if logger is initialized
		if logger != nil {
			logger.Warn("Failed to parse config file %s: %v. Using defaults.", filename, err)
		}
		return YAMLConfig{}
	}
	
	// Log success only if logger is initialized
	if logger != nil {
		logger.Info("Loaded configuration from %s", filename)
	}
	return yamlConfig
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

func getEnvOrDefault(key, yamlValue, defaultValue string) string {
	// Environment variable takes precedence
	if value := os.Getenv(key); value != "" {
		return value
	}
	// Use YAML value if available
	if yamlValue != "" {
		return yamlValue
	}
	// Fall back to default
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

func getEnvIntOrDefault(key string, yamlValue int, defaultValue int) int {
	// Environment variable takes precedence
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	// Use YAML value if available
	if yamlValue != 0 {
		return yamlValue
	}
	// Fall back to default
	return defaultValue
}
