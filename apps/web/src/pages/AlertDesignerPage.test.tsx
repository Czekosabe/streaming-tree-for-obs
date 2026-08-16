import { screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';

import * as alertsApi from '@/api/alerts';
import type { AlertProfile, AlertRule } from '@/api/alerts-schemas';
import * as visualDesignApi from '@/api/visualdesign';
import { renderWithProviders } from '@/test/render';

import { AlertDesignerPage } from './AlertDesignerPage';

vi.mock('@/api/alerts');
vi.mock('@/api/visualdesign');

const rule: AlertRule = {
  id: 'alrule_1', profileId: 'alprof_1', name: 'Follow alert', enabled: true, eventType: 'follow',
  priority: 50, durationMs: 5000, requiredRole: 'everyone', showPlatform: true, showUsername: true,
  showMessage: false, showQuantity: false, textTemplate: '{username} followed!', entryAnimation: 'fade',
  exitAnimation: 'fade', animationDurationMs: 400, providers: [], accounts: [],
  showAmount: false,
  allowGrouping: false, groupWindowMs: 5000, interruptMode: 'never', interruptible: true,
  audio: { soundEnabled: false, soundVolume: 1, ttsEnabled: false, ttsVolume: 1 },
  createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
};

const profile: AlertProfile = {
  id: 'alprof_1', publicSlug: 'slug', name: 'Main', enabled: true, language: 'en', theme: 'minimal',
  position: 'bottom', textAlign: 'center', maxQueueItems: 100, maximumQueueAgeSeconds: 120,
  createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z',
};

function renderPage() {
  return renderWithProviders(
    <MemoryRouter initialEntries={['/alerts/rules/alrule_1/designer']}>
      <Routes>
        <Route path="/alerts/rules/:ruleId/designer" element={<AlertDesignerPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('AlertDesignerPage', () => {
  it('shows a loading state before every dependency has resolved', () => {
    vi.mocked(alertsApi).fetchAlertRule.mockReturnValue(new Promise(() => {}));
    vi.mocked(alertsApi).fetchAlertEventTypes.mockResolvedValue([]);
    renderPage();
    expect(screen.getByTestId('alert-designer-loading')).toBeInTheDocument();
  });

  it('shows an error state when the rule fails to load', async () => {
    vi.mocked(alertsApi).fetchAlertRule.mockRejectedValue(new Error('not found'));
    vi.mocked(alertsApi).fetchAlertEventTypes.mockResolvedValue([]);
    renderPage();
    expect(await screen.findByTestId('alert-designer-error')).toBeInTheDocument();
  });

  it('renders the workspace once the rule, profile, event types and design have all loaded', async () => {
    vi.mocked(alertsApi).fetchAlertRule.mockResolvedValue(rule);
    vi.mocked(alertsApi).fetchAlertProfile.mockResolvedValue(profile);
    vi.mocked(alertsApi).fetchAlertEventTypes.mockResolvedValue([]);
    vi.mocked(alertsApi).previewAlertTemplate.mockResolvedValue({
      renderedText: 'Ann followed!', codePointCount: 12, resolvedPlaceholders: [], unresolvedPlaceholders: [], validForProvider: true,
    });
    vi.mocked(visualDesignApi).fetchVisualDesign.mockResolvedValue({
      persisted: false, revision: 0,
      document: { version: 1, canvas: { width: 1920, height: 1080, transparent: true }, layers: [] },
    });
    renderPage();
    expect(await screen.findByTestId('alert-designer-workspace')).toBeInTheDocument();
  });
});
