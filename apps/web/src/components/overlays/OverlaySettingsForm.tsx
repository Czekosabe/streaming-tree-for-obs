import { useTranslation } from 'react-i18next';

import type { ChatOverlayEditableFields } from '@/api/chat-overlay-schemas';
import { FormField } from '@/components/ui/FormField';
import { SelectInput } from '@/components/ui/SelectInput';
import { TextInput } from '@/components/ui/TextInput';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import type { SelectOption } from '@/models/platform';

type OverlaySettingsFormProps = {
  draft: ChatOverlayEditableFields;
  onChange: (next: ChatOverlayEditableFields) => void;
  /** True once a visual design is saved for this overlay - the layout/
   * alignment and font/color/animation/highlight sections below become
   * legacy-fallback-only in that case (Stage 13B task Part 10/23: "the
   * legacy controls remain stored so Reset to legacy restores the
   * previous presentation... never two panels appearing to control the
   * same output"). Filtering/lifecycle fields (account/hidden-user/
   * blocked-term selection above this form, maxVisibleItems, message
   * lifetime, and the bot/command/activity-type toggles below) remain
   * fully authoritative regardless - only sections that are genuinely
   * visual-presentation-only get the "legacy" framing. */
  designActive?: boolean;
};

const HEX_COLOR_PATTERN = /^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$/;

/** The bounded numeric ranges this form enforces client-side, mirroring
 * `internal/domain/chatoverlay/validation.go` exactly - the backend is
 * still the authority (a save can still fail its own validation), this
 * is only to keep the control's own min/max sane. */
const RANGES = {
  maxVisibleItems: { min: 1, max: 100 },
  messageLifetimeSeconds: { min: 0, max: 600 },
  fontSize: { min: 8, max: 64 },
  fontWeight: { min: 100, max: 900 },
  lineHeight: { min: 1, max: 3 },
  bubbleOpacity: { min: 0, max: 1 },
  borderRadius: { min: 0, max: 64 },
  itemSpacing: { min: 0, max: 64 },
  animationDurationMs: { min: 0, max: 5000 },
} as const;

/**
 * The full visual/filtering settings form - operates entirely on an
 * in-progress draft the parent owns (see OverlayEditor.tsx's own
 * draft-then-explicit-save state machine, Part 19). Never saves on its
 * own; every change flows back through `onChange`.
 */
