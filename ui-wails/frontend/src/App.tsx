import React, { useState, useEffect } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { AppProvider } from './context/AppContext';
import { ToastProvider } from './components/ToastNotification';
import { Layout } from './components/Layout';
import { Dashboard } from './pages/Dashboard';
import { Settings } from './pages/Settings';
import { Logs } from './pages/Logs';
import { Setup } from './pages/Setup';
import PartyCentral from './pages/PartyCentral';
import { GetConfig } from '../wailsjs/go/main/App';

function App() {
  const [setupDone, setSetupDone] = useState<boolean | null>(null);

  useEffect(() => {
    GetConfig().then((cfg: any) => {
      setSetupDone(cfg?.setupComplete === true);
    }).catch(() => setSetupDone(true)); // if config fails, skip setup
  }, []);

  // Wait until we know if setup is complete
  if (setupDone === null) return null;

  return (
    <AppProvider>
      <ToastProvider>
        <BrowserRouter>
          <Routes>
            {/* Setup wizard — full page, no sidebar */}
            <Route path="/setup" element={<Setup />} />

            {/* Main app with sidebar */}
            <Route path="/" element={<Layout />}>
              <Route index element={setupDone ? <Dashboard /> : <Navigate to="/setup" replace />} />
              <Route path="settings" element={<Settings />} />
              <Route path="logs" element={<Logs />} />
              <Route path="party" element={<PartyCentral />} />
            </Route>
          </Routes>
        </BrowserRouter>
      </ToastProvider>
    </AppProvider>
  );
}

export default App;
