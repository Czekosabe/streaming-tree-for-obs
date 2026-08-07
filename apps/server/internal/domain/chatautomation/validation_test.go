package chatautomation

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	if err := ValidateName("Hourly reminder"); err != nil {
		t.Errorf("ValidateName(valid) error = %v", err)
	}
	if err := ValidateName(""); err == nil {
		t.Error("ValidateName(empty) = nil, want error")
	}
	if err := ValidateName(strings.Repeat("a", MaxNameCodePoints+1)); err == nil {
		t.Error("ValidateName(too long) = nil, want error")
	}
}

func TestValidateScheduleTiming(t *testing.T) {
	cases := []struct {
		name                                              string
		interval, firstDelay, jitter, minChat, maxPerHour int
		wantErr                                           bool
	}{
		{"valid", 3600, 0, 0, 0, 10, false},
		{"interval too small", 59, 0, 0, 0, 10, true},
		{"interval too large", MaxIntervalSeconds + 1, 0, 0, 0, 10, true},
		{"negative first delay", 3600, -1, 0, 0, 10, true},
		{"first delay too large", 3600, MaxFirstDelaySeconds + 1, 0, 0, 10, true},
		{"jitter too large", 3600, 0, MaxJitterSeconds + 1, 0, 10, true},
		{"minimum chat too large", 3600, 0, 0, MaxMinimumChatMessages + 1, 10, true},
		{"max per hour zero", 3600, 0, 0, 0, 0, true},
		{"max per hour too large", 3600, 0, 0, 0, MaxMaximumSendsPerHour + 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateScheduleTiming(tc.interval, tc.firstDelay, tc.jitter, tc.minChat, tc.maxPerHour)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateScheduleTiming() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateMessages(t *testing.T) {
	if err := ValidateMessages(nil); !errors.Is(err, ErrMessageRequired) {
		t.Errorf("ValidateMessages(nil) = %v, want ErrMessageRequired", err)
	}
	if err := ValidateMessages([]string{"hi"}); err != nil {
		t.Errorf("ValidateMessages(one) error = %v", err)
	}
	many := make([]string, MaxMessagesPerSchedule+1)
	for i := range many {
		many[i] = "x"
	}
	if err := ValidateMessages(many); err == nil {
		t.Error("ValidateMessages(too many) = nil, want error")
	}
	if err := ValidateMessages([]string{strings.Repeat("a", MaxTemplateCodePoints+1)}); err == nil {
		t.Error("ValidateMessages(template too long) = nil, want error")
	}
	if err := ValidateMessages([]string{"   "}); err == nil {
		t.Error("ValidateMessages(blank template) = nil, want error")
	}
}

func TestValidateTargets(t *testing.T) {
	if err := ValidateTargets(nil); !errors.Is(err, ErrTargetRequired) {
		t.Errorf("ValidateTargets(nil) = %v, want ErrTargetRequired", err)
	}
	if err := ValidateTargets([]Target{{AccountID: "a"}, {AccountID: "a"}}); err == nil {
		t.Error("ValidateTargets(duplicate) = nil, want error")
	}
	if err := ValidateTargets([]Target{{AccountID: "a"}, {AccountID: "b"}}); err != nil {
		t.Errorf("ValidateTargets(valid) error = %v", err)
	}
}

func TestValidateCommandName(t *testing.T) {
	valid := []string{"discord", "socials", "uptime", "commands", "a", "a-b_c9"}
	for _, name := range valid {
		if err := ValidateCommandName(name); err != nil {
			t.Errorf("ValidateCommandName(%q) error = %v, want nil", name, err)
		}
	}
	invalid := []string{"", "Discord", "!discord", "dis cord", "dis/cord", "dïscord", strings.Repeat("a", MaxCommandNameLength+1)}
	for _, name := range invalid {
		if err := ValidateCommandName(name); err == nil {
			t.Errorf("ValidateCommandName(%q) = nil, want error", name)
		}
	}
}

func TestNormalizeCommandName(t *testing.T) {
	if got := NormalizeCommandName("  Discord  "); got != "discord" {
		t.Errorf("NormalizeCommandName() = %q, want discord", got)
	}
}

func TestValidateAliases(t *testing.T) {
	if err := ValidateAliases("discord", []string{"disc", "server"}); err != nil {
		t.Errorf("ValidateAliases(valid) error = %v", err)
	}
	if err := ValidateAliases("discord", []string{"discord"}); err == nil {
		t.Error("ValidateAliases(alias equals name) = nil, want error")
	}
	if err := ValidateAliases("discord", []string{"disc", "disc"}); err == nil {
		t.Error("ValidateAliases(duplicate alias) = nil, want error")
	}
}

func TestValidateRole(t *testing.T) {
	for _, r := range ValidRoles {
		if err := ValidateRole(r); err != nil {
			t.Errorf("ValidateRole(%q) error = %v", r, err)
		}
	}
	if err := ValidateRole(Role("follower")); err == nil {
		t.Error("ValidateRole(follower) = nil, want error - follower is deliberately excluded")
	}
}

func TestValidateCooldowns(t *testing.T) {
	if err := ValidateCooldowns(0, 0); err != nil {
		t.Errorf("ValidateCooldowns(0,0) error = %v", err)
	}
	if err := ValidateCooldowns(-1, 0); err == nil {
		t.Error("ValidateCooldowns(negative global) = nil, want error")
	}
	if err := ValidateCooldowns(0, MaxUserCooldownSeconds+1); err == nil {
		t.Error("ValidateCooldowns(user too large) = nil, want error")
	}
	if err := ValidateCooldowns(MaxGlobalCooldownSeconds+1, 0); err == nil {
		t.Error("ValidateCooldowns(global too large) = nil, want error")
	}
}
