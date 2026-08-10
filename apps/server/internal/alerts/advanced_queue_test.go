package alerts

import (
	"sync"
	"testing"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/alerts"
)

// mkGroupable builds a groupable, non-anonymous Bits Instance sharing a
// stable actor/rule identity - the base fixture every grouping test
// customizes from.
func mkGroupable(id, actor string, priority int, quantity int64, queuedAt time.Time) Instance {
	return Instance{
		ID: id, ProfileID: "alprof_1", RuleID: "alrule_1", RuleUpdatedAt: time.Unix(0, 0),
		ProviderID: domain.ProviderTwitch, ConnectedAccountID: "acct_1", EventType: domain.EventBits,
		Priority: priority, DurationMS: 5000, QueuedAt: queuedAt,
		ActorProviderUserID: actor, ActorDisplayName: actor,
		Quantity: &quantity, GroupCount: 1,
		AllowGrouping: true, GroupWindowMS: 5000,
		TextTemplate:  "{username} cheered {quantity} bits (x{groupCount})",
		Language:      domain.LanguageEnglish,
		Interruptible: true, InterruptMode: domain.InterruptNever,
	}
}

// --- pure grouping-key/eligibility/window tests -----------------------

func TestGroupingEligibleRejectsSyntheticReplayAnonymous(t *testing.T) {
	base := mkGroupable("a", "u1", 50, 10, time.Now())
	if !groupingEligible(base) {
		t.Fatal("groupingEligible(base) = false, want true")
	}
	synthetic := base
	synthetic.Synthetic = true
	if groupingEligible(synthetic) {
		t.Error("groupingEligible(synthetic) = true, want false")
	}
	replayed := base
	replayed.Replayed = true
	if groupingEligible(replayed) {
		t.Error("groupingEligible(replayed) = true, want false")
	}
	anon := base
	anon.Anonymous = true
	if groupingEligible(anon) {
		t.Error("groupingEligible(anonymous) = true, want false")
	}
	noActor := base
	noActor.ActorProviderUserID = ""
	if groupingEligible(noActor) {
		t.Error("groupingEligible(no actor id) = true, want false")
	}
	notAllowed := base
	notAllowed.AllowGrouping = false
	if groupingEligible(notAllowed) {
		t.Error("groupingEligible(AllowGrouping=false) = true, want false")
	}
	unsupported := base
	unsupported.EventType = domain.EventFollow
	if groupingEligible(unsupported) {
		t.Error("groupingEligible(follow, ungroupable type) = true, want false")
	}
}

func TestGroupKeyDiffersOnEveryDiscriminatingField(t *testing.T) {
	base := mkGroupable("a", "u1", 50, 10, time.Now())
	variants := []Instance{
		func() Instance { v := base; v.RuleID = "alrule_2"; return v }(),
		func() Instance { v := base; v.RuleUpdatedAt = time.Unix(1, 0); return v }(),
		func() Instance { v := base; v.ProviderID = "other"; return v }(),
		func() Instance { v := base; v.ConnectedAccountID = "acct_2"; return v }(),
		func() Instance { v := base; v.EventType = domain.EventSubscriptionGiftBatch; return v }(),
		func() Instance { v := base; v.ActorProviderUserID = "u2"; return v }(),
	}
	baseKey := groupKeyFor(base)
	for i, v := range variants {
		if groupKeyFor(v) == baseKey {
			t.Errorf("variant %d: groupKeyFor produced the same key as base - the field it changed must discriminate", i)
		}
	}
}

func TestGroupKeyRewardIDOnlyForSubjectFromRewardIDTypes(t *testing.T) {
	redemption := mkGroupable("a", "u1", 50, 0, time.Now())
	redemption.EventType = domain.EventChannelPointRedemption
	redemption.RewardID = "reward_1"
	other := redemption
	other.RewardID = "reward_2"
	if groupKeyFor(redemption) == groupKeyFor(other) {
		t.Error("two different reward ids produced the same group key for a redemption, want different keys")
	}

	// Bits has no SubjectFromRewardID - a stray RewardID value (should
	// never be set in practice) must not affect its key.
	bitsA := mkGroupable("a", "u1", 50, 10, time.Now())
	bitsA.RewardID = "irrelevant_1"
	bitsB := mkGroupable("a", "u1", 50, 10, time.Now())
	bitsB.RewardID = "irrelevant_2"
	if groupKeyFor(bitsA) != groupKeyFor(bitsB) {
		t.Error("RewardID affected the group key for Bits, which has no reward subject")
	}
}

