import React, { useState, useEffect, useCallback } from 'react';
import {
  FileText,
  Search,
  Filter,
  CheckCircle,
  AlertCircle,
  Clock,
  Sparkles,
  Cpu,
  User,
  Edit3,
  Check,
  X,
  ChevronRight,
  Plus,
  RefreshCw,
  History,
  Tag,
  ArrowUpRight,
  ArrowDownLeft,
  Calendar,
  Layers,
  AlertTriangle,
} from 'lucide-react';
import { apiClient } from '../api/client';
import { formatMoney, isValidDecimalString } from '../utils/money';

const EVENT_TYPES = [
  { value: 'purchase', label: 'Purchase' },
  { value: 'refund', label: 'Refund' },
  { value: 'peer_to_peer_transfer', label: 'P2P Transfer' },
  { value: 'bank_transfer', label: 'Bank Transfer' },
  { value: 'bill_payment', label: 'Bill Payment' },
  { value: 'subscription_renewal', label: 'Subscription Renewal' },
  { value: 'salary_income', label: 'Salary Income' },
  { value: 'investment_income', label: 'Investment Income' },
  { value: 'cash_withdrawal', label: 'Cash Withdrawal' },
  { value: 'fee', label: 'Fee / Charge' },
  { value: 'interest', label: 'Interest' },
  { value: 'reward_cashback', label: 'Reward / Cashback' },
  { value: 'tax', label: 'Tax' },
  { value: 'adjustment', label: 'Adjustment' },
  { value: 'other', label: 'Other' },
];

const CATEGORIES = [
  'Food & Beverage',
  'Groceries',
  'Shopping',
  'Transportation',
  'Utilities',
  'Entertainment',
  'Health & Fitness',
  'Housing',
  'Education',
  'Travel',
  'Financial Services',
  'Income',
  'Transfer',
  'Other',
];

