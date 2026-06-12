package tui

import "time"

// ─── Mock data types ──────────────────────────────────────────────────────────

// ModifiedFile is a file changed during the session.
type ModifiedFile struct {
	Mark  string // "M" or "+"
	Path  string
	Stats string // "+14 −3" etc.
}

// TreeNode is a single line in the file tree view.
type TreeNode struct {
	Indent   string // indentation prefix using │ characters
	Mark     string // "M", "+", " "
	IsDir    bool   // renders with dir color
	IsAct    bool   // currently active file (red highlight)
	Name     string
	FullPath string // optional full relative path (used by viewer for mock lookup)
}

// TaskStatus describes a task's completion state.
type TaskStatus int

// Task status constants.
const (
	TaskDone    TaskStatus = iota // completed task
	TaskActive                    // currently running task
	TaskPending                   // not yet started task
)

// SubStep is a step within the active task.
type SubStep struct {
	Done   bool
	Active bool
	Name   string
	Dur    string // "0:08"
}

// Task is a single entry in the tasks panel.
type Task struct {
	Status   TaskStatus
	Icon     string // "✓", "▸", "□"
	Name     string
	Duration string
	Subtitle string    // shown for done tasks
	SubSteps []SubStep // only for active task
}

// DiffKind describes the type of a diff line.
type DiffKind int

// Diff line kind constants.
const (
	DiffCtxKind  DiffKind = iota // context line
	DiffAddKind                  // added line
	DiffRemKind                  // removed line
	DiffHunkKind                 // @@ header line
)

// DiffLine is a rendered diff line.
type DiffLine struct {
	Kind   DiffKind
	LineNo string
	Text   string
}

// ServerStatus is the state of an MCP/LSP server.
type ServerStatus int

// Server status constants.
const (
	ServerOn   ServerStatus = iota // server is active
	ServerBusy                     // server is busy (amber)
	ServerOff                      // server is offline
)

// Server is one entry in the servers panel.
type Server struct {
	Status ServerStatus
	Name   string
	Count  string // tool count for MCPs; empty for LSPs
}

// UsageWindowRow holds the pre-formatted display strings for one usage time window
// (session / 5h / weekly).  All fields are strings to keep rendering trivial.
type UsageWindowRow struct {
	// Label is the window name shown in the left column: "session ctx", "5h usage", "weekly usage".
	Label string

	// Percent is the 0-100 fill ratio for this row's progress bar.
	//   - session row: % of model context window used (compaction risk).
	//   - 5h / weekly rows: % of plan quota consumed (when IsPlanQuota=true)
	//     OR % of time elapsed in window (when IsPlanQuota=false fallback).
	Percent int

	// IsPlanQuota is true when Percent comes from the Anthropic plan-usage
	// API (real plan-quota %), false when it's a time-elapsed approximation
	// derived from JSONL timestamps. Used by the panel to decide whether
	// to show the "(plan offline)" badge.
	IsPlanQuota bool

	// Input is the formatted input-token count, e.g. "12k".
	Input string

	// Output is the formatted output-token count, e.g. "5k".
	Output string

	// Cache is the formatted cache-read token count, e.g. "30k".
	Cache string

	// Cost is the formatted USD cost, e.g. "$0.42".
	Cost string

	// TotalUsed is the formatted total token count for the window, e.g. "3.4M".
	// Used in the two-line per-window display as the "total used" line.
	TotalUsed string

	// CurrentCtx is the "Nk / 1M (N%)" string for the current context window.
	// Only populated for the session row (LatestUsage context, not TotalUsage).
	// Empty for 5h and 7d rows.
	CurrentCtx string

	// ResetsIn is the human-readable countdown until the window resets,
	// e.g. "3h 57m" for the 5h window, "4d 14h" for the 7d window.
	// Empty when the reset time is unknown or has already passed.
	ResetsIn string

	// ResetAt is the wall-clock time when this window resets. Zero when
	// unknown (e.g. hand-authored demo rows). Used to pick the SOONEST
	// reset for the synthetic "next reset" row.
	ResetAt time.Time

	// ResetElapsedPct is the 0-100 percentage of TIME elapsed in this
	// window (0 = just reset, 100 = about to reset). This — not Percent,
	// which may hold the plan-quota % — feeds the "next reset" bar so its
	// fill always matches the countdown headline.
	ResetElapsedPct int

	// Empty is true when all token counts are zero (nothing to display).
	Empty bool

	// SessionCount is the number of sessions that contributed to this row.
	// Zero means "not available" (single-project fallback or this session row).
	// Positive values are formatted as "(N sessions)" in the display.
	SessionCount int
}

