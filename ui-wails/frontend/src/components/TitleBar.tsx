import React from 'react';
import { WindowMinimise, WindowToggleMaximise, Quit } from '../../wailsjs/runtime/runtime';
import { MinusIcon, XMarkIcon } from '@heroicons/react/24/outline';
import { Square2StackIcon } from '@heroicons/react/24/outline';
import ObiLogo from '../assets/images/Obi_logo-trans.png';

export const TitleBar: React.FC = () => {
  const handleMinimize = () => {
    WindowMinimise();
  };

  const handleMaximize = () => {
    WindowToggleMaximise();
  };

  const handleClose = () => {
    Quit();
  };

  return (
    <div className="flex items-center justify-between h-10 bg-bg-card/90 backdrop-blur-md border-b border-border-color/30 select-none rounded-t-xl">
      {/* Draggable area with logo and title */}
      <div
        className="flex items-center gap-2 px-4 flex-1 h-full"
        style={{ '--wails-draggable': 'drag' } as React.CSSProperties}
        data-wails-drag
      >
        <img src={ObiLogo} alt="Obsidian Network" className="w-6 h-6 flex-shrink-0" />
        <span className="text-sm font-semibold gradient-text">Tarkov Nexus</span>
      </div>

      {/* Window control buttons */}
      <div className="flex items-center h-full">
        {/* Minimize */}
        <button
          onClick={handleMinimize}
          className="flex items-center justify-center w-12 h-full hover:bg-bg-hover transition-colors duration-200 group"
          aria-label="Minimize"
        >
          <MinusIcon className="w-4 h-4 text-text-muted group-hover:text-primary-purple transition-colors" />
        </button>

        {/* Maximize/Restore */}
        <button
          onClick={handleMaximize}
          className="flex items-center justify-center w-12 h-full hover:bg-bg-hover transition-colors duration-200 group"
          aria-label="Maximize"
        >
          <Square2StackIcon className="w-4 h-4 text-text-muted group-hover:text-primary-purple transition-colors" />
        </button>

        {/* Close */}
        <button
          onClick={handleClose}
          className="flex items-center justify-center w-12 h-full hover:bg-error/20 transition-colors duration-200 group"
          aria-label="Close"
        >
          <XMarkIcon className="w-4 h-4 text-text-muted group-hover:text-error transition-colors" />
        </button>
      </div>
    </div>
  );
};

TitleBar.displayName = 'TitleBar';
