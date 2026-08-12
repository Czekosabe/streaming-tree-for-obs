package streamelements

import (
	"errors"
	"testing"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
)

func allowedTip() Tip {
	return Tip{
		Donation: TipDonation{
			User:          TipUser{Username: "Styler", Geo: "ZZ", Email: "styler@streamelements.com", Channel: "5ad23dcc18fff500d78c5348"},
			Message:       "great stream!",
			Amount:        num("4.2"),
			Currency:      "usd",
			PaymentMethod: "scheme",
		},
		ID: "67b5f39d07ecd4c594e60f73", Channel: "5ad23dcc18fff500d78c5348",
		Provider: "paypal", Approved: ApprovedAllowed, Status: StatusSuccess,
		CreatedAt: "2025-02-19T15:07:09.302Z", UpdatedAt: "2025-02-19T15:07:17.099Z",
		TransactionID: "2YH79902JR1691017",
	}
}

func TestNormalizeTipAllowedProducesADonationEvent(t *testing.T) {
	evt, err := NormalizeTip("donsrc_1", allowedTip())
	if err != nil {
		t.Fatalf("NormalizeTip() error = %v", err)
	}
	if evt.Type != engagement.TypeDonation {
		t.Errorf("Type = %q, want donation", evt.Type)
	}
	if evt.ProviderID != engagement.ProviderStreamElements {
		t.Errorf("ProviderID = %q, want streamelements", evt.ProviderID)
	}
	if evt.ConnectedAccountID != "donsrc_1" {
		t.Errorf("ConnectedAccountID = %q, want donsrc_1", evt.ConnectedAccountID)
	}
	if evt.ProviderEventID != "67b5f39d07ecd4c594e60f73" || evt.DedupeKey != "67b5f39d07ecd4c594e60f73" {
		t.Errorf("ProviderEventID/DedupeKey = %q/%q, want the tip's own _id", evt.ProviderEventID, evt.DedupeKey)
	}
	if evt.Money == nil || evt.Money.AmountMicros != 4_200_000 || evt.Money.Currency != "USD" {
		t.Fatalf("Money = %+v, want 4200000 USD", evt.Money)
	}
	if evt.User == nil || evt.User.DisplayName != "Styler" || evt.User.Anonymous {
		t.Fatalf("User = %+v, want DisplayName=Styler, Anonymous=false", evt.User)
	}
	if evt.Message == nil || evt.Message.Text != "great stream!" {
		t.Fatalf("Message = %+v, want \"great stream!\"", evt.Message)
	}
}

func TestNormalizeTipPendingIsNotPublishable(t *testing.T) {
	tip := allowedTip()
	tip.Approved = ApprovedPending
	if _, err := NormalizeTip("donsrc_1", tip); !errors.Is(err, ErrTipNotPublishable) {
		t.Fatalf("NormalizeTip(pending) error = %v, want ErrTipNotPublishable", err)
	}
}

func TestNormalizeTipRejectedIsNeverPublishable(t *testing.T) {
	tip := allowedTip()
	tip.Approved = ApprovedRejected
	if _, err := NormalizeTip("donsrc_1", tip); !errors.Is(err, ErrTipNotPublishable) {
		t.Fatalf("NormalizeTip(rejected) error = %v, want ErrTipNotPublishable", err)
	}
}

func TestNormalizeTipNonSuccessStatusIsNotPublishable(t *testing.T) {
	tip := allowedTip()
	tip.Status = "failed"
	if _, err := NormalizeTip("donsrc_1", tip); !errors.Is(err, ErrTipNotPublishable) {
		t.Fatalf("NormalizeTip(status=failed) error = %v, want ErrTipNotPublishable", err)
	}
	tip.Status = "pending"
	if _, err := NormalizeTip("donsrc_1", tip); !errors.Is(err, ErrTipNotPublishable) {
		t.Fatalf("NormalizeTip(status=pending) error = %v, want ErrTipNotPublishable", err)
	}
	tip.Status = "unknown_future_value"
	if _, err := NormalizeTip("donsrc_1", tip); !errors.Is(err, ErrTipNotPublishable) {
		t.Fatalf("NormalizeTip(status=unknown) error = %v, want ErrTipNotPublishable (never guessed as publishable)", err)
	}
}

