package mediamtx

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The supervisor tests need a real child process that behaves like MediaMTX:
// it must read the generated configuration, listen on the configured Control
// API address and answer the two endpoints the client uses.
//
// Rather than shipping a platform-specific shell script, the test binary
// re-executes itself. TestMain checks for the mode variable below and, when it
// is set, runs as a fake MediaMTX instead of running tests.
//
// This is test infrastructure only. The variable is passed through the
// unexported Options.extraEnv, which production code never populates, so it
// cannot weaken the real environment isolation.
const (
	fakeModeEnv = "STREAMING_TREE_TEST_FAKE_MEDIAMTX"

	// fakeModeReady serves the Control API until it is terminated.
	fakeModeReady = "ready"
	// fakeModeCrash exits immediately, as a misconfigured MediaMTX would.
	fakeModeCrash = "crash"
	// fakeModeSilent runs but never opens the Control API, so readiness times out.
	fakeModeSilent = "silent"
	// fakeModeExitAfterReady becomes ready and then exits on its own.
	fakeModeExitAfterReady = "exit-after-ready"

	// fakePublishingEnv makes the fake report an online publisher.
	fakePublishingEnv = "STREAMING_TREE_TEST_FAKE_PUBLISHING"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeModeEnv); mode != "" {
		runFakeMediaMTX(mode)
		return
	}
	os.Exit(m.Run())
}

// runFakeMediaMTX imitates the parts of MediaMTX the supervisor depends on.
func runFakeMediaMTX(mode string) {
	switch mode {
	case fakeModeCrash:
		// Exit non-zero straight away, like a rejected configuration.
		fmt.Fprintln(os.Stderr, `{"level":"ERR","message":"fake configuration failure"}`)
		os.Exit(1)

	case fakeModeSilent:
		// Run without ever opening the API, so readiness must time out.
		// A plain sleep rather than `select {}`, which the Go runtime would
		// report as a deadlock and abort.
		fmt.Println(`{"level":"INF","message":"fake mediamtx without an API"}`)
		time.Sleep(10 * time.Minute)
		os.Exit(0)
	}

	// The configuration path is the single argument, exactly as MediaMTX
	// receives it.
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "fake mediamtx: no configuration path")
		os.Exit(2)
	}

	apiAddress, err := readAPIAddress(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "fake mediamtx: %v\n", err)
		os.Exit(2)
	}

	listener, err := net.Listen("tcp", apiAddress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fake mediamtx: cannot listen on %s: %v\n", apiAddress, err)
		os.Exit(2)
	}

	// Structured output, matching what the real binary emits, so the log
	// parsing path is exercised too.
	fmt.Printf(`{"level":"INF","message":"fake MediaMTX %s"}`+"\n", SupportedVersion)
	fmt.Println("a plain line that is not JSON")

	publishing := os.Getenv(fakePublishingEnv) != ""

	mux := http.NewServeMux()
	mux.HandleFunc("/v3/config/global/get", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"rtmp":true,"api":true,"hls":false}`)
	})
	mux.HandleFunc("/v3/paths/list", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if publishing {
			fmt.Fprint(w, `{"itemCount":1,"pageCount":1,"items":[{"name":"live","confName":"live",`+
				`"ready":true,"readyTime":"2026-08-03T12:00:00Z",`+
				`"source":{"type":"rtmpConn","id":"abc"},"tracks":["H264","MPEG-4 Audio"],`+
				`"bytesReceived":1024}]}`)
			return
		}
		fmt.Fprint(w, `{"itemCount":1,"pageCount":1,"items":[{"name":"live","confName":"live",`+
			`"ready":false,"readyTime":null,"source":null,"tracks":[],"bytesReceived":0}]}`)
	})

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}

	if mode == fakeModeExitAfterReady {
		// Serve briefly so readiness succeeds, then die unexpectedly.
		go func() {
			time.Sleep(2 * time.Second)
			os.Exit(3)
		}()
	}

	_ = server.Serve(listener)
	os.Exit(0)
}

// readAPIAddress pulls apiAddress out of the generated configuration.
func readAPIAddress(configPath string) (string, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read configuration: %w", err)
	}

	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if after, found := strings.CutPrefix(trimmed, "apiAddress:"); found {
			return strings.TrimSpace(after), nil
		}
	}
	return "", fmt.Errorf("configuration has no apiAddress")
}

// freePort reserves a loopback port and releases it, so tests never collide
// with a service already running on the developer's machine.
func freePort(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a port: %v", err)
	}
	defer listener.Close()

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener address type")
	}
	return "127.0.0.1:" + strconv.Itoa(address.Port)
}
