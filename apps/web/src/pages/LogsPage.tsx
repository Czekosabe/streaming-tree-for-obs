import { Check, Copy, Download, FileText, Loader2, RefreshCw, ShieldCheck } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { LogEntry, LogSeverity } from '@/api/logs-schemas';
import { LOG_SEVERITIES } from '@/api/logs-schemas';
import { AppShell } from '@/components/layout/AppShell';
import { Button } from '@/components/ui/Button';
import { Panel, PanelBody, PanelHeader } from '@/components/ui/Panel';
import { SelectInput } from '@/components/ui/SelectInput';
import { TextInput } from '@/components/ui/TextInput';
import { useLogsQuery, useSupportBundleMutation, type LogsFilter } from '@/hooks/use-logs';
import { useLanguage } from '@/i18n/use-language';
import { resolveApiErrorMessage } from '@/lib/api-error-message';
import { formatTimestamp } from '@/lib/format';

/** Debounces free-text filter input so the frontend does not issue a
 * request per keystroke. Discrete controls (the severity select)
 * apply immediately instead - see their own onChange handlers. */
function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delayMs);
    return () => window.clearTimeout(timer);
  }, [value, delayMs]);
  return debounced;
}

const SEVERITY_BADGE_CLASSES: Record<LogSeverity, string> = {
  DEBUG: 'border-status-offline/40 bg-status-offline/12 text-status-offline',
  INFO: 'border-status-starting/40 bg-status-starting/12 text-status-starting',
  WARN: 'border-status-warning/40 bg-status-warning/12 text-status-warning',
  ERROR: 'border-status-error/40 bg-status-error/12 text-status-error',
};

function isKnownSeverity(value: string): value is LogSeverity {
  return (LOG_SEVERITIES as readonly string[]).includes(value);
}

function SeverityBadge({ severity, label }: { severity: string; label: string }) {
  const classes = isKnownSeverity(severity)
    ? SEVERITY_BADGE_CLASSES[severity]
    : SEVERITY_BADGE_CLASSES.INFO;

  return (
    <span
      className={`inline-flex shrink-0 items-center rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide ${classes}`}
    >
      {label}
    </span>
  );
}

function LogEntryText(entry: LogEntry): string {
  const parts = [entry.time, entry.severity, entry.subsystem, entry.message];
  return parts.join('\t');
}

function LogEntryRow({ entry, locale }: { entry: LogEntry; locale: string }) {
  const { t } = useTranslation('logs');
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) return;
    const timer = window.setTimeout(() => setCopied(false), 1_500);
    return () => window.clearTimeout(timer);
  }, [copied]);

  const severityLabel = isKnownSeverity(entry.severity)
    ? t(`filters.severity.${entry.severity}`)
    : entry.severity;

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(LogEntryText(entry));
      setCopied(true);
    } catch {
      setCopied(false);
    }
  };

  return (
    <div className="flex items-start gap-2 rounded-lg border border-line bg-surface-sunken px-3 py-2">
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <SeverityBadge severity={entry.severity} label={severityLabel} />
          <span className="font-mono text-[11px] text-ink-faint">
            {formatTimestamp(entry.time, locale)}
          </span>
          <span className="rounded bg-surface-raised px-1.5 py-0.5 font-mono text-[11px] text-ink-muted">
            {entry.subsystem}
          </span>
        </div>
        <p className="break-words text-xs leading-relaxed text-ink">{entry.message}</p>
      </div>
      <button
        type="button"
        onClick={() => void copy()}
        aria-label={t('list.copyEntry')}
        title={t('list.copyEntry')}
        className="inline-flex size-6 shrink-0 items-center justify-center rounded border border-line text-ink-faint transition-colors hover:border-line-strong hover:text-ink"
      >
        {copied ? (
          <Check aria-hidden="true" className="size-3 text-status-live" />
        ) : (
          <Copy aria-hidden="true" className="size-3" />
        )}
      </button>
    </div>
  );
}

