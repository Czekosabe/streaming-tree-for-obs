package engagement

import "fmt"

// maxProviderExtraEntries and maxProviderExtraValueLength bound the
// deliberately-small ProviderExtra bag (see event.go's doc comment) - a
// connector bug that tried to smuggle an unbounded or raw-JSON-shaped
// payload through it fails validation instead of silently growing every
// event's memory footprint.
const (
	maxProviderExtraEntries     = 8
	maxProviderExtraValueLength = 256
)

// Validate reports whether e is structurally acceptable to publish. It does
// not check Sequence, ReceivedAt or ID - those are assigned by the bus after
// Validate would normally be called by a connector, and are validated
// separately by the bus itself before it trusts them.
func (e Event) Validate() error {
	if e.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("%w: unsupported schema version %d", ErrInvalidEvent, e.SchemaVersion)
	}
	if e.ProviderID == "" {
		return fmt.Errorf("%w: providerId is required", ErrInvalidEvent)
	}
	if e.ConnectedAccountID == "" {
		return fmt.Errorf("%w: connectedAccountId is required", ErrInvalidEvent)
	}
	if e.Type == "" || !e.Type.Known() {
		return fmt.Errorf("%w: unknown event type %q", ErrInvalidEvent, e.Type)
	}
	if e.PlatformTimestamp.IsZero() {
		return fmt.Errorf("%w: platformTimestamp is required", ErrInvalidEvent)
	}
	if e.DedupeKey == "" {
		return fmt.Errorf("%w: dedupeKey is required", ErrInvalidEvent)
	}

	switch e.Type {
	case TypeChatMessage:
		if e.Message == nil {
			return fmt.Errorf("%w: %s requires a message", ErrInvalidEvent, e.Type)
		}
		if e.User == nil {
			return fmt.Errorf("%w: %s requires a user", ErrInvalidEvent, e.Type)
		}
	case TypeChatMessageDeleted:
		// References one specific earlier message - see §5.8: a deletion is
		// always "this message was removed," never a broader action.
		if e.ModerationRef == "" {
			return fmt.Errorf("%w: %s requires a moderationRef", ErrInvalidEvent, e.Type)
		}
	case TypeModeration:
		// Unlike chat.message_deleted, a moderation action is not always a
		// reference to one earlier event - "clear a user's messages" targets
		// a user (User field), not a specific prior message id. Either a
		// moderationRef (references an earlier event) or a stable
		// ModerationAction identifier (describes the action itself) must be
		// present, so a moderation event is never contentless.
		if e.ModerationRef == "" && e.ModerationAction == "" {
			return fmt.Errorf("%w: %s requires a moderationRef or a moderationAction", ErrInvalidEvent, e.Type)
		}
	case TypeGiftedSubscription, TypeSubscriptionGiftBatch:
		if e.Quantity == nil && e.Type == TypeSubscriptionGiftBatch {
			return fmt.Errorf("%w: %s requires a quantity", ErrInvalidEvent, e.Type)
		}
	case TypeBits:
		if e.Quantity == nil {
			return fmt.Errorf("%w: %s requires a quantity", ErrInvalidEvent, e.Type)
		}
	case TypeYouTubeSuperChat, TypeYouTubeSuperSticker, TypeDonation:
		if e.Money == nil {
			return fmt.Errorf("%w: %s requires money", ErrInvalidEvent, e.Type)
		}
	}

	if e.Money != nil {
		if e.Money.AmountMicros < 0 {
			return fmt.Errorf("%w: money amount must not be negative", ErrInvalidEvent)
		}
		if e.Money.Currency == "" {
			return fmt.Errorf("%w: money requires a currency", ErrInvalidEvent)
		}
	}

	if len(e.ProviderExtra) > maxProviderExtraEntries {
		return fmt.Errorf("%w: providerExtra has too many entries", ErrInvalidEvent)
	}
	for k, v := range e.ProviderExtra {
		if len(k) == 0 || len(v) > maxProviderExtraValueLength {
			return fmt.Errorf("%w: providerExtra value too long for key %q", ErrInvalidEvent, k)
		}
	}

	if e.Message != nil {
		if built := BuildText(e.Message.Fragments); built != e.Message.Text {
			return fmt.Errorf("%w: message text does not match its fragments", ErrInvalidEvent)
		}
	}

	return nil
}
