const REPO = 'ObsidianNetwork/Tarkov-Nexus';

export const RELEASES_API = `https://api.github.com/repos/${REPO}/releases?per_page=100`;

export interface ReleaseInfo {
  version: string;
  downloadUrl: string;
  totalDownloads: number;
}

const FALLBACK: ReleaseInfo = {
  version: 'v3.3.3',
  downloadUrl: `https://github.com/${REPO}/releases/latest`,
  totalDownloads: 0
};

/** Counts real binaries only — checksums and signatures are not downloads. */
export function countDownloads(releases: any[]): number {
  return releases.reduce(
    (total, release) =>
      total +
      (release.assets ?? []).reduce(
        (sum: number, asset: any) =>
          /\.(zip|exe|msi)$/i.test(asset.name) ? sum + (asset.download_count ?? 0) : sum,
        0
      ),
    0
  );
}

let cached: Promise<ReleaseInfo> | null = null;

export function getLatestRelease(): Promise<ReleaseInfo> {
  cached ??= fetchRelease();
  return cached;
}

async function fetchRelease(): Promise<ReleaseInfo> {
  try {
    const res = await fetch(RELEASES_API);
    const releases = await res.json();
    if (!Array.isArray(releases) || releases.length === 0) return FALLBACK;

    const latest = releases.find((r: any) => !r.draft && !r.prerelease) ?? releases[0];
    const windowsAsset = latest.assets?.find(
      (a: any) => a.name.includes('Windows') && a.name.endsWith('.zip')
    );

    return {
      version: latest.tag_name,
      downloadUrl:
        windowsAsset?.browser_download_url || `https://github.com/${REPO}/releases/latest`,
      totalDownloads: countDownloads(releases)
    };
  } catch {
    return FALLBACK;
  }
}
