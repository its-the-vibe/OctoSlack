package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/slack-go/slack"
)

func TestHandlePoppitCommandOutput(t *testing.T) {
	initLogger("ERROR")

	tests := []struct {
		name              string
		payload           string
		expectedReaction  string
		expectedAPICalls  int
		expectedListItems int
	}{
		{
			name: "github dispatcher adds package reaction",
			payload: `{
				"type": "github-dispatcher",
				"command": "docker compose up -d",
				"metadata": {"git_commit_sha": "abc123"}
			}`,
			expectedReaction:  "package",
			expectedAPICalls:  2,
			expectedListItems: 1,
		},
		{
			name: "service restart adds ship reaction",
			payload: `{
				"type": "service-restart",
				"command": "docker compose up -d",
				"metadata": {"git_commit_sha": "abc123"}
			}`,
			expectedReaction:  "ship",
			expectedAPICalls:  2,
			expectedListItems: 1,
		},
		{
			name: "unsupported event type is ignored",
			payload: `{
				"type": "other-event",
				"command": "docker compose up -d",
				"metadata": {"git_commit_sha": "abc123"}
			}`,
			expectedAPICalls:  0,
			expectedListItems: 0,
		},
		{
			name: "unsupported command is ignored",
			payload: `{
				"type": "service-restart",
				"command": "docker compose logs",
				"metadata": {"git_commit_sha": "abc123"}
			}`,
			expectedAPICalls:  0,
			expectedListItems: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var apiCalls int

			slackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				apiCalls++
				w.Header().Set("Content-Type", "application/json")

				switch r.URL.Path {
				case "/conversations.history":
					if err := json.NewEncoder(w).Encode(map[string]any{
						"ok": true,
						"messages": []map[string]any{
							{
								"ts": "111.222",
								"metadata": map[string]any{
									"event_type": "review_requested",
									"event_payload": map[string]any{
										"pull_request_number": "77",
									},
								},
							},
						},
					}); err != nil {
						t.Fatalf("failed to encode history response: %v", err)
					}
				case "/conversations.replies":
					if err := json.NewEncoder(w).Encode(map[string]any{
						"ok": true,
						"messages": []map[string]any{
							{
								"ts": "111.222",
							},
							{
								"ts":        "111.333",
								"thread_ts": "111.222",
								"metadata": map[string]any{
									"event_type": "closed",
									"event_payload": map[string]any{
										"merge_commit_sha": "abc123",
									},
								},
							},
						},
					}); err != nil {
						t.Fatalf("failed to encode replies response: %v", err)
					}
				default:
					t.Fatalf("unexpected Slack API path: %s", r.URL.Path)
				}
			}))
			defer slackServer.Close()

			redisServer, err := miniredis.Run()
			if err != nil {
				t.Fatalf("failed to start miniredis: %v", err)
			}
			defer redisServer.Close()

			rdb := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
			defer rdb.Close()

			slackClient := slack.New("test-token", slack.OptionAPIURL(slackServer.URL+"/"))
			config := Config{
				SlackChannelID:     "C123",
				SlackReactionsList: "slack_reactions",
				SlackSearchLimit:   10,
			}

			err = handlePoppitCommandOutput(context.Background(), tt.payload, rdb, slackClient, config)
			if err != nil {
				t.Fatalf("handlePoppitCommandOutput returned error: %v", err)
			}

			if apiCalls != tt.expectedAPICalls {
				t.Fatalf("expected %d Slack API calls, got %d", tt.expectedAPICalls, apiCalls)
			}

			reactions, err := rdb.LRange(context.Background(), config.SlackReactionsList, 0, -1).Result()
			if err != nil {
				t.Fatalf("failed to read reactions list: %v", err)
			}

			if len(reactions) != tt.expectedListItems {
				t.Fatalf("expected %d queued reactions, got %d", tt.expectedListItems, len(reactions))
			}

			if tt.expectedReaction == "" {
				return
			}

			var reaction SlackReaction
			if err := json.Unmarshal([]byte(reactions[0]), &reaction); err != nil {
				t.Fatalf("failed to unmarshal queued reaction: %v", err)
			}

			if reaction.Reaction != tt.expectedReaction {
				t.Fatalf("expected reaction %q, got %q", tt.expectedReaction, reaction.Reaction)
			}
			if reaction.Channel != config.SlackChannelID {
				t.Fatalf("expected reaction channel %q, got %q", config.SlackChannelID, reaction.Channel)
			}
			if reaction.TS != "111.222" {
				t.Fatalf("expected reaction ts %q, got %q", "111.222", reaction.TS)
			}
		})
	}
}
