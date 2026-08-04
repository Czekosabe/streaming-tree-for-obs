package mediamtx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newFixtureAPI serves one canned response per endpoint.
func newFixtureAPI(t *testing.T, routes map[string]http.HandlerFunc) *APIClient {
	t.Helper()

	mux := http.NewServeMux()
	for path, handler := range routes {
		mux.HandleFunc(path, handler)
	}

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return NewAPIClient(server.URL)
}

func jsonResponse(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

// --- readiness --------------------------------------------------------------

func TestPingAcceptsTheRealGlobalConfigShape(t *testing.T) {
	// Trimmed from a real v1.19.3 /v3/config/global/get response.
	client := newFixtureAPI(t, map[string]http.HandlerFunc{
		"/v3/config/global/get": jsonResponse(
			`{"logLevel":"info","readTimeout":"10s","api":true,"apiAddress":"127.0.0.1:9997",` +
				`"rtmp":true,"rtmpAddress":"127.0.0.1:1935","hls":false,"webrtc":false}`),
	})

	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() returned an error: %v", err)
	}
}

func TestPingToleratesUnknownFutureFields(t *testing.T) {
	client := newFixtureAPI(t, map[string]http.HandlerFunc{
		"/v3/config/global/get": jsonResponse(
			`{"api":true,"rtmp":true,"somethingAddedLater":{"nested":[1,2,3]},"another":"value"}`),
	})

	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() rejected a payload with unknown fields: %v", err)
	}
}

func TestPingRejectsAPayloadThatIsNotMediaMTX(t *testing.T) {
	// Another service answering on the same port must not count as readiness.
	client := newFixtureAPI(t, map[string]http.HandlerFunc{
		"/v3/config/global/get": jsonResponse(`{"hello":"world"}`),
	})

	if err := client.Ping(context.Background()); err == nil {
		t.Fatal("Ping() accepted a payload with no MediaMTX fields")
	}
}

func TestPingRejectsMalformedJSON(t *testing.T) {
	client := newFixtureAPI(t, map[string]http.HandlerFunc{
		"/v3/config/global/get": jsonResponse(`{"api":true`),
	})

	if err := client.Ping(context.Background()); err == nil {
		t.Fatal("Ping() accepted malformed JSON")
	}
}

func TestPingRejectsANonSuccessStatus(t *testing.T) {
	client := newFixtureAPI(t, map[string]http.HandlerFunc{
		"/v3/config/global/get": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	})

	if err := client.Ping(context.Background()); err == nil {
		t.Fatal("Ping() accepted a 500 response")
	}
}

func TestPingTimesOut(t *testing.T) {
	client := newFixtureAPI(t, map[string]http.HandlerFunc{
		"/v3/config/global/get": func(w http.ResponseWriter, r *http.Request) {
			// Longer than the client's own timeout.
			time.Sleep(apiTimeout + 2*time.Second)
		},
	})

	start := time.Now()
	if err := client.Ping(context.Background()); err == nil {
		t.Fatal("Ping() waited for a hung server instead of timing out")
	}
	if elapsed := time.Since(start); elapsed > apiTimeout+2*time.Second {
		t.Errorf("Ping() took %s, want it bounded by the client timeout", elapsed)
	}
}

func TestPingRejectsAnOversizedResponse(t *testing.T) {
	client := newFixtureAPI(t, map[string]http.HandlerFunc{
		"/v3/config/global/get": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			chunk := strings.Repeat("x", 1<<20)
			for i := 0; i < 6; i++ {
				_, _ = w.Write([]byte(chunk))
			}
		},
	})

	if err := client.Ping(context.Background()); err == nil {
		t.Fatal("Ping() accepted a response beyond the size limit")
	}
}

// --- path status ------------------------------------------------------------

// realWaitingResponse is the exact body a real v1.19.3 instance returned with
// the path configured and no publisher connected.
const realWaitingResponse = `{"itemCount":1,"pageCount":1,"items":[{"name":"live",` +
	`"confName":"live","ready":false,"readyTime":null,"available":false,"availableTime":null,` +
	`"online":false,"onlineTime":null,"source":null,"tracks":[],"tracks2":[],"readers":[],` +
	`"inboundBytes":0,"outboundBytes":0,"inboundFramesInError":0,"bytesReceived":0,"bytesSent":0}]}`

func TestPathStatusForReportsNoPublisher(t *testing.T) {
	client := newFixtureAPI(t, map[string]http.HandlerFunc{
		"/v3/paths/list": jsonResponse(realWaitingResponse),
	})

	status, err := client.PathStatusFor(context.Background(), "live")
	if err != nil {
		t.Fatalf("PathStatusFor() returned an error: %v", err)
	}

	if !status.Found {
		t.Error("the configured path was not found")
	}
	if status.Ready {
		t.Error("ready = true with no publisher connected")
	}
	if status.SourceType != "" {
		t.Errorf("sourceType = %q, want empty with no publisher", status.SourceType)
	}
	if len(status.Tracks) != 0 {
		t.Errorf("tracks = %v, want empty", status.Tracks)
	}
}

