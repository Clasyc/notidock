package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/docker/docker/client"
	"log/slog"
	"net/http"
	"net/url"
	"notidock/config"
	"notidock/notification"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

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

func main() {
	cfg := config.GetConfig()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cli, err := setupDockerClient(cfg.DockerSocket)
	if err != nil {
		panic(err)
	}

	err = checkDockerConnectivity(ctx, cli)
	if err != nil {
		panic(err)
	}

	throttler := NewNotificationThrottler(cfg)
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

	// Log configuration
	cfg.Log()
	if len(notificationManager.Notifiers()) == 0 {
		slog.Warn("notification settings", "status", "no notifiers configured")
	} else {
		slog.Info("notification settings", "notifiers_count", len(notificationManager.Notifiers()))
	}

	slog.Info("---")
	slog.Info("notidock started, listening for container events...")

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
				handleContainerEvent(ctx, event, cfg, notificationManager, throttler, cli)
			}
		}
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

func shouldTrackExitCode(cfg config.AppConfig, exitCode string, labels map[string]string) bool {
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

	if len(cfg.TrackedExitCodes) == 0 {
		return true
	}

	if exitCode == "" {
		return false
	}

	for _, trackedCode := range cfg.TrackedExitCodes {
		if exitCode == trackedCode {
			return true
		}
	}
	return false
}

func shouldMonitorContainer(cfg config.AppConfig, labels map[string]string) bool {
	if _, excluded := labels[LabelExclude]; excluded {
		return false
	}
	if cfg.MonitorAllContainers {
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

func shouldTrackEvent(cfg config.AppConfig, action string, labels map[string]string) bool {
	if eventsList, exists := labels[LabelTrackedEvents]; exists {
		containerEvents := strings.Split(eventsList, ",")
		for _, event := range containerEvents {
			if action == strings.TrimSpace(event) {
				return true
			}
		}
		return false
	}

	for _, event := range cfg.TrackedEvents {
		if action == strings.TrimSpace(event) {
			return true
		}
	}
	return false
}

func setupDockerClient(socketPath string) (*client.Client, error) {
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

func handleContainerEvent(ctx context.Context, event Event, cfg config.AppConfig, notificationManager *notification.Manager, throttler *NotificationThrottler, cli *client.Client) {
	if !shouldMonitorContainer(cfg, event.Actor.Attributes) {
		return
	}
	if !shouldTrackEvent(cfg, event.Action, event.Actor.Attributes) {
		return
	}

	exitCode := event.Actor.Attributes["exitCode"]
	if exitCode != "" && !shouldTrackExitCode(cfg, exitCode, event.Actor.Attributes) {
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
	if cfg.MonitorHealth && event.Action == "start" {
		go monitorContainerHealth(ctx, cli, event.Actor.ID, containerName, cfg, notificationManager)
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

func monitorContainerHealth(ctx context.Context, cli *client.Client, containerID, containerName string, cfg config.AppConfig, notificationManager *notification.Manager) {
	timeoutCtx, cancel := context.WithTimeout(ctx, cfg.HealthCheckTimeout)
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
				"timeout", cfg.HealthCheckTimeout,
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
				(failingStreak >= cfg.MaxFailingStreak && currentStatus != "healthy") {

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
			if failingStreak >= cfg.MaxFailingStreak {
				slog.Warn("stopping health monitoring due to excessive failures",
					"containerName", containerName,
					"containerID", containerID,
					"failingStreak", failingStreak,
					"maxAllowed", cfg.MaxFailingStreak,
				)
				return
			}
		}
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
