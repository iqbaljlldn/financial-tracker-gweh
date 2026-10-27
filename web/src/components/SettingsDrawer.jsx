import React, { useState } from 'react';
import { X, Check, Globe, Clock, ShieldCheck, User, Download, FileJson } from 'lucide-react';
import { useAuth } from '../context/AuthContext';
import { apiClient } from '../api/client';
import ErrorEnvelopeDisplay from './ErrorEnvelopeDisplay';

const TIMEZONE_OPTIONS = [
  'Asia/Jakarta',
  'Asia/Singapore',
  'Asia/Tokyo',
  'UTC',
  'Europe/London',
  'America/New_York',
  'America/Los_Angeles',
];

export default function SettingsDrawer({ isOpen, onClose }) {
  const { user, updateSettings } = useAuth();
  const [timezone, setTimezone] = useState(user?.timezone || 'Asia/Jakarta');
  const [notificationDailyTime, setNotificationDailyTime] = useState(
    user?.notificationDailyTime || '07:30'
  );
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(null);
  const [savedSuccess, setSavedSuccess] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [exportSuccess, setExportSuccess] = useState(false);

  const handleExportData = async () => {
    try {
      setExporting(true);
      setError(null);
      await apiClient.downloadDataExport();
      setExportSuccess(true);
      setTimeout(() => setExportSuccess(false), 3000);
    } catch (err) {
      setError(err);
    } finally {
      setExporting(false);
    }
  };

  if (!isOpen) return null;

  const handleSave = async (e) => {
    e.preventDefault();
    try {
      setSaving(true);
      setError(null);
      setSavedSuccess(false);

      await updateSettings({
        timezone,
        notificationDailyTime,
      });

      setSavedSuccess(true);
      setTimeout(() => setSavedSuccess(false), 2500);
    } catch (err) {
      setError(err);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.7)',
        backdropFilter: 'blur(8px)',
        zIndex: 50,
        display: 'flex',
        justifyContent: 'flex-end',
      }}
      onClick={onClose}
    >
      <div
        className="glass-panel"
        style={{
          width: '100%',
          maxWidth: '460px',
          height: '100%',
          borderRadius: 0,
          borderLeft: '1px solid var(--border-subtle)',
          padding: '32px 28px',
          display: 'flex',
          flexDirection: 'column',
          overflowY: 'auto',
          animation: 'slideInRight 0.25s ease-out',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Drawer Header */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            marginBottom: '28px',
            paddingBottom: '16px',
            borderBottom: '1px solid var(--border-subtle)',
          }}
        >
          <div>
            <h2 style={{ fontSize: '20px' }}>Account & Settings</h2>
            <p style={{ fontSize: '13px', color: 'var(--text-secondary)' }}>
              Configure ledger preferences & sync rules
            </p>
          </div>
          <button
            onClick={onClose}
            style={{
              background: 'transparent',
              border: 'none',
              color: 'var(--text-muted)',
              cursor: 'pointer',
              padding: '6px',
              borderRadius: '6px',
            }}
          >
            <X size={20} />
          </button>
        </div>

        {/* Identity Overview */}
        <div className="glass-card" style={{ padding: '16px', marginBottom: '24px' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <div
              style={{
                width: '42px',
                height: '42px',
                borderRadius: '50%',
                background: 'linear-gradient(135deg, #10b981 0%, #06b6d4 100%)',
                color: '#070a13',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontWeight: 700,
                fontSize: '18px',
              }}
            >
              {user?.displayName ? user.displayName[0].toUpperCase() : 'U'}
            </div>
            <div style={{ flex: 1, minWidth: 0 }}>
              <div style={{ fontWeight: 600, fontSize: '15px' }}>{user?.displayName || 'User'}</div>
              <div
                style={{
                  fontSize: '13px',
                  color: 'var(--text-muted)',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
              >
                {user?.email}
              </div>
            </div>
            <span className="badge badge-emerald">Verified</span>
          </div>

          <div
            style={{
              marginTop: '12px',
              paddingTop: '10px',
              borderTop: '1px solid var(--border-subtle)',
              fontSize: '11px',
              color: 'var(--text-muted)',
              fontFamily: 'var(--font-mono)',
            }}
          >
            User ID: {user?.id}
          </div>
        </div>

        {error && <ErrorEnvelopeDisplay error={error} onDismiss={() => setError(null)} />}

        {savedSuccess && (
          <div
            style={{
              padding: '12px 16px',
              background: 'rgba(16, 185, 129, 0.15)',
              border: '1px solid rgba(16, 185, 129, 0.3)',
              borderRadius: 'var(--radius-sm)',
              color: '#34d399',
              fontSize: '13px',
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              marginBottom: '20px',
            }}
          >
            <Check size={16} />
            <span>Settings saved successfully via PATCH /me/settings</span>
          </div>
        )}

        {/* Settings Form */}
        <form onSubmit={handleSave} style={{ display: 'flex', flexDirection: 'column', gap: '20px', flex: 1 }}>
          <div className="form-group">
            <label className="form-label" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Globe size={15} color="#06b6d4" />
              <span>Timezone (Reporting & Ledger)</span>
            </label>
            <select
              value={timezone}
              onChange={(e) => setTimezone(e.target.value)}
              className="form-select"
            >
              {TIMEZONE_OPTIONS.map((tz) => (
                <option key={tz} value={tz}>
                  {tz}
                </option>
              ))}
            </select>
            <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
              Determines daily bucket cutoffs for cash-flow analytics and summary jobs.
            </span>
          </div>

          <div className="form-group">
            <label className="form-label" style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
              <Clock size={15} color="#10b981" />
              <span>Daily Summary Notification Time</span>
            </label>
            <input
              type="time"
              value={notificationDailyTime}
              onChange={(e) => setNotificationDailyTime(e.target.value)}
              className="form-input"
            />
            <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
              Time in selected timezone when engagement notifications trigger.
            </span>
          </div>

          <div className="glass-card" style={{ padding: '14px', marginTop: '16px' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: '#10b981', fontSize: '13px', fontWeight: 600 }}>
                <FileJson size={16} />
                <span>GDPR Data Portability (Art. 15/20)</span>
              </div>
            </div>
            <p style={{ fontSize: '11px', color: 'var(--text-muted)', marginBottom: '12px', lineHeight: 1.4 }}>
              Download a complete archive of your financial ledger, budgets, overrides, and account metadata. Raw email content is excluded per Tier 4 minimization.
            </p>
            <button
              type="button"
              onClick={handleExportData}
              disabled={exporting}
              className="btn-secondary"
              style={{ width: '100%', fontSize: '12px', padding: '8px 12px', justifyContent: 'center' }}
            >
              {exporting ? (
                <>
                  <div className="spin" style={{ width: '14px', height: '14px', border: '2px solid rgba(255,255,255,0.3)', borderTopColor: '#fff', borderRadius: '50%' }}></div>
                  <span>Compiling Export...</span>
                </>
              ) : exportSuccess ? (
                <>
                  <Check size={14} color="#34d399" />
                  <span style={{ color: '#34d399' }}>Export Downloaded!</span>
                </>
              ) : (
                <>
                  <Download size={14} />
                  <span>Download My Data (JSON)</span>
                </>
              )}
            </button>
          </div>

          <div className="glass-card" style={{ padding: '14px', marginTop: '12px' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', color: '#38bdf8', fontSize: '12px', fontWeight: 600 }}>
              <ShieldCheck size={16} />
              <span>Row-Level Ownership Enforced</span>
            </div>
            <p style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '4px', lineHeight: 1.4 }}>
              Every settings query and update requires mandatory <code>userID</code> parameter filtering at the database repository layer.
            </p>
          </div>

          <button
            type="submit"
            disabled={saving}
            className="btn-primary"
            style={{ width: '100%', marginTop: '16px' }}
          >
            {saving ? (
              <>
                <div className="spin" style={{ width: '16px', height: '16px', border: '2px solid rgba(7,10,19,0.3)', borderTopColor: '#070a13', borderRadius: '50%' }}></div>
                <span>Saving Changes...</span>
              </>
            ) : (
              <>
                <Check size={16} /> Save Settings
              </>
            )}
          </button>
        </form>
      </div>
    </div>
  );
}