func TestPathStatusForReportsAnOnlinePublisher(t *testing.T) {
	client := newFixtureAPI(t, map[string]http.HandlerFunc{
		"/v3/paths/list": jsonResponse(
			`{"itemCount":1,"pageCount":1,"items":[{"name":"live","confName":"live",` +
				`"ready":true,"readyTime":"2026-08-03T12:00:00.123Z",` +
				`"source":{"type":"rtmpConn","id":"1234"},` +
				`"tracks":["H264","MPEG-4 Audio"],"bytesReceived":98765}]}`),
	})

	status, err := client.PathStatusFor(context.Background(), "live")
	if err != nil {
		t.Fatalf("PathStatusFor() returned an error: %v", err)
	}

	if !status.Found || !status.Ready {
		t.Fatalf("status = %+v, want found and ready", status)
	}
	if status.SourceType != "rtmpConn" {
		t.Errorf("sourceType = %q, want rtmpConn", status.SourceType)
	}
	if status.ReadyTime != "2026-08-03T12:00:00.123Z" {
		t.Errorf("readyTime = %q", status.ReadyTime)
	}
	if len(status.Tracks) != 2 || status.Tracks[0] != "H264" {
		t.Errorf("tracks = %v, want the reported codecs", status.Tracks)
	}
}

func TestPathStatusForReportsADisappearedPath(t *testing.T) {
	client := newFixtureAPI(t, map[string]http.HandlerFunc{
		"/v3/paths/list": jsonResponse(`{"itemCount":0,"pageCount":1,"items":[]}`),
	})

	status, err := client.PathStatusFor(context.Background(), "live")
	if err != nil {
		t.Fatalf("PathStatusFor() returned an error: %v", err)
	}
	if status.Found {
		t.Error("found = true for a path that is not listed")
	}
	// A nil slice would serialize as JSON null; the API contract says array.
	if status.Tracks == nil {
		t.Error("tracks is nil, want an empty slice")
	}
}

func TestPathStatusForIgnoresOtherPaths(t *testing.T) {
	client := newFixtureAPI(t, map[string]http.HandlerFunc{
		"/v3/paths/list": jsonResponse(
			`{"itemCount":2,"pageCount":1,"items":[` +
				`{"name":"other","ready":true,"source":{"type":"rtmpConn"},"tracks":["H264"]},` +
				`{"name":"live","ready":false,"source":null,"tracks":[]}]}`),
	})

	status, err := client.PathStatusFor(context.Background(), "live")
	if err != nil {
		t.Fatalf("PathStatusFor() returned an error: %v", err)
	}
	if status.Ready {
		t.Error("the status of a different path was returned")
	}
}

func TestPathStatusForToleratesUnknownFields(t *testing.T) {
	client := newFixtureAPI(t, map[string]http.HandlerFunc{
		"/v3/paths/list": jsonResponse(
			`{"itemCount":1,"items":[{"name":"live","ready":true,"tracks":["H264"],` +
				`"futureField":{"a":1},"anotherNewThing":[true,false]}],"newTopLevel":"x"}`),
	})

	status, err := client.PathStatusFor(context.Background(), "live")
	if err != nil {
		t.Fatalf("PathStatusFor() rejected unknown fields: %v", err)
	}
	if !status.Ready {
		t.Error("the known fields were not read correctly")
	}
}

func TestPathStatusForRejectsMissingRequiredFields(t *testing.T) {
	cases := map[string]string{
		"no name":  `{"items":[{"ready":true,"tracks":[]}]}`,
		"no ready": `{"items":[{"name":"live","tracks":[]}]}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			client := newFixtureAPI(t, map[string]http.HandlerFunc{
				"/v3/paths/list": jsonResponse(body),
			})

			// Treating a missing "ready" as false would silently report
			// "waiting" during an API problem, which is worse than an error.
			if _, err := client.PathStatusFor(context.Background(), "live"); err == nil {
				t.Fatal("PathStatusFor() accepted a response missing a required field")
			}
		})
	}
}

func TestPathStatusForRejectsMalformedJSON(t *testing.T) {
	client := newFixtureAPI(t, map[string]http.HandlerFunc{
		"/v3/paths/list": jsonResponse(`not json at all`),
	})

	if _, err := client.PathStatusFor(context.Background(), "live"); err == nil {
		t.Fatal("PathStatusFor() accepted malformed JSON")
	}
}

func TestPathStatusForRejectsANonSuccessStatus(t *testing.T) {
	client := newFixtureAPI(t, map[string]http.HandlerFunc{
		"/v3/paths/list": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		},
	})

	if _, err := client.PathStatusFor(context.Background(), "live"); err == nil {
		t.Fatal("PathStatusFor() accepted a 503 response")
	}
}

func TestAPIClientOnlyTargetsTheGivenBaseURL(t *testing.T) {
	// The client must never be pointed anywhere but the loopback address the
	// supervisor configured; a trailing slash must not produce a double slash.
	client := NewAPIClient("http://127.0.0.1:9997/")

	if client.baseURL != "http://127.0.0.1:9997" {
		t.Errorf("baseURL = %q, want the trailing slash trimmed", client.baseURL)
	}
}
