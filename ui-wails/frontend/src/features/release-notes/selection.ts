import type { UpdateStatus } from '../../types/updater';

// Preserve exact prerelease/build identifiers; only the optional tag prefix is removed.
export function offeredReleaseVersion(status: UpdateStatus | null, installed: string | null): string | null {
  if (!status?.updateAvailable) return null;
  const version = status.latestVersion.trim().replace(/^v/, '');
  const match = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/.exec(version);
  if (!match || match[4]?.split('.').some(part => /^0\d+$/.test(part))) return null;
  return version === installed?.trim().replace(/^v/, '') ? null : version;
}
