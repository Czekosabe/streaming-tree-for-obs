import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { I18nextProvider } from 'react-i18next';
import { describe, expect, it, vi } from 'vitest';

import { i18n } from '@/i18n';

import { Modal } from './Modal';

/**
 * Stage 20E regression coverage for the modal-layering contract found
 * broken by a real physical/manual Windows test: unrelated page
 * content (using the same `animate-fade-rise` entrance animation
 * every card/tab-panel in the app uses) rendered above an open modal
 * dialog. jsdom has no real layout/paint engine, so "which element
 * visually paints on top" cannot be asserted directly here - what
 * this file verifies is the two things that *are* meaningfully
 * testable in jsdom and are exactly what fixes that class of bug
 * structurally: that `Modal` renders through a React portal directly
 * under `document.body` (never as a descendant of whatever rendered
 * it, so no ancestor's CSS can ever trap its stacking order again),
 * and that its focus-trap/Escape/backdrop behavior survived the
 * portal conversion unchanged.
 */
function renderModal(props: Partial<Parameters<typeof Modal>[0]> = {}) {
  function Harness() {
    const [open, setOpen] = useState(props.open ?? true);
    return (
      <div data-testid="app-render-root">
        <button type="button">Opener</button>
        <Modal
          open={open}
          onClose={() => setOpen(false)}
          title="Test dialog"
          {...props}
        >
          <button type="button">First control</button>
          <button type="button">Last control</button>
        </Modal>
      </div>
    );
  }
  return render(
    <I18nextProvider i18n={i18n}>
      <Harness />
    </I18nextProvider>,
  );
}

describe('Modal', () => {
  it('renders through a portal directly under document.body, never as a descendant of its own render root', () => {
    renderModal();

    const dialog = screen.getByRole('dialog');
    const renderRoot = screen.getByTestId('app-render-root');

    expect(renderRoot.contains(dialog)).toBe(false);
    expect(document.body.contains(dialog)).toBe(true);
  });

  it('moves focus into the panel on open and restores it to the opener on close', async () => {
    // dismissible={false} (no header Close button rendered) so "Inside"
    // is unambiguously the panel's first focusable element - Escape is
    // exercised separately below, closing is done by toggling `open`
    // instead here.
    function Harness() {
      const [open, setOpen] = useState(false);
      return (
        <div>
          <button type="button" onClick={() => setOpen((v) => !v)}>
            Open
          </button>
          <Modal open={open} onClose={() => setOpen(false)} title="Test dialog" dismissible={false}>
            <button type="button">Inside</button>
          </Modal>
        </div>
      );
    }
    render(
      <I18nextProvider i18n={i18n}>
        <Harness />
      </I18nextProvider>,
    );

    const user = userEvent.setup();
    const openButton = screen.getByRole('button', { name: 'Open' });
    openButton.focus();
    await user.click(openButton);

    expect(await screen.findByRole('button', { name: 'Inside' })).toHaveFocus();

    await user.click(openButton);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(openButton).toHaveFocus();
  });

  it('Escape does nothing while not dismissible', async () => {
    renderModal({ dismissible: false });
    const user = userEvent.setup();
    await user.keyboard('{Escape}');
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('clicking the backdrop closes a dismissible modal', async () => {
    const onClose = vi.fn();
    function Harness() {
      const [open, setOpen] = useState(true);
      return (
        <Modal
          open={open}
          onClose={() => {
            onClose();
            setOpen(false);
          }}
          title="Test dialog"
        >
          <button type="button">Inside</button>
        </Modal>
      );
    }
    render(
      <I18nextProvider i18n={i18n}>
        <Harness />
      </I18nextProvider>,
    );

    const backdrop = document.querySelector('[role="presentation"]');
    expect(backdrop).not.toBeNull();
    await userEvent.setup().click(backdrop as Element);

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('Tab wraps focus from the last control back to the first', async () => {
    // dismissible={false}: without it the header's own Close button is
    // the panel's real first focusable element, which would make
    // "First control" a false name for this test's purposes.
    renderModal({ dismissible: false });
    const user = userEvent.setup();

    const first = screen.getByRole('button', { name: 'First control' });
    const last = screen.getByRole('button', { name: 'Last control' });

    last.focus();
    expect(last).toHaveFocus();
    await user.tab();
    expect(first).toHaveFocus();
  });
});
