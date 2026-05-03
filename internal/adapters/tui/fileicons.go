package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// FileIcon describes a single-cell glyph plus its accent color for a file type.
// Glyphs are chosen from Unicode Geometric Shapes / Block Elements so they
// render in any modern terminal font — no Nerd Font dependency.
type FileIcon struct {
	Glyph string
	Color color.Color
}

// iconCode is the FILLED DIAMOND used for source-code files.
const (
	iconCode    = "◆"
	iconConfig  = "◇"
	iconDoc     = "▤"
	iconImage   = "▦"
	iconArchive = "▣"
	iconBinary  = "▪"
	iconDefault = "·"
)

// fileIconByExt maps lowercase extensions (with leading dot) to a FileIcon.
// Colors mirror the Tokyo Night palette and approximate language brand colors
// where it makes sense — Go/TS get cyan, configs get amber, etc.
var fileIconByExt = map[string]FileIcon{
	// ── code ──────────────────────────────────────────────────────────────
	".go":     {iconCode, lipgloss.Color("#7dcfff")},
	".ts":     {iconCode, lipgloss.Color("#7dcfff")},
	".tsx":    {iconCode, lipgloss.Color("#7dcfff")},
	".js":     {iconCode, lipgloss.Color("#e0af68")},
	".jsx":    {iconCode, lipgloss.Color("#e0af68")},
	".mjs":    {iconCode, lipgloss.Color("#e0af68")},
	".cjs":    {iconCode, lipgloss.Color("#e0af68")},
	".py":     {iconCode, lipgloss.Color("#7dcfff")},
	".rs":     {iconCode, lipgloss.Color("#ff9e64")},
	".rb":     {iconCode, lipgloss.Color("#f7768e")},
	".java":   {iconCode, lipgloss.Color("#ff9e64")},
	".kt":     {iconCode, lipgloss.Color("#bb9af7")},
	".swift":  {iconCode, lipgloss.Color("#ff9e64")},
	".c":      {iconCode, lipgloss.Color("#7dcfff")},
	".h":      {iconCode, lipgloss.Color("#7dcfff")},
	".cc":     {iconCode, lipgloss.Color("#7dcfff")},
	".cpp":    {iconCode, lipgloss.Color("#7dcfff")},
	".hpp":    {iconCode, lipgloss.Color("#7dcfff")},
	".cs":     {iconCode, lipgloss.Color("#bb9af7")},
	".php":    {iconCode, lipgloss.Color("#bb9af7")},
	".lua":    {iconCode, lipgloss.Color("#7dcfff")},
	".elm":    {iconCode, lipgloss.Color("#7dcfff")},
	".ex":     {iconCode, lipgloss.Color("#bb9af7")},
	".exs":    {iconCode, lipgloss.Color("#bb9af7")},
	".vue":    {iconCode, lipgloss.Color("#9ece6a")},
	".svelte": {iconCode, lipgloss.Color("#ff9e64")},

	// ── shell / scripts ───────────────────────────────────────────────────
	".sh":   {iconCode, lipgloss.Color("#9ece6a")},
	".bash": {iconCode, lipgloss.Color("#9ece6a")},
	".zsh":  {iconCode, lipgloss.Color("#9ece6a")},
	".fish": {iconCode, lipgloss.Color("#9ece6a")},

	// ── markup / styling ──────────────────────────────────────────────────
	".html": {iconCode, lipgloss.Color("#ff9e64")},
	".htm":  {iconCode, lipgloss.Color("#ff9e64")},
	".css":  {iconCode, lipgloss.Color("#bb9af7")},
	".scss": {iconCode, lipgloss.Color("#bb9af7")},
	".sass": {iconCode, lipgloss.Color("#bb9af7")},
	".less": {iconCode, lipgloss.Color("#bb9af7")},

	// ── config / data ─────────────────────────────────────────────────────
	".json":      {iconConfig, lipgloss.Color("#e0af68")},
	".yaml":      {iconConfig, lipgloss.Color("#ff9e64")},
	".yml":       {iconConfig, lipgloss.Color("#ff9e64")},
	".toml":      {iconConfig, lipgloss.Color("#f7768e")},
	".ini":       {iconConfig, lipgloss.Color("#a9b1d6")},
	".env":       {iconConfig, lipgloss.Color("#e0af68")},
	".xml":       {iconConfig, lipgloss.Color("#ff9e64")},
	".sql":       {iconConfig, lipgloss.Color("#7dcfff")},
	".graphql":   {iconConfig, lipgloss.Color("#f7768e")},
	".proto":     {iconConfig, lipgloss.Color("#7dcfff")},
	".lock":      {iconConfig, lipgloss.Color("#565f89")},
	".mod":       {iconConfig, lipgloss.Color("#7dcfff")},
	".sum":       {iconConfig, lipgloss.Color("#565f89")},
	".gitignore": {iconConfig, lipgloss.Color("#565f89")},

	// ── docs / text ───────────────────────────────────────────────────────
	".md":       {iconDoc, lipgloss.Color("#9ece6a")},
	".markdown": {iconDoc, lipgloss.Color("#9ece6a")},
	".txt":      {iconDoc, lipgloss.Color("#a9b1d6")},
	".rst":      {iconDoc, lipgloss.Color("#a9b1d6")},
	".pdf":      {iconDoc, lipgloss.Color("#f7768e")},

	// ── images ────────────────────────────────────────────────────────────
	".png":  {iconImage, lipgloss.Color("#ff75a0")},
	".jpg":  {iconImage, lipgloss.Color("#ff75a0")},
	".jpeg": {iconImage, lipgloss.Color("#ff75a0")},
	".gif":  {iconImage, lipgloss.Color("#ff75a0")},
	".webp": {iconImage, lipgloss.Color("#ff75a0")},
	".svg":  {iconImage, lipgloss.Color("#e0af68")},
	".ico":  {iconImage, lipgloss.Color("#ff75a0")},

	// ── archives / binary ─────────────────────────────────────────────────
	".zip": {iconArchive, lipgloss.Color("#bb9af7")},
	".tar": {iconArchive, lipgloss.Color("#bb9af7")},
	".gz":  {iconArchive, lipgloss.Color("#bb9af7")},
	".tgz": {iconArchive, lipgloss.Color("#bb9af7")},
	".bz2": {iconArchive, lipgloss.Color("#bb9af7")},
	".xz":  {iconArchive, lipgloss.Color("#bb9af7")},
	".7z":  {iconArchive, lipgloss.Color("#bb9af7")},
	".rar": {iconArchive, lipgloss.Color("#bb9af7")},
}