function LogsFilterBar({
  severity,
  onSeverityChange,
  subsystemInput,
  onSubsystemInputChange,
  searchInput,
  onSearchInputChange,
  onClear,
  hasActiveFilters,
  onRefresh,
  refreshing,
}: {
  severity: string;
  onSeverityChange: (value: string) => void;
  subsystemInput: string;
  onSubsystemInputChange: (value: string) => void;
  searchInput: string;
  onSearchInputChange: (value: string) => void;
  onClear: () => void;
  hasActiveFilters: boolean;
  onRefresh: () => void;
  refreshing: boolean;
}) {
  const { t } = useTranslation('logs');

  return (
    <div className="flex flex-wrap items-end gap-3">
      <div className="space-y-1.5">
        <label className="text-xs font-medium text-ink-muted" htmlFor="logs-severity">
          {t('filters.severityLabel')}
        </label>
        <SelectInput
          id="logs-severity"
          className="h-9 w-40"
          value={severity}
          onChange={(event) => onSeverityChange(event.target.value)}
          options={[
            { value: '', label: t('filters.severityAll') },
            ...LOG_SEVERITIES.map((value) => ({ value, label: t(`filters.severity.${value}`) })),
          ]}
        />
      </div>

      <div className="space-y-1.5">
        <label className="text-xs font-medium text-ink-muted" htmlFor="logs-subsystem">
          {t('filters.subsystemLabel')}
        </label>
        <TextInput
          id="logs-subsystem"
          className="h-9 w-40"
          placeholder={t('filters.subsystemPlaceholder')}
          value={subsystemInput}
          onChange={(event) => onSubsystemInputChange(event.target.value)}
        />
      </div>

      <div className="min-w-0 flex-1 space-y-1.5">
        <label className="text-xs font-medium text-ink-muted" htmlFor="logs-search">
          {t('filters.searchLabel')}
        </label>
        <TextInput
          id="logs-search"
          className="h-9 w-full"
          placeholder={t('filters.searchPlaceholder')}
          value={searchInput}
          onChange={(event) => onSearchInputChange(event.target.value)}
        />
      </div>

      <div className="flex items-center gap-2">
        {hasActiveFilters && (
          <Button variant="ghost" size="sm" onClick={onClear}>
            {t('filters.clearFilters')}
          </Button>
        )}
        <Button
          variant="secondary"
          size="sm"
          disabled={refreshing}
          icon={
            refreshing ? (
              <Loader2 aria-hidden="true" className="size-3.5 animate-spin" />
            ) : (
              <RefreshCw aria-hidden="true" className="size-3.5" />
            )
          }
          onClick={onRefresh}
        >
          {refreshing ? t('filters.refreshing') : t('filters.refresh')}
        </Button>
      </div>
    </div>
  );
}

function SupportBundlePanel() {
  const { t } = useTranslation(['logs', 'errors']);
  const tErrors = useTranslation('errors').t;
  const bundleMutation = useSupportBundleMutation();

  return (
    <Panel>
      <PanelHeader
        title={t('logs:supportBundle.heading')}
        icon={<ShieldCheck className="size-4" />}
      />
      <PanelBody className="space-y-3">
        <p className="text-xs leading-relaxed text-ink-muted">
          {t('logs:supportBundle.description')}
        </p>
        <Button
          variant="secondary"
          size="sm"
          disabled={bundleMutation.isPending}
          icon={
            bundleMutation.isPending ? (
              <Loader2 aria-hidden="true" className="size-3.5 animate-spin" />
            ) : (
              <Download aria-hidden="true" className="size-3.5" />
            )
          }
          onClick={() => bundleMutation.mutate()}
        >
          {bundleMutation.isPending
            ? t('logs:supportBundle.exporting')
            : t('logs:supportBundle.export')}
        </Button>
        {bundleMutation.isSuccess && (
          <p className="text-xs font-medium text-status-live">{t('logs:supportBundle.success')}</p>
        )}
        {bundleMutation.isError && (
          <p className="text-xs font-medium text-status-error">
            {t('logs:supportBundle.failure')} {resolveApiErrorMessage(tErrors, bundleMutation.error)}
          </p>
        )}
      </PanelBody>
    </Panel>
  );
}

