import { Lock } from 'lucide-react';
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';

import { useAuth } from '@/app/auth-context';
import { Button } from '@/components/ui/Button';
import { Panel } from '@/components/ui/Panel';

/**
 * Stage 20D2B remote-management login (docs/remote-management.md
 * §13/§26): a single administrator password, no username field, no
 * registration, no password-recovery link - there is exactly one
 * identity. Rendered by AuthGate only when the backend reports an
 * active, unauthenticated remote-management session state; never
 * shown for a plain desktop/local-headless backend.
 */
export function LoginPage() {
  const { t } = useTranslation('auth');
  const { login } = useAuth();
  const [password, setPassword] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [errorKey, setErrorKey] = useState<'invalidCredentials' | 'rateLimited' | 'network' | null>(null);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (submitting || password.length === 0) return;

    setSubmitting(true);
    setErrorKey(null);
    const result = await login(password);
    setSubmitting(false);

    if (!result.ok) {
      setPassword('');
      if (result.reason === 'invalid-credentials') setErrorKey('invalidCredentials');
      else if (result.reason === 'rate-limited') setErrorKey('rateLimited');
      else setErrorKey('network');
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-surface-base px-4">
      <Panel raised className="w-full max-w-sm p-6">
        <div className="mb-5 flex flex-col items-center text-center">
          <div className="mb-3 flex size-11 items-center justify-center rounded-full bg-accent/15 text-accent">
            <Lock aria-hidden="true" className="size-5" />
          </div>
          <h1 className="text-lg font-semibold text-ink">{t('title')}</h1>
          <p className="mt-1 text-sm text-ink-muted">{t('description')}</p>
        </div>

        <form onSubmit={handleSubmit} className="flex flex-col gap-3" noValidate>
          <div className="flex flex-col gap-1.5">
            <label htmlFor="admin-password" className="text-xs font-medium text-ink-muted">
              {t('passwordLabel')}
            </label>
            <input
              id="admin-password"
              name="password"
              type="password"
              autoComplete="current-password"
              autoFocus
              required
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              disabled={submitting}
              className="h-10 rounded-lg border border-line bg-surface px-3 text-sm text-ink outline-none focus:border-accent"
              aria-invalid={errorKey !== null}
              aria-describedby={errorKey !== null ? 'admin-password-error' : undefined}
            />
          </div>

          {errorKey !== null && (
            <p id="admin-password-error" role="alert" className="text-sm text-status-error">
              {t(`errors.${errorKey}`)}
            </p>
          )}

          <Button type="submit" variant="primary" disabled={submitting || password.length === 0} className="mt-1 w-full justify-center">
            {submitting ? t('submitting') : t('submit')}
          </Button>
        </form>
      </Panel>
    </div>
  );
}
