import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { PublicAlert, PublicAlertProfileConfig } from '@/api/alerts-schemas';
import { VisualDesignRenderer } from '@/components/visual-design/VisualDesignRenderer';
import { cn } from '@/lib/cn';
import { providerGlyphClass } from '@/models/provider-labels';

import {
  alertContainerStyle,
  alertEntryAnimationClassName,
  alertExitAnimationClassName,
  alertExitAnimationFallbackMs,
  alertTextAlignStyle,
  alertThemeClassName,
} from './alert-style';

function supportsMatchMedia(): boolean {
  return typeof window !== 'undefined' && typeof window.matchMedia === 'function';
}

/** Tracks the `prefers-reduced-motion` media query - mirrors
 * components/chat-overlay/ChatOverlayRenderer.tsx's own identical
 * hook. */
function usePrefersReducedMotion(): boolean {
  const [reduced, setReduced] = useState(
    () => supportsMatchMedia() && window.matchMedia('(prefers-reduced-motion: reduce)').matches,
  );
  useEffect(() => {
    if (!supportsMatchMedia()) return;
    const query = window.matchMedia('(prefers-reduced-motion: reduce)');
    const onChange = () => setReduced(query.matches);
    query.addEventListener('change', onChange);
    return () => query.removeEventListener('change', onChange);
  }, []);
  return reduced;
}

type Displayed = { alert: PublicAlert; leaving: boolean };

/**
 * The Browser Source alert renderer: a single, transparent, positioned
 * banner styled entirely from a validated public profile config plus
 * one alert's own presentation fields. Used both by the real public
 * route (pages/PublicAlertPage.tsx, which sizes its own full-viewport
 * wrapper) and the management page's local editor preview - this
 * component always fills 100% of whatever its parent gives it, never a
 * fixed viewport size of its own.
 *
 * Unlike the chat-overlay renderer (a scrolling list), an alert profile
 * has exactly one slot: this component tracks its own local "leaving"
 * transition when `current` becomes null, using a hard fallback timer
 * exactly like OverlayLeavingItem.tsx's own reasoning (Part 25: "hide
 * always completes using a hard fallback timer... never rely only on
 * animationend"). A new alert arriving while one is still mid-exit
 * simply replaces it immediately - reset must never leave an old alert
 * visible.
 */
export function AlertRenderer({
  config,
  current,
}: {
  config: PublicAlertProfileConfig;
  current: PublicAlert | null;
}) {
  const { t } = useTranslation('alerts');
  const prefersReducedMotion = usePrefersReducedMotion();
  const [displayed, setDisplayed] = useState<Displayed | null>(null);

  useEffect(() => {
    if (current !== null) {
      setDisplayed({ alert: current, leaving: false });
      return;
    }
    setDisplayed((prev) => (prev === null || prev.leaving ? prev : { ...prev, leaving: true }));
  }, [current]);

  const exitClass = displayed
    ? alertExitAnimationClassName(displayed.alert.exitAnimation, prefersReducedMotion)
    : '';

  useEffect(() => {
    if (displayed === null || !displayed.leaving) return;
    if (exitClass === '') {
      setDisplayed(null);
      return;
    }
    const timeout = window.setTimeout(
      () => setDisplayed(null),
      alertExitAnimationFallbackMs(displayed.alert.animationDurationMs),
    );
    return () => window.clearTimeout(timeout);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- fire once per leaving alert id, not on every config identity change
  }, [displayed?.alert.alertId, displayed?.leaving, exitClass]);

  if (displayed === null) {
    return <div className="h-full w-full" style={alertContainerStyle(config.position, 0)} data-testid="alert-root" />;
  }

  const { alert, leaving } = displayed;
  const animationClass = leaving
    ? exitClass
    : alertEntryAnimationClassName(alert.entryAnimation, prefersReducedMotion);

  const isVisualDesign = alert.renderingMode === 'visual_design' && alert.visualDesign !== null && alert.visualDesign !== undefined;

  return (
    <div
      className="h-full w-full"
      style={alertContainerStyle(config.position, alert.animationDurationMs)}
      data-testid="alert-root"
    >
      {isVisualDesign && alert.visualDesign ? (
        <div
          className={cn('h-full w-full', animationClass)}
          data-testid="alert-item"
          data-rendering-mode="visual_design"
          data-alert-id={alert.alertId}
          data-synthetic={alert.synthetic}
          onAnimationEnd={leaving ? () => setDisplayed(null) : undefined}
        >
          <VisualDesignRenderer
            canvas={alert.visualDesign.canvas}
            layers={alert.visualDesign.layers}
            alert={{
              eventType: alert.eventType,
              providerId: alert.providerId,
              username: alert.username ?? null,
              message: alert.message ?? null,
              quantity: alert.quantity ?? null,
              groupCount: alert.groupCount,
              renderedText: alert.renderedText,
              avatarUrl: alert.avatarUrl ?? null,
            }}
            mode="public"
            prefersReducedMotion={prefersReducedMotion}
          />
        </div>
      ) : (
        <div
          className={cn('max-w-[90%]', alertThemeClassName(config.theme), animationClass)}
          style={alertTextAlignStyle(config.textAlign)}
          data-testid="alert-item"
          data-rendering-mode="legacy"
          data-alert-id={alert.alertId}
          data-synthetic={alert.synthetic}
          onAnimationEnd={leaving ? () => setDisplayed(null) : undefined}
        >
          <div data-testid="alert-text">{alert.renderedText}</div>
          {(alert.quantity !== null && alert.quantity !== undefined) || alert.providerId !== '' || alert.groupCount > 1 ? (
            <div className="mt-1 flex items-center justify-center gap-2 text-xs opacity-80" data-testid="alert-meta">
              {alert.providerId !== '' ? (
                <span className={providerGlyphClass(alert.providerId)} data-testid="alert-provider-glyph">
                  {alert.providerId}
                </span>
              ) : null}
              {alert.quantity !== null && alert.quantity !== undefined ? (
                <span data-testid="alert-quantity">{alert.quantity}</span>
              ) : null}
              {alert.groupCount > 1 ? (
                <span
                  className="rounded-full border border-current/40 px-1.5 py-0.5 font-mono tabular-nums"
                  data-testid="alert-group-count"
                  aria-label={t('renderer.groupCountLabel', { count: alert.groupCount })}
                >
                  {`×${alert.groupCount}`}
                </span>
              ) : null}
            </div>
          ) : null}
        </div>
      )}
    </div>
  );
}
