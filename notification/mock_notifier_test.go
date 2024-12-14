package notification

import (
	"context"
	"sync"
)

type MockNotifier struct {
	mu      sync.Mutex
	name    string
	events  []Event
	sendErr error
}

func NewMockNotifier(name string) *MockNotifier {
	return &MockNotifier{
		name:   name,
		events: make([]Event, 0),
	}
}

func (m *MockNotifier) Send(ctx context.Context, event Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return m.sendErr
}

func (m *MockNotifier) Name() string {
	return m.name
}

func (m *MockNotifier) SetError(err error) {
	m.sendErr = err
}

func (m *MockNotifier) GetEvents() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.events
}
