import enAccounts from './resources/en/accounts.json';
import enAlertDesigner from './resources/en/alertDesigner.json';
import enAlerts from './resources/en/alerts.json';
import enAutomation from './resources/en/automation.json';
import enChat from './resources/en/chat.json';
import enChatOverlayDesigner from './resources/en/chatOverlayDesigner.json';
import enCommon from './resources/en/common.json';
import enDashboard from './resources/en/dashboard.json';
import enEngagement from './resources/en/engagement.json';
import enErrors from './resources/en/errors.json';
import enMetadata from './resources/en/metadata.json';
import enNavigation from './resources/en/navigation.json';
import enOverlays from './resources/en/overlays.json';
import enPages from './resources/en/pages.json';
import enPlatforms from './resources/en/platforms.json';
import enRuntime from './resources/en/runtime.json';
import plAccounts from './resources/pl/accounts.json';
import plAlertDesigner from './resources/pl/alertDesigner.json';
import plAlerts from './resources/pl/alerts.json';
import plAutomation from './resources/pl/automation.json';
import plChat from './resources/pl/chat.json';
import plChatOverlayDesigner from './resources/pl/chatOverlayDesigner.json';
import plCommon from './resources/pl/common.json';
import plDashboard from './resources/pl/dashboard.json';
import plEngagement from './resources/pl/engagement.json';
import plErrors from './resources/pl/errors.json';
import plMetadata from './resources/pl/metadata.json';
import plNavigation from './resources/pl/navigation.json';
import plOverlays from './resources/pl/overlays.json';
import plPages from './resources/pl/pages.json';
import plPlatforms from './resources/pl/platforms.json';
import plRuntime from './resources/pl/runtime.json';

/**
 * Static translation bundle.
 *
 * Resources are imported at build time rather than fetched at runtime: the app
 * runs locally, both languages together are a few kilobytes, and bundling them
 * removes an entire class of "translation not loaded yet" states.
 *
 * There is no runtime translation service of any kind. Every string below is a
 * version-controlled resource reviewed like the rest of the codebase.
 */
export const enResources = {
  common: enCommon,
  navigation: enNavigation,
  dashboard: enDashboard,
  platforms: enPlatforms,
  metadata: enMetadata,
  pages: enPages,
  errors: enErrors,
  runtime: enRuntime,
  accounts: enAccounts,
  engagement: enEngagement,
  chat: enChat,
  overlays: enOverlays,
  automation: enAutomation,
  alerts: enAlerts,
  alertDesigner: enAlertDesigner,
  chatOverlayDesigner: enChatOverlayDesigner,
} as const;

const plResources = {
  common: plCommon,
  navigation: plNavigation,
  dashboard: plDashboard,
  platforms: plPlatforms,
  metadata: plMetadata,
  pages: plPages,
  errors: plErrors,
  runtime: plRuntime,
  accounts: plAccounts,
  engagement: plEngagement,
  chat: plChat,
  overlays: plOverlays,
  automation: plAutomation,
  alerts: plAlerts,
  alertDesigner: plAlertDesigner,
  chatOverlayDesigner: plChatOverlayDesigner,
} as const;

export const resources = {
  en: enResources,
  pl: plResources,
} as const;

/**
 * Shape of the English bundle. It is the canonical key structure that every
 * other language must mirror, and it drives i18next's key type-checking.
 */
export type AppResources = typeof enResources;
