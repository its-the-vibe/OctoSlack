# OctoSlack
A simple service that subscribes to a redis channel, receives github pull request notifications and posts them to a Redis list for SlackLiner to deliver to Slack

## Features

- Subscribes to Redis PubSub channels for GitHub events and poppit command output
- Listens for `pull_request.review_requested` events and posts notifications to Slack
- Listens for `pull_request.opened` events (non-draft PRs only) and posts notifications to Slack
- Supports selective notifications for draft PRs via configurable repository and branch prefix filters
- Supports blacklisting PRs based on branch name regex patterns (e.g., exclude dependabot rc versions)
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

The service can be configured via a combination of a YAML configuration file and environment variables:

- **Non-sensitive configuration** is stored in `config.yaml` (see `config.example.yaml` for template)
- **Sensitive credentials** (tokens, passwords) must be provided via environment variables
- Environment variables override values from the config file

### Configuration File

Create a `config.yaml` file from the example:

```bash
cp config.example.yaml config.yaml
```

Edit `config.yaml` to set your non-sensitive configuration. The config file supports:

- `redis.host` - Redis server hostname (default: `localhost`)
- `redis.port` - Redis server port (default: `6379`)
- `redis.channel` - Redis channel name to subscribe to (default: `github-events`)
- `slack.channel_id` - Slack channel ID to post messages to (required, e.g., `C0123456789`)
- `slack.redis_list` - Redis list key for SlackLiner messages (default: `slack_messages`)
- `slack.reactions_list` - Redis list key for Slack reactions (default: `slack_reactions`)
- `slack.search_limit` - Number of messages to search when looking for matches (default: `100`)
- `poppit.channel` - Redis channel for poppit command output (default: `poppit:command-output`)
- `timebomb.channel` - Redis channel for TimeBomb message deletion (default: `timebomb-messages`)
- `logging.level` - Logging level: `DEBUG`, `INFO`, `WARN`, or `ERROR` (default: `INFO`)
- `draft_pr_filter.enabled_repos` - List of repositories where draft PR notifications are enabled (default: empty)
- `draft_pr_filter.allowed_branch_prefixes` - List of branch prefixes that trigger draft PR notifications (default: empty)
- `branch_blacklist.patterns` - List of regex patterns for branch names to blacklist from notifications (default: empty)

### Branch Blacklist

The `branch_blacklist` configuration allows you to exclude PRs from specific branches using regex patterns. This is particularly useful for:
- Excluding Dependabot PRs with rc (release candidate) versions
- Filtering out automated PRs based on naming patterns
- Preventing notifications for specific types of branches

Example patterns in `config.yaml`:

```yaml
branch_blacklist:
  patterns:
    - "^dependabot/docker/golang-\\d+\\.\\d+rc\\d+-alpine$"  # Exclude Go rc versions
    - "dependabot/npm/.*-rc\\..*"                             # Exclude npm rc versions
    - "^renovate/.*-beta"                                     # Exclude Renovate beta updates
```

**Regex Escaping Rules:**
- In YAML files, use `\\` to escape special regex characters
- `\\.` matches a literal dot character (e.g., version 1.26)
- `.*` matches any characters (the dot is a regex wildcard, not a literal dot)
- `\\d+` matches one or more digits
- When in doubt, test your patterns before deploying

### Environment Variables

The following **sensitive** environment variables are **required**:

- `SLACK_BOT_TOKEN` - Slack bot token for API access (required, e.g., `xoxb-...`)

The following **sensitive** environment variable is **optional**:

- `REDIS_PASSWORD` - Redis password (default: empty)

All configuration values from the YAML file can be overridden using environment variables:

