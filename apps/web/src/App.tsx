import { QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { BrowserRouter, Route, Routes } from 'react-router-dom';

import { queryClient } from '@/app/query-client';
import { i18n } from '@/i18n';
import { ChatPage } from '@/pages/ChatPage';
import { DashboardPage } from '@/pages/DashboardPage';
import { EngagementPage } from '@/pages/EngagementPage';
import { NotFoundPage } from '@/pages/NotFoundPage';
import { LogsPage, MetadataPage, PlatformsPage } from '@/pages/PlannedPages';
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
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/logs" element={<LogsPage />} />
            <Route path="*" element={<NotFoundPage />} />
          </Routes>
        </BrowserRouter>
      </QueryClientProvider>
    </I18nextProvider>
  );
}
