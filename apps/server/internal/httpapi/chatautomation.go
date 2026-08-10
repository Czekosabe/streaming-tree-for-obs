package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/streaming-tree/server/internal/chatautomation"
	"github.com/streaming-tree/server/internal/domain/account"
	domain "github.com/streaming-tree/server/internal/domain/chatautomation"
	"github.com/streaming-tree/server/internal/outboundchat"
)

// maxChatAutomationBodyBytes caps schedule/command/preview request
// bodies - generous for up to 20 message alternatives at 500 code
// points each, well below the general maxRequestBodyBytes ceiling.
const maxChatAutomationBodyBytes = 32 * 1024

// ChatAutomationService is the subset of chatautomation.Manager the
// HTTP layer needs.
type ChatAutomationService interface {
	CreateSchedule(ctx context.Context, in domain.ScheduleInput) (domain.Schedule, error)
	GetSchedule(ctx context.Context, id string) (domain.Schedule, error)
	ListSchedules(ctx context.Context) ([]domain.Schedule, error)
	ReplaceSchedule(ctx context.Context, id string, in domain.ScheduleInput) (domain.Schedule, error)
	DeleteSchedule(ctx context.Context, id string) error
	SendNow(ctx context.Context, id string, accountIDs []string) ([]chatautomation.SendResult, error)
	ScheduleStatus(id string) (chatautomation.ScheduleSnapshot, bool)

	CreateCommand(ctx context.Context, in domain.CommandInput) (domain.Command, error)
	GetCommand(ctx context.Context, id string) (domain.Command, error)
	ListCommands(ctx context.Context) ([]domain.Command, error)
	ReplaceCommand(ctx context.Context, id string, in domain.CommandInput) (domain.Command, error)
	DeleteCommand(ctx context.Context, id string) error
	CommandStatus(id string) (chatautomation.CommandSnapshot, bool)

	Status(ctx context.Context) (chatautomation.Status, error)
	Preview(ctx context.Context, template, accountID, platformID string) (chatautomation.RenderResult, error)
}

