// Type definitions for Tarkov Nexus

export interface Config {
  remoteId: string;
  screenshotDir: string;
  logsDir: string;
  autoConnect: boolean;
  autoProcessExisting: boolean;
  positionUpdateThrottleMs: number;
  debugLogging: boolean;
  darkMode: boolean;
  enableQuestTracking: boolean;
  tarkovTrackerUrl: string;
  tarkovTrackerToken: string;
  tarkovTrackerGameMode: string; // "pvp" or "pve"
  monitorOptions: {
    debounceTimeMs: number;
    ignoreInitial: boolean;
  };
  reconnectOptions: {
    reconnectIntervalMs: number;
    maxReconnectAttempts: number;
  };
  updateSettings: {
    enableAutoCheck: boolean;
    checkIntervalHours: number;
    updateChannel: 'stable' | 'beta';
  };
  partySettings: {
    enabled: boolean;
    serverUrl: string;           // Centralized party server URL
    clientId: string;            // Persistent client ID (UUID)
    // Legacy fields (deprecated)
    serverPort?: number;
    partyCode?: string;
    displayName: string;
  };
  setupComplete: boolean;
}

export interface Status {
  isRunning: boolean;
  isConfigured: boolean;
  hasScreenshotDir: boolean;
  hasLogsDir: boolean;
  initialized: boolean;
  connected: boolean;
  monitoring: boolean;
  currentMap: string;
  currentNormalizedMap: string;
  manualMapOverride?: string;
  lastPosition?: PositionData;
  lastUpdate: string;
  screenshot: {
    isMonitoring: boolean;
    processedCount: number;
    pendingCount: number;
  };
  websocket: {
    totalReconnects: number;
    lastPingTime: string;
    connectionUptime: string;
  };
  questTracking: {
    enabled: boolean;
    configured: boolean;
  };
}

export interface PositionData {
  x: number;
  y: number;
  z: number;
  rotation: number;
  map: string;
  source: string;
  confidence: number;
  filepath: string;
  timestamp: string;
  processed: boolean;
}

export interface LogEntry {
  timestamp: string;
  level: string;
  message: string;
}

export interface LogStats {
  total: number;
  info: number;
  warning: number;
  error: number;
  debug: number;
}

export interface MapInfo {
  id: string;
  displayName: string;
}

export type LogLevel = 'info' | 'warning' | 'error' | 'debug' | 'all';

// TarkovTracker API Types
export interface TarkovTrackerProgress {
  data: {
    playerLevel: number;
    gameEdition: number;
    pmcFaction: string;
    displayName: string;
    userId: string;
    tasksProgress: TaskProgress[];
    taskObjectivesProgress: TaskObjectiveProgress[];
    hideoutModulesProgress: HideoutModuleProgress[];
    hideoutPartsProgress: HideoutPartProgress[];
  };
  meta: {
    self: string;
  };
}

export interface TaskProgress {
  id: string;
  complete: boolean;
  failed: boolean;
  invalid: boolean;
}

export interface TaskObjectiveProgress {
  id: string;
  count: number;
  complete: boolean;
  invalid: boolean;
}

export interface HideoutModuleProgress {
  id: string;
  complete: boolean;
}

export interface HideoutPartProgress {
  id: string;
  count: number;
  complete: boolean;
}

export interface QuestStats {
  completed: number;
  inProgress: number;
  failed: number;
  total: number;
}

export interface PlayerInfo {
  displayName: string;
  level: number;
  faction: string;
  gameEdition: string;
}

// Auto-Update Types
export interface UpdateInfo {
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

export interface UpdateStatus {
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

// Party/Multiplayer Types
export interface PartyStatus {
  active: boolean;
  isHost: boolean;
  partyCode: string;
  memberCount: number;
  enabled: boolean;
}

export interface PartyMember {
  remoteId: string;
  displayName: string;
  connected: boolean;
  currentMap: string;
  lastPosition?: Position;
  lastUpdate: string;
}

export interface Position {
  x: number;
  y: number;
  z: number;
  rotation: number;
}

export interface PartyPositionUpdate {
  remoteId: string;
  displayName: string;
  map: string;
  position: Position;
  timestamp: string;
}

export interface PartyServerInfo {
  isHosting: boolean;
  port: number;
  ips: string[];
  urls: string[];
  preferredIP: string;
  preferredURL: string;
  portAvailable: boolean;
  firewallWarning: boolean;
}

// Centralized Party Server Types (new)
export interface PartyMemberInfo {
  clientId: string;
  displayName: string;
  currentMap?: string;
  position?: Position;
  isHost: boolean;
}

export interface Friend {
  clientId: string;
  displayName: string;
  online: boolean;
  inParty: boolean;
  partyCode?: string;
}

export interface PartyInvite {
  fromClientId: string;
  fromDisplayName: string;
  partyCode: string;
}

// Connection status for centralized server
export interface CentralPartyStatus {
  connected: boolean;
  registered: boolean;
  inParty: boolean;
  partyCode: string;
  isHost: boolean;
  memberCount: number;
  serverUrl: string;
  clientId: string;
}
