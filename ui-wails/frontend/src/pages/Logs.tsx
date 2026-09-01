import React, { useState, useEffect, useRef } from 'react';
import { FixedSizeList } from 'react-window';
import {
  FunnelIcon,
  TrashIcon,
  ArrowDownTrayIcon,
  MagnifyingGlassIcon,
  DocumentTextIcon,
} from '@heroicons/react/24/outline';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { GetLogs, ClearLogs, ExportLogs, GetLogStats } from '../../wailsjs/go/main/App';
import { cn, getLogLevelColor, getLogLevelBadge } from '../utils';
import type { LogEntry, LogLevel, LogStats } from '../types';
import { Button, Badge } from '../components/ui';

export function Logs() {
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [filteredLogs, setFilteredLogs] = useState<LogEntry[]>([]);
  const [stats, setStats] = useState<LogStats | null>(null);
  const [selectedLevel, setSelectedLevel] = useState<LogLevel>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [autoScroll, setAutoScroll] = useState(true);
  const [isLoading, setIsLoading] = useState(true);
  const listRef = useRef<any>(null);
  const filteredLogsLengthRef = useRef(0);

  useEffect(() => {
    const init = async () => {
      await Promise.all([loadLogs(), loadStats()]);
      setIsLoading(false);
    };
    init();

    // Listen for new log entries
    const cleanup = EventsOn('log:added', (entry: LogEntry) => {
      setLogs((prev) => [...prev, entry]);
    });

    // Refresh logs every 2 seconds
    const interval = setInterval(() => {
      loadLogs();
      loadStats();
    }, 2000);

    return () => {
      if (typeof cleanup === 'function') cleanup();
      clearInterval(interval);
    };
  }, []);

  useEffect(() => {
    // Apply filters
    let filtered = logs;

    // Filter by level
    if (selectedLevel !== 'all') {
      filtered = filtered.filter((log) => log.level.toLowerCase() === selectedLevel);
    }

    // Filter by search query
    if (searchQuery) {
      const query = searchQuery.toLowerCase();
      filtered = filtered.filter((log) => log.message.toLowerCase().includes(query));
    }

    setFilteredLogs(filtered);
  }, [logs, selectedLevel, searchQuery]);

  // Auto-scroll only when new logs are added (length increases)
  useEffect(() => {
    if (autoScroll && listRef.current && filteredLogs.length > 0) {
      const currentLength = filteredLogs.length;
      if (currentLength > filteredLogsLengthRef.current) {
        listRef.current.scrollToItem(currentLength - 1, 'end');
        filteredLogsLengthRef.current = currentLength;
      }
    }
  }, [filteredLogs, autoScroll]);

  const loadLogs = async () => {
    try {
      const entries = await GetLogs();
      setLogs(entries as LogEntry[]);
    } catch (err) {
      console.error('Failed to load logs:', err);
    }
  };

  const loadStats = async () => {
    try {
      const logStats = await GetLogStats();
      setStats(logStats as unknown as LogStats);
    } catch (err) {
      console.error('Failed to load log stats:', err);
    }
  };

  const handleClearLogs = async () => {
    if (!confirm('Are you sure you want to clear all logs?')) return;

    try {
      await ClearLogs();
      setLogs([]);
      setFilteredLogs([]);
    } catch (err) {
      console.error('Failed to clear logs:', err);
    }
  };

  const handleExportLogs = async () => {
    try {
      const filepath = await ExportLogs();
      alert(`Logs exported to: ${filepath}`);
    } catch (err) {
      console.error('Failed to export logs:', err);
      alert('Failed to export logs');
    }
  };

  const LogRow = ({ index, style }: { index: number; style: React.CSSProperties }) => {
    const log = filteredLogs[index];
    if (!log) return null;

    const timestamp = new Date(log.timestamp).toLocaleTimeString();

    return (
      <div style={style} className="px-4 py-2 border-b border-border-color/20 hover:bg-bg-hover/30 overflow-hidden transition-colors duration-200">
        <div className="flex items-start gap-3 font-mono text-sm">
          <span className="text-text-muted flex-shrink-0">{timestamp}</span>
          <span className={cn('font-semibold flex-shrink-0 w-16', getLogLevelColor(log.level))}>
            {log.level.toUpperCase()}
          </span>
          <span
            className="text-text-secondary flex-1 line-clamp-2"
            title={log.message}
          >
            {log.message}
          </span>
        </div>
      </div>
    );
  };

  return (
    <div className="flex flex-col h-full animate-fade-in-up">
      {/* Header */}
      <div className="p-6 border-b border-border-color/30">
        <div className="flex items-center justify-between mb-4">
          <div>
            <h1 className="text-4xl font-bold gradient-text mb-2">Logs</h1>
            <p className="text-text-secondary">Real-time application logs and events</p>
          </div>
          <div className="flex gap-3">
            <Button
              onClick={handleExportLogs}
              variant="secondary"
              icon={<ArrowDownTrayIcon className="w-5 h-5" />}
            >
              Export
            </Button>
            <Button
              onClick={handleClearLogs}
              variant="danger"
              icon={<TrashIcon className="w-5 h-5" />}
            >
              Clear
            </Button>
          </div>
        </div>

        {/* Stats */}
        {stats && (
          <div className="flex gap-3 mb-4">
            <Badge variant="default">{`Total: ${stats.total}`}</Badge>
            <Badge variant="info">{`Info: ${stats.info}`}</Badge>
            <Badge variant="warning">{`Warning: ${stats.warning}`}</Badge>
            <Badge variant="error">{`Error: ${stats.error}`}</Badge>
            <Badge variant="default" className="bg-primary-purple/10 text-primary-purple border-primary-purple/20">{`Debug: ${stats.debug}`}</Badge>
          </div>
        )}

        {/* Filters */}
        <div className="flex gap-4">
          {/* Level Filter */}
          <div className="flex-shrink-0">
            <div className="flex items-center gap-3">
              <FunnelIcon className="w-5 h-5 text-primary-purple" />
              <div className="flex gap-2">
                {(['all', 'info', 'warning', 'error', 'debug'] as const).map((level) => (
                  <button
                    key={level}
                    onClick={() => setSelectedLevel(level)}
                    className={cn(
                      'px-3 py-1.5 rounded-lg text-sm font-medium transition-all duration-300 border',
                      selectedLevel === level
                        ? getLogLevelBadge(level)
                        : 'glass-card text-text-muted hover:text-text-primary hover:border-primary-purple/50'
                    )}
                  >
                    {level.charAt(0).toUpperCase() + level.slice(1)}
                  </button>
                ))}
              </div>
            </div>
          </div>

          {/* Search */}
          <div className="flex-1">
            <div className="relative">
              <MagnifyingGlassIcon className="absolute left-3 top-1/2 transform -translate-y-1/2 w-5 h-5 text-text-muted" />
              <input
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search logs..."
                className="glass-input w-full pl-10 pr-4 py-2"
              />
            </div>
          </div>

          {/* Auto-scroll toggle */}
          <label className="flex items-center cursor-pointer group">
            <input
              type="checkbox"
              checked={autoScroll}
              onChange={(e) => setAutoScroll(e.target.checked)}
              className="w-4 h-4 text-primary-purple bg-bg-card border-border-color rounded focus:ring-2 focus:ring-primary-purple/50 transition-all"
            />
            <span className="ml-3 text-text-secondary group-hover:text-text-primary text-sm transition-colors">Auto-scroll</span>
          </label>
        </div>
      </div>

      {/* Log List */}
      <div className="flex-1 bg-bg-darker">
        {isLoading ? (
          <div className="flex items-center justify-center h-full">
            <div className="text-text-secondary text-center">
              <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-purple mx-auto mb-4"></div>
              <p className="text-sm">Loading logs...</p>
            </div>
          </div>
        ) : filteredLogs.length === 0 ? (
          <div className="flex items-center justify-center h-full">
            <div className="text-text-muted text-center">
              <DocumentTextIcon className="w-16 h-16 mx-auto mb-4 text-text-muted/50" />
              <p className="text-lg mb-2">No logs to display</p>
              {searchQuery && <p className="text-sm">Try a different search query</p>}
            </div>
          </div>
        ) : (
          <FixedSizeList
            ref={listRef}
            height={window.innerHeight - 300} // Adjust based on header height
            itemCount={filteredLogs.length}
            itemSize={60}
            width="100%"
          >
            {LogRow}
          </FixedSizeList>
        )}
      </div>

      {/* Footer */}
      <div className="px-6 py-3 border-t border-border-color/30 bg-bg-card/50">
        <div className="flex items-center justify-between text-sm text-text-muted">
          <span>
            Showing <span className="text-primary-purple font-medium">{filteredLogs.length}</span> of <span className="text-primary-purple font-medium">{logs.length}</span> logs
          </span>
          <span>Buffer: Last 500 entries</span>
        </div>
      </div>
    </div>
  );
}
