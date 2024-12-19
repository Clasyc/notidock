package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
)

type SlackNotifier struct {
	webhookURL string
	client     *http.Client
}

type slackMessage struct {
	Text        string       `json:"text"`
	Attachments []attachment `json:"attachments,omitempty"`
}

type attachment struct {
	Color  string  `json:"color"`
	Fields []field `json:"fields"`
}

type field struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// getIcon returns an appropriate emoji based on the action
func getIcon(action string, exitCode string, labels map[string]string) string {
	// First check for specific exit codes that might override the action icon
	if exitCode != "" {
		// Check for OOM kill
		if exitCode == "137" {
			return ":warning: :memory:"
		}
		// Check for error exit codes
		if exitCode != "0" {
			return ":x:"
		}
	}

	// Check for health status events first
	if action == "health_status" {
		status := labels["health_status"]
		switch status {
		case "healthy":
			return ":white_check_mark:"
		case "unhealthy":
			return ":warning:"
		default:
			return ":question:"
		}
	}

	// Then check action-specific icons
	switch action {
	case "create":
		return ":package:"
	case "start":
		return ":arrow_forward:"
	case "die":
		return ":stop_sign:"
	case "stop":
		return ":octagonal_sign:"
	case "kill":
		return ":skull_and_crossbones:"
	case "oom":
		return ":warning: :memory:"
	case "pause":
		return ":pause_button:"
	case "unpause":
		return ":play_pause:"
	case "restart":
		return ":arrows_counterclockwise:"
	case "update":
		return ":arrows_clockwise:"
	case "destroy":
		return ":x:"
	case "exec_create":
		return ":terminal:"
	case "exec_start":
		return ":arrow_forward: :terminal:"
	case "exec_die":
		return ":x: :terminal:"
	default:
		return ":information_source:"
	}
}

// getColor improvements
func getColor(action string, labels map[string]string) string {
	// Check for health status events first
	if action == "health_status" {
		status := labels["health_status"]
		switch status {
		case "healthy":
			return "#36a64f" // green
		case "unhealthy":
			return "#ff0000" // red
		default:
			return "#808080" // grey
		}
	}

	switch action {
	case "create", "start", "unpause":
		return "#36a64f" // green
	case "die", "stop", "kill", "destroy":
		return "#ff0000" // red
	case "oom":
		return "#8B0000" // dark red
	case "pause":
		return "#FFA500" // orange
	case "restart", "update":
		return "#1E90FF" // blue
	case "exec_create", "exec_start":
		return "#36a64f" // green
	case "exec_die":
		return "#ff0000" // red
	default:
		return "#808080" // grey
	}
}

func NewSlackNotifier() (*SlackNotifier, error) {
	webhookURL := os.Getenv("NOTIDOCK_SLACK_WEBHOOK_URL")
	if webhookURL == "" {
		return nil, fmt.Errorf("NOTIDOCK_SLACK_WEBHOOK_URL environment variable is not set")
	}

	parsedURL, err := url.Parse(webhookURL)
	if err != nil || parsedURL.Scheme != "https" {
		return nil, errors.New("invalid webhook URL: must be a valid URL and use https")
	}

	return &SlackNotifier{
		webhookURL: webhookURL,
		client:     &http.Client{},
	}, nil
}

func (s *SlackNotifier) Name() string {
	return "slack"
}

// Send implements the Notifier interface for Slack
func (s *SlackNotifier) Send(ctx context.Context, event Event) error {
	fields := []field{
		{
			Title: "Action",
			Value: event.Action,
			Short: true,
		},
		{
			Title: "Time",
			Value: event.Time,
			Short: true,
		},
	}

	// Special handling for health status events
	if event.Action == "health_status" {
		if status, ok := event.Labels["health_status"]; ok {
			fields = append(fields, field{
				Title: "Health Status",
				Value: status,
				Short: true,
			})
		}
		if streak, ok := event.Labels["failing_streak"]; ok {
			fields = append(fields, field{
				Title: "Failing Streak",
				Value: streak,
				Short: true,
			})
		}
	} else {
		// Regular event handling
		// Add image information if available
		if image, ok := event.Labels["image"]; ok {
			fields = append(fields, field{
				Title: "Image",
				Value: image,
				Short: true,
			})
		}

		// Add execution duration if available
		if event.ExecDuration != "N/A" {
			fields = append(fields, field{
				Title: "Duration",
				Value: event.ExecDuration,
				Short: true,
			})
		}

		// Add exit code if available
		if event.ExitCode != "" {
			fields = append(fields, field{
				Title: "Exit Code",
				Value: event.ExitCode,
				Short: true,
			})
		}
	}

	// Add remaining labels that haven't been explicitly handled
	for k, v := range event.Labels {
		// Skip labels we've already handled
		if k == "image" || k == "exitCode" || k == "execDuration" ||
			(event.Action == "health_status" && (k == "health_status" || k == "failing_streak")) {
			continue
		}
		fields = append(fields, field{
			Title: k,
			Value: v,
			Short: true,
		})
	}

	icon := getIcon(event.Action, event.Labels["exitCode"], event.Labels)
	color := getColor(event.Action, event.Labels)

	msg := slackMessage{
		Text: fmt.Sprintf("%s Container Event: %s", icon, event.ContainerName),
		Attachments: []attachment{
			{
				Color:  color,
				Fields: fields,
			},
		},
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal slack message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.webhookURL, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send slack notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack notification failed with status code: %d", resp.StatusCode)
	}

	return nil
}
