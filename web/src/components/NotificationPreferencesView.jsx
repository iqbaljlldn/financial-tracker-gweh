import React, { useState, useEffect } from 'react';
import {
  MessageSquare,
  Bell,
  Smartphone,
  ShieldCheck,
  CheckCircle2,
  AlertTriangle,
  RefreshCw,
  Send,
  Sliders,
  Check,
  Sparkles,
  Info,
  Calendar,
  DollarSign,
  PieChart,
} from 'lucide-react';
import { apiClient } from '../api/client';
import ErrorEnvelopeDisplay from './ErrorEnvelopeDisplay';

export default function NotificationPreferencesView() {
  const [preferences, setPreferences] = useState(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [savingField, setSavingField] = useState(null);
  const [phoneInput, setPhoneInput] = useState('');
  const [error, setError] = useState(null);
  const [successToast, setSuccessToast] = useState(null);
  const [previewTab, setPreviewTab] = useState('daily'); // 'daily' | 'budget' | 'review'

  const fetchPreferences = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await apiClient.getNotificationPreferences();
      setPreferences(data);
      setPhoneInput(data.phoneNumber || '');
    } catch (err) {
      console.error('Failed to load notification preferences:', err);
      setError(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchPreferences();
  }, []);

  const showToast = (message) => {
    setSuccessToast(message);
    setTimeout(() => {
      setSuccessToast(null);
    }, 3000);
  };

  const handleToggle = async (field, currentValue) => {
    if (!preferences) return;
    const newValue = !currentValue;

    // Optimistic UI update
    setPreferences((prev) => ({
      ...prev,
      [field]: newValue,
    }));
    setSavingField(field);

    try {
      const payload = { [field]: newValue };
      const updated = await apiClient.updateNotificationPreferences(payload);
      setPreferences(updated);
      showToast(`Updated ${field.replace('Enabled', '')} preference`);
    } catch (err) {
      // Rollback on failure
      setPreferences((prev) => ({
        ...prev,
        [field]: currentValue,
      }));
      setError(err);
    } finally {
      setSavingField(null);
    }
  };

  const handleSavePhone = async (e) => {
    e.preventDefault();
    try {
      setSaving(true);
      setError(null);
      const cleaned = phoneInput.trim().replace(/^0/, '62').replace(/^\+/, '');
      const updated = await apiClient.updateNotificationPreferences({
        phoneNumber: cleaned || null,
      });
      setPreferences(updated);
      setPhoneInput(updated.phoneNumber || '');
      showToast('WhatsApp phone number saved successfully!');
    } catch (err) {
      setError(err);
    } finally {
      setSaving(false);
    }
  };

  const hasPhone = Boolean(preferences?.phoneNumber);

  return (
    <div style={{ maxWidth: '1200px', margin: '0 auto', display: 'flex', flexDirection: 'column', gap: '28px' }}>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', flexWrap: 'wrap', gap: '16px' }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', marginBottom: '6px' }}>
            <h1 style={{ fontSize: '26px', fontWeight: 800 }}>WhatsApp Notifications & Engagement</h1>
            <span
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: '6px',
                padding: '4px 10px',
                background: 'rgba(37, 211, 102, 0.12)',
                border: '1px solid rgba(37, 211, 102, 0.3)',
                borderRadius: 'var(--radius-full)',
                color: '#25D366',
                fontSize: '11px',
                fontWeight: 700,
                letterSpacing: '0.04em',
              }}
            >
              <Smartphone size={13} />
              Agnostic Gateway (Fonnte / Meta / Mock)
            </span>
          </div>
          <p style={{ color: 'var(--text-secondary)', fontSize: '14px', maxWidth: '680px' }}>
            Konfigurasi ringkasan finansial berkala, alert anggaran, dan verifikasi transaksi via WhatsApp. Setiap preferensi dapat dinyalakan/dimatikan secara independen tanpa memengaruhi tipe notifikasi lain.
          </p>
        </div>

        <button
          onClick={fetchPreferences}
          disabled={loading}
          className="btn-secondary"
          style={{ padding: '8px 14px', fontSize: '13px' }}
          title="Reload Preferences"
        >
          <RefreshCw size={14} className={loading ? 'spin' : ''} />
          <span>Refresh</span>
        </button>
      </div>

      {/* Error / Success Banners */}
      {error && <ErrorEnvelopeDisplay error={error} onDismiss={() => setError(null)} />}

      {successToast && (
        <div
          style={{
            padding: '12px 18px',
            background: 'rgba(16, 185, 129, 0.15)',
            border: '1px solid rgba(16, 185, 129, 0.3)',
            borderRadius: 'var(--radius-sm)',
            color: '#34d399',
            display: 'flex',
            alignItems: 'center',
            gap: '10px',
            fontSize: '13px',
            fontWeight: 500,
            animation: 'fadeIn 0.2s ease',
          }}
        >
          <CheckCircle2 size={16} />
          <span>{successToast}</span>
        </div>
      )}

      {/* Top Section: Phone Configuration Card */}
      <div className="glass-panel" style={{ padding: '24px', display: 'flex', flexDirection: 'column', gap: '16px' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '12px' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <div
              style={{
                width: '40px',
                height: '40px',
                borderRadius: 'var(--radius-sm)',
                background: 'rgba(37, 211, 102, 0.15)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: '#25D366',
              }}
            >
              <Smartphone size={22} />
            </div>
            <div>
              <h3 style={{ fontSize: '16px', fontWeight: 700 }}>Nomor WhatsApp Terhubung</h3>
              <p style={{ fontSize: '12px', color: 'var(--text-secondary)' }}>
                Format nomor internasional (contoh: <code>6281234567890</code>). Nomor ini digunakan gateway Fonnte / WhatsApp untuk mengirim pesan.
              </p>
            </div>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            {hasPhone ? (
              <span className="badge badge-emerald" style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                <CheckCircle2 size={12} /> WhatsApp Terdaftar
              </span>
            ) : (
              <span className="badge badge-amber" style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                <AlertTriangle size={12} /> Belum Ada Nomor (Gate Diblokir)
              </span>
            )}
          </div>
        </div>

        <form onSubmit={handleSavePhone} style={{ display: 'flex', gap: '12px', alignItems: 'center', maxWidth: '520px', marginTop: '4px' }}>
          <div style={{ position: 'relative', flex: 1 }}>
            <input
              type="text"
              placeholder="6281234567890"
              value={phoneInput}
              onChange={(e) => setPhoneInput(e.target.value)}
              className="input-field"
              style={{
                width: '100%',
                paddingLeft: '44px',
                fontSize: '14px',
                fontFamily: 'var(--font-mono)',
              }}
            />
            <span
              style={{
                position: 'absolute',
                left: '14px',
                top: '50%',
                transform: 'translateY(-50%)',
                color: 'var(--text-muted)',
                fontWeight: 600,
                fontSize: '13px',
              }}
            >
              +
            </span>
          </div>

          <button
            type="submit"
            disabled={saving || loading}
            className="btn-primary"
            style={{ padding: '9px 18px', fontSize: '13px', whiteSpace: 'nowrap' }}
          >
            {saving ? <RefreshCw size={14} className="spin" /> : <Check size={14} />}
            <span>Simpan Nomor</span>
          </button>
        </form>

        {!hasPhone && (
          <div
            style={{
              padding: '10px 14px',
              borderRadius: 'var(--radius-sm)',
              background: 'rgba(245, 158, 11, 0.08)',
              border: '1px solid rgba(245, 158, 11, 0.25)',
              color: '#fbbf24',
              fontSize: '12px',
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
            }}
          >
            <Info size={14} />
            <span>
              <strong>Perhatian:</strong> Backend Preference Gate secara otomatis menolak (clean no-op) pembuatan notifikasi jika nomor WhatsApp belum terisi.
            </span>
          </div>
        )}
      </div>

      {/* Main Grid: Preferences Toggles & WhatsApp Phone Mockup Simulator */}
      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(340px, 1fr) minmax(360px, 480px)', gap: '24px', alignItems: 'start' }}>
        {/* Left Column: Preference Toggles */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '4px' }}>
            <Sliders size={18} color="#10b981" />
            <h2 style={{ fontSize: '18px', fontWeight: 700 }}>Pengaturan Notifikasi Independen</h2>
          </div>

          <p style={{ fontSize: '13px', color: 'var(--text-secondary)', marginBottom: '8px' }}>
            Setiap notifikasi dihitung secara terpisah dari read models modul Analytics (CQRS) untuk efisiensi beban database.
          </p>

          {/* Toggle: Daily Summary */}
          <div className="glass-card" style={{ padding: '18px 20px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px' }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: '12px' }}>
              <div
                style={{
                  width: '36px',
                  height: '36px',
                  borderRadius: 'var(--radius-sm)',
                  background: 'rgba(16, 185, 129, 0.1)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  color: '#34d399',
                  flexShrink: 0,
                }}
              >
                <Calendar size={18} />
              </div>
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                  <span style={{ fontWeight: 600, fontSize: '14px' }}>Ringkasan Harian (Daily Summary)</span>
                  <span style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>07:30 Pagi</span>
                </div>
                <p style={{ fontSize: '12px', color: 'var(--text-secondary)', marginTop: '4px' }}>
                  Total pengeluaran & pemasukan kemarin, rincian kategori terbesar, serta spending status.
                </p>
              </div>
            </div>

            <label style={{ position: 'relative', display: 'inline-block', width: '44px', height: '24px', flexShrink: 0, cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={Boolean(preferences?.dailySummaryEnabled)}
                disabled={savingField === 'dailySummaryEnabled'}
                onChange={() => handleToggle('dailySummaryEnabled', preferences?.dailySummaryEnabled)}
                style={{ opacity: 0, width: 0, height: 0 }}
              />
              <span
                style={{
                  position: 'absolute',
                  inset: 0,
                  backgroundColor: preferences?.dailySummaryEnabled ? '#10b981' : 'rgba(255, 255, 255, 0.15)',
                  borderRadius: '24px',
                  transition: '0.2s',
                }}
              />
              <span
                style={{
                  position: 'absolute',
                  content: '""',
                  height: '18px',
                  width: '18px',
                  left: preferences?.dailySummaryEnabled ? '23px' : '3px',
                  bottom: '3px',
                  backgroundColor: '#ffffff',
                  borderRadius: '50%',
                  transition: '0.2s',
                }}
              />
            </label>
          </div>

          {/* Toggle: Weekly Summary */}
          <div className="glass-card" style={{ padding: '18px 20px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px' }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: '12px' }}>
              <div
                style={{
                  width: '36px',
                  height: '36px',
                  borderRadius: 'var(--radius-sm)',
                  background: 'rgba(6, 182, 212, 0.1)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  color: '#06b6d4',
                  flexShrink: 0,
                }}
              >
                <PieChart size={18} />
              </div>
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                  <span style={{ fontWeight: 600, fontSize: '14px' }}>Ringkasan Mingguan (Weekly Summary)</span>
                  <span style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>Senin 08:00</span>
                </div>
                <p style={{ fontSize: '12px', color: 'var(--text-secondary)', marginTop: '4px' }}>
                  Perbandingan performa pengeluaran mingguan dengan minggu sebelumnya.
                </p>
              </div>
            </div>

            <label style={{ position: 'relative', display: 'inline-block', width: '44px', height: '24px', flexShrink: 0, cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={Boolean(preferences?.weeklySummaryEnabled)}
                disabled={savingField === 'weeklySummaryEnabled'}
                onChange={() => handleToggle('weeklySummaryEnabled', preferences?.weeklySummaryEnabled)}
                style={{ opacity: 0, width: 0, height: 0 }}
              />
              <span
                style={{
                  position: 'absolute',
                  inset: 0,
                  backgroundColor: preferences?.weeklySummaryEnabled ? '#06b6d4' : 'rgba(255, 255, 255, 0.15)',
                  borderRadius: '24px',
                  transition: '0.2s',
                }}
              />
              <span
                style={{
                  position: 'absolute',
                  content: '""',
                  height: '18px',
                  width: '18px',
                  left: preferences?.weeklySummaryEnabled ? '23px' : '3px',
                  bottom: '3px',
                  backgroundColor: '#ffffff',
                  borderRadius: '50%',
                  transition: '0.2s',
                }}
              />
            </label>
          </div>

          {/* Toggle: Monthly Report */}
          <div className="glass-card" style={{ padding: '18px 20px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px' }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: '12px' }}>
              <div
                style={{
                  width: '36px',
                  height: '36px',
                  borderRadius: 'var(--radius-sm)',
                  background: 'rgba(99, 102, 241, 0.1)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  color: '#818cf8',
                  flexShrink: 0,
                }}
              >
                <DollarSign size={18} />
              </div>
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                  <span style={{ fontWeight: 600, fontSize: '14px' }}>Laporan Bulanan (Monthly Report)</span>
                  <span style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>Tgl 1, 09:00</span>
                </div>
                <p style={{ fontSize: '12px', color: 'var(--text-secondary)', marginTop: '4px' }}>
                  Rekapitulasi total bulanan, net cashflow, dan rasio tabungan yang tercapai.
                </p>
              </div>
            </div>

            <label style={{ position: 'relative', display: 'inline-block', width: '44px', height: '24px', flexShrink: 0, cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={Boolean(preferences?.monthlyReportEnabled)}
                disabled={savingField === 'monthlyReportEnabled'}
                onChange={() => handleToggle('monthlyReportEnabled', preferences?.monthlyReportEnabled)}
                style={{ opacity: 0, width: 0, height: 0 }}
              />
              <span
                style={{
                  position: 'absolute',
                  inset: 0,
                  backgroundColor: preferences?.monthlyReportEnabled ? '#6366f1' : 'rgba(255, 255, 255, 0.15)',
                  borderRadius: '24px',
                  transition: '0.2s',
                }}
              />
              <span
                style={{
                  position: 'absolute',
                  content: '""',
                  height: '18px',
                  width: '18px',
                  left: preferences?.monthlyReportEnabled ? '23px' : '3px',
                  bottom: '3px',
                  backgroundColor: '#ffffff',
                  borderRadius: '50%',
                  transition: '0.2s',
                }}
              />
            </label>
          </div>

          {/* Toggle: Budget Alerts */}
          <div className="glass-card" style={{ padding: '18px 20px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px' }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: '12px' }}>
              <div
                style={{
                  width: '36px',
                  height: '36px',
                  borderRadius: 'var(--radius-sm)',
                  background: 'rgba(245, 158, 11, 0.1)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  color: '#f59e0b',
                  flexShrink: 0,
                }}
              >
                <Bell size={18} />
              </div>
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                  <span style={{ fontWeight: 600, fontSize: '14px' }}>Peringatan Anggaran (Budget Alerts)</span>
                  <span className="badge badge-amber" style={{ fontSize: '10px' }}>Real-time</span>
                </div>
                <p style={{ fontSize: '12px', color: 'var(--text-secondary)', marginTop: '4px' }}>
                  Pemberitahuan instan saat pengeluaran kategori melampaui batas 80% atau 100% dari alokasi budget.
                </p>
              </div>
            </div>

            <label style={{ position: 'relative', display: 'inline-block', width: '44px', height: '24px', flexShrink: 0, cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={Boolean(preferences?.budgetAlertsEnabled)}
                disabled={savingField === 'budgetAlertsEnabled'}
                onChange={() => handleToggle('budgetAlertsEnabled', preferences?.budgetAlertsEnabled)}
                style={{ opacity: 0, width: 0, height: 0 }}
              />
              <span
                style={{
                  position: 'absolute',
                  inset: 0,
                  backgroundColor: preferences?.budgetAlertsEnabled ? '#f59e0b' : 'rgba(255, 255, 255, 0.15)',
                  borderRadius: '24px',
                  transition: '0.2s',
                }}
              />
              <span
                style={{
                  position: 'absolute',
                  content: '""',
                  height: '18px',
                  width: '18px',
                  left: preferences?.budgetAlertsEnabled ? '23px' : '3px',
                  bottom: '3px',
                  backgroundColor: '#ffffff',
                  borderRadius: '50%',
                  transition: '0.2s',
                }}
              />
            </label>
          </div>

          {/* Toggle: AI Recommendations */}
          <div className="glass-card" style={{ padding: '18px 20px', display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px' }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: '12px' }}>
              <div
                style={{
                  width: '36px',
                  height: '36px',
                  borderRadius: 'var(--radius-sm)',
                  background: 'rgba(236, 72, 153, 0.1)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  color: '#f472b6',
                  flexShrink: 0,
                }}
              >
                <Sparkles size={18} />
              </div>
              <div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                  <span style={{ fontWeight: 600, fontSize: '14px' }}>Rekomendasi Pintar (AI Insights)</span>
                  <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>Opsional</span>
                </div>
                <p style={{ fontSize: '12px', color: 'var(--text-secondary)', marginTop: '4px' }}>
                  Analisis gaya hidup dan rekomendasi penghematan mingguan berbasis pola pengeluaran.
                </p>
              </div>
            </div>

            <label style={{ position: 'relative', display: 'inline-block', width: '44px', height: '24px', flexShrink: 0, cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={Boolean(preferences?.aiRecommendationsEnabled)}
                disabled={savingField === 'aiRecommendationsEnabled'}
                onChange={() => handleToggle('aiRecommendationsEnabled', preferences?.aiRecommendationsEnabled)}
                style={{ opacity: 0, width: 0, height: 0 }}
              />
              <span
                style={{
                  position: 'absolute',
                  inset: 0,
                  backgroundColor: preferences?.aiRecommendationsEnabled ? '#ec4899' : 'rgba(255, 255, 255, 0.15)',
                  borderRadius: '24px',
                  transition: '0.2s',
                }}
              />
              <span
                style={{
                  position: 'absolute',
                  content: '""',
                  height: '18px',
                  width: '18px',
                  left: preferences?.aiRecommendationsEnabled ? '23px' : '3px',
                  bottom: '3px',
                  backgroundColor: '#ffffff',
                  borderRadius: '50%',
                  transition: '0.2s',
                }}
              />
            </label>
          </div>
        </div>

        {/* Right Column: Interactive WhatsApp Message Simulator */}
        <div className="glass-panel" style={{ padding: '24px', display: 'flex', flexDirection: 'column', gap: '16px' }}>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <MessageSquare size={18} color="#25D366" />
              <h3 style={{ fontSize: '16px', fontWeight: 700 }}>WhatsApp Message Preview</h3>
            </div>
            <span style={{ fontSize: '11px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
              Fonnte / WhatsApp UI
            </span>
          </div>

          {/* Preview Tab Selector */}
          <div style={{ display: 'flex', gap: '6px', background: 'rgba(255, 255, 255, 0.04)', padding: '4px', borderRadius: 'var(--radius-sm)' }}>
            <button
              onClick={() => setPreviewTab('daily')}
              style={{
                flex: 1,
                padding: '6px 10px',
                border: 'none',
                borderRadius: 'var(--radius-sm)',
                background: previewTab === 'daily' ? 'rgba(37, 211, 102, 0.2)' : 'transparent',
                color: previewTab === 'daily' ? '#25D366' : 'var(--text-secondary)',
                fontSize: '12px',
                fontWeight: 600,
                cursor: 'pointer',
              }}
            >
              Daily
            </button>
            <button
              onClick={() => setPreviewTab('budget')}
              style={{
                flex: 1,
                padding: '6px 10px',
                border: 'none',
                borderRadius: 'var(--radius-sm)',
                background: previewTab === 'budget' ? 'rgba(37, 211, 102, 0.2)' : 'transparent',
                color: previewTab === 'budget' ? '#25D366' : 'var(--text-secondary)',
                fontSize: '12px',
                fontWeight: 600,
                cursor: 'pointer',
              }}
            >
              Budget Alert
            </button>
            <button
              onClick={() => setPreviewTab('review')}
              style={{
                flex: 1,
                padding: '6px 10px',
                border: 'none',
                borderRadius: 'var(--radius-sm)',
                background: previewTab === 'review' ? 'rgba(37, 211, 102, 0.2)' : 'transparent',
                color: previewTab === 'review' ? '#25D366' : 'var(--text-secondary)',
                fontSize: '12px',
                fontWeight: 600,
                cursor: 'pointer',
              }}
            >
              Review (Interactive)
            </button>
          </div>

          {/* WhatsApp Chat Simulation Frame */}
          <div
            style={{
              background: '#0b141a',
              borderRadius: '12px',
              border: '1px solid #202c33',
              padding: '16px',
              display: 'flex',
              flexDirection: 'column',
              gap: '12px',
              minHeight: '320px',
            }}
          >
            {/* Chat Header inside simulation */}
            <div style={{ display: 'flex', alignItems: 'center', gap: '10px', borderBottom: '1px solid #202c33', paddingBottom: '10px' }}>
              <div
                style={{
                  width: '32px',
                  height: '32px',
                  borderRadius: '50%',
                  background: '#25D366',
                  color: '#070a13',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontWeight: 800,
                  fontSize: '13px',
                }}
              >
                FG
              </div>
              <div>
                <div style={{ fontSize: '13px', fontWeight: 600, color: '#e9edef' }}>FinGweh Bot</div>
                <div style={{ fontSize: '11px', color: '#8696a0' }}>Official WhatsApp Bot • Online</div>
              </div>
            </div>

            {/* Simulated Bubble */}
            <div
              style={{
                background: '#005c4b',
                color: '#e9edef',
                borderRadius: '8px',
                padding: '12px 14px',
                fontSize: '13px',
                lineHeight: '1.5',
                maxWidth: '92%',
                alignSelf: 'flex-start',
                position: 'relative',
                boxShadow: '0 1px 2px rgba(0,0,0,0.3)',
              }}
            >
              {previewTab === 'daily' && (
                <div>
                  <div>📊 *Ringkasan Finansial Harian FinGweh*</div>
                  <div style={{ fontSize: '12px', color: '#aebac1', marginTop: '2px' }}>Tanggal: 27 Okt 2026</div>
                  <div style={{ margin: '8px 0', borderTop: '1px solid rgba(255,255,255,0.1)', paddingTop: '6px' }}>
                    <div>💸 *Pengeluaran:* Rp 185.000</div>
                    <div>💰 *Pemasukan:* Rp 0</div>
                  </div>
                  <div>
                    *Pengeluaran Terbesar:*<br />
                    • Food & Dining: Rp 120.000<br />
                    • Transportation: Rp 65.000
                  </div>
                  <div style={{ marginTop: '8px', fontSize: '12px', fontStyle: 'italic', color: '#d1d7db' }}>
                    💡 Tips: Pengeluaran makanan hari ini stabil di bawah batas anggaran. Mantap!
                  </div>
                </div>
              )}

              {previewTab === 'budget' && (
                <div>
                  <div>⚠️ *Peringatan Anggaran FinGweh*</div>
                  <div style={{ margin: '8px 0' }}>
                    Kategori *Food & Dining* telah mencapai *85.4%* dari alokasi budget bulanan Anda!
                  </div>
                  <div>
                    • Terpakai: *Rp 2.135.000*<br />
                    • Batas Anggaran: Rp 2.500.000<br />
                    • Sisa Aman: Rp 365.000
                  </div>
                  <div style={{ marginTop: '8px', fontSize: '12px', color: '#fed7aa' }}>
                    Mohon perhatikan pengeluaran beberapa hari ke depan agar tidak overbudget.
                  </div>
                </div>
              )}

              {previewTab === 'review' && (
                <div>
                  <div>🔍 *Verifikasi Transaksi Baru*</div>
                  <div style={{ margin: '6px 0' }}>
                    Kami mendeteksi transaksi email dengan tingkat keyakinan AI sedang:
                  </div>
                  <div>
                    • Nominal: *Rp 75.000*<br />
                    • Merchant: *Bakso Solo Grand Mall*<br />
                    • Kategori Terduga: *Food*
                  </div>
                  <div style={{ marginTop: '8px', fontSize: '12px', color: '#aebac1' }}>
                    Apakah transaksi di atas ingin langsung dicatat ke Ledger?
                  </div>
                </div>
              )}

              <div style={{ textAlign: 'right', fontSize: '10px', color: '#8696a0', marginTop: '6px' }}>
                12:45 ✓✓
              </div>
            </div>

            {/* Interactive WhatsApp Buttons for Review Request */}
            {previewTab === 'review' && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '6px', maxWidth: '92%', alignSelf: 'flex-start' }}>
                <div
                  style={{
                    background: '#202c33',
                    border: '1px solid #2a3942',
                    borderRadius: '8px',
                    padding: '8px 12px',
                    color: '#00a884',
                    fontSize: '12px',
                    fontWeight: 600,
                    textAlign: 'center',
                    cursor: 'pointer',
                  }}
                >
                  ✅ Ya, Catat ke Ledger
                </div>
                <div
                  style={{
                    background: '#202c33',
                    border: '1px solid #2a3942',
                    borderRadius: '8px',
                    padding: '8px 12px',
                    color: '#e9edef',
                    fontSize: '12px',
                    textAlign: 'center',
                    cursor: 'pointer',
                  }}
                >
                  ✏️ Edit Kategori
                </div>
                <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '4px' }}>
                  💬 <em>Atau balas teks langsung dengan ketik <strong>"Y"</strong> untuk konfirmasi, atau <strong>"N"</strong> untuk tolak.</em>
                </div>
              </div>
            )}
          </div>

          {/* Delivery Observability Tier */}
          <div
            style={{
              padding: '12px 14px',
              borderRadius: 'var(--radius-sm)',
              background: 'rgba(255, 255, 255, 0.02)',
              border: '1px solid var(--border-subtle)',
              fontSize: '12px',
              color: 'var(--text-secondary)',
              lineHeight: 1.5,
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: '6px', color: 'var(--text-primary)', fontWeight: 600, marginBottom: '4px' }}>
              <ShieldCheck size={14} color="#34d399" /> Two-Tier Delivery Confirmation
            </div>
            Pelacakan status gateway: <code>provider_accepted</code> → <code>delivered</code> → <code>read</code>. Status 'read' hanya untuk telemetri observabilitas dan tidak memblokir flow logika ledger.
          </div>
        </div>
      </div>
    </div>
  );
}
