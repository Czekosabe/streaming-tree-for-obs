# streamlistpb

Generated Go gRPC client code for YouTube's `liveChatMessages.streamList`
server-streaming RPC. See `stream_list.proto` for provenance (vendored
verbatim, with one added `import` and one added `go_package` option, from
Google's official documentation at
https://developers.google.com/youtube/v3/live/streaming-live-chat).

`stream_list.pb.go` and `stream_list_grpc.pb.go` are **checked into the
repository**. A normal `go build`/`go test`/`go vet` never regenerates them
and never requires `protoc` to be installed - see
docs/provider-integrations/youtube-engagement.md for the full rationale
(Stage 15A transport corrective pass).

## Regenerating (maintainers only)

Only needed if `stream_list.proto` itself changes (e.g. Google revises the
schema). Requires:

- [`protoc`](https://github.com/protocolbuffers/protobuf/releases) (this
  package was generated with v29.3)
- `protoc-gen-go` and `protoc-gen-go-grpc`, installed with:

  ```sh
  go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
  ```

Then, from this directory:

```sh
protoc -I . \
  --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  stream_list.proto
```

`google/protobuf/duration.proto` (the well-known type
`LiveChatGiftDetails.gift_duration` depends on) is bundled inside `protoc`
itself, so no extra `-I` path is required for it.

After regenerating, run `go build ./... && go vet ./...` from `apps/server`
and re-run this package's own tests plus
`internal/runtime/youtubeengagement`'s tests before committing.
