import React, { useState, useEffect, useRef } from 'react';
import {
  Mail,
  RefreshCw,
  Plus,
  CheckCircle2,
  AlertCircle,
  Clock,
  ExternalLink,
  ShieldCheck,
  AlertTriangle,
  ArrowRight,
  Database,
  Search,
} from 'lucide-react';
import { apiClient } from '../api/client';
import { useAuth } from '../context/AuthContext';

export default function GmailAccountsView() {
  const { user } = useAuth();
  const [accounts, setAccounts] = useState([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(null);
  const [isLinkingOpen, setIsLinkingOpen] = useState(false);
  const [newEmail, setNewEmail] = useState('');
  const [syncDays, setSyncDays] = useState(90);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [syncingAccountId, setSyncingAccountId] = useState(null);
  const [funnelStage, setFunnelStage] = useState('ready'); // ready, linking, syncing, complete

  const pollTimerRef = useRef(null);

  const fetchAccounts = async () => {
    try {
      setError(null);
      const data = await apiClient.listGmailAccounts();
      setAccounts(data || []);

      // Check if any account is currently in initial_sync
      const hasSyncing = (data || []).some((a) => a.status === 'initial_sync' || a.status === 'linking');
      if (hasSyncing) {
        setFunnelStage('syncing');
      } else if ((data || []).some((a) => a.status === 'active')) {
        setFunnelStage('complete');
      }
    } catch (err) {
      setError(err.message || 'Failed to fetch accounts');
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchAccounts();

    // Auto-poll if any account is in initial_sync or linking
    pollTimerRef.current = setInterval(() => {
      apiClient.listGmailAccounts().then((data) => {
        if (data) {
          setAccounts(data);
          const hasSyncing = data.some((a) => a.status === 'initial_sync' || a.status === 'linking');
          if (!hasSyncing && data.some((a) => a.status === 'active')) {
            setFunnelStage('complete');
          }
        }
      }).catch(() => {});
    }, 3000);

    return () => {
      if (pollTimerRef.current) clearInterval(pollTimerRef.current);
    };
  }, []);

  const handleLinkDirect = async (e) => {
    e.preventDefault();
    if (!newEmail.trim()) return;

    try {
      setIsSubmitting(true);
      setError(null);
      setFunnelStage('linking');

      await apiClient.linkGmailAccount(newEmail.trim(), Number(syncDays) || 90);
      setNewEmail('');
      setIsLinkingOpen(false);
      await fetchAccounts();
    } catch (err) {
      if (err.status === 409 || err.code === 'EMAIL_ALREADY_LINKED') {
        setError('409 Conflict: This email address is already linked to an account [GATE].');
      } else {
        setError(err.message || 'Failed to link account');
      }
      setFunnelStage('ready');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleConnectOAuth = async () => {
    try {
      setIsSubmitting(true);
      setError(null);
      const res = await apiClient.initiateGoogleAuth();
      if (res?.authorizationUrl) {
        setFunnelStage('linking');
        window.location.href = res.authorizationUrl;
      }
    } catch (err) {
      setError(err.message || 'Failed to initiate Google OAuth');
      setIsSubmitting(false);
    }
  };

  const handleTriggerSync = async (accountId) => {
    try {
      setSyncingAccountId(accountId);
      setError(null);
      await apiClient.triggerGmailSync(accountId);
      await fetchAccounts();
    } catch (err) {
      setError(err.message || 'Failed to trigger sync');
    } finally {
      setSyncingAccountId(null);
    }
  };

  const getStatusBadge = (status) => {
    switch (status) {
      case 'active':
        return (
          <span className="badge badge-emerald" style={{ display: 'inline-flex', alignItems: 'center', gap: '5px' }}>
            <CheckCircle2 size={12} /> Active • Synchronized
          </span>
        );
      case 'initial_sync':
        return (
          <span className="badge badge-cyan" style={{ display: 'inline-flex', alignItems: 'center', gap: '5px' }}>
            <RefreshCw size={12} className="spin" /> Syncing (90-Day Lookback)...
          </span>
        );
      case 'linking':
        return (
          <span className="badge badge-amber" style={{ display: 'inline-flex', alignItems: 'center', gap: '5px' }}>
            <Clock size={12} /> Linking...
          </span>
        );
      case 'disconnected':
        return (
          <span className="badge" style={{ background: 'rgba(244, 63, 94, 0.15)', color: '#fb7185', border: '1px solid rgba(244, 63, 94, 0.3)' }}>
            <AlertCircle size={12} /> Consent Revoked
          </span>
        );
      default:
        return (
          <span className="badge" style={{ background: 'rgba(239, 68, 68, 0.15)', color: '#f87171' }}>
            {status}
          </span>
        );
    }
  };

  return (
    <div>
      {/* Top Header */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '24px', flexWrap: 'wrap', gap: '16px' }}>
        <div>
          <h1 style={{ fontSize: '26px', marginBottom: '6px' }}>Gmail Account Onboarding</h1>
          <p style={{ color: 'var(--text-secondary)', fontSize: '14px' }}>
            Standalone Ingestion Module • Read-only mailbox connector with 90-day lookback bounded sync
          </p>
        </div>

        <div style={{ display: 'flex', gap: '10px' }}>
          <button
            onClick={fetchAccounts}
            className="btn-secondary"
            style={{ padding: '8px 14px', fontSize: '13px' }}
            title="Refresh list"
          >
            <RefreshCw size={14} className={isLoading ? 'spin' : ''} />
            <span>Refresh</span>
          </button>

          <button
            onClick={() => setIsLinkingOpen(!isLinkingOpen)}
            className="btn-primary"
            style={{ padding: '8px 16px', fontSize: '13px' }}
          >
            <Plus size={15} />
            <span>Connect Gmail</span>
          </button>
        </div>
      </div>

      {/* Onboarding Funnel Progress Card */}
      <div
        className="glass-card"
        style={{
          padding: '16px 20px',
          marginBottom: '24px',
          background: 'linear-gradient(90deg, rgba(16, 185, 129, 0.08) 0%, rgba(6, 182, 212, 0.05) 100%)',
          border: '1px solid rgba(16, 185, 129, 0.2)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '12px' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', fontSize: '13px', fontWeight: 600 }}>
            <ShieldCheck size={18} color="#34d399" />
            <span>Onboarding Pipeline Funnel:</span>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '16px', fontSize: '12px' }}>
            <span style={{ color: accounts.length > 0 ? '#34d399' : 'var(--text-muted)', display: 'flex', alignItems: 'center', gap: '4px' }}>
              <CheckCircle2 size={13} /> 1. Account Linked
            </span>
            <ArrowRight size={12} color="var(--text-muted)" />
            <span style={{ color: funnelStage === 'syncing' || funnelStage === 'complete' ? '#38bdf8' : 'var(--text-muted)', display: 'flex', alignItems: 'center', gap: '4px' }}>
              <Clock size={13} /> 2. 90-Day Full Sync
            </span>
            <ArrowRight size={12} color="var(--text-muted)" />
            <span style={{ color: funnelStage === 'complete' ? '#34d399' : 'var(--text-muted)', display: 'flex', alignItems: 'center', gap: '4px' }}>
              <CheckCircle2 size={13} /> 3. Realtime Active
            </span>
          </div>
        </div>
      </div>

      {/* Error Alert */}
      {error && (
        <div
          style={{
            padding: '12px 16px',
            marginBottom: '20px',
            background: 'rgba(244, 63, 94, 0.1)',
            border: '1px solid rgba(244, 63, 94, 0.3)',
            borderRadius: 'var(--radius-sm)',
            color: '#fb7185',
            fontSize: '13px',
            display: 'flex',
            alignItems: 'center',
            gap: '10px',
          }}
        >
          <AlertCircle size={16} />
          <span>{error}</span>
        </div>
      )}

      {/* Connect Account Form Modal / Accordion */}
      {isLinkingOpen && (
        <div className="glass-panel" style={{ padding: '24px', marginBottom: '28px', border: '1px solid var(--border-active)' }}>
          <h3 style={{ fontSize: '16px', marginBottom: '8px' }}>Link a Gmail Account</h3>
          <p style={{ color: 'var(--text-secondary)', fontSize: '13px', marginBottom: '20px' }}>
            Choose whether to link your primary authenticated address or enter a secondary Gmail mailbox.
          </p>

          <form onSubmit={handleLinkDirect} style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: '16px' }}>
              <div>
                <label style={{ display: 'block', fontSize: '12px', fontWeight: 600, color: 'var(--text-secondary)', marginBottom: '6px' }}>
                  Gmail Address
                </label>
                <input
                  type="email"
                  value={newEmail}
                  onChange={(e) => setNewEmail(e.target.value)}
                  placeholder="e.g. user@gmail.com"
                  className="input-field"
                  style={{ width: '100%' }}
                  required
                />
              </div>

              <div>
                <label style={{ display: 'block', fontSize: '12px', fontWeight: 600, color: 'var(--text-secondary)', marginBottom: '6px' }}>
                  Lookback Window (Days)
                </label>
                <select
                  value={syncDays}
                  onChange={(e) => setSyncDays(e.target.value)}
                  className="input-field"
                  style={{ width: '100%' }}
                >
                  <option value={30}>30 Days (Quick)</option>
                  <option value={90}>90 Days (Recommended / Default)</option>
                  <option value={180}>180 Days (Extended)</option>
                </select>
              </div>
            </div>

            <div style={{ display: 'flex', alignItems: 'center', gap: '12px', marginTop: '8px' }}>
              <button
                type="submit"
                disabled={isSubmitting}
                className="btn-primary"
                style={{ padding: '10px 20px', fontSize: '14px' }}
              >
                {isSubmitting ? 'Linking...' : 'Link Mailbox'}
              </button>

              <button
                type="button"
                onClick={handleConnectOAuth}
                disabled={isSubmitting}
                className="btn-secondary"
                style={{ padding: '10px 18px', fontSize: '14px' }}
              >
                <ExternalLink size={14} />
                <span>Authorize via Google OAuth</span>
              </button>

              <button
                type="button"
                onClick={() => setIsLinkingOpen(false)}
                className="btn-secondary"
                style={{ padding: '10px 14px', fontSize: '14px' }}
              >
                Cancel
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Account List View */}
      {accounts.length === 0 && !isLoading ? (
        /* Empty-state messaging for a newly-linked account with zero mailboxes */
        <div
          className="glass-panel"
          style={{
            padding: '48px 24px',
            textAlign: 'center',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          <div
            style={{
              width: '64px',
              height: '64px',
              borderRadius: '50%',
              background: 'rgba(16, 185, 129, 0.1)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              marginBottom: '16px',
              color: '#34d399',
            }}
          >
            <Mail size={32} />
          </div>
          <h3 style={{ fontSize: '18px', marginBottom: '8px' }}>No Mailboxes Connected Yet</h3>
          <p style={{ color: 'var(--text-secondary)', maxWidth: '440px', fontSize: '14px', marginBottom: '24px' }}>
            Link your Gmail mailbox to enable automated transaction extraction. We request least-privilege <code>gmail.readonly</code> access and never store raw passwords.
          </p>
          <button
            onClick={() => setIsLinkingOpen(true)}
            className="btn-primary"
            style={{ padding: '12px 24px', fontSize: '14px' }}
          >
            <Plus size={16} />
            <span>Connect Primary Mailbox</span>
          </button>
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
          {accounts.map((acc) => {
            const isSyncing = acc.status === 'initial_sync' || syncingAccountId === acc.id;

            return (
              <div key={acc.id} className="glass-card" style={{ padding: '24px' }}>
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '16px' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
                    <div
                      style={{
                        width: '48px',
                        height: '48px',
                        borderRadius: '12px',
                        background: 'rgba(66, 133, 244, 0.15)',
                        color: '#4285F4',
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                      }}
                    >
                      <Mail size={24} />
                    </div>

                    <div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '10px', flexWrap: 'wrap' }}>
                        <span style={{ fontSize: '16px', fontWeight: 600 }}>{acc.emailAddress}</span>
                        {getStatusBadge(acc.status)}
                      </div>

                      <div style={{ display: 'flex', gap: '16px', fontSize: '12px', color: 'var(--text-muted)', marginTop: '6px', flexWrap: 'wrap' }}>
                        <span>
                          Account ID: <code style={{ color: 'var(--text-secondary)' }}>{acc.id.slice(0, 8)}...</code>
                        </span>
                        <span>Lookback: <strong>{acc.initialSyncWindowDays || 90} Days</strong></span>
                        {acc.lastHistoryId && (
                          <span>
                            Cursor: <code style={{ color: '#38bdf8' }}>{acc.lastHistoryId}</code>
                          </span>
                        )}
                        <span>Linked: {new Date(acc.createdAt).toLocaleDateString()}</span>
                      </div>
                    </div>
                  </div>

                  <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
                    <button
                      onClick={() => handleTriggerSync(acc.id)}
                      disabled={isSyncing}
                      className="btn-secondary"
                      style={{ padding: '8px 14px', fontSize: '13px' }}
                      title="Run manual synchronization"
                    >
                      <RefreshCw size={13} className={isSyncing ? 'spin' : ''} />
                      <span>{isSyncing ? 'Syncing...' : 'Sync Now'}</span>
                    </button>
                  </div>
                </div>

                {/* Syncing Progress Indication Bar */}
                {acc.status === 'initial_sync' && (
                  <div style={{ marginTop: '16px', paddingTop: '16px', borderTop: '1px solid var(--border-subtle)' }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '12px', color: '#38bdf8', marginBottom: '6px' }}>
                      <span style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
                        <RefreshCw size={12} className="spin" />
                        Enumerating historical messages via Gmail API (lookback: {acc.initialSyncWindowDays} days)...
                      </span>
                      <span>Polling status...</span>
                    </div>
                    <div style={{ height: '4px', width: '100%', background: 'rgba(6, 182, 212, 0.15)', borderRadius: '2px', overflow: 'hidden' }}>
                      <div
                        style={{
                          height: '100%',
                          width: '65%',
                          background: 'linear-gradient(90deg, #06b6d4, #10b981)',
                          borderRadius: '2px',
                          animation: 'pulse 1.5s infinite',
                        }}
                      />
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
