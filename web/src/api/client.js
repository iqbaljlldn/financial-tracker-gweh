/**
 * API client wrapper per 09_api_specification.md §5 & §7.2.
 * - Handles Authorization: Bearer <sessionToken>
 * - Manages session token and refresh token storage
 * - Proactive and reactive silent session refresh
 * - Standardized API error envelopes
 */

const API_BASE = '/api/v1';

class ApiClient {
  constructor() {
    this.sessionToken = localStorage.getItem('session_token') || null;
    this.refreshToken = localStorage.getItem('refresh_token') || null;
    this.tokenExpiresAt = localStorage.getItem('token_expires_at') || null;
    this.isRefreshing = false;
    this.refreshSubscribers = [];
  }

  setSession(sessionToken, refreshToken, expiresAt) {
    this.sessionToken = sessionToken;
    this.refreshToken = refreshToken || this.refreshToken;
    this.tokenExpiresAt = expiresAt;

    if (sessionToken) {
      localStorage.setItem('session_token', sessionToken);
    } else {
      localStorage.removeItem('session_token');
    }

    if (refreshToken) {
      localStorage.setItem('refresh_token', refreshToken);
    } else if (refreshToken === null) {
      localStorage.removeItem('refresh_token');
    }

    if (expiresAt) {
      localStorage.setItem('token_expires_at', expiresAt);
    } else {
      localStorage.removeItem('token_expires_at');
    }
  }

  clearSession() {
    this.setSession(null, null, null);
  }

  isAuthenticated() {
    return Boolean(this.sessionToken);
  }

  onRefreshed(token) {
    this.refreshSubscribers.forEach((cb) => cb(token));
    this.refreshSubscribers = [];
  }

  addRefreshSubscriber(cb) {
    this.refreshSubscribers.push(cb);
  }

