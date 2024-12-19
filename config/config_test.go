package config

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestGetConfig(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected AppConfig
	}{
		{
			name:     "default values when no env vars set",
			envVars:  map[string]string{},
			expected: getDefaultConfig(),
		},
		{
			name: "custom monitoring settings",
			envVars: map[string]string{
				"NOTIDOCK_MONITOR_ALL":    "true",
				"NOTIDOCK_TRACKED_EVENTS": "start,stop",
			},
			expected: func() AppConfig {
				cfg := getDefaultConfig()
				cfg.MonitorAllContainers = true
				cfg.TrackedEvents = []string{"start", "stop"}
				return cfg
			}(),
		},
		{
			name: "custom health check settings",
			envVars: map[string]string{
				"NOTIDOCK_MONITOR_HEALTH":     "true",
				"NOTIDOCK_HEALTH_TIMEOUT":     "120s",
				"NOTIDOCK_MAX_FAILING_STREAK": "5",
			},
			expected: func() AppConfig {
				cfg := getDefaultConfig()
				cfg.MonitorHealth = true
				cfg.HealthCheckTimeout = 120 * time.Second
				cfg.MaxFailingStreak = 5
				return cfg
			}(),
		},
		{
			name: "custom throttling settings",
			envVars: map[string]string{
				"NOTIDOCK_WINDOW_DURATION":       "30s",
				"NOTIDOCK_EVENT_THRESHOLD":       "10",
				"NOTIDOCK_NOTIFICATION_COOLDOWN": "5s",
			},
			expected: func() AppConfig {
				cfg := getDefaultConfig()
				cfg.WindowDuration = 30 * time.Second
				cfg.EventThreshold = 10
				cfg.NotificationCooldown = 5 * time.Second
				return cfg
			}(),
		},
		{
			name: "invalid values should use defaults",
			envVars: map[string]string{
				"NOTIDOCK_HEALTH_TIMEOUT":     "invalid",
				"NOTIDOCK_MAX_FAILING_STREAK": "not-a-number",
				"NOTIDOCK_EVENT_THRESHOLD":    "invalid-number",
			},
			expected: getDefaultConfig(),
		},
		{
			name: "empty tracked events should use default values",
			envVars: map[string]string{
				"NOTIDOCK_TRACKED_EVENTS": "",
			},
			expected: getDefaultConfig(),
		},
		{
			name: "custom docker socket",
			envVars: map[string]string{
				"NOTIDOCK_DOCKER_SOCKET": "tcp://localhost:2375",
			},
			expected: func() AppConfig {
				cfg := getDefaultConfig()
				cfg.DockerSocket = "tcp://localhost:2375"
				return cfg
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment before each test
			os.Clearenv()

			// Set environment variables for test
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			got := GetConfig()
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("GetConfig() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParsers(t *testing.T) {
	t.Run("parseBool", func(t *testing.T) {
		tests := []struct {
			input    string
			expected bool
		}{
			{"true", true},
			{"false", false},
			{"True", false},
			{"False", false},
			{"1", false},
			{"0", false},
			{"", false},
		}

		for _, tt := range tests {
			got, _ := parseBool(tt.input)
			if got != tt.expected {
				t.Errorf("parseBool(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		}
	})

	t.Run("parseInt", func(t *testing.T) {
		tests := []struct {
			input    string
			expected int
			wantErr  bool
		}{
			{"123", 123, false},
			{"-123", -123, false},
			{"0", 0, false},
			{"abc", 0, true},
			{"", 0, true},
			{"12.34", 0, true},
		}

		for _, tt := range tests {
			got, err := parseInt(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseInt(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				continue
			}
			if !tt.wantErr && got != tt.expected {
				t.Errorf("parseInt(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		}
	})

	t.Run("parseDuration", func(t *testing.T) {
		tests := []struct {
			input    string
			expected time.Duration
			wantErr  bool
		}{
			{"10s", 10 * time.Second, false},
			{"5m", 5 * time.Minute, false},
			{"2h", 2 * time.Hour, false},
			{"invalid", 0, true},
			{"", 0, true},
			{"5", 0, true},
		}

		for _, tt := range tests {
			got, err := parseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				continue
			}
			if !tt.wantErr && got != tt.expected {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		}
	})

	t.Run("parseStringSlice", func(t *testing.T) {
		tests := []struct {
			input    string
			expected []string
		}{
			{"a,b,c", []string{"a", "b", "c"}},
			{"a", []string{"a"}},
			{"", nil},
			{" a , b , c ", []string{"a", "b", "c"}},
			{",,,", nil},
			{" , , ", nil},
		}

		for _, tt := range tests {
			got, err := parseStringSlice(tt.input)
			if err != nil {
				t.Errorf("parseStringSlice(%q) unexpected error: %v", tt.input, err)
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("parseStringSlice(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		}
	})
}

// Helper function to get default config for comparison
func getDefaultConfig() AppConfig {
	return AppConfig{
		MonitorAllContainers: DefaultMonitorAll,
		TrackedEvents:        strings.Split(DefaultTrackedEvents, ","),
		TrackedExitCodes:     nil,
		MonitorHealth:        DefaultMonitorHealth,
		HealthCheckTimeout:   DefaultHealthTimeout,
		MaxFailingStreak:     DefaultMaxFailingStreak,
		DockerSocket:         DefaultDockerSocket,
		WindowDuration:       DefaultWindowDuration,
		EventThreshold:       DefaultEventThreshold,
		NotificationCooldown: DefaultNotificationCooldown,
	}
}

func TestEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultValue string
		parser       func(string) (string, error)
		expected     string
	}{
		{
			name:         "env var set",
			key:          "TEST_KEY",
			envValue:     "test-value",
			defaultValue: "default",
			parser:       parseString,
			expected:     "test-value",
		},
		{
			name:         "env var not set",
			key:          "TEST_KEY",
			envValue:     "",
			defaultValue: "default",
			parser:       parseString,
			expected:     "default",
		},
		{
			name:         "env var empty",
			key:          "TEST_KEY",
			envValue:     "",
			defaultValue: "default",
			parser:       parseString,
			expected:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			if tt.envValue != "" {
				os.Setenv(EnvPrefix+tt.key, tt.envValue)
			}

			got := EnvOrDefault(tt.key, tt.defaultValue, tt.parser)
			if got != tt.expected {
				t.Errorf("EnvOrDefault() = %v, want %v", got, tt.expected)
			}
		})
	}
}
