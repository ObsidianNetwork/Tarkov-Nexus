import React, { useState, useEffect, useCallback, createContext, useContext } from 'react';
import { Transition } from '@headlessui/react';
import {
  UserPlusIcon,
  UserGroupIcon,
  CheckIcon,
  XMarkIcon,
  BellIcon,
  ExclamationTriangleIcon,
  InformationCircleIcon,
} from '@heroicons/react/24/outline';
import { Button } from './ui';

// Sound utility using Web Audio API
const createNotificationSound = (type: 'friendRequest' | 'partyInvite' | 'success' | 'info') => {
  try {
    const audioContext = new (window.AudioContext || (window as any).webkitAudioContext)();
    const oscillator = audioContext.createOscillator();
    const gainNode = audioContext.createGain();

    oscillator.connect(gainNode);
    gainNode.connect(audioContext.destination);

    // Different sounds for different notification types
    switch (type) {
      case 'friendRequest':
        // Two-tone ascending - friendly notification
        oscillator.frequency.setValueAtTime(523.25, audioContext.currentTime); // C5
        oscillator.frequency.setValueAtTime(659.25, audioContext.currentTime + 0.1); // E5
        oscillator.type = 'sine';
        gainNode.gain.setValueAtTime(0.3, audioContext.currentTime);
        gainNode.gain.exponentialRampToValueAtTime(0.01, audioContext.currentTime + 0.3);
        oscillator.start(audioContext.currentTime);
        oscillator.stop(audioContext.currentTime + 0.3);
        break;
      case 'partyInvite':
        // Three-tone chord - attention grabbing
        oscillator.frequency.setValueAtTime(440, audioContext.currentTime); // A4
        oscillator.frequency.setValueAtTime(554.37, audioContext.currentTime + 0.1); // C#5
        oscillator.frequency.setValueAtTime(659.25, audioContext.currentTime + 0.2); // E5
        oscillator.type = 'sine';
        gainNode.gain.setValueAtTime(0.3, audioContext.currentTime);
        gainNode.gain.exponentialRampToValueAtTime(0.01, audioContext.currentTime + 0.4);
        oscillator.start(audioContext.currentTime);
        oscillator.stop(audioContext.currentTime + 0.4);
        break;
      case 'success':
        // Quick ascending ding
        oscillator.frequency.setValueAtTime(880, audioContext.currentTime); // A5
        oscillator.type = 'sine';
        gainNode.gain.setValueAtTime(0.2, audioContext.currentTime);
        gainNode.gain.exponentialRampToValueAtTime(0.01, audioContext.currentTime + 0.15);
        oscillator.start(audioContext.currentTime);
        oscillator.stop(audioContext.currentTime + 0.15);
        break;
      case 'info':
        // Soft single tone
        oscillator.frequency.setValueAtTime(587.33, audioContext.currentTime); // D5
        oscillator.type = 'sine';
        gainNode.gain.setValueAtTime(0.15, audioContext.currentTime);
        gainNode.gain.exponentialRampToValueAtTime(0.01, audioContext.currentTime + 0.2);
        oscillator.start(audioContext.currentTime);
        oscillator.stop(audioContext.currentTime + 0.2);
        break;
    }
  } catch (e) {
    console.warn('Could not play notification sound:', e);
  }
};

export type ToastType = 'friendRequest' | 'partyInvite' | 'success' | 'error' | 'info';

export interface ToastAction {
  label: string;
  onClick: () => void;
  variant?: 'primary' | 'secondary';
  icon?: React.ReactNode;
}

export interface Toast {
  id: string;
  type: ToastType;
  title: string;
  message?: string;
  actions?: ToastAction[];
  duration?: number; // 0 = persistent until dismissed
  data?: any; // Additional data (e.g., clientId for friend requests)
}

interface ToastContextType {
  addToast: (toast: Omit<Toast, 'id'>) => string;
  removeToast: (id: string) => void;
  clearAll: () => void;
}

const ToastContext = createContext<ToastContextType | null>(null);

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error('useToast must be used within a ToastProvider');
  }
  return context;
}

interface ToastProviderProps {
  children: React.ReactNode;
  soundEnabled?: boolean;
}

