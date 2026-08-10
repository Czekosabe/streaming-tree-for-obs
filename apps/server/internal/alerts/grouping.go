package alerts

import (
	"math"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/alerts"
)

// groupKey identifies which still-queued alerts a candidate may merge
// into (Stage 12B task Part 4). Profile scoping is structural (a
// profileRuntime only ever searches its own queue), so it is
// deliberately not a field here.
type groupKey struct {
	ruleID              string
	ruleUpdatedAt       time.Time
	providerID          domain.ProviderID
	connectedAccountID  string
	eventType           domain.EventType
	actorProviderUserID string
	rewardID            string
}

// groupingEligible reports whether inst may ever participate in
// automatic grouping - as a brand-new group of one, or as a later
// joiner. Part 14: only genuine, newly matched, non-anonymous live real
// alerts ever do - a synthetic Test Rule alert, a Replay Previous
// re-show, and an anonymous actor (whose "same actor" identity can never
// be verified) are all permanently excluded.
func groupingEligible(inst Instance) bool {
	if inst.Synthetic || inst.Replayed || inst.Anonymous {
		return false
	}
	if !inst.AllowGrouping {
		return false
	}
	if !domain.GroupingCapabilityFor(inst.EventType).Groupable {
		return false
	}
	return inst.ActorProviderUserID != ""
}

func groupKeyFor(inst Instance) groupKey {
	capability := domain.GroupingCapabilityFor(inst.EventType)
	reward := ""
	if capability.SubjectFromRewardID {
		reward = inst.RewardID
	}
	return groupKey{
		ruleID: inst.RuleID, ruleUpdatedAt: inst.RuleUpdatedAt,
		providerID: inst.ProviderID, connectedAccountID: inst.ConnectedAccountID,
		eventType: inst.EventType, actorProviderUserID: inst.ActorProviderUserID,
		rewardID: reward,
	}
}

// windowOpen reports whether now still falls within groupWindowMS of
// firstQueuedAt - the fixed, first-member-anchored window (Stage 12B
// task Part 5). A later member joining never extends it.
func windowOpen(firstQueuedAt time.Time, groupWindowMS int, now time.Time) bool {
	return !now.After(firstQueuedAt.Add(time.Duration(groupWindowMS) * time.Millisecond))
}

// safeAddInt64 sums a and b, clamping to math.MaxInt64 instead of
// wrapping on overflow (Stage 12B task Part 38: "integer overflow/upper
// bound safely handled") - purely defensive, since a real Bits/gift
// quantity never approaches this range.
func safeAddInt64(a, b int64) int64 {
	if b > 0 && a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// mergeGroupMember folds candidate into member (an existing queued
// instance), in place: increments GroupCount, sums Quantity only when
// the type's capability says it is truthfully additive, and re-renders
// RenderedText from member's own stored snapshot - never from
// candidate's. member's QueuedAt, Priority, and queue position are
// never touched (Stage 12B task Part 12/13).
func mergeGroupMember(member *Instance, candidate Instance) {
	if member.GroupCount < domain.MaxGroupMembers {
		member.GroupCount++
	}
	capability := domain.GroupingCapabilityFor(member.EventType)
	if capability.QuantitySummable && candidate.Quantity != nil {
		if member.Quantity == nil {
			q := *candidate.Quantity
			member.Quantity = &q
		} else {
			sum := safeAddInt64(*member.Quantity, *candidate.Quantity)
			member.Quantity = &sum
		}
	}
	member.RenderedText = renderGroupedText(*member)
}

// renderGroupedText re-renders an instance's own template against its
// own stored snapshot fields (never candidate's, never a live re-read of
// domain.Rule) - the deterministic re-render a grouping merge performs
// so {quantity}/{groupCount} reflect the updated aggregate. Message is
// deliberately never included: RequiresNoMessage grouping types can
// never have a real Message at this point (validation forbids
// ShowMessage=true and a {message} reference together with grouping),
// and grouping is never enabled for a type where it would matter.
func renderGroupedText(inst Instance) string {
	ctx := Context{
		Platform: inst.PlatformLabel, EventType: EventTypeLabel(inst.EventType, inst.Language),
		GroupCount: inst.GroupCount,
	}
	if inst.ActorDisplayName != "" {
		name := inst.ActorDisplayName
		ctx.Username = &name
	}
	if inst.Quantity != nil {
		q := *inst.Quantity
		ctx.Quantity = &q
	}
	if result, err := Render(inst.TextTemplate, ctx); err == nil {
		return result.Text
	}
	return inst.RenderedText
}
