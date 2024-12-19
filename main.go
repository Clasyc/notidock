package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/docker/docker/client"
	"log/slog"
	"net/http"
	"net/url"
	"notidock/notification"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	EnvPrefix = "NOTIDOCK_"

	DefaultHealthTimeout     = 60 * time.Second
	DefaultMaxFailingStreak  = 3
	DefaultMonitorAll        = false
	DefaultMonitorHealth     = false
	DefaultDockerSocket      = "unix:///var/run/docker.sock"
	DefaultWindowDuration    = 60 // seconds
	DefaultEventThreshold    = 20
	DefaultNotificationDelay = 0 // seconds

	DefaultTrackedEvents = "create,start,die,stop,kill"
)

type Config struct {
	MonitorAllContainers bool
	TrackedEvents        []string
	TrackedExitCodes     []string
	MonitorHealth        bool
	HealthCheckTimeout   time.Duration
	MaxFailingStreak     int
}

type Event struct {
	Type     string `json:"Type"`
	Action   string `json:"Action"`
	Actor    Actor  `json:"Actor"`
	Scope    string `json:"scope"`
	Time     int64  `json:"time"`
	TimeNano int64  `json:"timeNano"`
}

type Actor struct {
	ID         string            `json:"ID"`
	Attributes map[string]string `json:"Attributes"`
}

const (
	LabelPrefix        = "notidock."
	LabelExclude       = LabelPrefix + "exclude"
	LabelInclude       = LabelPrefix + "include"
	LabelName          = LabelPrefix + "name"
	LabelTrackedEvents = LabelPrefix + "events"
	LabelExitCodes     = LabelPrefix + "exitcodes"
)

var config Config

func main() {
	config = getConfig()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cli, err := setupDockerClient()
	if err != nil {
		panic(err)
	}
	err = checkDockerConnectivity(ctx, cli)
	if err != nil {
		panic(err)
	}
	throttler, err := NewNotificationThrottler()
	if err != nil {
		panic(err)
	}

	notificationManager := setupNotificationManager()

	req, err := createEventRequest(ctx)
	if err != nil {
		panic(err)
	}

	resp, err := cli.HTTPClient().Do(req.WithContext(ctx))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	eventChan := processEvents(ctx, decoder)

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logConfig(config, notificationManager)

	for {
		select {
		case <-sigChan:
			slog.Info("shutting down...")
			return
		case event, ok := <-eventChan:
			if !ok {
				slog.Info("event stream closed")
				return
			}
			if event.Type == "container" {
				handleContainerEvent(ctx, event, notificationManager, throttler, cli)
			}
		}
	}
}

func getConfig() Config {
	return Config{
		MonitorAllContainers: EnvOrDefault("MONITOR_ALL", DefaultMonitorAll, parseBool),
		TrackedEvents:        EnvOrDefault("TRACKED_EVENTS", strings.Split(DefaultTrackedEvents, ","), parseStringSlice),
		TrackedExitCodes:     EnvOrDefault("TRACKED_EXITCODES", []string(nil), parseStringSlice),
		MonitorHealth:        EnvOrDefault("MONITOR_HEALTH", DefaultMonitorHealth, parseBool),
		HealthCheckTimeout:   EnvOrDefault("HEALTH_TIMEOUT", DefaultHealthTimeout, parseDuration),
		MaxFailingStreak:     EnvOrDefault("MAX_FAILING_STREAK", DefaultMaxFailingStreak, parseInt),
	}
}

func parseEvents(events string) []string {
	if events == "" {
		return []string{"create", "start", "die", "stop", "kill"}
	}
	return strings.Split(events, ",")
}

func parseExitCodes(codes string) []string {
	if codes == "" {
		return nil // nil means track all exit codes
	}
	// Clean up the input by trimming spaces
	exitCodes := make([]string, 0)
	for _, code := range strings.Split(codes, ",") {
		if trimmed := strings.TrimSpace(code); trimmed != "" {
			exitCodes = append(exitCodes, trimmed)
		}
	}
	return exitCodes
}

func shouldTrackExitCode(config Config, exitCode string, labels map[string]string) bool {
	if containerExitCodes, exists := labels[LabelExitCodes]; exists {
		codes := make([]string, 0)
		for _, code := range strings.Split(containerExitCodes, ",") {
			if trimmed := strings.TrimSpace(code); trimmed != "" {
				codes = append(codes, trimmed)
			}
		}

		for _, code := range codes {
			if exitCode == code {
				return true
			}
		}
		return false
	}

	if len(config.TrackedExitCodes) == 0 {
		return true
	}

	if exitCode == "" {
		return false
	}

	for _, trackedCode := range config.TrackedExitCodes {
		if exitCode == trackedCode {
			return true
		}
	}
	return false
}