// registerChatAutomationRoutes wires the Stage 11B automation API under
// /api/chat-automation - schedules, commands, status and local preview
// rendering. Registered only alongside Accounts (router.go's own gate
// condition), since every target references a connected account, even
// though this function itself talks only to ChatAutomationService.
func registerChatAutomationRoutes(mux *http.ServeMux, logger *slog.Logger, automation ChatAutomationService) {
	mux.HandleFunc("GET /api/chat-automation/status", handleChatAutomationStatus(logger, automation))
	mux.HandleFunc("/api/chat-automation/status", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/chat-automation/schedules", handleListSchedules(logger, automation))
	mux.HandleFunc("POST /api/chat-automation/schedules", handleCreateSchedule(logger, automation))
	mux.HandleFunc("/api/chat-automation/schedules", methodNotAllowed(logger, http.MethodGet, http.MethodPost))

	mux.HandleFunc("GET /api/chat-automation/schedules/{id}", handleGetSchedule(logger, automation))
	mux.HandleFunc("PUT /api/chat-automation/schedules/{id}", handleUpdateSchedule(logger, automation))
	mux.HandleFunc("DELETE /api/chat-automation/schedules/{id}", handleDeleteSchedule(logger, automation))
	mux.HandleFunc("/api/chat-automation/schedules/{id}",
		methodNotAllowed(logger, http.MethodGet, http.MethodPut, http.MethodDelete))

	mux.HandleFunc("POST /api/chat-automation/schedules/{id}/send-now", handleSendNowSchedule(logger, automation))
	mux.HandleFunc("/api/chat-automation/schedules/{id}/send-now", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("GET /api/chat-automation/commands", handleListCommands(logger, automation))
	mux.HandleFunc("POST /api/chat-automation/commands", handleCreateCommand(logger, automation))
	mux.HandleFunc("/api/chat-automation/commands", methodNotAllowed(logger, http.MethodGet, http.MethodPost))

	mux.HandleFunc("GET /api/chat-automation/commands/{id}", handleGetCommand(logger, automation))
	mux.HandleFunc("PUT /api/chat-automation/commands/{id}", handleUpdateCommand(logger, automation))
	mux.HandleFunc("DELETE /api/chat-automation/commands/{id}", handleDeleteCommand(logger, automation))
	mux.HandleFunc("/api/chat-automation/commands/{id}",
		methodNotAllowed(logger, http.MethodGet, http.MethodPut, http.MethodDelete))

	mux.HandleFunc("POST /api/chat-automation/preview", handleChatAutomationPreview(logger, automation))
	mux.HandleFunc("/api/chat-automation/preview", methodNotAllowed(logger, http.MethodPost))
}

// --- DTOs -------------------------------------------------------------

type chatAutomationTargetDTO struct {
	AccountID  string `json:"accountId"`
	PlatformID string `json:"platformId,omitempty"`
}

type chatAutomationTargetStatusDTO struct {
	AccountID      string `json:"accountId"`
	LastAttemptAt  string `json:"lastAttemptAt,omitempty"`
	LastSuccessAt  string `json:"lastSuccessAt,omitempty"`
	LastSkipReason string `json:"lastSkipReason,omitempty"`
	SendsThisHour  int    `json:"sendsThisHour"`
}

type scheduleMessageDTO struct {
	ID       string `json:"id,omitempty"`
	Template string `json:"template"`
}

type scheduleRequest struct {
	Name                     string                    `json:"name"`
	Enabled                  bool                      `json:"enabled"`
	IntervalSeconds          int                       `json:"intervalSeconds"`
	FirstDelaySeconds        int                       `json:"firstDelaySeconds"`
	JitterSeconds            int                       `json:"jitterSeconds"`
	OnlyWhileIngestReceiving bool                      `json:"onlyWhileIngestReceiving"`
	MinimumChatMessages      int                       `json:"minimumChatMessages"`
	MaximumSendsPerHour      int                       `json:"maximumSendsPerHour"`
	Targets                  []chatAutomationTargetDTO `json:"targets"`
	Messages                 []string                  `json:"messages"`
}

func (r scheduleRequest) toInput() domain.ScheduleInput {
	targets := make([]domain.Target, len(r.Targets))
	for i, t := range r.Targets {
		targets[i] = domain.Target{AccountID: t.AccountID, PlatformID: t.PlatformID}
	}
	return domain.ScheduleInput{
		Name: r.Name, Enabled: r.Enabled, IntervalSeconds: r.IntervalSeconds,
		FirstDelaySeconds: r.FirstDelaySeconds, JitterSeconds: r.JitterSeconds,
		OnlyWhileIngestReceiving: r.OnlyWhileIngestReceiving, MinimumChatMessages: r.MinimumChatMessages,
		MaximumSendsPerHour: r.MaximumSendsPerHour, Targets: targets, MessageTemplates: r.Messages,
	}
}

type scheduleResponse struct {
	ID                       string                          `json:"id"`
	Name                     string                          `json:"name"`
	Enabled                  bool                            `json:"enabled"`
	IntervalSeconds          int                             `json:"intervalSeconds"`
	FirstDelaySeconds        int                             `json:"firstDelaySeconds"`
	JitterSeconds            int                             `json:"jitterSeconds"`
	OnlyWhileIngestReceiving bool                            `json:"onlyWhileIngestReceiving"`
	MinimumChatMessages      int                             `json:"minimumChatMessages"`
	MaximumSendsPerHour      int                             `json:"maximumSendsPerHour"`
	Targets                  []chatAutomationTargetDTO       `json:"targets"`
	Messages                 []scheduleMessageDTO            `json:"messages"`
	CreatedAt                string                          `json:"createdAt"`
	UpdatedAt                string                          `json:"updatedAt"`
	State                    string                          `json:"state"`
	NextRunAt                string                          `json:"nextRunAt,omitempty"`
	LastAttemptAt            string                          `json:"lastAttemptAt,omitempty"`
	LastSuccessAt            string                          `json:"lastSuccessAt,omitempty"`
	LastSkipReason           string                          `json:"lastSkipReason,omitempty"`
	TargetStatus             []chatAutomationTargetStatusDTO `json:"targetStatus,omitempty"`
}

func toScheduleResponse(sch domain.Schedule, snap chatautomation.ScheduleSnapshot, found bool) scheduleResponse {
	targets := make([]chatAutomationTargetDTO, len(sch.Targets))
	for i, t := range sch.Targets {
		targets[i] = chatAutomationTargetDTO{AccountID: t.AccountID, PlatformID: t.PlatformID}
	}
	messages := make([]scheduleMessageDTO, len(sch.Messages))
	for i, m := range sch.Messages {
		messages[i] = scheduleMessageDTO{ID: m.ID, Template: m.MessageTemplate}
	}
	resp := scheduleResponse{
		ID: sch.ID, Name: sch.Name, Enabled: sch.Enabled,
		IntervalSeconds: sch.IntervalSeconds, FirstDelaySeconds: sch.FirstDelaySeconds, JitterSeconds: sch.JitterSeconds,
		OnlyWhileIngestReceiving: sch.OnlyWhileIngestReceiving, MinimumChatMessages: sch.MinimumChatMessages,
		MaximumSendsPerHour: sch.MaximumSendsPerHour, Targets: targets, Messages: messages,
		CreatedAt: sch.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: sch.UpdatedAt.UTC().Format(time.RFC3339Nano),
		State: string(chatautomation.ScheduleDisabled),
	}
	if !found {
		return resp
	}
	resp.State = string(snap.State)
	if snap.NextRunAt != nil {
		resp.NextRunAt = snap.NextRunAt.UTC().Format(time.RFC3339Nano)
	}
	if snap.LastAttemptAt != nil {
		resp.LastAttemptAt = snap.LastAttemptAt.UTC().Format(time.RFC3339Nano)
	}
	if snap.LastSuccessAt != nil {
		resp.LastSuccessAt = snap.LastSuccessAt.UTC().Format(time.RFC3339Nano)
	}
	resp.LastSkipReason = string(snap.LastSkipReason)
	for _, ts := range snap.Targets {
		dto := chatAutomationTargetStatusDTO{AccountID: ts.AccountID, SendsThisHour: ts.SendsThisHour, LastSkipReason: string(ts.LastSkipReason)}
		if ts.LastAttemptAt != nil {
			dto.LastAttemptAt = ts.LastAttemptAt.UTC().Format(time.RFC3339Nano)
		}
		if ts.LastSuccessAt != nil {
			dto.LastSuccessAt = ts.LastSuccessAt.UTC().Format(time.RFC3339Nano)
		}
		resp.TargetStatus = append(resp.TargetStatus, dto)
	}
	return resp
}

type commandRequest struct {
	Name                  string                    `json:"name"`
	Enabled               bool                      `json:"enabled"`
	ResponseTemplate      string                    `json:"responseTemplate"`
	RequiredRole          string                    `json:"requiredRole"`
	GlobalCooldownSeconds int                       `json:"globalCooldownSeconds"`
	UserCooldownSeconds   int                       `json:"userCooldownSeconds"`
	Aliases               []string                  `json:"aliases"`
	Targets               []chatAutomationTargetDTO `json:"targets"`
}

func (r commandRequest) toInput() domain.CommandInput {
	targets := make([]domain.Target, len(r.Targets))
	for i, t := range r.Targets {
		targets[i] = domain.Target{AccountID: t.AccountID, PlatformID: t.PlatformID}
	}
	return domain.CommandInput{
		Name: r.Name, Enabled: r.Enabled, ResponseTemplate: r.ResponseTemplate, RequiredRole: domain.Role(r.RequiredRole),
		GlobalCooldownSeconds: r.GlobalCooldownSeconds, UserCooldownSeconds: r.UserCooldownSeconds,
		Aliases: r.Aliases, Targets: targets,
	}
}

type commandResponse struct {
	ID                    string                    `json:"id"`
	Name                  string                    `json:"name"`
	Enabled               bool                      `json:"enabled"`
	ResponseTemplate      string                    `json:"responseTemplate"`
	RequiredRole          string                    `json:"requiredRole"`
	GlobalCooldownSeconds int                       `json:"globalCooldownSeconds"`
	UserCooldownSeconds   int                       `json:"userCooldownSeconds"`
	Aliases               []string                  `json:"aliases"`
	Targets               []chatAutomationTargetDTO `json:"targets"`
	CreatedAt             string                    `json:"createdAt"`
	UpdatedAt             string                    `json:"updatedAt"`
	MatchCount            int64                     `json:"matchCount"`
	ResponseCount         int64                     `json:"responseCount"`
	LastResponseAt        string                    `json:"lastResponseAt,omitempty"`
}

func toCommandResponse(cmd domain.Command, snap chatautomation.CommandSnapshot) commandResponse {
	targets := make([]chatAutomationTargetDTO, len(cmd.Targets))
	for i, t := range cmd.Targets {
		targets[i] = chatAutomationTargetDTO{AccountID: t.AccountID, PlatformID: t.PlatformID}
	}
	resp := commandResponse{
		ID: cmd.ID, Name: cmd.Name, Enabled: cmd.Enabled, ResponseTemplate: cmd.ResponseTemplate,
		RequiredRole: string(cmd.RequiredRole), GlobalCooldownSeconds: cmd.GlobalCooldownSeconds,
		UserCooldownSeconds: cmd.UserCooldownSeconds, Aliases: cmd.Aliases, Targets: targets,
		CreatedAt: cmd.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: cmd.UpdatedAt.UTC().Format(time.RFC3339Nano),
		MatchCount: snap.MatchCount, ResponseCount: snap.ResponseCount,
	}
	if snap.LastResponseAt != nil {
		resp.LastResponseAt = snap.LastResponseAt.UTC().Format(time.RFC3339Nano)
	}
	return resp
}

type statusResponse struct {
	Engine    engineStatusDTO    `json:"engine"`
	Schedules []scheduleResponse `json:"schedules"`
	Commands  []commandResponse  `json:"commands"`
}

type engineStatusDTO struct {
	Running             bool   `json:"running"`
	SubscribedToBus     bool   `json:"subscribedToBus"`
	CommandCount        int    `json:"commandCount"`
	EnabledCommandCount int    `json:"enabledCommandCount"`
	TotalMatched        int64  `json:"totalMatched"`
	TotalResponses      int64  `json:"totalResponses"`
	TotalCooldownSkips  int64  `json:"totalCooldownSkips"`
	TotalRoleSkips      int64  `json:"totalRoleSkips"`
	TotalSelfSkips      int64  `json:"totalSelfSkips"`
	LastErrorCode       string `json:"lastErrorCode,omitempty"`
}

type sendNowRequest struct {
	AccountIDs []string `json:"accountIds,omitempty"`
}

type sendNowResultDTO struct {
	AccountID         string `json:"accountId"`
	Sent              bool   `json:"sent"`
	ProviderMessageID string `json:"providerMessageId,omitempty"`
	SkipReason        string `json:"skipReason,omitempty"`
}

type sendNowResponse struct {
	Results []sendNowResultDTO `json:"results"`
}

type previewRequest struct {
	Template   string `json:"template"`
	AccountID  string `json:"accountId"`
	PlatformID string `json:"platformId,omitempty"`
}

type previewResponse struct {
	RenderedText           string   `json:"renderedText"`
	CodePointCount         int      `json:"codePointCount"`
	ResolvedPlaceholders   []string `json:"resolvedPlaceholders,omitempty"`
	UnresolvedPlaceholders []string `json:"unresolvedPlaceholders,omitempty"`
	ValidForProvider       bool     `json:"validForProvider"`
	Warnings               []string `json:"warnings,omitempty"`
}

// --- handlers: status ---------------------------------------------------

func handleChatAutomationStatus(logger *slog.Logger, automation ChatAutomationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := automation.Status(r.Context())
		if err != nil {
			writeChatAutomationError(w, logger, err)
			return
		}
		resp := statusResponse{
			Engine: engineStatusDTO{
				Running: status.Engine.Running, SubscribedToBus: status.Engine.SubscribedToBus,
				CommandCount: status.Engine.CommandCount, EnabledCommandCount: status.Engine.EnabledCommandCount,
				TotalMatched: status.Engine.TotalMatched, TotalResponses: status.Engine.TotalResponses,
				TotalCooldownSkips: status.Engine.TotalCooldownSkips, TotalRoleSkips: status.Engine.TotalRoleSkips,
				TotalSelfSkips: status.Engine.TotalSelfSkips, LastErrorCode: status.Engine.LastErrorCode,
			},
			Schedules: make([]scheduleResponse, 0, len(status.Schedules)),
			Commands:  make([]commandResponse, 0, len(status.Commands)),
		}
		for _, s := range status.Schedules {
			resp.Schedules = append(resp.Schedules, toScheduleResponse(domain.Schedule{ID: s.ScheduleID, Enabled: s.Enabled}, s, true))
		}
		for _, c := range status.Commands {
			resp.Commands = append(resp.Commands, commandResponse{ID: c.CommandID, Enabled: c.Enabled, MatchCount: c.MatchCount, ResponseCount: c.ResponseCount})
		}
		writeJSON(w, logger, http.StatusOK, resp)
	}
}

// --- handlers: schedules -------------------------------------------------

func handleListSchedules(logger *slog.Logger, automation ChatAutomationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := automation.ListSchedules(r.Context())
		if err != nil {
			writeChatAutomationError(w, logger, err)
			return
		}
		out := make([]scheduleResponse, 0, len(list))
		for _, sch := range list {
			snap, found := automation.ScheduleStatus(sch.ID)
			out = append(out, toScheduleResponse(sch, snap, found))
		}
		writeJSON(w, logger, http.StatusOK, out)
	}
}

