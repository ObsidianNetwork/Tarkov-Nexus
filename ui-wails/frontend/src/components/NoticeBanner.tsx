import React from 'react';
import {
  ExclamationTriangleIcon,
  InformationCircleIcon,
  XMarkIcon,
  XCircleIcon,
} from '@heroicons/react/24/outline';
import type { notice } from '../../wailsjs/go/models';
import { OpenURL } from '../../wailsjs/go/main/App';

interface NoticeBannerProps {
  notices: notice.Notice[];
  onDismiss: (id: string) => void;
}

const severityStyles = {
  critical: {
    wrapper: 'bg-red-500/10 border-red-500/40',
    icon: <XCircleIcon className="w-5 h-5 text-red-400 flex-shrink-0 mt-0.5" />,
    pill: 'bg-red-500/25 text-red-300',
    label: 'Critical',
  },
  warning: {
    wrapper: 'bg-amber-500/10 border-amber-500/40',
    icon: <ExclamationTriangleIcon className="w-5 h-5 text-amber-400 flex-shrink-0 mt-0.5" />,
    pill: 'bg-amber-500/25 text-amber-300',
    label: 'Warning',
  },
  info: {
    wrapper: 'bg-primary-purple/10 border-primary-purple/40',
    icon: <InformationCircleIcon className="w-5 h-5 text-primary-purple flex-shrink-0 mt-0.5" />,
    pill: 'bg-primary-purple/25 text-primary-purple',
    label: 'Info',
  },
} as const;

function styleFor(severity: string) {
  return severityStyles[severity as keyof typeof severityStyles] ?? severityStyles.info;
}

/**
 * Status notices published by the maintainer via notices/status.json —
 * no release required. Severity drives prominence: critical (red),
 * warning (amber), info (purple).
 */
export function NoticeBanner({ notices, onDismiss }: NoticeBannerProps) {
  if (!notices.length) return null;

  return (
    <div className="space-y-3">
      {notices.map((n) => {
        const s = styleFor(n.severity);
        return (
          <div key={n.id} className={`rounded-xl border p-4 ${s.wrapper}`} role="alert">
            <div className="flex items-start gap-3">
              {s.icon}
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                  <span className="font-semibold text-sm text-text-primary">{n.title}</span>
                  <span className={`text-[10px] font-bold uppercase tracking-wide px-2 py-0.5 rounded-full ${s.pill}`}>
                    {s.label}
                  </span>
                </div>
                {n.body && (
                  <p className="text-sm text-text-secondary mt-1 whitespace-pre-wrap">{n.body}</p>
                )}
                {n.link && (
                  <a
                    href="#"
                    onClick={(e) => {
                      e.preventDefault();
                      void OpenURL(n.link!);
                    }}
                    className="text-sm text-primary-purple hover:text-electric-purple transition-colors mt-2 inline-block"
                  >
                    {n.linkText || 'Learn more'} →
                  </a>
                )}
              </div>
              <button
                onClick={() => onDismiss(n.id)}
                className="text-text-muted hover:text-text-primary transition-colors"
                aria-label="Dismiss notice"
              >
                <XMarkIcon className="w-4 h-4" />
              </button>
            </div>
          </div>
        );
      })}
    </div>
  );
}