func shouldMonitorContainer(config Config, labels map[string]string) bool {
	if _, excluded := labels[LabelExclude]; excluded {
		return false
	}
	if config.MonitorAllContainers {
		return true
	}
	_, included := labels[LabelInclude]
	return included
}

func getContainerName(labels map[string]string) string {
	if customName, exists := labels[LabelName]; exists {
		return customName
	}
	return labels["name"]
}

func shouldTrackEvent(config Config, action string, labels map[string]string) bool {
	if eventsList, exists := labels[LabelTrackedEvents]; exists {
		containerEvents := strings.Split(eventsList, ",")
		for _, event := range containerEvents {
			if action == strings.TrimSpace(event) {
				return true
			}
		}
		return false
	}

	for _, event := range config.TrackedEvents {
		if action == strings.TrimSpace(event) {
			return true
		}
	}
	return false
}

func setupDockerClient() (*client.Client, error) {
	socketPath := os.Getenv("NOTIDOCK_DOCKER_SOCKET")
	if socketPath == "" {
		socketPath = "unix:///var/run/docker.sock"
	}

	if !strings.HasPrefix(socketPath, "unix://") && !strings.HasPrefix(socketPath, "tcp://") {
		return nil, fmt.Errorf("invalid socket path format: must start with unix:// or tcp://")
	}

	return client.NewClientWithOpts(
		client.FromEnv,
		client.WithHost(socketPath),
	)
}

func setupNotificationManager() *notification.Manager {
	var notifiers []notification.Notifier
	if slackNotifier, err := notification.NewSlackNotifier(); err != nil {
		slog.Error("failed to initialize slack notifier", "error", err)
	} else {
		notifiers = append(notifiers, slackNotifier)
	}
	return notification.NewManager(notifiers...)
}

func createEventRequest(ctx context.Context) (*http.Request, error) {
	query := url.Values{}
	query.Add("filters", `{"type":["container"]}`)

	return http.NewRequest("GET", "http://unix/v1.43/events?"+query.Encode(), nil)
}

func processEvents(ctx context.Context, decoder *json.Decoder) chan Event {
	eventChan := make(chan Event)

	go func() {
		defer close(eventChan)
		for {
			var event Event
			if err := decoder.Decode(&event); err != nil {
				if ctx.Err() != nil {
					return // Context was cancelled
				}
				slog.Error("failed to decode event", "error", err)
				continue
			}
			select {
			case eventChan <- event:
			case <-ctx.Done():
				return
			}
		}
	}()

	return eventChan
}

func handleContainerEvent(ctx context.Context, event Event, notificationManager *notification.Manager, throttler *NotificationThrottler, cli *client.Client) {
	if !shouldMonitorContainer(config, event.Actor.Attributes) {
		return
	}
	if !shouldTrackEvent(config, event.Action, event.Actor.Attributes) {
		return
	}

	exitCode := event.Actor.Attributes["exitCode"]
	if exitCode != "" && !shouldTrackExitCode(config, exitCode, event.Actor.Attributes) {
		return
	}

	containerName := getContainerName(event.Actor.Attributes)
	imageTag := event.Actor.Attributes["image"]
	if !throttler.ShouldNotify(containerName, imageTag) {
		slog.Info("notification throttled",
			"containerName", containerName,
			"imageTag", imageTag,
			"action", event.Action,
		)
		return
	}

	// Handle health monitoring for newly created containers
	if config.MonitorHealth && event.Action == "start" {
		go monitorContainerHealth(ctx, cli, event.Actor.ID, containerName, config, notificationManager)
	}

	exitCodeFormatted := FormatExitCode(exitCode)

	execDuration := "N/A"
	if durationStr, exists := event.Actor.Attributes["execDuration"]; exists {
		if duration, err := strconv.ParseInt(durationStr, 10, 64); err == nil {
			execDuration = FormatDuration(duration)
		}
	}

	slog.Info("container event",
		"containerName", containerName,
		"action", event.Action,
		"time", event.Time,
		"exitCode", exitCodeFormatted,
		"execDuration", execDuration,
		"labels", event.Actor.Attributes,
	)

	notificationEvent := notification.Event{
		ContainerName: containerName,
		Action:        event.Action,
		Time:          FormatTimestamp(event.Time),
		Labels:        event.Actor.Attributes,
		ExitCode:      exitCodeFormatted,
		ExecDuration:  execDuration,
	}

	if err := notificationManager.Send(ctx, notificationEvent); err != nil {
		slog.Error("failed to send notification", "error", err)
	}
}