export function OverlaySettingsForm({ draft, onChange, designActive = false }: OverlaySettingsFormProps) {
  const { t } = useTranslation('overlays');
  const { t: tChat } = useTranslation('chatOverlayDesigner');

  function set<K extends keyof ChatOverlayEditableFields>(key: K, value: ChatOverlayEditableFields[K]) {
    onChange({ ...draft, [key]: value });
  }

  const layoutOptions: SelectOption[] = [
    { value: 'horizontal', label: t('settings.layoutHorizontal') },
    { value: 'vertical', label: t('settings.layoutVertical') },
  ];
  const stackOptions: SelectOption[] = [
    { value: 'top_down', label: t('settings.stackTopDown') },
    { value: 'bottom_up', label: t('settings.stackBottomUp') },
  ];
  const alignOptions: SelectOption[] = [
    { value: 'left', label: t('settings.alignLeft') },
    { value: 'center', label: t('settings.alignCenter') },
    { value: 'right', label: t('settings.alignRight') },
  ];
  const fontFamilyOptions: SelectOption[] = [
    { value: 'sans_serif', label: t('settings.fontFamilySansSerif') },
    { value: 'serif', label: t('settings.fontFamilySerif') },
    { value: 'monospace', label: t('settings.fontFamilyMonospace') },
    { value: 'rounded', label: t('settings.fontFamilyRounded') },
  ];
  const usernameColorOptions: SelectOption[] = [
    { value: 'provider', label: t('settings.usernameColorProvider') },
    { value: 'fixed', label: t('settings.usernameColorFixed') },
  ];
  const animationOptions: SelectOption[] = [
    { value: 'none', label: t('settings.animationNone') },
    { value: 'fade', label: t('settings.animationFade') },
    { value: 'slide_up', label: t('settings.animationSlideUp') },
    { value: 'slide_left', label: t('settings.animationSlideLeft') },
    { value: 'scale', label: t('settings.animationScale') },
  ];
  const languageOptions: SelectOption[] = [
    { value: 'en', label: t('settings.languageEnglish') },
    { value: 'pl', label: t('settings.languagePolish') },
  ];

  function colorField(
    label: string,
    key: 'textColor' | 'bubbleColor',
  ) {
    const value = draft[key];
    const valid = HEX_COLOR_PATTERN.test(value);
    return (
      <FormField label={label} error={valid ? undefined : t('validation.invalidColor')}>
        {({ inputId, describedBy }) => (
          <div className="flex items-center gap-2">
            <input
              type="color"
              aria-label={label}
              value={valid ? value.slice(0, 7) : '#000000'}
              onChange={(event) => set(key, event.target.value)}
              className="h-9 w-10 shrink-0 cursor-pointer rounded border border-line bg-transparent"
            />
            <TextInput
              id={inputId}
              aria-describedby={describedBy}
              value={value}
              onChange={(event) => set(key, event.target.value)}
            />
          </div>
        )}
      </FormField>
    );
  }

  const legacySectionClassName = designActive
    ? 'space-y-1 rounded-lg border border-line-strong/60 bg-surface-sunken/60 p-3'
    : undefined;

  return (
    <div className="space-y-6">
      {designActive ? (
        <p className="text-xs font-medium text-ink-muted" data-testid="overlay-legacy-presentation-label">
          {tChat('designerLink.legacyDescription')}
        </p>
      ) : null}
      <div className={legacySectionClassName}>
        <section className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <FormField label={t('settings.layout')}>
            {({ inputId }) => (
              <SelectInput
                id={inputId}
                options={layoutOptions}
                value={draft.layoutMode}
                onChange={(e) => set('layoutMode', e.target.value as ChatOverlayEditableFields['layoutMode'])}
              />
            )}
          </FormField>
          <FormField label={t('settings.stackDirection')}>
            {({ inputId }) => (
              <SelectInput
                id={inputId}
                options={stackOptions}
                value={draft.stackDirection}
                onChange={(e) => set('stackDirection', e.target.value as ChatOverlayEditableFields['stackDirection'])}
              />
            )}
          </FormField>
          <FormField label={t('settings.horizontalAlignment')}>
            {({ inputId }) => (
              <SelectInput
                id={inputId}
                options={alignOptions}
                value={draft.horizontalAlignment}
                onChange={(e) =>
                  set('horizontalAlignment', e.target.value as ChatOverlayEditableFields['horizontalAlignment'])
                }
              />
            )}
          </FormField>
        </section>
      </div>

      <section className="grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
        <ToggleSwitch label={t('settings.showPlatformIcon')} checked={draft.showPlatformIcon} onCheckedChange={(v) => set('showPlatformIcon', v)} />
        <ToggleSwitch label={t('settings.showPlatformName')} checked={draft.showPlatformName} onCheckedChange={(v) => set('showPlatformName', v)} />
        <ToggleSwitch label={t('settings.showAccountLabel')} checked={draft.showAccountLabel} onCheckedChange={(v) => set('showAccountLabel', v)} />
        <ToggleSwitch label={t('settings.showAvatar')} checked={draft.showAvatar} onCheckedChange={(v) => set('showAvatar', v)} />
        <ToggleSwitch label={t('settings.showBadges')} checked={draft.showBadges} onCheckedChange={(v) => set('showBadges', v)} />
        <ToggleSwitch label={t('settings.showTimestamp')} checked={draft.showTimestamp} onCheckedChange={(v) => set('showTimestamp', v)} />
        <ToggleSwitch label={t('settings.showActivityEvents')} checked={draft.showActivityEvents} onCheckedChange={(v) => set('showActivityEvents', v)} />
        <ToggleSwitch label={t('settings.showDeletedPlaceholder')} checked={draft.showDeletedPlaceholder} onCheckedChange={(v) => set('showDeletedPlaceholder', v)} />
        <ToggleSwitch label={t('settings.hideCommands')} checked={draft.hideCommands} onCheckedChange={(v) => set('hideCommands', v)} />
        <ToggleSwitch label={t('settings.hideBots')} checked={draft.hideBots} onCheckedChange={(v) => set('hideBots', v)} />
      </section>

      <section className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <FormField label={t('settings.maxVisibleItems')}>
          {({ inputId }) => (
            <TextInput
              id={inputId}
              type="number"
              min={RANGES.maxVisibleItems.min}
              max={RANGES.maxVisibleItems.max}
              value={draft.maxVisibleItems}
              onChange={(e) => set('maxVisibleItems', Number(e.target.value))}
            />
          )}
        </FormField>
        <FormField label={t('settings.messageLifetimeSeconds')}>
          {({ inputId }) => (
            <TextInput
              id={inputId}
              type="number"
              min={RANGES.messageLifetimeSeconds.min}
              max={RANGES.messageLifetimeSeconds.max}
              value={draft.messageLifetimeSeconds}
              onChange={(e) => set('messageLifetimeSeconds', Number(e.target.value))}
            />
          )}
        </FormField>
      </section>

      <div className={legacySectionClassName}>
      <section className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <FormField label={t('settings.fontFamily')}>
          {({ inputId }) => (
            <SelectInput
              id={inputId}
              options={fontFamilyOptions}
              value={draft.fontFamily}
              onChange={(e) => set('fontFamily', e.target.value as ChatOverlayEditableFields['fontFamily'])}
            />
          )}
        </FormField>
        <FormField label={t('settings.fontSize')}>
          {({ inputId }) => (
            <TextInput id={inputId} type="number" {...RANGES.fontSize} value={draft.fontSize} onChange={(e) => set('fontSize', Number(e.target.value))} />
          )}
        </FormField>
        <FormField label={t('settings.fontWeight')}>
          {({ inputId }) => (
            <TextInput id={inputId} type="number" step={100} {...RANGES.fontWeight} value={draft.fontWeight} onChange={(e) => set('fontWeight', Number(e.target.value))} />
          )}
        </FormField>
        <FormField label={t('settings.lineHeight')}>
          {({ inputId }) => (
            <TextInput id={inputId} type="number" step={0.1} {...RANGES.lineHeight} value={draft.lineHeight} onChange={(e) => set('lineHeight', Number(e.target.value))} />
          )}
        </FormField>
        <FormField label={t('settings.usernameColorMode')}>
          {({ inputId }) => (
            <SelectInput
              id={inputId}
              options={usernameColorOptions}
              value={draft.usernameColorMode}
              onChange={(e) => set('usernameColorMode', e.target.value as ChatOverlayEditableFields['usernameColorMode'])}
            />
          )}
        </FormField>
      </section>

      <section className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {colorField(t('settings.textColor'), 'textColor')}
        {colorField(t('settings.bubbleColor'), 'bubbleColor')}
        <FormField label={t('settings.bubbleOpacity')}>
          {({ inputId }) => (
            <TextInput id={inputId} type="number" step={0.05} {...RANGES.bubbleOpacity} value={draft.bubbleOpacity} onChange={(e) => set('bubbleOpacity', Number(e.target.value))} />
          )}
        </FormField>
        <FormField label={t('settings.borderRadius')}>
          {({ inputId }) => (
            <TextInput id={inputId} type="number" {...RANGES.borderRadius} value={draft.borderRadius} onChange={(e) => set('borderRadius', Number(e.target.value))} />
          )}
        </FormField>
        <FormField label={t('settings.itemSpacing')}>
          {({ inputId }) => (
            <TextInput id={inputId} type="number" {...RANGES.itemSpacing} value={draft.itemSpacing} onChange={(e) => set('itemSpacing', Number(e.target.value))} />
          )}
        </FormField>
      </section>

      <section className="grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
        <ToggleSwitch label={t('settings.textOutline')} checked={draft.textOutline} onCheckedChange={(v) => set('textOutline', v)} />
        <ToggleSwitch label={t('settings.textShadow')} checked={draft.textShadow} onCheckedChange={(v) => set('textShadow', v)} />
      </section>

      <section className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <FormField label={t('settings.entryAnimation')}>
          {({ inputId }) => (
            <SelectInput
              id={inputId}
              options={animationOptions}
              value={draft.entryAnimation}
              onChange={(e) => set('entryAnimation', e.target.value as ChatOverlayEditableFields['entryAnimation'])}
            />
          )}
        </FormField>
        <FormField label={t('settings.exitAnimation')}>
          {({ inputId }) => (
            <SelectInput
              id={inputId}
              options={animationOptions}
              value={draft.exitAnimation}
              onChange={(e) => set('exitAnimation', e.target.value as ChatOverlayEditableFields['exitAnimation'])}
            />
          )}
        </FormField>
        <FormField label={t('settings.animationDurationMs')}>
          {({ inputId }) => (
            <TextInput id={inputId} type="number" step={50} {...RANGES.animationDurationMs} value={draft.animationDurationMs} onChange={(e) => set('animationDurationMs', Number(e.target.value))} />
          )}
        </FormField>
      </section>

      <section className="grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
        <ToggleSwitch label={t('settings.highlightBroadcaster')} checked={draft.highlightBroadcaster} onCheckedChange={(v) => set('highlightBroadcaster', v)} />
        <ToggleSwitch label={t('settings.highlightModerators')} checked={draft.highlightModerators} onCheckedChange={(v) => set('highlightModerators', v)} />
        <ToggleSwitch label={t('settings.highlightSubscribers')} checked={draft.highlightSubscribers} onCheckedChange={(v) => set('highlightSubscribers', v)} />
        <ToggleSwitch label={t('settings.highlightVips')} checked={draft.highlightVips} onCheckedChange={(v) => set('highlightVips', v)} />
      </section>
      </div>

      <section className="max-w-xs">
        <FormField label={t('settings.language')}>
          {({ inputId }) => (
            <SelectInput
              id={inputId}
              options={languageOptions}
              value={draft.language}
              onChange={(e) => set('language', e.target.value as ChatOverlayEditableFields['language'])}
            />
          )}
        </FormField>
      </section>
    </div>
  );
}
