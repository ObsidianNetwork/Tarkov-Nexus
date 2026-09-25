import { useCallback, useEffect, useRef, useState } from 'react';
import { GetSavedReleaseNotes, GetVersion, RefreshReleaseNotes } from '../../../wailsjs/go/main/App';
import type { UpdateInfo } from '../../types/updater';
import type { ReleaseNotesState } from './types';
import { preloadReleaseNotes } from './ReleaseNotesContent';
import { offeredNotes } from './offeredNotes';
import { failedReleaseNotes, loadingReleaseNotes, releaseNotesState, retryDelay } from './state';

// A null selection resolves the installed version through the application binding.
export function useReleaseNotes(open: boolean, selection: string | null = null, offer?: UpdateInfo | null) {
  const seed = offer?.version === selection ? offeredNotes(offer) : null;
  const seedRef = useRef(seed);
  seedRef.current = seed;
  const [snapshot, setSnapshot] = useState(() => ({ selection, state: seed ?? loadingReleaseNotes(selection) }));
  const [attempt, setAttempt] = useState(0);
  const request = useRef(0);
  const pending = useRef(false);
  const current = useRef(snapshot);

  useEffect(() => {
    if (!open) return;
    preloadReleaseNotes();
    const token = ++request.current;
    const isCurrent = () => request.current === token;
    let version = selection;
    let freshFinished = false;
    pending.current = true;
    const publish = (state: ReleaseNotesState) => {
      if (!isCurrent()) return;
      current.current = { selection, state };
      setSnapshot(current.current);
    };
    const previous = current.current.selection === selection ? current.current.state : null;
    publish(attempt > 0 && previous?.kind === 'content'
      ? { ...previous, refreshing: true }
      : seedRef.current ?? loadingReleaseNotes(version));

    async function load() {
      try {
        version = version ?? (await GetVersion()).trim();
        if (!isCurrent()) return;
        const resolvedVersion = version;
        if (current.current.state.version !== version) publish(loadingReleaseNotes(version));
        // Start both reads together. A slow disk response cannot replace fresh content.
        void GetSavedReleaseNotes(version).then(saved => {
          if (!isCurrent()) return;
          const cached = releaseNotesState(saved, resolvedVersion, !freshFinished);
          const displayed = current.current.state;
          if (freshFinished) {
            if (displayed.kind === 'failed' && cached.kind === 'content') {
              publish({ ...failedReleaseNotes(resolvedVersion, cached), retryAt: displayed.retryAt });
            }
          } else if (displayed.kind !== 'content' || (cached.kind === 'content' && displayed.note.body === cached.note.body)) {
            publish(cached);
          } else if (displayed.kind === 'content' && displayed.source === 'fresh') {
            publish({ ...displayed, source: 'session', refreshing: true });
          }
        }).catch((error: unknown) => {
          console.error('Saved release notes could not be read', error instanceof Error ? error.message : String(error));
        });
        const result = await RefreshReleaseNotes(version);
        if (!isCurrent()) return;
        freshFinished = true;
        const next = releaseNotesState(result, version);
        publish(next.kind === 'failed' && current.current.state.kind === 'content'
          ? { ...failedReleaseNotes(version, current.current.state), retryAt: next.retryAt }
          : next);
      } catch (error: unknown) {
        freshFinished = true;
        if (isCurrent()) publish(failedReleaseNotes(version, current.current.state));
        console.error('Release notes could not be loaded', error instanceof Error ? error.message : String(error));
      } finally {
        if (isCurrent()) pending.current = false;
      }
    }

    void load();
    return () => { ++request.current; pending.current = false; };
  }, [open, selection, attempt]);

  const retry = useCallback(() => {
    const state = current.current.state;
    if (!open || pending.current || !state.retryable || retryDelay(state.retryAt) > 0) return;
    pending.current = true;
    setAttempt(value => value + 1);
  }, [open]);

  // Render the new identity immediately, before its effect starts.
  const state = snapshot.selection === selection ? snapshot.state : seed ?? loadingReleaseNotes(selection);
  return { state, retry };
}
