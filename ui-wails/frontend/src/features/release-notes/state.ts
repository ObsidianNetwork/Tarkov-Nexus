import type { updater } from '../../../wailsjs/go/models';
import type { ReleaseNotesState } from './types';

export const allReleasesURL = 'https://github.com/ObsidianNetwork/Tarkov-Nexus/releases';

export function loadingReleaseNotes(version: string | null): ReleaseNotesState {
  return { kind: 'loading', version, releaseUrl: version ? `${allReleasesURL}/tag/${encodeURIComponent(`v${version.replace(/^v/, '')}`)}` : '', publishedAt: null, retryAt: null, refreshing: true, retryable: false };
}

export function failedReleaseNotes(version: string | null, previous?: ReleaseNotesState): ReleaseNotesState {
  if (previous?.kind === 'content' && previous.version === version) {
    return { ...previous, source: previous.source === 'session' ? 'session' : 'saved', publication: 'failed', refreshing: false, retryable: true, retryAt: null };
  }
  return { ...loadingReleaseNotes(version), kind: 'failed', refreshing: false, retryable: true };
}

export function releaseNotesState(result: updater.ReleaseNotesResult, version: string, refreshing = false): ReleaseNotesState {
  if (result.version !== version) return failedReleaseNotes(version);
  const retryAt = typeof result.retryAt === 'string' && Number.isFinite(Date.parse(result.retryAt)) ? result.retryAt : null;
  const common = { version, releaseUrl: result.releaseUrl, publishedAt: null, retryAt, refreshing, retryable: result.lookup === 'failed' };
  let publication: 'published' | 'unchecked' | 'unavailable' | 'failed';
  switch (result.lookup) {
    case 'published': publication = 'published'; break;
    case 'unchecked': publication = 'unchecked'; break;
    case 'missing': publication = 'unavailable'; break;
    case 'failed': publication = 'failed'; break;
    default: return failedReleaseNotes(version);
  }
  if (result.note) {
    const publishedAtMs = typeof result.note.publishedAt === 'string' ? Date.parse(result.note.publishedAt) : NaN;
    const publishedAt = Number.isFinite(publishedAtMs) && publishedAtMs > Date.parse('0001-01-01T00:00:00Z') ? result.note.publishedAt : null;
    const note = { ...result.note, publishedAt };
    return { ...common, kind: 'content', note, publishedAt: note.publishedAt, publication, source: !result.saved ? 'session' : refreshing || publication !== 'published' ? 'saved' : 'fresh' };
  }
  if (publication === 'unavailable') return { ...common, kind: 'missing' };
  if (publication === 'unchecked') return { ...common, kind: 'loading' };
  return { ...common, kind: 'failed', retryable: true };
}

export function retryDelay(retryAt: string | null, now = Date.now()): number {
  const deadline = retryAt ? Date.parse(retryAt) : NaN;
  return Number.isFinite(deadline) ? Math.max(0, deadline - now) : 0;
}
