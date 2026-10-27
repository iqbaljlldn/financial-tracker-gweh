import React, { Component } from 'react';
import { AlertOctagon, RefreshCw } from 'lucide-react';

/**
 * Top-level React Error Boundary with error reporting.
 */
export default class ErrorBoundary extends Component {
  constructor(props) {
    super(props);
    this.state = { hasError: false, error: null, errorInfo: null };
  }

  static getDerivedStateFromError(error) {
    return { hasError: true, error };
  }

  componentDidCatch(error, errorInfo) {
    this.setState({ errorInfo });
    // Strip potential PII before logging per 16_security.md §7
    const sanitizedMsg = String(error?.message || 'Unknown error').replace(/[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}/g, '[REDACTED_EMAIL]');
    console.error('[FinGweh Frontend Error Boundary Caught]', sanitizedMsg, errorInfo?.componentStack);
  }

  handleReload = () => {
    window.location.reload();
  };

  render() {
    if (this.state.hasError) {
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
              maxWidth: '540px',
              width: '100%',
              padding: '36px',
              textAlign: 'center',
              border: '1px solid rgba(244, 63, 94, 0.3)',
            }}
          >
            <div
              style={{
                width: '56px',
                height: '56px',
                borderRadius: '50%',
                background: 'rgba(244, 63, 94, 0.15)',
                color: '#fb7185',
                display: 'inline-flex',
                alignItems: 'center',
                justifyContent: 'center',
                marginBottom: '20px',
              }}
            >
              <AlertOctagon size={28} />
            </div>

            <h2 style={{ fontSize: '22px', marginBottom: '12px' }}>
              Something unexpected happened
            </h2>
            <p style={{ color: 'var(--text-secondary)', marginBottom: '24px', fontSize: '14px' }}>
              An unhandled client error occurred. Our observability systems have captured this event.
            </p>

            <div
              style={{
                textAlign: 'left',
                background: 'rgba(0, 0, 0, 0.4)',
                borderRadius: 'var(--radius-sm)',
                padding: '12px 16px',
                marginBottom: '24px',
                fontFamily: 'var(--font-mono)',
                fontSize: '12px',
                color: '#fb7185',
                overflowX: 'auto',
              }}
            >
              {this.state.error?.toString() || 'Unknown error'}
            </div>

            <button onClick={this.handleReload} className="btn-primary" style={{ width: '100%' }}>
              <RefreshCw size={16} /> Reload Application
            </button>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
