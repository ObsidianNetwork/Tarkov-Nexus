import { Dialog, Transition } from '@headlessui/react';
import { Fragment } from 'react';
import {
  ArrowDownTrayIcon,
  XMarkIcon,
  CheckCircleIcon,
  ExclamationTriangleIcon
} from '@heroicons/react/24/outline';
import { Button } from './ui';
import { isDowngrade } from '../utils/version';

import type { UpdateInfo } from '../types/updater';
import { useReleaseNotes } from '../features/release-notes/useReleaseNotes';
import { ReleaseNotesContent, ReleaseNotesMetadata } from '../features/release-notes/ReleaseNotesContent';
import '../features/release-notes/release-notes.css';

interface UpdateNotificationProps {
  isOpen: boolean;
  onClose: () => void;
  updateInfo: UpdateInfo | null;
  onUpdate: () => void;
  onRestart?: () => void;
  isDownloading?: boolean;
  isInstalling?: boolean;
  isUpdateReady?: boolean;
  downloadProgress?: number;
  error?: string;
  currentVersion?: string;
}

export function UpdateNotification({
  isOpen,
  onClose,
  updateInfo,
  onUpdate,
  onRestart,
  isDownloading = false,
  isInstalling = false,
  isUpdateReady = false,
  downloadProgress = 0,
  error = '',
  currentVersion,
}: UpdateNotificationProps) {
  const { state: notes, retry } = useReleaseNotes(isOpen && updateInfo !== null, updateInfo?.version ?? null, updateInfo);

  if (!updateInfo) return null;

  const downgrade =
    currentVersion !== undefined && isDowngrade(updateInfo.version, currentVersion);

  const formatFileSize = (bytes: number) => {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i];
  };

  return (
    <Transition appear show={isOpen} as={Fragment}>
      <Dialog as="div" className="relative z-50" onClose={onClose}>
        <Transition.Child
          as={Fragment}
          enter="ease-out duration-300"
          enterFrom="opacity-0"
          enterTo="opacity-100"
          leave="ease-in duration-200"
          leaveFrom="opacity-100"
          leaveTo="opacity-0"
        >
          <div className="fixed inset-0 bg-black/60 backdrop-blur-sm" />
        </Transition.Child>

        <div className="fixed inset-0">
          <div className="flex min-h-full items-center justify-center p-4">
            <Transition.Child
              as={Fragment}
              enter="ease-out duration-300"
              enterFrom="opacity-0 scale-95"
              enterTo="opacity-100 scale-100"
              leave="ease-in duration-200"
              leaveFrom="opacity-100 scale-100"
              leaveTo="opacity-0 scale-95"
            >
              <Dialog.Panel className="release-notes-panel transform rounded-2xl bg-bg-card border border-border-color shadow-2xl">
                {/* Header */}
                <div className="relative shrink-0 bg-gradient-to-r from-primary-purple/20 to-electric-purple/10 p-4 sm:p-6 border-b border-border-color">
                  <div className="flex items-start justify-between">
                    <div className="flex items-center gap-3">
                      <div className="flex-shrink-0 w-10 h-10 rounded-full bg-primary-purple/20 flex items-center justify-center">
                        <ArrowDownTrayIcon className="w-5 h-5 text-primary-purple" />
                      </div>
                      <div>
                        <Dialog.Title className="text-xl font-bold text-text-primary">
                          {downgrade ? 'Switch to Older Version' : 'Update Available'}
                        </Dialog.Title>
                        <p className="text-sm text-text-secondary">
                          Version {updateInfo.version}
                          {downgrade ? ' (downgrade from ' + currentVersion + ')' : ''}
                        </p>
                      </div>
                    </div>
                    <button
                      onClick={onClose}
                      aria-label="Close update dialog"
                      className="min-h-11 min-w-11 flex items-center justify-center text-text-secondary hover:text-text-primary transition-colors"
                      disabled={isDownloading || isInstalling}
                    >
                      <XMarkIcon className="w-5 h-5" />
                    </button>
                  </div>
                  <ReleaseNotesMetadata state={notes} />
                  {updateInfo.assetSize > 0 && <p className="mt-2 text-sm text-text-secondary">Download size: {formatFileSize(updateInfo.assetSize)}</p>}
                </div>

                <div data-release-notes-scroll className="min-h-0 overflow-y-auto overscroll-contain custom-scrollbar bg-bg-darker p-4 sm:p-6">
                  <ReleaseNotesContent state={notes} onRetry={retry} />
                </div>
                <div className="shrink-0 px-4 sm:px-6">
                  {/* Progress */}
                  {(isDownloading || isInstalling) && (
                    <div className="mb-4">
                      <div className="flex items-center justify-between mb-2">
                        <span className="text-sm text-text-secondary">
                          {isInstalling ? 'Installing...' : 'Downloading...'}
                        </span>
                        {!isInstalling && (
                          <span className="text-sm text-text-muted">
                            {downloadProgress}%
                          </span>
                        )}
                      </div>
                      <div className="w-full bg-bg-dark rounded-full h-2 overflow-hidden">
                        <div
                          className="h-full bg-gradient-to-r from-primary-purple to-electric-purple transition-all duration-300"
                          style={{
                            width: isInstalling ? '100%' : `${downloadProgress}%`,
                          }}
                        />
                      </div>
                    </div>
                  )}

                  {/* Error */}
                  {error && (
                    <div className="mb-4 flex items-start gap-2 p-3 bg-red-500/10 border border-red-500/30 rounded-lg">
                      <ExclamationTriangleIcon className="w-5 h-5 text-red-400 flex-shrink-0 mt-0.5" />
                      <p className="text-sm text-red-400">{error}</p>
                    </div>
                  )}

                  {/* Success Message */}
                  {isUpdateReady && !error && (
                    <div className="mb-4 flex items-start gap-2 p-3 bg-green-500/10 border border-green-500/30 rounded-lg">
                      <CheckCircleIcon className="w-5 h-5 text-green-400 flex-shrink-0 mt-0.5" />
                      <div className="text-sm text-green-400">
                        <p className="font-medium mb-1">Update Ready!</p>
                        <p className="text-green-300/80">
                          The update has been downloaded and installed. Click "Restart Now" to apply the update.
                        </p>
                      </div>
                    </div>
                  )}
                </div>

                {/* Actions */}
                <div className="shrink-0 p-4 sm:px-6 bg-bg-dark/50 border-t border-border-color flex gap-3">
                  {!isUpdateReady ? (
                    <>
                      <Button
                        variant="secondary"
                        onClick={onClose}
                        className="flex-1 min-h-11 text-text-primary"
                        disabled={isDownloading || isInstalling}
                      >
                        Skip
                      </Button>
                      <Button
                        variant="primary"
                        onClick={onUpdate}
                        className="flex-1 min-h-11 text-text-primary"
                        disabled={isDownloading || isInstalling}
                        loading={isDownloading || isInstalling}
                      >
                        {isDownloading
                          ? 'Downloading...'
                          : isInstalling
                            ? 'Installing...'
                            : downgrade
                              ? 'Install (downgrade)'
                              : 'Update Now'}
                      </Button>
                    </>
                  ) : (
                    <>
                      <Button
                        variant="secondary"
                        onClick={onClose}
                        className="flex-1 min-h-11 text-text-primary"
                      >
                        Restart Later
                      </Button>
                      {onRestart && (
                        <Button
                          variant="primary"
                          onClick={onRestart}
                          className="flex-1 min-h-11 bg-neon-green hover:bg-neon-green/90"
                        >
                          Restart Now
                        </Button>
                      )}
                    </>
                  )}
                </div>
              </Dialog.Panel>
            </Transition.Child>
          </div>
        </div>
      </Dialog>
    </Transition>
  );
}
