package engagement

import "testing"

func TestNewMoneyNormalizesCurrencyToUppercase(t *testing.T) {
	m, err := NewMoney(1_750_000, "usd", "$1.75")
	if err != nil {
		t.Fatalf("expected valid money, got %v", err)
	}
	if m.Currency != "USD" {
		t.Fatalf("expected uppercase currency, got %q", m.Currency)
	}
	if m.AmountMicros != 1_750_000 {
		t.Fatalf("expected 1750000 micros, got %d", m.AmountMicros)
	}
	if m.DisplayAmount != "$1.75" {
		t.Fatalf("expected display amount preserved, got %q", m.DisplayAmount)
	}
}

func TestNewMoneyRejectsNegativeAmount(t *testing.T) {
	if _, err := NewMoney(-1, "USD", ""); err == nil {
		t.Fatal("expected error for negative amount")
	}
}

func TestNewMoneyRejectsOverflowAmount(t *testing.T) {
	if _, err := NewMoney(maxAmountMicros+1, "USD", ""); err == nil {
		t.Fatal("expected error for amount exceeding the supported bound")
	}
}

func TestNewMoneyAcceptsZeroAmount(t *testing.T) {
	m, err := NewMoney(0, "USD", "")
	if err != nil {
		t.Fatalf("expected zero amount to be valid, got %v", err)
	}
	if m.AmountMicros != 0 {
		t.Fatalf("expected 0 micros, got %d", m.AmountMicros)
	}
}

func TestNewMoneyRejectsEmptyCurrency(t *testing.T) {
	if _, err := NewMoney(100, "", ""); err == nil {
		t.Fatal("expected error for empty currency")
	}
}

func TestValidateRequiresMoneyForSuperChatAndSuperSticker(t *testing.T) {
	for _, ty := range []Type{TypeYouTubeSuperChat, TypeYouTubeSuperSticker} {
		e := baseEvent()
		e.ProviderID = ProviderYouTube
		e.Type = ty
		if err := e.Validate(); err == nil {
			t.Fatalf("expected error: %s with no money", ty)
		}
		money, err := NewMoney(1_000_000, "USD", "$1.00")
		if err != nil {
			t.Fatalf("unexpected NewMoney error: %v", err)
		}
		e.Money = &money
		if err := e.Validate(); err != nil {
			t.Fatalf("expected valid %s with money, got %v", ty, err)
		}
	}
}

func TestValidateRejectsNegativeMoneyAmountBypassingNewMoney(t *testing.T) {
	e := baseEvent()
	e.ProviderID = ProviderYouTube
	e.Type = TypeYouTubeSuperChat
	e.Money = &Money{AmountMicros: -1, Currency: "USD"}
	if err := e.Validate(); err == nil {
		t.Fatal("expected error: negative money amount")
	}
}

func TestValidateRejectsMoneyWithEmptyCurrencyBypassingNewMoney(t *testing.T) {
	e := baseEvent()
	e.ProviderID = ProviderYouTube
	e.Type = TypeYouTubeSuperChat
	e.Money = &Money{AmountMicros: 100}
	if err := e.Validate(); err == nil {
		t.Fatal("expected error: money with empty currency")
	}
}

func TestValidateAcceptsYouTubeMembershipTypes(t *testing.T) {
	for _, ty := range []Type{TypeYouTubeMembership, TypeYouTubeMembershipMilestone} {
		e := baseEvent()
		e.ProviderID = ProviderYouTube
		e.Type = ty
		e.User = &User{ProviderUserID: "u1"}
		if err := e.Validate(); err != nil {
			t.Fatalf("expected valid %s, got %v", ty, err)
		}
	}
}

func TestKnownTypesIncludesYouTubeTypes(t *testing.T) {
	for _, ty := range []Type{
		TypeYouTubeMembership, TypeYouTubeMembershipMilestone,
		TypeYouTubeSuperChat, TypeYouTubeSuperSticker,
	} {
		if !ty.Known() {
			t.Fatalf("expected %s to be a known type", ty)
		}
	}
}
