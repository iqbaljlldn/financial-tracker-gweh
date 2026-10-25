import React from 'react';
import { AlertTriangle, X, RefreshCw, Hash, FileQuestion } from 'lucide-react';

/**
 * Renders an API error envelope per 09_api_specification.md §7.2:
 * {
 *   "error": {
 *     "code": "VALIDATION_FAILED",
 *     "message": "...",
 *     "field": "...",
 *     "requestId": "req_..."
 *   }
 * }
 */
export default function ErrorEnvelopeDisplay({ error, onDismiss, onRetry }) {
  if (!error) return null;

  const envelope = error.envelope || {
    code: error.code || 'UNKNOWN_ERROR',
    message: error.message || 'An unexpected error occurred',
    field: error.field,
    requestId: error.requestId,
  };

  return (
    <div
      style={{
        position: 'relative',
        background: 'rgba(244, 63, 94, 0.08)',
        border: '1px solid rgba(244, 63, 94, 0.3)',
        backdropFilter: 'blur(16px)',
        borderRadius: 'var(--radius-md)',
        padding: '16px 20px',
        margin: '16px 0',
        animation: 'fadeIn 0.25s ease-out',
      }}
      role="alert"
    >
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '12px' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
          <div
            style={{
              padding: '6px',
              borderRadius: '8px',
              background: 'rgba(244, 63, 94, 0.2)',
              color: '#fb7185',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <AlertTriangle size={18} />
          </div>
          <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
              <span
                style={{
                  fontFamily: 'var(--font-mono)',
                  fontSize: '11px',
                  fontWeight: 600,
                  padding: '2px 8px',
                  borderRadius: '4px',
                  background: 'rgba(244, 63, 94, 0.2)',
                  color: '#fb7185',
                  letterSpacing: '0.05em',
                }}
              >
                {envelope.code}
              </span>
              {envelope.field && (
                <span
                  style={{
                    fontFamily: 'var(--font-mono)',
                    fontSize: '11px',
                    color: 'var(--text-muted)',
                    display: 'flex',
                    alignItems: 'center',
                    gap: '4px',
                  }}
                >
                  <FileQuestion size={12} /> field: {envelope.field}
                </span>
              )}
            </div>
            <p style={{ marginTop: '6px', color: '#fecdd3', fontSize: '14px', lineHeight: 1.5 }}>
              {envelope.message}
            </p>
          </div>
        </div>

        {onDismiss && (
          <button
            onClick={onDismiss}
            style={{
              background: 'transparent',
              border: 'none',
              color: '#fb7185',
              cursor: 'pointer',
              padding: '4px',
              borderRadius: '4px',
              display: 'flex',
              alignItems: 'center',
            }}
            title="Dismiss error"
          >
            <X size={16} />
          </button>
        )}
      </div>

      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          marginTop: '12px',
          paddingTop: '8px',
          borderTop: '1px solid rgba(244, 63, 94, 0.15)',
          fontSize: '12px',
          color: 'var(--text-muted)',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '4px', fontFamily: 'var(--font-mono)' }}>
          <Hash size={12} />
          <span>RequestId: {envelope.requestId || 'local_untraced'}</span>
        </div>

        {onRetry && (
          <button
            onClick={onRetry}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
              background: 'rgba(244, 63, 94, 0.2)',
              border: '1px solid rgba(244, 63, 94, 0.4)',
              color: '#fff',
              fontSize: '12px',
              padding: '4px 10px',
              borderRadius: 'var(--radius-sm)',
              cursor: 'pointer',
              transition: 'background 0.2s',
            }}
          >
            <RefreshCw size={12} /> Retry
          </button>
        )}
      </div>
    </div>
  );
}
