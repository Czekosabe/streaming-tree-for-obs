import type { AlertEventType } from '@/api/alerts-schemas';
import type { VisualDesignAlertData } from '@/components/visual-design/VisualDesignRenderer';

/**
 * Deterministic, local, synthetic preview fixtures (Stage 13A task
 * Part 39) - frontend-local, never touches the Event Bus, the real
 * queue, or a real Twitch account. Distinct from Test Rule (Part 40),
 * which goes through the real backend queue and always uses the
 * rule's last SAVED design, never an unsaved draft.
 *
 * Covers the 8 real event-type fixtures plus 7 presentation-edge
 * scenarios (grouped, anonymous, missing avatar, very long username/
 * message, missing message) - deliberately not exhaustive of every
 * possible combination, mirroring internal/alerts/testevents.go's own
 * small, representative fixture set rather than a combinatorial
 * explosion.
 */
export const PREVIEW_SCENARIOS = [
  'follow',
  'subscription',
  'resubscription',
  'gifted_subscription',
  'subscription_gift_batch',
  'bits',
  'raid',
  'channel_point_redemption',
  'grouped_bits',
  'grouped_gift_batch',
  'anonymous',
  'missing_avatar',
  'very_long_username',
  'very_long_message',
  'missing_message',
] as const;
export type PreviewScenario = (typeof PREVIEW_SCENARIOS)[number];

/** The real event type each scenario represents - used to resolve
 * text-binding availability and to fetch a representative
 * `alert_rendered_text` preview render for the rule's own template. */
export function baseEventTypeForScenario(scenario: PreviewScenario): AlertEventType {
  switch (scenario) {
    case 'grouped_bits':
      return 'bits';
    case 'grouped_gift_batch':
      return 'subscription_gift_batch';
    case 'anonymous':
      return 'bits';
    case 'missing_avatar':
    case 'very_long_username':
    case 'very_long_message':
    case 'missing_message':
      return 'follow';
    default:
      return scenario;
  }
}

const VERY_LONG_USERNAME = 'VeryLongTestViewerName'.repeat(6);
const VERY_LONG_MESSAGE = 'This is a very long test alert message. '.repeat(8);

/** Builds the scenario's own synthetic binding data - renderedText is
 * filled in separately by the caller (via the existing, already-local
 * `/api/alert-rule-preview` endpoint, reusing Stage 12A's own
 * established "local template preview" precedent) since it depends on
 * the rule's own saved template, not just the scenario. */
export function previewScenarioFixture(scenario: PreviewScenario): Omit<VisualDesignAlertData, 'renderedText'> {
  const providerId = 'twitch';
  switch (scenario) {
    case 'follow':
      return { eventType: 'follow', providerId, username: 'TestViewer', message: null, quantity: null, groupCount: 1, avatarUrl: null };
    case 'subscription':
      return { eventType: 'subscription', providerId, username: 'TestViewer', message: null, quantity: null, groupCount: 1, avatarUrl: null };
    case 'resubscription':
      return { eventType: 'resubscription', providerId, username: 'TestViewer', message: 'This is a test alert message.', quantity: null, groupCount: 1, avatarUrl: null };
    case 'gifted_subscription':
      return { eventType: 'gifted_subscription', providerId, username: 'TestViewer', message: null, quantity: null, groupCount: 1, avatarUrl: null };
    case 'subscription_gift_batch':
      return { eventType: 'subscription_gift_batch', providerId, username: 'TestViewer', message: null, quantity: 5, groupCount: 1, avatarUrl: null };
    case 'bits':
      return { eventType: 'bits', providerId, username: 'TestViewer', message: 'This is a test alert message.', quantity: 250, groupCount: 1, avatarUrl: null };
    case 'raid':
      return { eventType: 'raid', providerId, username: 'TestViewer', message: null, quantity: 42, groupCount: 1, avatarUrl: null };
    case 'channel_point_redemption':
      return { eventType: 'channel_point_redemption', providerId, username: 'TestViewer', message: 'This is a test alert message.', quantity: null, groupCount: 1, avatarUrl: null };
    case 'grouped_bits':
      return { eventType: 'bits', providerId, username: 'TestViewer', message: null, quantity: 750, groupCount: 3, avatarUrl: null };
    case 'grouped_gift_batch':
      return { eventType: 'subscription_gift_batch', providerId, username: 'TestViewer', message: null, quantity: 12, groupCount: 4, avatarUrl: null };
    case 'anonymous':
      return { eventType: 'bits', providerId, username: null, message: null, quantity: 250, groupCount: 1, avatarUrl: null };
    case 'missing_avatar':
      return { eventType: 'follow', providerId, username: 'TestViewer', message: null, quantity: null, groupCount: 1, avatarUrl: null };
    case 'very_long_username':
      return { eventType: 'follow', providerId, username: VERY_LONG_USERNAME, message: null, quantity: null, groupCount: 1, avatarUrl: null };
    case 'very_long_message':
      return { eventType: 'follow', providerId, username: 'TestViewer', message: VERY_LONG_MESSAGE, quantity: null, groupCount: 1, avatarUrl: null };
    case 'missing_message':
      return { eventType: 'follow', providerId, username: 'TestViewer', message: null, quantity: null, groupCount: 1, avatarUrl: null };
  }
}
