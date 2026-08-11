import type { AlertEventType } from '@/api/alerts-schemas';
import type { VisualDesignDataContext } from '@/components/visual-design/VisualDesignRenderer';

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

/** The subset of an alert fixture this module can build on its own -
 * `bindings.renderedText` (depends on the rule's own saved template,
 * fetched separately via the existing `/api/alert-rule-preview`
 * endpoint) and `bindings.eventType`/`bindings.platform` (resolved
 * display labels - this module never imports react-i18next itself) are
 * always filled in by the caller (AlertDesignerWorkspace.tsx). */
export type AlertPreviewFixture = Omit<VisualDesignDataContext, 'bindings'> & {
  bindings: Omit<VisualDesignDataContext['bindings'], 'renderedText' | 'eventType' | 'platform'>;
};

/** Builds the scenario's own synthetic binding data. */
export function previewScenarioFixture(scenario: PreviewScenario): AlertPreviewFixture {
  function fixture(username: string | null, message: string | null, quantity: number | null, groupCount: number): AlertPreviewFixture {
    return {
      providerId: 'twitch',
      avatarUrl: null,
      bindings: { username, message, quantity, groupCount, timestamp: null, accountLabel: null },
    };
  }
  switch (scenario) {
    case 'follow':
      return fixture('TestViewer', null, null, 1);
    case 'subscription':
      return fixture('TestViewer', null, null, 1);
    case 'resubscription':
      return fixture('TestViewer', 'This is a test alert message.', null, 1);
    case 'gifted_subscription':
      return fixture('TestViewer', null, null, 1);
    case 'subscription_gift_batch':
      return fixture('TestViewer', null, 5, 1);
    case 'bits':
      return fixture('TestViewer', 'This is a test alert message.', 250, 1);
    case 'raid':
      return fixture('TestViewer', null, 42, 1);
    case 'channel_point_redemption':
      return fixture('TestViewer', 'This is a test alert message.', null, 1);
    case 'grouped_bits':
      return fixture('TestViewer', null, 750, 3);
    case 'grouped_gift_batch':
      return fixture('TestViewer', null, 12, 4);
    case 'anonymous':
      return fixture(null, null, 250, 1);
    case 'missing_avatar':
      return fixture('TestViewer', null, null, 1);
    case 'very_long_username':
      return fixture(VERY_LONG_USERNAME, null, null, 1);
    case 'very_long_message':
      return fixture('TestViewer', VERY_LONG_MESSAGE, null, 1);
    case 'missing_message':
      return fixture('TestViewer', null, null, 1);
  }
}