func handleCreateSchedule(logger *slog.Logger, automation ChatAutomationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body scheduleRequest
		if err := decodeJSONWithLimit(w, r, &body, maxChatAutomationBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		if err := validateTemplatesKnown(body.Messages); err != nil {
			writeChatAutomationError(w, logger, err)
			return
		}
		sch, err := automation.CreateSchedule(r.Context(), body.toInput())
		if err != nil {
			writeChatAutomationError(w, logger, err)
			return
		}
		snap, found := automation.ScheduleStatus(sch.ID)
		w.Header().Set("Location", "/api/chat-automation/schedules/"+sch.ID)
		writeJSON(w, logger, http.StatusCreated, toScheduleResponse(sch, snap, found))
	}
}

func handleGetSchedule(logger *slog.Logger, automation ChatAutomationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		sch, err := automation.GetSchedule(r.Context(), id)
		if err != nil {
			writeChatAutomationError(w, logger, err)
			return
		}
		snap, found := automation.ScheduleStatus(id)
		writeJSON(w, logger, http.StatusOK, toScheduleResponse(sch, snap, found))
	}
}

func handleUpdateSchedule(logger *slog.Logger, automation ChatAutomationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body scheduleRequest
		if err := decodeJSONWithLimit(w, r, &body, maxChatAutomationBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		if err := validateTemplatesKnown(body.Messages); err != nil {
			writeChatAutomationError(w, logger, err)
			return
		}
		sch, err := automation.ReplaceSchedule(r.Context(), id, body.toInput())
		if err != nil {
			writeChatAutomationError(w, logger, err)
			return
		}
		snap, found := automation.ScheduleStatus(id)
		writeJSON(w, logger, http.StatusOK, toScheduleResponse(sch, snap, found))
	}
}

