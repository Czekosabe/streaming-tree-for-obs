package support

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/streaming-tree/server/internal/diagnostics"
)

// syntheticSecrets are secret-shaped values matching every category the
// governing task requires the self-audit to seed: a stream key, an
// OAuth access/refresh token pair, a remote-overlay capability token, a
// session cookie value, a CSRF token, and an RTMPS credential
// fragment. None of these are real - they exist only to prove the
// bundle-generation path can never leak a value shaped like them.
var syntheticSecrets = []string{
	"sk_live_synthetic_stream_key_9f8e7d6c5b4a39281706152433425162",
	"synthetic_oauth_access_token_AbCdEfGhIjKlMnOpQrStUvWxYz01234567",
	"synthetic_oauth_refresh_token_ZyXwVuTsRqPoNmLkJiHgFeDcBa76543210",
	"synthetic_remote_overlay_capability_QqWwEeRrTtYyUu112233445566778899",
	"synthetic_session_cookie_value_00112233445566778899aabbccddeeff",
	"synthetic_csrf_token_112233445566778899aabbccddeeff00112233",
	"pass=synthetic_rtmps_password_value_1234567890abcdef",
}

// nonSecretMarker is a distinctive, definitely-non-secret log message
// the self-audit also seeds, so a bundle that happens to be empty (or
// whose log capture silently failed) cannot pass this test merely by
// containing nothing.
const nonSecretMarker = "mediamtx runtime observed as running (self-audit marker)"

func TestSupportBundleNeverLeaksSecretShapedValues(t *testing.T) {
	recorder := diagnostics.NewRecorder()
	handler := diagnostics.NewHandler(slog.NewTextHandler(io.Discard, nil), recorder)
	logger := slog.New(handler)

	for _, secret := range syntheticSecrets {
		logger.Error("a synthetic upstream error mentioning a secret-shaped value: " + secret)
	}
	logger.Info(nonSecretMarker)

	snapshotFn := func(ctx context.Context) (Snapshot, error) {
		return Snapshot{
			Version:          "0.1.0-test",
			OS:               "linux",
			Arch:             "amd64",
			GoRuntimeVersion: "go1.26",
			Headless:         true,
			SubsystemStates: map[string]string{
				"mediamtx": "running",
			},
			PlatformSupportSummary: "Linux headless: Native CI verified",
		}, nil
	}

	builder := NewBuilder(recorder, snapshotFn)
	data, filename, err := builder.BuildSupportBundle(context.Background())
	if err != nil {
		t.Fatalf("BuildSupportBundle returned an error: %v", err)
	}
	if filename == "" {
		t.Fatalf("BuildSupportBundle returned an empty filename")
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("support bundle is not a valid zip: %v", err)
	}

	var allBytes bytes.Buffer
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s in support bundle: %v", f.Name, err)
		}
		if _, err := io.Copy(&allBytes, rc); err != nil {
			rc.Close()
			t.Fatalf("read %s in support bundle: %v", f.Name, err)
		}
		rc.Close()
	}

	full := allBytes.String()

	for _, secret := range syntheticSecrets {
		if strings.Contains(full, secret) {
			t.Errorf("support bundle contains a synthetic secret-shaped value verbatim: %q", secret)
		}
	}

	if !strings.Contains(full, nonSecretMarker) {
		t.Errorf("support bundle is missing the non-secret marker message - the bundle must not be trivially empty")
	}
	if !strings.Contains(full, "mediamtx") {
		t.Errorf("support bundle is missing expected non-secret subsystem-state data")
	}
	if !strings.Contains(full, "0.1.0-test") {
		t.Errorf("support bundle is missing expected non-secret manifest version data")
	}
}

func TestSupportBundleFilenameHasNoUserInput(t *testing.T) {
	recorder := diagnostics.NewRecorder()
	snapshotFn := func(ctx context.Context) (Snapshot, error) {
		return Snapshot{Version: "9.9.9"}, nil
	}
	builder := NewBuilder(recorder, snapshotFn)

	_, filename, err := builder.BuildSupportBundle(context.Background())
	if err != nil {
		t.Fatalf("BuildSupportBundle returned an error: %v", err)
	}
	if !strings.HasPrefix(filename, "streaming-tree-support-") || !strings.HasSuffix(filename, ".zip") {
		t.Errorf("filename %q does not match the expected app-controlled shape", filename)
	}
	if strings.ContainsAny(filename, "/\\") {
		t.Errorf("filename %q contains a path separator - traversal risk", filename)
	}
}