func TestWindowOpenAnchoredToFirstMember(t *testing.T) {
	first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !windowOpen(first, 5000, first.Add(4*time.Second)) {
		t.Error("windowOpen at +4s of a 5s window = false, want true")
	}
	if !windowOpen(first, 5000, first.Add(5*time.Second)) {
		t.Error("windowOpen exactly at the boundary (+5s of a 5s window) = false, want true (inclusive)")
	}
	if windowOpen(first, 5000, first.Add(6*time.Second)) {
		t.Error("windowOpen at +6s of a 5s window = true, want false")
	}
}

func TestSafeAddInt64ClampsOnOverflow(t *testing.T) {
	const maxInt64 = 1<<63 - 1
	if got := safeAddInt64(maxInt64-1, 5); got != maxInt64 {
		t.Errorf("safeAddInt64(near-max, 5) = %d, want clamped to %d", got, int64(maxInt64))
	}
	if got := safeAddInt64(10, 20); got != 30 {
		t.Errorf("safeAddInt64(10, 20) = %d, want 30", got)
	}
}

func TestMergeGroupMemberSumsQuantityAndRerenders(t *testing.T) {
	member := mkGroupable("a", "u1", 50, 100, time.Now())
	candidate := mkGroupable("b", "u1", 50, 50, time.Now())
	mergeGroupMember(&member, candidate)
	if member.GroupCount != 2 {
		t.Errorf("GroupCount = %d, want 2", member.GroupCount)
	}
	if member.Quantity == nil || *member.Quantity != 150 {
		t.Errorf("Quantity = %v, want 150 (truthful sum)", member.Quantity)
	}
	if member.RenderedText != "u1 cheered 150 bits (x2)" {
		t.Errorf("RenderedText = %q, want the re-rendered aggregate", member.RenderedText)
	}
}

func TestMergeGroupMemberRespectsMaxGroupMembers(t *testing.T) {
	member := mkGroupable("a", "u1", 50, 1, time.Now())
	member.GroupCount = domain.MaxGroupMembers
	candidate := mkGroupable("b", "u1", 50, 1, time.Now())
	mergeGroupMember(&member, candidate)
	if member.GroupCount != domain.MaxGroupMembers {
		t.Errorf("GroupCount = %d, want capped at %d", member.GroupCount, domain.MaxGroupMembers)
	}
}

func TestMergeGroupMemberNeverSumsForSameActorSameSubjectCountStrategy(t *testing.T) {
	member := mkGroupable("a", "u1", 50, 0, time.Now())
	member.EventType = domain.EventChannelPointRedemption
	member.Quantity = nil
	candidate := member
	candidate.ID = "b"
	mergeGroupMember(&member, candidate)
	if member.GroupCount != 2 {
		t.Errorf("GroupCount = %d, want 2", member.GroupCount)
	}
	if member.Quantity != nil {
		t.Errorf("Quantity = %v, want nil (redemption has no quantity to sum)", member.Quantity)
	}
}

// --- profileRuntime-level grouping integration tests -------------------

func TestTryGroupFirstMemberStartsAsNormalQueueItemNotGrouped(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	pr.enqueueMatched([]Instance{mkGroupable("a", "u1", 50, 100, now)}, now, staticID)
	st := pr.status()
	if st.QueuedCount != 1 || st.NextQueued[0].GroupCount != 1 {
		t.Fatalf("status = %+v, want one queued item with GroupCount=1", st)
	}
	if st.TotalGroupsCreated != 0 || st.TotalGroupedMembers != 0 {
		t.Errorf("first member alone must not count as a created group or a grouped member: status = %+v", st)
	}
}

func TestTryGroupSecondCompatibleEventMergesWithoutNewQueueSlot(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	pr.enqueueMatched([]Instance{mkGroupable("a", "u1", 50, 100, now)}, now, staticID)
	pr.enqueueMatched([]Instance{mkGroupable("b", "u1", 50, 50, now.Add(time.Second))}, now.Add(time.Second), staticID)

	st := pr.status()
	if st.QueuedCount != 1 {
		t.Fatalf("QueuedCount = %d, want 1 (the merge never consumes a second queue slot)", st.QueuedCount)
	}
	if st.NextQueued[0].GroupCount != 2 {
		t.Errorf("GroupCount = %d, want 2", st.NextQueued[0].GroupCount)
	}
	if q := st.NextQueued[0].Quantity; q == nil || *q != 150 {
		t.Errorf("Quantity = %v, want 150", q)
	}
	if st.TotalGroupsCreated != 1 {
		t.Errorf("TotalGroupsCreated = %d, want 1", st.TotalGroupsCreated)
	}
	if st.TotalGroupedMembers != 1 {
		t.Errorf("TotalGroupedMembers = %d, want 1 (only the second, joining member counts)", st.TotalGroupedMembers)
	}
	if st.TotalEnqueued != 1 {
		t.Errorf("TotalEnqueued = %d, want 1 (a grouped merge never occupies a new queue slot, so it is not counted as a separate enqueue - TotalGroupedMembers tracks it instead)", st.TotalEnqueued)
	}
}

