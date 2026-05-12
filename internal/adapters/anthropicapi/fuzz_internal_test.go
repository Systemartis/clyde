package anthropicapi

import "testing"

// FuzzParseCredentialsJSON exercises the credentials-file parser against
// arbitrary input. Contract: must not panic on any input, including
// empty buffers, truncated JSON, embedded NULs, or fields with unexpected
// types. Errors are expected and acceptable; panics are not.
func FuzzParseCredentialsJSON(f *testing.F) {
	f.Add([]byte(`{"access_token":"sk-ant-xxx","refresh_token":"r","expires_at":1700000000}`))
	f.Add([]byte(`{"access_token":""}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(``))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"access_token":123}`))
	f.Add([]byte(`{"expires_at":"not-a-number"}`))

	f.Fuzz(func(_ *testing.T, raw []byte) {
		_, _ = parseCredentialsJSON(raw)
	})
}
