import { z } from 'zod';

/**
 * Zod contracts for the Stage 12A alert API
 * (`internal/httpapi/alerts.go`): alert profiles, alert rules, the
 * queue/playback status, and the public Browser Source payload.
 *
 * No field here ever carries a token, a raw provider payload, or
 * queued-future-alert content - the backend response shapes never
 * carry one, so there is nothing to strip.
 */

export const alertEventTypeSchema = z.enum([
  'follow',
  'subscription',
  'resubscription',
  'gifted_subscription',
  'subscription_gift_batch',
  'bits',
  'raid',
  'channel_point_redemption',
]);
export type AlertEventType = z.infer<typeof alertEventTypeSchema>;

export const alertRoleSchema = z.enum(['everyone', 'subscriber', 'vip', 'moderator', 'broadcaster']);
export type AlertRole = z.infer<typeof alertRoleSchema>;

export const alertAnimationSchema = z.enum(['none', 'fade', 'slide_up', 'slide_left', 'scale']);
export type AlertAnimation = z.infer<typeof alertAnimationSchema>;

export const alertThemeSchema = z.enum(['minimal', 'compact', 'large']);
export type AlertTheme = z.infer<typeof alertThemeSchema>;

export const alertPositionSchema = z.enum(['top', 'center', 'bottom']);
export type AlertPosition = z.infer<typeof alertPositionSchema>;

export const alertTextAlignSchema = z.enum(['left', 'center', 'right']);
export type AlertTextAlign = z.infer<typeof alertTextAlignSchema>;

export const alertLanguageSchema = z.enum(['en', 'pl']);
export type AlertLanguage = z.infer<typeof alertLanguageSchema>;

/** Stage 12B: whether a rule's own alert may interrupt whatever is
 * currently playing when it is newly matched. See
 * internal/domain/alerts.InterruptMode. */
export const alertInterruptModeSchema = z.enum(['never', 'lower_priority']);
export type AlertInterruptMode = z.infer<typeof alertInterruptModeSchema>;

/** Stage 12B: the closed public reason an `alert.hide` revision
 * carries - see internal/alerts.HideReason. */
export const alertHideReasonSchema = z.enum(['completed', 'skipped', 'preempted', 'profile_disabled', 'reset']);
export type AlertHideReason = z.infer<typeof alertHideReasonSchema>;

export const alertEventTypeCapabilitySchema = z.object({
  eventType: alertEventTypeSchema,
  hasUser: z.boolean(),
  hasMessage: z.boolean(),
  hasQuantity: z.boolean(),
  hasAnonymity: z.boolean(),
  hasRewardTitle: z.boolean(),
  hasRoles: z.boolean(),
  availablePlaceholders: z.array(z.string()),
  /** Stage 12B: whether this event type has any safe grouping strategy
   * at all - see internal/domain/alerts.GroupingCapability. */
  groupable: z.boolean(),
  /** Stage 12B: true only when this type also has a real message -
   * enabling grouping forces "show message" off. */
  groupingRequiresHiddenMessage: z.boolean(),
});
export type AlertEventTypeCapability = z.infer<typeof alertEventTypeCapabilitySchema>;

