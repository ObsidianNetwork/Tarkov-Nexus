import React, { createContext, useContext, useState, useEffect, ReactNode } from 'react';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { GetStatus, IsRunning, GetUpdateStatus, DownloadUpdate, RestartApplication } from '../../wailsjs/go/main/App';
import type { Status } from '../types';
import { UpdateNotification } from '../components/UpdateNotification';

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

interface UpdateStatus {
  checking: boolean;
  downloading: boolean;
  installing: boolean;
  updateAvailable: boolean;
  currentVersion: string;
  latestVersion: string;
  downloadProgress: number;
  error: string;
  lastChecked: string;
}

interface AppContextType {
  status: Status | null;
  isLoading: boolean;
  error: string | null;
  refreshStatus: () => Promise<void>;
  updateInfo: UpdateInfo | null;
  updateStatus: UpdateStatus | null;
  refreshUpdateStatus: () => Promise<void>;
}

const AppContext = createContext<AppContextType | undefined>(undefined);

export function AppProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<Status | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null);
  const [updateStatus, setUpdateStatus] = useState<UpdateStatus | null>(null);
  const [showUpdateModal, setShowUpdateModal] = useState(false);
  const [isUpdateReady, setIsUpdateReady] = useState(false);

  const refreshStatus = async () => {
    try {
      const newStatus = await GetStatus();
      setStatus(newStatus as Status);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch status');
      console.error('Failed to fetch status:', err);
    }
  };

  const refreshUpdateStatus = async () => {
    try {
      const status = await GetUpdateStatus();
      setUpdateStatus(status as UpdateStatus);
    } catch (err) {
      console.error('Failed to fetch update status:', err);
    }
  };

  const handleUpdate = async () => {
    if (!updateInfo) return;
    try {
      await DownloadUpdate(updateInfo.version);
    } catch (err) {
      console.error('Failed to download update:', err);
    }
  };

  const handleRestart = async () => {
    try {
      await RestartApplication();
      // Application will restart, so no further action needed
    } catch (err) {
      console.error('Failed to restart application:', err);
    }
  };

  useEffect(() => {
    // Initial load
    refreshStatus().finally(() => setIsLoading(false));

    // Poll status every second when running
    const interval = setInterval(async () => {
      try {
        const running = await IsRunning();
        if (running) {
          await refreshStatus();
        }
      } catch (err) {
        console.error('Status poll error:', err);
      }
    }, 1000);

    // Initial update status
    refreshUpdateStatus();

    // Listen for integration events
    const cleanupFunctions = [
      EventsOn('integration:started', () => refreshStatus()),
      EventsOn('integration:stopped', () => refreshStatus()),
      EventsOn('integration:connected', () => refreshStatus()),
      EventsOn('integration:disconnected', () => refreshStatus()),
      EventsOn('integration:mapDetected', () => refreshStatus()),
      EventsOn('integration:screenshotProcessed', () => refreshStatus()),
      EventsOn('integration:questStatusChanged', () => refreshStatus()),

      // Update events
      EventsOn('update:available', (info: UpdateInfo) => {
        setUpdateInfo(info);
        setShowUpdateModal(true);
        setIsUpdateReady(false);
        refreshUpdateStatus();
      }),
      EventsOn('update:downloading', () => refreshUpdateStatus()),
      EventsOn('update:installing', () => refreshUpdateStatus()),
      EventsOn('update:ready', () => {
        setIsUpdateReady(true);
        refreshUpdateStatus();
      }),
    ];

    return () => {
      clearInterval(interval);
      // Cleanup event listeners (if EventsOff is available)
      cleanupFunctions.forEach(cleanup => {
        if (typeof cleanup === 'function') cleanup();
      });
    };
  }, []);

  return (
    <AppContext.Provider value={{
      status,
      isLoading,
      error,
      refreshStatus,
      updateInfo,
      updateStatus,
      refreshUpdateStatus,
    }}>
      {children}

      {/* Update Notification Modal */}
      <UpdateNotification
        isOpen={showUpdateModal}
        onClose={() => setShowUpdateModal(false)}
        updateInfo={updateInfo}
        onUpdate={handleUpdate}
        onRestart={handleRestart}
        isDownloading={updateStatus?.downloading || false}
        isInstalling={updateStatus?.installing || false}
        isUpdateReady={isUpdateReady}
        downloadProgress={updateStatus?.downloadProgress || 0}
        error={updateStatus?.error || ''}
      />
    </AppContext.Provider>
  );
}

export function useApp() {
  const context = useContext(AppContext);
  if (context === undefined) {
    throw new Error('useApp must be used within an AppProvider');
  }
  return context;
}
