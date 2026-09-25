import type { UpdateInfo } from '../../types/updater';
import type { ReleaseNotesState } from './types';
import { exactReleaseURL } from './links';
import { failedReleaseNotes } from './state';

export function offeredNotes(info: UpdateInfo): ReleaseNotesState {
  const releaseUrl = exactReleaseURL(info.version, info.releaseUrl);
  const releaseMs = info.releaseDate ? Date.parse(info.releaseDate) : NaN;
  const publishedAt = Number.isFinite(releaseMs) && releaseMs > Date.parse('0001-01-01T00:00:00Z') ? info.releaseDate : null;
  if (!releaseUrl || typeof info.releaseBody !== 'string' || new TextEncoder().encode(info.releaseBody).length > 1024 * 1024 || (info.releaseDate && !Number.isFinite(releaseMs))) {
    return failedReleaseNotes(info.version);
  }
  return {
    kind: 'content', version: info.version, releaseUrl, publishedAt,
    note: { tag: `v${info.version.replace(/^v/, '')}`, name: info.releaseName, body: info.releaseBody, releaseUrl, publishedAt },
    source: 'fresh', publication: 'published', refreshing: false, retryAt: null, retryable: false,
  };
}
