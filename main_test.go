package main

import "testing"

func TestShouldMonitorContainer(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		labels map[string]string
		want   bool
	}{
		{
			name: "excluded container",
			config: Config{
				MonitorAllContainers: true,
			},
			labels: map[string]string{
				"notidock.exclude": "",
			},
			want: false,
		},
		{
			name: "monitor all containers",
			config: Config{
				MonitorAllContainers: true,
			},
			labels: map[string]string{},
			want:   true,
		},
		{
			name: "included container",
			config: Config{
				MonitorAllContainers: false,
			},
			labels: map[string]string{
				"notidock.include": "",
			},
			want: true,
		},
		{
			name: "not included container",
			config: Config{
				MonitorAllContainers: false,
			},
			labels: map[string]string{},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldMonitorContainer(tt.config, tt.labels)
			if got != tt.want {
				t.Errorf("shouldMonitorContainer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldTrackExitCode(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		exitCode string
		labels   map[string]string
		want     bool
	}{
		{
			name: "container specific exit code",
			config: Config{
				TrackedExitCodes: []string{"1", "2"},
			},
			exitCode: "137",
			labels: map[string]string{
				"notidock.exitcodes": "137,143",
			},
			want: true,
		},
		{
			name: "global exit code",
			config: Config{
				TrackedExitCodes: []string{"137", "143"},
			},
			exitCode: "137",
			labels:   map[string]string{},
			want:     true,
		},
		{
			name: "untracked exit code",
			config: Config{
				TrackedExitCodes: []string{"137", "143"},
			},
			exitCode: "1",
			labels:   map[string]string{},
			want:     false,
		},
		{
			name: "track all exit codes",
			config: Config{
				TrackedExitCodes: nil,
			},
			exitCode: "1",
			labels:   map[string]string{},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldTrackExitCode(tt.config, tt.exitCode, tt.labels)
			if got != tt.want {
				t.Errorf("shouldTrackExitCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldTrackEvent(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		action string
		labels map[string]string
		want   bool
	}{
		{
			name: "container specific event",
			config: Config{
				TrackedEvents: []string{"start", "stop"},
			},
			action: "die",
			labels: map[string]string{
				"notidock.events": "die,kill",
			},
			want: true,
		},
		{
			name: "global event",
			config: Config{
				TrackedEvents: []string{"start", "die"},
			},
			action: "die",
			labels: map[string]string{},
			want:   true,
		},
		{
			name: "untracked event",
			config: Config{
				TrackedEvents: []string{"start", "stop"},
			},
			action: "die",
			labels: map[string]string{},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldTrackEvent(tt.config, tt.action, tt.labels)
			if got != tt.want {
				t.Errorf("shouldTrackEvent() = %v, want %v", got, tt.want)
			}
		})
	}
}