func TestNormalizeTipEmptyUsernameIsAnonymousNeverFabricated(t *testing.T) {
	tip := allowedTip()
	tip.Donation.User.Username = ""
	evt, err := NormalizeTip("donsrc_1", tip)
	if err != nil {
		t.Fatalf("NormalizeTip() error = %v", err)
	}
	if evt.User == nil || !evt.User.Anonymous || evt.User.DisplayName != "" {
		t.Fatalf("User = %+v, want Anonymous=true, DisplayName empty", evt.User)
	}
	if evt.User.ProviderUserID != "" {
		t.Fatalf("ProviderUserID = %q, want empty - never fabricate a stable donor id", evt.User.ProviderUserID)
	}
}

func TestNormalizeTipEmptyMessageIsNilNotEmptyString(t *testing.T) {
	tip := allowedTip()
	tip.Donation.Message = ""
	evt, err := NormalizeTip("donsrc_1", tip)
	if err != nil {
		t.Fatalf("NormalizeTip() error = %v", err)
	}
	if evt.Message != nil {
		t.Fatalf("Message = %+v, want nil for an empty donation message", evt.Message)
	}
}

func TestNormalizeTipMalformedAmountIsRejected(t *testing.T) {
	tip := allowedTip()
	tip.Donation.Amount = num("not-a-number")
	if _, err := NormalizeTip("donsrc_1", tip); !errors.Is(err, ErrAmountMalformed) {
		t.Fatalf("NormalizeTip() error = %v, want ErrAmountMalformed", err)
	}
}

func TestNormalizeTipInvalidCreatedAtIsRejected(t *testing.T) {
	tip := allowedTip()
	tip.CreatedAt = "not-a-timestamp"
	if _, err := NormalizeTip("donsrc_1", tip); !errors.Is(err, ErrMalformedPayload) {
		t.Fatalf("NormalizeTip() error = %v, want ErrMalformedPayload", err)
	}
}

// TestNormalizeTipNeverExposesSensitiveFields is the explicit privacy-
// boundary regression test docs/provider-integrations/
// external-donations.md §22 requires: email, geo, paymentMethod,
// transactionId, and the payment-rail "provider" value must never appear
// anywhere in the normalized Event - not in a struct field, and not
// smuggled into ProviderExtra.
func TestNormalizeTipNeverExposesSensitiveFields(t *testing.T) {
	tip := allowedTip()
	evt, err := NormalizeTip("donsrc_1", tip)
	if err != nil {
		t.Fatalf("NormalizeTip() error = %v", err)
	}
	if evt.User.Login != "" {
		t.Errorf("User.Login = %q, want empty (email/geo must never reach any User field)", evt.User.Login)
	}
	if evt.User.AvatarURL != "" {
		t.Errorf("User.AvatarURL = %q, want empty", evt.User.AvatarURL)
	}
	if len(evt.ProviderExtra) != 0 {
		t.Errorf("ProviderExtra = %+v, want empty - no sensitive field smuggled through it", evt.ProviderExtra)
	}
	if evt.ModerationRef != "" || evt.ModerationAction != "" {
		t.Errorf("ModerationRef/ModerationAction = %q/%q, want empty", evt.ModerationRef, evt.ModerationAction)
	}
}

func TestParseTipRejectsMissingID(t *testing.T) {
	if _, err := ParseTip([]byte(`{"donation":{"amount":5,"currency":"USD"},"approved":"allowed","status":"success"}`)); !errors.Is(err, ErrMissingTipID) {
		t.Fatalf("ParseTip() error = %v, want ErrMissingTipID", err)
	}
}

func TestParseTipRejectsMalformedJSON(t *testing.T) {
	if _, err := ParseTip([]byte(`{not json`)); !errors.Is(err, ErrMalformedPayload) {
		t.Fatalf("ParseTip() error = %v, want ErrMalformedPayload", err)
	}
}

func TestTipPublishableRequiresBothAllowedAndSuccess(t *testing.T) {
	cases := []struct {
		approved, status string
		want             bool
	}{
		{ApprovedAllowed, StatusSuccess, true},
		{ApprovedPending, StatusSuccess, false},
		{ApprovedRejected, StatusSuccess, false},
		{ApprovedAllowed, "failed", false},
		{ApprovedAllowed, "", false},
	}
	for _, c := range cases {
		tip := Tip{Approved: c.approved, Status: c.status}
		if got := tip.Publishable(); got != c.want {
			t.Errorf("Tip{Approved:%q,Status:%q}.Publishable() = %v, want %v", c.approved, c.status, got, c.want)
		}
	}
}