// MockData is the complete v3 mock dataset.
type MockData struct {
	// Title bar
	ProjectPath string
	ProjectName string
	Duration    string
	Tokens      string
	Cost        string

	// Explorer
	ModifiedFiles []ModifiedFile
	Tree          []TreeNode

	// Now panel
	NowMode    string // "writing · 47 t/s"
	NowOp      string // "edit auth.ts"
	NowMeta    string // "14 lines staged · line 46"
	NowTS      bool   // ts done
	NowLint    bool   // lint done
	NowCompile bool   // compile active (not done)

	// Calls panel (replaces tasks panel in v13)
	AgentGroups []AgentGroup

	// Diff panel
	DiffFile  string // "auth.ts · +14 −3"
	DiffLines []DiffLine
	IsGitRepo bool // true when cwd contains a .git directory

	// Usage panel
	Tokens200k int    // used out of 200k
	TokenPct   int    // 23
	Cost142    string // "$1.42"
	Turns      string // "38"
	Model      string // "opus 4.7"
	Errors     string
	Warnings   string
	Tests      string

	// Multi-window usage rows with per-token-type breakdown.
	// Each row shows label · in · out · cache · cost for one time window.
	UsageSession UsageWindowRow // this session
	Usage5h      UsageWindowRow // rolling last 5 hours
	UsageWeek    UsageWindowRow // rolling last 7 days

	// BurnRate is a derived display string shown below the usage rows:
	// e.g. "~3.2k tok/min · $0.18/hr". Empty when there is too little data.
	BurnRate string

	// PlanTier is the human-readable subscription plan tier
	// (e.g. "Max 5x", "Pro"). Empty when unknown OR when the user is
	// on API auth (in which case IsAPIUser is true and renderers show
	// "api" via that flag instead of trying to fake a tier here).
	PlanTier string

	// IsAPIUser is true when the plan-usage source returned
	// ErrPlanUsageUnavailable — the user has no subscription and is
	// paying per-token via API. Used to gate the cost-threshold
	// notification (subscribers' "cost" is mostly cosmetic) and to
	// display "api" as the tier label so the user can see at a glance
	// what mode they're in.
	IsAPIUser bool

	// PlanUsageOffline is true when the plan-usage API is unreachable or
	// the credentials are present but the request failed. Distinct from
	// IsAPIUser — offline means "we expected data and didn't get it",
	// API means "no subscription credentials by design".
	PlanUsageOffline bool

	// Servers panel
	ServersOn    int // 5
	ServersTotal int // 7
	MCPs         []Server
	LSPs         []Server

	// Bash audit panel (v22+, opt-in via settings)
	BashLog []BashRow

	// Cache efficiency panel (v22+, opt-in via settings)
	Cache CacheStatsView

	// Multi-session tab strip — one entry per session in the current cwd
	// (Σ aggregate first when populated). Populated from
	// livesession.View.SessionStats; empty in demo mode and when only a
	// single session is recently active.
	Sessions []SessionTab

	// SessionTabIndex is the index of the active tab in Sessions. -1 means
	// the Σ aggregate tab is active; 0..len(Sessions)-1 selects a specific
	// session. Default 0 (most-recently-active session).
	SessionTabIndex int

	// SessionLeaderboard mirrors Sessions for the usage panel: top sessions
	// by ContextPct desc, used to render per-session ctx bars when the Σ
	// tab is active. Empty when the panel should render its single-session
	// fallback bar.
	SessionLeaderboard []SessionTab
}

// CacheStatsView mirrors livesession.CacheStats for the TUI layer.
type CacheStatsView struct {
	HitRatio          float64
	FromCache         int64
	Recomputed        int64
	BiggestMissTokens int64
	BiggestMissAt     string // formatted "HH:MM"
	Trend             []float64
	TurnCount         int
}

// BashRow is one Bash command entry rendered in the bash audit panel.
type BashRow struct {
	Time     string    // "14:32:08" — short time-of-day
	Command  string    // "go test ./auth/..."
	Duration string    // "12s" / "<1s" / "" when running
	State    CallState // CallDone / CallActive / CallFailed
}

