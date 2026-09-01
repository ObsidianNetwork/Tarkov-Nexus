import React, { useState, useEffect } from 'react';
import { NavLink, Outlet } from 'react-router-dom';
import {
  HomeIcon,
  Cog6ToothIcon,
  DocumentTextIcon,
  UserGroupIcon,
} from '@heroicons/react/24/outline';
import { useApp } from '../context/AppContext';
import { cn } from '../utils';
import { StatusIndicator, ParticleBackground } from './ui';
import { TitleBar } from './TitleBar';
import { PartyNotifications } from './PartyNotifications';
import ObiLogo from '../assets/images/Obi_logo-trans.png';
import { GetVersion } from '../../wailsjs/go/main/App';

const navigation = [
  { name: 'Dashboard', href: '/', icon: HomeIcon },
  { name: 'Party', href: '/party', icon: UserGroupIcon },
  { name: 'Settings', href: '/settings', icon: Cog6ToothIcon },
  { name: 'Logs', href: '/logs', icon: DocumentTextIcon },
];

export function Layout() {
  const { status } = useApp();
  const [version, setVersion] = useState('');

  useEffect(() => {
    GetVersion().then(v => setVersion(v)).catch(() => setVersion(''));
  }, []);

  return (
    <div className="flex flex-col h-full bg-bg-dark relative p-0">
      <div className="flex flex-col h-full bg-bg-dark relative overflow-hidden rounded-xl" style={{ boxShadow: '0 0 8px 2px rgba(12, 14, 20, 0.95)' }}>
      {/* Particle Background */}
      <ParticleBackground />

      {/* Title Bar */}
      <TitleBar />

      {/* Main Layout: Sidebar + Content */}
      <div className="flex flex-1 overflow-hidden relative z-10">
        {/* Sidebar */}
        <div className="flex flex-col h-full w-64 relative z-10 rounded-bl-xl overflow-hidden">
        {/* Glass effect background */}
        <div className="absolute inset-0 bg-bg-card/90 backdrop-blur-xl border-r border-border-color/30 rounded-bl-xl" />

        {/* Gradient border on right edge */}
        <div className="absolute inset-y-0 right-0 w-px bg-gradient-to-b from-transparent via-primary-purple/50 to-transparent" />

        {/* Content */}
        <div className="relative flex flex-col h-full">
          {/* Logo/Title */}
          <div className="flex items-center gap-3 h-16 px-6 border-b border-border-color/30">
            <img src={ObiLogo} alt="Obsidian Network" className="w-10 h-10 flex-shrink-0" />
            <div className="flex flex-col">
              <h1 className="text-xl font-bold gradient-text leading-tight">Tarkov Nexus</h1>
              <span className="text-xs text-text-muted leading-tight">By Obsidian Network</span>
            </div>
          </div>

          {/* Navigation */}
          <nav className="flex-1 px-4 py-6 space-y-2">
            {navigation.map((item) => (
              <NavLink
                key={item.name}
                to={item.href}
                end={item.href === '/'}
                className={({ isActive }) =>
                  cn(
                    'group flex items-center px-4 py-3 text-sm font-medium rounded-lg',
                    'transition-all duration-300 ease-out relative overflow-hidden',
                    isActive
                      ? 'bg-primary-purple/20 text-text-primary shadow-glow-sm'
                      : 'text-text-secondary hover:bg-bg-hover hover:text-text-primary'
                  )
                }
              >
                {({ isActive }) => (
                  <>
                    {/* Active indicator gradient */}
                    {isActive && (
                      <div className="absolute inset-0 bg-gradient-to-r from-primary-purple/20 via-electric-purple/10 to-transparent" />
                    )}

                    {/* Icon */}
                    <item.icon
                      className={cn(
                        'w-5 h-5 mr-3 relative z-10 transition-all duration-300',
                        isActive
                          ? 'text-primary-purple'
                          : 'text-text-muted group-hover:text-primary-purple group-hover:scale-110'
                      )}
                    />

                    {/* Text */}
                    <span className="relative z-10">{item.name}</span>

                    {/* Animated underline on hover (for inactive items) */}
                    {!isActive && (
                      <div className="absolute bottom-0 left-4 right-4 h-0.5 bg-gradient-to-r from-primary-purple to-electric-purple scale-x-0 group-hover:scale-x-100 transition-transform duration-300 origin-left" />
                    )}
                  </>
                )}
              </NavLink>
            ))}
          </nav>

          {/* Status indicator */}
          <div className="px-4 py-4 border-t border-border-color/30">
            <div className="glass-card p-3">
              <StatusIndicator
                status={status?.connected ? 'success' : 'offline'}
                label={status?.connected ? 'Connected' : 'Disconnected'}
                size="md"
                pulse={status?.connected}
                glow={status?.connected}
              />
            </div>
          </div>

          {/* Version */}
          <div className="px-6 py-3 text-xs text-text-muted border-t border-border-color/20">
            <div className="flex items-center justify-between">
              <span>Version</span>
              <span className="font-mono text-text-secondary">v{version || '...'}</span>
            </div>
          </div>
        </div>
      </div>

      {/* Main content */}
      <div className="flex-1 overflow-auto relative z-10 custom-scrollbar rounded-br-xl">
        <Outlet />
      </div>
    </div>

    {/* Global party notifications */}
    <PartyNotifications />
    </div>
  </div>
  );
}
