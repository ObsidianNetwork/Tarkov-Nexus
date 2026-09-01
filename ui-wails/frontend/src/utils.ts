// Utility functions

import clsx, { ClassValue } from 'clsx';

// Combine class names (Tailwind utility)
export function cn(...inputs: ClassValue[]) {
  return clsx(inputs);
}

// Format timestamp
export function formatTimestamp(timestamp: string): string {
  if (!timestamp) return 'Never';

  try {
    const date = new Date(timestamp);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffSecs = Math.floor(diffMs / 1000);

    if (diffSecs < 60) return `${diffSecs}s ago`;
    if (diffSecs < 3600) return `${Math.floor(diffSecs / 60)}m ago`;
    if (diffSecs < 86400) return `${Math.floor(diffSecs / 3600)}h ago`;

    return date.toLocaleString();
  } catch (e) {
    return timestamp;
  }
}

// Format duration
export function formatDuration(durationStr: string): string {
  if (!durationStr) return '0s';

  // Parse Go duration format (e.g., "1h23m45s" or "1.234567s")
  const hours = durationStr.match(/(\d+)h/);
  const minutes = durationStr.match(/(\d+)m/);
  const seconds = durationStr.match(/([\d.]+)s/);

  const parts: string[] = [];
  if (hours) parts.push(`${hours[1]}h`);
  if (minutes) parts.push(`${minutes[1]}m`);
  if (seconds && !hours && !minutes) {
    const secs = parseFloat(seconds[1]);
    parts.push(`${Math.floor(secs)}s`);
  }

  return parts.join(' ') || '0s';
}

// Get log level color
export function getLogLevelColor(level: string): string {
  switch (level.toLowerCase()) {
    case 'error':
      return 'text-red-400';
    case 'warning':
      return 'text-yellow-400';
    case 'info':
      return 'text-blue-400';
    case 'debug':
      return 'text-primary-purple';
    default:
      return 'text-gray-400';
  }
}

// Get log level badge style
export function getLogLevelBadge(level: string): string {
  switch (level.toLowerCase()) {
    case 'error':
      return 'bg-red-500/10 text-red-400 border-red-500/20';
    case 'warning':
      return 'bg-yellow-500/10 text-yellow-400 border-yellow-500/20';
    case 'info':
      return 'bg-blue-500/10 text-blue-400 border-blue-500/20';
    case 'debug':
      return 'bg-primary-purple/10 text-primary-purple border-primary-purple/20';
    default:
      return 'bg-gray-500/10 text-gray-400 border-gray-500/20';
  }
}
