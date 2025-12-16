# OctoSlack
A simple service that subscribes to a redis channel, receives github pull request notifications and posts a message to a slack channel

## Features

- Subscribes to Redis PubSub channel for GitHub events
- Listens for `pull_request.ready_for_review` events
- Posts formatted notifications to Slack via webhook
- Configurable via environment variables
- Minimal Docker image (6.87MB) using scratch runtime

## Configuration

The service is configured via environment variables:

- `REDIS_HOST` - Redis server hostname (default: `localhost`)
- `REDIS_PORT` - Redis server port (default: `6379`)
- `REDIS_CHANNEL` - Redis channel name to subscribe to (default: `github-events`)
- `SLACK_WEBHOOK_URL` - Slack incoming webhook URL (required)

## Usage

### Using Docker Compose

1. Copy `.env.example` to `.env` and configure your settings:

```bash
cp .env.example .env
```

2. Edit `.env` and set your Slack webhook URL:

```
SLACK_WEBHOOK_URL=https://hooks.slack.com/services/YOUR/WEBHOOK/URL
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
  -e SLACK_WEBHOOK_URL=https://hooks.slack.com/services/YOUR/WEBHOOK/URL \
  octoslack
```

### Using Go

Run directly with Go:

```bash
# Set environment variables
export REDIS_HOST=localhost
export REDIS_PORT=6379
export REDIS_CHANNEL=github-events
export SLACK_WEBHOOK_URL=https://hooks.slack.com/services/YOUR/WEBHOOK/URL

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

