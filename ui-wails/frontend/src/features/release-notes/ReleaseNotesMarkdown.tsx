import { memo, useId, useRef } from 'react';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeRaw from 'rehype-raw';
import rehypeSanitize from 'rehype-sanitize';
import { limitNoteImages, noteHeadings, noteSchema } from './markdown';
import { resolveNoteURL } from './links';
import { ReleaseNotesLink } from './ReleaseNotesLink';
import { ReleaseNotesImage } from './ReleaseNotesImage';
import type { ReleaseNote } from './types';

export const ReleaseNotesMarkdown = memo(function ReleaseNotesMarkdown({ note }: { readonly note: ReleaseNote }) {
  const instance = useId();
  const prefix = `release-note-${instance.replace(/:/g, '')}-`;
  const root = useRef<HTMLDivElement>(null);

  function followFragment(href: string): boolean {
    const destination = new URL(href);
    const release = new URL(note.releaseUrl);
    if (!destination.hash || destination.origin !== release.origin || destination.pathname !== release.pathname || destination.search !== release.search) return false;
    let fragment: string;
    try {
      fragment = decodeURIComponent(destination.hash.slice(1));
    } catch (error: unknown) {
      if (error instanceof URIError) return false;
      throw error;
    }
    const target = Array.from(root.current?.querySelectorAll<HTMLElement>('[id], [data-note-anchor]') ?? [])
      .find(element => element.id === prefix + fragment || element.getAttribute('data-note-anchor') === prefix + fragment);
    const scroll = root.current?.closest<HTMLElement>('[data-release-notes-scroll]');
    if (!target || !scroll) return false;
    scroll.scrollTop += target.getBoundingClientRect().top - scroll.getBoundingClientRect().top - parseFloat(getComputedStyle(scroll).paddingTop);
    target.tabIndex = -1;
    target.focus({ preventScroll: true });
    return true;
  }

  return (
    <div ref={root} className="release-notes-markdown">
      <Markdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeRaw, noteHeadings, limitNoteImages, [rehypeSanitize, { ...noteSchema, clobberPrefix: prefix }]]}
        urlTransform={(url, key) => resolveNoteURL(url, note, key === 'src') ?? ''}
        components={{
          a: ({ node, href, children, id, title, 'aria-label': ariaLabel, 'aria-describedby': ariaDescribedBy }) => <ReleaseNotesLink
            href={href ?? ''} id={id} anchorName={typeof node?.properties.name === 'string' ? node.properties.name : undefined} title={title} ariaLabel={ariaLabel} ariaDescribedBy={ariaDescribedBy}
            onFollow={followFragment}>{children}</ReleaseNotesLink>,
          img: ({ src, alt, title }) => <ReleaseNotesImage src={src} alt={alt} title={title} />,
          input: ({ checked }) => <input type="checkbox" checked={Boolean(checked)} disabled aria-label={checked ? 'Completed task' : 'Incomplete task'} />,
          table: ({ children }) => <div className="release-notes-table" tabIndex={0} role="region" aria-label="Release notes table"><table>{children}</table></div>,
          pre: ({ children }) => <pre tabIndex={0} aria-label="Release notes code">{children}</pre>,
        }}>
        {note.body}
      </Markdown>
    </div>
  );
});
