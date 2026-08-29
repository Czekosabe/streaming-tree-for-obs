module github.com/streaming-tree/server

// The router relies on method-aware ServeMux patterns ("GET /api/health"),
// which need Go 1.22+. The floor is raised to 1.25 by modernc.org/sqlite, the
// CGO-free SQLite driver used for persistence. Stage 20E's own final
// dependency/security audit (docs/progress.md) found six reachable
// govulncheck findings (GO-2026-6218/6091/6090/6089/5972/5026), every
// one a Go-stdlib issue fixed in go1.26.6 - raised here rather than
// left to an implicit toolchain floor, so CI's own
// actions/setup-go@v5 with go-version-file picks up the same patched
// version this local build now requires.
go 1.26.6

require (
	github.com/99designs/keyring v1.2.2
	github.com/coder/websocket v1.8.15
	github.com/go-ole/go-ole v1.3.0
	github.com/shirou/gopsutil/v4 v4.26.7
	golang.org/x/crypto v0.55.0
	golang.org/x/sys v0.47.0
	golang.org/x/term v0.45.0
	google.golang.org/grpc v1.83.0
	google.golang.org/protobuf v1.36.12
	modernc.org/sqlite v1.55.0
)

require (
	github.com/99designs/go-keychain v0.0.0-20191008050251-8e49817e8af4 // indirect
	github.com/danieljoos/wincred v1.1.2 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/dvsekhvalnov/jose2go v1.7.0 // indirect
	github.com/ebitengine/purego v0.10.2 // indirect
	github.com/godbus/dbus v0.0.0-20190726142602-4481cbc300e2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gsterjov/go-libsecret v0.0.0-20161001094733-a6f4afe4910c // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mtibben/percent v0.2.1 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	modernc.org/libc v1.74.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
