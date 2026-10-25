import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { apiClient } from '../api/client';

const AuthContext = createContext(null);

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null);
  const [sessionToken, setSessionToken] = useState(apiClient.sessionToken);
  const [loading, setLoading] = useState(true);
  const [authError, setAuthError] = useState(null);

  // Load user profile on startup if session token is available
  const loadProfile = useCallback(async () => {
    if (!apiClient.isAuthenticated()) {
      setLoading(false);
      return;
    }

    try {
      setLoading(true);
      const profile = await apiClient.getMe();
      setUser(profile);
      setSessionToken(apiClient.sessionToken);
      setAuthError(null);
    } catch (err) {
      console.warn('Failed to load user profile on startup:', err);
      // If error is unauthorized and cannot be refreshed, clear session
      if (err.status === 401) {
        apiClient.clearSession();
        setUser(null);
        setSessionToken(null);
      }
      setAuthError(err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadProfile();

    const handleAuthExpired = () => {
      setUser(null);
      setSessionToken(null);
      setAuthError(new Error('Your session has expired. Please sign in again.'));
    };

    window.addEventListener('auth:expired', handleAuthExpired);
    return () => window.removeEventListener('auth:expired', handleAuthExpired);
  }, [loadProfile]);

  // Initiate Google OAuth 2.0 flow
  const loginWithGoogle = async () => {
    try {
      setLoading(true);
      setAuthError(null);
      const data = await apiClient.initiateGoogleAuth();
      if (!data.authorizationUrl) {
        throw new Error('Server returned invalid authorization URL');
      }

      // Store CSRF state token to verify upon callback
      if (data.state) {
        sessionStorage.setItem('oauth_state', data.state);
      }

      // Redirect browser to Google's consent screen
      window.location.href = data.authorizationUrl;
    } catch (err) {
      setLoading(false);
      setAuthError(err);
      throw err;
    }
  };

  // Complete OAuth token exchange after redirect back from Google
  const handleOAuthCallback = async (code, state) => {
    try {
      setLoading(true);
      setAuthError(null);

      // Verify CSRF state token
      const savedState = sessionStorage.getItem('oauth_state');
      if (savedState && savedState !== state) {
        throw new Error('OAuth CSRF state mismatch detected. Request aborted for your security.');
      }
      sessionStorage.removeItem('oauth_state');

      const data = await apiClient.googleAuthCallback(code, state);
      apiClient.setSession(data.sessionToken, data.refreshToken, data.expiresAt);
      setSessionToken(data.sessionToken);
      setUser(data.user);

      // Clean query parameters from URL
      window.history.replaceState({}, document.title, window.location.pathname);
      return data.user;
    } catch (err) {
      setAuthError(err);
      throw err;
    } finally {
      setLoading(false);
    }
  };

  // Update user profile settings
  const updateSettings = async (settings) => {
    try {
      setAuthError(null);
      const updatedUser = await apiClient.updateSettings(settings);
      setUser((prev) => ({ ...prev, ...updatedUser }));
      return updatedUser;
    } catch (err) {
      setAuthError(err);
      throw err;
    }
  };

  // Logout
  const logout = () => {
    apiClient.clearSession();
    setUser(null);
    setSessionToken(null);
    setAuthError(null);
  };

  const value = {
    user,
    sessionToken,
    isAuthenticated: Boolean(user && sessionToken),
    loading,
    authError,
    setAuthError,
    loginWithGoogle,
    handleOAuthCallback,
    updateSettings,
    refreshProfile: loadProfile,
    logout,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}
