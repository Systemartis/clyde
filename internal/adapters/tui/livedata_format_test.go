package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestFormatDuration covers the 4 ranges of formatDuration output:
//
//	< 60s                     → "Ns"
//	60s–<60m, secs == 0       → "Nm"
//	60s–<60m, secs > 0        → "Nm Ns"
//	>= 60m, mins == 0         → "Nh"
//	>= 60m, mins > 0          → "Nh Nm"
//
// The h/m branch is the regression: previously formatDuration capped at minutes,
// so "1h 5m" came out as "65m 0s" — useless for long idle / long thinking states.
func TestFormatDuration(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"sub-minute", 14 * time.Second, "14s"},
		{"zero", 0, "0s"},
		{"negative clamps to zero", -3 * time.Second, "0s"},
		{"exactly 60s rolls into minutes", 60 * time.Second, "1m"},
		{"minutes only", 2 * time.Minute, "2m"},
		{"minutes + seconds", time.Minute + 4*time.Second, "1m 4s"},
		{"59m 59s stays in minutes", 59*time.Minute + 59*time.Second, "59m 59s"},
		{"exactly 60m rolls into hours", 60 * time.Minute, "1h"},
		{"1h 5m", time.Hour + 5*time.Minute, "1h 5m"},
		{"2h exact", 2 * time.Hour, "2h"},
		{"3h 47m", 3*time.Hour + 47*time.Minute, "3h 47m"},
		{"seconds drop out beyond an hour", time.Hour + 5*time.Minute + 12*time.Second, "1h 5m"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := formatDuration(tc.in)
			if got != tc.want {
				t.Errorf("formatDuration(%s) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestFormatAge verifies the "X ago" suffix is appended unchanged.
func TestFormatAge(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   time.Duration
		want string
	}{
		{45 * time.Second, "45s ago"},
		{2 * time.Minute, "2m ago"},
		{90 * time.Minute, "1h 30m ago"},
	}
	for _, tc := range cases {
		got := formatAge(tc.in)
		if got != tc.want {
			t.Errorf("formatAge(%s) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestFormatResetsIn_WeeklyUnderOneHour is the regression test for the
// weekly-reset bug: when remaining drops below an hour, the old code returned
// "0h" because the int truncation of remaining.Hours() was zero. The user
// should see minutes instead.
func TestFormatResetsIn_WeeklyUnderOneHour(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name    string
		resetAt time.Time
		want    string
	}{
		{"45 minutes left", now.Add(45 * time.Minute), "45m"},
		{"1 minute left", now.Add(1 * time.Minute), "1m"},
		{"under 1 minute returns empty", now.Add(30 * time.Second), ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := formatResetsIn(tc.resetAt, now, true)
			if got != tc.want {
				t.Errorf("formatResetsIn(weekly %s) = %q, want %q", tc.resetAt.Sub(now), got, tc.want)
			}
		})
	}
}

// TestFormatTimestamp verifies the wall-clock display:
//
//   - empty input → empty string
//   - non-empty input is rendered in the SYSTEM LOCAL timezone, not UTC
//   - the literal "UTC" must not appear (regression: previous impl
//     hardcoded " UTC" and used ts.Hour() on a UTC time)
//   - the same instant in any source zone produces the same output
//     because the function localizes by instant
func TestFormatTimestamp(t *testing.T) {
	t.Parallel()

	if got := formatTimestamp(time.Time{}); got != "" {
		t.Errorf("formatTimestamp(zero) = %q, want \"\"", got)
	}

	utc := time.Date(2026, 5, 3, 14, 3, 0, 0, time.UTC)
	local := utc.Local()
	want := fmt.Sprintf("%02d:%02d", local.Hour(), local.Minute())

	if got := formatTimestamp(utc); got != want {
		t.Errorf("formatTimestamp(%s) = %q, want %q (system-local of the same instant)", utc, got, want)
	}

	if got := formatTimestamp(utc); strings.Contains(got, "UTC") {
		t.Errorf("formatTimestamp(%s) = %q must not contain 'UTC' suffix", utc, got)
	}

	plus3 := time.FixedZone("UTC+3", 3*60*60)
	sameInstant := time.Date(2026, 5, 3, 17, 3, 0, 0, plus3)
	if got := formatTimestamp(sameInstant); got != want {
		t.Errorf("formatTimestamp(same instant in +03) = %q, want %q (must localize by instant, not by source zone)", got, want)
	}
}

// TestFormatBashTime — sibling of TestFormatTimestamp for the bash-log
// HH:MM:SS variant. Same localization contract: zero → 8 spaces (column
// alignment placeholder), non-zero → system-local wall clock.
func TestFormatBashTime(t *testing.T) {
	t.Parallel()

	if got := formatBashTime(time.Time{}); got != "        " {
		t.Errorf("formatBashTime(zero) = %q, want 8 spaces", got)
	}

	utc := time.Date(2026, 5, 3, 14, 3, 27, 0, time.UTC)
	local := utc.Local()
	want := fmt.Sprintf("%02d:%02d:%02d", local.Hour(), local.Minute(), local.Second())

	if got := formatBashTime(utc); got != want {
		t.Errorf("formatBashTime(%s) = %q, want %q (system-local of the same instant)", utc, got, want)
	}

	plus3 := time.FixedZone("UTC+3", 3*60*60)
	sameInstant := time.Date(2026, 5, 3, 17, 3, 27, 0, plus3)
	if got := formatBashTime(sameInstant); got != want {
		t.Errorf("formatBashTime(same instant in +03) = %q, want %q (must localize by instant)", got, want)
	}
}

// TestFormatResetsIn_FiveHourSubMinute covers the 5h-window edge where
// remaining is below a minute. We want "" (effectively reset) rather than
// "0m".
func TestFormatResetsIn_FiveHourSubMinute(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	got := formatResetsIn(now.Add(20*time.Second), now, false)
	if got != "" {
		t.Errorf("formatResetsIn(5h, 20s) = %q, want \"\"", got)
	}
}
