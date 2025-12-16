# OctoSlack Project Instructions

## Project Overview

OctoSlack is a lightweight Redis-to-Slack bridge service written in Go. It subscribes to a Redis PubSub channel, listens for GitHub pull request events (specifically `pull_request.review_requested` events), and posts formatted notifications to Slack via webhooks.

## Tech Stack

- **Language**: Go 1.24
- **Dependencies**: 
  - `github.com/redis/go-redis/v9` for Redis connectivity
- **Infrastructure**:
  - Redis for event pub/sub
  - Slack webhooks for notifications
  - Docker for containerization with minimal scratch-based images
- **Build**: Multi-stage Docker builds for production deployment

## Architecture & Design

- **Single file application**: All logic is in `main.go` for simplicity
- **Event-driven**: Uses Redis PubSub pattern for loose coupling
- **Stateless service**: No persistent storage, can be scaled horizontally
- **Graceful shutdown**: Handles SIGTERM/SIGINT signals properly
- **Configuration**: Environment variables only, no config files
- **Error handling**: Log errors but continue processing other events

## Build & Run Instructions

### Local Development
```bash
# Install dependencies (vendored)
go mod download
go mod vendor

# Build
go build -o octoslack .

# Run (requires environment variables)
export REDIS_HOST=localhost
export REDIS_PORT=6379
export REDIS_CHANNEL=github-events
export SLACK_WEBHOOK_URL=https://hooks.slack.com/services/YOUR/WEBHOOK/URL
./octoslack
```

### Docker
```bash
# Build
docker build -t octoslack .

# Run
docker-compose up -d
```

## Testing

Currently, the project has no automated tests. When adding tests:
- Use standard Go testing (`go test ./...`)
- Test the JSON parsing of GitHub webhook events
- Test Slack message formatting
- Mock Redis and HTTP clients for unit tests
- Consider table-driven tests for event handling

To manually test, publish a test event to Redis:
```bash
redis-cli PUBLISH github-events '{"action":"review_requested","pull_request":{"number":123,"title":"Test PR","html_url":"https://github.com/owner/repo/pull/123","user":{"login":"testuser"},"head":{"ref":"test-branch"},"base":{"repo":{"full_name":"owner/repo"}}}}'
```

## Code Style & Guidelines

- **Simplicity first**: Keep the codebase simple and readable
- **Standard Go conventions**: Follow Go standard library patterns
- **Error handling**: Always handle errors, log them appropriately
- **Logging**: Use standard `log` package with descriptive messages
- **Configuration**: All config via environment variables using `getEnv()` helper
- **Vendoring**: Use Go modules with vendoring (`go mod vendor`)
- **Comments**: Add comments for exported types and non-obvious logic only

## Environment Variables

Required:
- `SLACK_WEBHOOK_URL`: Slack incoming webhook URL

Optional (with defaults):
- `REDIS_HOST`: Redis hostname (default: `localhost`)
- `REDIS_PORT`: Redis port (default: `6379`)
- `REDIS_CHANNEL`: Redis channel name (default: `github-events`)

## Docker Considerations

- Use multi-stage builds to minimize image size
- Final image uses scratch base (~6.87MB total)
- Build with CGO disabled for static binary
- No shell available in production container

## Event Format

The service expects GitHub PR webhook events matching this structure:
```json
{
  "action": "review_requested",
  "pull_request": {
    "number": 123,
    "title": "PR Title",
    "html_url": "https://github.com/owner/repo/pull/123",
    "user": {"login": "username"},
    "head": {"ref": "branch-name"},
    "base": {"repo": {"full_name": "owner/repo"}}
  }
}
```

## Pull Request Guidelines

- Keep changes minimal and focused
- Ensure Docker builds succeed
- Update README.md if adding features or changing configuration
- Test with actual Redis and Slack webhook if possible
- Consider backward compatibility with event format