// V3MockData returns the authoritative v3 mock content matching v3-mock.html.
func V3MockData() MockData {
	return MockData{
		ProjectPath: "~/projects/",
		ProjectName: "claude-companion",
		Duration:    "1h 24m",
		Tokens:      "47k",
		Cost:        "$1.42",

		ModifiedFiles: []ModifiedFile{
			{Mark: "M", Path: "api/auth.ts", Stats: "+14 −3"},
			{Mark: "M", Path: "ui/Sidebar.tsx", Stats: "+28 −12"},
			{Mark: "+", Path: "ui/Mascot.tsx", Stats: "+47"},
			{Mark: "M", Path: "package.json", Stats: "+2"},
		},

		Tree: []TreeNode{
			{Indent: "", Mark: " ", IsDir: true, Name: "▼ src"},
			{Indent: "│ ", Mark: " ", IsDir: true, Name: "▼ ui"},
			{Indent: "│ │ ", Mark: "M", Name: "Sidebar.tsx", FullPath: "src/ui/Sidebar.tsx"},
			{Indent: "│ │ ", Mark: "+", Name: "Mascot.tsx", FullPath: "src/ui/Mascot.tsx"},
			{Indent: "│ │ ", Mark: " ", Name: "Header.tsx", FullPath: "src/ui/Header.tsx"},
			{Indent: "│ │ ", Mark: " ", Name: "Button.tsx", FullPath: "src/ui/Button.tsx"},
			{Indent: "│ ", Mark: " ", IsDir: true, Name: "▼ api"},
			{Indent: "│ │ ", Mark: " ", IsAct: true, Name: "▸ auth.ts", FullPath: "src/api/auth.ts"},
			{Indent: "│ │ ", Mark: " ", Name: "routes.ts", FullPath: "src/api/routes.ts"},
			{Indent: "│ │ ", Mark: " ", Name: "middleware.ts", FullPath: "src/api/middleware.ts"},
			{Indent: "│ ", Mark: " ", Name: "index.ts", FullPath: "src/index.ts"},
			{Indent: "│ ", Mark: " ", Name: "App.tsx", FullPath: "src/App.tsx"},
			{Indent: "", Mark: " ", IsDir: true, Name: "▼ tests"},
			{Indent: "│ ", Mark: " ", Name: "auth.test.ts", FullPath: "tests/auth.test.ts"},
			{Indent: "│ ", Mark: " ", Name: "routes.test.ts", FullPath: "tests/routes.test.ts"},
			{Indent: "", Mark: " ", IsDir: true, Name: "▼ public"},
			{Indent: "│ ", Mark: " ", Name: "logo.png", FullPath: "public/logo.png"},
			{Indent: "", Mark: " ", Name: "README.md", FullPath: "README.md"},
			{Indent: "", Mark: "M", Name: "package.json", FullPath: "package.json"},
			{Indent: "", Mark: " ", Name: "tsconfig.json", FullPath: "tsconfig.json"},
			{Indent: "", Mark: " ", Name: ".gitignore", FullPath: ".gitignore"},
			{Indent: "", Mark: " ", Name: "vite.config.ts", FullPath: "vite.config.ts"},
		},

		NowMode:    "writing · 47 t/s",
		NowOp:      "edit auth.ts",
		NowMeta:    "14 lines staged · line 46",
		NowTS:      true,
		NowLint:    true,
		NowCompile: false,

		AgentGroups: []AgentGroup{
			{
				AgentID:   "main",
				AgentName: "main session",
				Active:    true,
				Calls: []ToolCall{
					{Tool: "Read", KeyArg: "/clyde/internal/adapters/tui/proto/auth.ts", Duration: "12s", State: CallDone},
					{Tool: "Edit", KeyArg: "/clyde/internal/adapters/tui/proto/auth.ts", Duration: "4s", State: CallDone},
					{Tool: "Bash", KeyArg: "'go test ./auth/...'", Duration: "18s", State: CallDone},
					{Tool: "Read", KeyArg: "/clyde/internal/adapters/tui/proto/middleware.ts", Duration: "3s", State: CallActive},
				},
			},
			{
				AgentID:    "explore-content",
				AgentName:  "explore-content",
				IsSubagent: true,
				Active:     false,
				Calls: []ToolCall{
					{Tool: "Read", KeyArg: "docs/api-spec.md", Duration: "2s", State: CallDone},
					{Tool: "Grep", KeyArg: "'verifyToken' --type go", Duration: "1s", State: CallDone},
					{Tool: "Read", KeyArg: "internal/handler.go", Duration: "3s", State: CallDone},
				},
			},
			{
				AgentID:    "propose-rendering",
				AgentName:  "propose-rendering",
				IsSubagent: true,
				Active:     false,
				Calls: []ToolCall{
					{Tool: "Read", KeyArg: "openspec/changes/event-rendering/spec.md", Duration: "2s", State: CallDone},
					{Tool: "Read", KeyArg: "internal/adapters/jsonl/jsonl.go", Duration: "4s", State: CallDone},
					{Tool: "Edit", KeyArg: "openspec/changes/event-rendering/proposal.md", Duration: "1s", State: CallActive},
				},
			},
			{
				AgentID:    "run-tests-batch",
				AgentName:  "run-tests-batch",
				IsSubagent: true,
				Active:     true,
				Calls: []ToolCall{
					{Tool: "Bash", KeyArg: "'go test ./domain/...'", Duration: "21s", State: CallDone},
					{Tool: "Bash", KeyArg: "'go test ./adapters/...'", Duration: "11s", State: CallActive},
				},
			},
			{
				AgentID:    "compaction-check",
				AgentName:  "compaction-check",
				IsSubagent: true,
				Active:     false,
				Calls:      []ToolCall{},
			},
		},

		IsGitRepo: true,
		DiffFile:  "auth.ts · +28 −6",
		DiffLines: []DiffLine{
			{Kind: DiffHunkKind, Text: "@@ −12,5 +12,9 @@ import { verify } from './jwt'"},
			{Kind: DiffCtxKind, LineNo: "12", Text: "import { db } from './database';"},
			{Kind: DiffCtxKind, LineNo: "13", Text: "import { logger } from './logger';"},
			{Kind: DiffAddKind, LineNo: "14", Text: "+ import { TokenError } from './errors';"},
			{Kind: DiffAddKind, LineNo: "15", Text: "+ import { now } from './time';"},
			{Kind: DiffCtxKind, LineNo: "16", Text: ""},
			{Kind: DiffHunkKind, Text: "@@ −42,7 +46,18 @@ authenticate(token)"},
			{Kind: DiffCtxKind, LineNo: "46", Text: "  if (!token) {"},
			{Kind: DiffRemKind, LineNo: "47", Text: "−   throw new Error('no token');"},
			{Kind: DiffAddKind, LineNo: "47", Text: "+   return { ok: false, reason: 'missing' };"},
			{Kind: DiffCtxKind, LineNo: "48", Text: "  }"},
			{Kind: DiffAddKind, LineNo: "49", Text: "+ const decoded = await verify(token);"},
			{Kind: DiffAddKind, LineNo: "50", Text: "+ if (!decoded?.sub) {"},
			{Kind: DiffAddKind, LineNo: "51", Text: "+   return { ok: false, reason: 'expired' };"},
			{Kind: DiffAddKind, LineNo: "52", Text: "+ }"},
			{Kind: DiffCtxKind, LineNo: "53", Text: "  return { ok: true, user: decoded };"},
			{Kind: DiffCtxKind, LineNo: "54", Text: "}"},
			{Kind: DiffHunkKind, Text: "@@ −67,4 +83,14 @@ refreshToken()"},
			{Kind: DiffCtxKind, LineNo: "83", Text: "  const stored = await db.get(uid);"},
			{Kind: DiffRemKind, LineNo: "84", Text: "−   return stored.token;"},
			{Kind: DiffRemKind, LineNo: "85", Text: "−   // TODO: check expiry"},
			{Kind: DiffAddKind, LineNo: "84", Text: "+ if (!stored?.refreshExp) return null;"},
			{Kind: DiffAddKind, LineNo: "85", Text: "+ if (stored.refreshExp < now()) {"},
			{Kind: DiffAddKind, LineNo: "86", Text: "+   await db.del(uid);"},
			{Kind: DiffAddKind, LineNo: "87", Text: "+   return null;"},
			{Kind: DiffAddKind, LineNo: "88", Text: "+ }"},
			{Kind: DiffCtxKind, LineNo: "89", Text: "  return stored.token;"},
			{Kind: DiffCtxKind, LineNo: "90", Text: "}"},
			{Kind: DiffHunkKind, Text: "@@ −102,3 +108,7 @@ revokeToken(uid)"},
			{Kind: DiffCtxKind, LineNo: "108", Text: "  const entry = await db.get(uid);"},
			{Kind: DiffRemKind, LineNo: "109", Text: "−   db.del(uid);"},
			{Kind: DiffAddKind, LineNo: "109", Text: "+ if (!entry) return false;"},
			{Kind: DiffAddKind, LineNo: "110", Text: "+ await db.del(uid);"},
			{Kind: DiffAddKind, LineNo: "111", Text: "+ logger.info('revoked', uid);"},
			{Kind: DiffAddKind, LineNo: "112", Text: "+ return true;"},
			{Kind: DiffCtxKind, LineNo: "113", Text: "}"},
		},

		Tokens200k: 47200,
		TokenPct:   23,
		Cost142:    "$1.42",
		Turns:      "38",
		Model:      "opus 4.7",
		Errors:     "0",
		Warnings:   "2",
		Tests:      "18 / 18",
		UsageSession: UsageWindowRow{
			Label:      "session ctx",
			Input:      "12k",
			Output:     "5k",
			Cache:      "30k",
			Cost:       "$1.42",
			TotalUsed:  "47.2k",
			CurrentCtx: "47k / 200k (23%)",
			Percent:    23,
			// Session ctx is always real (model context window fill)
			// — not a plan-quota metric, but Percent here is authoritative.
			IsPlanQuota: false,
		},
		Usage5h: UsageWindowRow{
			Label:       "5h session",
			Input:       "48k",
			Output:      "18k",
			Cache:       "120k",
			Cost:        "$2.83",
			TotalUsed:   "186k",
			ResetsIn:    "1h 31m",
			Percent:     49, // mirrors user's screenshot: "Current session 49% used"
			IsPlanQuota: true,
			// 1h 31m left of 5h → ~70% of the window elapsed.
			ResetElapsedPct: 70,
		},
		UsageWeek: UsageWindowRow{
			Label:       "weekly · all models",
			Input:       "380k",
			Output:      "142k",
			Cache:       "980k",
			Cost:        "$14.30",
			TotalUsed:   "1.5M",
			ResetsIn:    "2d 7h",
			Percent:     79, // mirrors screenshot: "All models 79% used"
			IsPlanQuota: true,
			// 2d 7h left of 7d → ~67% of the window elapsed.
			ResetElapsedPct: 67,
		},
		BurnRate:     "~3.2k tok/min · $0.18/hr",
		PlanTier:     "Max 5x",
		ServersOn:    5,
		ServersTotal: 7,
		MCPs: []Server{
			{Status: ServerOn, Name: "filesystem", Count: "12"},
			{Status: ServerOn, Name: "github", Count: "8"},
			{Status: ServerOn, Name: "memory", Count: "3"},
			{Status: ServerOff, Name: "playwright"},
		},
		LSPs: []Server{
			{Status: ServerOn, Name: "tsserver"},
			{Status: ServerOn, Name: "rust-analyzer"},
			{Status: ServerBusy, Name: "pylsp"},
		},
		Cache: CacheStatsView{
			HitRatio:          0.87,
			FromCache:         3_200_000,
			Recomputed:        478_000,
			BiggestMissTokens: 210_000,
			BiggestMissAt:     "14:32",
			Trend:             []float64{0.62, 0.71, 0.78, 0.83, 0.87, 0.92, 0.94, 0.91, 0.88, 0.87},
			TurnCount:         38,
		},
		BashLog: []BashRow{
			{Time: "14:32:08", Command: "npm test", Duration: "12s", State: CallDone},
			{Time: "14:32:21", Command: "rg -n 'TODO' src/", Duration: "<1s", State: CallDone},
			{Time: "14:32:22", Command: "git diff --stat", Duration: "<1s", State: CallDone},
			{Time: "14:33:04", Command: "npm run build", Duration: "4s", State: CallFailed},
			{Time: "14:33:09", Command: "npm run lint", Duration: "2s", State: CallDone},
			{Time: "14:34:12", Command: "go test ./domain/...", Duration: "21s", State: CallDone},
			{Time: "14:34:35", Command: "go test ./adapters/...", Duration: "11s", State: CallActive},
		},
	}
}