export const alertProfileSchema = z.object({
  id: z.string(),
  publicSlug: z.string(),
  name: z.string(),
  enabled: z.boolean(),
  language: alertLanguageSchema,
  theme: alertThemeSchema,
  position: alertPositionSchema,
  textAlign: alertTextAlignSchema,
  maxQueueItems: z.number(),
  maximumQueueAgeSeconds: z.number(),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type AlertProfile = z.infer<typeof alertProfileSchema>;

/** Request body for updating a profile (PUT). Creating one only ever
 * takes a name - see createAlertProfile in api/alerts.ts. */
export type AlertProfileInput = {
  name: string;
  enabled: boolean;
  language: AlertLanguage;
  theme: AlertTheme;
  position: AlertPosition;
  textAlign: AlertTextAlign;
  maxQueueItems: number;
  maximumQueueAgeSeconds: number;
};

export const alertRuleSchema = z.object({
  id: z.string(),
  profileId: z.string(),
  name: z.string(),
  enabled: z.boolean(),
  eventType: alertEventTypeSchema,
  priority: z.number(),
  durationMs: z.number(),
  minimumQuantity: z.number().nullable().optional(),
  maximumQuantity: z.number().nullable().optional(),
  requiredRole: alertRoleSchema,
  showPlatform: z.boolean(),
  showUsername: z.boolean(),
  showMessage: z.boolean(),
  showQuantity: z.boolean(),
  textTemplate: z.string(),
  entryAnimation: alertAnimationSchema,
  exitAnimation: alertAnimationSchema,
  animationDurationMs: z.number(),
  providers: z.array(z.string()),
  accounts: z.array(z.string()),
  allowGrouping: z.boolean(),
  groupWindowMs: z.number(),
  interruptMode: alertInterruptModeSchema,
  interruptible: z.boolean(),
  createdAt: z.string(),
  updatedAt: z.string(),
});
export type AlertRule = z.infer<typeof alertRuleSchema>;

export type AlertRuleInput = {
  name: string;
  enabled: boolean;
  eventType: AlertEventType;
  priority: number;
  durationMs: number;
  minimumQuantity?: number | null;
  maximumQuantity?: number | null;
  requiredRole: AlertRole;
  showPlatform: boolean;
  showUsername: boolean;
  showMessage: boolean;
  showQuantity: boolean;
  textTemplate: string;
  entryAnimation: AlertAnimation;
  exitAnimation: AlertAnimation;
  animationDurationMs: number;
  providers: string[];
  accounts: string[];
  allowGrouping: boolean;
  groupWindowMs: number;
  interruptMode: AlertInterruptMode;
  interruptible: boolean;
};

export const alertOverlapWarningSchema = z.object({
  ruleId: z.string(),
  otherRuleId: z.string(),
  eventType: alertEventTypeSchema,
});
export type AlertOverlapWarning = z.infer<typeof alertOverlapWarningSchema>;

export const listAlertRulesResponseSchema = z.object({
  rules: z.array(alertRuleSchema),
  overlapWarnings: z.array(alertOverlapWarningSchema),
});
export type ListAlertRulesResponse = z.infer<typeof listAlertRulesResponseSchema>;

export const alertSummarySchema = z.object({
  alertId: z.string(),
  ruleId: z.string(),
  eventType: alertEventTypeSchema,
  queuedAt: z.string(),
  priority: z.number(),
  username: z.string().optional(),
  message: z.string().optional(),
  quantity: z.number().nullable().optional(),
  renderedText: z.string(),
  synthetic: z.boolean(),
  replayed: z.boolean(),
  groupCount: z.number(),
  interruptible: z.boolean(),
});
export type AlertSummary = z.infer<typeof alertSummarySchema>;

export const alertQueueStatusSchema = z.object({
  profileId: z.string(),
  enabled: z.boolean(),
  paused: z.boolean(),
  current: alertSummarySchema.optional(),
  queuedCount: z.number(),
  queueCapacity: z.number(),
  nextQueued: z.array(alertSummarySchema),
  totalEnqueued: z.number(),
  totalPlayed: z.number(),
  totalExpired: z.number(),
  totalCapacityDropped: z.number(),
  totalManuallySkipped: z.number(),
  totalSynthetic: z.number(),
  totalGroupedMembers: z.number(),
  totalGroupsCreated: z.number(),
  totalPreempted: z.number(),
  replayAvailable: z.boolean(),
  activeSubscribers: z.number(),
  lastAlertAt: z.string().optional(),
  lastSkipReason: z.string().optional(),
  inputGap: z.boolean(),
});
export type AlertQueueStatus = z.infer<typeof alertQueueStatusSchema>;

export const alertRulePreviewResponseSchema = z.object({
  renderedText: z.string(),
  codePointCount: z.number(),
  resolvedPlaceholders: z.array(z.string()).optional(),
  unresolvedPlaceholders: z.array(z.string()).optional(),
  validForProvider: z.boolean(),
});
export type AlertRulePreviewResponse = z.infer<typeof alertRulePreviewResponseSchema>;

export type AlertRulePreviewInput = {
  eventType: AlertEventType;
  template: string;
  language?: AlertLanguage;
};

/** Public, presentation-only profile config
 * (`GET /api/public/alert-profiles/{slug}/config`) - never a
 * management id, queue setting, or any other operator-only field. */
export const publicAlertProfileConfigSchema = z.object({
  schemaVersion: z.number(),
  theme: alertThemeSchema,
  position: alertPositionSchema,
  textAlign: alertTextAlignSchema,
  language: alertLanguageSchema,
});
export type PublicAlertProfileConfig = z.infer<typeof publicAlertProfileConfigSchema>;

/** The public alert payload delivered over SSE - Part 22's exact,
 * presentation-only field list. */
export const publicAlertSchema = z.object({
  schemaVersion: z.number(),
  alertId: z.string(),
  eventType: alertEventTypeSchema,
  providerId: z.string(),
  synthetic: z.boolean(),
  replayed: z.boolean(),
  username: z.string().nullable().optional(),
  message: z.string().nullable().optional(),
  quantity: z.number().nullable().optional(),
  groupCount: z.number(),
  renderedText: z.string(),
  durationMs: z.number(),
  entryAnimation: alertAnimationSchema,
  exitAnimation: alertAnimationSchema,
  animationDurationMs: z.number(),
});
export type PublicAlert = z.infer<typeof publicAlertSchema>;

export const publicAlertRevisionPayloadSchema = z.object({
  paused: z.boolean(),
  alert: publicAlertSchema.nullable(),
});
export type PublicAlertRevisionPayload = z.infer<typeof publicAlertRevisionPayloadSchema>;

/** Stage 12B: the `alert.hide` revision's own distinct payload shape -
 * deliberately never `{alert}` (Part 20/36: no prior rendered content),
 * only the hidden alert's id and a stable reason. */
export const publicAlertHideRevisionPayloadSchema = z.object({
  paused: z.boolean(),
  alertId: z.string(),
  reason: alertHideReasonSchema,
});
export type PublicAlertHideRevisionPayload = z.infer<typeof publicAlertHideRevisionPayloadSchema>;

/** Every stable alert_* error code the backend can return. */
export const alertErrorCodeSchema = z.enum([
  'alert_profile_not_found',
  'alert_profile_disabled',
  'alert_profile_invalid',
  'alert_rule_not_found',
  'alert_rule_invalid',
  'alert_rule_event_unsupported',
  'alert_rule_condition_unsupported',
  'alert_rule_account_not_found',
  'alert_rule_threshold_invalid',
  'alert_template_invalid',
  'alert_template_unresolved',
  'alert_queue_paused',
  'alert_queue_empty',
  'alert_queue_full',
  'alert_queue_unavailable',
  'alert_public_slug_invalid',
  'alert_stream_limit_reached',
  'alert_gap',
]);
export type AlertErrorCode = z.infer<typeof alertErrorCodeSchema>;
