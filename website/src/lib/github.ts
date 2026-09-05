const REPO = 'ObsidianNetwork/Tarkov-Nexus';

/** Public, CORS-enabled, no auth — the download counter calls this from the browser. */
export const RELEASES_API = `https://api.github.com/repos/${REPO}/releases?per_page=100`;

export interface ReleaseInfo {
  version: string;
  downloadUrl: string;
}

interface GitHubRelease {
  tag_name: string;
  prerelease: boolean;
  draft: boolean;
  published_at: string;
  html_url: string;
  assets: { name: string; browser_download_url: string }[];
}

export async function getLatestBetaRelease(): Promise<ReleaseInfo | null> {
  const res = await fetch(RELEASES_API);
  if (!res.ok) throw new Error(`Failed to fetch beta releases: HTTP ${res.status}`);
  const releases: GitHubRelease[] = await res.json();
  const beta = releases
    .filter(release =>
      !release.draft && release.prerelease &&
      /-beta(?:\.|$)/.test(release.tag_name.split('+')[0])
    )
    .sort((a, b) => Date.parse(b.published_at) - Date.parse(a.published_at))[0];
  if (!beta) return null;

  const windowsAsset = beta.assets.find(
    asset => asset.name.includes('Windows') && asset.name.endsWith('.zip')
  );
  return {
    version: beta.tag_name,
    downloadUrl: windowsAsset?.browser_download_url ?? beta.html_url
  };
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
