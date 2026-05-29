package deltaneutral

import (
	"testing"
	"time"
)

func TestParseInterval(t *testing.T) {
	tests := []struct {
		name      string
		interval  string
		expected  time.Duration
		wantError bool
	}{
		{"30 seconds", "30s", 30 * time.Second, false},
		{"1 minute", "1m", 1 * time.Minute, false},
		{"3 minutes", "3m", 3 * time.Minute, false},
		{"5 minutes", "5m", 5 * time.Minute, false},
		{"10 minutes", "10m", 10 * time.Minute, false},
		{"15 minutes", "15m", 15 * time.Minute, false},
		{"30 minutes", "30m", 30 * time.Minute, false},
		{"1 hour", "1h", 1 * time.Hour, false},
		{"2 hours", "2h", 2 * time.Hour, false},
		{"3 hours", "3h", 3 * time.Hour, false},
		{"4 hours", "4h", 4 * time.Hour, false},
		{"8 hours", "8h", 8 * time.Hour, false},
		{"1 day", "1d", 24 * time.Hour, false},
		{"unsupported 7 minutes", "7m", 0, true},
		{"unsupported 2 days", "2d", 0, true},
		{"empty string", "", 0, true},
		{"zero", "0", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseInterval(tt.interval)
			if (err != nil) != tt.wantError {
				t.Errorf("ParseInterval(%q) error = %v, wantError %v", tt.interval, err, tt.wantError)
				return
			}
			if err == nil && got != tt.expected {
				t.Errorf("ParseInterval(%q) = %v, want %v", tt.interval, got, tt.expected)
			}
		})
	}
}

func TestIntervalToMS(t *testing.T) {
	tests := []struct {
		name      string
		interval  string
		expected  int64
		wantError bool
	}{
		{"30 seconds", "30s", 30000, false},
		{"1 minute", "1m", 60000, false},
		{"5 minutes", "5m", 300000, false},
		{"1 hour", "1h", 3600000, false},
		{"1 day", "1d", 86400000, false},
		{"unsupported", "7m", 0, true},
		{"empty string", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IntervalToMS(tt.interval)
			if (err != nil) != tt.wantError {
				t.Errorf("IntervalToMS(%q) error = %v, wantError %v", tt.interval, err, tt.wantError)
				return
			}
			if err == nil && got != tt.expected {
				t.Errorf("IntervalToMS(%q) = %v, want %v", tt.interval, got, tt.expected)
			}
		})
	}
}

func TestIsSubMinute(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		expected bool
	}{
		{"30 seconds is sub-minute", "30s", true},
		{"1 minute is sub-minute", "1m", true},
		{"5 minutes is not sub-minute", "5m", false},
		{"1 hour is not sub-minute", "1h", false},
		{"1 day is not sub-minute", "1d", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSubMinute(tt.interval)
			if got != tt.expected {
				t.Errorf("IsSubMinute(%q) = %v, want %v", tt.interval, got, tt.expected)
			}
		})
	}
}

func TestDefaultMonitorInterval(t *testing.T) {
	if !ValidInterval(DefaultMonitorInterval) {
		t.Errorf("DefaultMonitorInterval %q is not valid", DefaultMonitorInterval)
	}
}

func TestValidInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		expected bool
	}{
		{"30 seconds", "30s", true},
		{"1 minute", "1m", true},
		{"3 minutes", "3m", true},
		{"5 minutes", "5m", true},
		{"10 minutes", "10m", true},
		{"15 minutes", "15m", true},
		{"30 minutes", "30m", true},
		{"1 hour", "1h", true},
		{"2 hours", "2h", true},
		{"3 hours", "3h", true},
		{"4 hours", "4h", true},
		{"8 hours", "8h", true},
		{"1 day", "1d", true},
		{"unsupported 7 minutes", "7m", false},
		{"unsupported 2 days", "2d", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidInterval(tt.interval)
			if got != tt.expected {
				t.Errorf("ValidInterval(%q) = %v, want %v", tt.interval, got, tt.expected)
			}
		})
	}
}
