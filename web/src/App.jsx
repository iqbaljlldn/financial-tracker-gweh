import React, { useState, useEffect } from 'react';
import ErrorBoundary from './components/ErrorBoundary';
import { AuthProvider, useAuth } from './context/AuthContext';
import LandingPage from './components/LandingPage';
import AppShell from './components/AppShell';
import OAuthCallback from './components/OAuthCallback';

function MainRouter() {
  const { isAuthenticated, loading } = useAuth();
  const [isHandlingCallback, setIsHandlingCallback] = useState(false);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if ((params.has('code') && params.has('state')) || params.has('error')) {
      setIsHandlingCallback(true);
    }
  }, []);

  if (isHandlingCallback) {
    return (
      <OAuthCallback
        onComplete={() => {
          setIsHandlingCallback(false);
        }}
        onCancel={() => {
          window.history.replaceState({}, document.title, window.location.pathname);
          setIsHandlingCallback(false);
        }}
      />
    );
  }

  if (loading) {
    return (
      <div
        style={{
          minHeight: '100vh',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          background: 'var(--bg-primary)',
          gap: '16px',
        }}
      >
        <div
          className="spin"
          style={{
            width: '40px',
            height: '40px',
            border: '3px solid rgba(255, 255, 255, 0.1)',
            borderTopColor: '#10b981',
            borderRadius: '50%',
          }}
        />
        <div style={{ color: 'var(--text-secondary)', fontSize: '14px', fontFamily: 'var(--font-display)' }}>
          Loading FinGweh Platform...
        </div>
      </div>
    );
  }

  if (isAuthenticated) {
    return <AppShell />;
  }

  return <LandingPage />;
}

export default function App() {
  return (
    <ErrorBoundary>
      <AuthProvider>
        <MainRouter />
      </AuthProvider>
    </ErrorBoundary>
  );
}
