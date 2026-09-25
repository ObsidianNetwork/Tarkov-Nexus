export interface UpdateInfo {
  readonly version: string;
  readonly releaseUrl: string;
  readonly releaseDate: string;
  readonly releaseName: string;
  readonly releaseBody: string;
  readonly assetUrl: string;
  readonly assetName: string;
  readonly assetSize: number;
  readonly isPrerelease: boolean;
}

export interface UpdateStatus {
  readonly checking: boolean;
  readonly downloading: boolean;
  readonly installing: boolean;
  readonly updateAvailable: boolean;
  readonly currentVersion: string;
  readonly latestVersion: string;
  readonly downloadProgress: number;
  readonly error: string;
  readonly lastChecked: string;
}

export function parseUpdateStatus(value: unknown): UpdateStatus | null {
  if (typeof value !== 'object' || value === null
    || !('currentVersion' in value) || typeof value.currentVersion !== 'string'
    || !('updateAvailable' in value) || typeof value.updateAvailable !== 'boolean'
    || !('checking' in value) || typeof value.checking !== 'boolean'
    || !('downloading' in value) || typeof value.downloading !== 'boolean'
    || !('installing' in value) || typeof value.installing !== 'boolean') return null;
  return {
    currentVersion: value.currentVersion, updateAvailable: value.updateAvailable,
    checking: value.checking, downloading: value.downloading, installing: value.installing,
    latestVersion: 'latestVersion' in value && typeof value.latestVersion === 'string' ? value.latestVersion : '',
    downloadProgress: 'downloadProgress' in value && typeof value.downloadProgress === 'number' ? value.downloadProgress : 0,
    error: 'error' in value && typeof value.error === 'string' ? value.error : '',
    lastChecked: 'lastChecked' in value && typeof value.lastChecked === 'string' ? value.lastChecked : '',
  };
}