export function ToastProvider({ children, soundEnabled = true }: ToastProviderProps) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const addToast = useCallback((toast: Omit<Toast, 'id'>) => {
    const id = `toast-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
    const newToast: Toast = { ...toast, id };

    setToasts((prev) => [...prev, newToast]);

    // Play sound for non-error types
    if (soundEnabled && toast.type !== 'error') {
      createNotificationSound(toast.type);
    }

    // Auto-dismiss after duration (default 8 seconds for actionable, 5 for info)
    const duration = toast.duration ?? (toast.actions ? 8000 : 5000);
    if (duration > 0) {
      setTimeout(() => {
        removeToast(id);
      }, duration);
    }

    return id;
  }, [soundEnabled]);

  const removeToast = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const clearAll = useCallback(() => {
    setToasts([]);
  }, []);

  return (
    <ToastContext.Provider value={{ addToast, removeToast, clearAll }}>
      {children}
      <ToastContainer toasts={toasts} onDismiss={removeToast} />
    </ToastContext.Provider>
  );
}

interface ToastContainerProps {
  toasts: Toast[];
  onDismiss: (id: string) => void;
}

function ToastContainer({ toasts, onDismiss }: ToastContainerProps) {
  return (
    <div className="fixed top-4 right-4 z-[100] flex flex-col gap-3 pointer-events-none">
      {toasts.map((toast) => (
        <ToastItem key={toast.id} toast={toast} onDismiss={() => onDismiss(toast.id)} />
      ))}
    </div>
  );
}

interface ToastItemProps {
  toast: Toast;
  onDismiss: () => void;
}

function ToastItem({ toast, onDismiss }: ToastItemProps) {
  const [isVisible, setIsVisible] = useState(false);

  useEffect(() => {
    // Trigger enter animation
    const timer = setTimeout(() => setIsVisible(true), 10);
    return () => clearTimeout(timer);
  }, []);

  const handleDismiss = () => {
    setIsVisible(false);
    setTimeout(onDismiss, 200);
  };

  const getIcon = () => {
    switch (toast.type) {
      case 'friendRequest':
        return <UserPlusIcon className="h-6 w-6 text-primary-purple" />;
      case 'partyInvite':
        return <UserGroupIcon className="h-6 w-6 text-blue-400" />;
      case 'success':
        return <CheckIcon className="h-6 w-6 text-neon-green" />;
      case 'error':
        return <ExclamationTriangleIcon className="h-6 w-6 text-error" />;
      case 'info':
        return <InformationCircleIcon className="h-6 w-6 text-text-secondary" />;
    }
  };

  const getBorderColor = () => {
    switch (toast.type) {
      case 'friendRequest':
        return 'border-primary-purple/50';
      case 'partyInvite':
        return 'border-blue-500/50';
      case 'success':
        return 'border-neon-green/50';
      case 'error':
        return 'border-error/50';
      case 'info':
        return 'border-border-color';
    }
  };

  const getGlowColor = () => {
    switch (toast.type) {
      case 'friendRequest':
        return 'shadow-primary-purple/20';
      case 'partyInvite':
        return 'shadow-blue-500/20';
      case 'success':
        return 'shadow-neon-green/20';
      case 'error':
        return 'shadow-error/20';
      case 'info':
        return 'shadow-black/20';
    }
  };

  return (
    <Transition
      show={isVisible}
      enter="transition-all duration-300 ease-out"
      enterFrom="opacity-0 translate-x-full scale-95"
      enterTo="opacity-100 translate-x-0 scale-100"
      leave="transition-all duration-200 ease-in"
      leaveFrom="opacity-100 translate-x-0 scale-100"
      leaveTo="opacity-0 translate-x-full scale-95"
    >
      <div
        className={`
          pointer-events-auto
          w-80 max-w-[calc(100vw-2rem)]
          bg-bg-card/95 backdrop-blur-md
          border ${getBorderColor()}
          rounded-lg shadow-lg ${getGlowColor()}
          overflow-hidden
        `}
      >
        {/* Progress bar for auto-dismiss */}
        {toast.duration !== 0 && (
          <div className="h-0.5 bg-bg-hover overflow-hidden">
            <div
              className={`h-full ${toast.type === 'error' ? 'bg-error' : toast.type === 'friendRequest' ? 'bg-primary-purple' : toast.type === 'partyInvite' ? 'bg-blue-500' : 'bg-neon-green'}`}
              style={{
                animation: `shrink ${toast.duration || (toast.actions ? 8000 : 5000)}ms linear forwards`,
              }}
            />
          </div>
        )}

        <div className="p-4">
          <div className="flex items-start gap-3">
            {/* Icon */}
            <div className="flex-shrink-0 mt-0.5">
              {getIcon()}
            </div>

            {/* Content */}
            <div className="flex-1 min-w-0">
              <div className="flex items-start justify-between gap-2">
                <h4 className="font-semibold text-text-primary text-sm">
                  {toast.title}
                </h4>
                <button
                  onClick={handleDismiss}
                  className="text-text-muted hover:text-text-primary transition-colors flex-shrink-0"
                >
                  <XMarkIcon className="h-4 w-4" />
                </button>
              </div>

              {toast.message && (
                <p className="text-sm text-text-secondary mt-1">
                  {toast.message}
                </p>
              )}

              {/* Actions */}
              {toast.actions && toast.actions.length > 0 && (
                <div className="flex gap-2 mt-3">
                  {toast.actions.map((action, i) => (
                    <Button
                      key={i}
                      size="sm"
                      variant={action.variant || (i === 0 ? 'primary' : 'secondary')}
                      onClick={() => {
                        action.onClick();
                        handleDismiss();
                      }}
                    >
                      {action.icon}
                      {action.label}
                    </Button>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </Transition>
  );
}

// Add CSS keyframe animation for progress bar
const styleSheet = document.createElement('style');
styleSheet.textContent = `
  @keyframes shrink {
    from { width: 100%; }
    to { width: 0%; }
  }
`;
document.head.appendChild(styleSheet);

export default ToastProvider;
