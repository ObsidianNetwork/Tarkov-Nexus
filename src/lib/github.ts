const REPO = 'ObsidianNetwork/Tarkov-Nexus';

/** Public, CORS-enabled, no auth — the download counter calls this from the browser. */
export const RELEASES_API = `https://api.github.com/repos/${REPO}/releases?per_page=100`;

export interface ReleaseInfo {
  version: string;
  downloadUrl: string;
}

const FALLBACK: ReleaseInfo = {
  version: 'v3.3.3',
  downloadUrl: `https://github.com/${REPO}/releases/latest`
};

let cached: Promise<ReleaseInfo> | null = null;

export function getLatestRelease(): Promise<ReleaseInfo> {
  cached ??= fetchRelease();
  return cached;
}

async function fetchRelease(): Promise<ReleaseInfo> {
  try {
    const res = await fetch(`https://api.github.com/repos/${REPO}/releases/latest`);
    const release = await res.json();
    if (!release?.tag_name) return FALLBACK;

    const windowsAsset = release.assets?.find(
      (a: any) => a.name.includes('Windows') && a.name.endsWith('.zip')
    );

    return {
      version: release.tag_name,
      downloadUrl:
        windowsAsset?.browser_download_url || `https://github.com/${REPO}/releases/latest`
    };
  } catch {
    return FALLBACK;
  }
}