func TestTryGroupPreservesEarliestQueuedAtAndExpirationAnchor(t *testing.T) {
	p := testProfile()
	p.MaximumQueueAgeSeconds = 5
	pr := newProfileRuntime("alprof_1", p, staticID)
	first := time.Now()
	pr.enqueueMatched([]Instance{mkGroupable("a", "u1", 50, 1, first)}, first, staticID)
	pr.enqueueMatched([]Instance{mkGroupable("b", "u1", 50, 1, first.Add(3*time.Second))}, first.Add(3*time.Second), staticID)

	// Expiration is anchored to the FIRST member's own queuedAt (5s
	// maxAge): by first+6s (3s after the second member joined, but 6s
	// after the group's own true start) the group must already be stale.
	pr.tick(first.Add(6 * time.Second))
	st := pr.status()
	if st.Current != nil {
		t.Errorf("Current = %+v, want nil (the whole group expired, anchored to the first member's own queuedAt)", st.Current)
	}
	if st.TotalExpired != 1 {
		t.Errorf("TotalExpired = %d, want 1 (one expired queue item, regardless of its own GroupCount)", st.TotalExpired)
	}
}

func TestTryGroupPriorityAndInsertionSequenceUnchangedByMerge(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	pr.enqueueMatched([]Instance{mkGroupable("a", "u1", 50, 1, now)}, now, staticID)
	pr.enqueueMatched([]Instance{mkInstance("other", 90, now.Add(time.Second))}, now.Add(time.Second), staticID)
	pr.enqueueMatched([]Instance{mkGroupable("b", "u1", 50, 1, now.Add(2*time.Second))}, now.Add(2*time.Second), staticID)

	st := pr.status()
	if len(st.NextQueued) != 2 {
		t.Fatalf("NextQueued = %+v, want 2 items (the group counts as one)", st.NextQueued)
	}
	// The unrelated priority-90 item must still sort first - grouping the
	// Bits pair together never changed its own priority.
	if st.NextQueued[0].Priority != 90 {
		t.Errorf("NextQueued[0].Priority = %d, want 90 (grouping never re-prioritizes)", st.NextQueued[0].Priority)
	}
	if st.NextQueued[1].GroupCount != 2 {
		t.Errorf("NextQueued[1].GroupCount = %d, want 2", st.NextQueued[1].GroupCount)
	}
}

func TestTryGroupEventOutsideWindowStartsNewCandidate(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	pr.enqueueMatched([]Instance{mkGroupable("a", "u1", 50, 1, now)}, now, staticID)
	// group window is 5000ms - an event 6s later must never join it.
	pr.enqueueMatched([]Instance{mkGroupable("b", "u1", 50, 1, now.Add(6*time.Second))}, now.Add(6*time.Second), staticID)

	st := pr.status()
	if st.QueuedCount != 2 {
		t.Fatalf("QueuedCount = %d, want 2 (outside the window, a separate queue item)", st.QueuedCount)
	}
	for _, s := range st.NextQueued {
		if s.GroupCount != 1 {
			t.Errorf("GroupCount = %d, want 1 for every independent item", s.GroupCount)
		}
	}
}

func TestTryGroupLaterMemberNeverExtendsTheWindow(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	pr.enqueueMatched([]Instance{mkGroupable("a", "u1", 50, 1, now)}, now, staticID)
	// +4s: still within the original 5s window - joins.
	pr.enqueueMatched([]Instance{mkGroupable("b", "u1", 50, 1, now.Add(4*time.Second))}, now.Add(4*time.Second), staticID)
	if pr.status().NextQueued[0].GroupCount != 2 {
		t.Fatalf("GroupCount after the +4s member = %d, want 2", pr.status().NextQueued[0].GroupCount)
	}
	// +7s from the ORIGINAL queuedAt (only +3s from the second member) -
	// if the window had been extended by member b, this would still be
	// "within 5s of the last member" and wrongly join; it must not.
	pr.enqueueMatched([]Instance{mkGroupable("c", "u1", 50, 1, now.Add(7*time.Second))}, now.Add(7*time.Second), staticID)
	st := pr.status()
	if st.QueuedCount != 2 {
		t.Fatalf("QueuedCount = %d, want 2 (c must start a new candidate, not extend the window)", st.QueuedCount)
	}
	if st.NextQueued[0].GroupCount != 2 {
		t.Errorf("the original group's own GroupCount changed to %d, want unchanged at 2", st.NextQueued[0].GroupCount)
	}
}

