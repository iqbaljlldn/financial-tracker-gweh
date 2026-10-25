import React, { useState } from 'react';
import { Shield, Lock, Eye, Mail, ArrowRight, Zap, CheckCircle2, AlertCircle } from 'lucide-react';
import { useAuth } from '../context/AuthContext';
import ErrorEnvelopeDisplay from './ErrorEnvelopeDisplay';

export default function LandingPage() {
  const { loginWithGoogle, authError, setAuthError } = useAuth();
  const [initiating, setInitiating] = useState(false);

  const handleSignIn = async () => {
    try {
      setInitiating(true);
      await loginWithGoogle();
    } catch (err) {
      setInitiating(false);
      // authError handled in context
    }
  };

  return (
    <div style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column' }}>
      {/* Top Navbar */}
      <header
        style={{
          padding: '20px 32px',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          borderBottom: '1px solid var(--border-subtle)',
          background: 'rgba(7, 10, 19, 0.8)',
          backdropFilter: 'blur(16px)',
          position: 'sticky',
          top: 0,
          zIndex: 40,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
          <img src="/logo.svg" alt="FinGweh" style={{ width: '36px', height: '36px' }} />
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <span style={{ fontFamily: 'var(--font-display)', fontSize: '20px', fontWeight: 800, letterSpacing: '-0.02em' }}>
                FinGweh
              </span>
              <span className="badge badge-emerald" style={{ fontSize: '10px' }}>Sprint 1 Active</span>
            </div>
            <p style={{ fontSize: '11px', color: 'var(--text-muted)', lineHeight: 1 }}>Autonomous Financial Intelligence</p>
          </div>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '16px' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '12px', color: 'var(--text-secondary)' }}>
            <span className="pulse-indicator"></span>
            <span>API Online</span>
          </div>
        </div>
      </header>

      {/* Hero Section */}
      <main style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '60px 24px' }}>
        <div style={{ maxWidth: '960px', width: '100%', textAlign: 'center' }}>
          {/* Security Banner */}
          <div
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: '8px',
              padding: '6px 16px',
              background: 'rgba(16, 185, 129, 0.1)',
              border: '1px solid rgba(16, 185, 129, 0.3)',
              borderRadius: 'var(--radius-full)',
              marginBottom: '28px',
              color: '#34d399',
              fontSize: '13px',
              fontWeight: 500,
            }}
          >
            <Shield size={14} />
            <span>KMS Envelope Encrypted • Read-Only Scopes • Zero Cross-Schema Foreign Keys</span>
          </div>

          <h1
            style={{
              fontSize: 'clamp(38px, 6vw, 64px)',
              lineHeight: 1.15,
              marginBottom: '24px',
              background: 'linear-gradient(135deg, #ffffff 0%, #cbd5e1 50%, #94a3b8 100%)',
              WebkitBackgroundClip: 'text',
              WebkitTextFillColor: 'transparent',
            }}
          >
            Turn Your Gmail Receipts into an
            <br />
            <span
              style={{
                background: 'linear-gradient(135deg, #10b981 0%, #06b6d4 100%)',
                WebkitBackgroundClip: 'text',
                WebkitTextFillColor: 'transparent',
              }}
            >
              Autonomous Financial Ledger
            </span>
          </h1>

          <p
            style={{
              fontSize: 'clamp(16px, 2vw, 19px)',
              color: 'var(--text-secondary)',
              maxWidth: '680px',
              margin: '0 auto 40px',
              lineHeight: 1.6,
            }}
          >
            Connect your Gmail with strictly read-only permissions. FinGweh ingests receipts,
            runs deterministic template and LLM inference, and maintains a clean, double-entry ledger.
          </p>

          {/* Error envelope notice if any */}
          {authError && (
            <div style={{ maxWidth: '580px', margin: '0 auto 24px', textAlign: 'left' }}>
              <ErrorEnvelopeDisplay error={authError} onDismiss={() => setAuthError(null)} />
            </div>
          )}

          {/* Sign In CTA */}
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '16px' }}>
            <button
              onClick={handleSignIn}
              disabled={initiating}
              className="btn-google"
              style={{ minWidth: '280px' }}
              id="google-signin-btn"
            >
              {initiating ? (
                <>
                  <div className="spin" style={{ width: '18px', height: '18px', border: '2px solid rgba(255,255,255,0.3)', borderTopColor: '#fff', borderRadius: '50%' }}></div>
                  <span>Connecting to Google OAuth...</span>
                </>
              ) : (
                <>
                  <svg width="20" height="20" viewBox="0 0 24 24">
                    <path
                      fill="#4285F4"
                      d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"
                    />
                    <path
                      fill="#34A853"
                      d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"
                    />
                    <path
                      fill="#FBBC05"
                      d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.06H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.94l2.85-2.22.81-.63z"
                    />
                    <path
                      fill="#EA4335"
                      d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.06l3.66 2.84c.87-2.6 3.3-4.52 6.16-4.52z"
                    />
                  </svg>
                  <span>Continue with Google</span>
                  <ArrowRight size={18} style={{ marginLeft: '4px', opacity: 0.8 }} />
                </>
              )}
            </button>

            <div style={{ display: 'flex', alignItems: 'center', gap: '20px', fontSize: '12px', color: 'var(--text-muted)' }}>
              <span style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                <CheckCircle2 size={13} color="#10b981" /> No credit card required
              </span>
              <span style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                <CheckCircle2 size={13} color="#10b981" /> Read-only scope guaranteed
              </span>
              <span style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                <CheckCircle2 size={13} color="#10b981" /> AES-256 Envelope Encryption
              </span>
            </div>
          </div>

          {/* Pillars Grid */}
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))',
              gap: '20px',
              marginTop: '64px',
              textAlign: 'left',
            }}
          >
            <div className="glass-card" style={{ padding: '24px' }}>
              <div
                style={{
                  width: '40px',
                  height: '40px',
                  borderRadius: '10px',
                  background: 'rgba(16, 185, 129, 0.15)',
                  color: '#10b981',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  marginBottom: '16px',
                }}
              >
                <Lock size={20} />
              </div>
              <h3 style={{ fontSize: '17px', marginBottom: '8px' }}>Envelope Encrypted</h3>
              <p style={{ color: 'var(--text-secondary)', fontSize: '14px', lineHeight: 1.5 }}>
                OAuth refresh tokens are wrapped with KMS-generated Data Encryption Keys (DEKs). Plaintext tokens never hit the database.
              </p>
            </div>

            <div className="glass-card" style={{ padding: '24px' }}>
              <div
                style={{
                  width: '40px',
                  height: '40px',
                  borderRadius: '10px',
                  background: 'rgba(6, 182, 212, 0.15)',
                  color: '#06b6d4',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  marginBottom: '16px',
                }}
              >
                <Eye size={20} />
              </div>
              <h3 style={{ fontSize: '17px', marginBottom: '8px' }}>Strictly Read-Only</h3>
              <p style={{ color: 'var(--text-secondary)', fontSize: '14px', lineHeight: 1.5 }}>
                Enforced by PKCE authorization client requesting only <code>gmail.readonly</code>. We never read your personal messages or request write permissions.
              </p>
            </div>

            <div className="glass-card" style={{ padding: '24px' }}>
              <div
                style={{
                  width: '40px',
                  height: '40px',
                  borderRadius: '10px',
                  background: 'rgba(99, 102, 241, 0.15)',
                  color: '#6366f1',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  marginBottom: '16px',
                }}
              >
                <Zap size={20} />
              </div>
              <h3 style={{ fontSize: '17px', marginBottom: '8px' }}>CQRS & Clean Architecture</h3>
              <p style={{ color: 'var(--text-secondary)', fontSize: '14px', lineHeight: 1.5 }}>
                Isolated modular monolith across 8 database schemas with zero cross-schema foreign keys and tamper-resistant audit logs.
              </p>
            </div>
          </div>
        </div>
      </main>

      {/* Footer */}
      <footer
        style={{
          padding: '24px 32px',
          borderTop: '1px solid var(--border-subtle)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          fontSize: '13px',
          color: 'var(--text-muted)',
          flexWrap: 'wrap',
          gap: '12px',
        }}
      >
        <div>FinGweh Platform — Built with Go, PostgreSQL & React</div>
        <div style={{ display: 'flex', gap: '20px' }}>
          <span>Sprint 1: Identity & DB Foundations</span>
          <span>Security Gate Verified</span>
        </div>
      </footer>
    </div>
  );
}
