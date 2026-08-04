import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, type RenderResult } from '@testing-library/react';
import type { ReactElement, ReactNode } from 'react';
import { I18nextProvider } from 'react-i18next';

import { i18n } from '@/i18n';

/**
 * Renders a component wrapped with the same providers `App.tsx` supplies
 * (i18next, TanStack Query), for tests that render real components instead
 * of only exercising pure functions.
 *
 * Each call gets a fresh QueryClient with retries disabled, so a mocked
 * rejection resolves on the first attempt instead of the default retry
 * delay slowing the test down.
 */
export function renderWithProviders(ui: ReactElement): RenderResult {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });

  const wrap = (element: ReactNode) => (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>{element}</QueryClientProvider>
    </I18nextProvider>
  );

  const result = render(wrap(ui));
  return {
    ...result,
    rerender: (nextUi: ReactNode) => result.rerender(wrap(nextUi)),
  };
}