func TestTryGroupMaxGroupSizeStartsNewCandidate(t *testing.T) {
	p := testProfile()
	p.MaxQueueItems = 500
	pr := newProfileRuntime("alprof_1", p, staticID)
	now := time.Now()
	first := mkGroupable("a", "u1", 50, 1, now)
	first.GroupCount = domain.MaxGroupMembers // already at the bound
	pr.enqueueMatched([]Instance{first}, now, staticID)
	pr.enqueueMatched([]Instance{mkGroupable("b", "u1", 50, 1, now.Add(time.Second))}, now.Add(time.Second), staticID)

	st := pr.status()
	if st.QueuedCount != 2 {
		t.Fatalf("QueuedCount = %d, want 2 (a full group never continues incrementing)", st.QueuedCount)
	}
}

func TestTryGroupRejectsDifferentActorProviderAccountEventTypeRuleSnapshot(t *testing.T) {
	now := time.Now()
	cases := map[string]func(Instance) Instance{
		"different actor":     func(v Instance) Instance { v.ActorProviderUserID = "u2"; v.ActorDisplayName = "u2"; return v },
		"different provider":  func(v Instance) Instance { v.ProviderID = "other"; return v },
		"different account":   func(v Instance) Instance { v.ConnectedAccountID = "acct_2"; return v },
		"different eventType": func(v Instance) Instance { v.EventType = domain.EventSubscriptionGiftBatch; return v },
		"different rule id":   func(v Instance) Instance { v.RuleID = "alrule_2"; return v },
		"different rule snapshot (edited)": func(v Instance) Instance {
			v.RuleUpdatedAt = v.RuleUpdatedAt.Add(time.Hour)
			return v
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			pr := newTestRuntime()
			pr.enqueueMatched([]Instance{mkGroupable("a", "u1", 50, 1, now)}, now, staticID)
			candidate := mutate(mkGroupable("b", "u1", 50, 1, now.Add(time.Second)))
			pr.enqueueMatched([]Instance{candidate}, now.Add(time.Second), staticID)
			if st := pr.status(); st.QueuedCount != 2 {
				t.Errorf("QueuedCount = %d, want 2 (must not group across %s)", st.QueuedCount, name)
			}
		})
	}
}

func TestTryGroupRealAndSyntheticNeverGroupTogether(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	pr.enqueueMatched([]Instance{mkGroupable("a", "u1", 50, 1, now)}, now, staticID)
	synthetic := mkGroupable("b", "u1", 50, 1, now.Add(time.Second))
	synthetic.Synthetic = true
	// Route through the real TestRule-style entry point, which never
	// groups at all (groupingEligible excludes every synthetic
	// instance).
	pr.enqueueTest(synthetic, now.Add(time.Second), staticID)
	if st := pr.status(); st.QueuedCount != 2 {
		t.Errorf("QueuedCount = %d, want 2 (a synthetic alert must never merge with a real one)", st.QueuedCount)
	}
}

func TestTryGroupReplayNeverGroups(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	pr.enqueueMatched([]Instance{mkGroupable("a", "u1", 50, 1, now)}, now, staticID)
	pr.tick(now)
	pr.tick(now.Add(6 * time.Second)) // "a" completes -> becomes the replay snapshot
	if err := pr.replayPrevious(); err != nil {
		t.Fatalf("replayPrevious() error = %v", err)
	}
	// A second, otherwise-compatible real event arrives while the replay
	// is pending - it must never merge into the replay slot.
	pr.enqueueMatched([]Instance{mkGroupable("b", "u1", 50, 1, now.Add(7*time.Second))}, now.Add(7*time.Second), staticID)
	if st := pr.status(); st.QueuedCount != 1 {
		t.Errorf("QueuedCount = %d, want 1 (the real event queued normally, never merged with the pending replay)", st.QueuedCount)
	}
}

func TestTryGroupGiftedRecipientNeverGroupsWithGiftBatch(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	// Neither type is groupable at all (see GroupingCapabilities), so
	// even identical actor/provider/account never merges them - this
	// documents the guarantee explicitly rather than relying only on
	// GroupingCapabilities' own table test.
	recipient := mkGroupable("a", "u1", 50, 0, now)
	recipient.EventType = domain.EventGiftedSubscription
	recipient.Quantity = nil
	recipient.AllowGrouping = false // matches real validation: never settable for this type
	batch := mkGroupable("b", "u1", 50, 5, now.Add(time.Second))
	batch.EventType = domain.EventSubscriptionGiftBatch

	pr.enqueueMatched([]Instance{recipient}, now, staticID)
	pr.enqueueMatched([]Instance{batch}, now.Add(time.Second), staticID)
	if st := pr.status(); st.QueuedCount != 2 {
		t.Errorf("QueuedCount = %d, want 2 (gifted_subscription and subscription_gift_batch must never merge)", st.QueuedCount)
	}
}

