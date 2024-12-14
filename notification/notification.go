package notification

import "context"

// Event represents a container event that can be sent
// via notifications
type Event struct {
	ContainerName string
	Action        string
	Time          string // Changed to string to store formatted time
	Labels        map[string]string
	ExitCode      string
	ExecDuration  string
}

type Notifier interface {
	Send(ctx context.Context, event Event) error
	Name() string
}

// Manager handles multiple notification methods
type Manager struct {
	notifiers []Notifier
}

// NewManager creates a new notification manager
func NewManager(notifiers ...Notifier) *Manager {
	return &Manager{
		notifiers: notifiers,
	}
}

// Send sends the event to all configured notifiers
func (m *Manager) Send(ctx context.Context, event Event) error {
	var lastErr error
	for _, n := range m.notifiers {
		if err := n.Send(ctx, event); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (m *Manager) AddNotifier(n Notifier) {
	m.notifiers = append(m.notifiers, n)
}

func (m *Manager) Notifiers() []Notifier {
	return m.notifiers
}