// fileIconByExactName lets us match icons by the full filename rather than the
// extension — useful for special files like Makefile, Dockerfile, README.
var fileIconByExactName = map[string]FileIcon{
	"Dockerfile":         {iconConfig, lipgloss.Color("#7dcfff")},
	"docker-compose.yml": {iconConfig, lipgloss.Color("#7dcfff")},
	"Makefile":           {iconConfig, lipgloss.Color("#a9b1d6")},
	"makefile":           {iconConfig, lipgloss.Color("#a9b1d6")},
	"README":             {iconDoc, lipgloss.Color("#9ece6a")},
	"README.md":          {iconDoc, lipgloss.Color("#9ece6a")},
	"LICENSE":            {iconDoc, lipgloss.Color("#565f89")},
	"CHANGELOG":          {iconDoc, lipgloss.Color("#9ece6a")},
	".gitignore":         {iconConfig, lipgloss.Color("#565f89")},
	".dockerignore":      {iconConfig, lipgloss.Color("#565f89")},
	".editorconfig":      {iconConfig, lipgloss.Color("#a9b1d6")},
	".golangci.yml":      {iconConfig, lipgloss.Color("#7dcfff")},
	"go.mod":             {iconConfig, lipgloss.Color("#7dcfff")},
	"go.sum":             {iconConfig, lipgloss.Color("#565f89")},
	"package.json":       {iconConfig, lipgloss.Color("#9ece6a")},
	"tsconfig.json":      {iconConfig, lipgloss.Color("#7dcfff")},
}

// iconForFile returns the icon for a file's basename (e.g. "auth.ts" or "Makefile").
// Falls back to a neutral dot when no specific match exists.
func iconForFile(name string) FileIcon {
	base := name
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		base = name[idx+1:]
	}
	if ic, ok := fileIconByExactName[base]; ok {
		return ic
	}
	// Match longest extension first (e.g. ".tar.gz" before ".gz").
	lower := strings.ToLower(base)
	for _, ext := range []string{".tar.gz", ".tar.bz2", ".tar.xz"} {
		if strings.HasSuffix(lower, ext) {
			return FileIcon{iconArchive, lipgloss.Color("#bb9af7")}
		}
	}
	if dot := strings.LastIndex(lower, "."); dot >= 0 {
		ext := lower[dot:]
		if ic, ok := fileIconByExt[ext]; ok {
			return ic
		}
	}
	return FileIcon{iconDefault, lipgloss.Color("#565f89")}
}

// renderFileIcon styles the icon glyph with its mapped color.
func renderFileIcon(name string) string {
	ic := iconForFile(name)
	return lipgloss.NewStyle().Foreground(ic.Color).Render(ic.Glyph)
}
