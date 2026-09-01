import { QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { BrowserRouter, Route, Routes } from 'react-router-dom';

import { AuthProvider } from '@/app/auth-context';
import { queryClient } from '@/app/query-client';
import { AuthGate } from '@/components/auth/AuthGate';
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
                <Route path="/" element={<DashboardPage />} />
                <Route path="/onboarding" element={<OnboardingPage />} />
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
                {/* No AppShell: the Designer wants full control of the
                    viewport for its own canvas/panel layout and top bar -
                    see AlertDesignerPage's own doc comment. */}
                <Route path="/alerts/rules/:ruleId/designer" element={<AlertDesignerPage />} />
                {/* No AppShell here either - see ChatOverlayDesignerPage's own
                    doc comment. */}
                <Route path="/overlays/:overlayId/designer" element={<ChatOverlayDesignerPage />} />
                <Route path="/settings" element={<SettingsPage />} />
                <Route path="/settings/about" element={<AboutLegalPage />} />
                <Route path="/logs" element={<LogsPage />} />
                <Route path="/history" element={<HistoryPage />} />
                {/* No AppShell: a standalone Browser Source page, never the
                    operator dashboard's chrome - see OverlayChatPage's own
                    doc comment. */}
                <Route path="/overlay/chat/:publicSlug" element={<OverlayChatPage />} />
                {/* No AppShell here either - see PublicAlertPage's own doc
                    comment. */}
                <Route path="/overlay/alerts/:publicSlug" element={<PublicAlertPage />} />
                {/* No AppShell here either - see PublicAudioPage's own doc
                    comment. */}
                <Route path="/overlay/audio/:publicSlug" element={<PublicAudioPage />} />
                {/* No AppShell here either - see PublicWidgetPage's own doc
                    comment. */}
                <Route path="/overlay/widgets/:publicSlug" element={<PublicWidgetPage />} />
                <Route path="*" element={<NotFoundPage />} />
              </Routes>
            </AuthGate>
          </AuthProvider>
        </BrowserRouter>
      </QueryClientProvider>
    </I18nextProvider>
  );
}