func TestGroupingCapabilitiesMatchTaskWorkedExamples(t *testing.T) {
	want := map[domain.EventType]bool{
		domain.EventFollow: false, domain.EventSubscription: false, domain.EventResubscription: false,
		domain.EventGiftedSubscription: false, domain.EventSubscriptionGiftBatch: true,
		domain.EventBits: true, domain.EventRaid: false, domain.EventChannelPointRedemption: true,
	}
	for t2, groupable := range want {
		if got := domain.GroupingCapabilityFor(t2).Groupable; got != groupable {
			t.Errorf("GroupingCapabilityFor(%s).Groupable = %v, want %v", t2, got, groupable)
		}
	}
}

// --- preemption tests ---------------------------------------------------

func mkPreemptable(id string, priority int, interruptMode domain.InterruptMode, interruptible bool, queuedAt time.Time) Instance {
	inst := mkInstance(id, priority, queuedAt)
	inst.InterruptMode = interruptMode
	inst.Interruptible = interruptible
	return inst
}

func TestPreemptionDefaultRulesNeverPreempt(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	current := mkPreemptable("current", 10, domain.InterruptNever, true, now)
	current.DurationMS = 30000
	pr.enqueueMatched([]Instance{current}, now, staticID)
	pr.tick(now)

	// Default incoming rule (InterruptNever) with a much higher priority
	// - still must never preempt.
	incoming := mkPreemptable("incoming", 100, domain.InterruptNever, true, now.Add(time.Second))
	pr.enqueueMatched([]Instance{incoming}, now.Add(time.Second), staticID)
	st := pr.status()
	if st.Current == nil || st.Current.Priority != 10 {
		t.Errorf("Current = %+v, want the original priority-10 alert still playing (interruptMode=never)", st.Current)
	}
	if st.QueuedCount != 1 {
		t.Errorf("QueuedCount = %d, want 1 (the higher-priority candidate just queues normally)", st.QueuedCount)
	}
}

func TestPreemptionEqualOrLowerPriorityNeverPreempts(t *testing.T) {
	for _, priority := range []int{50, 40} {
		pr := newTestRuntime()
		now := time.Now()
		current := mkPreemptable("current", 50, domain.InterruptNever, true, now)
		current.DurationMS = 30000
		pr.enqueueMatched([]Instance{current}, now, staticID)
		pr.tick(now)

		incoming := mkPreemptable("incoming", priority, domain.InterruptLowerPriority, true, now.Add(time.Second))
		pr.enqueueMatched([]Instance{incoming}, now.Add(time.Second), staticID)
		if st := pr.status(); st.Current.Priority != 50 {
			t.Errorf("priority %d: Current.Priority = %d, want 50 unchanged (equal/lower never preempts)", priority, st.Current.Priority)
		}
	}
}

func TestPreemptionCurrentNotInterruptibleBlocksEvenEligibleIncoming(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	current := mkPreemptable("current", 10, domain.InterruptNever, false, now)
	current.DurationMS = 30000
	pr.enqueueMatched([]Instance{current}, now, staticID)
	pr.tick(now)

	incoming := mkPreemptable("incoming", 100, domain.InterruptLowerPriority, true, now.Add(time.Second))
	pr.enqueueMatched([]Instance{incoming}, now.Add(time.Second), staticID)
	if st := pr.status(); st.Current.Priority != 10 {
		t.Errorf("Current.Priority = %d, want 10 unchanged (current rule is protected: interruptible=false)", st.Current.Priority)
	}
}

func TestPreemptionEligibleHigherPriorityPreemptsImmediately(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	current := mkPreemptable("current", 10, domain.InterruptNever, true, now)
	current.DurationMS = 30000
	pr.enqueueMatched([]Instance{current}, now, staticID)
	pr.tick(now)

	incoming := mkPreemptable("incoming", 100, domain.InterruptLowerPriority, true, now.Add(time.Second))
	pr.enqueueMatched([]Instance{incoming}, now.Add(time.Second), staticID)
	st := pr.status()
	if st.Current == nil || st.Current.Priority != 100 {
		t.Fatalf("Current = %+v, want the priority-100 incoming alert now playing", st.Current)
	}
	if st.TotalPreempted != 1 {
		t.Errorf("TotalPreempted = %d, want 1", st.TotalPreempted)
	}
	if st.QueuedCount != 0 {
		t.Errorf("QueuedCount = %d, want 0 (preemption never enters the normal queue)", st.QueuedCount)
	}
	if !st.ReplayAvailable {
		t.Error("ReplayAvailable = false, want true (the preempted alert becomes the safe replay snapshot)")
	}
}

