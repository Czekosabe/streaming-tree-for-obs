package chatoverlay

import (
	operatorchat "github.com/streaming-tree/server/internal/operatorchat"
)

// operatorChatProjection is the shape of the real
// *internal/operatorchat.Projection this package's Manager needs.
// operatorchat.Projection.Subscribe returns its own concrete
// *operatorchat.Subscription rather than this package's unexported
// upstreamSubscription interface, and Go requires exact method
// signatures for interface satisfaction - so *operatorchat.Projection
// cannot implement UpstreamSource directly. WrapOperatorChatSource below
// is the thin adapter that closes that gap at the wiring boundary (see
// cmd/server/main.go and cmd/testserver/main.go).
type operatorChatProjection interface {
	OperatorChatSource
	Subscribe(after uint64) (*operatorchat.Subscription, bool, error)
}

type upstreamSourceAdapter struct {
	inner operatorChatProjection
}

// WrapOperatorChatSource adapts a real operator-chat projection to this
// package's own UpstreamSource, for use by Manager.
func WrapOperatorChatSource(p operatorChatProjection) UpstreamSource {
	return upstreamSourceAdapter{inner: p}
}

func (a upstreamSourceAdapter) ItemsAfter(after uint64, limit int) ([]operatorchat.Item, bool) {
	return a.inner.ItemsAfter(after, limit)
}

func (a upstreamSourceAdapter) Subscribe(after uint64) (upstreamSubscription, bool, error) {
	return a.inner.Subscribe(after)
}
