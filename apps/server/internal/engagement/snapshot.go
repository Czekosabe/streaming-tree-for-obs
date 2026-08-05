package bus

// Snapshot is the Event Bus's own status, with no message content - see
// Bus.Snapshot. The HTTP layer (internal/httpapi) combines this with
// connector-manager summaries to answer GET /api/engagement/status.
type Snapshot struct {
	SchemaVersion     int
	Capacity          int
	RetainedCount     int
	OldestSequence    uint64
	NewestSequence    uint64
	ActiveSubscribers int
}
