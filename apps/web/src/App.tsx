import { Suspense } from 'react';
import { QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { BrowserRouter, Route, Routes } from 'react-router-dom';

import { AuthProvider } from '@/app/auth-context';
import { queryClient } from '@/app/query-client';
import { AuthGate } from '@/components/auth/AuthGate';
import { ShellLayout } from '@/components/layout/AppShell';
import { RouteLoadingFallback } from '@/components/layout/RouteLoadingFallback';
import { i18n } from '@/i18n';
import { lazyPage } from '@/lib/lazy-page';
import { DashboardPage } from '@/pages/DashboardPage';
import { OnboardingAutoRedirect } from '@/components/onboarding/OnboardingAutoRedirect';
import { NotFoundPage } from '@/pages/NotFoundPage';

/*
 * Route-level code splitting (performance-hardening pass).
 *
 * Only `DashboardPage` (the very first thing a returning operator
 * sees) and `NotFoundPage` (tiny, always needed as the catch-all) stay
 * in the eager, initial bundle alongside the persistent shell/auth/
 * onboarding-redirect machinery above. Every other page - each a real,
 * substantial, independent surface a given session may never even
 * visit - is its own on-demand chunk, so a fresh load downloads
 * Dashboard-sized code, not the whole application. See
 * `docs/development.md`'s "Typography and static assets" section's
 * sibling entry on this and `src/lib/lazy-page.ts` for why this needs
 * a small adapter (every page here is a named export, never a default
 * one - `React.lazy` only accepts the latter).
 *
 * `<ShellLayout>` (the sidebar, the OBS connection panel) is the
 * *parent* route element, rendered synchronously - it is never part of
 * any lazy boundary below and never remounts across a route change,
 * exactly as before this pass (Stage 20E defect A).
 */
const PlatformsPage = lazyPage(() => import('@/pages/PlatformsPage'), 'PlatformsPage');
const StreamsPage = lazyPage(() => import('@/pages/StreamsPage'), 'StreamsPage');
const MetadataPage = lazyPage(() => import('@/pages/MetadataPage'), 'MetadataPage');
const EngagementPage = lazyPage(() => import('@/pages/EngagementPage'), 'EngagementPage');
const ChatPage = lazyPage(() => import('@/pages/ChatPage'), 'ChatPage');
const OverlaysPage = lazyPage(() => import('@/pages/OverlaysPage'), 'OverlaysPage');
const AutomationPage = lazyPage(() => import('@/pages/AutomationPage'), 'AutomationPage');
const AlertsPage = lazyPage(() => import('@/pages/AlertsPage'), 'AlertsPage');
const AudioPage = lazyPage(() => import('@/pages/AudioPage'), 'AudioPage');
const GoalsPage = lazyPage(() => import('@/pages/GoalsPage'), 'GoalsPage');
const SettingsPage = lazyPage(() => import('@/pages/SettingsPage'), 'SettingsPage');
const AboutLegalPage = lazyPage(() => import('@/pages/AboutLegalPage'), 'AboutLegalPage');
const LogsPage = lazyPage(() => import('@/pages/LogsPage'), 'LogsPage');
const HistoryPage = lazyPage(() => import('@/pages/HistoryPage'), 'HistoryPage');

const OnboardingPage = lazyPage(() => import('@/pages/OnboardingPage'), 'OnboardingPage');
const AlertDesignerPage = lazyPage(() => import('@/pages/AlertDesignerPage'), 'AlertDesignerPage');
const ChatOverlayDesignerPage = lazyPage(
  () => import('@/pages/ChatOverlayDesignerPage'),
  'ChatOverlayDesignerPage',
);

// Standalone OBS Browser Source pages: never navigated to from within an
// operator's own dashboard session, always their own separate page load
// (a real OBS instance, or this suite's own browser, opening exactly this
// URL and nothing else) - lazy here means a Browser Source never
// downloads the entire operator-dashboard bundle just to render an
// overlay. No visible loading fallback: these render as a quiet/
// transparent canvas until their own real content streams in regardless,
// so a spinner would only add an extra, unwanted flash on stream.
const OverlayChatPage = lazyPage(() => import('@/pages/OverlayChatPage'), 'OverlayChatPage');
const PublicAlertPage = lazyPage(() => import('@/pages/PublicAlertPage'), 'PublicAlertPage');
const PublicAudioPage = lazyPage(() => import('@/pages/PublicAudioPage'), 'PublicAudioPage');
const PublicWidgetPage = lazyPage(() => import('@/pages/PublicWidgetPage'), 'PublicWidgetPage');

export function App() {
  return (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <AuthProvider>
            <AuthGate>
              <OnboardingAutoRedirect />
              <Routes>
                {/* Layout route: sidebar and mobile drawer render once here
                    and never remount as the routed page below changes - see
                    ShellLayout's own doc comment (Stage 20E defect A). Every
                    page nested under it still renders its own `<AppShell>`
                    for its title/description/actions, unchanged. The
                    Suspense boundary below is scoped to each route's own
                    `<Outlet>` content only, never to ShellLayout itself. */}
                <Route element={<ShellLayout />}>
                  <Route path="/" element={<DashboardPage />} />
                  <Route
                    path="/platforms"
                    element={
                      <Suspense fallback={<RouteLoadingFallback />}>
                        <PlatformsPage />
                      </Suspense>
                    }
                  />
                  <Route
                    path="/streams"
                    element={
                      <Suspense fallback={<RouteLoadingFallback />}>
                        <StreamsPage />
                      </Suspense>
                    }
                  />
                  <Route
                    path="/metadata"
                    element={
                      <Suspense fallback={<RouteLoadingFallback />}>
                        <MetadataPage />
                      </Suspense>
                    }
                  />
                  <Route
                    path="/engagement"
                    element={
                      <Suspense fallback={<RouteLoadingFallback />}>
                        <EngagementPage />
                      </Suspense>
                    }
                  />
                  <Route
                    path="/chat"
                    element={
                      <Suspense fallback={<RouteLoadingFallback />}>
                        <ChatPage />
                      </Suspense>
                    }
                  />
                  <Route
                    path="/overlays"
                    element={
                      <Suspense fallback={<RouteLoadingFallback />}>
                        <OverlaysPage />
                      </Suspense>
                    }
                  />
                  <Route
                    path="/automation"
                    element={
                      <Suspense fallback={<RouteLoadingFallback />}>
                        <AutomationPage />
                      </Suspense>
                    }
                  />
                  <Route
                    path="/alerts"
                    element={
                      <Suspense fallback={<RouteLoadingFallback />}>
                        <AlertsPage />
                      </Suspense>
                    }
                  />
                  <Route
                    path="/audio"
                    element={
                      <Suspense fallback={<RouteLoadingFallback />}>
                        <AudioPage />
                      </Suspense>
                    }
                  />
                  <Route
                    path="/goals"
                    element={
                      <Suspense fallback={<RouteLoadingFallback />}>
                        <GoalsPage />
                      </Suspense>
                    }
                  />
                  <Route
                    path="/settings"
                    element={
                      <Suspense fallback={<RouteLoadingFallback />}>
                        <SettingsPage />
                      </Suspense>
                    }
                  />
                  <Route
                    path="/settings/about"
                    element={
                      <Suspense fallback={<RouteLoadingFallback />}>
                        <AboutLegalPage />
                      </Suspense>
                    }
                  />
                  <Route
                    path="/logs"
                    element={
                      <Suspense fallback={<RouteLoadingFallback />}>
                        <LogsPage />
                      </Suspense>
                    }
                  />
                  <Route
                    path="/history"
                    element={
                      <Suspense fallback={<RouteLoadingFallback />}>
                        <HistoryPage />
                      </Suspense>
                    }
                  />
                  <Route path="*" element={<NotFoundPage />} />
                </Route>

                {/* Outside ShellLayout: deliberately no sidebar/nav chrome. */}
                <Route
                  path="/onboarding"
                  element={
                    <Suspense fallback={<RouteLoadingFallback />}>
                      <OnboardingPage />
                    </Suspense>
                  }
                />
                {/* The Designer wants full control of the viewport for its
                    own canvas/panel layout and top bar - see
                    AlertDesignerPage's own doc comment. */}
                <Route
                  path="/alerts/rules/:ruleId/designer"
                  element={
                    <Suspense fallback={<RouteLoadingFallback />}>
                      <AlertDesignerPage />
                    </Suspense>
                  }
                />
                {/* Same reasoning - see ChatOverlayDesignerPage's own doc
                    comment. */}
                <Route
                  path="/overlays/:overlayId/designer"
                  element={
                    <Suspense fallback={<RouteLoadingFallback />}>
                      <ChatOverlayDesignerPage />
                    </Suspense>
                  }
                />
                {/* A standalone Browser Source page, never the operator
                    dashboard's chrome - see OverlayChatPage's own doc
                    comment. */}
                <Route
                  path="/overlay/chat/:publicSlug"
                  element={
                    <Suspense fallback={null}>
                      <OverlayChatPage />
                    </Suspense>
                  }
                />
                {/* Same reasoning - see PublicAlertPage's own doc comment. */}
                <Route
                  path="/overlay/alerts/:publicSlug"
                  element={
                    <Suspense fallback={null}>
                      <PublicAlertPage />
                    </Suspense>
                  }
                />
                {/* Same reasoning - see PublicAudioPage's own doc comment. */}
                <Route
                  path="/overlay/audio/:publicSlug"
                  element={
                    <Suspense fallback={null}>
                      <PublicAudioPage />
                    </Suspense>
                  }
                />
                {/* Same reasoning - see PublicWidgetPage's own doc comment. */}
                <Route
                  path="/overlay/widgets/:publicSlug"
                  element={
                    <Suspense fallback={null}>
                      <PublicWidgetPage />
                    </Suspense>
                  }
                />
              </Routes>
            </AuthGate>
          </AuthProvider>
        </BrowserRouter>
      </QueryClientProvider>
    </I18nextProvider>
  );
}
