module github.com/streaming-tree/server

// The router relies on method-aware ServeMux patterns ("GET /api/health"),
// which need Go 1.22+. The floor is raised to 1.25 by modernc.org/sqlite, the
// CGO-free SQLite driver used for persistence.
go 1.25.0

require modernc.org/sqlite v1.55.0

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.46.0 // indirect
	modernc.org/libc v1.74.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
