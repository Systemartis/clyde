package jsonl

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// FuzzDecodeLineWithMsgID exercises the JSONL line decoder against
// arbitrary input. The contract is "must not panic on any input."
// Seed corpus is drawn from any *.jsonl files committed under testdata/
// plus a handful of minimal envelopes.
func FuzzDecodeLineWithMsgID(f *testing.F) {
	if entries, err := os.ReadDir("testdata"); err == nil {
		for _, e := range entries {
			if filepath.Ext(e.Name()) != ".jsonl" {
				continue
			}
			data, rerr := os.ReadFile(filepath.Join("testdata", e.Name()))
			if rerr != nil {
				continue
			}
			for _, line := range bytes.Split(data, []byte("\n")) {
				if len(line) > 0 {
					f.Add(line)
				}
			}
		}
	}
	f.Add([]byte(`{"type":"user","uuid":"x"}`))
	f.Add([]byte(`{"type":"assistant","message":{"id":"x"}}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(``))
	f.Add([]byte(`not json`))

	f.Fuzz(func(_ *testing.T, raw []byte) {
		_, _, _ = decodeLineWithMsgID(raw)
	})
}

// TestDecodeLineWithMsgID_RejectsOversizedLine pins the input bound: the
// production reader caps lines at scannerMaxToken via the Scanner buffer, so
// the decoder must enforce the same invariant when called directly. Without
// it, mutation-grown fuzz inputs reach json.Unmarshal unbounded and can run
// long enough to trip the fuzz engine's shutdown deadline on slow runners.
func TestDecodeLineWithMsgID_RejectsOversizedLine(t *testing.T) {
	t.Parallel()
	// A syntactically VALID oversized line: without the bound it decodes
	// successfully (slowly); with the bound it must be rejected up front.
	pad := make([]byte, scannerMaxToken)
	for i := range pad {
		pad[i] = 'a'
	}
	big := []byte(`{"type":"assistant","pad":"` + string(pad) + `"}`)
	if _, _, err := decodeLineWithMsgID(big); err == nil {
		t.Fatal("decodeLineWithMsgID accepted an oversized line; want error")
	}
}