export function LedgerView() {
  const [events, setEvents] = useState([]);
  const [nextCursor, setNextCursor] = useState(null);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState(null);

  // Filters
  const [statusFilter, setStatusFilter] = useState('');
  const [categoryFilter, setCategoryFilter] = useState('');
  const [merchantQuery, setMerchantQuery] = useState('');
  const [fromDate, setFromDate] = useState('');
  const [toDate, setToDate] = useState('');

  // Selected event for detail drawer
  const [selectedEventId, setSelectedEventId] = useState(null);
  const [selectedEvent, setSelectedEvent] = useState(null);
  const [overrides, setOverrides] = useState([]);
  const [loadingDetail, setLoadingDetail] = useState(false);

  // Override / Edit Modal state
  const [isEditModalOpen, setIsEditModalOpen] = useState(false);
  const [editTargetEvent, setEditTargetEvent] = useState(null);
  const [isConfirmingWithEdit, setIsConfirmingWithEdit] = useState(false);
  const [overrideForm, setOverrideForm] = useState({
    amount: '',
    currency: 'IDR',
    merchant: '',
    eventType: '',
    category: '',
  });
  const [submittingOverride, setSubmittingOverride] = useState(false);
  const [overrideError, setOverrideError] = useState(null);

  // Create Manual Event Modal state
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [createForm, setCreateForm] = useState({
    eventType: 'purchase',
    amount: '',
    currency: 'IDR',
    merchantName: '',
    category: 'Shopping',
    occurredAt: new Date().toISOString().slice(0, 16),
    paymentMethodType: 'qris',
    paymentMethodHint: '',
  });
  const [submittingCreate, setSubmittingCreate] = useState(false);
  const [createError, setCreateError] = useState(null);

  // Fetch events list
  const fetchEvents = useCallback(
    async (isRefresh = false, cursor = null) => {
      try {
        if (cursor) {
          setLoadingMore(true);
        } else if (isRefresh) {
          setRefreshing(true);
        } else {
          setLoading(true);
        }
        setError(null);

        const params = { limit: 20 };
        if (cursor) params.cursor = cursor;
        if (statusFilter) params.status = statusFilter;
        if (categoryFilter) params.category = categoryFilter;
        if (merchantQuery) params.merchant = merchantQuery;
        if (fromDate) params.from = new Date(fromDate).toISOString();
        if (toDate) params.to = new Date(toDate).toISOString();

        const res = await apiClient.listFinancialEvents(params);
        const newItems = res?.items || [];
        setNextCursor(res?.nextCursor || null);

        if (cursor) {
          setEvents((prev) => [...prev, ...newItems]);
        } else {
          setEvents(newItems);
        }
      } catch (err) {
        setError(err.message || 'Failed to load financial events');
      } finally {
        setLoading(false);
        setLoadingMore(false);
        setRefreshing(false);
      }
    },
    [statusFilter, categoryFilter, merchantQuery, fromDate, toDate]
  );

  useEffect(() => {
    fetchEvents();
  }, [fetchEvents]);

  // Load event details & override history
  const openDetailDrawer = async (eventId) => {
    try {
      setSelectedEventId(eventId);
      setLoadingDetail(true);
      const [eventData, overrideList] = await Promise.all([
        apiClient.getFinancialEvent(eventId),
        apiClient.listEventOverrides(eventId).catch(() => []),
      ]);
      setSelectedEvent(eventData);
      setOverrides(overrideList || []);
    } catch (err) {
      setError(err.message || 'Failed to load event details');
    } finally {
      setLoadingDetail(false);
    }
  };

  const closeDetailDrawer = () => {
    setSelectedEventId(null);
    setSelectedEvent(null);
    setOverrides([]);
  };

  // Direct confirm action
  const handleQuickConfirm = async (e, eventId) => {
    e.stopPropagation();
    try {
      await apiClient.confirmPendingReviewEvent(eventId);
      // Update locally
      setEvents((prev) =>
        prev.map((ev) => (ev.id === eventId ? { ...ev, status: 'finalized' } : ev))
      );
      if (selectedEvent && selectedEvent.id === eventId) {
        setSelectedEvent((prev) => ({ ...prev, status: 'finalized' }));
      }
    } catch (err) {
      alert(`Confirm failed: ${err.message}`);
    }
  };

  // Open Edit / Override Modal
  const openEditModal = (event, isConfirm = false) => {
    setEditTargetEvent(event);
    setIsConfirmingWithEdit(isConfirm);
    const effAmount = event.effectiveAmount || event.originalAmount || { value: '', currency: 'IDR' };
    const effMerchant = event.effectiveMerchant || event.originalMerchant || { rawName: '' };
    const effEventType = event.effectiveEventType || event.originalEventType || 'purchase';
    const effCategory = event.category?.category || '';

    setOverrideForm({
      amount: effAmount.value || '',
      currency: effAmount.currency || 'IDR',
      merchant: effMerchant.rawName || '',
      eventType: effEventType,
      category: effCategory,
    });
    setOverrideError(null);
    setIsEditModalOpen(true);
  };

  const handleSaveOverride = async (e) => {
    e.preventDefault();
    if (!editTargetEvent) return;

    if (overrideForm.amount && !isValidDecimalString(overrideForm.amount)) {
      setOverrideError('Amount must be a valid decimal string (e.g. 45.00 or 120000)');
      return;
    }

    try {
      setSubmittingOverride(true);
      setOverrideError(null);

      const fields = {};
      if (overrideForm.amount) {
        fields.amount = {
          value: overrideForm.amount.trim(),
          currency: overrideForm.currency.trim(),
        };
      }
      if (overrideForm.merchant) {
        fields.merchant = overrideForm.merchant.trim();
      }
      if (overrideForm.eventType) {
        fields.eventType = overrideForm.eventType;
      }
      if (overrideForm.category) {
        fields.category = overrideForm.category;
      }

      if (isConfirmingWithEdit) {
        await apiClient.confirmPendingReviewEvent(editTargetEvent.id, fields);
      } else {
        await apiClient.submitEventOverride(editTargetEvent.id, fields);
      }

      setIsEditModalOpen(false);
      setEditTargetEvent(null);
      // Refresh list and details
      fetchEvents(true);
      if (selectedEventId) {
        openDetailDrawer(selectedEventId);
      }
    } catch (err) {
      setOverrideError(err.message || 'Failed to submit override');
    } finally {
      setSubmittingOverride(false);
    }
  };

  // Handle Create Manual Event
  const handleCreateManualEvent = async (e) => {
    e.preventDefault();
    if (!createForm.amount || !isValidDecimalString(createForm.amount)) {
      setCreateError('Amount must be a valid decimal string (e.g. 50000 or 25.50)');
      return;
    }
    if (!createForm.merchantName) {
      setCreateError('Merchant name is required');
      return;
    }

    try {
      setSubmittingCreate(true);
      setCreateError(null);

      const payload = {
        eventType: createForm.eventType,
        amount: {
          value: createForm.amount.trim(),
          currency: createForm.currency.trim(),
        },
        occurredAt: new Date(createForm.occurredAt).toISOString(),
        merchantName: createForm.merchantName.trim(),
        paymentMethod: {
          type: createForm.paymentMethodType,
          hint: createForm.paymentMethodHint.trim(),
        },
        category: createForm.category,
      };

      await apiClient.createManualEvent(payload);
      setIsCreateModalOpen(false);
      setCreateForm({
        eventType: 'purchase',
        amount: '',
        currency: 'IDR',
        merchantName: '',
        category: 'Shopping',
        occurredAt: new Date().toISOString().slice(0, 16),
        paymentMethodType: 'qris',
        paymentMethodHint: '',
      });
      fetchEvents(true);
    } catch (err) {
      setCreateError(err.message || 'Failed to create manual event');
    } finally {
      setSubmittingCreate(false);
    }
  };

  const pendingCount = events.filter((ev) => ev.status === 'pending_review').length;

  return (
    <div>
      {/* Header Bar */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          marginBottom: '24px',
          flexWrap: 'wrap',
          gap: '16px',
        }}
      >
        <div>
          <h1 style={{ fontSize: '26px', marginBottom: '4px' }}>Financial Ledger</h1>
          <p style={{ color: 'var(--text-secondary)', fontSize: '14px' }}>
            Auditable, immutable ledger events backed by confidence-gated outbox state.
          </p>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
          <button
            onClick={() => fetchEvents(true)}
            className="btn-secondary"
            title="Refresh Ledger"
            disabled={refreshing}
          >
            <RefreshCw size={14} className={refreshing ? 'spin' : ''} />
            <span>Refresh</span>
          </button>

          <button
            onClick={() => setIsCreateModalOpen(true)}
            className="btn-primary"
            style={{ padding: '9px 18px', fontSize: '14px' }}
          >
            <Plus size={16} />
            <span>New Transaction</span>
          </button>
        </div>
      </div>

      {/* Pending Review Alert Banner */}
      {pendingCount > 0 && (
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '14px 20px',
            marginBottom: '20px',
            background: 'rgba(245, 158, 11, 0.1)',
            border: '1px solid rgba(245, 158, 11, 0.3)',
            borderRadius: 'var(--radius-md)',
            color: '#fbbf24',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <AlertTriangle size={18} />
            <span style={{ fontSize: '14px', fontWeight: 600 }}>
              {pendingCount} transaction{pendingCount > 1 ? 's' : ''} require user review (confidence below threshold).
            </span>
          </div>
          <button
            onClick={() => setStatusFilter('pending_review')}
            className="btn-secondary"
            style={{
              padding: '6px 12px',
              fontSize: '12px',
              borderColor: 'rgba(245, 158, 11, 0.4)',
              color: '#fbbf24',
            }}
          >
            Filter Pending Items
          </button>
        </div>
      )}

      {/* Filter Toolbar */}
      <div
        className="glass-card"
        style={{
          padding: '16px 20px',
          marginBottom: '20px',
          display: 'flex',
          flexWrap: 'wrap',
          gap: '12px',
          alignItems: 'center',
        }}
      >
        {/* Merchant Search */}
        <div style={{ display: 'flex', alignItems: 'center', position: 'relative', minWidth: '220px', flex: '1 1 200px' }}>
          <Search size={14} style={{ position: 'absolute', left: '12px', color: 'var(--text-muted)' }} />
          <input
            type="text"
            className="form-input"
            placeholder="Search merchant..."
            value={merchantQuery}
            onChange={(e) => setMerchantQuery(e.target.value)}
            style={{ width: '100%', paddingLeft: '34px' }}
          />
        </div>

        {/* Status Filter */}
        <select
          className="form-select"
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          style={{ minWidth: '150px' }}
        >
          <option value="">All Statuses</option>
          <option value="pending_review">Pending Review</option>
          <option value="finalized">Finalized</option>
          <option value="archived">Archived</option>
        </select>

        {/* Category Filter */}
        <select
          className="form-select"
          value={categoryFilter}
          onChange={(e) => setCategoryFilter(e.target.value)}
          style={{ minWidth: '160px' }}
        >
          <option value="">All Categories</option>
          {CATEGORIES.map((cat) => (
            <option key={cat} value={cat}>
              {cat}
            </option>
          ))}
        </select>

        {/* Date From */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>From:</span>
          <input
            type="date"
            className="form-input"
            value={fromDate}
            onChange={(e) => setFromDate(e.target.value)}
            style={{ padding: '8px 10px', fontSize: '13px' }}
          />
        </div>

        {/* Date To */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>To:</span>
          <input
            type="date"
            className="form-input"
            value={toDate}
            onChange={(e) => setToDate(e.target.value)}
            style={{ padding: '8px 10px', fontSize: '13px' }}
          />
        </div>

        {(statusFilter || categoryFilter || merchantQuery || fromDate || toDate) && (
          <button
            onClick={() => {
              setStatusFilter('');
              setCategoryFilter('');
              setMerchantQuery('');
              setFromDate('');
              setToDate('');
            }}
            className="btn-secondary"
            style={{ padding: '8px 12px', fontSize: '12px' }}
          >
            Clear Filters
          </button>
        )}
      </div>

      {/* Events Table / List View */}
      <div className="glass-panel" style={{ overflow: 'hidden' }}>
        {loading ? (
          <div style={{ padding: '48px', textAlign: 'center', color: 'var(--text-secondary)' }}>
            <div
              className="spin"
              style={{
                width: '32px',
                height: '32px',
                border: '3px solid rgba(255, 255, 255, 0.1)',
                borderTopColor: 'var(--accent-emerald)',
                borderRadius: '50%',
                margin: '0 auto 16px',
              }}
            />
            <span>Loading ledger transactions...</span>
          </div>
        ) : error ? (
          <div style={{ padding: '40px', textAlign: 'center', color: '#fb7185' }}>
            <AlertCircle size={28} style={{ margin: '0 auto 12px' }} />
            <div>{error}</div>
            <button
              onClick={() => fetchEvents()}
              className="btn-secondary"
              style={{ marginTop: '16px' }}
            >
              Retry
            </button>
          </div>
        ) : events.length === 0 ? (
          <div style={{ padding: '60px 20px', textAlign: 'center' }}>
            <FileText size={40} style={{ color: 'var(--text-muted)', margin: '0 auto 16px' }} />
            <div style={{ fontSize: '16px', fontWeight: 600, marginBottom: '6px' }}>
              No Financial Events Found
            </div>
            <p style={{ color: 'var(--text-secondary)', fontSize: '13px', maxWidth: '400px', margin: '0 auto' }}>
              {statusFilter || categoryFilter || merchantQuery
                ? 'Try adjusting your filters to find events.'
                : 'Connect your Gmail account or create a manual transaction to populate the ledger.'}
            </p>
          </div>
        ) : (
          <div>
            <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontSize: '14px' }}>
              <thead>
                <tr
                  style={{
                    borderBottom: '1px solid var(--border-subtle)',
                    background: 'rgba(255, 255, 255, 0.02)',
                    color: 'var(--text-muted)',
                    fontSize: '12px',
                    textTransform: 'uppercase',
                    letterSpacing: '0.04em',
                  }}
                >
                  <th style={{ padding: '14px 20px' }}>Date & Merchant</th>
                  <th style={{ padding: '14px 20px' }}>Type & Category</th>
                  <th style={{ padding: '14px 20px' }}>Confidence & Source</th>
                  <th style={{ padding: '14px 20px', textAlign: 'right' }}>Amount</th>
                  <th style={{ padding: '14px 20px', textAlign: 'center' }}>Status</th>
                  <th style={{ padding: '14px 20px', textAlign: 'right' }}>Actions</th>
                </tr>
              </thead>
              <tbody>
                {events.map((ev) => {
                  const effAmount = ev.effectiveAmount || ev.originalAmount;
                  const effMerchant = ev.effectiveMerchant || ev.originalMerchant;
                  const effType = ev.effectiveEventType || ev.originalEventType;
                  const occurred = ev.occurredAt ? new Date(ev.occurredAt).toLocaleString() : '—';
                  const isPending = ev.status === 'pending_review';
                  const conf = ev.confidence?.aggregate ?? 0;
                  const confPct = Math.round(conf * 100);
                  const isHighConf = conf >= (ev.gateDecision?.threshold || 0.8);
                  const method = ev.provenance?.extractionMethod || 'manual';

                  return (
                    <tr
                      key={ev.id}
                      onClick={() => openDetailDrawer(ev.id)}
                      style={{
                        borderBottom: '1px solid var(--border-subtle)',
                        cursor: 'pointer',
                        transition: 'background 0.15s ease',
                      }}
                      onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(255, 255, 255, 0.03)')}
                      onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                    >
                      {/* Date & Merchant */}
                      <td style={{ padding: '14px 20px' }}>
                        <div style={{ fontWeight: 600, color: 'var(--text-primary)', marginBottom: '2px' }}>
                          {effMerchant?.rawName || 'Unknown Merchant'}
                        </div>
                        <div style={{ fontSize: '12px', color: 'var(--text-muted)', display: 'flex', alignItems: 'center', gap: '6px' }}>
                          <Clock size={11} /> {occurred}
                        </div>
                      </td>

                      {/* Type & Category */}
                      <td style={{ padding: '14px 20px' }}>
                        <div style={{ display: 'flex', flexDirection: 'column', gap: '4px', alignItems: 'flex-start' }}>
                          <span style={{ fontSize: '13px', fontWeight: 500, color: '#e2e8f0' }}>
                            {effType?.replace(/_/g, ' ').toUpperCase()}
                          </span>
                          {ev.category?.category && (
                            <span
                              style={{
                                display: 'inline-flex',
                                alignItems: 'center',
                                gap: '4px',
                                fontSize: '11px',
                                color: 'var(--text-secondary)',
                                background: 'rgba(255, 255, 255, 0.05)',
                                padding: '2px 8px',
                                borderRadius: 'var(--radius-full)',
                              }}
                            >
                              <Tag size={10} /> {ev.category.category}
                            </span>
                          )}
                        </div>
                      </td>

                      {/* Confidence & Source */}
                      <td style={{ padding: '14px 20px' }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                          {/* Method Badge */}
                          <span
                            className={`badge ${
                              method === 'template'
                                ? 'badge-cyan'
                                : method === 'ai'
                                ? 'badge-rose'
                                : 'badge-neutral'
                            }`}
                            style={{ fontSize: '10px', padding: '2px 6px' }}
                          >
                            {method === 'template' ? (
                              <Cpu size={10} />
                            ) : method === 'ai' ? (
                              <Sparkles size={10} />
                            ) : (
                              <User size={10} />
                            )}
                            <span>{method}</span>
                          </span>

                          {/* Confidence Indicator */}
                          <span
                            style={{
                              fontSize: '12px',
                              fontWeight: 600,
                              color: isHighConf ? '#34d399' : '#fbbf24',
                            }}
                          >
                            {confPct}%
                          </span>
                        </div>
                      </td>

                      {/* Amount */}
                      <td style={{ padding: '14px 20px', textAlign: 'right' }}>
                        <div
                          style={{
                            fontFamily: 'var(--font-mono)',
                            fontSize: '15px',
                            fontWeight: 700,
                            color:
                              effType === 'salary_income' || effType === 'reward_cashback' || effType === 'refund'
                                ? '#34d399'
                                : 'var(--text-primary)',
                          }}
                        >
                          {formatMoney(effAmount)}
                        </div>
                      </td>

                      {/* Status */}
                      <td style={{ padding: '14px 20px', textAlign: 'center' }}>
                        <span
                          className={`badge ${
                            ev.status === 'finalized'
                              ? 'badge-emerald'
                              : ev.status === 'pending_review'
                              ? 'badge-warning'
                              : 'badge-neutral'
                          }`}
                        >
                          {ev.status === 'finalized' && <CheckCircle size={12} />}
                          {ev.status === 'pending_review' && <AlertCircle size={12} />}
                          <span>{ev.status.replace('_', ' ')}</span>
                        </span>
                      </td>

                      {/* Row Actions */}
                      <td style={{ padding: '14px 20px', textAlign: 'right' }}>
                        <div style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
                          {isPending && (
                            <>
                              <button
                                onClick={(e) => handleQuickConfirm(e, ev.id)}
                                className="btn-secondary"
                                style={{
                                  padding: '5px 10px',
                                  fontSize: '11px',
                                  color: '#34d399',
                                  borderColor: 'rgba(16, 185, 129, 0.4)',
                                }}
                                title="One-click Confirm"
                              >
                                <Check size={12} /> Confirm
                              </button>
                              <button
                                onClick={(e) => {
                                  e.stopPropagation();
                                  openEditModal(ev, true);
                                }}
                                className="btn-secondary"
                                style={{
                                  padding: '5px 10px',
                                  fontSize: '11px',
                                  color: '#fbbf24',
                                  borderColor: 'rgba(245, 158, 11, 0.4)',
                                }}
                                title="Edit and Confirm"
                              >
                                <Edit3 size={12} /> Edit
                              </button>
                            </>
                          )}
                          {!isPending && (
                            <button
                              onClick={(e) => {
                                e.stopPropagation();
                                openEditModal(ev, false);
                              }}
                              className="btn-secondary"
                              style={{ padding: '5px 8px', fontSize: '11px' }}
                              title="Manual Override"
                            >
                              <Edit3 size={12} />
                            </button>
                          )}
                          <ChevronRight size={16} color="var(--text-muted)" />
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>

            {/* Pagination Button */}
            {nextCursor && (
              <div style={{ padding: '16px', textAlign: 'center', borderTop: '1px solid var(--border-subtle)' }}>
                <button
                  onClick={() => fetchEvents(false, nextCursor)}
                  className="btn-secondary"
                  disabled={loadingMore}
                  style={{ minWidth: '160px' }}
                >
                  {loadingMore ? (
                    <RefreshCw size={14} className="spin" />
                  ) : (
                    <span>Load More Transactions</span>
                  )}
                </button>
              </div>
            )}
          </div>
        )}
      </div>

      {/* DETAIL DRAWER */}
      {selectedEventId && (
        <div
          style={{
            position: 'fixed',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            background: 'rgba(0, 0, 0, 0.7)',
            backdropFilter: 'blur(6px)',
            zIndex: 50,
            display: 'flex',
            justifyContent: 'flex-end',
          }}
          onClick={closeDetailDrawer}
        >
          <div
            style={{
              width: '100%',
              maxWidth: '560px',
              height: '100%',
              background: 'var(--bg-secondary)',
              borderLeft: '1px solid var(--border-subtle)',
              display: 'flex',
              flexDirection: 'column',
              boxShadow: '-10px 0 30px rgba(0, 0, 0, 0.6)',
              overflowY: 'auto',
            }}
            onClick={(e) => e.stopPropagation()}
          >
            {/* Drawer Header */}
            <div
              style={{
                padding: '20px 24px',
                borderBottom: '1px solid var(--border-subtle)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                position: 'sticky',
                top: 0,
                background: 'rgba(13, 19, 34, 0.95)',
                backdropFilter: 'blur(8px)',
                zIndex: 10,
              }}
            >
              <div>
                <h2 style={{ fontSize: '18px', marginBottom: '2px' }}>Event Details</h2>
                <div style={{ fontSize: '12px', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
                  ID: {selectedEventId}
                </div>
              </div>

              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                {selectedEvent && (
                  <button
                    onClick={() => openEditModal(selectedEvent, selectedEvent.status === 'pending_review')}
                    className="btn-secondary"
                    style={{ padding: '6px 12px', fontSize: '12px' }}
                  >
                    <Edit3 size={13} />
                    <span>{selectedEvent.status === 'pending_review' ? 'Edit & Confirm' : 'Override'}</span>
                  </button>
                )}
                <button
                  onClick={closeDetailDrawer}
                  className="btn-secondary"
                  style={{ padding: '6px 10px' }}
                >
                  <X size={16} />
                </button>
              </div>
            </div>

            {/* Drawer Body */}
            <div style={{ padding: '24px', flex: 1 }}>
              {loadingDetail || !selectedEvent ? (
                <div style={{ padding: '40px', textAlign: 'center', color: 'var(--text-secondary)' }}>
                  <div className="spin" style={{ width: '28px', height: '28px', border: '3px solid rgba(255,255,255,0.1)', borderTopColor: 'var(--accent-emerald)', borderRadius: '50%', margin: '0 auto 12px' }} />
                  <span>Loading transaction specification...</span>
                </div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
                  {/* Summary Card */}
                  <div className="glass-card" style={{ padding: '20px' }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}>
                      <span className={`badge ${selectedEvent.status === 'finalized' ? 'badge-emerald' : selectedEvent.status === 'pending_review' ? 'badge-warning' : 'badge-neutral'}`}>
                        {selectedEvent.status.replace('_', ' ')}
                      </span>
                      <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>
                        Occurred: {new Date(selectedEvent.occurredAt).toLocaleString()}
                      </span>
                    </div>

                    <div style={{ marginBottom: '12px' }}>
                      <div style={{ fontSize: '12px', color: 'var(--text-muted)' }}>Effective Amount</div>
                      <div style={{ fontSize: '28px', fontWeight: 800, fontFamily: 'var(--font-mono)', color: 'var(--text-primary)' }}>
                        {formatMoney(selectedEvent.effectiveAmount || selectedEvent.originalAmount)}
                      </div>
                    </div>

                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px', fontSize: '13px' }}>
                      <div>
                        <div style={{ color: 'var(--text-muted)', fontSize: '11px' }}>Merchant</div>
                        <div style={{ fontWeight: 600, marginTop: '2px' }}>
                          {(selectedEvent.effectiveMerchant || selectedEvent.originalMerchant)?.rawName}
                        </div>
                      </div>
                      <div>
                        <div style={{ color: 'var(--text-muted)', fontSize: '11px' }}>Event Type</div>
                        <div style={{ fontWeight: 600, marginTop: '2px' }}>
                          {(selectedEvent.effectiveEventType || selectedEvent.originalEventType)?.replace(/_/g, ' ')}
                        </div>
                      </div>
                      <div>
                        <div style={{ color: 'var(--text-muted)', fontSize: '11px' }}>Category</div>
                        <div style={{ fontWeight: 600, marginTop: '2px' }}>
                          {selectedEvent.category?.category || 'Unassigned'}
                        </div>
                      </div>
                      <div>
                        <div style={{ color: 'var(--text-muted)', fontSize: '11px' }}>Payment Method</div>
                        <div style={{ fontWeight: 600, marginTop: '2px' }}>
                          {selectedEvent.paymentMethod?.type || '—'} {selectedEvent.paymentMethod?.hint ? `(${selectedEvent.paymentMethod.hint})` : ''}
                        </div>
                      </div>
                    </div>
                  </div>

                  {/* Provenance & Confidence Panel */}
                  <div className="glass-panel" style={{ padding: '20px' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '16px' }}>
                      <Sparkles size={16} color="#06b6d4" />
                      <h3 style={{ fontSize: '15px' }}>Extraction Provenance & Confidence Gate</h3>
                    </div>

                    <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: '12px', marginBottom: '16px', fontSize: '13px' }}>
                      <div className="glass-card" style={{ padding: '12px' }}>
                        <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>Extraction Method</div>
                        <div style={{ fontWeight: 600, marginTop: '4px', textTransform: 'capitalize' }}>
                          {selectedEvent.provenance?.extractionMethod || 'manual'}
                        </div>
                      </div>
                      <div className="glass-card" style={{ padding: '12px' }}>
                        <div style={{ fontSize: '11px', color: 'var(--text-muted)' }}>Aggregate Score</div>
                        <div style={{ fontWeight: 700, marginTop: '4px', color: (selectedEvent.confidence?.aggregate ?? 0) >= 0.8 ? '#34d399' : '#fbbf24' }}>
                          {Math.round((selectedEvent.confidence?.aggregate ?? 0) * 100)}% (Threshold: {Math.round((selectedEvent.gateDecision?.threshold || 0.8) * 100)}%)
                        </div>
                      </div>
                    </div>

                    {/* Breakdown bars */}
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', fontSize: '12px' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                        <span style={{ color: 'var(--text-secondary)' }}>Amount Confidence</span>
                        <span style={{ fontFamily: 'var(--font-mono)' }}>{Math.round((selectedEvent.confidence?.amount ?? 0) * 100)}%</span>
                      </div>
                      <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                        <span style={{ color: 'var(--text-secondary)' }}>Merchant Confidence</span>
                        <span style={{ fontFamily: 'var(--font-mono)' }}>{Math.round((selectedEvent.confidence?.merchant ?? 0) * 100)}%</span>
                      </div>
                      <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                        <span style={{ color: 'var(--text-secondary)' }}>Event Type Confidence</span>
                        <span style={{ fontFamily: 'var(--font-mono)' }}>{Math.round((selectedEvent.confidence?.eventType ?? 0) * 100)}%</span>
                      </div>
                      <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                        <span style={{ color: 'var(--text-secondary)' }}>Category Confidence</span>
                        <span style={{ fontFamily: 'var(--font-mono)' }}>{Math.round((selectedEvent.confidence?.category ?? 0) * 100)}%</span>
                      </div>
                    </div>
                  </div>

                  {/* Type-Specific Payload */}
                  {selectedEvent.typeSpecificPayload && Object.keys(selectedEvent.typeSpecificPayload).length > 0 && (
                    <div className="glass-panel" style={{ padding: '20px' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '14px' }}>
                        <Layers size={16} color="#a855f7" />
                        <h3 style={{ fontSize: '15px' }}>Type-Specific Payload</h3>
                      </div>
                      <pre
                        style={{
                          background: 'rgba(0, 0, 0, 0.4)',
                          padding: '12px',
                          borderRadius: 'var(--radius-sm)',
                          fontSize: '12px',
                          fontFamily: 'var(--font-mono)',
                          overflowX: 'auto',
                          color: '#38bdf8',
                        }}
                      >
                        {JSON.stringify(selectedEvent.typeSpecificPayload, null, 2)}
                      </pre>
                    </div>
                  )}

                  {/* Audit Trail of Overrides */}
                  <div className="glass-panel" style={{ padding: '20px' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '16px' }}>
                      <History size={16} color="#fb7185" />
                      <h3 style={{ fontSize: '15px' }}>Audit Trail (Event Overrides)</h3>
                    </div>

                    {overrides.length === 0 ? (
                      <div style={{ fontSize: '13px', color: 'var(--text-muted)' }}>
                        No manual corrections recorded for this event. Original extracted values remain intact.
                      </div>
                    ) : (
                      <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                        {overrides.map((ov) => (
                          <div
                            key={ov.id}
                            style={{
                              padding: '12px',
                              background: 'rgba(255, 255, 255, 0.02)',
                              border: '1px solid var(--border-subtle)',
                              borderRadius: 'var(--radius-sm)',
                              fontSize: '13px',
                            }}
                          >
                            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '6px' }}>
                              <span style={{ fontWeight: 600, color: 'var(--accent-cyan)' }}>
                                Field: {ov.fieldName}
                              </span>
                              <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>
                                {new Date(ov.createdAt).toLocaleString()}
                              </span>
                            </div>
                            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '8px', fontSize: '12px' }}>
                              <div style={{ color: '#fb7185', background: 'rgba(244, 63, 94, 0.08)', padding: '6px', borderRadius: '4px' }}>
                                <div style={{ fontSize: '10px', color: 'var(--text-muted)' }}>Before:</div>
                                <code>{ov.previousValue || '—'}</code>
                              </div>
                              <div style={{ color: '#34d399', background: 'rgba(16, 185, 129, 0.08)', padding: '6px', borderRadius: '4px' }}>
                                <div style={{ fontSize: '10px', color: 'var(--text-muted)' }}>After:</div>
                                <code>{ov.newValue || '—'}</code>
                              </div>
                            </div>
                            <div style={{ fontSize: '11px', color: 'var(--text-muted)', marginTop: '6px' }}>
                              Actor: {ov.appliedByUserId}
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* EDIT & OVERRIDE MODAL */}
      {isEditModalOpen && editTargetEvent && (
        <div
          style={{
            position: 'fixed',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            background: 'rgba(0, 0, 0, 0.7)',
            backdropFilter: 'blur(6px)',
            zIndex: 60,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            padding: '20px',
          }}
          onClick={() => setIsEditModalOpen(false)}
        >
          <div
            className="glass-panel"
            style={{
              width: '100%',
              maxWidth: '520px',
              padding: '28px',
              background: 'var(--bg-secondary)',
              position: 'relative',
            }}
            onClick={(e) => e.stopPropagation()}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
              <div>
                <h2 style={{ fontSize: '19px' }}>
                  {isConfirmingWithEdit ? 'Edit & Confirm Transaction' : 'Manual Event Override'}
                </h2>
                <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginTop: '2px' }}>
                  Audited correction adhering to SRS-FR-LEDGER-02 (non-destructive).
                </p>
              </div>
              <button
                onClick={() => setIsEditModalOpen(false)}
                className="btn-secondary"
                style={{ padding: '6px 10px' }}
              >
                <X size={16} />
              </button>
            </div>

            {overrideError && (
              <div
                style={{
                  padding: '12px',
                  marginBottom: '16px',
                  background: 'rgba(244, 63, 94, 0.1)',
                  border: '1px solid rgba(244, 63, 94, 0.3)',
                  borderRadius: 'var(--radius-sm)',
                  color: '#fb7185',
                  fontSize: '13px',
                }}
              >
                {overrideError}
              </div>
            )}

            <form onSubmit={handleSaveOverride}>
              {/* Amount & Currency */}
              <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr', gap: '12px', marginBottom: '16px' }}>
                <div className="form-group" style={{ marginBottom: 0 }}>
                  <label className="form-label">Amount (Decimal String)</label>
                  <input
                    type="text"
                    className="form-input"
                    value={overrideForm.amount}
                    onChange={(e) => setOverrideForm({ ...overrideForm, amount: e.target.value })}
                    placeholder="e.g. 50000.00"
                    required
                  />
                </div>
                <div className="form-group" style={{ marginBottom: 0 }}>
                  <label className="form-label">Currency</label>
                  <input
                    type="text"
                    className="form-input"
                    value={overrideForm.currency}
                    onChange={(e) => setOverrideForm({ ...overrideForm, currency: e.target.value.toUpperCase() })}
                    placeholder="IDR"
                    required
                  />
                </div>
              </div>

              {/* Merchant Name */}
              <div className="form-group">
                <label className="form-label">Merchant Name</label>
                <input
                  type="text"
                  className="form-input"
                  value={overrideForm.merchant}
                  onChange={(e) => setOverrideForm({ ...overrideForm, merchant: e.target.value })}
                  placeholder="Merchant name"
                  required
                />
              </div>

              {/* Event Type */}
              <div className="form-group">
                <label className="form-label">Event Type</label>
                <select
                  className="form-select"
                  value={overrideForm.eventType}
                  onChange={(e) => setOverrideForm({ ...overrideForm, eventType: e.target.value })}
                >
                  {EVENT_TYPES.map((t) => (
                    <option key={t.value} value={t.value}>
                      {t.label}
                    </option>
                  ))}
                </select>
              </div>

              {/* Category */}
              <div className="form-group">
                <label className="form-label">Category</label>
                <select
                  className="form-select"
                  value={overrideForm.category}
                  onChange={(e) => setOverrideForm({ ...overrideForm, category: e.target.value })}
                >
                  <option value="">Unassigned</option>
                  {CATEGORIES.map((cat) => (
                    <option key={cat} value={cat}>
                      {cat}
                    </option>
                  ))}
                </select>
              </div>

              {/* Side-by-side Before vs After Diff Preview */}
              <div
                style={{
                  padding: '12px 16px',
                  background: 'rgba(0, 0, 0, 0.3)',
                  border: '1px solid var(--border-subtle)',
                  borderRadius: 'var(--radius-sm)',
                  marginBottom: '20px',
                  fontSize: '12px',
                }}
              >
                <div style={{ color: 'var(--text-muted)', marginBottom: '8px', fontWeight: 600 }}>
                  Before vs After Comparison:
                </div>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '8px' }}>
                  <div>
                    <span style={{ color: 'var(--text-muted)' }}>Before: </span>
                    <span style={{ color: '#fb7185' }}>
                      {formatMoney(editTargetEvent.effectiveAmount || editTargetEvent.originalAmount)} •{' '}
                      {(editTargetEvent.effectiveMerchant || editTargetEvent.originalMerchant)?.rawName}
                    </span>
                  </div>
                  <div>
                    <span style={{ color: 'var(--text-muted)' }}>After: </span>
                    <span style={{ color: '#34d399', fontWeight: 600 }}>
                      {formatMoney({ value: overrideForm.amount || '0', currency: overrideForm.currency })} •{' '}
                      {overrideForm.merchant || '—'}
                    </span>
                  </div>
                </div>
              </div>

              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '12px' }}>
                <button
                  type="button"
                  onClick={() => setIsEditModalOpen(false)}
                  className="btn-secondary"
                  disabled={submittingOverride}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  className="btn-primary"
                  disabled={submittingOverride}
                  style={{ minWidth: '140px' }}
                >
                  {submittingOverride ? (
                    <RefreshCw size={14} className="spin" />
                  ) : isConfirmingWithEdit ? (
                    'Confirm & Save'
                  ) : (
                    'Save Override'
                  )}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* CREATE MANUAL EVENT MODAL */}
      {isCreateModalOpen && (
        <div
          style={{
            position: 'fixed',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            background: 'rgba(0, 0, 0, 0.7)',
            backdropFilter: 'blur(6px)',
            zIndex: 60,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            padding: '20px',
          }}
          onClick={() => setIsCreateModalOpen(false)}
        >
          <div
            className="glass-panel"
            style={{
              width: '100%',
              maxWidth: '520px',
              padding: '28px',
              background: 'var(--bg-secondary)',
              position: 'relative',
            }}
            onClick={(e) => e.stopPropagation()}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '20px' }}>
              <div>
                <h2 style={{ fontSize: '19px' }}>Create Manual Transaction</h2>
                <p style={{ fontSize: '12px', color: 'var(--text-muted)', marginTop: '2px' }}>
                  User-created financial ledger entry with full confidence and immediate finalization.
                </p>
              </div>
              <button
                onClick={() => setIsCreateModalOpen(false)}
                className="btn-secondary"
                style={{ padding: '6px 10px' }}
              >
                <X size={16} />
              </button>
            </div>

            {createError && (
              <div
                style={{
                  padding: '12px',
                  marginBottom: '16px',
                  background: 'rgba(244, 63, 94, 0.1)',
                  border: '1px solid rgba(244, 63, 94, 0.3)',
                  borderRadius: 'var(--radius-sm)',
                  color: '#fb7185',
                  fontSize: '13px',
                }}
              >
                {createError}
              </div>
            )}

            <form onSubmit={handleCreateManualEvent}>
              {/* Amount & Currency */}
              <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr', gap: '12px', marginBottom: '16px' }}>
                <div className="form-group" style={{ marginBottom: 0 }}>
                  <label className="form-label">Amount (Decimal String)</label>
                  <input
                    type="text"
                    className="form-input"
                    value={createForm.amount}
                    onChange={(e) => setCreateForm({ ...createForm, amount: e.target.value })}
                    placeholder="e.g. 50000.00"
                    required
                  />
                </div>
                <div className="form-group" style={{ marginBottom: 0 }}>
                  <label className="form-label">Currency</label>
                  <input
                    type="text"
                    className="form-input"
                    value={createForm.currency}
                    onChange={(e) => setCreateForm({ ...createForm, currency: e.target.value.toUpperCase() })}
                    placeholder="IDR"
                    required
                  />
                </div>
              </div>

              {/* Merchant Name */}
              <div className="form-group">
                <label className="form-label">Merchant Name</label>
                <input
                  type="text"
                  className="form-input"
                  value={createForm.merchantName}
                  onChange={(e) => setCreateForm({ ...createForm, merchantName: e.target.value })}
                  placeholder="e.g. Kopi Kenangan"
                  required
                />
              </div>

              {/* Event Type & Category */}
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px', marginBottom: '16px' }}>
                <div className="form-group" style={{ marginBottom: 0 }}>
                  <label className="form-label">Event Type</label>
                  <select
                    className="form-select"
                    value={createForm.eventType}
                    onChange={(e) => setCreateForm({ ...createForm, eventType: e.target.value })}
                  >
                    {EVENT_TYPES.map((t) => (
                      <option key={t.value} value={t.value}>
                        {t.label}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="form-group" style={{ marginBottom: 0 }}>
                  <label className="form-label">Category</label>
                  <select
                    className="form-select"
                    value={createForm.category}
                    onChange={(e) => setCreateForm({ ...createForm, category: e.target.value })}
                  >
                    {CATEGORIES.map((cat) => (
                      <option key={cat} value={cat}>
                        {cat}
                      </option>
                    ))}
                  </select>
                </div>
              </div>

              {/* Date & Payment Method */}
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px', marginBottom: '24px' }}>
                <div className="form-group" style={{ marginBottom: 0 }}>
                  <label className="form-label">Date & Time</label>
                  <input
                    type="datetime-local"
                    className="form-input"
                    value={createForm.occurredAt}
                    onChange={(e) => setCreateForm({ ...createForm, occurredAt: e.target.value })}
                    required
                  />
                </div>
                <div className="form-group" style={{ marginBottom: 0 }}>
                  <label className="form-label">Payment Method</label>
                  <select
                    className="form-select"
                    value={createForm.paymentMethodType}
                    onChange={(e) => setCreateForm({ ...createForm, paymentMethodType: e.target.value })}
                  >
                    <option value="qris">QRIS</option>
                    <option value="bank_transfer">Bank Transfer</option>
                    <option value="e_wallet">E-Wallet</option>
                    <option value="credit_card">Credit Card</option>
                    <option value="debit_card">Debit Card</option>
                    <option value="cash">Cash</option>
                  </select>
                </div>
              </div>

              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '12px' }}>
                <button
                  type="button"
                  onClick={() => setIsCreateModalOpen(false)}
                  className="btn-secondary"
                  disabled={submittingCreate}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  className="btn-primary"
                  disabled={submittingCreate}
                  style={{ minWidth: '150px' }}
                >
                  {submittingCreate ? (
                    <RefreshCw size={14} className="spin" />
                  ) : (
                    'Record Transaction'
                  )}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
