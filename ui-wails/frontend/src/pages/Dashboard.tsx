import React, { useState, useEffect } from 'react';
import {
  PlayIcon,
  StopIcon,
  ArrowPathIcon,
  GlobeAltIcon,
  MapIcon,
  ClockIcon,
  PhotoIcon,
  CheckCircleIcon,
  XCircleIcon,
  UserIcon,
  UserGroupIcon,
  ExclamationTriangleIcon,
} from '@heroicons/react/24/outline';
import { useApp } from '../context/AppContext';
import {
  StartIntegration,
  StopIntegration,
  OpenTarkovDevBrowser,
  OpenMapWindow,
  GetAvailableMaps,
  SetManualMapOverride,
  ClearManualMapOverride,
  GetQuestProgress,
  GetPartyStatus,
  GetConfig,
} from '../../wailsjs/go/main/App';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { cn, formatTimestamp, formatDuration } from '../utils';
import type { MapInfo, TarkovTrackerProgress, QuestStats, PlayerInfo, PartyStatus } from '../types';
import { Button, Card, CardHeader, CardContent, Badge, StatusIndicator } from '../components/ui';

export function Dashboard() {
  const { status, refreshStatus } = useApp();
  const [isStarting, setIsStarting] = useState(false);
  const [isStopping, setIsStopping] = useState(false);
  const [availableMaps, setAvailableMaps] = useState<MapInfo[]>([]);
  const [showMapSelector, setShowMapSelector] = useState(false);
  const [selectedMap, setSelectedMap] = useState('');
  const [mapError, setMapError] = useState<string | null>(null);
  const [trackerProgress, setTrackerProgress] = useState<TarkovTrackerProgress | null>(null);
  const [isLoadingTracker, setIsLoadingTracker] = useState(false);
  const [trackerError, setTrackerError] = useState<string | null>(null);
  const [partyStatus, setPartyStatus] = useState<PartyStatus | null>(null);
  const [remoteId, setRemoteId] = useState<string>('');

  // Helper: Get game edition name from number
  const getEditionName = (edition: number): string => {
    const editions: Record<number, string> = {
      1: 'Standard',
      2: 'Left Behind',
      3: 'Prepare for Escape',
      4: 'Edge of Darkness',
      5: 'The Unheard',
    };
    return editions[edition] || 'Unknown';
  };

  // Helper: Calculate quest statistics
  const calculateQuestStats = (progress: TarkovTrackerProgress | null): QuestStats => {
    if (!progress || !progress.data.tasksProgress) {
      return { completed: 0, inProgress: 0, failed: 0, total: 0 };
    }

    const validTasks = progress.data.tasksProgress.filter((t) => !t.invalid);
    return {
      completed: validTasks.filter((t) => t.complete && !t.failed).length,
      inProgress: validTasks.filter((t) => !t.complete && !t.failed).length,
      failed: validTasks.filter((t) => t.failed).length,
      total: validTasks.length,
    };
  };

  // Helper: Get player info
  const getPlayerInfo = (progress: TarkovTrackerProgress | null): PlayerInfo | null => {
    if (!progress) return null;

    return {
      displayName: progress.data.displayName || 'Unknown',
      level: progress.data.playerLevel || 0,
      faction: progress.data.pmcFaction || 'Unknown',
      gameEdition: getEditionName(progress.data.gameEdition),
    };
  };

  // Load TarkovTracker progress
  const loadTrackerProgress = async () => {
    setIsLoadingTracker(true);
    setTrackerError(null);
    try {
      const result = await GetQuestProgress();
      setTrackerProgress(result as unknown as TarkovTrackerProgress);
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to load progress';
      setTrackerError(errorMessage);
      console.error('Failed to load tracker progress:', err);
    } finally {
      setIsLoadingTracker(false);
    }
  };

  // Load tracker progress on mount if quest tracking is enabled
  useEffect(() => {
    if (status?.questTracking?.enabled && status?.questTracking?.configured) {
      loadTrackerProgress();
    }
  }, [status?.questTracking?.enabled, status?.questTracking?.configured]);

  // Load party status periodically
  useEffect(() => {
    const loadPartyStatus = async () => {
      try {
        const status = await GetPartyStatus();
        setPartyStatus(status as PartyStatus);
      } catch (err) {
        console.error('Failed to load party status:', err);
      }
    };

    loadPartyStatus();
    const interval = setInterval(loadPartyStatus, 2000);
    return () => clearInterval(interval);
  }, []);

  // Load remote ID from config, and update live when captured from map
  useEffect(() => {
    GetConfig().then((cfg) => {
      if (cfg?.remoteId && cfg.remoteId !== 'your_remote_id_here') {
        setRemoteId(cfg.remoteId);
      }
    }).catch(() => {});

    const unsub = EventsOn('config:remoteIdUpdated', (id: string) => {
      if (id) setRemoteId(id);
    });
    return () => { unsub(); };
  }, []);

  // Calculate derived data
  const questStats = calculateQuestStats(trackerProgress);
  const playerInfo = getPlayerInfo(trackerProgress);

  // Load available maps when selector is opened
  const handleOpenMapSelector = async () => {
    try {
      const maps = await GetAvailableMaps();
      setAvailableMaps(maps as unknown as MapInfo[]);
      setShowMapSelector(true);
    } catch (err) {
      console.error('Failed to load maps:', err);
    }
  };

  const handleSetMap = async (mapId: string) => {
    setMapError(null);
    try {
      await SetManualMapOverride(mapId);
      setSelectedMap(mapId);
      await refreshStatus();
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to set map';
      setMapError(errorMessage);
      console.error('Failed to set map:', err);
    }
  };

  const handleClearMap = async () => {
    setMapError(null);
    try {
      await ClearManualMapOverride();
      setSelectedMap('');
      await refreshStatus();
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to clear map';
      setMapError(errorMessage);
      console.error('Failed to clear map:', err);
    }
  };

  const handleStart = async () => {
    setIsStarting(true);
    try {
      await StartIntegration();
      await refreshStatus();
    } catch (err) {
      console.error('Failed to start:', err);
      alert(err instanceof Error ? err.message : 'Failed to start integration');
    } finally {
      setIsStarting(false);
    }
  };

  const handleStop = async () => {
    setIsStopping(true);
    try {
      await StopIntegration();
      await refreshStatus();
    } catch (err) {
      console.error('Failed to stop:', err);
    } finally {
      setIsStopping(false);
    }
  };

  const handleOpenBrowser = async () => {
    try {
      await OpenTarkovDevBrowser();
    } catch (err) {
      console.error('Failed to open browser:', err);
    }
  };

  const handleOpenMapWindow = async () => {
    try {
      await OpenMapWindow();
    } catch (err) {
      console.error('Failed to open map window:', err);
    }
  };

  const isRunning = status?.isRunning || false;
  const isConnected = status?.connected || false;
  const currentMap = status?.currentMap || status?.currentNormalizedMap || status?.manualMapOverride || 'None';

  return (
    <div className="min-h-full p-8 animate-fade-in-up">
      {/* Header row: title + status + remote ID */}
      <div className="mb-8 flex items-center justify-between flex-wrap gap-4">
        <div>
          <h1 className="text-4xl font-bold gradient-text mb-1">Dashboard</h1>
          <p className="text-text-secondary text-sm">Monitor your Tarkov Map Sync integration</p>
        </div>
        <div className="flex items-center gap-3">
          <div className="glass-card inline-flex items-center px-4 py-2.5">
            <StatusIndicator
              status={isConnected ? 'success' : isRunning ? 'warning' : 'offline'}
              label={isConnected ? 'Connected' : isRunning ? 'Connecting...' : 'Stopped'}
              size="md"
              pulse={isConnected}
              glow={isConnected}
            />
          </div>
          {remoteId && (
            <div className="glass-card inline-flex items-center gap-2 px-4 py-2.5">
              <span className="text-xs text-text-muted uppercase tracking-wider">ID</span>
              <span className="text-sm font-bold text-neon-green tracking-widest font-mono">{remoteId}</span>
            </div>
          )}
        </div>
      </div>

      {/* Hero Actions — the two primary buttons */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-8">
        {!isRunning ? (
          <button
            onClick={handleStart}
            disabled={isStarting}
            className={cn(
              'group relative overflow-hidden rounded-xl p-6 text-left transition-all duration-300',
              'bg-gradient-to-br from-neon-green/20 to-neon-green/5 border border-neon-green/30',
              'hover:border-neon-green/60 hover:shadow-[0_0_30px_rgba(0,212,170,0.15)]',
              isStarting && 'opacity-70 cursor-wait'
            )}
          >
            <div className="flex items-center gap-4">
              <div className="w-14 h-14 rounded-xl bg-neon-green/20 flex items-center justify-center group-hover:bg-neon-green/30 transition-colors">
                <PlayIcon className="w-7 h-7 text-neon-green" />
              </div>
              <div>
                <div className="text-xl font-bold text-text-primary">{isStarting ? 'Starting...' : 'Start Integration'}</div>
                <div className="text-sm text-text-muted">Connect to tarkov.dev and begin position tracking</div>
              </div>
            </div>
          </button>
        ) : (
          <button
            onClick={handleStop}
            disabled={isStopping}
            className={cn(
              'group relative overflow-hidden rounded-xl p-6 text-left transition-all duration-300',
              'bg-gradient-to-br from-error/20 to-error/5 border border-error/30',
              'hover:border-error/60 hover:shadow-[0_0_30px_rgba(239,68,68,0.15)]',
              isStopping && 'opacity-70 cursor-wait'
            )}
          >
            <div className="flex items-center gap-4">
              <div className="w-14 h-14 rounded-xl bg-error/20 flex items-center justify-center group-hover:bg-error/30 transition-colors">
                <StopIcon className="w-7 h-7 text-error" />
              </div>
              <div>
                <div className="text-xl font-bold text-text-primary">{isStopping ? 'Stopping...' : 'Stop Integration'}</div>
                <div className="text-sm text-text-muted">Disconnect and stop position tracking</div>
              </div>
            </div>
          </button>
        )}

        <button
          onClick={handleOpenMapWindow}
          className="group relative overflow-hidden rounded-xl p-6 text-left transition-all duration-300 bg-gradient-to-br from-primary-purple/20 to-primary-purple/5 border border-primary-purple/30 hover:border-primary-purple/60 hover:shadow-[0_0_30px_rgba(139,92,246,0.15)]"
        >
          <div className="flex items-center gap-4">
            <div className="w-14 h-14 rounded-xl bg-primary-purple/20 flex items-center justify-center group-hover:bg-primary-purple/30 transition-colors">
              <MapIcon className="w-7 h-7 text-primary-purple" />
            </div>
            <div>
              <div className="text-xl font-bold text-text-primary">Open Map</div>
              <div className="text-sm text-text-muted">Interactive tarkov.dev map with party markers</div>
            </div>
          </div>
        </button>
      </div>

      {/* Secondary actions row */}
      <div className="mb-8 flex items-center gap-3 flex-wrap">
        <Button onClick={handleOpenBrowser} variant="ghost" icon={<GlobeAltIcon className="w-4 h-4" />} size="sm">
          Open tarkov.dev in Browser
        </Button>
        {status?.manualMapOverride && (
          <Button onClick={handleClearMap} variant="ghost" icon={<ArrowPathIcon className="w-4 h-4" />} size="sm" className="text-primary-purple">
            Clear Manual Map ({status.manualMapOverride})
          </Button>
        )}
      </div>

      {/* Quick Stats Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8 animate-fade-in-up" style={{ animationDelay: '0.1s' }}>
        <StatCard icon={MapIcon} label="Current Map" value={currentMap} />
        <StatCard
          icon={ClockIcon}
          label="Connection Uptime"
          value={status?.websocket?.connectionUptime ? formatDuration(status.websocket.connectionUptime) : '—'}
        />
        <StatCard
          icon={PhotoIcon}
          label="Screenshots Processed"
          value={status?.screenshot?.processedCount?.toString() || '0'}
        />
        <StatCard
          icon={CheckCircleIcon}
          label="Last Update"
          value={status?.lastUpdate ? formatTimestamp(status.lastUpdate) : 'Never'}
        />
      </div>

      {/* Party Status Card */}
      {partyStatus?.enabled && partyStatus?.active && (
        <Card className="mb-8 border-primary-purple/30 bg-primary-purple/5 animate-fade-in-up" style={{ animationDelay: '0.15s' }}>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 rounded-full bg-primary-purple/20 flex items-center justify-center">
                <UserGroupIcon className="w-6 h-6 text-primary-purple" />
              </div>
              <div>
                <h3 className="text-lg font-semibold text-text-primary flex items-center gap-2">
                  Party Active
                  {partyStatus.isHost && (
                    <Badge className="bg-neon-green/20 text-neon-green border-neon-green/30">
                      Host
                    </Badge>
                  )}
                </h3>
                <div className="flex items-center gap-4 mt-1">
                  <p className="text-text-secondary text-sm">
                    Code: <span className="font-mono font-semibold text-primary-purple">{partyStatus.partyCode}</span>
                  </p>
                  <p className="text-text-secondary text-sm">
                    {partyStatus.memberCount} {partyStatus.memberCount === 1 ? 'member' : 'members'}
                  </p>
                </div>
              </div>
            </div>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => window.location.href = '#/party'}
              className="text-primary-purple hover:bg-primary-purple/10"
            >
              Manage Party
            </Button>
          </div>
        </Card>
      )}

      {/* Manual Map Selector */}
      {isRunning && !status?.currentNormalizedMap && (
        <Card className="mb-8 border-warning/30 bg-warning/5 hover-glow animate-fade-in-up" style={{ animationDelay: '0.2s' }}>
          <div className="flex items-start">
            <MapIcon className="w-6 h-6 text-warning mr-3 flex-shrink-0 mt-1" />
            <div className="flex-1">
              <h3 className="text-lg font-semibold text-warning mb-2">Manual Map Selection</h3>
              <p className="text-text-secondary mb-2">
                Please wait for automatic detection, or select a map manually below. (Playing offline/locally requires manual map selection.)
              </p>
              {!isConnected && (
                <div className="flex items-center mb-4 text-sm text-error">
                  <XCircleIcon className="w-4 h-4 mr-2 flex-shrink-0" />
                  <span>Warning: WebSocket is not connected. Map changes will not be sent to tarkov.dev until connected.</span>
                </div>
              )}
              {!showMapSelector ? (
                <Button
                  onClick={handleOpenMapSelector}
                  variant="primary"
                  size="md"
                  className="bg-warning hover:bg-warning/90"
                >
                  Select Map
                </Button>
              ) : (
                <div className="space-y-3">
                  <select
                    value={selectedMap}
                    onChange={(e) => handleSetMap(e.target.value)}
                    className="glass-input w-full px-4 py-2 text-text-primary focus:ring-2 focus:ring-warning/50"
                  >
                    <option value="">Select a map...</option>
                    {availableMaps.map((map) => (
                      <option key={map.id} value={map.id}>
                        {map.displayName}
                      </option>
                    ))}
                  </select>
                  {selectedMap && (
                    <Button
                      onClick={handleClearMap}
                      variant="ghost"
                      size="md"
                    >
                      Clear Selection
                    </Button>
                  )}
                  {mapError && (
                    <div className="flex items-center text-sm text-error">
                      <XCircleIcon className="w-4 h-4 mr-2 flex-shrink-0" />
                      <span>{mapError}</span>
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>
        </Card>
      )}

      {/* Connection Info */}
      {isRunning && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6 animate-fade-in-up" style={{ animationDelay: '0.3s' }}>
          <InfoPanel
            title="WebSocket Status"
            items={[
              { label: 'Connected', value: isConnected ? 'Yes' : 'No', status: isConnected ? 'success' : 'error' },
              { label: 'Total Reconnects', value: status?.websocket?.totalReconnects?.toString() || '0' },
              {
                label: 'Last Ping',
                value: status?.websocket?.lastPingTime ? formatTimestamp(status.websocket.lastPingTime) : 'Never',
              },
            ]}
          />

          <InfoPanel
            title="Screenshot Monitoring"
            items={[
              {
                label: 'Monitoring',
                value: status?.screenshot?.isMonitoring ? 'Active' : 'Inactive',
                status: status?.screenshot?.isMonitoring ? 'success' : 'error',
              },
              { label: 'Processed', value: status?.screenshot?.processedCount?.toString() || '0' },
              { label: 'Pending', value: status?.screenshot?.pendingCount?.toString() || '0' },
            ]}
          />
        </div>
      )}

      {/* TarkovTracker Player Stats */}
      {status?.questTracking?.enabled && (
        <div className="mt-8 animate-fade-in-up" style={{ animationDelay: '0.4s' }}>
          {/* Header with refresh button */}
          <div className="flex items-center justify-between mb-6">
            <div className="flex items-center">
              <UserIcon className="w-6 h-6 text-primary-purple mr-2" />
              <h2 className="text-2xl font-bold gradient-text">Player Stats</h2>
            </div>
            {status?.questTracking?.configured && (
              <Button
                onClick={loadTrackerProgress}
                disabled={isLoadingTracker}
                loading={isLoadingTracker}
                variant="secondary"
                icon={<ArrowPathIcon className="w-5 h-5" />}
              >
                {isLoadingTracker ? 'Refreshing...' : 'Refresh'}
              </Button>
            )}
          </div>

          {/* Not configured message */}
          {!status?.questTracking?.configured && (
            <Card className="border-warning/30 bg-warning/5">
              <div className="flex items-start">
                <ExclamationTriangleIcon className="w-6 h-6 text-warning mr-3 flex-shrink-0 mt-1" />
                <div>
                  <h3 className="text-lg font-semibold text-warning mb-2">TarkovTracker Not Configured</h3>
                  <p className="text-text-secondary mb-2">
                    Please configure your TarkovTracker API token in the Settings page to view player stats and quest
                    progress.
                  </p>
                  <p className="text-text-muted text-sm">
                    Visit{' '}
                    <a
                      href="https://tarkovtracker.org"
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-primary-purple hover:text-light-purple transition-colors duration-300"
                    >
                      tarkovtracker.org
                    </a>{' '}
                    to generate an API token.
                  </p>
                </div>
              </div>
            </Card>
          )}

          {/* Loading state */}
          {isLoadingTracker && !trackerProgress && (
            <Card>
              <div className="flex items-center justify-center py-4">
                <ArrowPathIcon className="w-6 h-6 text-primary-purple animate-spin mr-3" />
                <span className="text-text-secondary">Loading player stats...</span>
              </div>
            </Card>
          )}

          {/* Error state */}
          {trackerError && !trackerProgress && (
            <Card className="border-error/30 bg-error/5">
              <div className="flex items-start">
                <XCircleIcon className="w-6 h-6 text-error mr-3 flex-shrink-0 mt-1" />
                <div>
                  <h3 className="text-lg font-semibold text-error mb-2">Failed to Load Stats</h3>
                  <p className="text-text-secondary">{trackerError}</p>
                </div>
              </div>
            </Card>
          )}

          {/* Stats panels */}
          {playerInfo && (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              <InfoPanel
                title="Player Info"
                items={[
                  { label: 'Name', value: playerInfo.displayName },
                  { label: 'Level', value: playerInfo.level.toString() },
                  { label: 'Faction', value: playerInfo.faction },
                  { label: 'Edition', value: playerInfo.gameEdition },
                ]}
              />

              <InfoPanel
                title="Quest Progress"
                items={[
                  {
                    label: 'Completed',
                    value: questStats.completed.toString(),
                    status: 'success',
                  },
                  {
                    label: 'Failed',
                    value: questStats.failed.toString(),
                    status: questStats.failed > 0 ? 'error' : undefined,
                  },
                  {
                    label: 'Total Tracked',
                    value: questStats.total.toString(),
                  },
                ]}
              />
            </div>
          )}
        </div>
      )}

      {/* Configuration Warning */}
      {!status?.isConfigured && (
        <Card className="mt-8 border-error/30 bg-error/5 animate-fade-in-up" style={{ animationDelay: '0.5s' }}>
          <div className="flex items-start">
            <XCircleIcon className="w-6 h-6 text-error mr-3 flex-shrink-0 mt-1" />
            <div>
              <h3 className="text-lg font-semibold text-error mb-2">Configuration Required</h3>
              <p className="text-text-secondary">
                Please configure your Remote ID and screenshot directory in the Settings page before starting the
                integration.
              </p>
            </div>
          </div>
        </Card>
      )}
    </div>
  );
}

function StatCard({ icon: Icon, label, value }: { icon: React.ElementType; label: string; value: string }) {
  return (
    <Card hoverable glowOnHover className="hover-lift">
      <div className="flex items-center mb-3">
        <Icon className="w-5 h-5 text-primary-purple mr-2" />
        <span className="text-sm text-text-muted uppercase tracking-wide">{label}</span>
      </div>
      <div className="text-2xl font-bold text-text-primary truncate">{value}</div>
    </Card>
  );
}

function InfoPanel({
  title,
  items,
}: {
  title: string;
  items: Array<{ label: string; value: string; status?: 'success' | 'error' }>;
}) {
  return (
    <Card hoverable className="hover-lift">
      <CardHeader>
        <h3 className="text-lg font-semibold text-text-primary">{title}</h3>
      </CardHeader>
      <CardContent>
        <div className="space-y-3">
          {items.map((item, index) => (
            <div key={index} className="flex justify-between items-center">
              <span className="text-text-muted">{item.label}</span>
              <span
                className={cn(
                  'font-medium',
                  item.status === 'success'
                    ? 'text-neon-green'
                    : item.status === 'error'
                    ? 'text-error'
                    : 'text-text-primary'
                )}
              >
                {item.value}
              </span>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}