func TestPreemptionPausedQueuePreventsIt(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	current := mkPreemptable("current", 10, domain.InterruptNever, true, now)
	current.DurationMS = 30000
	pr.enqueueMatched([]Instance{current}, now, staticID)
	pr.tick(now)
	pr.pause()

	incoming := mkPreemptable("incoming", 100, domain.InterruptLowerPriority, true, now.Add(time.Second))
	pr.enqueueMatched([]Instance{incoming}, now.Add(time.Second), staticID)
	st := pr.status()
	if st.Current.Priority != 10 {
		t.Errorf("Current.Priority = %d, want 10 unchanged (paused prevents preemption)", st.Current.Priority)
	}
	if st.QueuedCount != 1 {
		t.Errorf("QueuedCount = %d, want 1 (queued normally instead)", st.QueuedCount)
	}
}

func TestPreemptionNotCountedAsPlayedOrExpired(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	current := mkPreemptable("current", 10, domain.InterruptNever, true, now)
	current.DurationMS = 30000
	pr.enqueueMatched([]Instance{current}, now, staticID)
	pr.tick(now)
	incoming := mkPreemptable("incoming", 100, domain.InterruptLowerPriority, true, now.Add(time.Second))
	pr.enqueueMatched([]Instance{incoming}, now.Add(time.Second), staticID)

	st := pr.status()
	if st.TotalPlayed != 0 {
		t.Errorf("TotalPlayed = %d, want 0", st.TotalPlayed)
	}
	if st.TotalExpired != 0 {
		t.Errorf("TotalExpired = %d, want 0", st.TotalExpired)
	}
	if st.TotalManuallySkipped != 0 {
		t.Errorf("TotalManuallySkipped = %d, want 0 (preemption is its own separate counter)", st.TotalManuallySkipped)
	}
}

// TestPreemptionStaleTimerCannotHideReplacement reproduces the Stage 12B
// task's own Part 19 scenario: A is playing, B preempts A, A's original
// end time passes, and B must remain correct until B's own duration
// completes - proven here by ticking exactly at A's own original
// (now-abandoned) deadline and confirming B is untouched.
func TestPreemptionStaleTimerCannotHideReplacement(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	a := mkPreemptable("a", 10, domain.InterruptNever, true, now)
	a.DurationMS = 2000 // A's own original deadline: now+2s
	pr.enqueueMatched([]Instance{a}, now, staticID)
	pr.tick(now)

	b := mkPreemptable("b", 100, domain.InterruptLowerPriority, true, now.Add(500*time.Millisecond))
	b.DurationMS = 5000 // B's own deadline: now+500ms+5s = now+5.5s
	pr.enqueueMatched([]Instance{b}, now.Add(500*time.Millisecond), staticID)
	if pr.status().Current.Priority != 100 {
		t.Fatal("B did not become current after preempting A")
	}

	// Tick exactly at A's original (stale) deadline - B must still be
	// playing, not hidden by a stale completion.
	pr.tick(now.Add(2 * time.Second))
	if st := pr.status(); st.Current == nil || st.Current.Priority != 100 {
		t.Fatalf("Current = %+v at A's stale original deadline, want B still playing", st.Current)
	}

	// Tick at B's own real deadline - B must now complete normally.
	pr.tick(now.Add(6 * time.Second))
	if st := pr.status(); st.Current != nil {
		t.Errorf("Current = %+v past B's own real deadline, want nil (B completed on its own schedule)", st.Current)
	}
}

func TestPreemptionLowerPriorityQueuedItemsKeepTheirOrder(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	current := mkPreemptable("current", 10, domain.InterruptNever, true, now)
	current.DurationMS = 30000
	pr.enqueueMatched([]Instance{current}, now, staticID)
	pr.tick(now)
	pr.enqueueMatched([]Instance{mkInstance("q1", 30, now.Add(time.Second))}, now.Add(time.Second), staticID)
	pr.enqueueMatched([]Instance{mkInstance("q2", 20, now.Add(2*time.Second))}, now.Add(2*time.Second), staticID)

	incoming := mkPreemptable("urgent", 100, domain.InterruptLowerPriority, true, now.Add(3*time.Second))
	pr.enqueueMatched([]Instance{incoming}, now.Add(3*time.Second), staticID)

	st := pr.status()
	if len(st.NextQueued) != 2 || st.NextQueued[0].Priority != 30 || st.NextQueued[1].Priority != 20 {
		t.Fatalf("NextQueued = %+v, want [30, 20] unchanged by the preemption", st.NextQueued)
	}
}