- `REDIS_HOST` - Overrides `redis.host`
- `REDIS_PORT` - Overrides `redis.port`
- `REDIS_CHANNEL` - Overrides `redis.channel`
- `SLACK_REDIS_LIST` - Overrides `slack.redis_list`
- `SLACK_CHANNEL_ID` - Overrides `slack.channel_id`
- `POPPIT_CHANNEL` - Overrides `poppit.channel`
- `SLACK_REACTIONS_LIST` - Overrides `slack.reactions_list`
- `TIMEBOMB_CHANNEL` - Overrides `timebomb.channel`
- `SLACK_SEARCH_LIMIT` - Overrides `slack.search_limit`
- `LOG_LEVEL` - Overrides `logging.level`
- `DRAFT_NOTIFY_REPOS` - Comma-separated list overriding `draft_pr_filter.enabled_repos` (e.g., `owner/repo1,owner/repo2`)
- `DRAFT_NOTIFY_BRANCH_PREFIXES` - Comma-separated list overriding `draft_pr_filter.allowed_branch_prefixes` (e.g., `feature/,hotfix/,release/`)
- `BRANCH_BLACKLIST_PATTERNS` - Comma-separated list overriding `branch_blacklist.patterns` (e.g., `^dependabot/.*rc.*,^renovate/.*-beta`)

### Setting up SlackLiner

This service requires [SlackLiner](https://github.com/its-the-vibe/SlackLiner) to be running to deliver messages to Slack. SlackLiner:

1. Reads messages from the Redis list (default: `slack_messages`)
2. Posts them to Slack using the Slack API
3. Requires a Slack Bot Token with appropriate permissions

See the [SlackLiner documentation](https://github.com/its-the-vibe/SlackLiner) for setup instructions.

## Usage

### Using Docker Compose

1. Copy the example config file and customize it:

```bash
cp config.example.yaml config.yaml
```

2. Edit `config.yaml` and set your configuration (especially `slack.channel_id`).

3. Copy `.env.example` to `.env` and set sensitive credentials:

```bash
cp .env.example .env
```

4. Edit `.env` and set your sensitive configuration:

```
SLACK_BOT_TOKEN=xoxb-your-slack-bot-token
REDIS_PASSWORD=your-redis-password-if-needed
```

5. Start the service (along with SlackLiner if needed):

```bash
docker-compose up -d
```

**Note:** To override non-sensitive config values from `config.yaml` using environment variables in Docker Compose, add them to your `.env` file or specify them in the `docker-compose.yml` environment section.

### Using Docker

Build and run directly with Docker:

```bash
# Build the image
docker build -t octoslack .

# Run the container (mount config.yaml and pass sensitive env vars)
docker run -d \
  -v $(pwd)/config.yaml:/config.yaml \
  -e SLACK_BOT_TOKEN=xoxb-your-token \
  -e REDIS_PASSWORD=your-password \
  octoslack
```

### Using Go

Run directly with Go:

```bash
# Create config file
cp config.example.yaml config.yaml
# Edit config.yaml as needed

# Set sensitive environment variables
export SLACK_BOT_TOKEN=xoxb-your-token
export REDIS_PASSWORD=your-password

# Run the service
go run .
```

Or build and run:

```bash
go build -o octoslack .
./octoslack
```

**Note:** The service looks for `config.yaml` in the current working directory. If the file doesn't exist, it will use default values which can be overridden with environment variables.

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

**Note**: Draft PRs (`"draft": true`) are ignored by default. To receive notifications for draft PRs, configure both `DRAFT_NOTIFY_REPOS` and `DRAFT_NOTIFY_BRANCH_PREFIXES` environment variables. When configured, draft PRs will only trigger notifications if:
1. The repository matches one of the specified repositories in `DRAFT_NOTIFY_REPOS`
2. AND the branch name starts with one of the prefixes in `DRAFT_NOTIFY_BRANCH_PREFIXES`

For example, with `DRAFT_NOTIFY_REPOS=owner/repo` and `DRAFT_NOTIFY_BRANCH_PREFIXES=release/,hotfix/`, only draft PRs from `owner/repo` with branches starting with `release/` or `hotfix/` will send notifications.

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
  "type": "github-dispatcher",
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
redis-cli PUBLISH poppit:command-output '{"type":"git-dispatcher","command":"docker compose up --build -d","output":"Service deployed successfully","metadata":{"git_commit_sha":"66978703a4cd8d23e8dade6b4104cdfc98582128"}}'
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

