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
func getIcon(action string, exitCode string) string {
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
	default:
		return ":information_source:"
	}
}

// getColor returns a color based on the action
func getColor(action string) string {
	switch action {
	case "create", "start", "unpause":
		return "#36a64f" // green
	case "die", "stop", "kill":
		return "#ff0000" // red
	case "oom":
		return "#8B0000" // dark red
	case "pause":
		return "#FFA500" // orange
	case "restart", "update":
		return "#1E90FF" // blue
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

	// Add all labels
	for k, v := range event.Labels {
		fields = append(fields, field{
			Title: k,
			Value: v,
			Short: true,
		})
	}

	// Get appropriate icon for the event
	icon := getIcon(event.Action, event.Labels["exitCode"])

	msg := slackMessage{
		Text: fmt.Sprintf("%s Container Event: %s", icon, event.ContainerName),
		Attachments: []attachment{
			{
				Color:  getColor(event.Action),
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
