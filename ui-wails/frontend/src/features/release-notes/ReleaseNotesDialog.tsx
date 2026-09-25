import { useEffect, useRef, useState } from 'react';
import { Dialog, DialogPanel, DialogTitle, Description } from '@headlessui/react';
import { XMarkIcon } from '@heroicons/react/24/outline';
import { Button } from '../../components/ui';
import { useApp } from '../../context/AppContext';
import { ReleaseNotesContent, ReleaseNotesMetadata } from './ReleaseNotesContent';
import { useReleaseNotes } from './useReleaseNotes';
import { offeredReleaseVersion } from './selection';
import './release-notes.css';

type ReleaseNotesDialogProps = {
  readonly open: boolean;
  readonly onClose: () => void;
};

export function ReleaseNotesDialog({ open, onClose }: ReleaseNotesDialogProps) {
  return open ? <ReleaseNotesReader onClose={onClose} /> : null;
}

function ReleaseNotesReader({ onClose }: Pick<ReleaseNotesDialogProps, 'onClose'>) {
  const closeButton = useRef<HTMLButtonElement>(null);
  const { updateStatus, updateInfo, refreshUpdateStatus } = useApp();
  const [selection, setSelection] = useState<string | null>(null);
  const [installed, setInstalled] = useState<string | null>(null);
  const offered = offeredReleaseVersion(updateStatus, installed);
  const { state, retry } = useReleaseNotes(true, selection, updateInfo?.version === selection ? updateInfo : null);
  useEffect(() => {
    if (selection === null && state.version !== null) setInstalled(state.version);
  }, [selection, state.version]);
  useEffect(() => {
    void refreshUpdateStatus();
    // The updater has no withdrawal event. Read its local status while the reader is open.
    const interval = setInterval(() => { void refreshUpdateStatus(); }, 1000);
    return () => clearInterval(interval);
  }, [refreshUpdateStatus]);
  const noLongerOffered = selection !== null && selection !== offered;
  const version = state.version === null ? 'Loading version…' : state.version;

  return (
    <Dialog open={true} onClose={onClose} initialFocus={closeButton} className="relative z-50">
      <div className="fixed inset-0 bg-black/60" aria-hidden="true" />
      <div className="fixed inset-0 flex items-center justify-center p-4 sm:p-6">
        <DialogPanel className="release-notes-panel rounded-2xl border border-border-color bg-bg-card shadow-2xl">
          <header className="shrink-0 border-b border-border-color bg-gradient-to-r from-primary-purple/20 to-electric-purple/10 p-4 sm:p-6">
            <div className="flex items-start justify-between gap-4">
              <div className="min-w-0">
                <DialogTitle className="text-xl font-bold text-text-primary">Release notes</DialogTitle>
                <Description className="mt-2 break-words text-sm text-text-secondary">{selection === null ? 'Installed version' : noLongerOffered ? 'Selected version' : 'Offered update'} · {version}</Description>
              </div>
              <Button variant="ghost" className="min-h-11 min-w-11 shrink-0 p-2" onClick={onClose} aria-label="Close release notes">
                <XMarkIcon className="h-5 w-5" />
              </Button>
            </div>
            {(offered || selection !== null) && (
              <div className="mt-4 flex flex-wrap gap-2" role="group" aria-label="Release note version">
                <Button variant={selection === null ? 'primary' : 'secondary'} className="min-h-11 max-w-full break-all text-text-primary" aria-pressed={selection === null} onClick={() => setSelection(null)}>
                  Installed{installed ? ` · ${installed}` : ''}
                </Button>
                {offered && (
                  <Button variant={selection === offered ? 'primary' : 'secondary'} className="min-h-11 max-w-full break-all text-text-primary" aria-pressed={selection === offered} onClick={() => setSelection(offered)}>
                    Offered update · {offered}
                  </Button>
                )}
              </div>
            )}
            {noLongerOffered && <p className="mt-3 text-sm text-text-secondary">This version is no longer the offered update.</p>}
            <ReleaseNotesMetadata state={state} />
          </header>
          <div data-release-notes-scroll className="min-h-0 overflow-y-auto overscroll-contain custom-scrollbar bg-bg-darker p-4 sm:p-6" aria-busy={state.kind === 'loading'}>
            <ReleaseNotesContent state={state} onRetry={retry} />
          </div>
          <footer className="flex shrink-0 justify-end border-t border-border-color p-4 sm:px-6">
            <Button ref={closeButton} variant="secondary" className="min-h-11 text-text-primary" onClick={onClose}>Close</Button>
          </footer>
        </DialogPanel>
      </div>
    </Dialog>
  );
}
