package alerts

import (
	"errors"
	"testing"
)

func TestValidateThresholds(t *testing.T) {
	i := func(v int64) *int64 { return &v }
	cases := []struct {
		name     string
		min, max *int64
		wantErr  bool
	}{
		{"both nil", nil, nil, false},
		{"min only", i(1), nil, false},
		{"max only", nil, i(100), false},
		{"equal bounds", i(50), i(50), false},
		{"min < max", i(1), i(100), false},
		{"min > max", i(100), i(1), true},
		{"negative min", i(-1), nil, true},
		{"negative max", nil, i(-1), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateThresholds(c.min, c.max)
			if c.wantErr && err == nil {
				t.Errorf("ValidateThresholds(%v,%v) = nil, want an error", c.min, c.max)
			}
			if !c.wantErr && err != nil {
				t.Errorf("ValidateThresholds(%v,%v) = %v, want nil", c.min, c.max, err)
			}
		})
	}
}

func TestValidateRuleFieldsDurationBounds(t *testing.T) {
	r := baseValidRule()
	r.DurationMS = 999
	if err := ValidateRuleFields(r); !errors.Is(err, ErrValidation) {
		t.Errorf("DurationMS=999 error = %v, want ErrValidation", err)
	}
	r.DurationMS = 30001
	if err := ValidateRuleFields(r); !errors.Is(err, ErrValidation) {
		t.Errorf("DurationMS=30001 error = %v, want ErrValidation", err)
	}
	r.DurationMS = 30000
	r.MinimumQuantity, r.MaximumQuantity = nil, nil
	if err := ValidateRuleFields(r); err != nil {
		t.Errorf("DurationMS=30000 (boundary) error = %v, want nil", err)
	}
}

func TestValidateRuleFieldsPriorityBounds(t *testing.T) {
	r := baseValidRule()
	r.Priority = -1
	if err := ValidateRuleFields(r); !errors.Is(err, ErrValidation) {
		t.Errorf("Priority=-1 error = %v, want ErrValidation", err)
	}
	r.Priority = 101
	if err := ValidateRuleFields(r); !errors.Is(err, ErrValidation) {
		t.Errorf("Priority=101 error = %v, want ErrValidation", err)
	}
}

func TestValidateRuleFieldsTemplateTooLong(t *testing.T) {
	r := baseValidRule()
	long := make([]byte, MaxTemplateCodePoints+1)
	for i := range long {
		long[i] = 'a'
	}
	r.TextTemplate = string(long)
	if err := ValidateRuleFields(r); !errors.Is(err, ErrValidation) {
		t.Errorf("over-long template error = %v, want ErrValidation", err)
	}
}

func TestValidateRuleFieldsUnknownEventType(t *testing.T) {
	r := baseValidRule()
	r.EventType = EventType("donation")
	if err := ValidateRuleFields(r); !errors.Is(err, ErrValidation) {
		t.Errorf("unknown event type error = %v, want ErrValidation", err)
	}
}

func TestValidateRuleConditionsBitsAllowsAnonymityImplicitly(t *testing.T) {
	r := baseValidRule()
	r.EventType = EventBits
	r.ShowQuantity = true
	if err := ValidateRuleConditions(r); err != nil {
		t.Errorf("bits with ShowQuantity error = %v, want nil", err)
	}
}

func TestValidateRuleConditionsChannelPointRedemptionAllowsMessage(t *testing.T) {
	r := baseValidRule()
	r.EventType = EventChannelPointRedemption
	r.ShowMessage = true
	if err := ValidateRuleConditions(r); err != nil {
		t.Errorf("channel_point_redemption with ShowMessage error = %v, want nil", err)
	}
}

func TestValidateRuleConditionsRaidRejectsMessage(t *testing.T) {
	r := baseValidRule()
	r.EventType = EventRaid
	r.ShowMessage = true
	if !errors.Is(ValidateRuleConditions(r), ErrConditionUnsupported) {
		t.Error("raid with ShowMessage should be rejected as unsupported")
	}
}

func TestValidateRuleFieldsGroupWindowBounds(t *testing.T) {
	r := baseValidRule()
	r.GroupWindowMS = MinGroupWindowMS - 1
	if err := ValidateRuleFields(r); !errors.Is(err, ErrValidation) {
		t.Errorf("GroupWindowMS below minimum error = %v, want ErrValidation", err)
	}
	r.GroupWindowMS = MaxGroupWindowMS + 1
	if err := ValidateRuleFields(r); !errors.Is(err, ErrValidation) {
		t.Errorf("GroupWindowMS above maximum error = %v, want ErrValidation", err)
	}
	r.GroupWindowMS = MinGroupWindowMS
	if err := ValidateRuleFields(r); err != nil {
		t.Errorf("GroupWindowMS at the minimum boundary error = %v, want nil", err)
	}
	r.GroupWindowMS = MaxGroupWindowMS
	if err := ValidateRuleFields(r); err != nil {
		t.Errorf("GroupWindowMS at the maximum boundary error = %v, want nil", err)
	}
}

