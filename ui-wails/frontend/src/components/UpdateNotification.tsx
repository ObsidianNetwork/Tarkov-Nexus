import React, { useState } from 'react';
import { Dialog, Transition } from '@headlessui/react';
import { Fragment } from 'react';
import {
  ArrowDownTrayIcon,
  XMarkIcon,
  CheckCircleIcon,
  ExclamationTriangleIcon
} from '@heroicons/react/24/outline';
import { Button } from './ui';

interface UpdateInfo {
  version: string;
  releaseUrl: string;
  releaseDate: string;
  releaseName: string;
  releaseBody: string;
  assetUrl: string;
  assetName: string;
  assetSize: number;
  isPrerelease: boolean;
}

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
}: UpdateNotificationProps) {
  const [showReleaseNotes, setShowReleaseNotes] = useState(false);

  if (!updateInfo) return null;

  const formatFileSize = (bytes: number) => {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i];
  };

  const formatDate = (dateStr: string) => {
    try {
      return new Date(dateStr).toLocaleDateString('en-US', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
      });
    } catch {
      return dateStr;
    }
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

        <div className="fixed inset-0 overflow-y-auto">
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
              <Dialog.Panel className="w-full max-w-md transform overflow-hidden rounded-2xl bg-bg-card border border-border-color shadow-2xl transition-all">
                {/* Header */}
                <div className="relative bg-gradient-to-r from-primary-purple/20 to-electric-purple/10 px-6 py-4 border-b border-border-color">
                  <div className="flex items-start justify-between">
                    <div className="flex items-center gap-3">
                      <div className="flex-shrink-0 w-10 h-10 rounded-full bg-primary-purple/20 flex items-center justify-center">
                        <ArrowDownTrayIcon className="w-5 h-5 text-primary-purple" />
                      </div>
                      <div>
                        <Dialog.Title className="text-lg font-semibold text-text-primary">
                          Update Available
                        </Dialog.Title>
                        <p className="text-sm text-text-muted">
                          Version {updateInfo.version}
                        </p>
                      </div>
                    </div>
                    <button
                      onClick={onClose}
                      className="text-text-muted hover:text-text-primary transition-colors"
                      disabled={isDownloading || isInstalling}
                    >
                      <XMarkIcon className="w-5 h-5" />
                    </button>
                  </div>
                </div>

                {/* Content */}
                <div className="px-6 py-4">
                  {/* Release Info */}
                  <div className="space-y-3 mb-4">
                    {updateInfo.releaseName && (
                      <div>
                        <h3 className="text-sm font-medium text-text-secondary mb-1">
                          Release Name
                        </h3>
                        <p className="text-sm text-text-primary">
                          {updateInfo.releaseName}
                        </p>
                      </div>
                    )}

                    {updateInfo.releaseDate && (
                      <div>
                        <h3 className="text-sm font-medium text-text-secondary mb-1">
                          Release Date
                        </h3>
                        <p className="text-sm text-text-primary">
                          {formatDate(updateInfo.releaseDate)}
                        </p>
                      </div>
                    )}

                    {updateInfo.assetSize > 0 && (
                      <div>
                        <h3 className="text-sm font-medium text-text-secondary mb-1">
                          Download Size
                        </h3>
                        <p className="text-sm text-text-primary">
                          {formatFileSize(updateInfo.assetSize)}
                        </p>
                      </div>
                    )}
                  </div>

                  {/* Release Notes */}
                  {updateInfo.releaseBody && (
                    <div className="mb-4">
                      <button
                        onClick={() => setShowReleaseNotes(!showReleaseNotes)}
                        className="text-sm font-medium text-primary-purple hover:text-electric-purple transition-colors mb-2"
                      >
                        {showReleaseNotes ? 'Hide' : 'Show'} Release Notes
                      </button>
                      {showReleaseNotes && (
                        <div className="bg-bg-dark rounded-lg p-3 max-h-48 overflow-y-auto custom-scrollbar">
                          <pre className="text-xs text-text-secondary whitespace-pre-wrap font-sans">
                            {updateInfo.releaseBody}
                          </pre>
                        </div>
                      )}
                    </div>
                  )}

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
                <div className="px-6 py-4 bg-bg-dark/50 border-t border-border-color flex gap-3">
                  {!isUpdateReady ? (
                    <>
                      <Button
                        variant="secondary"
                        onClick={onClose}
                        className="flex-1"
                        disabled={isDownloading || isInstalling}
                      >
                        Skip
                      </Button>
                      <Button
                        variant="primary"
                        onClick={onUpdate}
                        className="flex-1"
                        disabled={isDownloading || isInstalling}
                        loading={isDownloading || isInstalling}
                      >
                        {isDownloading ? 'Downloading...' : isInstalling ? 'Installing...' : 'Update Now'}
                      </Button>
                    </>
                  ) : (
                    <>
                      <Button
                        variant="secondary"
                        onClick={onClose}
                        className="flex-1"
                      >
                        Restart Later
                      </Button>
                      {onRestart && (
                        <Button
                          variant="primary"
                          onClick={onRestart}
                          className="flex-1 bg-neon-green hover:bg-neon-green/90"
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
