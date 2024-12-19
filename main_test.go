package main

import (
	"notidock/config"
	"testing"
)

func TestShouldMonitorContainer(t *testing.T) {
	tests := []struct {
		name   string
		cfg    config.AppConfig
		labels map[string]string
		want   bool
	}{
		{
			name: "excluded container",
			cfg: config.AppConfig{
				MonitorAllContainers: true,
			},
			labels: map[string]string{
				"notidock.exclude": "",
			},
			want: false,
		},
		{
			name: "monitor all containers",
			cfg: config.AppConfig{
				MonitorAllContainers: true,
			},
			labels: map[string]string{},
			want:   true,
		},
		{
			name: "included container",
			cfg: config.AppConfig{
				MonitorAllContainers: false,
			},
			labels: map[string]string{
				"notidock.include": "",
			},
			want: true,
		},
		{
			name: "not included container",
			cfg: config.AppConfig{
				MonitorAllContainers: false,
			},
			labels: map[string]string{},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldMonitorContainer(tt.cfg, tt.labels)
			if got != tt.want {
				t.Errorf("shouldMonitorContainer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldTrackExitCode(t *testing.T) {
	tests := []struct {
		name     string
		cfg      config.AppConfig
		exitCode string
		labels   map[string]string
		want     bool
	}{
		{
			name: "container specific exit code",
			cfg: config.AppConfig{
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
			cfg: config.AppConfig{
				TrackedExitCodes: []string{"137", "143"},
			},
			exitCode: "137",
			labels:   map[string]string{},
			want:     true,
		},
		{
			name: "untracked exit code",
			cfg: config.AppConfig{
				TrackedExitCodes: []string{"137", "143"},
			},
			exitCode: "1",
			labels:   map[string]string{},
			want:     false,
		},
		{
			name: "track all exit codes",
			cfg: config.AppConfig{
				TrackedExitCodes: nil,
			},
			exitCode: "1",
			labels:   map[string]string{},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldTrackExitCode(tt.cfg, tt.exitCode, tt.labels)
			if got != tt.want {
				t.Errorf("shouldTrackExitCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldTrackEvent(t *testing.T) {
	tests := []struct {
		name   string
		cfg    config.AppConfig
		action string
		labels map[string]string
		want   bool
	}{
		{
			name: "container specific event",
			cfg: config.AppConfig{
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
			cfg: config.AppConfig{
				TrackedEvents: []string{"start", "die"},
			},
			action: "die",
			labels: map[string]string{},
			want:   true,
		},
		{
			name: "untracked event",
			cfg: config.AppConfig{
				TrackedEvents: []string{"start", "stop"},
			},
			action: "die",
			labels: map[string]string{},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldTrackEvent(tt.cfg, tt.action, tt.labels)
			if got != tt.want {
				t.Errorf("shouldTrackEvent() = %v, want %v", got, tt.want)
			}
		})
	}
}
