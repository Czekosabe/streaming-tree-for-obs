package goals

import "time"

// Goal is one persisted, accumulating goal/counter (docs/goals-
// widgets.md §8). Current means "observed contributions since Baseline"
// - never a provider-canonical total (§1).
type Goal struct {
	ID      string
	Name    string
	Kind    Kind
	Enabled bool

	// Target/Current/Baseline are plain integer counts for
	// KindFollowers/KindSubscriptions/KindBits, and AmountMicros for
	// KindDonations - never a float, bounds enforced by validation.go.
	Target   int64
	Current  int64
	Baseline int64

	// Currency is required and non-empty only when Kind.RequiresCurrency()
	// - an uppercase, provider-style code (e.g. "USD"). Empty otherwise.
	Currency string

	// Providers/Accounts filters: empty means "any" - mirrors
	// alerts.Rule's own identical convention. Accounts holds a
	// connected_accounts id or a donation source id, both validated for
	// existence by AccountLookup (see service.go).
	Providers []ProviderID
	Accounts  []string

	CreatedAt time.Time
	UpdatedAt time.Time
	// StartedAt is set at creation and refreshed by an explicit Reset or
	// a Baseline-changing reconfigure (docs/goals-widgets.md §9.2-9.3) -
	// never by a backend restart.
	StartedAt time.Time

	// ConfigRevision guards only operator configuration edits (PUT) via
	// optimistic concurrency - never touched by contribution application
	// (docs/goals-widgets.md §8.1).
	ConfigRevision int64
}

// WidgetProfile is one persisted public presentation of exactly one Goal
// (docs/goals-widgets.md §18).
type WidgetProfile struct {
	ID         string
	GoalID     string
	Name       string
	Enabled    bool
	PublicSlug string

	// TitleOverride falls back to the goal's own Name when empty.
	TitleOverride string
	ShowCurrent   bool
	ShowTarget    bool
	ShowPercent   bool

	Orientation Orientation
	TextAlign   TextAlign
	FontFamily  FontFamily

	BackgroundColor string
	ForegroundColor string
	FillColor       string
	BorderColor     string
	BorderRadiusPx  int
	Opacity         float64

	CreatedAt time.Time
	UpdatedAt time.Time
}

// DefaultGoal returns a new goal's safe, validated defaults plus the
// caller's chosen name/kind/target. ID and timestamps are filled by the
// Service.
func DefaultGoal(name string, kind Kind, target int64) Goal {
	return Goal{
		Name:    name,
		Kind:    kind,
		Enabled: true,
		Target:  target,
	}
}

// DefaultWidgetProfile returns a new widget profile's safe, validated
// presentation defaults plus the caller's chosen name/goal. ID,
// PublicSlug, and timestamps are filled by the Service.
func DefaultWidgetProfile(goalID, name string) WidgetProfile {
	return WidgetProfile{
		GoalID:          goalID,
		Name:            name,
		Enabled:         true,
		ShowCurrent:     true,
		ShowTarget:      true,
		ShowPercent:     true,
		Orientation:     OrientationHorizontal,
		TextAlign:       AlignCenter,
		FontFamily:      FontSansSerif,
		BackgroundColor: "#00000080",
		ForegroundColor: "#ffffff",
		FillColor:       "#7c3aed",
		BorderColor:     "#ffffff33",
		BorderRadiusPx:  DefaultBorderRadiusPx,
		Opacity:         1.0,
	}
}

// ProgressBasisPoints returns g's progress as an integer 0..10000+ scale
// (docs/goals-widgets.md §21) - may exceed 10000 when Current > Target;
// callers that render a bar clamp it themselves. Target is assumed > 0
// (validation.go guarantees this for any persisted Goal).
func (g Goal) ProgressBasisPoints() int64 {
	if g.Target <= 0 {
		return 0
	}
	return g.Current * 10000 / g.Target
}

// Completed reports whether g has reached or exceeded its target.
func (g Goal) Completed() bool {
	return g.Current >= g.Target
}
