// Minimal semver comparison for update UX decisions (downgrade detection).
// Mirrors the Go side: three numeric parts, optional "-suffix" prerelease,
// prerelease sorts BEFORE the release (3.4.0-beta.1 < 3.4.0).

interface ParsedVersion {
  major: number;
  minor: number;
  patch: number;
  prerelease: string;
}

function parseSemver(input: string): ParsedVersion | null {
  const trimmed = input.trim().replace(/^v/, '');
  const [core, prerelease = ''] = trimmed.split(/[-+]/, 2);
  const parts = core.split('.');
  if (parts.length > 3) return null;
  const nums: number[] = [];
  for (let i = 0; i < 3; i++) {
    const n = Number(parts[i] ?? '0');
    if (!Number.isInteger(n) || n < 0) return null;
    nums.push(n);
  }
  return { major: nums[0], minor: nums[1], patch: nums[2], prerelease };
}

function compareNumeric(a: number, b: number): number {
  return a < b ? -1 : a > b ? 1 : 0;
}

/** Returns -1, 0 or 1 comparing a against b. Invalid input returns null. */
export function compareSemver(a: string, b: string): number | null {
  const pa = parseSemver(a);
  const pb = parseSemver(b);
  if (!pa || !pb) return null;
  const base =
    compareNumeric(pa.major, pb.major) ||
    compareNumeric(pa.minor, pb.minor) ||
    compareNumeric(pa.patch, pb.patch);
  if (base !== 0) return base;
  // Equal base versions: no prerelease is NEWER than any prerelease.
  if (pa.prerelease === pb.prerelease) return 0;
  if (pa.prerelease === '') return 1;
  if (pb.prerelease === '') return -1;
  return pa.prerelease < pb.prerelease ? -1 : pa.prerelease > pb.prerelease ? 1 : 0;
}

/** True when candidate is strictly older than current (a deliberate downgrade). */
export function isDowngrade(candidate: string, current: string): boolean {
  const cmp = compareSemver(candidate, current);
  return cmp !== null && cmp < 0;
}
