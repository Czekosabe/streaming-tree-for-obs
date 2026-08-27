import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { i18n } from '@/i18n';

import { ConfirmDialog } from './ConfirmDialog';
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

  it('Shift+Tab wraps focus from the first control back to the last', async () => {
    renderModal({ dismissible: false });
    const user = userEvent.setup();

    const first = screen.getByRole('button', { name: 'First control' });
    const last = screen.getByRole('button', { name: 'Last control' });

    first.focus();
    expect(first).toHaveFocus();
    await user.tab({ shift: true });
    expect(last).toHaveFocus();
  });

  it('Escape closes a dismissible modal', async () => {
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

    await userEvent.setup().keyboard('{Escape}');
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  // Stage 20E regression: a real physical/manual Windows test found that
  // typing into the Add Platform form's Display name field kicked focus to
  // the modal's Close button after every single character. The root cause
  // was in the shared Modal, not Add Platform: the focus-trap/lifecycle
  // effect depended on `onClose`, and every caller in the app passes a new
  // `onClose` closure on each render (an inline arrow, or an unmemoized
  // handler) - so a controlled input's own re-render (needed just to show
  // the typed character) re-ran the whole effect, including the "move
  // focus to the panel's first focusable element" step. The existing Tab/
  // Escape/backdrop tests above never caught this because none of them
  // hold an open modal through a series of *unrelated* controlled-state
  // re-renders the way real typing does.
  it('typing into a controlled input keeps focus there through every rerender it causes', async () => {
    function Harness() {
      const [open, setOpen] = useState(true);
      const [value, setValue] = useState('');
      return (
        <Modal open={open} onClose={() => setOpen(false)} title="Test dialog">
          <label htmlFor="name-field">Display name</label>
          <input
            id="name-field"
            value={value}
            onChange={(event) => setValue(event.target.value)}
          />
        </Modal>
      );
    }
    render(
      <I18nextProvider i18n={i18n}>
        <Harness />
      </I18nextProvider>,
    );

    const user = userEvent.setup();
    const input = screen.getByLabelText('Display name');

    await user.click(input);
    expect(input).toHaveFocus();

    await user.type(input, 'Streaming Tree');

    expect(input).toHaveValue('Streaming Tree');
    expect(input).toHaveFocus();
  });
});

describe('Modal background scroll lock', () => {
  beforeEach(() => {
    document.body.style.overflow = '';
  });

  afterEach(() => {
    document.body.style.overflow = '';
  });

  it('locks the background while open and restores it once closed', async () => {
    function Harness() {
      const [open, setOpen] = useState(true);
      return (
        <Modal open={open} onClose={() => setOpen(false)} title="Test dialog">
          <button type="button">Inside</button>
        </Modal>
      );
    }
    render(
      <I18nextProvider i18n={i18n}>
        <Harness />
      </I18nextProvider>,
    );

    expect(document.body.style.overflow).toBe('hidden');

    await userEvent.setup().keyboard('{Escape}');
    expect(document.body.style.overflow).toBe('');
  });

  // Stage 20E regression: a real physical/manual Windows test found the
  // Dashboard permanently unscrollable (until a manual refresh) after
  // deleting a destination. `PlatformSettingsDialog` (the real component
  // this reproduces the shape of) keeps its own settings Modal open
  // alongside a `ConfirmDialog`-wrapped Modal for the delete step - two
  // modals locking the background at once. A successful delete closes
  // both in the same batched update; before the shared, reference-counted
  // lock existed, whichever modal's cleanup ran second overwrote the
  // other's correct restoration with its own contaminated "previous
  // value" (captured while the other modal had already locked the page),
  // leaving the body permanently locked.
  it('stays locked while a nested confirm dialog is open, and restores once both close together', async () => {
    function Harness() {
      const [settingsOpen, setSettingsOpen] = useState(true);
      const [confirmOpen, setConfirmOpen] = useState(false);

      // Mirrors PlatformSettingsDialog.handleDelete: both the confirm
      // dialog and the settings modal close in the same event handler,
      // landing in the same batched React update - the exact sequence
      // the real bug depended on.
      const handleDeleteConfirmed = () => {
        setConfirmOpen(false);
        setSettingsOpen(false);
      };

      return (
        <>
          <Modal open={settingsOpen} onClose={() => setSettingsOpen(false)} title="Platform settings">
            <button type="button" onClick={() => setConfirmOpen(true)}>
              Delete destination
            </button>
          </Modal>
          <ConfirmDialog
            open={confirmOpen}
            title="Delete platform?"
            message="This cannot be undone."
            confirmLabel="Confirm delete"
            destructive
            onConfirm={handleDeleteConfirmed}
            onCancel={() => setConfirmOpen(false)}
          />
        </>
      );
    }
    render(
      <I18nextProvider i18n={i18n}>
        <Harness />
      </I18nextProvider>,
    );

    const user = userEvent.setup();
    expect(document.body.style.overflow).toBe('hidden');

    // Opens the nested confirm dialog - both modals are now open at once.
    await user.click(screen.getByRole('button', { name: 'Delete destination' }));
    const confirmButton = await screen.findByRole('button', { name: 'Confirm delete' });
    expect(document.body.style.overflow).toBe('hidden');

    // Confirming closes BOTH modals in one batched update, same as
    // PlatformSettingsDialog.handleDelete's real onSuccess callback.
    await user.click(confirmButton);

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(document.body.style.overflow).toBe('');
  });
});
