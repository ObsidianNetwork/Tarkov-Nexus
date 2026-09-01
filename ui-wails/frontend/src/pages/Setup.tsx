import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  CheckCircleIcon,
  ChevronRightIcon,
  FolderIcon,
  RocketLaunchIcon,
  MapIcon,
  ShieldCheckIcon,
} from '@heroicons/react/24/outline';
import {
  GetConfig,
  SaveConfig,
  GetDefaultScreenshotDir,
  GetDefaultLogsDir,
  OpenMapWindow,
} from '../../wailsjs/go/main/App';
import { cn } from '../utils';
import type { Config } from '../types';
import { Button, Card } from '../components/ui';

const STEPS = [
  { id: 1, title: 'Welcome', description: 'Get started' },
  { id: 2, title: 'Setup', description: 'Configure & connect' },
  { id: 3, title: 'Complete', description: 'Ready to go' },
];

export function Setup() {
  const [currentStep, setCurrentStep] = useState(1);
  const [config, setConfig] = useState<Config | null>(null);
  const [autoDetecting, setAutoDetecting] = useState(false);
  const [autoDetectDone, setAutoDetectDone] = useState(false);
  const [mapOpened, setMapOpened] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    loadConfig();
  }, []);

  const loadConfig = async () => {
    try {
      const cfg = await GetConfig();
      setConfig(cfg as Config);
    } catch (err) {
      console.error('Failed to load config:', err);
    }
  };

  const handleAutoDetect = async () => {
    if (!config) return;
    setAutoDetecting(true);
    try {
      const [ssDir, logsDir] = await Promise.all([
        GetDefaultScreenshotDir().catch(() => ''),
        GetDefaultLogsDir().catch(() => ''),
      ]);
      const updated = { ...config };
      if (ssDir) updated.screenshotDir = ssDir;
      if (logsDir) updated.logsDir = logsDir;
      updated.autoConnect = true;
      setConfig(updated);
      setAutoDetectDone(true);
    } catch (err) {
      console.error('Auto-detect failed:', err);
    } finally {
      setAutoDetecting(false);
    }
  };

  const handleOpenMap = async () => {
    if (!config) return;
    // Save config first so the map gets the remote ID
    try {
      await SaveConfig(config as any);
      await OpenMapWindow();
      setMapOpened(true);
    } catch (err) {
      console.error('Failed to open map:', err);
    }
  };

  const handleFinish = async () => {
    if (!config) return;
    try {
      const final = { ...config, setupComplete: true, autoConnect: true };
      await SaveConfig(final as any);
      // Full reload so App.tsx re-reads the config and sees setupComplete=true
      window.location.href = '/';
    } catch (err) {
      console.error('Failed to save config:', err);
    }
  };

  const handleNext = () => {
    if (currentStep < STEPS.length) {
      setCurrentStep(currentStep + 1);
    }
  };

  if (!config) {
    return (
      <div className="flex items-center justify-center h-full">
        <div className="text-text-secondary">Loading...</div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      {/* Progress Bar */}
      <div className="border-b border-border-color/30 bg-bg-card/50 backdrop-blur-md">
        <div className="max-w-3xl mx-auto px-8 py-6">
          <div className="flex items-center justify-between">
            {STEPS.map((step, index) => (
              <React.Fragment key={step.id}>
                <div className="flex flex-col items-center">
                  <div
                    className={cn(
                      'w-10 h-10 rounded-full flex items-center justify-center border-2 transition-all duration-300',
                      currentStep > step.id
                        ? 'bg-neon-green border-neon-green text-bg-dark shadow-glow-green'
                        : currentStep === step.id
                        ? 'bg-primary-purple border-primary-purple text-white shadow-glow-md'
                        : 'glass-card border-border-color text-text-muted'
                    )}
                  >
                    {currentStep > step.id ? <CheckCircleIcon className="w-6 h-6" /> : step.id}
                  </div>
                  <div className="mt-2 text-center">
                    <div className={cn('text-sm font-medium', currentStep >= step.id ? 'text-text-primary' : 'text-text-muted')}>
                      {step.title}
                    </div>
                  </div>
                </div>
                {index < STEPS.length - 1 && (
                  <div className={cn('flex-1 h-0.5 mx-4', currentStep > step.id ? 'bg-neon-green' : 'bg-border-color')} />
                )}
              </React.Fragment>
            ))}
          </div>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-auto">
        <div className="max-w-3xl mx-auto px-8 py-12">

          {/* Step 1: Welcome */}
          {currentStep === 1 && (
            <div className="text-center animate-fade-in-up">
              <RocketLaunchIcon className="w-24 h-24 mx-auto mb-8 text-primary-purple" />
              <h1 className="text-5xl font-bold gradient-text mb-6">Welcome to Tarkov Nexus</h1>
              <p className="text-xl text-text-secondary mb-10 max-w-lg mx-auto">
                Real-time position tracking and interactive maps for Escape from Tarkov.
              </p>

              <div className="grid grid-cols-1 md:grid-cols-3 gap-6 mb-12 text-left">
                <Card className="p-6">
                  <MapIcon className="w-8 h-8 text-neon-green mb-3" />
                  <h3 className="font-semibold text-text-primary mb-1">Interactive Map</h3>
                  <p className="text-sm text-text-muted">Built-in tarkov.dev map with party markers overlay</p>
                </Card>
                <Card className="p-6">
                  <ShieldCheckIcon className="w-8 h-8 text-neon-green mb-3" />
                  <h3 className="font-semibold text-text-primary mb-1">Position Tracking</h3>
                  <p className="text-sm text-text-muted">Automatically syncs your position from screenshots</p>
                </Card>
                <Card className="p-6">
                  <RocketLaunchIcon className="w-8 h-8 text-neon-green mb-3" />
                  <h3 className="font-semibold text-text-primary mb-1">Zero Setup Required</h3>
                  <p className="text-sm text-text-muted">Remote ID is generated automatically, just configure directories</p>
                </Card>
              </div>

              <Button onClick={handleNext} size="lg" className="bg-neon-green hover:bg-neon-green/90 hover:shadow-glow-green px-12">
                Get Started
                <ChevronRightIcon className="w-5 h-5 ml-2" />
              </Button>
            </div>
          )}

          {/* Step 2: Setup (Directories + Map) */}
          {currentStep === 2 && (
            <div className="animate-fade-in-up">
              <h2 className="text-3xl font-bold gradient-text mb-2">Quick Setup</h2>
              <p className="text-text-secondary mb-8">
                We'll auto-detect your directories and connect to tarkov.dev.
              </p>

              {/* Remote ID - already generated */}
              <Card className="mb-6 border-neon-green/30 bg-neon-green/5">
                <div className="flex items-center gap-4">
                  <div className="w-10 h-10 rounded-full bg-neon-green/20 flex items-center justify-center flex-shrink-0">
                    <CheckCircleIcon className="w-6 h-6 text-neon-green" />
                  </div>
                  <div className="flex-1">
                    <h3 className="font-semibold text-text-primary">Remote ID Generated</h3>
                    <p className="text-sm text-text-muted">Your connection ID is ready to use</p>
                  </div>
                  <span className="text-2xl font-bold text-neon-green tracking-widest font-mono">{config.remoteId}</span>
                </div>
              </Card>

              {/* Auto-detect directories */}
              <Card className="mb-6">
                <h3 className="text-lg font-semibold text-text-primary mb-4">Game Directories</h3>
                <p className="text-sm text-text-muted mb-4">
                  Click below to automatically find your Tarkov screenshot and log folders.
                </p>

                {!autoDetectDone ? (
                  <Button
                    onClick={handleAutoDetect}
                    loading={autoDetecting}
                    disabled={autoDetecting}
                    variant="primary"
                    icon={<FolderIcon className="w-5 h-5" />}
                    size="lg"
                    className="w-full"
                  >
                    {autoDetecting ? 'Detecting...' : 'Auto-Detect Directories'}
                  </Button>
                ) : (
                  <div className="space-y-3">
                    <div className="flex items-center gap-3 p-3 rounded-lg bg-bg-dark/50">
                      <CheckCircleIcon className="w-5 h-5 text-neon-green flex-shrink-0" />
                      <div className="flex-1 min-w-0">
                        <div className="text-xs text-text-muted">Screenshots</div>
                        <div className="text-sm text-text-secondary truncate font-mono">
                          {config.screenshotDir || 'Not found — set manually in Settings'}
                        </div>
                      </div>
                    </div>
                    <div className="flex items-center gap-3 p-3 rounded-lg bg-bg-dark/50">
                      <CheckCircleIcon className="w-5 h-5 text-neon-green flex-shrink-0" />
                      <div className="flex-1 min-w-0">
                        <div className="text-xs text-text-muted">Logs</div>
                        <div className="text-sm text-text-secondary truncate font-mono">
                          {config.logsDir || 'Not found — set manually in Settings'}
                        </div>
                      </div>
                    </div>
                  </div>
                )}
              </Card>

              {/* Open map to verify */}
              <Card className="mb-8">
                <h3 className="text-lg font-semibold text-text-primary mb-4">Verify Map Connection</h3>
                <p className="text-sm text-text-muted mb-4">
                  Open the interactive map to verify everything is working. Your Remote ID will connect automatically.
                </p>
                <Button
                  onClick={handleOpenMap}
                  variant="secondary"
                  icon={<MapIcon className="w-5 h-5" />}
                  size="lg"
                  className="w-full"
                >
                  {mapOpened ? 'Map Opened — Check the Window' : 'Open Map'}
                </Button>
                {mapOpened && (
                  <p className="text-sm text-neon-green mt-3 text-center">
                    Map window opened! You should see the interactive map with your Remote ID connected.
                  </p>
                )}
              </Card>

              <Button onClick={handleNext} size="lg" className="bg-neon-green hover:bg-neon-green/90 hover:shadow-glow-green w-full">
                Continue
                <ChevronRightIcon className="w-5 h-5 ml-2" />
              </Button>
            </div>
          )}

          {/* Step 3: Complete */}
          {currentStep === 3 && (
            <div className="text-center animate-fade-in-up">
              <div className="w-24 h-24 bg-neon-green rounded-full flex items-center justify-center mx-auto mb-8 shadow-glow-green">
                <CheckCircleIcon className="w-14 h-14 text-bg-dark" />
              </div>
              <h2 className="text-4xl font-bold gradient-text mb-4">You're All Set!</h2>
              <p className="text-xl text-text-secondary mb-10">
                Tarkov Nexus is configured and ready to track your position.
              </p>

              <Card className="text-left mb-10 max-w-md mx-auto">
                <h3 className="text-lg font-semibold text-text-primary mb-4">Summary</h3>
                <dl className="space-y-3">
                  <div className="flex justify-between">
                    <dt className="text-sm text-text-muted">Remote ID</dt>
                    <dd className="text-sm font-mono font-bold text-neon-green">{config.remoteId}</dd>
                  </div>
                  <div className="flex justify-between">
                    <dt className="text-sm text-text-muted">Screenshots</dt>
                    <dd className="text-sm text-text-secondary">{config.screenshotDir ? 'Configured' : 'Not set'}</dd>
                  </div>
                  <div className="flex justify-between">
                    <dt className="text-sm text-text-muted">Logs</dt>
                    <dd className="text-sm text-text-secondary">{config.logsDir ? 'Configured' : 'Not set'}</dd>
                  </div>
                  <div className="flex justify-between">
                    <dt className="text-sm text-text-muted">Auto-connect</dt>
                    <dd className="text-sm text-text-secondary">Enabled</dd>
                  </div>
                </dl>
              </Card>

              <p className="text-text-muted text-sm mb-8">
                You can change any of these settings later from the Settings page.
              </p>

              <Button onClick={handleFinish} size="lg" className="bg-neon-green hover:bg-neon-green/90 hover:shadow-glow-green px-12">
                <CheckCircleIcon className="w-5 h-5 mr-2" />
                Go to Dashboard
              </Button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
