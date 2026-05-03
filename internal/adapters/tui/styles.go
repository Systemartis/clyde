package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// blendColor returns a linear interpolation of two colors. alpha=0 returns
// `from` exactly, alpha=1 returns `to`. Mid values blend in 16-bit RGBA
// space (color.RGBA64) so the math stays precise across themes whose hex
// values are tightly packed in chroma.
//
// Used by Styles.WithFadedFocus to interpolate the focus chrome between
// its dim and bright equivalents during the focus-decay handoff.
func blendColor(from, to color.Color, alpha float64) color.Color {
	if alpha <= 0 {
		return from
	}
	if alpha >= 1 {
		return to
	}
	fr, fg, fb, fa := from.RGBA()
	tr, tg, tb, ta := to.RGBA()
	inv := 1 - alpha
	return color.RGBA64{
		R: uint16(float64(fr)*inv + float64(tr)*alpha),
		G: uint16(float64(fg)*inv + float64(tg)*alpha),
		B: uint16(float64(fb)*inv + float64(tb)*alpha),
		A: uint16(float64(fa)*inv + float64(ta)*alpha),
	}
}

// Styles holds all per-component lipgloss styles derived from a Palette.
// Never create lipgloss styles inline in rendering code — go through this
// struct so the theme stays consistent.
type Styles struct {
	// Panel chrome (normal border)
	Panel       lipgloss.Style
	PanelFocus  lipgloss.Style // Expanded-Passive: purple border
	PanelActive lipgloss.Style // Expanded-Active: pink double border

	// Panel label / metadata riding the border
	PanelLabel       lipgloss.Style
	PanelLabelFocus  lipgloss.Style // purple when focused (passive)
	PanelLabelActive lipgloss.Style // pink when in active mode
	PanelMeta        lipgloss.Style
	PanelModeBadge   lipgloss.Style // active-mode badge text (pink)

	// Collapsed panel one-liner summary
	PanelCollapsedLabel      lipgloss.Style // dim when blurred
	PanelCollapsedLabelFocus lipgloss.Style // purple when focused

	// Title bar
	TitleBar    lipgloss.Style
	TitleBrand  lipgloss.Style // "clyde" in purple
	TitlePath   lipgloss.Style // dim path
	TitleProjct lipgloss.Style // cyan project name
	TitleLive   lipgloss.Style // green "claude · live"
	TitleMeta   lipgloss.Style // dim meta labels
	TitleValue  lipgloss.Style // white values

	// Explorer
	SectionHeader lipgloss.Style // dim section label
	SectionCount  lipgloss.Style // fade count on right
	FileModMark   lipgloss.Style // M amber
	FileAddMark   lipgloss.Style // + green
	FileName      lipgloss.Style // mid text
	FileNameAct   lipgloss.Style // red — active file
	FileStats     lipgloss.Style // dim +/- counts
	TreeDir       lipgloss.Style // blue-ish dir color
	TreeIndent    lipgloss.Style // fade
	HintKey       lipgloss.Style // purple kbd hints
	HintText      lipgloss.Style // dim hint text

	// Now panel
	Mascot      lipgloss.Style // pink — bunny body
	MascotZZ    lipgloss.Style // dim — sleep zZ annotation
	NowOp       lipgloss.Style // purple operation line
	NowMeta     lipgloss.Style // dim meta
	NowCode     lipgloss.Style // cyan inline code
	NowDetail   lipgloss.Style // dim detail (e.g. · line 46)
	StatusGreen lipgloss.Style // green status dot
	StatusPurpl lipgloss.Style // purple status dot/chevron

	// Tasks panel
	TaskDoneIcon    lipgloss.Style // green ✓
	TaskDoneName    lipgloss.Style // muted strikethrough-ish
	TaskDur         lipgloss.Style // dim duration
	TaskSubtitle    lipgloss.Style // small dim subtitle
	TaskActBorder   lipgloss.Style // active task outer box
	TaskActName     lipgloss.Style // text active task name
	TaskActDur      lipgloss.Style // purple duration
	TaskActOp       lipgloss.Style // spinner+code line
	TaskActCode     lipgloss.Style // cyan code ref
	TaskSubDoneIcon lipgloss.Style
	TaskSubDoneName lipgloss.Style
	TaskSubActIcon  lipgloss.Style // purple
	TaskSubActName  lipgloss.Style
	TaskSubPenIcon  lipgloss.Style // fade
	TaskSubPenName  lipgloss.Style // dim
	TaskPenIcon     lipgloss.Style // fade □
	TaskPenName     lipgloss.Style // dim
	ProgPct         lipgloss.Style // purple %
	ProgLeft        lipgloss.Style // dim "N left"
	ProgBarBg       lipgloss.Style // dark bar bg

	// Diff panel
	DiffHunk    lipgloss.Style // dim italic @@ line
	DiffLineNum lipgloss.Style // fade line number
	DiffCtx     lipgloss.Style // mid context text
	DiffAdd     lipgloss.Style // green added text
	DiffRem     lipgloss.Style // red removed text

	// Usage panel
	UsageLabel   lipgloss.Style
	UsageValue   lipgloss.Style
	UsageModel   lipgloss.Style // purple model name
	UsageDivider lipgloss.Style
	IssueDot     lipgloss.Style // base
	IssueName    lipgloss.Style
	IssueValue   lipgloss.Style

	// Servers panel
	SrvSubHeader lipgloss.Style
	SrvName      lipgloss.Style
	SrvCount     lipgloss.Style
	SrvOff       lipgloss.Style // dim name when off

	// Semantic color shortcuts (for inline dot/accent usage)
	TextFade lipgloss.Style // #3b4261 gutters/disabled
	Amber    lipgloss.Style // #e0af68 warnings

	// Notification banner
	NotifIcon lipgloss.Style // purple ◆
	NotifWho  lipgloss.Style // pink "claude" — ONLY place pink is used on text
	NotifText lipgloss.Style
	NotifCmd  lipgloss.Style // amber code
	NotifPath lipgloss.Style // cyan path
	KbdChip   lipgloss.Style // keyboard chip [y]

	// Status bar
	StatusBar   lipgloss.Style
	StatusKey   lipgloss.Style // purple keybind
	StatusSep   lipgloss.Style // fade ·
	StatusVer   lipgloss.Style // dim version
	StatusReady lipgloss.Style // green "ready"

	// Tab strip (Mode B)
	TabActive   lipgloss.Style // active tab — purple text
	TabInactive lipgloss.Style // inactive tab — dim
	TabBorder   lipgloss.Style // tab separator
}