func TestValidateRuleFieldsGroupWindowBoundsEnforcedEvenWhenGroupingDisabled(t *testing.T) {
	r := baseValidRule()
	r.AllowGrouping = false
	r.GroupWindowMS = 0
	if err := ValidateRuleFields(r); !errors.Is(err, ErrValidation) {
		t.Errorf("GroupWindowMS=0 with grouping disabled error = %v, want ErrValidation (bounds are unconditional)", err)
	}
}

func TestValidateRuleFieldsUnknownInterruptMode(t *testing.T) {
	r := baseValidRule()
	r.InterruptMode = InterruptMode("sometimes")
	if err := ValidateRuleFields(r); !errors.Is(err, ErrValidation) {
		t.Errorf("unknown interrupt mode error = %v, want ErrValidation", err)
	}
}

func TestValidateRuleConditionsGroupingRejectedForUngroupableType(t *testing.T) {
	r := baseValidRule()
	r.EventType = EventFollow
	r.AllowGrouping = true
	if !errors.Is(ValidateRuleConditions(r), ErrConditionUnsupported) {
		t.Error("AllowGrouping=true on a follow rule should be rejected as unsupported")
	}
}

func TestValidateRuleConditionsGroupingAllowedForBitsWithoutMessage(t *testing.T) {
	r := baseValidRule()
	r.EventType = EventBits
	r.ShowQuantity = true
	r.ShowMessage = false
	r.AllowGrouping = true
	if err := ValidateRuleConditions(r); err != nil {
		t.Errorf("groupable bits rule with ShowMessage=false error = %v, want nil", err)
	}
}

func TestValidateRuleConditionsGroupingRejectedForBitsWithMessage(t *testing.T) {
	r := baseValidRule()
	r.EventType = EventBits
	r.ShowMessage = true
	r.AllowGrouping = true
	if !errors.Is(ValidateRuleConditions(r), ErrConditionUnsupported) {
		t.Error("AllowGrouping=true with ShowMessage=true on Bits should be rejected (Part 11)")
	}
}

func TestValidateRuleConditionsGroupingAllowedForGiftBatchAndRedemption(t *testing.T) {
	giftBatch := baseValidRule()
	giftBatch.EventType = EventSubscriptionGiftBatch
	giftBatch.ShowQuantity = true
	giftBatch.AllowGrouping = true
	if err := ValidateRuleConditions(giftBatch); err != nil {
		t.Errorf("groupable gift-batch rule error = %v, want nil", err)
	}

	redemption := baseValidRule()
	redemption.EventType = EventChannelPointRedemption
	redemption.ShowMessage = false
	redemption.AllowGrouping = true
	if err := ValidateRuleConditions(redemption); err != nil {
		t.Errorf("groupable redemption rule (ShowMessage=false) error = %v, want nil", err)
	}
	redemption.ShowMessage = true
	if !errors.Is(ValidateRuleConditions(redemption), ErrConditionUnsupported) {
		t.Error("groupable redemption rule with ShowMessage=true should be rejected")
	}
}

func TestValidateProviders(t *testing.T) {
	if err := ValidateProviders(nil); err != nil {
		t.Errorf("ValidateProviders(nil) error = %v, want nil (means 'any')", err)
	}
	if err := ValidateProviders([]ProviderID{ProviderTwitch}); err != nil {
		t.Errorf("ValidateProviders([twitch]) error = %v, want nil", err)
	}
	if err := ValidateProviders([]ProviderID{"kick"}); err == nil {
		t.Error("ValidateProviders([kick]) = nil, want an error (unsupported provider)")
	}
	if err := ValidateProviders([]ProviderID{ProviderTwitch, ProviderTwitch}); err == nil {
		t.Error("ValidateProviders with a duplicate = nil, want an error")
	}
}

func baseValidRule() Rule {
	return Rule{
		ProfileID: "alprof_1", Name: "Follow alert", EventType: EventFollow,
		Priority: DefaultPriority, DurationMS: DefaultDurationMS, RequiredRole: RoleEveryone,
		ShowPlatform: true, ShowUsername: true,
		TextTemplate:   "{username} just followed!",
		EntryAnimation: AnimationFade, ExitAnimation: AnimationFade, AnimationDurationMS: DefaultAnimationDurationMS,
		GroupWindowMS: DefaultGroupWindowMS, InterruptMode: InterruptNever, Interruptible: true,
	}
}