/**
 * Logs page: the Stage 20E operator diagnostics surface backed by
 * `GET /api/logs`.
 *
 * Deliberately not a live-scrolling terminal (governing task's own
 * requirement): the operator reads a stable, filterable snapshot and
 * refreshes it explicitly. Every entry shown here already went
 * through the backend's own redaction before capture - the notice
 * below states that plainly rather than leaving it implicit.
 */
export function LogsPage() {
  const { t } = useTranslation(['logs', 'errors']);
  const tErrors = useTranslation('errors').t;
  const { locale } = useLanguage();

  const [severity, setSeverity] = useState('');
  const [subsystemInput, setSubsystemInput] = useState('');
  const [searchInput, setSearchInput] = useState('');

  const subsystem = useDebouncedValue(subsystemInput, 300);
  const search = useDebouncedValue(searchInput, 300);

  const filters: LogsFilter = { severity, subsystem, search };
  const hasActiveFilters = severity !== '' || subsystemInput !== '' || searchInput !== '';

  const logsQuery = useLogsQuery(filters);
  const entries = logsQuery.data?.pages.flatMap((page) => page.entries) ?? [];

  const clearFilters = () => {
    setSeverity('');
    setSubsystemInput('');
    setSearchInput('');
  };

  return (
    <AppShell title={t('logs:page.title')} description={t('logs:page.description')}>
      <div className="mx-auto max-w-4xl space-y-4">
        <Panel>
          <PanelHeader title={t('logs:page.title')} icon={<FileText className="size-4" />} />
          <PanelBody className="space-y-4">
            <p className="rounded-lg border border-line bg-surface-sunken px-3 py-2 text-[11px] leading-relaxed text-ink-faint">
              {t('logs:redactionNotice')}
            </p>

            <LogsFilterBar
              severity={severity}
              onSeverityChange={setSeverity}
              subsystemInput={subsystemInput}
              onSubsystemInputChange={setSubsystemInput}
              searchInput={searchInput}
              onSearchInputChange={setSearchInput}
              onClear={clearFilters}
              hasActiveFilters={hasActiveFilters}
              onRefresh={() => void logsQuery.refetch()}
              refreshing={logsQuery.isFetching && !logsQuery.isFetchingNextPage}
            />

            {logsQuery.isPending && (
              <div className="flex items-center justify-center gap-2 py-10 text-sm text-ink-muted">
                <Loader2 aria-hidden="true" className="size-4 animate-spin" />
                {t('logs:list.loading')}
              </div>
            )}

            {logsQuery.isError && (
              <div className="space-y-2 py-8 text-center">
                <p className="text-sm font-medium text-status-error">
                  {t('logs:list.backendUnavailable')}
                </p>
                <p className="mx-auto max-w-md text-xs leading-relaxed text-ink-muted">
                  {resolveApiErrorMessage(tErrors, logsQuery.error)}
                </p>
              </div>
            )}

            {logsQuery.isSuccess && entries.length === 0 && (
              <p className="py-8 text-center text-sm text-ink-muted">
                {hasActiveFilters ? t('logs:list.emptyFiltered') : t('logs:list.empty')}
              </p>
            )}

            {entries.length > 0 && (
              <div className="space-y-2">
                {entries.map((entry) => (
                  <LogEntryRow key={entry.seq} entry={entry} locale={locale} />
                ))}
              </div>
            )}

            {logsQuery.hasNextPage === true && (
              <div className="flex justify-center pt-1">
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={logsQuery.isFetchingNextPage}
                  onClick={() => void logsQuery.fetchNextPage()}
                >
                  {logsQuery.isFetchingNextPage
                    ? t('logs:list.loadingOlder')
                    : t('logs:list.loadOlder')}
                </Button>
              </div>
            )}
          </PanelBody>
        </Panel>

        <SupportBundlePanel />
      </div>
    </AppShell>
  );
}
