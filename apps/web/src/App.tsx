import { QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { BrowserRouter, Route, Routes } from 'react-router-dom';

import { AuthProvider } from '@/app/auth-context';
import { queryClient } from '@/app/query-client';
import { AuthGate } from '@/components/auth/AuthGate';
import { ShellLayout } from '@/components/layout/AppShell';
import { i18n } from '@/i18n';
import { AboutLegalPage } from '@/pages/AboutLegalPage';
import { AlertDesignerPage } from '@/pages/AlertDesignerPage';
import { AlertsPage } from '@/pages/AlertsPage';
import { AudioPage } from '@/pages/AudioPage';
import { AutomationPage } from '@/pages/AutomationPage';
import { ChatOverlayDesignerPage } from '@/pages/ChatOverlayDesignerPage';
import { ChatPage } from '@/pages/ChatPage';
import { DashboardPage } from '@/pages/DashboardPage';
import { EngagementPage } from '@/pages/EngagementPage';
import { GoalsPage } from '@/pages/GoalsPage';
import { HistoryPage } from '@/pages/HistoryPage';
import { LogsPage } from '@/pages/LogsPage';
import { OnboardingAutoRedirect } from '@/components/onboarding/OnboardingAutoRedirect';
import { NotFoundPage } from '@/pages/NotFoundPage';
import { OnboardingPage } from '@/pages/OnboardingPage';
import { OverlayChatPage } from '@/pages/OverlayChatPage';
import { OverlaysPage } from '@/pages/OverlaysPage';
import { MetadataPage, PlatformsPage } from '@/pages/PlannedPages';
import { PublicAlertPage } from '@/pages/PublicAlertPage';
import { PublicAudioPage } from '@/pages/PublicAudioPage';
import { PublicWidgetPage } from '@/pages/PublicWidgetPage';
import { SettingsPage } from '@/pages/SettingsPage';
import { StreamsPage } from '@/pages/StreamsPage';

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
                    for its title/description/actions, unchanged. */}
                <Route element={<ShellLayout />}>
                  <Route path="/" element={<DashboardPage />} />
                  <Route path="/platforms" element={<PlatformsPage />} />
                  <Route path="/streams" element={<StreamsPage />} />
                  <Route path="/metadata" element={<MetadataPage />} />
                  <Route path="/engagement" element={<EngagementPage />} />
                  <Route path="/chat" element={<ChatPage />} />
                  <Route path="/overlays" element={<OverlaysPage />} />
                  <Route path="/automation" element={<AutomationPage />} />
                  <Route path="/alerts" element={<AlertsPage />} />
                  <Route path="/audio" element={<AudioPage />} />
                  <Route path="/goals" element={<GoalsPage />} />
                  <Route path="/settings" element={<SettingsPage />} />
                  <Route path="/settings/about" element={<AboutLegalPage />} />
                  <Route path="/logs" element={<LogsPage />} />
                  <Route path="/history" element={<HistoryPage />} />
                  <Route path="*" element={<NotFoundPage />} />
                </Route>

                {/* Outside ShellLayout: deliberately no sidebar/nav chrome. */}
                <Route path="/onboarding" element={<OnboardingPage />} />
                {/* The Designer wants full control of the viewport for its
                    own canvas/panel layout and top bar - see
                    AlertDesignerPage's own doc comment. */}
                <Route path="/alerts/rules/:ruleId/designer" element={<AlertDesignerPage />} />
                {/* Same reasoning - see ChatOverlayDesignerPage's own doc
                    comment. */}
                <Route path="/overlays/:overlayId/designer" element={<ChatOverlayDesignerPage />} />
                {/* A standalone Browser Source page, never the operator
                    dashboard's chrome - see OverlayChatPage's own doc
                    comment. */}
                <Route path="/overlay/chat/:publicSlug" element={<OverlayChatPage />} />
                {/* Same reasoning - see PublicAlertPage's own doc comment. */}
                <Route path="/overlay/alerts/:publicSlug" element={<PublicAlertPage />} />
                {/* Same reasoning - see PublicAudioPage's own doc comment. */}
                <Route path="/overlay/audio/:publicSlug" element={<PublicAudioPage />} />
                {/* Same reasoning - see PublicWidgetPage's own doc comment. */}
                <Route path="/overlay/widgets/:publicSlug" element={<PublicWidgetPage />} />
              </Routes>
            </AuthGate>
          </AuthProvider>
        </BrowserRouter>
      </QueryClientProvider>
    </I18nextProvider>
  );
}