  async refreshTokens() {
    if (!this.refreshToken) {
      this.clearSession();
      throw new Error('No refresh token available');
    }

    try {
      const resp = await fetch(`${API_BASE}/auth/refresh`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refreshToken: this.refreshToken }),
      });

      if (!resp.ok) {
        const errPayload = await resp.json().catch(() => ({}));
        this.clearSession();
        throw new Error(errPayload?.error?.message || 'Session refresh failed');
      }

      const data = await resp.json();
      this.setSession(data.sessionToken, data.refreshToken, data.expiresAt);
      return data.sessionToken;
    } catch (err) {
      this.clearSession();
      throw err;
    }
  }

  async request(endpoint, options = {}) {
    const url = endpoint.startsWith('http') ? endpoint : `${API_BASE}${endpoint}`;
    const headers = {
      'Content-Type': 'application/json',
      ...options.headers,
    };

    if (this.sessionToken) {
      headers['Authorization'] = `Bearer ${this.sessionToken}`;
    }

    let response = await fetch(url, { ...options, headers });

    // Handle token expiration & automatic silent refresh
    if (response.status === 401 && this.refreshToken && !endpoint.includes('/auth/refresh')) {
      if (!this.isRefreshing) {
        this.isRefreshing = true;
        try {
          const newToken = await this.refreshTokens();
          this.isRefreshing = false;
          this.onRefreshed(newToken);
        } catch (refreshErr) {
          this.isRefreshing = false;
          this.refreshSubscribers = [];
          // Trigger auth error event for app redirection
          window.dispatchEvent(new CustomEvent('auth:expired', { detail: refreshErr }));
          throw refreshErr;
        }
      }

      // Wait for refresh to complete if parallel requests occur
      const retryPromise = new Promise((resolve) => {
        this.addRefreshSubscriber(async (newToken) => {
          headers['Authorization'] = `Bearer ${newToken}`;
          const retryResp = await fetch(url, { ...options, headers });
          resolve(retryResp);
        });
      });

      response = await retryPromise;
    }

    // Process Response
    const contentType = response.headers.get('content-type') || '';
    let payload = null;
    if (contentType.includes('application/json')) {
      payload = await response.json();
    } else {
      payload = await response.text();
    }

    if (!response.ok) {
      // Shape matches 09_api_specification.md §7.2
      const errorObj = payload?.error || {
        code: `HTTP_${response.status}`,
        message: typeof payload === 'string' ? payload : 'An unexpected error occurred',
        requestId: response.headers.get('x-request-id') || undefined,
      };

      const error = new Error(errorObj.message);
      error.status = response.status;
      error.code = errorObj.code;
      error.field = errorObj.field;
      error.requestId = errorObj.requestId;
      error.envelope = errorObj;
      throw error;
    }

    return payload;
  }

  // --- Auth Endpoints ---
  async initiateGoogleAuth() {
    return this.request('/auth/google/initiate', { method: 'POST' });
  }

  async googleAuthCallback(code, state) {
    const query = new URLSearchParams({ code, state }).toString();
    return this.request(`/auth/google/callback?${query}`, { method: 'GET' });
  }

  // --- User & Settings Endpoints ---
  async getMe() {
    return this.request('/me', { method: 'GET' });
  }

  async updateSettings(settings) {
    return this.request('/me/settings', {
      method: 'PATCH',
      body: JSON.stringify(settings),
    });
  }

  // --- Ingestion Endpoints (Sprint 2) ---
  async listGmailAccounts() {
    return this.request('/me/gmail-accounts', { method: 'GET' });
  }

  async linkGmailAccount(emailAddress, syncWindowDays = 90) {
    return this.request('/me/gmail-accounts', {
      method: 'POST',
      body: JSON.stringify({ emailAddress, syncWindowDays }),
    });
  }

  async triggerGmailSync(accountId) {
    return this.request(`/me/gmail-accounts/${accountId}/sync`, {
      method: 'POST',
    });
  }

  async disconnectGmailAccount(accountId, purge = false) {
    return this.request(`/me/gmail-accounts/${accountId}?purge=${purge}`, {
      method: 'DELETE',
    });
  }

  // --- Parser Admin Endpoints (Sprint 4) ---
  async listParserTemplates() {
    return this.request('/admin/parser/templates', { method: 'GET' });
  }

  async listParserExtractions(limit = 50) {
    return this.request(`/admin/parser/extractions?limit=${limit}`, { method: 'GET' });
  }

  async getParserMetrics() {
    return this.request('/admin/parser/metrics', { method: 'GET' });
  }

  async publishParserTemplate(payload) {
    return this.request('/admin/parser/templates', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  // --- Financial Core Ledger Endpoints (Sprint 6) ---
  async listFinancialEvents(params = {}) {
    const query = new URLSearchParams();
    if (params.limit) query.set('limit', params.limit);
    if (params.cursor) query.set('cursor', params.cursor);
    if (params.from) query.set('from', params.from);
    if (params.to) query.set('to', params.to);
    if (params.status) query.set('status', params.status);
    if (params.category) query.set('category', params.category);
    if (params.merchant) query.set('merchant', params.merchant);
    const qs = query.toString();
    return this.request(`/financial-events${qs ? `?${qs}` : ''}`, { method: 'GET' });
  }

  async getFinancialEvent(id) {
    return this.request(`/financial-events/${id}`, { method: 'GET' });
  }

  async createManualEvent(eventData) {
    return this.request('/financial-events', {
      method: 'POST',
      body: JSON.stringify(eventData),
    });
  }

  async submitEventOverride(id, fields) {
    return this.request(`/financial-events/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(fields),
    });
  }

  async confirmPendingReviewEvent(id, corrections = null, idempotencyKey = null) {
    const payload = {};
    if (corrections) payload.corrections = corrections;
    if (idempotencyKey) payload.idempotencyKey = idempotencyKey;
    return this.request(`/financial-events/${id}/confirm`, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  async listEventOverrides(id) {
    return this.request(`/financial-events/${id}/overrides`, { method: 'GET' });
  }
}

export const apiClient = new ApiClient();