func monitorContainerHealth(ctx context.Context, cli *client.Client, containerID, containerName string, config Config, notificationManager *notification.Manager) {
	timeoutCtx, cancel := context.WithTimeout(ctx, config.HealthCheckTimeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastReportedStatus string

	for {
		select {
		case <-timeoutCtx.Done():
			slog.Warn("health check timeout reached",
				"containerName", containerName,
				"containerID", containerID,
				"timeout", config.HealthCheckTimeout,
			)
			return
		case <-ticker.C:
			container, err := cli.ContainerInspect(ctx, containerID)
			if err != nil {
				slog.Error("failed to inspect container",
					"error", err,
					"containerName", containerName,
					"containerID", containerID,
				)
				return
			}

			// Check if container has health check configured
			if container.State.Health == nil {
				slog.Info("container has no health check configured",
					"containerName", containerName,
					"containerID", containerID,
				)
				return
			}

			currentStatus := container.State.Health.Status
			failingStreak := container.State.Health.FailingStreak

			// Only send notifications when status changes or failing streak exceeds threshold
			if currentStatus != lastReportedStatus ||
				(failingStreak >= config.MaxFailingStreak && currentStatus != "healthy") {

				healthEvent := notification.Event{
					ContainerName: containerName,
					Action:        "health_status",
					Time:          time.Now().Format(time.RFC3339),
					Labels: map[string]string{
						"health_status":  currentStatus,
						"failing_streak": strconv.Itoa(failingStreak),
					},
				}

				if err := notificationManager.Send(ctx, healthEvent); err != nil {
					slog.Error("failed to send health notification",
						"error", err,
						"containerName", containerName,
					)
				}

				slog.Info("container health status update",
					"containerName", containerName,
					"containerID", containerID,
					"status", currentStatus,
					"failingStreak", failingStreak,
				)

				lastReportedStatus = currentStatus
			}

			// Stop monitoring if container is healthy
			if currentStatus == "healthy" {
				return
			}

			// Stop monitoring if failing streak exceeds maximum
			if failingStreak >= config.MaxFailingStreak {
				slog.Warn("stopping health monitoring due to excessive failures",
					"containerName", containerName,
					"containerID", containerID,
					"failingStreak", failingStreak,
					"maxAllowed", config.MaxFailingStreak,
				)
				return
			}
		}
	}
}

func logConfig(config Config, m *notification.Manager) {
	slog.Info("notidock started with configuration")
	slog.Info("monitor all containers", "value", config.MonitorAllContainers)
	slog.Info("tracked events", "value", config.TrackedEvents)
	if len(config.TrackedExitCodes) > 0 {
		slog.Info("tracked exit codes", "value", config.TrackedExitCodes)
	} else {
		slog.Info("tracked exit codes", "value", "all")
	}

	if timeout := os.Getenv("NOTIDOCK_NOTIFICATION_TIMEOUT"); timeout != "" {
		slog.Info("notification timeout", "value", timeout+"s")
	} else {
		slog.Info("notification timeout", "value", "disabled")
	}
	if cooldown := os.Getenv("NOTIDOCK_NOTIFICATION_COOLDOWN"); cooldown != "" {
		slog.Info("notification cooldown", "value", cooldown+"s")
	} else {
		slog.Info("notification cooldown", "value", "disabled")
	}

	// Log Docker socket path
	socketPath := os.Getenv("NOTIDOCK_DOCKER_SOCKET")
	if socketPath == "" {
		socketPath = "unix:///var/run/docker.sock"
	}
	slog.Info("docker socket path", "value", socketPath)

	if len(m.Notifiers()) == 0 {
		slog.Warn("0 notifiers configured, no notifications will be sent")
		return
	}
}

func checkDockerConnectivity(ctx context.Context, cli *client.Client) error {
	ping, err := cli.Ping(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to Docker daemon: %w", err)
	}

	slog.Info("successfully connected to Docker daemon",
		"apiVersion", ping.APIVersion,
		"osType", ping.OSType,
		"experimental", ping.Experimental,
		"builderVersion", ping.BuilderVersion,
	)

	return nil
}
