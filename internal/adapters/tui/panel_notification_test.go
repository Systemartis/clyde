package tui

import (
	"strings"
	"testing"
)

// TestNotificationStyle_Cycle verifies the chip cycles through every value
// before returning to the start. This is the user-facing contract of the
// settings overlay's Enter handler.
func TestNotificationStyle_Cycle(t *testing.T) {
	t.Parallel()

	got := NotificationFullscreen.Next()
	if got != NotificationBanner {
		t.Errorf("fullscreen.Next = %q, want %q", got, NotificationBanner)
	}
	got = got.Next()
	if got != NotificationOff {
		t.Errorf("banner.Next = %q, want %q", got, NotificationOff)
	}
	got = got.Next()
	if got != NotificationFullscreen {
		t.Errorf("off.Next = %q, want %q (cycle)", got, NotificationFullscreen)
	}
}

// TestNotificationStyle_IsValid covers the fall-back path used when a
// hand-edited TOML file slips an unknown value through the decoder.
func TestNotificationStyle_IsValid(t *testing.T) {
	t.Parallel()

	for _, ok := range []NotificationStyle{NotificationFullscreen, NotificationBanner, NotificationOff} {
		if !ok.IsValid() {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []NotificationStyle{"", "popup", "Fullscreen"} {
		if bad.IsValid() {
			t.Errorf("%q should not be valid", bad)
		}
	}
}

// TestResolveNotification covers the dispatch matrix.
func TestResolveNotification(t *testing.T) {
	t.Parallel()

	hookActive := HookNotification{Active: true, Tool: "Bash", KeyArg: "ls", Cwd: "/tmp"}
	hookIdle := HookNotification{}

	cases := []struct {
		name       string
		style      NotificationStyle
		notifAck   bool
		hook       HookNotification
		compaction CompactionState
		want       notificationDecision
	}{
		{
			name:  "off mutes everything even with a live hook",
			style: NotificationOff, hook: hookActive,
			want: notificationDecision{Hook: hookActive},
		},
		{
			name:  "ack mutes everything until a fresh trigger arrives",
			style: NotificationFullscreen, notifAck: true, hook: hookActive,
			want: notificationDecision{Hook: hookActive},
		},
		{
			name:  "fullscreen + hook → active fullscreen",
			style: NotificationFullscreen, hook: hookActive,
			want: notificationDecision{Active: true, Fullscreen: true, Hook: hookActive},
		},
		{
			name:  "banner + hook → active inline",
			style: NotificationBanner, hook: hookActive,
			want: notificationDecision{Active: true, Fullscreen: false, Hook: hookActive},
		},
		{
			name:       "fullscreen + compaction danger → active fullscreen",
			style:      NotificationFullscreen,
			hook:       hookIdle,
			compaction: CompactionDanger,
			want:       notificationDecision{Active: true, Fullscreen: true, Compaction: CompactionDanger},
		},
		{
			name:       "banner + compaction warn (not danger) → idle",
			style:      NotificationBanner,
			compaction: CompactionWarn,
			want:       notificationDecision{Compaction: CompactionWarn},
		},
		{
			name:  "no triggers, default style → idle",
			style: NotificationFullscreen,
			want:  notificationDecision{},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := resolveNotification(tc.style, tc.notifAck, tc.hook, tc.compaction, QuotaNotification{})
			if got != tc.want {
				t.Errorf("resolve(%q, ack=%v) = %+v, want %+v", tc.style, tc.notifAck, got, tc.want)
			}
		})
	}
}

// TestNotificationHeight_FullscreenIsZero ensures the layout reserves NO
// inline strip for fullscreen mode — the overlay replaces the body, so
// the banner-row budget must collapse to zero or the title bar pushes
// down by 4 rows for nothing.
func TestNotificationHeight_FullscreenIsZero(t *testing.T) {
	t.Parallel()

	hook := HookNotification{Active: true, Tool: "Bash", KeyArg: "ls"}
	q := QuotaNotification{}
	got := notificationHeight(NotificationFullscreen, false, hook, CompactionOK, q)
	if got != 0 {
		t.Errorf("fullscreen + active hook should reserve 0 inline rows, got %d", got)
	}

	got = notificationHeight(NotificationBanner, false, hook, CompactionOK, q)
	if got != 4 {
		t.Errorf("banner + active hook should reserve 4 inline rows, got %d", got)
	}

	got = notificationHeight(NotificationOff, false, hook, CompactionOK, q)
	if got != 0 {
		t.Errorf("off should reserve 0 inline rows, got %d", got)
	}
}

// TestRenderFullscreenNotification_AnimatesAcrossTicks verifies the
// rendered string actually changes with the frame counter — the whole
// point of "animated" notifications. We compare a tick-0 frame to a
// tick-12 frame; if they're identical the pulse code is dead.
func TestRenderFullscreenNotification_AnimatesAcrossTicks(t *testing.T) {
	t.Parallel()

	pal := TokyoNightPalette()
	st := NewStyles(pal)
	hook := HookNotification{Active: true, Tool: "Bash", KeyArg: "ls -la", Cwd: "/tmp"}
	dec := notificationDecision{Active: true, Fullscreen: true, Hook: hook}

	frame0 := FrameState{Tick: 0}
	frame12 := FrameState{Tick: 12}

	a := renderFullscreenNotification(st, pal, frame0, dec, 80, 12)
	b := renderFullscreenNotification(st, pal, frame12, dec, 80, 12)

	if a == "" {
		t.Fatal("frame 0 produced empty card")
	}
	if a == b {
		t.Error("card identical across tick 0 and tick 12 — animation never advances")
	}
}

// TestRenderFullscreenNotification_Compaction shows the compaction copy
// when there's no live hook, validating the title swap.
func TestRenderFullscreenNotification_Compaction(t *testing.T) {
	t.Parallel()

	pal := TokyoNightPalette()
	st := NewStyles(pal)
	dec := notificationDecision{Active: true, Fullscreen: true, Compaction: CompactionDanger}

	got := renderFullscreenNotification(st, pal, FrameState{Tick: 0}, dec, 80, 12)
	if !strings.Contains(got, "compaction") {
		t.Errorf("compaction card missing 'compaction' wording:\n%s", got)
	}
}

// TestDefaultConfig_NotificationStyle locks in the default — fullscreen.
// If we ever change the default we should change the test deliberately.
func TestDefaultConfig_NotificationStyle(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if cfg.NotificationStyle != NotificationFullscreen {
		t.Errorf("default NotificationStyle = %q, want %q", cfg.NotificationStyle, NotificationFullscreen)
	}
}
