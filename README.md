# OctoSlack
A simple service that subscribes to a redis channel, receives github pull request notifications and posts a message to a slack channel

## Features

- Subscribes to Redis PubSub channel for GitHub events
- Listens for `pull_request.ready_for_review` events
- Posts formatted notifications to Slack via Slack App API
- Configurable via environment variables
- Minimal Docker image (6.87MB) using scratch runtime

## Configuration

The service is configured via environment variables:

- `REDIS_HOST` - Redis server hostname (default: `localhost`)
- `REDIS_PORT` - Redis server port (default: `6379`)
- `REDIS_CHANNEL` - Redis channel name to subscribe to (default: `github-events`)
- `SLACK_BOT_TOKEN` - Slack Bot User OAuth Token (required, starts with `xoxb-`)
- `SLACK_CHANNEL_ID` - Slack channel ID to post messages to (required, e.g., `C0123456789`)

### Setting up a Slack App

1. Go to [api.slack.com/apps](https://api.slack.com/apps) and create a new app
2. Under "OAuth & Permissions", add the following bot token scopes:
   - `chat:write` - to post messages
   - `chat:write.public` - to post to public channels without joining
3. Install the app to your workspace
4. Copy the "Bot User OAuth Token" (starts with `xoxb-`)
5. Get your channel ID by right-clicking the channel in Slack → View channel details

## Usage

### Using Docker Compose

1. Copy `.env.example` to `.env` and configure your settings:

```bash
cp .env.example .env
```

2. Edit `.env` and set your Slack bot token and channel ID:

```
SLACK_BOT_TOKEN=xoxb-your-bot-token-here
SLACK_CHANNEL_ID=C0123456789
```

3. Start the service:

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
  -e SLACK_BOT_TOKEN=xoxb-your-bot-token-here \
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
export SLACK_BOT_TOKEN=xoxb-your-bot-token-here
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
  "action": "ready_for_review",
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

## Testing

To test the service, publish a test event to Redis:

```bash
redis-cli PUBLISH github-events '{"action":"ready_for_review","pull_request":{"number":123,"title":"Test PR","html_url":"https://github.com/owner/repo/pull/123","user":{"login":"testuser"},"head":{"ref":"test-branch"},"base":{"repo":{"full_name":"owner/repo"}}}}'
```

## Architecture

- Written in Go 1.24
- Uses [go-redis/v9](https://github.com/redis/go-redis) for Redis connectivity
- Uses [slack-go/slack](https://github.com/slack-go/slack) for Slack API integration
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