func handleDeleteSchedule(logger *slog.Logger, automation ChatAutomationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := automation.DeleteSchedule(r.Context(), id); err != nil {
			writeChatAutomationError(w, logger, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleSendNowSchedule(logger *slog.Logger, automation ChatAutomationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		// The request body is optional (omitted entirely, or "{}", both mean
		// "every current target"). Checking r.ContentLength rather than
		// hasRequestBody's own one-byte peek matters here: hasRequestBody
		// reads (and so permanently consumes) one byte from r.Body to
		// answer the question, which would corrupt a subsequent real
		// decode of that same body - fine for requireEmptyBody elsewhere
		// (which never decodes afterward), wrong here.
		var body sendNowRequest
		if r.ContentLength > 0 {
			if err := decodeJSONWithLimit(w, r, &body, maxChatAutomationBodyBytes); err != nil {
				writeDecodeError(w, logger, err)
				return
			}
		}
		results, err := automation.SendNow(r.Context(), id, body.AccountIDs)
		if err != nil {
			writeChatAutomationError(w, logger, err)
			return
		}
		resp := sendNowResponse{Results: make([]sendNowResultDTO, 0, len(results))}
		for _, res := range results {
			resp.Results = append(resp.Results, sendNowResultDTO{
				AccountID: res.AccountID, Sent: res.Sent, ProviderMessageID: res.ProviderMessageID, SkipReason: string(res.SkipReason),
			})
		}
		writeJSON(w, logger, http.StatusOK, resp)
	}
}

// --- handlers: commands ----------------------------------------------

func handleListCommands(logger *slog.Logger, automation ChatAutomationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := automation.ListCommands(r.Context())
		if err != nil {
			writeChatAutomationError(w, logger, err)
			return
		}
		out := make([]commandResponse, 0, len(list))
		for _, cmd := range list {
			snap, _ := automation.CommandStatus(cmd.ID)
			out = append(out, toCommandResponse(cmd, snap))
		}
		writeJSON(w, logger, http.StatusOK, out)
	}
}

func handleCreateCommand(logger *slog.Logger, automation ChatAutomationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body commandRequest
		if err := decodeJSONWithLimit(w, r, &body, maxChatAutomationBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		if err := validateTemplatesKnown([]string{body.ResponseTemplate}); err != nil {
			writeChatAutomationError(w, logger, err)
			return
		}
		cmd, err := automation.CreateCommand(r.Context(), body.toInput())
		if err != nil {
			writeChatAutomationError(w, logger, err)
			return
		}
		snap, _ := automation.CommandStatus(cmd.ID)
		w.Header().Set("Location", "/api/chat-automation/commands/"+cmd.ID)
		writeJSON(w, logger, http.StatusCreated, toCommandResponse(cmd, snap))
	}
}

func handleGetCommand(logger *slog.Logger, automation ChatAutomationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		cmd, err := automation.GetCommand(r.Context(), id)
		if err != nil {
			writeChatAutomationError(w, logger, err)
			return
		}
		snap, _ := automation.CommandStatus(id)
		writeJSON(w, logger, http.StatusOK, toCommandResponse(cmd, snap))
	}
}