func TestPreemptionThenNormalQueueSelectionResumes(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	current := mkPreemptable("current", 10, domain.InterruptNever, true, now)
	current.DurationMS = 1000
	pr.enqueueMatched([]Instance{current}, now, staticID)
	pr.tick(now)
	pr.enqueueMatched([]Instance{mkInstance("q1", 30, now)}, now, staticID)

	incoming := mkPreemptable("urgent", 100, domain.InterruptLowerPriority, true, now.Add(200*time.Millisecond))
	incoming.DurationMS = 1000
	pr.enqueueMatched([]Instance{incoming}, now.Add(200*time.Millisecond), staticID)

	// The urgent alert finishes on its own schedule; the next tick must
	// promote the highest-priority *queued* item normally.
	pr.tick(now.Add(1300 * time.Millisecond))
	st := pr.status()
	if st.Current == nil || st.Current.Priority != 30 {
		t.Fatalf("Current = %+v after the urgent alert finished, want the priority-30 queued item promoted normally", st.Current)
	}
}

func TestPreemptionReplayNeverPreempts(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	// Build a replay-eligible snapshot with a high priority and
	// interruptMode enabled - if replay bypassed canPreemptLocked, this
	// would wrongly preempt.
	completed := mkPreemptable("done", 100, domain.InterruptLowerPriority, true, now)
	completed.DurationMS = 1000
	pr.enqueueMatched([]Instance{completed}, now, staticID)
	pr.tick(now)
	pr.tick(now.Add(2 * time.Second)) // completes -> replay snapshot

	current := mkPreemptable("current", 10, domain.InterruptNever, true, now.Add(2*time.Second))
	current.DurationMS = 30000
	pr.enqueueMatched([]Instance{current}, now.Add(2*time.Second), staticID)
	pr.tick(now.Add(2 * time.Second))

	if err := pr.replayPrevious(); err != nil {
		t.Fatalf("replayPrevious() error = %v", err)
	}
	pr.tick(now.Add(2 * time.Second))
	if st := pr.status(); st.Current == nil || st.Current.Priority != 10 {
		t.Fatalf("Current = %+v, want the priority-10 alert still playing (replay must never preempt)", st.Current)
	}
}

func TestPreemptionSyntheticNeverPreemptsReal(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	current := mkPreemptable("current", 10, domain.InterruptNever, true, now)
	current.DurationMS = 30000
	pr.enqueueMatched([]Instance{current}, now, staticID)
	pr.tick(now)

	synthetic := mkPreemptable("synthetic", 100, domain.InterruptLowerPriority, true, now.Add(time.Second))
	synthetic.Synthetic = true
	pr.enqueueTest(synthetic, now.Add(time.Second), staticID)
	if st := pr.status(); st.Current.Priority != 10 {
		t.Errorf("Current.Priority = %d, want 10 unchanged (a synthetic candidate must never preempt a real current)", st.Current.Priority)
	}
}

func TestPreemptionRealMayPreemptSyntheticCurrent(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	syntheticCurrent := mkPreemptable("synthetic-current", 10, domain.InterruptNever, true, now)
	syntheticCurrent.DurationMS = 30000
	syntheticCurrent.Synthetic = true
	pr.enqueueTest(syntheticCurrent, now, staticID)
	pr.tick(now)
	if !pr.status().Current.Synthetic {
		t.Fatal("expected the synthetic alert to be current")
	}

	real := mkPreemptable("real", 100, domain.InterruptLowerPriority, true, now.Add(time.Second))
	pr.enqueueMatched([]Instance{real}, now.Add(time.Second), staticID)
	st := pr.status()
	if st.Current == nil || st.Current.Synthetic {
		t.Fatalf("Current = %+v, want the real alert now playing (a real alert may preempt a synthetic current)", st.Current)
	}
}

