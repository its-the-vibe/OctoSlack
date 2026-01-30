# OctoSlack
A simple service that subscribes to a redis channel, receives github pull request notifications and posts them to a Redis list for SlackLiner to deliver to Slack

## Features

- Subscribes to Redis PubSub channels for GitHub events and poppit command output
- Listens for `pull_request.review_requested` events and posts notifications to Slack
- Listens for `pull_request.opened` events (non-draft PRs only) and posts notifications to Slack
- Listens for `pull_request.closed` events (when merged) and posts thread replies
- Listens for `pull_request.closed` events (when NOT merged/rejected) and adds ❌ reaction, then schedules message deletion after 1 hour
- Listens for poppit command output and adds emoji reactions on deployment completion
- Uses Slack SDK to search for messages directly via Slack API
- Posts formatted notifications to Redis list for SlackLiner processing
- Includes metadata (PR number, repository, URL, merge commit SHA) for automation
- Configurable via environment variables
- Minimal Docker image (6.87MB) using scratch runtime

## Architecture

This service works in conjunction with [SlackLiner](https://github.com/its-the-vibe/SlackLiner), which reads messages from a Redis list and posts them to Slack. OctoSlack transforms GitHub events into Slack-formatted messages and queues them for SlackLiner to deliver.

### Event Flow

1. **Review Requested**: When a PR review is requested, OctoSlack posts a notification to Slack with metadata
2. **PR Opened (Non-Draft)**: When a non-draft PR is opened, OctoSlack posts a notification to Slack with metadata
3. **PR Merged**: When a PR is closed and merged, OctoSlack searches for the original notification and replies in a thread
4. **PR Closed (Rejected)**: When a PR is closed without merging, OctoSlack searches for the original notification, adds a ❌ emoji reaction, and schedules the message for deletion after 1 hour using TimeBomb
5. **Deployment Complete**: When poppit detects a deployment (via command output), OctoSlack adds a 📦 emoji reaction to the parent message

## Configuration

The service is configured via environment variables:

- `REDIS_HOST` - Redis server hostname (default: `localhost`)
- `REDIS_PORT` - Redis server port (default: `6379`)
- `REDIS_CHANNEL` - Redis channel name to subscribe to (default: `github-events`)
- `REDIS_PASSWORD` - Redis password (default: empty)
- `SLACK_REDIS_LIST` - Redis list key for SlackLiner messages (default: `slack_messages`)
- `SLACK_CHANNEL_ID` - Slack channel ID to post messages to (required, e.g., `C0123456789`)
- `SLACK_BOT_TOKEN` - Slack bot token for API access (required, e.g., `xoxb-...`)
- `POPPIT_CHANNEL` - Redis channel for poppit command output (default: `poppit:command-output`)
- `SLACK_REACTIONS_LIST` - Redis list key for Slack reactions (default: `slack_reactions`)
- `TIMEBOMB_CHANNEL` - Redis channel for TimeBomb message deletion (default: `timebomb-messages`)
- `SLACK_SEARCH_LIMIT` - Number of messages to search when looking for matches (default: `100`)
- `LOG_LEVEL` - Logging level: `DEBUG`, `INFO`, `WARN`, or `ERROR` (default: `INFO`)

### Setting up SlackLiner

This service requires [SlackLiner](https://github.com/its-the-vibe/SlackLiner) to be running to deliver messages to Slack. SlackLiner:

1. Reads messages from the Redis list (default: `slack_messages`)
2. Posts them to Slack using the Slack API
3. Requires a Slack Bot Token with appropriate permissions

See the [SlackLiner documentation](https://github.com/its-the-vibe/SlackLiner) for setup instructions.

## Usage

### Using Docker Compose

1. Copy `.env.example` to `.env` and configure your settings:

```bash
cp .env.example .env
```

2. Edit `.env` and set your Slack configuration:

```
SLACK_CHANNEL_ID=C0123456789
SLACK_BOT_TOKEN=xoxb-your-slack-bot-token
```

3. Start the service (along with SlackLiner if needed):

```bash
docker-compose up -d
```

### Using Docker

Build and run directly with Docker:

```bash
# Build the image
docker build -t octoslack .

# Run the container
docker run -d \
  -e REDIS_HOST=host.docker.internal \
  -e REDIS_PORT=6379 \
  -e REDIS_CHANNEL=github-events \
  -e SLACK_REDIS_LIST=slack_messages \
  -e SLACK_CHANNEL_ID=C0123456789 \
  -e SLACK_BOT_TOKEN=xoxb-your-token \
  octoslack
```

### Using Go

Run directly with Go:

```bash
# Set environment variables
export REDIS_HOST=localhost
export REDIS_PORT=6379
export REDIS_CHANNEL=github-events
export SLACK_REDIS_LIST=slack_messages
export SLACK_CHANNEL_ID=C0123456789
export SLACK_BOT_TOKEN=xoxb-your-token

# Run the service
go run main.go
```

Or build and run:

```bash
go build -o octoslack .
./octoslack
```

## Event Formats

### GitHub Pull Request Events

The service expects GitHub pull request events in JSON format on the Redis channel.

#### Review Requested Event

```json
{
  "action": "review_requested",
  "pull_request": {
    "number": 123,
    "title": "Add new feature",
    "html_url": "https://github.com/owner/repo/pull/123",
    "user": {
      "login": "username"
    },
    "head": {
      "ref": "feature-branch"
    },
    "base": {
      "repo": {
        "full_name": "owner/repo"
      }
    }
  }
}
```

#### Opened Event (Non-Draft)

```json
{
  "action": "opened",
  "pull_request": {
    "number": 124,
    "title": "Add new feature",
    "html_url": "https://github.com/owner/repo/pull/124",
    "draft": false,
    "user": {
      "login": "username"
    },
    "head": {
      "ref": "feature-branch"
    },
    "base": {
      "repo": {
        "full_name": "owner/repo"
      }
    }
  }
}
```

**Note**: Draft PRs (`"draft": true`) are ignored and will not trigger notifications.

#### Closed (Merged) Event

```json
{
  "action": "closed",
  "pull_request": {
    "number": 123,
    "html_url": "https://github.com/owner/repo/pull/123",
    "merged": true,
    "merge_commit_sha": "66978703a4cd8d23e8dade6b4104cdfc98582128"
  }
}
```

#### Closed (Rejected/Not Merged) Event

```json
{
  "action": "closed",
  "pull_request": {
    "number": 124,
    "html_url": "https://github.com/owner/repo/pull/124",
    "merged": false,
    "user": {
      "login": "username"
    },
    "base": {
      "repo": {
        "full_name": "owner/repo"
      }
    }
  }
}
```

### Poppit Command Output Events

The service also listens for poppit command output events on the `poppit:command-output` channel:

```json
{
  "type": "git-webhook",
  "command": "docker compose up --build -d",
  "output": "...",
  "metadata": {
    "git_commit_sha": "66978703a4cd8d23e8dade6b4104cdfc98582128"
  }
}
```

## Output Formats

The service publishes different types of messages to Redis lists for SlackLiner processing.

### Review Requested Notification

Pushed to `slack_messages` list:

```json
{
  "channel": "C0123456789",
  "text": "👀 Review Requested for Pull Request!\n\n*Repository:* owner/repo\n...",
  "metadata": {
    "event_type": "review_requested",
    "event_payload": {
      "pr_number": 123,
      "repository": "owner/repo",
      "pr_url": "https://github.com/owner/repo/pull/123",
      "author": "username",
      "branch": "feature-branch"
    }
  }
}
```

### PR Opened Notification

Pushed to `slack_messages` list:

```json
{
  "channel": "C0123456789",
  "text": "🚀 New Pull Request Opened!\n\n*Repository:* owner/repo\n...",
  "metadata": {
    "event_type": "opened",
    "event_payload": {
      "pr_number": 124,
      "repository": "owner/repo",
      "pr_url": "https://github.com/owner/repo/pull/124",
      "author": "username",
      "branch": "feature-branch"
    }
  }
}
```

### PR Merged Thread Reply

Pushed to `slack_messages` list:

```json
{
  "channel": "C0123456789",
  "text": "✅ Pull Request merged! Commit: 6697870",
  "thread_ts": "1234567890.123456",
  "metadata": {
    "event_type": "closed",
    "event_payload": {
      "merge_commit_sha": "66978703a4cd8d23e8dade6b4104cdfc98582128"
    }
  }
}
```

### PR Closed (Rejected) Reaction

Pushed to `slack_reactions` list:

```json
{
  "reaction": "x",
  "channel": "C0123456789",
  "ts": "1234567890.123456"
}
```

Published to `timebomb-messages` channel:

```json
{
  "channel": "C0123456789",
  "ts": "1234567890.123456",
  "ttl": 3600
}
```

### Deployment Reaction

Pushed to `slack_reactions` list:

```json
{
  "reaction": "package",
  "channel": "C0123456789",
  "ts": "1234567890.123456"
}
```

The metadata field follows the [Slack message metadata format](https://api.slack.com/reference/metadata) with `event_type` and `event_payload` for compatibility with SlackLiner and Slack's metadata-driven automations.

## Testing

To test the service, publish test events to Redis:

### Test Review Requested Event

```bash
redis-cli PUBLISH github-events '{"action":"review_requested","pull_request":{"number":123,"title":"Test PR","html_url":"https://github.com/owner/repo/pull/123","user":{"login":"testuser"},"head":{"ref":"test-branch"},"base":{"repo":{"full_name":"owner/repo"}}}}'
```

### Test PR Opened Event (Non-Draft)

```bash
redis-cli PUBLISH github-events '{"action":"opened","pull_request":{"number":124,"title":"Test PR Opened","html_url":"https://github.com/owner/repo/pull/124","draft":false,"user":{"login":"testuser"},"head":{"ref":"test-branch"},"base":{"repo":{"full_name":"owner/repo"}}}}'
```

### Test PR Opened Event (Draft - Should Be Ignored)

```bash
redis-cli PUBLISH github-events '{"action":"opened","pull_request":{"number":125,"title":"Test Draft PR","html_url":"https://github.com/owner/repo/pull/125","draft":true,"user":{"login":"testuser"},"head":{"ref":"test-branch"},"base":{"repo":{"full_name":"owner/repo"}}}}'
```

### Test PR Merged Event

```bash
redis-cli PUBLISH github-events '{"action":"closed","pull_request":{"number":123,"html_url":"https://github.com/owner/repo/pull/123","merged":true,"merge_commit_sha":"66978703a4cd8d23e8dade6b4104cdfc98582128"}}'
```

### Test PR Closed (Rejected) Event

```bash
redis-cli PUBLISH github-events '{"action":"closed","pull_request":{"number":124,"title":"Test Rejected PR","html_url":"https://github.com/owner/repo/pull/124","merged":false,"user":{"login":"testuser"},"head":{"ref":"test-branch"},"base":{"repo":{"full_name":"owner/repo"}}}}'
```

### Test Poppit Command Output Event

```bash
redis-cli PUBLISH poppit:command-output '{"type":"git-webhook","command":"docker compose up --build -d","output":"Service deployed successfully","metadata":{"git_commit_sha":"66978703a4cd8d23e8dade6b4104cdfc98582128"}}'
```

Then check the Redis lists to see the queued messages:

```bash
# Check Slack messages
redis-cli LRANGE slack_messages 0 -1

# Check Slack reactions
redis-cli LRANGE slack_reactions 0 -1
```

## Architecture

- Written in Go 1.24
- Uses [go-redis/v9](https://github.com/redis/go-redis) for Redis connectivity
- Works with [SlackLiner](https://github.com/its-the-vibe/SlackLiner) for Slack message delivery
- Multi-stage Docker build for minimal image size
- Scratch-based runtime container (no OS overhead)
- Graceful shutdown on SIGTERM/SIGINT

## Development

Build locally:

```bash
go build -o octoslack .
```

Build Docker image:

```bash
docker build -t octoslack .
```

Run tests (when available):

```bash
go test ./...
```

