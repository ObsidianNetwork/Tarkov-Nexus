const REPO = 'Jordanmuss99/Tarkov-Nexus';

export interface ReleaseInfo {
  version: string;
  downloadUrl: string;
}

export async function getLatestRelease(): Promise<ReleaseInfo> {
  try {
    const res = await fetch(`https://api.github.com/repos/${REPO}/releases/latest`);
    const release = await res.json();

    const version = release.tag_name;
    const windowsAsset = release.assets?.find((a: any) =>
      a.name.includes('Windows') && a.name.endsWith('.zip')
    );

    return {
      version,
      downloadUrl: windowsAsset?.browser_download_url ||
        `https://github.com/${REPO}/releases/latest`
    };
  } catch {
    // Fallback if API fails
    return {
      version: 'v3.2.2',
      downloadUrl: `https://github.com/${REPO}/releases/latest`
    };
  }
}
