package notification

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewSlackNotifier(t *testing.T) {
	tests := []struct {
		name       string
		webhookURL string
		wantErr    bool
	}{
		{
			name:       "valid webhook URL",
			webhookURL: "https://hooks.slack.com/services/xxx/yyy/zzz",
			wantErr:    false,
		},
		{
			name:       "invalid webhook URL scheme",
			webhookURL: "http://hooks.slack.com/services/xxx/yyy/zzz",
			wantErr:    true,
		},
		{
			name:       "empty webhook URL",
			webhookURL: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NOTIDOCK_SLACK_WEBHOOK_URL", tt.webhookURL)

			notifier, err := NewSlackNotifier()
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if notifier == nil {
				t.Error("expected notifier, got nil")
			}
		})
	}
}

func TestSlackNotifier_Send(t *testing.T) {
	// Create a test server to mock Slack's webhook endpoint
	var receivedBody string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create a test event
	event := Event{
		ContainerName: "test-container",
		Action:        "die",
		Time:          "2024-12-14T17:34:36Z",
		ExitCode:      "137 (SIGKILL) Container received kill signal",
		ExecDuration:  "1m 30s",
		Labels: map[string]string{
			"environment": "test",
			"exitCode":    "137",
		},
	}

	// Create a notifier with our test server URL
	notifier := &SlackNotifier{
		webhookURL: server.URL,
		client:     server.Client(),
	}

	// Send the notification
	err := notifier.Send(context.Background(), event)
	if err != nil {
		t.Errorf("failed to send notification: %v", err)
	}

	// Basic payload verification
	expectedContents := []string{
		"test-container",
		"die",
		"2024-12-14T17:34:36Z",
		"137 (SIGKILL)",
		"1m 30s",
		"environment",
		"test",
	}

	for _, expected := range expectedContents {
		if !strings.Contains(receivedBody, expected) {
			t.Errorf("expected payload to contain %q, but it doesn't.\nPayload: %s", expected, receivedBody)
		}
	}

	// Verify the message format with icon (for OOM kill)
	expectedText := fmt.Sprintf(`"text":":warning: :memory: Container Event: test-container"`)
	if !strings.Contains(receivedBody, expectedText) {
		t.Errorf("payload doesn't contain expected text field with icon.\nExpected: %s\nGot: %s", expectedText, receivedBody)
	}

	if !strings.Contains(receivedBody, `"color":"#ff0000"`) {
		t.Error("payload doesn't contain expected color for 'die' action")
	}
}

func TestGetColor(t *testing.T) {
	tests := []struct {
		name     string
		action   string
		expected string
	}{
		{
			name:     "create action",
			action:   "create",
			expected: "#36a64f",
		},
		{
			name:     "start action",
			action:   "start",
			expected: "#36a64f",
		},
		{
			name:     "die action",
			action:   "die",
			expected: "#ff0000",
		},
		{
			name:     "stop action",
			action:   "stop",
			expected: "#ff0000",
		},
		{
			name:     "unknown action",
			action:   "unknown",
			expected: "#808080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			color := getColor(tt.action)
			if color != tt.expected {
				t.Errorf("getColor(%q) = %q, want %q", tt.action, color, tt.expected)
			}
		})
	}
}

func TestGetIcon(t *testing.T) {
	tests := []struct {
		name     string
		action   string
		exitCode string
		want     string
	}{
		{
			name:     "create action",
			action:   "create",
			exitCode: "",
			want:     ":package:",
		},
		{
			name:     "start action",
			action:   "start",
			exitCode: "",
			want:     ":arrow_forward:",
		},
		{
			name:     "die action",
			action:   "die",
			exitCode: "",
			want:     ":stop_sign:",
		},
		{
			name:     "stop action",
			action:   "stop",
			exitCode: "",
			want:     ":octagonal_sign:",
		},
		{
			name:     "kill action",
			action:   "kill",
			exitCode: "",
			want:     ":skull_and_crossbones:",
		},
		{
			name:     "oom kill",
			action:   "die",
			exitCode: "137",
			want:     ":warning: :memory:",
		},
		{
			name:     "error exit",
			action:   "die",
			exitCode: "1",
			want:     ":x:",
		},
		{
			name:     "successful exit",
			action:   "die",
			exitCode: "0",
			want:     ":stop_sign:",
		},
		{
			name:     "unknown action",
			action:   "unknown",
			exitCode: "",
			want:     ":information_source:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getIcon(tt.action, tt.exitCode)
			if got != tt.want {
				t.Errorf("getIcon() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSlackNotifier_Send_ContextCancellation(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := &SlackNotifier{
		webhookURL: server.URL,
		client:     server.Client(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	event := Event{
		ContainerName: "test-container",
		Action:        "start",
	}

	err := notifier.Send(ctx, event)
	if err == nil {
		t.Error("expected error due to cancelled context, got nil")
	}
}
