import React, { ReactNode } from 'react';
import { Disclosure, Transition } from '@headlessui/react';
import { ChevronDownIcon } from '@heroicons/react/24/outline';
import { cn } from '../../utils';

export interface PanelProps {
  title: string;
  icon?: ReactNode;
  defaultOpen?: boolean;
  children: ReactNode;
  className?: string;
  headerClassName?: string;
}

export const Panel: React.FC<PanelProps> = ({
  title,
  icon,
  defaultOpen = false,
  children,
  className,
  headerClassName,
}) => {
  return (
    <Disclosure defaultOpen={defaultOpen}>
      {({ open }) => (
        <div className={cn('glass-panel overflow-hidden', className)}>
          <Disclosure.Button
            className={cn(
              'flex w-full items-center justify-between p-6 text-left transition-all duration-300',
              'hover:bg-bg-hover focus:outline-none focus:ring-2 focus:ring-primary-purple/50 focus:ring-inset',
              open ? 'rounded-t-xl' : 'rounded-xl',
              headerClassName
            )}
          >
            <div className="flex items-center gap-3">
              {icon && <div className="flex-shrink-0 text-primary-purple">{icon}</div>}
              <h3 className="text-xl font-semibold text-text-primary">{title}</h3>
            </div>

            <ChevronDownIcon
              className={cn(
                'h-5 w-5 text-primary-purple transition-transform duration-300',
                open && 'rotate-180'
              )}
            />
          </Disclosure.Button>

          <Transition
            enter="transition duration-300 ease-out"
            enterFrom="transform opacity-0 -translate-y-2"
            enterTo="transform opacity-100 translate-y-0"
            leave="transition duration-200 ease-out"
            leaveFrom="transform opacity-100 translate-y-0"
            leaveTo="transform opacity-0 -translate-y-2"
          >
            <Disclosure.Panel className="px-6 pb-6 border-t border-border-color/30 overflow-visible">
              <div className="pt-6">{children}</div>
            </Disclosure.Panel>
          </Transition>
        </div>
      )}
    </Disclosure>
  );
};

Panel.displayName = 'Panel';
