import { useState } from 'react';
import type { MouseEvent, ReactNode } from 'react';
import { OpenReleaseNotesLink } from '../../../wailsjs/go/main/App';
import { safeNoteURL } from './links';

type ReleaseNotesLinkProps = {
  readonly href: string;
  readonly children: ReactNode;
  readonly id?: string;
  readonly anchorName?: string;
  readonly title?: string;
  readonly ariaLabel?: string;
  readonly ariaDescribedBy?: string;
  readonly onFollow?: (href: string) => boolean;
};

export function ReleaseNotesLink({ href, children, id, anchorName, title, ariaLabel, ariaDescribedBy, onFollow }: ReleaseNotesLinkProps) {
  const [failed, setFailed] = useState(false);
  const destination = safeNoteURL(href);
  if (!destination) return <span id={id} data-note-anchor={anchorName}>{children}</span>;

  const open = async (event: MouseEvent<HTMLAnchorElement>) => {
    event.preventDefault();
    if (event.button !== 0 && event.button !== 1) return;
    if (onFollow?.(destination)) return;
    try {
      await OpenReleaseNotesLink(destination);
      setFailed(false);
    } catch (error: unknown) {
      setFailed(true);
      console.error('Release notes link could not be opened', error instanceof Error ? error.message : String(error));
    }
  };

  return (
    <>
      <a id={id} data-note-anchor={anchorName} title={title} aria-label={ariaLabel} aria-describedby={ariaDescribedBy} href={destination}
        onClick={open} onAuxClick={open} onContextMenu={event => event.preventDefault()}
        className="text-light-purple underline underline-offset-2 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-purple">
        {children}
      </a>
      {failed && <span role="status" className="ml-2 text-sm text-text-secondary">Couldn’t open link</span>}
    </>
  );
}
