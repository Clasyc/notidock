package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

func FormatDuration(seconds int64) string {
	duration := time.Duration(seconds) * time.Second

	days := duration / (24 * time.Hour)
	duration = duration % (24 * time.Hour)

	hours := duration / time.Hour
	duration = duration % time.Hour

	minutes := duration / time.Minute
	duration = duration % time.Minute

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, duration/time.Second)
	}
	return fmt.Sprintf("%ds", duration/time.Second)
}

func FormatTimestamp(timestamp int64) string {
	t := time.Unix(timestamp, 0)
	return t.Format(time.RFC3339)
}

func ExitCodeExplanation(code string) string {
	if code == "" {
		return ""
	}

	// Convert string to int for switch statement
	// If conversion fails, return empty string
	codeInt, err := strconv.Atoi(code)
	if err != nil {
		return ""
	}

	switch codeInt {
	// Standard Linux exit codes
	case 0:
		return "(Success) Container exited normally"
	case 1:
		return "(Error) Container exited with general error"
	case 2:
		return "(Error) Container exited due to misuse of shell builtins"
	case 126:
		return "(Error) Command invoked cannot execute"
	case 127:
		return "(Error) Command not found"
	case 128:
		return "(Error) Invalid exit argument"
	case 130:
		return "(Terminated) Container terminated by Ctrl-C"
	case 137:
		return "(SIGKILL) Container received kill signal or exceeded memory limit"
	case 139:
		return "(SIGSEGV) Container crashed with segmentation fault"
	case 143:
		return "(SIGTERM) Container received termination signal"

	// Docker specific codes
	case 255:
		return "(Error) Container exited with Docker fatal error"

	// Special Docker signal offset cases (128 + signal number)
	case 129: // 128 + 1 (SIGHUP)
		return "(SIGHUP) Container terminated by hangup"
	case 131: // 128 + 3 (SIGQUIT)
		return "(SIGQUIT) Container quit by quit signal"
	case 132: // 128 + 4 (SIGILL)
		return "(SIGILL) Container terminated by illegal instruction"
	case 134: // 128 + 6 (SIGABRT)
		return "(SIGABRT) Container aborted"
	case 135: // 128 + 7 (SIGBUS)
		return "(SIGBUS) Container terminated by bus error"
	case 136: // 128 + 8 (SIGFPE)
		return "(SIGFPE) Container terminated by floating point exception"
	case 138: // 128 + 10 (SIGUSR1)
		return "(SIGUSR1) Container terminated by user-defined signal 1"
	case 140: // 128 + 12 (SIGUSR2)
		return "(SIGUSR2) Container terminated by user-defined signal 2"
	case 141: // 128 + 13 (SIGPIPE)
		return "(SIGPIPE) Container terminated by broken pipe"
	case 142: // 128 + 14 (SIGALRM)
		return "(SIGALRM) Container terminated by timer"

	default:
		if codeInt > 128 {
			return fmt.Sprintf("(Signal %d) Container terminated by signal %d", codeInt-128, codeInt-128)
		}
		return fmt.Sprintf("(Code %d) Unknown exit code", codeInt)
	}
}

// FormatExitCode formats the exit code with its explanation
func FormatExitCode(code string) string {
	if code == "" {
		return ""
	}

	explanation := ExitCodeExplanation(code)
	if explanation == "" {
		return code
	}

	return fmt.Sprintf("%s %s", code, explanation)
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
	return result, nil
}
