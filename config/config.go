package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Constants for environment variable keys
const (
	EnvPrefix = "NOTIDOCK_"

	KeyMonitorAll           = "MONITOR_ALL"
	KeyTrackedEvents        = "TRACKED_EVENTS"
	KeyTrackedExitCodes     = "TRACKED_EXITCODES"
	KeyMonitorHealth        = "MONITOR_HEALTH"
	KeyHealthTimeout        = "HEALTH_TIMEOUT"
	KeyMaxFailingStreak     = "MAX_FAILING_STREAK"
	KeyDockerSocket         = "DOCKER_SOCKET"
	KeyWindowDuration       = "WINDOW_DURATION"
	KeyEventThreshold       = "EVENT_THRESHOLD"
	KeyNotificationCooldown = "NOTIFICATION_COOLDOWN"
)

// Default values
const (
	DefaultHealthTimeout        = 60 * time.Second
	DefaultMaxFailingStreak     = 3
	DefaultMonitorAll           = false
	DefaultMonitorHealth        = false
	DefaultDockerSocket         = "unix:///var/run/docker.sock"
	DefaultWindowDuration       = 60 * time.Second
	DefaultEventThreshold       = 20
	DefaultNotificationCooldown = 0 * time.Second
	DefaultTrackedEvents        = "create,start,die,stop,kill"
)

// AppConfig holds all application configuration
type AppConfig struct {
	// Container monitoring
	MonitorAllContainers bool
	TrackedEvents        []string
	TrackedExitCodes     []string

	// Health checking
	MonitorHealth      bool
	HealthCheckTimeout time.Duration
	MaxFailingStreak   int

	// Docker connection
	DockerSocket string

	// Throttling
	WindowDuration       time.Duration
	EventThreshold       int
	NotificationCooldown time.Duration
}

// GetConfig returns the complete application configuration
func GetConfig() AppConfig {
	return AppConfig{
		// Container monitoring
		MonitorAllContainers: EnvOrDefault(KeyMonitorAll, DefaultMonitorAll, parseBool),
		TrackedEvents:        EnvOrDefault(KeyTrackedEvents, strings.Split(DefaultTrackedEvents, ","), parseStringSlice),
		TrackedExitCodes:     EnvOrDefault(KeyTrackedExitCodes, []string(nil), parseStringSlice),

		// Health checking
		MonitorHealth:      EnvOrDefault(KeyMonitorHealth, DefaultMonitorHealth, parseBool),
		HealthCheckTimeout: EnvOrDefault(KeyHealthTimeout, DefaultHealthTimeout, parseDuration),
		MaxFailingStreak:   EnvOrDefault(KeyMaxFailingStreak, DefaultMaxFailingStreak, parseInt),

		// Docker connection
		DockerSocket: EnvOrDefault(KeyDockerSocket, DefaultDockerSocket, parseString),

		// Throttling
		WindowDuration:       EnvOrDefault(KeyWindowDuration, DefaultWindowDuration, parseDuration),
		EventThreshold:       EnvOrDefault(KeyEventThreshold, DefaultEventThreshold, parseInt),
		NotificationCooldown: EnvOrDefault(KeyNotificationCooldown, DefaultNotificationCooldown, parseDuration),
	}
}

func EnvOrDefault[T any](key string, defaultValue T, parser func(string) (T, error)) T {
	if value, exists := os.LookupEnv(EnvPrefix + key); exists && value != "" {
		if parsed, err := parser(value); err == nil {
			return parsed
		}
		// Log warning about invalid value and fallback to default
		slog.Warn("invalid environment variable value",
			"key", EnvPrefix+key,
			"value", value,
			"fallback", defaultValue,
		)
	}
	return defaultValue
}

// Parsers for different types
func parseBool(s string) (bool, error) {
	return s == "true", nil
}

func parseString(s string) (string, error) {
	return s, nil
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}

func parseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}

func parseStringSlice(s string) ([]string, error) {
	if s == "" {
		return nil, nil
	}
	result := make([]string, 0)
	for _, item := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	// If no valid items were found, return nil
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func (c AppConfig) Log() {
	slog.Info("notidock started with configuration")

	// Container monitoring settings
	slog.Info("container monitoring settings",
		"monitor_all", c.MonitorAllContainers,
		"tracked_events", c.TrackedEvents,
		"tracked_exit_codes", formatExitCodes(c.TrackedExitCodes),
	)

	// Health check settings
	slog.Info("health check settings",
		"enabled", c.MonitorHealth,
		"timeout", c.HealthCheckTimeout,
		"max_failing_streak", c.MaxFailingStreak,
	)

	// Docker connection settings
	slog.Info("docker connection settings",
		"socket_path", c.DockerSocket,
	)

	// Throttling settings
	slog.Info("throttling settings",
		"window_duration", c.WindowDuration,
		"event_threshold", c.EventThreshold,
		"notification_cooldown", formatDuration(c.NotificationCooldown),
	)
}

func formatExitCodes(codes []string) any {
	if len(codes) == 0 {
		return "all"
	}
	return codes
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "disabled"
	}
	return d.String()
}
