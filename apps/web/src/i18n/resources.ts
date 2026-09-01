import enAbout from './resources/en/about.json';
import enAccounts from './resources/en/accounts.json';
import enAlertDesigner from './resources/en/alertDesigner.json';
import enAlerts from './resources/en/alerts.json';
import enAudio from './resources/en/audio.json';
import enAuth from './resources/en/auth.json';
import enAutomation from './resources/en/automation.json';
import enBackup from './resources/en/backup.json';
import enChat from './resources/en/chat.json';
import enChatOverlayDesigner from './resources/en/chatOverlayDesigner.json';
import enCommon from './resources/en/common.json';
import enDashboard from './resources/en/dashboard.json';
import enEngagement from './resources/en/engagement.json';
import enErrors from './resources/en/errors.json';
import enGoals from './resources/en/goals.json';
import enHistory from './resources/en/history.json';
import enLogs from './resources/en/logs.json';
import enMetadata from './resources/en/metadata.json';
import enMetadataPresets from './resources/en/metadataPresets.json';
import enNavigation from './resources/en/navigation.json';
import enOnboarding from './resources/en/onboarding.json';
import enOverlays from './resources/en/overlays.json';
import enPages from './resources/en/pages.json';
import enPlatforms from './resources/en/platforms.json';
import enRuntime from './resources/en/runtime.json';
import enUpdates from './resources/en/updates.json';
import enVisualTemplates from './resources/en/visualTemplates.json';
import plAbout from './resources/pl/about.json';
import plAccounts from './resources/pl/accounts.json';
import plAlertDesigner from './resources/pl/alertDesigner.json';
import plAlerts from './resources/pl/alerts.json';
import plAudio from './resources/pl/audio.json';
import plAuth from './resources/pl/auth.json';
import plAutomation from './resources/pl/automation.json';
import plBackup from './resources/pl/backup.json';
import plChat from './resources/pl/chat.json';
import plChatOverlayDesigner from './resources/pl/chatOverlayDesigner.json';
import plCommon from './resources/pl/common.json';
import plDashboard from './resources/pl/dashboard.json';
import plEngagement from './resources/pl/engagement.json';
import plErrors from './resources/pl/errors.json';
import plGoals from './resources/pl/goals.json';
import plHistory from './resources/pl/history.json';
import plLogs from './resources/pl/logs.json';
import plMetadata from './resources/pl/metadata.json';
import plMetadataPresets from './resources/pl/metadataPresets.json';
import plNavigation from './resources/pl/navigation.json';
import plOnboarding from './resources/pl/onboarding.json';
import plOverlays from './resources/pl/overlays.json';
import plPages from './resources/pl/pages.json';
import plPlatforms from './resources/pl/platforms.json';
import plRuntime from './resources/pl/runtime.json';
import plUpdates from './resources/pl/updates.json';
import plVisualTemplates from './resources/pl/visualTemplates.json';

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
  metadataPresets: enMetadataPresets,
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
  visualTemplates: enVisualTemplates,
  audio: enAudio,
  goals: enGoals,
  history: enHistory,
  about: enAbout,
  updates: enUpdates,
  auth: enAuth,
  logs: enLogs,
  onboarding: enOnboarding,
  backup: enBackup,
} as const;

const plResources = {
  common: plCommon,
  navigation: plNavigation,
  dashboard: plDashboard,
  platforms: plPlatforms,
  metadata: plMetadata,
  metadataPresets: plMetadataPresets,
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
  visualTemplates: plVisualTemplates,
  audio: plAudio,
  goals: plGoals,
  history: plHistory,
  about: plAbout,
  updates: plUpdates,
  auth: plAuth,
  logs: plLogs,
  onboarding: plOnboarding,
  backup: plBackup,
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
