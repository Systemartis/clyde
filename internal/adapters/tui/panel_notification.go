package tui

import (
	"path/filepath"
	"strings"
)

// HookNotification holds the live hook event currently displayed in the banner.
// Zero value means no live event (fall through to mock data or hidden).
type HookNotification struct {
	// Active is true when an unanswered hook event is in flight.
	Active bool

	// Tool is the tool name, e.g. "Bash", "Edit".
	Tool string

	// KeyArg is the main argument for the tool (command, path, etc.).
	KeyArg string

	// Cwd is the working directory context for the call.
	Cwd string
}

// notificationDecision is the routing result for the active notification
// state. It separates "what to show" from "how to show it" so each
// render path picks the right path without re-deriving the rules.
//
// Sources are mutually exclusive in the dispatcher: priority is
// Hook > Compaction > Quota. Whichever wins is the only one that
// gets rendered until the user dismisses.
type notificationDecision struct {
	// Active is true when something is worth telling the user about
	// AND the user hasn't muted via Esc.
	Active bool
	// Fullscreen is true when the resolved style is fullscreen and a
	// notification is Active. Caller replaces the body region with
	// renderFullscreenNotification; banner-style decisions still use
	// renderNotificationMaybe inline.
	Fullscreen bool
	// Hook holds the in-flight hook event (when applicable).
	Hook HookNotification
	// Compaction is the saturation state (Danger triggers a notification
	// of its own, even when no hook is in flight).
	Compaction CompactionState
	// Quota is a plan-quota or cost alert.
	Quota QuotaNotification
}

// resolveNotification computes the dispatch state for the current frame.
// Encapsulates the "is there anything to show + how" decision in one
// place so render paths can branch cleanly.
//
// The decision drops to inactive when:
//   - the user dismissed via Esc (notifAck), OR
//   - the user picked NotificationOff in settings, OR
//   - no source is currently flagged (no hook, compaction below
//     danger, no quota crossing latched).
func resolveNotification(style NotificationStyle, notifAck bool, hook HookNotification, compaction CompactionState, quota QuotaNotification) notificationDecision {
	d := notificationDecision{Hook: hook, Compaction: compaction, Quota: quota}
	if notifAck || style == NotificationOff {
		return d
	}
	hasSource := hook.Active || compaction == CompactionDanger || quota.Active
	if !hasSource {
		return d
	}
	d.Active = true
	d.Fullscreen = style == NotificationFullscreen
	return d
}

// notificationOverlay returns the fullscreen card sized to bodyW × bodyH,
// or "" when fullscreen mode is not active. Render paths consult this
// before drawing the panel grid: when non-empty, the overlay REPLACES the
// grid (titlebar + statusbar still flank it).
func (m Model) notificationOverlay(bodyW, bodyH int) string {
	d := resolveNotification(m.cfg.NotificationStyle, m.notifAck, m.hookNotif, m.compaction, m.quotaNotif)
	if !d.Active || !d.Fullscreen {
		return ""
	}
	return renderFullscreenNotification(m.styles, m.palette, m.frame, d, bodyW, bodyH)
}

// renderNotificationMaybe renders the inline notification banner, or
// an empty string if no banner should be shown.
//
// Priority (highest first): hook → compaction → quota. Fullscreen-mode
// notifications are NOT rendered here: each render path inspects
// resolveNotification(...).Fullscreen and replaces the body instead.
// Off-mode notifications and dismissed banners produce "".
func renderNotificationMaybe(s Styles, p Palette, width int, style NotificationStyle, dismissed bool, hook HookNotification, compaction CompactionState, quota QuotaNotification) string {
	d := resolveNotification(style, dismissed, hook, compaction, quota)
	if !d.Active || d.Fullscreen {
		return ""
	}
	if hook.Active {
		return renderHookNotification(s, p, width, hook)
	}
	if compaction == CompactionDanger {
		return renderCompactionNotification(s, p, width)
	}
	if quota.Active {
		return renderQuotaBanner(s, p, width, quota)
	}
	return ""
}

// notificationHeight returns the number of rows the inline banner will
// occupy in the current state (3 rows + 1 separator newline = 4, or 0
// when nothing inline is shown — fullscreen mode also returns 0 because
// it replaces the body, not the inline strip).
func notificationHeight(style NotificationStyle, notifAck bool, hook HookNotification, compaction CompactionState, quota QuotaNotification) int {
	d := resolveNotification(style, notifAck, hook, compaction, quota)
	if !d.Active || d.Fullscreen {
		return 0
	}
	return 4
}

