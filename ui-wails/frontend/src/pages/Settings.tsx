import React, { useState, useEffect } from 'react';
import {
  FolderIcon,
  CheckCircleIcon,
  XCircleIcon,
  Cog6ToothIcon,
  ArrowPathIcon,
  ArrowDownTrayIcon,
  InformationCircleIcon,
  RocketLaunchIcon,
} from '@heroicons/react/24/outline';
import {
  GetConfig,
  SaveConfig,
  SelectScreenshotDirectory,
  SelectLogsDirectory,
  GetDefaultScreenshotDir,
  GetDefaultLogsDir,
  ValidateQuestToken,
  TestConnection,
  CheckForUpdates,
  GetUpdateStatus,
  SetUpdateChannel,
  SetAutoUpdateCheck,
  OpenReleaseURL,
} from '../../wailsjs/go/main/App';
import { cn } from '../utils';
import type { Config, UpdateInfo, UpdateStatus } from '../types';
import { Button, Input, Badge, Card } from '../components/ui';
import { EventsOn } from '../../wailsjs/runtime/runtime';

function Section({ title, icon, children }: { title: string; icon: React.ReactNode; children: React.ReactNode }) {
  return (
    <div className="glass-card p-6 rounded-xl">
      <div className="flex items-center gap-3 mb-5">
        <div className="text-primary-purple">{icon}</div>
        <h2 className="text-lg font-semibold text-text-primary">{title}</h2>
      </div>
      <div className="space-y-5">{children}</div>
    </div>
  );
}

