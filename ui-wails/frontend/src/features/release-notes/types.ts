import type { updater } from '../../../wailsjs/go/models';

export type ReleaseNote = Readonly<Pick<updater.ReleaseNote, 'tag' | 'name' | 'body' | 'releaseUrl'>> & {
  readonly publishedAt: string | null;
};

export type ReleaseNotesState = {
  readonly releaseUrl: string;
  readonly publishedAt: string | null;
  readonly retryAt: string | null;
  readonly refreshing: boolean;
  readonly retryable: boolean;
} & (
  | { readonly kind: 'loading'; readonly version: string | null }
  | {
      readonly kind: 'content';
      readonly version: string;
      readonly note: ReleaseNote;
      readonly source: 'fresh' | 'saved' | 'session';
      readonly publication: 'unchecked' | 'published' | 'unavailable' | 'failed';
    }
  | { readonly kind: 'missing'; readonly version: string }
  | { readonly kind: 'failed'; readonly version: string | null });
