import { useState } from 'react';
import { ReleaseNotesLink } from './ReleaseNotesLink';
import { canAutoLoadNoteImage } from './links';

type ReleaseNotesImageProps = {
  readonly src?: string;
  readonly alt?: string;
  readonly title?: string;
};

export function ReleaseNotesImage({ src, alt, title }: ReleaseNotesImageProps) {
  const [failedSrc, setFailedSrc] = useState<string | null>(null);
  const failed = failedSrc !== null && failedSrc === src;
  if (!src || failed || !canAutoLoadNoteImage(src)) {
    return <span className="my-4 block rounded-lg border border-border-color p-4 text-sm text-text-secondary">
      <span>{alt || 'Image unavailable'}</span>
      {src && <span className="ml-3"><ReleaseNotesLink href={src}>Open image</ReleaseNotesLink></span>}
    </span>;
  }
  return <img src={src} alt={alt ?? ''} title={title} loading="lazy" decoding="async" referrerPolicy="no-referrer" onError={() => setFailedSrc(src)} />;
}
