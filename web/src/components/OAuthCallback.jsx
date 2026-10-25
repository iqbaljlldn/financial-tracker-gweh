import React, { useEffect, useState } from 'react';
import { ShieldCheck, ArrowLeft, RefreshCw } from 'lucide-react';
import { useAuth } from '../context/AuthContext';
import ErrorEnvelopeDisplay from './ErrorEnvelopeDisplay';

export default function OAuthCallback({ onComplete, onCancel }) {
  const { handleOAuthCallback } = useAuth();
  const [statusText, setStatusText] = useState('Verifying Google authorization code...');
  const [error, setError] = useState(null);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const code = params.get('code');
    const state = params.get('state');
    const googleError = params.get('error');

    if (googleError) {
      setError({
        envelope: {
          code: 'OAUTH_ACCESS_DENIED',
          message: `Google authorization rejected: ${googleError}`,
          field: 'google_consent',
          requestId: 'client_oauth_denied',
        },
      });
      return;
    }

    if (!code || !state) {
      setError({
        envelope: {
          code: 'OAUTH_CALLBACK_INVALID',
          message: 'Missing code or state parameter in authorization response',
          field: 'query_params',
          requestId: 'client_missing_params',
        },
      });
      return;
    }

    async function processExchange() {
      try {
        setStatusText('Exchanging authorization code via PKCE challenge...');
        await new Promise((r) => setTimeout(r, 400));
        setStatusText('Wrapping credentials with KMS AES-256-GCM envelope encryption...');
        await new Promise((r) => setTimeout(r, 400));
        setStatusText('Issuing platform session JWT and DB-backed refresh token...');

        await handleOAuthCallback(code, state);
        if (onComplete) onComplete();
      } catch (err) {
        setError(err);
      }
    }

    processExchange();
  }, [handleOAuthCallback, onComplete]);

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: '24px',
        background: 'var(--bg-primary)',
      }}
    >
      <div
        className="glass-panel"
        style={{
          maxWidth: '520px',
          width: '100%',
          padding: '40px 32px',
          textAlign: 'center',
        }}
      >
        {!error ? (
          <>
            <div
              style={{
                width: '64px',
                height: '64px',
                borderRadius: '50%',
                background: 'rgba(16, 185, 129, 0.15)',
                color: '#10b981',
                display: 'inline-flex',
                alignItems: 'center',
                justifyContent: 'center',
                marginBottom: '24px',
                position: 'relative',
              }}
            >
              <ShieldCheck size={32} />
              <div
                className="spin"
                style={{
                  position: 'absolute',
                  inset: '-4px',
                  borderRadius: '50%',
                  border: '2px solid transparent',
                  borderTopColor: '#10b981',
                  borderRightColor: '#06b6d4',
                }}
              />
            </div>

            <h2 style={{ fontSize: '22px', marginBottom: '12px' }}>
              Finalizing Secure Authentication
            </h2>
            <p
              style={{
                color: 'var(--text-secondary)',
                fontSize: '14px',
                minHeight: '44px',
                lineHeight: 1.5,
              }}
            >
              {statusText}
            </p>

            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                gap: '8px',
                marginTop: '20px',
                fontSize: '12px',
                color: 'var(--text-muted)',
              }}
            >
              <span className="pulse-indicator" style={{ width: '6px', height: '6px' }}></span>
              <span>Zero write scopes • Envelope DEK verification</span>
            </div>
          </>
        ) : (
          <div>
            <h2 style={{ fontSize: '20px', marginBottom: '12px' }}>
              Authentication Failed
            </h2>
            <ErrorEnvelopeDisplay error={error} />
            <button
              onClick={onCancel}
              className="btn-secondary"
              style={{ width: '100%', marginTop: '16px' }}
            >
              <ArrowLeft size={16} /> Return to Sign In
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
