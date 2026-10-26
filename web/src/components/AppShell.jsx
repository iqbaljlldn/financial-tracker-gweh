import React, { useState } from 'react';
import {
  Shield,
  Lock,
  Mail,
  Settings,
  LogOut,
  TrendingUp,
  FileText,
  PieChart,
  Activity,
  CheckCircle2,
  Clock,
  Sparkles,
  Database,
  ExternalLink,
  RefreshCw,
  Cpu,
} from 'lucide-react';
import { useAuth } from '../context/AuthContext';
import SettingsDrawer from './SettingsDrawer';
import GmailAccountsView from './GmailAccountsView';
import { ParserMonitoringView } from './ParserMonitoringView';
import { LedgerView } from './LedgerView';
import { formatMoney } from '../utils/money';

export default function AppShell() {
  const { user, logout, refreshProfile } = useAuth();
  const [activeTab, setActiveTab] = useState('overview');
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [isRefreshing, setIsRefreshing] = useState(false);

  const handleRefresh = async () => {
    try {
      setIsRefreshing(true);
      await refreshProfile();
    } finally {
      setTimeout(() => setIsRefreshing(false), 500);
    }
  };

  return (
    <div style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column' }}>
      {/* Top Navbar */}
      <header
        style={{
          padding: '16px 32px',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          borderBottom: '1px solid var(--border-subtle)',
          background: 'rgba(7, 10, 19, 0.85)',
          backdropFilter: 'blur(16px)',
          position: 'sticky',
          top: 0,
          zIndex: 30,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '20px' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <img src="/logo.svg" alt="FinGweh" style={{ width: '32px', height: '32px' }} />
            <span
              style={{
                fontFamily: 'var(--font-display)',
                fontSize: '19px',
                fontWeight: 800,
                letterSpacing: '-0.02em',
              }}
            >
              FinGweh
            </span>
          </div>

          <div
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: '6px',
              padding: '4px 12px',
              background: 'rgba(16, 185, 129, 0.1)',
              border: '1px solid rgba(16, 185, 129, 0.25)',
              borderRadius: 'var(--radius-full)',
              color: '#34d399',
              fontSize: '11px',
              fontWeight: 600,
            }}
          >
            <Shield size={12} />
            <span>KMS Envelope Encrypted</span>
          </div>
        </div>

        {/* User Profile & Actions */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
          <button
            onClick={handleRefresh}
            className="btn-secondary"
            style={{ padding: '8px 12px', fontSize: '13px' }}
            title="Refresh session data"
          >
            <RefreshCw size={14} className={isRefreshing ? 'spin' : ''} />
            <span>Sync</span>
          </button>

          <button
            onClick={() => setIsSettingsOpen(true)}
            className="btn-secondary"
            style={{ padding: '8px 12px', fontSize: '13px' }}
            title="Open Settings"
          >
            <Settings size={14} />
            <span>Settings</span>
          </button>

          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '10px',
              padding: '6px 12px',
              background: 'rgba(255, 255, 255, 0.04)',
              border: '1px solid var(--border-subtle)',
              borderRadius: 'var(--radius-sm)',
            }}
          >
            <div
              style={{
                width: '28px',
                height: '28px',
                borderRadius: '50%',
                background: 'linear-gradient(135deg, #10b981 0%, #06b6d4 100%)',
                color: '#070a13',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontWeight: 700,
                fontSize: '13px',
              }}
            >
              {user?.displayName ? user.displayName[0].toUpperCase() : 'U'}
            </div>
            <div style={{ fontSize: '13px', lineHeight: 1.2 }}>
              <div style={{ fontWeight: 600 }}>{user?.displayName || 'User'}</div>
              <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>{user?.email}</div>
            </div>
          </div>

          <button
            onClick={logout}
            className="btn-secondary"
            style={{ padding: '8px 12px', color: '#fb7185', borderColor: 'rgba(244, 63, 94, 0.3)' }}
            title="Sign out"
          >
            <LogOut size={14} />
          </button>
        </div>
      </header>

      {/* Main Container */}
      <div style={{ flex: 1, display: 'flex', maxWidth: '1440px', width: '100%', margin: '0 auto' }}>
        {/* Sidebar Nav */}
        <nav
          style={{
            width: '240px',
            borderRight: '1px solid var(--border-subtle)',
            padding: '24px 16px',
            display: 'flex',
            flexDirection: 'column',
            gap: '6px',
          }}
        >
          <button
            onClick={() => setActiveTab('overview')}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '10px',
              padding: '10px 14px',
              borderRadius: 'var(--radius-sm)',
              border: 'none',
              background: activeTab === 'overview' ? 'rgba(16, 185, 129, 0.15)' : 'transparent',
              color: activeTab === 'overview' ? '#34d399' : 'var(--text-secondary)',
              fontWeight: activeTab === 'overview' ? 600 : 500,
              fontSize: '14px',
              cursor: 'pointer',
              textAlign: 'left',
              transition: 'all 0.15s ease',
            }}
          >
            <Activity size={16} /> Overview
          </button>

          <button
            onClick={() => setActiveTab('mailboxes')}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '10px',
              padding: '10px 14px',
              borderRadius: 'var(--radius-sm)',
              border: 'none',
              background: activeTab === 'mailboxes' ? 'rgba(16, 185, 129, 0.15)' : 'transparent',
              color: activeTab === 'mailboxes' ? '#34d399' : 'var(--text-secondary)',
              fontWeight: activeTab === 'mailboxes' ? 600 : 500,
              fontSize: '14px',
              cursor: 'pointer',
              textAlign: 'left',
              transition: 'all 0.15s ease',
            }}
          >
            <Mail size={16} /> Gmail Accounts
          </button>

          <button
            onClick={() => setActiveTab('ledger')}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '10px',
              padding: '10px 14px',
              borderRadius: 'var(--radius-sm)',
              border: 'none',
              background: activeTab === 'ledger' ? 'rgba(16, 185, 129, 0.15)' : 'transparent',
              color: activeTab === 'ledger' ? '#34d399' : 'var(--text-secondary)',
              fontWeight: activeTab === 'ledger' ? 600 : 500,
              fontSize: '14px',
              cursor: 'pointer',
              textAlign: 'left',
              transition: 'all 0.15s ease',
            }}
          >
            <FileText size={16} /> Ledger Events
          </button>

          <button
            onClick={() => setActiveTab('analytics')}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '10px',
              padding: '10px 14px',
              borderRadius: 'var(--radius-sm)',
              border: 'none',
              background: activeTab === 'analytics' ? 'rgba(16, 185, 129, 0.15)' : 'transparent',
              color: activeTab === 'analytics' ? '#34d399' : 'var(--text-secondary)',
              fontWeight: activeTab === 'analytics' ? 600 : 500,
              fontSize: '14px',
              cursor: 'pointer',
              textAlign: 'left',
              transition: 'all 0.15s ease',
            }}
          >
            <PieChart size={16} /> Analytics (CQRS)
          </button>

          <button
            onClick={() => setActiveTab('audit')}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '10px',
              padding: '10px 14px',
              borderRadius: 'var(--radius-sm)',
              border: 'none',
              background: activeTab === 'audit' ? 'rgba(16, 185, 129, 0.15)' : 'transparent',
              color: activeTab === 'audit' ? '#34d399' : 'var(--text-secondary)',
              fontWeight: activeTab === 'audit' ? 600 : 500,
              fontSize: '14px',
              cursor: 'pointer',
              textAlign: 'left',
              transition: 'all 0.15s ease',
            }}
          >
            <Database size={16} /> Audit Trail
          </button>

          <button
            onClick={() => setActiveTab('parser')}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '10px',
              padding: '10px 14px',
              borderRadius: 'var(--radius-sm)',
              border: 'none',
              background: activeTab === 'parser' ? 'rgba(16, 185, 129, 0.15)' : 'transparent',
              color: activeTab === 'parser' ? '#34d399' : 'var(--text-secondary)',
              fontWeight: activeTab === 'parser' ? 600 : 500,
              fontSize: '14px',
              cursor: 'pointer',
              textAlign: 'left',
              transition: 'all 0.15s ease',
            }}
          >
            <Cpu size={16} /> Parser Telemetry
          </button>
        </nav>

        {/* Tab Content Area */}
        <main style={{ flex: 1, padding: '32px', overflowY: 'auto' }}>
          {activeTab === 'overview' && (
            <div>
              <div style={{ marginBottom: '28px' }}>
                <h1 style={{ fontSize: '28px', marginBottom: '8px' }}>Welcome, {user?.displayName}</h1>
                <p style={{ color: 'var(--text-secondary)' }}>
                  Platform Session Active • User ID: <code style={{ fontFamily: 'var(--font-mono)', color: '#38bdf8' }}>{user?.id}</code>
                </p>
              </div>

              {/* Status Metric Cards */}
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))',
                  gap: '20px',
                  marginBottom: '32px',
                }}
              >
                <div className="glass-card" style={{ padding: '20px' }}>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '12px' }}>
                    <span style={{ fontSize: '13px', color: 'var(--text-secondary)' }}>Identity Status</span>
                    <span className="badge badge-emerald">Authenticated</span>
                  </div>
                  <div style={{ fontSize: '20px', fontWeight: 700 }}>Google OAuth PKCE</div>
                  <div style={{ fontSize: '12px', color: 'var(--text-muted)', marginTop: '4px' }}>
                    1-Hour JWT Session + 30-Day Refresh Token
                  </div>
                </div>

                <div className="glass-card" style={{ padding: '20px' }}>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '12px' }}>
                    <span style={{ fontSize: '13px', color: 'var(--text-secondary)' }}>Encryption Key</span>
                    <span className="badge badge-cyan">KMS Active</span>
                  </div>
                  <div style={{ fontSize: '20px', fontWeight: 700 }}>AES-256-GCM Envelope</div>
                  <div style={{ fontSize: '12px', color: 'var(--text-muted)', marginTop: '4px' }}>
                    Per-token wrapped DEKs at rest in DB
                  </div>
                </div>

                <div className="glass-card" style={{ padding: '20px' }}>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '12px' }}>
                    <span style={{ fontSize: '13px', color: 'var(--text-secondary)' }}>Timezone Preference</span>
                    <span className="badge badge-emerald">{user?.timezone || 'UTC'}</span>
                  </div>
                  <div style={{ fontSize: '20px', fontWeight: 700 }}>Daily Cutoff at 00:00</div>
                  <div style={{ fontSize: '12px', color: 'var(--text-muted)', marginTop: '4px' }}>
                    Notification target: {user?.notificationDailyTime || '07:30'}
                  </div>
                </div>
              </div>

              {/* Foundation Roadmap Showcase */}
              <div className="glass-panel" style={{ padding: '28px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '16px' }}>
                  <Sparkles size={20} color="#10b981" />
                  <h2 style={{ fontSize: '18px' }}>Sprint 1 Foundation Status & Roadmap</h2>
                </div>

                <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: '16px' }}>
                  <div style={{ padding: '16px', background: 'rgba(16, 185, 129, 0.08)', borderRadius: 'var(--radius-sm)', border: '1px solid rgba(16, 185, 129, 0.2)' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: '#34d399', fontWeight: 600, fontSize: '14px', marginBottom: '6px' }}>
                      <CheckCircle2 size={16} /> Sprint 1: Identity & DB
                    </div>
                    <p style={{ fontSize: '13px', color: 'var(--text-secondary)' }}>
                      All 8 Postgres schemas migrated, zero cross-schema foreign keys, KMS envelope encryption, clean architecture AST enforced.
                    </p>
                  </div>

                  <div style={{ padding: '16px', background: 'rgba(255, 255, 255, 0.02)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border-subtle)' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: 'var(--text-primary)', fontWeight: 600, fontSize: '14px', marginBottom: '6px' }}>
                      <Clock size={16} color="#06b6d4" /> Sprint 2: Ingestion & Poller
                    </div>
                    <p style={{ fontSize: '13px', color: 'var(--text-secondary)' }}>
                      Pub/Sub webhook handler, incremental sync poller, and raw receipt storage in <code>ingestion.ingested_messages</code>.
                    </p>
                  </div>

                  <div style={{ padding: '16px', background: 'rgba(255, 255, 255, 0.02)', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border-subtle)' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: 'var(--text-primary)', fontWeight: 600, fontSize: '14px', marginBottom: '6px' }}>
                      <Clock size={16} color="#6366f1" /> Sprint 3: Extraction & Gemini
                    </div>
                    <p style={{ fontSize: '13px', color: 'var(--text-secondary)' }}>
                      Deterministic regex/HTML parser + Gemini 2.5 Flash structured inference with confidence scores and prompt versioning.
                    </p>
                  </div>
                </div>
              </div>
            </div>
          )}

          {activeTab === 'mailboxes' && <GmailAccountsView />}

          {activeTab === 'ledger' && <LedgerView />}

          {activeTab === 'analytics' && (
            <div>
              <div style={{ marginBottom: '24px' }}>
                <h1 style={{ fontSize: '24px', marginBottom: '6px' }}>CQRS Analytics Projection</h1>
                <p style={{ color: 'var(--text-secondary)', fontSize: '14px' }}>
                  Read side backed by schema-isolated <code>analytics.*</code> tables and <code>projection_checkpoint</code>.
                </p>
              </div>

              <div className="glass-card" style={{ padding: '24px' }}>
                <p style={{ fontSize: '14px', color: 'var(--text-secondary)' }}>
                  Analytics projections will activate in Sprint 5, processing events emitted via the outbox relay.
                </p>
              </div>
            </div>
          )}

          {activeTab === 'audit' && (
            <div>
              <div style={{ marginBottom: '24px' }}>
                <h1 style={{ fontSize: '24px', marginBottom: '6px' }}>Tamper-Resistant Audit Trail</h1>
                <p style={{ color: 'var(--text-secondary)', fontSize: '14px' }}>
                  <code>administration.audit_log</code> is append-only; <code>DELETE</code> and <code>UPDATE</code> grants have been revoked from the service role.
                </p>
              </div>

              <div className="glass-card" style={{ padding: '24px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: '#10b981', fontWeight: 600, fontSize: '14px' }}>
                  <CheckCircle2 size={16} /> Immutability Security Gate Verified
                </div>
                <p style={{ fontSize: '13px', color: 'var(--text-secondary)', marginTop: '8px' }}>
                  Attempts to mutate or purge audit records will fail at the PostgreSQL catalog permission level.
                </p>
              </div>
            </div>
          )}

          {activeTab === 'parser' && <ParserMonitoringView />}
        </main>
      </div>

      <SettingsDrawer isOpen={isSettingsOpen} onClose={() => setIsSettingsOpen(false)} />
    </div>
  );
}