func handleUpdateCommand(logger *slog.Logger, automation ChatAutomationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body commandRequest
		if err := decodeJSONWithLimit(w, r, &body, maxChatAutomationBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		if err := validateTemplatesKnown([]string{body.ResponseTemplate}); err != nil {
			writeChatAutomationError(w, logger, err)
			return
		}
		cmd, err := automation.ReplaceCommand(r.Context(), id, body.toInput())
		if err != nil {
			writeChatAutomationError(w, logger, err)
			return
		}
		snap, _ := automation.CommandStatus(id)
		writeJSON(w, logger, http.StatusOK, toCommandResponse(cmd, snap))
	}
}

func handleDeleteCommand(logger *slog.Logger, automation ChatAutomationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := automation.DeleteCommand(r.Context(), id); err != nil {
			writeChatAutomationError(w, logger, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- handlers: preview -----------------------------------------------

// handleChatAutomationPreview renders a template locally against an
// account's own already-available context - never sends anything,
// never persists anything, never makes a provider network request. See
// the Stage 11B task's own Part 22.
func handleChatAutomationPreview(logger *slog.Logger, automation ChatAutomationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body previewRequest
		if err := decodeJSONWithLimit(w, r, &body, maxChatAutomationBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		result, err := automation.Preview(r.Context(), body.Template, body.AccountID, body.PlatformID)
		if err != nil {
			writeChatAutomationError(w, logger, err)
			return
		}
		resp := previewResponse{
			RenderedText: result.Text, CodePointCount: result.CodePointCount,
			ResolvedPlaceholders: result.Resolved, UnresolvedPlaceholders: result.Unresolved,
			ValidForProvider: result.ValidForProvider,
		}
		if len(result.Unresolved) > 0 {
			resp.Warnings = append(resp.Warnings, "unresolved_placeholder")
		}
		if !result.ValidForProvider && len(result.Unresolved) == 0 {
			resp.Warnings = append(resp.Warnings, "rendered_message_too_long")
		}
		writeJSON(w, logger, http.StatusOK, resp)
	}
}

// validateTemplatesKnown rejects an unknown placeholder name at save
// time (Part 19) - a malformed template (unmatched brace) is also
// rejected here, before ever reaching domain validation.
func validateTemplatesKnown(templates []string) error {
	for _, t := range templates {
		if err := chatautomation.ValidateTemplatePlaceholders(t); err != nil {
			return err
		}
	}
	return nil
}

// --- errors -------------------------------------------------------------

func writeChatAutomationError(w http.ResponseWriter, logger *slog.Logger, err error) {
	var rateLimitErr *outboundchat.RateLimitedError
	switch {
	case errors.Is(err, domain.ErrScheduleNotFound), errors.Is(err, domain.ErrCommandNotFound):
		writeError(w, logger, http.StatusNotFound, "chat_automation_not_found", "The requested automation rule does not exist.")
	case errors.Is(err, account.ErrNotFound), errors.Is(err, domain.ErrAccountNotFound):
		writeError(w, logger, http.StatusNotFound, "chat_automation_account_not_found", "The requested connected account does not exist.")
	case errors.Is(err, domain.ErrTargetRequired):
		writeError(w, logger, http.StatusUnprocessableEntity, "chat_automation_target_required", "At least one target account is required.")
	case errors.Is(err, domain.ErrPlatformNotFound), errors.Is(err, domain.ErrPlatformProviderMismatch), errors.Is(err, domain.ErrPlatformNotLinked):
		writeError(w, logger, http.StatusUnprocessableEntity, "chat_automation_target_invalid", "The target's platform context is invalid.")
	case errors.Is(err, domain.ErrCommandNameConflict):
		writeError(w, logger, http.StatusConflict, "chat_automation_command_conflict", "This command name or alias is already in use.")
	case errors.Is(err, domain.ErrValidation), errors.Is(err, domain.ErrMessageRequired):
		writeError(w, logger, http.StatusUnprocessableEntity, "chat_automation_invalid", "The automation rule failed validation.")
	case errors.Is(err, chatautomation.ErrPlaceholderInvalid):
		writeError(w, logger, http.StatusUnprocessableEntity, "chat_automation_placeholder_invalid", "The template uses an unknown or malformed placeholder.")
	case errors.Is(err, outboundchat.ErrUnsupportedProvider):
		writeError(w, logger, http.StatusServiceUnavailable, "chat_automation_provider_unsupported", "This provider does not support outbound chat.")
	case errors.Is(err, outboundchat.ErrPermissionRequired), errors.Is(err, account.ErrMissingScope):
		writeError(w, logger, http.StatusUnprocessableEntity, "chat_automation_permission_required", "Outbound chat permission has not been granted for this account.")
	case errors.Is(err, outboundchat.ErrQueueFull):
		writeError(w, logger, http.StatusTooManyRequests, "chat_automation_queue_full", "Too many automated messages are already queued for this account.")
	case errors.As(err, &rateLimitErr):
		writeError(w, logger, http.StatusTooManyRequests, "chat_automation_rate_limited", "Sending is temporarily rate limited.")
	case errors.Is(err, account.ErrReconnectRequired):
		writeError(w, logger, http.StatusConflict, "account_reconnect_required", "This account must be reconnected before it can send chat messages.")
	default:
		writeError(w, logger, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}
