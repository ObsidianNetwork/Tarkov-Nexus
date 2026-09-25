import { Component, lazy, Suspense } from 'react';
import type { ReactNode } from 'react';
import { ReleaseNotesLink } from './ReleaseNotesLink';
import { Button } from '../../components/ui';
import type { ReleaseNotesState } from './types';
import { ReleaseNotesRetry } from './ReleaseNotesRetry';

const loadMarkdown = () => import('./ReleaseNotesMarkdown');
const NoteMarkdown = lazy(() => loadMarkdown().then(module => ({ default: module.ReleaseNotesMarkdown })));

export function preloadReleaseNotes() {
  void loadMarkdown().catch((error: unknown) => {
    console.error('Release notes renderer could not be loaded', error instanceof Error ? error.message : String(error));
  });
}

class NotesBoundary extends Component<{ readonly children: ReactNode; readonly body: string }, { readonly failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() { return { failed: true }; }
  componentDidUpdate(previous: { readonly body: string }) {
    if (this.state.failed && previous.body !== this.props.body) this.setState({ failed: false });
  }
  render() {
    return this.state.failed
      ? <div role="alert" className="text-text-secondary">
        <p>Couldn’t display release notes. Read this release on GitHub or reload Tarkov Nexus to try the formatter again.</p>
        <Button variant="secondary" className="mt-4 min-h-11 text-text-primary" onClick={() => window.location.reload()}>Reload Tarkov Nexus</Button>
      </div>
      : this.props.children;
  }
}

export function ReleaseNotesMetadata({ state }: { readonly state: ReleaseNotesState }) {
  const missing = state.kind === 'missing' || (state.kind === 'content' && state.publication === 'unavailable');
  return (
    <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-text-secondary">
      {state.publishedAt && <time dateTime={state.publishedAt}>{new Date(state.publishedAt).toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' })}</time>}
      {state.releaseUrl && <ReleaseNotesLink href={state.releaseUrl}>{missing ? 'View all GitHub releases' : 'View release on GitHub'}</ReleaseNotesLink>}
    </div>
  );
}

export function ReleaseNotesContent({ state, onRetry }: { readonly state: ReleaseNotesState; readonly onRetry: () => void }) {
  switch (state.kind) {
    case 'loading':
      return <p role="status" className="text-text-secondary">Loading release notes…</p>;
    case 'failed':
      return <div><p role="alert" className="text-text-secondary">Couldn’t load release notes</p><ReleaseNotesRetry state={state} onRetry={onRetry} /></div>;
    case 'missing':
      return <p role="status" className="text-text-secondary">No published release was found for version {state.version}.</p>;
    case 'content':
      const storageIndicator = state.source === 'session'
        ? 'Available this session; couldn’t save for offline use.'
        : state.source === 'saved'
          ? state.refreshing ? 'Saved copy · Refreshing…' : state.publication === 'failed' ? 'Saved copy · Refresh unavailable.' : null
          : null;
      return (
        <div>
          {state.publication === 'unavailable' && <p role="status" className="mb-4 text-sm text-text-secondary">Published release unavailable.</p>}
          {storageIndicator && <p role="status" className="mb-4 text-sm text-text-secondary">{storageIndicator}</p>}
          {!state.note.body.trim() ? <p className="text-text-secondary">No release notes were provided for this version.</p> : <NotesBoundary key={state.version} body={state.note.body}>
            <Suspense fallback={<p role="status" className="text-text-secondary">Formatting release notes…</p>}>
              <NoteMarkdown note={state.note} />
            </Suspense>
          </NotesBoundary>}
          <ReleaseNotesRetry state={state} onRetry={onRetry} />
        </div>
      );
    default: {
      const exhaustive: never = state;
      return exhaustive;
    }
  }
}
