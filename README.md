# OctoSlack
A simple service that subscribes to a redis channel, receives github pull request notifications and posts them to a Redis list for SlackLiner to deliver to Slack

## Features

- Subscribes to Redis PubSub channel for GitHub events
- Listens for `pull_request.review_requested` events
- Posts formatted notifications to Redis list for SlackLiner processing
- Includes metadata (PR number, repository, URL) for future automation
- Configurable via environment variables
- Minimal Docker image (6.87MB) using scratch runtime

## Architecture

This service works in conjunction with [SlackLiner](https://github.com/its-the-vibe/SlackLiner), which reads messages from a Redis list and posts them to Slack. OctoSlack transforms GitHub events into Slack-formatted messages and queues them for SlackLiner to deliver.

## Configuration

The service is configured via environment variables:

- `REDIS_HOST` - Redis server hostname (default: `localhost`)
- `REDIS_PORT` - Redis server port (default: `6379`)
- `REDIS_CHANNEL` - Redis channel name to subscribe to (default: `github-events`)
- `SLACK_REDIS_LIST` - Redis list key for SlackLiner messages (default: `slack_messages`)
- `SLACK_CHANNEL_ID` - Slack channel ID to post messages to (required, e.g., `C0123456789`)

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

2. Edit `.env` and set your Slack channel:

```
SLACK_CHANNEL_ID=C0123456789
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

# Run the service
go run main.go
```

Or build and run:

```bash
go build -o octoslack .
./octoslack
```

## Event Format

The service expects GitHub pull request events in JSON format on the Redis channel:

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

## Output Format

The service publishes messages to a Redis list in the format expected by SlackLiner:

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

The metadata field follows the [Slack message metadata format](https://api.slack.com/reference/metadata) with `event_type` and `event_payload` for compatibility with SlackLiner and Slack's metadata-driven automations.

## Testing

To test the service, publish a test event to Redis:

```bash
redis-cli PUBLISH github-events '{"action":"review_requested","pull_request":{"number":123,"title":"Test PR","html_url":"https://github.com/owner/repo/pull/123","user":{"login":"testuser"},"head":{"ref":"test-branch"},"base":{"repo":{"full_name":"owner/repo"}}}}'
```

Then check the Redis list to see the queued message:

```bash
redis-cli LRANGE slack_messages 0 -1
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

