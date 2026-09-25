export function safeNoteURL(raw: string, image = false): string | null {
  if (!/^https?:\/\//i.test(raw) || /[\u0000-\u0020\u007f-\u009f\\]/u.test(raw)) return null;
  try {
    const url = new URL(raw);
    if (!url.hostname || url.username || url.password) return null;
    if (url.protocol !== 'https:' && (image || url.protocol !== 'http:')) return null;
    return url.href;
  } catch (error: unknown) {
    if (error instanceof TypeError) return null;
    throw error;
  }
}

export function canAutoLoadNoteImage(raw: string): boolean {
  const safe = safeNoteURL(raw, true);
  if (!safe) return false;
  const host = new URL(safe).hostname.toLowerCase();
  return host === 'github.com' || host.endsWith('.githubusercontent.com');
}

export function exactReleaseURL(version: string, raw: string): string | null {
  const safe = safeNoteURL(raw);
  if (!safe) return null;
  const url = new URL(safe);
  const tag = `v${version.replace(/^v/, '')}`;
  if (url.origin !== 'https://github.com' || url.search || url.hash) return null;
  try {
    if (decodeURIComponent(url.pathname) !== `/ObsidianNetwork/Tarkov-Nexus/releases/tag/${tag}`) return null;
  } catch (error: unknown) {
    if (error instanceof URIError) return null;
    throw error;
  }
  return safe;
}

export function resolveNoteURL(raw: string, note: { readonly tag: string; readonly releaseUrl: string }, image = false): string | null {
  if (!raw || /[\u0000-\u0020\u007f-\u009f\\]/u.test(raw)) return null;
  if (raw.startsWith('//')) return safeNoteURL(`https:${raw}`, image);
  if (/^[a-z][a-z0-9+.-]*:/i.test(raw)) return safeNoteURL(raw, image);
  try {
    if (raw.startsWith('#')) return image ? null : safeNoteURL(new URL(raw, note.releaseUrl).href);
    if (raw.startsWith('/')) return safeNoteURL(new URL(raw, 'https://github.com/').href, image);
    // Normalize dot segments at a synthetic root before adding the immutable tag path.
    const relative = new URL(raw, 'https://release.invalid/');
    const base = image
      ? 'https://raw.githubusercontent.com/ObsidianNetwork/Tarkov-Nexus/'
      : 'https://github.com/ObsidianNetwork/Tarkov-Nexus/blob/';
    return safeNoteURL(`${base}${encodeURIComponent(note.tag)}${relative.pathname}${relative.search}${relative.hash}`, image);
  } catch (error: unknown) {
    if (error instanceof TypeError) return null;
    throw error;
  }
}
