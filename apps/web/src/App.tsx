import { QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Route, Routes } from 'react-router-dom';

import { queryClient } from '@/app/query-client';
import { DashboardPage } from '@/pages/DashboardPage';
import { NotFoundPage } from '@/pages/NotFoundPage';
import {
  LogsPage,
  MetadataPage,
  PlatformsPage,
  SettingsPage,
  StreamsPage,
} from '@/pages/PlannedPages';
import { DemoStreamProvider } from '@/state/DemoStreamProvider';

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <DemoStreamProvider>
        <BrowserRouter>
          <Routes>
            <Route path="/" element={<DashboardPage />} />
            <Route path="/platforms" element={<PlatformsPage />} />
            <Route path="/streams" element={<StreamsPage />} />
            <Route path="/metadata" element={<MetadataPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/logs" element={<LogsPage />} />
            <Route path="*" element={<NotFoundPage />} />
          </Routes>
        </BrowserRouter>
      </DemoStreamProvider>
    </QueryClientProvider>
  );
}
