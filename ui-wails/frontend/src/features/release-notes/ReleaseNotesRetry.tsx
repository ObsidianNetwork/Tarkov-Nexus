import { useEffect, useState } from 'react';
import { Button } from '../../components/ui';
import type { ReleaseNotesState } from './types';
import { retryDelay } from './state';

type ReleaseNotesRetryProps = {
  readonly state: ReleaseNotesState;
  readonly onRetry: () => void;
};

export function ReleaseNotesRetry({ state, onRetry }: ReleaseNotesRetryProps) {
  const [now, setNow] = useState(Date.now);
  const delay = retryDelay(state.retryAt, now);
  useEffect(() => {
    const remaining = retryDelay(state.retryAt);
    if (!remaining) {
      if (retryDelay(state.retryAt, now) > 0) setNow(Date.now());
      return;
    }
    const timer = window.setTimeout(() => setNow(Date.now()), Math.min(remaining, 2147483647));
    return () => window.clearTimeout(timer);
  }, [state.retryAt, now]);

  if (!state.retryable) return null;
  return (
    <div className="mt-4 flex flex-wrap items-center gap-3">
      <Button variant="secondary" className="min-h-11 text-text-primary" disabled={state.refreshing || delay > 0} onClick={onRetry}>
        {state.refreshing ? 'Retrying…' : 'Retry'}
      </Button>
      {delay > 0 && state.retryAt && <p role="status" className="text-sm text-text-secondary">Retry available {new Date(state.retryAt).toLocaleString()}.</p>}
    </div>
  );
}