// NewStyles builds a Styles struct from a Palette.
func NewStyles(p Palette) Styles {
	return Styles{
		Panel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.BorderDim),
		PanelFocus: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.BorderAcc),
		PanelActive: lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(p.Magenta),

		PanelLabel: lipgloss.NewStyle().
			Foreground(p.TextDim),
		PanelLabelFocus: lipgloss.NewStyle().
			Foreground(p.Purple),
		PanelLabelActive: lipgloss.NewStyle().
			Foreground(p.Magenta).
			Bold(true),
		PanelMeta: lipgloss.NewStyle().
			Foreground(p.TextDim),
		PanelModeBadge: lipgloss.NewStyle().
			Foreground(p.Magenta),

		PanelCollapsedLabel: lipgloss.NewStyle().
			Foreground(p.TextDim),
		PanelCollapsedLabelFocus: lipgloss.NewStyle().
			Foreground(p.Purple),

		// Title bar
		TitleBar: lipgloss.NewStyle().
			Foreground(p.TextDim),
		TitleBrand: lipgloss.NewStyle().
			Foreground(p.Purple).
			Bold(true),
		TitlePath: lipgloss.NewStyle().
			Foreground(p.TextDim),
		TitleProjct: lipgloss.NewStyle().
			Foreground(p.Cyan).
			Bold(true),
		TitleLive: lipgloss.NewStyle().
			Foreground(p.Green).
			Bold(true),
		TitleMeta: lipgloss.NewStyle().
			Foreground(p.TextDim),
		TitleValue: lipgloss.NewStyle().
			Foreground(p.Text).
			Bold(true),

		// Explorer
		SectionHeader: lipgloss.NewStyle().
			Foreground(p.TextDim),
		SectionCount: lipgloss.NewStyle().
			Foreground(p.TextFade),
		FileModMark: lipgloss.NewStyle().
			Foreground(p.Amber),
		FileAddMark: lipgloss.NewStyle().
			Foreground(p.Green),
		FileName: lipgloss.NewStyle().
			Foreground(p.TextMid),
		FileNameAct: lipgloss.NewStyle().
			Foreground(p.Red),
		FileStats: lipgloss.NewStyle().
			Foreground(p.TextDim),
		TreeDir: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7aa2f7")),
		TreeIndent: lipgloss.NewStyle().
			Foreground(p.TextFade),
		HintKey: lipgloss.NewStyle().
			Foreground(p.Purple),
		HintText: lipgloss.NewStyle().
			Foreground(p.TextDim),

		// Now panel
		Mascot: lipgloss.NewStyle().
			Foreground(p.Pink),
		MascotZZ: lipgloss.NewStyle().
			Foreground(p.TextMid),
		NowOp: lipgloss.NewStyle().
			Foreground(p.Purple).
			Bold(true),
		NowMeta: lipgloss.NewStyle().
			Foreground(p.TextDim),
		NowCode: lipgloss.NewStyle().
			Foreground(p.Cyan),
		NowDetail: lipgloss.NewStyle().
			Foreground(p.TextDim),
		StatusGreen: lipgloss.NewStyle().
			Foreground(p.Green),
		StatusPurpl: lipgloss.NewStyle().
			Foreground(p.Purple),

		// Tasks panel
		TaskDoneIcon: lipgloss.NewStyle().
			Foreground(p.Green),
		TaskDoneName: lipgloss.NewStyle().
			Foreground(p.TextFade),
		TaskDur: lipgloss.NewStyle().
			Foreground(p.TextDim),
		TaskSubtitle: lipgloss.NewStyle().
			Foreground(p.TextDim),
		TaskActName: lipgloss.NewStyle().
			Foreground(p.Text).
			Bold(true),
		TaskActDur: lipgloss.NewStyle().
			Foreground(p.Purple),
		TaskActOp: lipgloss.NewStyle().
			Foreground(p.TextMid),
		TaskActCode: lipgloss.NewStyle().
			Foreground(p.Cyan),
		TaskSubDoneIcon: lipgloss.NewStyle().
			Foreground(p.Green),
		TaskSubDoneName: lipgloss.NewStyle().
			Foreground(p.TextFade),
		TaskSubActIcon: lipgloss.NewStyle().
			Foreground(p.Purple),
		TaskSubActName: lipgloss.NewStyle().
			Foreground(p.Text),
		TaskSubPenIcon: lipgloss.NewStyle().
			Foreground(p.TextFade),
		TaskSubPenName: lipgloss.NewStyle().
			Foreground(p.TextDim),
		TaskPenIcon: lipgloss.NewStyle().
			Foreground(p.TextFade),
		TaskPenName: lipgloss.NewStyle().
			Foreground(p.TextDim),
		ProgPct: lipgloss.NewStyle().
			Foreground(p.Purple).
			Bold(true),
		ProgLeft: lipgloss.NewStyle().
			Foreground(p.TextDim),
		ProgBarBg: lipgloss.NewStyle().
			Foreground(p.TextFade),

		// Diff panel
		DiffHunk: lipgloss.NewStyle().
			Foreground(p.TextDim).
			Italic(true),
		DiffLineNum: lipgloss.NewStyle().
			Foreground(p.TextFade),
		DiffCtx: lipgloss.NewStyle().
			Foreground(p.TextMid),
		DiffAdd: lipgloss.NewStyle().
			Foreground(p.Green),
		DiffRem: lipgloss.NewStyle().
			Foreground(p.Red),

		// Usage panel
		UsageLabel: lipgloss.NewStyle().
			Foreground(p.TextDim),
		UsageValue: lipgloss.NewStyle().
			Foreground(p.Text).
			Bold(true),
		UsageModel: lipgloss.NewStyle().
			Foreground(p.Purple),
		UsageDivider: lipgloss.NewStyle().
			Foreground(p.TextFade),
		IssueName: lipgloss.NewStyle().
			Foreground(p.TextMid),
		IssueValue: lipgloss.NewStyle().
			Foreground(p.Text),

		// Servers
		SrvSubHeader: lipgloss.NewStyle().
			Foreground(p.TextDim),
		SrvName: lipgloss.NewStyle().
			Foreground(p.Text),
		SrvCount: lipgloss.NewStyle().
			Foreground(p.TextDim),
		SrvOff: lipgloss.NewStyle().
			Foreground(p.TextDim),

		// Notification
		NotifIcon: lipgloss.NewStyle().
			Foreground(p.Purple),
		NotifWho: lipgloss.NewStyle().
			Foreground(p.Pink).
			Bold(true),
		NotifText: lipgloss.NewStyle().
			Foreground(p.Text),
		NotifCmd: lipgloss.NewStyle().
			Foreground(p.Amber),
		NotifPath: lipgloss.NewStyle().
			Foreground(p.Cyan),
		KbdChip: lipgloss.NewStyle().
			Foreground(p.Text).
			Border(lipgloss.NormalBorder()).
			BorderForeground(p.TextFade).
			Padding(0, 1),

		// Semantic shortcuts
		TextFade: lipgloss.NewStyle().Foreground(p.TextFade),
		Amber:    lipgloss.NewStyle().Foreground(p.Amber),

		// Status bar
		StatusBar: lipgloss.NewStyle().
			Foreground(p.TextDim),
		StatusKey: lipgloss.NewStyle().
			Foreground(p.Purple),
		StatusSep: lipgloss.NewStyle().
			Foreground(p.TextFade),
		StatusVer: lipgloss.NewStyle().
			Foreground(p.TextDim),
		StatusReady: lipgloss.NewStyle().
			Foreground(p.Green).
			Bold(true),

		// Tab strip
		TabActive: lipgloss.NewStyle().
			Foreground(p.Purple).
			Bold(true),
		TabInactive: lipgloss.NewStyle().
			Foreground(p.TextDim),
		TabBorder: lipgloss.NewStyle().
			Foreground(p.TextFade),
	}
}

