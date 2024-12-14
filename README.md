# Notidock

A Docker container event monitoring tool that watches for container lifecycle events with customizable filtering and 
labeling.

## Features

- Monitor all or selected containers via labels
- Customizable event tracking (create, start, die, stop, kill)
- Container-specific event filtering
- Custom container naming via labels
- Secure by default (read-only, no-new-privileges, non-root user)

## Quick Install

```bash
docker run --network host --read-only --security-opt no-new-privileges=true -v /var/run/docker.sock:/var/run/docker.sock:ro --group-add $(stat -c '%g' /var/run/docker.sock) -e NOTIDOCK_SLACK_WEBHOOK_URL=your_webhook_url notidock
```
Replace `your_webhook_url` with your Slack webhook URL.

## Configuration

### Environment Variables

| Environment Variable         | Description                                                                                                                          | Default                      |
|------------------------------|--------------------------------------------------------------------------------------------------------------------------------------|------------------------------|
| `NOTIDOCK_MONITOR_ALL`       | When "true", monitors all containers unless explicitly excluded. When "false", only monitors containers with explicit include labels | `false`                      |
| `NOTIDOCK_TRACKED_EVENTS`    | Comma-separated list of events to track                                                                                              | `create,start,die,stop,kill` |
| `NOTIDOCK_TRACKED_EXITCODES` | Comma-separated list of container exit codes to track. When empty, tracks all exit codes                                             | `""` (all exit codes)        |
| `NOTIDOCK_SLACK_WEBHOOK_URL` | Webhook URL for Slack notifications                                                                                                  | -                            |

### Container Labels

All labels use the `notidock.` prefix:

- `notidock.exclude`: Exclude container from monitoring when in monitor-all mode
- `notidock.include`: Include container for monitoring when in selective mode
- `notidock.name`: Custom name to use in log and notifications
- `notidock.events`: Comma-separated list of events to track for this specific container
- `notidock.exitcodes`: Comma-separated list of container exit codes to track for this specific container

## Running

### Using Docker Compose

```yaml
services:
  notidock:
    build:
      context: .
      dockerfile: Dockerfile
    read_only: true
    security_opt:
      - no-new-privileges:true
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    environment:
      - SLACK_WEBHOOK_URL=your_webhook_url
    group_add:
      - "984"  # Docker socket group ID
    networks:
      - notidock-net

networks:
  notidock-net:
    driver: bridge
```

> [!NOTE]
> **Note:** check your Docker socket group ID with `stat -c '%g' /var/run/docker.sock` and set it accordingly in the
> `group_add` section.

Run with:
```bash
docker compose up
```

### Using Docker Run

One-liner to build and run:
```bash
docker build -t notidock . && docker run --network host --read-only --security-opt no-new-privileges=true -v /var/run/docker.sock:/var/run/docker.sock:ro --group-add $(stat -c '%g' /var/run/docker.sock) -e SLACK_WEBHOOK_URL=your_webhook_url notidock
```

## Example Usage

Monitor a specific container:
```bash
docker run -d \
  --label notidock.name=my-app \
  --label notidock.include=true \
  --label notidock.events=create,die \
  alpine sleep infinity
```

Exclude a container from monitoring:
```bash
docker run -d \
  --label notidock.exclude=true \
  alpine sleep infinity
```

## Security

Notidock is designed with security in mind:
- Runs as a non-root user
- Read-only filesystem
- No new privileges
- Read-only access to Docker socket
- Minimal base image

## Event Output

Events are logged in structured format:
```
2024/12/14 18:31:56 INFO Container event containerName=my-app action=create time=1734193947 labels=map[...]
```

## Building from Source

Requirements:
- Go 1.23 or later
- Docker

Build the binary:
```bash
go build -o notidock
```

## License