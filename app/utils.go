package app

import (
	"fmt"
	"time"
)

// TimeAgo converts a time to a human-readable "time ago" string.
// It handles pluralization correctly (e.g., "1 minute ago" vs "2 minutes ago").
// The maximum resolution is seconds (e.g., "30 seconds ago").
func TimeAgo(t time.Time) string {
	now := time.Now()
	duration := now.Sub(t)

	// Handle future times (just in case)
	if duration < 0 {
		return "in the future"
	}

	// Calculate time differences
	seconds := int(duration.Seconds())
	minutes := int(duration.Minutes())
	hours := int(duration.Hours())
	days := hours / 24
	weeks := days / 7
	months := days / 30
	years := days / 365

	switch {
	case years > 0:
		if years == 1 {
			return "1 year ago"
		}
		return plural(years, "year")
	case months > 0:
		if months == 1 {
			return "1 month ago"
		}
		return plural(months, "month")
	case weeks > 0:
		if weeks == 1 {
			return "1 week ago"
		}
		return plural(weeks, "week")
	case days > 0:
		if days == 1 {
			return "1 day ago"
		}
		return plural(days, "day")
	case hours > 0:
		if hours == 1 {
			return "1 hour ago"
		}
		return plural(hours, "hour")
	case minutes > 0:
		if minutes == 1 {
			return "1 minute ago"
		}
		return plural(minutes, "minute")
	default:
		if seconds <= 5 {
			return "just now"
		}
		if seconds == 1 {
			return "1 second ago"
		}
		return plural(seconds, "second")
	}
}

// plural is a helper function that formats a count and noun with proper pluralization
func plural(count int, noun string) string {
	return fmt.Sprintf("%d %s ago", count, noun+"s")
}

func BytesToHuman(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp+1])
}
