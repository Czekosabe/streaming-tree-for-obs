import { QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { BrowserRouter, Route, Routes } from 'react-router-dom';

import { queryClient } from '@/app/query-client';
import { i18n } from '@/i18n';
import { AlertDesignerPage } from '@/pages/AlertDesignerPage';
import { AlertsPage } from '@/pages/AlertsPage';
import { AutomationPage } from '@/pages/AutomationPage';
import { ChatPage } from '@/pages/ChatPage';
import { DashboardPage } from '@/pages/DashboardPage';
import { EngagementPage } from '@/pages/EngagementPage';
import { NotFoundPage } from '@/pages/NotFoundPage';
import { OverlayChatPage } from '@/pages/OverlayChatPage';
import { OverlaysPage } from '@/pages/OverlaysPage';
import { LogsPage, MetadataPage, PlatformsPage } from '@/pages/PlannedPages';
import { PublicAlertPage } from '@/pages/PublicAlertPage';
import { SettingsPage } from '@/pages/SettingsPage';
import { StreamsPage } from '@/pages/StreamsPage';

export function App() {
  return (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <Routes>
            <Route path="/" element={<DashboardPage />} />
            <Route path="/platforms" element={<PlatformsPage />} />
            <Route path="/streams" element={<StreamsPage />} />
            <Route path="/metadata" element={<MetadataPage />} />
            <Route path="/engagement" element={<EngagementPage />} />
            <Route path="/chat" element={<ChatPage />} />
            <Route path="/overlays" element={<OverlaysPage />} />
            <Route path="/automation" element={<AutomationPage />} />
            <Route path="/alerts" element={<AlertsPage />} />
            {/* No AppShell: the Designer wants full control of the
                viewport for its own canvas/panel layout and top bar -
                see AlertDesignerPage's own doc comment. */}
            <Route path="/alerts/rules/:ruleId/designer" element={<AlertDesignerPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/logs" element={<LogsPage />} />
            {/* No AppShell: a standalone Browser Source page, never the
                operator dashboard's chrome - see OverlayChatPage's own
                doc comment. */}
            <Route path="/overlay/chat/:publicSlug" element={<OverlayChatPage />} />
            {/* No AppShell here either - see PublicAlertPage's own doc
                comment. */}
            <Route path="/overlay/alerts/:publicSlug" element={<PublicAlertPage />} />
            <Route path="*" element={<NotFoundPage />} />
          </Routes>
        </BrowserRouter>
      </QueryClientProvider>
    </I18nextProvider>
  );
}