export function Settings() {
  const [config, setConfig] = useState<Config | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [saveMessage, setSaveMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [isValidatingToken, setIsValidatingToken] = useState(false);
  const [tokenValidation, setTokenValidation] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [isTestingConnection, setIsTestingConnection] = useState(false);
  const [connectionTest, setConnectionTest] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
  const [updateStatus, setUpdateStatus] = useState<UpdateStatus | null>(null);
  const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null);
  const [isCheckingUpdate, setIsCheckingUpdate] = useState(false);
  const [updateMessage, setUpdateMessage] = useState<{ type: 'success' | 'error' | 'info'; text: string } | null>(null);

  useEffect(() => {
    loadConfig();
    loadUpdateStatus();

    const cleanupFunctions = [
      EventsOn('update:available', (info: UpdateInfo) => {
        setUpdateInfo(info);
        setUpdateMessage({ type: 'info', text: `Update available: v${info.version}` });
        loadUpdateStatus();
      }),
      EventsOn('update:ready', () => {
        setUpdateMessage({ type: 'success', text: 'Update ready! Please restart the application.' });
        loadUpdateStatus();
      }),
      EventsOn('update:error', (error: string) => {
        setUpdateMessage({ type: 'error', text: error });
        loadUpdateStatus();
      }),
    ];

    return () => { cleanupFunctions.forEach(cleanup => cleanup()); };
  }, []);

  const loadConfig = async () => {
    try {
      const cfg = await GetConfig();
      setConfig(cfg as Config);
    } catch (err) {
      console.error('Failed to load config:', err);
    } finally {
      setIsLoading(false);
    }
  };

  const loadUpdateStatus = async () => {
    try {
      const status = await GetUpdateStatus();
      setUpdateStatus(status as UpdateStatus);
    } catch (err) {
      console.error('Failed to load update status:', err);
    }
  };

  const handleSave = async () => {
    if (!config) return;
    setIsSaving(true);
    setSaveMessage(null);
    try {
      await SaveConfig(config as any);
      setSaveMessage({ type: 'success', text: 'Settings saved!' });
      setTimeout(() => setSaveMessage(null), 3000);
    } catch (err) {
      setSaveMessage({ type: 'error', text: err instanceof Error ? err.message : 'Failed to save' });
    } finally {
      setIsSaving(false);
    }
  };

  const handleSelectScreenshotDir = async () => {
    try {
      const dir = await SelectScreenshotDirectory();
      if (dir && config) setConfig({ ...config, screenshotDir: dir });
    } catch (err) { console.error(err); }
  };

  const handleSelectLogsDir = async () => {
    try {
      const dir = await SelectLogsDirectory();
      if (dir && config) setConfig({ ...config, logsDir: dir });
    } catch (err) { console.error(err); }
  };

  const handleAutoDetectScreenshotDir = async () => {
    try {
      const dir = await GetDefaultScreenshotDir();
      if (dir && config) setConfig({ ...config, screenshotDir: dir });
    } catch (err) { console.error(err); }
  };

  const handleAutoDetectLogsDir = async () => {
    try {
      const dir = await GetDefaultLogsDir();
      if (dir && config) setConfig({ ...config, logsDir: dir });
    } catch (err) { console.error(err); }
  };

  const handleValidateToken = async () => {
    setIsValidatingToken(true);
    setTokenValidation(null);
    try {
      await ValidateQuestToken();
      setTokenValidation({ type: 'success', text: 'Token valid!' });
    } catch (err) {
      setTokenValidation({ type: 'error', text: err instanceof Error ? err.message : 'Validation failed' });
    } finally {
      setIsValidatingToken(false);
    }
  };

  const handleTestConnection = async () => {
    setIsTestingConnection(true);
    setConnectionTest(null);
    try {
      await TestConnection();
      setConnectionTest({ type: 'success', text: 'Connected!' });
    } catch (err) {
      setConnectionTest({ type: 'error', text: err instanceof Error ? err.message : 'Failed' });
    } finally {
      setIsTestingConnection(false);
    }
  };

  const handleCheckForUpdates = async () => {
    setIsCheckingUpdate(true);
    setUpdateMessage(null);
    try {
      const update = await CheckForUpdates();
      if (update && update.version) {
        setUpdateInfo(update as UpdateInfo);
        setUpdateMessage({ type: 'info', text: `Update available: v${update.version}` });
      } else {
        setUpdateMessage({ type: 'success', text: 'You are on the latest version!' });
        setTimeout(() => setUpdateMessage(null), 3000);
      }
      await loadUpdateStatus();
    } catch (err) {
      setUpdateMessage({ type: 'error', text: err instanceof Error ? err.message : 'Check failed' });
    } finally {
      setIsCheckingUpdate(false);
    }
  };

  const handleUpdateChannelChange = async (channel: 'stable' | 'beta') => {
    if (!config) return;
    try {
      await SetUpdateChannel(channel);
      setConfig({ ...config, updateSettings: { ...config.updateSettings, updateChannel: channel } });
    } catch (err) { console.error(err); }
  };

  const handleAutoCheckToggle = async (enabled: boolean) => {
    if (!config) return;
    try {
      await SetAutoUpdateCheck(enabled);
      setConfig({ ...config, updateSettings: { ...config.updateSettings, enableAutoCheck: enabled } });
    } catch (err) { console.error(err); }
  };

  const handleRerunWizard = async () => {
    if (!config) return;
    try {
      await SaveConfig({ ...config, setupComplete: false } as any);
      window.location.href = '/';
    } catch (err) { console.error(err); }
  };

  const formatFileSize = (bytes: number): string => {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i];
  };

  const formatDate = (dateString: string): string => {
    if (!dateString) return 'N/A';
    const date = new Date(dateString);
    return date.toLocaleDateString() + ' ' + date.toLocaleTimeString();
  };

  // Auto-save after config changes (debounced)
  const [initialLoad, setInitialLoad] = useState(true);
  useEffect(() => {
    if (!config || initialLoad) {
      if (config) setInitialLoad(false);
      return;
    }
    const timer = setTimeout(() => {
      handleSave();
    }, 1000);
    return () => clearTimeout(timer);
  }, [config]);

  if (isLoading || !config) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-text-secondary">Loading...</div>
      </div>
    );
  }

  return (
    <div className="min-h-full p-8 animate-fade-in-up">
      {/* Header */}
      <div className="mb-8 flex items-center justify-between">
        <div>
          <h1 className="text-4xl font-bold gradient-text mb-2">Settings</h1>
          <p className="text-text-secondary">Configure your Tarkov Nexus integration</p>
        </div>
        <div className="flex items-center gap-3">
          {saveMessage && (
            <Badge variant={saveMessage.type === 'success' ? 'success' : 'error'} glow pulse>
              {saveMessage.type === 'success' ? <CheckCircleIcon className="w-4 h-4 mr-1" /> : <XCircleIcon className="w-4 h-4 mr-1" />}
              {saveMessage.text}
            </Badge>
          )}
          <Button onClick={handleSave} disabled={isSaving} loading={isSaving} size="sm" className="bg-neon-green hover:bg-neon-green/90 hover:shadow-glow-green">
            {isSaving ? 'Saving...' : 'Save'}
          </Button>
          <Button onClick={handleRerunWizard} variant="ghost" icon={<RocketLaunchIcon className="w-4 h-4" />} size="sm">
            Setup Wizard
          </Button>
        </div>
      </div>

      <div className="space-y-6 max-w-4xl">

        {/* Connection */}
        <Section title="Connection" icon={<Cog6ToothIcon className="w-5 h-5" />}>
          <Input
            label="Remote ID"
            type="text"
            value={config.remoteId || ''}
            onChange={(e) => setConfig({ ...config, remoteId: e.target.value })}
            placeholder="Auto-generated on first boot"
            helperText="Automatically generated. Only change this if you need to match a specific session."
          />

          <label className="flex items-center cursor-pointer group">
            <input
              type="checkbox"
              checked={config.autoConnect}
              onChange={(e) => setConfig({ ...config, autoConnect: e.target.checked })}
              className="w-4 h-4 text-primary-purple bg-bg-card border-border-color rounded focus:ring-2 focus:ring-primary-purple/50"
            />
            <span className="ml-3 text-text-secondary group-hover:text-text-primary transition-colors">
              Automatically start integration on launch
            </span>
          </label>

          <div className="flex items-center gap-3">
            <Button onClick={handleTestConnection} disabled={isTestingConnection} loading={isTestingConnection} variant="secondary" size="sm">
              {isTestingConnection ? 'Testing...' : 'Test Connection'}
            </Button>
            {connectionTest && (
              <Badge variant={connectionTest.type === 'success' ? 'success' : 'error'} glow>
                {connectionTest.type === 'success' ? <CheckCircleIcon className="w-4 h-4 mr-1" /> : <XCircleIcon className="w-4 h-4 mr-1" />}
                {connectionTest.text}
              </Badge>
            )}
          </div>
        </Section>

        {/* Directories */}
        <Section title="Directories" icon={<FolderIcon className="w-5 h-5" />}>
          <div>
            <label className="block text-sm font-medium text-text-secondary mb-2">
              Screenshot Directory <span className="text-error ml-1">*</span>
            </label>
            <div className="flex gap-2">
              <input
                type="text"
                value={config.screenshotDir || ''}
                onChange={(e) => setConfig({ ...config, screenshotDir: e.target.value })}
                placeholder="Path to EFT screenshots folder"
                className="glass-input flex-1 px-4 py-2"
              />
              <Button onClick={handleSelectScreenshotDir} variant="ghost" icon={<FolderIcon className="w-4 h-4" />} size="sm">Browse</Button>
              <Button onClick={handleAutoDetectScreenshotDir} variant="secondary" size="sm">Auto-detect</Button>
            </div>
          </div>

          <div>
            <label className="block text-sm font-medium text-text-secondary mb-2">
              Logs Directory <span className="text-error ml-1">*</span>
            </label>
            <div className="flex gap-2">
              <input
                type="text"
                value={config.logsDir || ''}
                onChange={(e) => setConfig({ ...config, logsDir: e.target.value })}
                placeholder="Path to EFT logs folder"
                className="glass-input flex-1 px-4 py-2"
              />
              <Button onClick={handleSelectLogsDir} variant="ghost" icon={<FolderIcon className="w-4 h-4" />} size="sm">Browse</Button>
              <Button onClick={handleAutoDetectLogsDir} variant="secondary" size="sm">Auto-detect</Button>
            </div>
            <p className="text-xs text-text-muted mt-1">Required for automatic map detection and quest tracking.</p>
          </div>

          <label className="flex items-center cursor-pointer group">
            <input
              type="checkbox"
              checked={config.autoProcessExisting}
              onChange={(e) => setConfig({ ...config, autoProcessExisting: e.target.checked })}
              className="w-4 h-4 text-primary-purple bg-bg-card border-border-color rounded focus:ring-2 focus:ring-primary-purple/50"
            />
            <span className="ml-3 text-text-secondary group-hover:text-text-primary transition-colors">
              Process existing screenshots on start
            </span>
          </label>
        </Section>

        {/* Quest Tracking */}
        <Section title="Quest Tracking" icon={<CheckCircleIcon className="w-5 h-5" />}>
          <label className="flex items-center cursor-pointer group">
            <input
              type="checkbox"
              checked={config.enableQuestTracking}
              onChange={(e) => setConfig({ ...config, enableQuestTracking: e.target.checked })}
              className="w-4 h-4 text-primary-purple bg-bg-card border-border-color rounded focus:ring-2 focus:ring-primary-purple/50"
            />
            <span className="ml-3 text-text-secondary group-hover:text-text-primary transition-colors">
              Sync quest progress to TarkovTracker
            </span>
          </label>

          {config.enableQuestTracking && (
            <>
              <Input
                label="TarkovTracker URL"
                type="text"
                value={config.tarkovTrackerUrl || ''}
                onChange={(e) => setConfig({ ...config, tarkovTrackerUrl: e.target.value })}
                placeholder="https://tarkovtracker.org/api/v2"
              />
              <div className="space-y-3">
                <Input
                  label="API Token"
                  type="password"
                  value={config.tarkovTrackerToken || ''}
                  onChange={(e) => setConfig({ ...config, tarkovTrackerToken: e.target.value })}
                  placeholder="Your TarkovTracker API token"
                  helperText={<>Get your token from <a href="https://tarkovtracker.org/api" target="_blank" rel="noopener noreferrer" className="text-primary-purple hover:text-light-purple">TarkovTracker API</a></>}
                />
                <div className="flex items-center gap-3">
                  <Button onClick={handleValidateToken} disabled={!config.tarkovTrackerToken || isValidatingToken} loading={isValidatingToken} variant="secondary" size="sm">
                    Validate Token
                  </Button>
                  {tokenValidation && (
                    <Badge variant={tokenValidation.type === 'success' ? 'success' : 'error'} glow>
                      {tokenValidation.type === 'success' ? <CheckCircleIcon className="w-4 h-4 mr-1" /> : <XCircleIcon className="w-4 h-4 mr-1" />}
                      {tokenValidation.text}
                    </Badge>
                  )}
                </div>
              </div>
              <div>
                <label className="block text-sm font-medium text-text-secondary mb-2">Game Mode</label>
                <select
                  value={config.tarkovTrackerGameMode || 'pvp'}
                  onChange={(e) => setConfig({ ...config, tarkovTrackerGameMode: e.target.value })}
                  className="glass-input w-full px-4 py-2"
                >
                  <option value="pvp">PVP</option>
                  <option value="pve">PVE</option>
                </select>
              </div>
            </>
          )}
        </Section>

        {/* Advanced */}
        <Section title="Advanced" icon={<Cog6ToothIcon className="w-5 h-5" />}>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <Input
              label="Position Throttle (ms)"
              type="text"
              value={config.positionUpdateThrottleMs}
              onChange={(e) => setConfig({ ...config, positionUpdateThrottleMs: parseInt(e.target.value) || 1000 })}
            />
            <Input
              label="Screenshot Debounce (ms)"
              type="text"
              value={config.monitorOptions.debounceTimeMs}
              onChange={(e) => setConfig({ ...config, monitorOptions: { ...config.monitorOptions, debounceTimeMs: parseInt(e.target.value) || 500 } })}
            />
            <Input
              label="Reconnect Interval (ms)"
              type="text"
              value={config.reconnectOptions.reconnectIntervalMs}
              onChange={(e) => setConfig({ ...config, reconnectOptions: { ...config.reconnectOptions, reconnectIntervalMs: parseInt(e.target.value) || 5000 } })}
            />
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Input
              label="Max Reconnect Attempts"
              type="text"
              value={config.reconnectOptions.maxReconnectAttempts}
              onChange={(e) => setConfig({ ...config, reconnectOptions: { ...config.reconnectOptions, maxReconnectAttempts: parseInt(e.target.value) || 10 } })}
            />
          </div>

          <label className="flex items-center cursor-pointer group">
            <input
              type="checkbox"
              checked={config.debugLogging}
              onChange={(e) => setConfig({ ...config, debugLogging: e.target.checked })}
              className="w-4 h-4 text-primary-purple bg-bg-card border-border-color rounded focus:ring-2 focus:ring-primary-purple/50"
            />
            <span className="ml-3 text-text-secondary group-hover:text-text-primary transition-colors">
              Enable debug logging
            </span>
          </label>
        </Section>

        {/* Updates */}
        <Section title="Updates" icon={<ArrowDownTrayIcon className="w-5 h-5" />}>
          {updateMessage && (
            <Badge variant={updateMessage.type === 'success' ? 'success' : updateMessage.type === 'error' ? 'error' : 'default'} glow>
              {updateMessage.type === 'success' ? <CheckCircleIcon className="w-4 h-4 mr-1" /> : updateMessage.type === 'error' ? <XCircleIcon className="w-4 h-4 mr-1" /> : <InformationCircleIcon className="w-4 h-4 mr-1" />}
              {updateMessage.text}
            </Badge>
          )}

          <div className="flex items-center justify-between">
            <div>
              <span className="text-text-muted text-sm">Current Version: </span>
              <span className="text-text-primary font-mono">v{updateStatus?.currentVersion || '3.0.0'}</span>
            </div>
            <Button onClick={handleCheckForUpdates} disabled={isCheckingUpdate} loading={isCheckingUpdate} variant="secondary" icon={<ArrowPathIcon className="w-4 h-4" />} size="sm">
              Check for Updates
            </Button>
          </div>

          {updateInfo && updateStatus?.updateAvailable && (
            <div className="p-4 rounded-lg border border-primary-purple/30 bg-primary-purple/5">
              <div className="flex items-center justify-between mb-2">
                <span className="font-semibold text-text-primary">v{updateInfo.version} available</span>
                <Button onClick={() => OpenReleaseURL()} variant="ghost" size="sm">View Release</Button>
              </div>
              {updateStatus?.downloading && (
                <div className="w-full bg-bg-dark rounded-full h-2 overflow-hidden">
                  <div className="h-full bg-gradient-to-r from-primary-purple to-electric-purple transition-all" style={{ width: `${updateStatus.downloadProgress}%` }} />
                </div>
              )}
            </div>
          )}

          <div className="flex items-center gap-6">
            <label className="flex items-center cursor-pointer group">
              <input
                type="checkbox"
                checked={config.updateSettings.enableAutoCheck}
                onChange={(e) => handleAutoCheckToggle(e.target.checked)}
                className="w-4 h-4 text-primary-purple bg-bg-card border-border-color rounded focus:ring-2 focus:ring-primary-purple/50"
              />
              <span className="ml-3 text-text-secondary group-hover:text-text-primary transition-colors text-sm">
                Auto-check on startup
              </span>
            </label>

            <select
              value={config.updateSettings.updateChannel}
              onChange={(e) => handleUpdateChannelChange(e.target.value as 'stable' | 'beta')}
              className="glass-input px-3 py-1 text-sm"
            >
              <option value="stable">Stable</option>
              <option value="beta">Beta</option>
            </select>
          </div>
        </Section>
      </div>

    </div>
  );
}