// renderQuotaBanner renders the inline banner for plan-quota / cost
// alerts. Same chrome as the compaction banner; the copy comes from
// the QuotaNotification headline + detail.
func renderQuotaBanner(s Styles, _ Palette, width int, quota QuotaNotification) string {
	inner := width - 2

	icon := s.NotifIcon.Render("◆")
	who := s.NotifIcon.Render("clyde:")
	msg := s.NotifText.Render(" " + quota.Headline)
	if quota.Detail != "" {
		msg += s.NotifText.Render(" — " + quota.Detail)
	}

	content := icon + " " + who + msg

	bs := borderCharStyle(s, true)
	topLine := bs.Render("╭") + bs.Render(strings.Repeat("─", inner)) + bs.Render("╮")
	row := bs.Render("│") + padLine(content, inner) + bs.Render("│")
	botLine := bs.Render("╰") + bs.Render(strings.Repeat("─", inner)) + bs.Render("╯")

	return topLine + "\n" + row + "\n" + botLine
}

// renderCompactionNotification renders the compaction-imminent warning banner.
// Format: ◆ clyde: claude is at >90% context — compaction imminent
//
// The "clyde:" prefix is intentional — this is clyde itself warning, not claude.
func renderCompactionNotification(s Styles, _ Palette, width int) string {
	inner := width - 2

	icon := s.NotifIcon.Render("◆")
	who := s.NotifIcon.Render("clyde:")
	msg := s.NotifText.Render(" context window is above 90% — compaction imminent")

	content := icon + " " + who + msg

	bs := borderCharStyle(s, true)
	topLine := bs.Render("╭") + bs.Render(strings.Repeat("─", inner)) + bs.Render("╮")
	row := bs.Render("│") + padLine(content, inner) + bs.Render("│")
	botLine := bs.Render("╰") + bs.Render(strings.Repeat("─", inner)) + bs.Render("╯")

	return topLine + "\n" + row + "\n" + botLine
}

// renderHookNotification renders a banner for a live hook event.
// Format: ◆ claude wants to run `<cmd>` in `<cwd>`   [y] [n] [esc]
func renderHookNotification(s Styles, _ Palette, width int, hook HookNotification) string {
	inner := width - 2

	// Build verb phrase based on tool type.
	var verb, cmdText, inPart, pathText string
	switch strings.ToLower(hook.Tool) {
	case "bash":
		verb = " wants to run "
		cmdText = truncate(hook.KeyArg, 40)
		inPart = " in "
		pathText = cwdDisplay(hook.Cwd)
	case "edit", "multiedit", "write":
		verb = " wants to edit "
		cmdText = filepath.Base(hook.KeyArg)
		inPart = " in "
		pathText = cwdDisplay(hook.Cwd)
	case "read":
		verb = " wants to read "
		cmdText = filepath.Base(hook.KeyArg)
		inPart = " in "
		pathText = cwdDisplay(hook.Cwd)
	default:
		verb = " wants to call "
		cmdText = hook.Tool
		if hook.KeyArg != "" {
			cmdText += " " + truncate(hook.KeyArg, 30)
		}
		inPart = " in "
		pathText = cwdDisplay(hook.Cwd)
	}

	icon := s.NotifIcon.Render("◆")
	who := s.NotifWho.Render("claude")
	verbR := s.NotifText.Render(verb)
	cmd := s.NotifCmd.Render(cmdText)
	inR := s.NotifText.Render(inPart)
	path := s.NotifPath.Render(pathText)

	msg := icon + " " + who + verbR + cmd + inR + path

	chip := func(label string) string {
		return "[" + s.NotifText.Render(label) + "]"
	}
	chips := chip("y") + " " + chip("n") + " " + chip("esc")

	msgW := ansiWidth(msg)
	chipsW := ansiWidth(chips)
	gapW := inner - msgW - chipsW
	if gapW < 2 {
		gapW = 2
	}

	content := msg + strings.Repeat(" ", gapW) + chips

	bs := borderCharStyle(s, true)
	topLine := bs.Render("╭") + bs.Render(strings.Repeat("─", inner)) + bs.Render("╮")
	row := bs.Render("│") + padLine(content, inner) + bs.Render("│")
	botLine := bs.Render("╰") + bs.Render(strings.Repeat("─", inner)) + bs.Render("╯")

	return topLine + "\n" + row + "\n" + botLine
}

// cwdDisplay formats a cwd path for display in the notification banner,
// shortening the home directory to "~".
func cwdDisplay(cwd string) string {
	if cwd == "" {
		return "~"
	}
	h := homeDir()
	if h != "" && strings.HasPrefix(cwd, h) {
		return "~" + cwd[len(h):]
	}
	return cwd
}
