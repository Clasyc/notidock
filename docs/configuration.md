# Configuration Guide

This document provides detailed configuration information for Notidock.

## Table of Contents

- [Environment Variables](#environment-variables)
- [Container Labels](#container-labels)
- [Event Types](#event-types)
- [Health Monitoring](#health-monitoring)
- [Notification Features](#notification-features)
- [Security Considerations](#security-considerations)

## Environment Variables

| Environment Variable | Description | Default Value |
|---------------------|-------------|---------------|
| `NOTIDOCK_MONITOR_ALL` | When "true", monitors all containers unless explicitly excluded. When "false", only monitors containers with explicit include labels | `false` |
| `NOTIDOCK_TRACKED_EVENTS` | Comma-separated list of Docker events to track | `create,start,die,stop,kill` |
| `NOTIDOCK_TRACKED_EXITCODES` | Comma-separated list of container exit codes to track. When empty or unset, tracks all exit codes | `""` (all exit codes) |
| `NOTIDOCK_MONITOR_HEALTH` | When "true", enables container health monitoring | `false` |
| `NOTIDOCK_HEALTH_TIMEOUT` | Maximum duration to wait for health checks | `60s` |
| `NOTIDOCK_MAX_FAILING_STREAK` | Maximum number of consecutive health check failures before stopping monitoring | `3` |
| `NOTIDOCK_DOCKER_SOCKET` | Docker socket path for connecting to Docker daemon | `unix:///var/run/docker.sock` |
| `NOTIDOCK_WINDOW_DURATION` | Duration for the sliding window used in notification throttling | `60s` |
| `NOTIDOCK_EVENT_THRESHOLD` | Maximum number of events allowed within the window duration | `20` |
| `NOTIDOCK_NOTIFICATION_COOLDOWN` | Duration to wait before resuming notifications after throttling | `0s` (disabled) |
| `NOTIDOCK_SLACK_WEBHOOK_URL` | Webhook URL for Slack notifications (must use HTTPS) | Required |

## Container Labels

| Label | Description |
|-------|-------------|
| `notidock.exclude` | Exclude this container from monitoring regardless of global settings |
| `notidock.include` | Include this container for monitoring (required when MONITOR_ALL is false) |
| `notidock.name` | Custom name for the container in notifications (falls back to container name) |
| `notidock.events` | Comma-separated list of events to track for this specific container |
| `notidock.exitcodes` | Comma-separated list of exit codes to track for this specific container |

## Event Types

The following container events can be tracked (default: `create,start,die,stop,kill`):

### Lifecycle Events
- `create`: Container creation
- `destroy`: Container destruction
- `die`: Container termination with exit code
- `kill`: Container killed by signal
- `pause`: Container paused
- `restart`: Container restarted
- `start`: Container started
- `stop`: Container stopped
- `unpause`: Container unpaused
- `update`: Container resources updated

### Health Events
- `health_status`: Container health status change
- `oom`: Container out of memory

### Execution Events
- `exec_create`: Exec instance created
- `exec_start`: Exec instance started
- `exec_die`: Exec instance died

## Health Monitoring

When `NOTIDOCK_MONITOR_HEALTH` is enabled, Notidock:

- Monitors containers that have health checks configured
- Sends notifications on health status changes:
    - Healthy
    - Unhealthy
    - Starting
    - Health status with failing streak count
- Stops monitoring after `NOTIDOCK_MAX_FAILING_STREAK` consecutive failures
- Times out after `NOTIDOCK_HEALTH_TIMEOUT` duration

## Notification Features

### Slack Integration

Messages are sent via webhook with the following features:

#### Color Coding
- 🟢 Green: `create`, `start`, `unpause`, `healthy`
- 🔴 Red: `die`, `stop`, `kill`, `unhealthy`
- 🟤 Dark Red: `oom`
- 🟡 Orange: `pause`
- 🔵 Blue: `restart`, `update`
- ⚪ Grey: other events

#### Event Icons
- 📦 Create: `:package:`
- ▶️ Start: `:arrow_forward:`
- ⛔ Die: `:stop_sign:`
- 🛑 Stop: `:octagonal_sign:`
- ☠️ Kill: `:skull_and_crossbones:`
- ⚠️ OOM: `:warning: :memory:`
- ⏸️ Pause: `:pause_button:`
- ⏯️ Unpause: `:play_pause:`
- 🔄 Restart: `:arrows_counterclockwise:`
- 🔁 Update: `:arrows_clockwise:`

#### Message Content
- Container name (custom or default)
- Event type/action
- Timestamp
- Image details
- Execution duration (when applicable)
- Exit code (when applicable)
- Additional container labels
- Health status and streak (for health events)

### Throttling

Notification throttling helps prevent notification floods:

- Uses sliding window approach
- Window duration: `NOTIDOCK_WINDOW_DURATION`
- Event threshold: `NOTIDOCK_EVENT_THRESHOLD`
- Optional cooldown period: `NOTIDOCK_NOTIFICATION_COOLDOWN`

## Security Considerations

### Running as Non-Root

Notidock is designed to run as a non-root user with minimal privileges:

```bash
docker run \
  --user nobody \
  --read-only \
  --security-opt no-new-privileges=true \
  ...
```

### Docker Socket Access

- Mount socket read-only: `-v /var/run/docker.sock:/var/run/docker.sock:ro`
- Add container to Docker group: `--group-add $(stat -c '%g' /var/run/docker.sock)`

### Filesystem Security

- Read-only root filesystem: `--read-only`
- No new privileges: `--security-opt no-new-privileges=true`
- Minimal base image