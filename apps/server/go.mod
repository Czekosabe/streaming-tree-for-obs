module github.com/streaming-tree/server

// 1.22 is the minimum: the router relies on method-aware patterns
// ("GET /api/health") introduced in the 1.22 net/http ServeMux.
go 1.22