func TestPreemptionSyntheticMayPreemptSyntheticCurrent(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	first := mkPreemptable("first-synthetic", 10, domain.InterruptNever, true, now)
	first.DurationMS = 30000
	first.Synthetic = true
	pr.enqueueTest(first, now, staticID)
	pr.tick(now)

	second := mkPreemptable("second-synthetic", 100, domain.InterruptLowerPriority, true, now.Add(time.Second))
	second.Synthetic = true
	stored, accepted := pr.enqueueTest(second, now.Add(time.Second), staticID)
	if !accepted {
		t.Fatal("enqueueTest(second synthetic) = not accepted, want accepted")
	}
	if !stored.Synthetic || stored.Priority != 100 {
		t.Fatalf("stored = %+v, want the second synthetic instance", stored)
	}
	if st := pr.status(); st.Current == nil || st.Current.Priority != 100 {
		t.Fatalf("Current = %+v, want the second synthetic alert now playing (synthetic may preempt synthetic)", st.Current)
	}
}

func TestPreemptionHideRevisionCarriesReasonAndNoPriorContent(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()

	// Subscribe BEFORE the first alert ever starts, so the only "show" in
	// this subscription's own stream is the one produced by the
	// preemption itself, not a replayed original one.
	sub, _, err := pr.subscribe(0)
	if err != nil {
		t.Fatalf("subscribe() error = %v", err)
	}
	defer sub.Cancel()

	current := mkPreemptable("current", 10, domain.InterruptNever, true, now)
	current.DurationMS = 30000
	current.RenderedText = "secret rendered text"
	pr.enqueueMatched([]Instance{current}, now, staticID)
	pr.tick(now)

	incoming := mkPreemptable("urgent", 100, domain.InterruptLowerPriority, true, now.Add(time.Second))
	pr.enqueueMatched([]Instance{incoming}, now.Add(time.Second), staticID)

	var revisions []Revision
	for i := 0; i < 4; i++ {
		select {
		case rev := <-sub.Revisions():
			revisions = append(revisions, rev)
		case <-time.After(time.Second):
		}
	}
	// Expect exactly: show(current), hide(preempted), show(urgent).
	if len(revisions) < 3 {
		t.Fatalf("received %d revisions, want at least 3 (show, hide, show): %+v", len(revisions), revisions)
	}
	hideRev, showRev := revisions[1], revisions[2]
	if hideRev.Operation != OpHide {
		t.Fatalf("revisions[1].Operation = %q, want %q: %+v", hideRev.Operation, OpHide, revisions)
	}
	if showRev.Operation != OpShow {
		t.Fatalf("revisions[2].Operation = %q, want %q: %+v", showRev.Operation, OpShow, revisions)
	}
	if hideRev.Sequence >= showRev.Sequence {
		t.Errorf("hide sequence %d is not before show sequence %d", hideRev.Sequence, showRev.Sequence)
	}
	if hideRev.Reason != HideReasonPreempted {
		t.Errorf("hide reason = %q, want %q", hideRev.Reason, HideReasonPreempted)
	}
	if hideRev.Alert != nil {
		t.Error("hide revision carries a non-nil Alert, want nil (no prior rendered content)")
	}
	if showRev.Alert == nil || showRev.Alert.RenderedText == "secret rendered text" {
		t.Errorf("show revision after preemption = %+v, want the NEW alert's own content, never the outgoing one's", showRev.Alert)
	}
}

// TestPreemptionConcurrentEnqueueMatchedIsSerialized proves the shared
// mutex correctly serializes two goroutines racing to preempt/enqueue for
// the same profile at once (Stage 12B task Part 28) - the total accepted
// item count must be exactly as expected, with no lost or duplicated
// state, regardless of goroutine interleaving.
func TestPreemptionConcurrentEnqueueMatchedIsSerialized(t *testing.T) {
	pr := newTestRuntime()
	now := time.Now()
	current := mkPreemptable("current", 10, domain.InterruptNever, true, now)
	current.DurationMS = 30000
	pr.enqueueMatched([]Instance{current}, now, staticID)
	pr.tick(now)

	// 20 concurrent candidates with distinct, strictly increasing
	// priorities, each individually eligible to preempt. profileRuntime's
	// own mutex serializes every call regardless of goroutine scheduling
	// order, so the final winner is fully deterministic: whichever
	// instant the globally-highest-priority candidate (69) actually runs,
	// nothing submitted here can ever outrank or displace it afterward -
	// proving no corruption (a lost update, a torn read, a duplicated
	// playback timer) occurred under concurrent access.
	const n = 20
	const maxPriority = 50 + n - 1
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(priority int) {
			defer wg.Done()
			inst := mkPreemptable("c", priority, domain.InterruptLowerPriority, true, now)
			pr.enqueueMatched([]Instance{inst}, now, staticID)
		}(50 + i)
	}
	wg.Wait()

	st := pr.status()
	if st.Current == nil || st.Current.Priority != maxPriority {
		t.Fatalf("Current = %+v, want the single globally-highest-priority candidate (%d)", st.Current, maxPriority)
	}
}