// WithFadedFocus returns a copy of s with the focus-chrome foregrounds
// interpolated toward their dim equivalents based on alpha.
//
//	alpha=1 → unchanged (full purple/BorderAcc on focused panels)
//	alpha=0 → focus colors collapse to dim/TextDim equivalents
//	in between → blendColor across BorderDim↔BorderAcc and TextDim↔Purple
//
// Only the chrome that distinguishes Passive (focused) from Normal flips —
// PanelStateActive / PanelLabelActive / PanelMeta etc are left alone so
// active mode and meta text don't get caught up in the fade.
//
// Used by render_stack/2col/multicol when the focus-decay handoff is in
// flight: the previously-focused panel's border tints purple → gray over
// 3s and the now-panel's border tints gray → purple over the next 3s.
func (s Styles) WithFadedFocus(p Palette, alpha float64) Styles {
	if alpha >= 1 {
		return s
	}
	if alpha < 0 {
		alpha = 0
	}
	border := blendColor(p.BorderDim, p.BorderAcc, alpha)
	label := blendColor(p.TextDim, p.Purple, alpha)
	out := s
	out.PanelFocus = out.PanelFocus.BorderForeground(border)
	out.PanelLabelFocus = out.PanelLabelFocus.Foreground(label)
	out.PanelCollapsedLabelFocus = out.PanelCollapsedLabelFocus.Foreground(label)
	return out
}
