import { act, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import * as aboutApi from '@/api/about';
import * as runtimeApi from '@/api/runtime';
import { renderWithProviders } from '@/test/render';

import { AppShell, ShellLayout } from './AppShell';

vi.mock('@/api/runtime');
vi.mock('@/api/about');

/**
 * Covers Stage 20E defect A (sidebar scroll position resetting on
 * navigation) and defect C (the brand mark becoming a Dashboard link).
 *
 * Renders the same route shape App.tsx now uses: a `ShellLayout` layout
 * route wrapping two pages, each with its own `<AppShell>` - the real
 * production wiring, not a simplified stand-in.
 */
function renderApp(initialPath: string) {
  return renderWithProviders(
    <MemoryRouter initialEntries={[initialPath]}>
      <Routes>
        <Route element={<ShellLayout />}>
          <Route
            path="/"
            element={
              <AppShell title="Dashboard" description="Dashboard page">
                <div>dashboard-content</div>
              </AppShell>
            }
          />
          <Route
            path="/logs"
            element={
              <AppShell title="Logs" description="Logs page">
                <div>logs-content</div>
              </AppShell>
            }
          />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  // Never resolves: keeps SidebarFooter in its "checking" state, which is
  // irrelevant to what these tests assert on.
  vi.mocked(runtimeApi).fetchRuntime.mockReturnValue(new Promise(() => {}));
  vi.mocked(aboutApi).fetchAbout.mockReturnValue(new Promise(() => {}));
});

describe('ShellLayout / AppShell', () => {
  it('keeps the sidebar nav scroll container mounted (same DOM node) across a route change', async () => {
    renderApp('/');
    const nav = screen.getByRole('navigation', { name: /primary/i });
    const scrollContainer = nav.parentElement;
    expect(scrollContainer).not.toBeNull();

    // Simulate the operator having scrolled the sidebar down.
    act(() => {
      if (scrollContainer !== null) scrollContainer.scrollTop = 240;
    });
    expect(scrollContainer?.scrollTop).toBe(240);

    await userEvent.click(screen.getByRole('link', { name: /logs/i }));

    expect(await screen.findByText('logs-content')).toBeInTheDocument();
    // Same node, and its scroll position was never reset by the navigation -
    // the whole point of lifting the sidebar into a layout route.
    const scrollContainerAfter = screen.getByRole('navigation', { name: /primary/i }).parentElement;
    expect(scrollContainerAfter).toBe(scrollContainer);
    expect(scrollContainerAfter?.scrollTop).toBe(240);
  });

  it('renders the brand mark as a link back to the Dashboard', async () => {
    renderApp('/logs');

    const brandLink = screen.getByRole('link', { name: /streaming tree for obs.*dashboard/i });
    expect(brandLink).toHaveAttribute('href', '/');

    await userEvent.click(brandLink);

    expect(await screen.findByText('dashboard-content')).toBeInTheDocument();
  });

  it('opens the mobile drawer from a page-level AppShell menu button, via the ShellLayout outlet context', async () => {
    renderApp('/');

    await userEvent.click(screen.getByRole('button', { name: /open menu/i }));

    expect(screen.getByRole('dialog', { name: /main menu/i })).toHaveAttribute('aria-modal', 'true');
  });
});
