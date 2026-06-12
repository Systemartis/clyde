package main

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteCrashReport_BasicShape verifies the tarball is a valid gzip+tar
// stream with at least report.txt at the expected path. We deliberately do
// NOT depend on a specific log file existing — the helper includes it best-
// effort, and a clean test environment legitimately won't have one.
func TestWriteCrashReport_BasicShape(t *testing.T) {
	// Pin the report destination via $HOME and seed a fake log file that
	// clydelog would have written, so we exercise the log-inclusion branch.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))
	if err := os.MkdirAll(filepath.Join(tmp, "cache", "clyde"), 0o700); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	logBody := []byte(`{"msg":"seed-record"}` + "\n")
	logPath := filepath.Join(tmp, "cache", "clyde", "clyde.log")
	if err := os.WriteFile(logPath, logBody, 0o600); err != nil {
		t.Fatalf("seed log: %v", err)
	}

	path, err := writeCrashReport()
	if err != nil {
		t.Fatalf("writeCrashReport: %v", err)
	}
	defer os.Remove(path)

	if !strings.HasPrefix(path, tmp) {
		t.Errorf("path = %q, want it under %q", path, tmp)
	}
	if !strings.HasSuffix(path, ".tar.gz") {
		t.Errorf("path = %q, want .tar.gz suffix", path)
	}

	// Walk the archive — collect filenames and the report.txt body.
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open report: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	files := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		body, _ := io.ReadAll(tr)
		files[hdr.Name] = body
	}

	if _, ok := files["report.txt"]; !ok {
		t.Fatal("report.txt missing from archive")
	}
	if _, ok := files["clyde.log"]; !ok {
		t.Fatal("clyde.log missing from archive — expected to be picked up via XDG_CACHE_HOME")
	}

	report := string(files["report.txt"])
	for _, want := range []string{"clyde crash report", "version :", "os/arch :"} {
		if !strings.Contains(report, want) {
			t.Errorf("report.txt missing %q\n%s", want, report)
		}
	}

	if got := string(files["clyde.log"]); got != string(logBody) {
		t.Errorf("clyde.log roundtrip\n got: %q\nwant: %q", got, logBody)
	}
}

// TestBuildCrashReportText_FieldsPresent verifies the human-readable header
// includes every diagnostic field the support docs tell users to provide.
func TestBuildCrashReportText_FieldsPresent(t *testing.T) {
	got := buildCrashReportText()
	for _, want := range []string{"version :", "commit  :", "built   :", "go      :", "os/arch :", "TERM    :", "shell   :", "ts      :"} {
		if !strings.Contains(got, want) {
			t.Errorf("buildCrashReportText() missing %q\n%s", want, got)
		}
	}
}
