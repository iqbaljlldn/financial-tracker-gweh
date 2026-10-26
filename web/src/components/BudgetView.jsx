import React, { useState, useEffect, useCallback } from 'react';
import {
  Wallet,
  Target,
  Plus,
  RefreshCw,
  AlertTriangle,
  CheckCircle,
  TrendingUp,
  Tag,
  Globe,
  Calendar,
  Trash2,
  Edit2,
  Sliders,
  X,
  AlertCircle,
  Layers,
  ArrowRight,
  Sparkles,
} from 'lucide-react';
import { apiClient } from '../api/client';

export function BudgetView() {
  const [activeTab, setActiveTab] = useState('budgets'); // 'budgets' | 'goals'
  const [budgets, setBudgets] = useState([]);
  const [savingsGoals, setSavingsGoals] = useState([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [includeInactive, setIncludeInactive] = useState(false);
  const [errorMessage, setErrorMessage] = useState(null);

  // Budget modal state
  const [isBudgetModalOpen, setIsBudgetModalOpen] = useState(false);
  const [editingBudget, setEditingBudget] = useState(null);
  const [budgetForm, setBudgetForm] = useState({
    scopeType: 'global', // 'global' | 'category'
    categoryName: '',
    amountValue: '',
    currency: 'IDR',
    alertThresholds: [0.8, 1.0],
    active: true,
  });

  // Savings Goal modal state
  const [isGoalModalOpen, setIsGoalModalOpen] = useState(false);
  const [editingGoal, setEditingGoal] = useState(null);
  const [goalForm, setGoalForm] = useState({
    targetAmountValue: '',
    currency: 'IDR',
    targetDate: '',
  });

  // Action status (saving/deleting)
  const [submitting, setSubmitting] = useState(false);

  const fetchData = useCallback(async (isSilent = false) => {
    try {
      if (!isSilent) setLoading(true);
      else setRefreshing(true);
      setErrorMessage(null);

      const [budgetsRes, goalsRes] = await Promise.all([
        apiClient.listBudgets(includeInactive),
        apiClient.listSavingsGoals(),
      ]);

      setBudgets(budgetsRes?.items || []);
      setSavingsGoals(goalsRes?.items || []);
    } catch (err) {
      console.error('Failed to fetch budget data:', err);
      setErrorMessage(err.message || 'Failed to load budget data');
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [includeInactive]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  // --- Budget Actions ---
  const handleOpenCreateBudget = () => {
    setEditingBudget(null);
    setBudgetForm({
      scopeType: 'global',
      categoryName: '',
      amountValue: '',
      currency: 'IDR',
      alertThresholds: [0.8, 1.0],
      active: true,
    });
    setIsBudgetModalOpen(true);
  };

  const handleOpenEditBudget = (b) => {
    setEditingBudget(b);
    const isCategory = b.scope?.startsWith('category:');
    setBudgetForm({
      scopeType: isCategory ? 'category' : 'global',
      categoryName: isCategory ? b.scope.replace('category:', '') : '',
      amountValue: b.thresholdAmount?.value || '',
      currency: b.thresholdAmount?.currency || 'IDR',
      alertThresholds: b.alertThresholds || [0.8, 1.0],
      active: b.active ?? true,
    });
    setIsBudgetModalOpen(true);
  };

  const handleSaveBudget = async (e) => {
    e.preventDefault();
    try {
      setSubmitting(true);
      const val = parseFloat(budgetForm.amountValue);
      if (isNaN(val) || val <= 0) {
        throw new Error('Please enter a valid positive budget amount');
      }

      let scope = 'global';
      if (budgetForm.scopeType === 'category') {
        const cat = budgetForm.categoryName.trim();
        if (!cat) throw new Error('Category name cannot be empty');
        scope = `category:${cat}`;
      }

      const payload = {
        scope,
        periodType: 'monthly',
        thresholdAmount: {
          value: val,
          currency: budgetForm.currency.toUpperCase(),
        },
        alertThresholds: budgetForm.alertThresholds,
      };

      if (editingBudget) {
        payload.active = budgetForm.active;
        await apiClient.updateBudget(editingBudget.id, payload);
      } else {
        await apiClient.createBudget(payload);
      }

      setIsBudgetModalOpen(false);
      await fetchData(true);
    } catch (err) {
      setErrorMessage(err.message || 'Failed to save budget');
    } finally {
      setSubmitting(false);
    }
  };

  const handleDeleteBudget = async (id) => {
    if (!window.confirm('Are you sure you want to delete this budget limit?')) return;
    try {
      setSubmitting(true);
      await apiClient.deleteBudget(id);
      await fetchData(true);
    } catch (err) {
      setErrorMessage(err.message || 'Failed to delete budget');
    } finally {
      setSubmitting(false);
    }
  };

  // --- Savings Goal Actions ---
  const handleOpenCreateGoal = () => {
    setEditingGoal(null);
    setGoalForm({
      targetAmountValue: '',
      currency: 'IDR',
      targetDate: '',
    });
    setIsGoalModalOpen(true);
  };

  const handleOpenEditGoal = (g) => {
    setEditingGoal(g);
    setGoalForm({
      targetAmountValue: g.targetAmount?.value || '',
      currency: g.targetAmount?.currency || 'IDR',
      targetDate: g.targetDate || '',
    });
    setIsGoalModalOpen(true);
  };

  const handleSaveGoal = async (e) => {
    e.preventDefault();
    try {
      setSubmitting(true);
      const val = parseFloat(goalForm.targetAmountValue);
      if (isNaN(val) || val <= 0) {
        throw new Error('Please enter a valid positive target amount');
      }
      if (!goalForm.targetDate) {
        throw new Error('Please select a target completion date');
      }

      const payload = {
        targetAmount: {
          value: val,
          currency: goalForm.currency.toUpperCase(),
        },
        targetDate: goalForm.targetDate,
      };

      if (editingGoal) {
        await apiClient.updateSavingsGoal(editingGoal.id, payload);
      } else {
        await apiClient.createSavingsGoal(payload);
      }

      setIsGoalModalOpen(false);
      await fetchData(true);
    } catch (err) {
      setErrorMessage(err.message || 'Failed to save savings goal');
    } finally {
      setSubmitting(false);
    }
  };

  const handleDeleteGoal = async (id) => {
    if (!window.confirm('Are you sure you want to delete this savings goal?')) return;
    try {
      setSubmitting(true);
      await apiClient.deleteSavingsGoal(id);
      await fetchData(true);
    } catch (err) {
      setErrorMessage(err.message || 'Failed to delete savings goal');
    } finally {
      setSubmitting(false);
    }
  };

  // Format currency numbers
  const formatMoney = (val, currency = 'IDR') => {
    const num = Number(val || 0);
    return new Intl.NumberFormat('id-ID', {
      style: 'currency',
      currency: currency,
      maximumFractionDigits: 2,
    }).format(num);
  };

  return (
    <div style={{ maxWidth: '1200px', margin: '0 auto', display: 'flex', flexDirection: 'column', gap: '28px' }}>
      {/* Top Header */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: '16px' }}>
        <div>
          <h1 style={{ fontSize: '26px', fontWeight: 800, marginBottom: '6px', letterSpacing: '-0.02em' }}>
            Budgets & Savings Goals
          </h1>
          <p style={{ color: 'var(--text-secondary)', fontSize: '14px' }}>
            Proactive threshold alerting reacting to ledger finalized expenses with automated idempotency.
          </p>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
          <button
            onClick={() => fetchData(true)}
            className="btn-secondary"
            style={{ padding: '8px 14px', fontSize: '13px' }}
            disabled={refreshing}
            title="Refresh budget statistics"
          >
            <RefreshCw size={14} className={refreshing ? 'spin' : ''} />
            <span>Refresh</span>
          </button>

          <button
            onClick={activeTab === 'budgets' ? handleOpenCreateBudget : handleOpenCreateGoal}
            className="btn-primary"
            style={{ padding: '8px 18px', fontSize: '13px', display: 'flex', alignItems: 'center', gap: '8px' }}
          >
            <Plus size={16} />
            <span>{activeTab === 'budgets' ? 'New Budget' : 'New Savings Goal'}</span>
          </button>
        </div>
      </div>

      {/* Error Banner */}
      {errorMessage && (
        <div
          style={{
            padding: '14px 18px',
            background: 'rgba(239, 68, 68, 0.12)',
            border: '1px solid rgba(239, 68, 68, 0.3)',
            borderRadius: 'var(--radius-md)',
            color: '#f87171',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            fontSize: '14px',
          }}
        >
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
            <AlertCircle size={18} />
            <span>{errorMessage}</span>
          </div>
          <button
            onClick={() => setErrorMessage(null)}
            style={{ background: 'transparent', border: 'none', color: '#f87171', cursor: 'pointer' }}
          >
            <X size={16} />
          </button>
        </div>
      )}

      {/* Tab Navigation */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', borderBottom: '1px solid var(--border-subtle)', paddingBottom: '12px' }}>
        <div style={{ display: 'flex', gap: '8px' }}>
          <button
            onClick={() => setActiveTab('budgets')}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              padding: '8px 18px',
              borderRadius: 'var(--radius-full)',
              border: 'none',
              background: activeTab === 'budgets' ? 'rgba(16, 185, 129, 0.15)' : 'transparent',
              color: activeTab === 'budgets' ? '#34d399' : 'var(--text-secondary)',
              fontWeight: 600,
              fontSize: '14px',
              cursor: 'pointer',
              transition: 'all 0.15s ease',
            }}
          >
            <Wallet size={16} />
            <span>Budgets</span>
            <span
              style={{
                fontSize: '11px',
                padding: '2px 8px',
                borderRadius: '12px',
                background: activeTab === 'budgets' ? '#10b981' : 'rgba(255, 255, 255, 0.1)',
                color: activeTab === 'budgets' ? '#070a13' : 'var(--text-muted)',
                fontWeight: 700,
              }}
            >
              {budgets.length}
            </span>
          </button>

          <button
            onClick={() => setActiveTab('goals')}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              padding: '8px 18px',
              borderRadius: 'var(--radius-full)',
              border: 'none',
              background: activeTab === 'goals' ? 'rgba(56, 189, 248, 0.15)' : 'transparent',
              color: activeTab === 'goals' ? '#38bdf8' : 'var(--text-secondary)',
              fontWeight: 600,
              fontSize: '14px',
              cursor: 'pointer',
              transition: 'all 0.15s ease',
            }}
          >
            <Target size={16} />
            <span>Savings Goals</span>
            <span
              style={{
                fontSize: '11px',
                padding: '2px 8px',
                borderRadius: '12px',
                background: activeTab === 'goals' ? '#38bdf8' : 'rgba(255, 255, 255, 0.1)',
                color: activeTab === 'goals' ? '#070a13' : 'var(--text-muted)',
                fontWeight: 700,
              }}
            >
              {savingsGoals.length}
            </span>
          </button>
        </div>

        {activeTab === 'budgets' && (
          <label style={{ display: 'flex', alignItems: 'center', gap: '8px', fontSize: '13px', color: 'var(--text-secondary)', cursor: 'pointer' }}>
            <input
              type="checkbox"
              checked={includeInactive}
              onChange={(e) => setIncludeInactive(e.target.checked)}
              style={{ accentColor: '#10b981' }}
            />
            <span>Include inactive budgets</span>
          </label>
        )}
      </div>

      {/* Main Tab Content */}
      {loading ? (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))', gap: '20px' }}>
          {[1, 2, 3].map((i) => (
            <div key={i} className="glass-card" style={{ height: '220px', padding: '24px', opacity: 0.6 }} />
          ))}
        </div>
      ) : activeTab === 'budgets' ? (
        budgets.length === 0 ? (
          <div className="glass-card" style={{ padding: '48px', textAlign: 'center' }}>
            <Wallet size={48} style={{ margin: '0 auto 16px', opacity: 0.4, color: '#10b981' }} />
            <h3 style={{ fontSize: '18px', fontWeight: 700, marginBottom: '8px' }}>No Budgets Configured</h3>
            <p style={{ color: 'var(--text-secondary)', fontSize: '14px', maxWidth: '420px', margin: '0 auto 20px' }}>
              Set monthly spending targets for all expenses or specific categories to receive automated threshold warnings.
            </p>
            <button onClick={handleOpenCreateBudget} className="btn-primary" style={{ padding: '10px 20px', fontSize: '14px' }}>
              Create Your First Budget
            </button>
          </div>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(340px, 1fr))', gap: '20px' }}>
            {budgets.map((b) => {
              const percent = b.percentUsed || 0;
              const isOver = percent >= 100;
              const isWarn = percent >= 80 && percent < 100;

              let barGradient = 'linear-gradient(90deg, #10b981 0%, #06b6d4 100%)';
              let badgeColor = '#34d399';
              let badgeBg = 'rgba(16, 185, 129, 0.12)';
              let badgeText = 'Normal';

              if (isOver) {
                barGradient = 'linear-gradient(90deg, #f43f5e 0%, #e11d48 100%)';
                badgeColor = '#fb7185';
                badgeBg = 'rgba(244, 63, 94, 0.18)';
                badgeText = 'Limit Exceeded';
              } else if (isWarn) {
                barGradient = 'linear-gradient(90deg, #f59e0b 0%, #f97316 100%)';
                badgeColor = '#fbbf24';
                badgeBg = 'rgba(245, 158, 11, 0.18)';
                badgeText = 'Warning: Near Limit';
              }

              const isCategory = b.scope?.startsWith('category:');
              const scopeDisplay = isCategory ? b.scope.replace('category:', '') : 'Global Spend';

              return (
                <div
                  key={b.id}
                  className="glass-card"
                  style={{
                    padding: '24px',
                    display: 'flex',
                    flexDirection: 'column',
                    gap: '18px',
                    border: isOver ? '1px solid rgba(244, 63, 94, 0.35)' : undefined,
                    position: 'relative',
                  }}
                >
                  {/* Card Header */}
                  <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between' }}>
                    <div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '6px' }}>
                        <span
                          style={{
                            display: 'inline-flex',
                            alignItems: 'center',
                            gap: '5px',
                            padding: '3px 10px',
                            borderRadius: '12px',
                            background: isCategory ? 'rgba(147, 51, 234, 0.15)' : 'rgba(56, 189, 248, 0.15)',
                            color: isCategory ? '#c084fc' : '#38bdf8',
                            fontSize: '12px',
                            fontWeight: 600,
                          }}
                        >
                          {isCategory ? <Tag size={12} /> : <Globe size={12} />}
                          {scopeDisplay}
                        </span>

                        <span
                          style={{
                            padding: '3px 10px',
                            borderRadius: '12px',
                            background: badgeBg,
                            color: badgeColor,
                            fontSize: '11px',
                            fontWeight: 700,
                            letterSpacing: '0.02em',
                          }}
                        >
                          {badgeText}
                        </span>
                      </div>

                      <div style={{ fontSize: '13px', color: 'var(--text-muted)' }}>
                        Monthly Budget • {b.periodStart ? new Date(b.periodStart).toLocaleDateString(undefined, { month: 'long', year: 'numeric' }) : 'Current Month'}
                      </div>
                    </div>

                    <div style={{ display: 'flex', gap: '6px' }}>
                      <button
                        onClick={() => handleOpenEditBudget(b)}
                        style={{
                          padding: '6px',
                          borderRadius: 'var(--radius-sm)',
                          border: '1px solid var(--border-subtle)',
                          background: 'rgba(255, 255, 255, 0.05)',
                          color: 'var(--text-secondary)',
                          cursor: 'pointer',
                        }}
                        title="Edit Budget"
                      >
                        <Edit2 size={13} />
                      </button>
                      <button
                        onClick={() => handleDeleteBudget(b.id)}
                        style={{
                          padding: '6px',
                          borderRadius: 'var(--radius-sm)',
                          border: '1px solid rgba(244, 63, 94, 0.2)',
                          background: 'rgba(244, 63, 94, 0.08)',
                          color: '#fb7185',
                          cursor: 'pointer',
                        }}
                        title="Delete Budget"
                      >
                        <Trash2 size={13} />
                      </button>
                    </div>
                  </div>

                  {/* Amounts */}
                  <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between' }}>
                    <div>
                      <div style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '2px' }}>Current Spend</div>
                      <div style={{ fontSize: '22px', fontWeight: 800, fontFamily: 'var(--font-display)', color: isOver ? '#fb7185' : '#f8fafc' }}>
                        {formatMoney(b.currentSpend?.value, b.thresholdAmount?.currency)}
                      </div>
                    </div>
                    <div style={{ textAlign: 'right' }}>
                      <div style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '2px' }}>Limit</div>
                      <div style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-secondary)' }}>
                        {formatMoney(b.thresholdAmount?.value, b.thresholdAmount?.currency)}
                      </div>
                    </div>
                  </div>

                  {/* Progress Bar */}
                  <div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '12px', marginBottom: '6px' }}>
                      <span style={{ color: 'var(--text-muted)' }}>Usage</span>
                      <span style={{ fontWeight: 700, color: badgeColor }}>{percent.toFixed(1)}%</span>
                    </div>
                    <div
                      style={{
                        width: '100%',
                        height: '8px',
                        borderRadius: '999px',
                        background: 'rgba(255, 255, 255, 0.08)',
                        overflow: 'hidden',
                        position: 'relative',
                      }}
                    >
                      <div
                        style={{
                          width: `${Math.min(percent, 100)}%`,
                          height: '100%',
                          background: barGradient,
                          borderRadius: '999px',
                          transition: 'width 0.4s ease',
                        }}
                      />
                    </div>
                  </div>

                  {/* Alert Thresholds Footer */}
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', paddingTop: '10px', borderTop: '1px solid rgba(255, 255, 255, 0.06)', fontSize: '12px', color: 'var(--text-muted)' }}>
                    <span>Alerts at:</span>
                    <div style={{ display: 'flex', gap: '6px' }}>
                      {(b.alertThresholds || []).map((th) => (
                        <span
                          key={th}
                          style={{
                            padding: '2px 7px',
                            borderRadius: '4px',
                            background: 'rgba(255, 255, 255, 0.06)',
                            fontSize: '11px',
                            fontWeight: 600,
                          }}
                        >
                          {(th * 100).toFixed(0)}%
                        </span>
                      ))}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )
      ) : (
        /* Savings Goals Tab */
        savingsGoals.length === 0 ? (
          <div className="glass-card" style={{ padding: '48px', textAlign: 'center' }}>
            <Target size={48} style={{ margin: '0 auto 16px', opacity: 0.4, color: '#38bdf8' }} />
            <h3 style={{ fontSize: '18px', fontWeight: 700, marginBottom: '8px' }}>No Savings Goals Yet</h3>
            <p style={{ color: 'var(--text-secondary)', fontSize: '14px', maxWidth: '420px', margin: '0 auto 20px' }}>
              Define targets for emergency funds, vacations, or major purchases. Progress is dynamically calculated against your real net cash flow.
            </p>
            <button onClick={handleOpenCreateGoal} className="btn-primary" style={{ padding: '10px 20px', fontSize: '14px' }}>
              Create Savings Goal
            </button>
          </div>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(340px, 1fr))', gap: '20px' }}>
            {savingsGoals.map((g) => {
              const progress = g.progressPercentage || 0;
              const isAchieved = progress >= 100;
              const targetDateObj = new Date(g.targetDate);
              const daysLeft = Math.ceil((targetDateObj - new Date()) / (1000 * 60 * 60 * 24));

              return (
                <div
                  key={g.id}
                  className="glass-card"
                  style={{
                    padding: '24px',
                    display: 'flex',
                    flexDirection: 'column',
                    gap: '18px',
                    position: 'relative',
                    border: isAchieved ? '1px solid rgba(16, 185, 129, 0.4)' : undefined,
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between' }}>
                    <div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '6px' }}>
                        <span
                          style={{
                            display: 'inline-flex',
                            alignItems: 'center',
                            gap: '5px',
                            padding: '3px 10px',
                            borderRadius: '12px',
                            background: isAchieved ? 'rgba(16, 185, 129, 0.15)' : 'rgba(56, 189, 248, 0.15)',
                            color: isAchieved ? '#34d399' : '#38bdf8',
                            fontSize: '12px',
                            fontWeight: 700,
                          }}
                        >
                          <Target size={12} />
                          {isAchieved ? 'Goal Reached!' : 'In Progress'}
                        </span>

                        <span style={{ fontSize: '12px', color: 'var(--text-muted)' }}>
                          {daysLeft > 0 ? `${daysLeft} days remaining` : daysLeft === 0 ? 'Due today' : 'Past due'}
                        </span>
                      </div>

                      <div style={{ fontSize: '13px', color: 'var(--text-muted)', display: 'flex', alignItems: 'center', gap: '4px' }}>
                        <Calendar size={13} /> Target: {targetDateObj.toLocaleDateString(undefined, { dateStyle: 'medium' })}
                      </div>
                    </div>

                    <div style={{ display: 'flex', gap: '6px' }}>
                      <button
                        onClick={() => handleOpenEditGoal(g)}
                        style={{
                          padding: '6px',
                          borderRadius: 'var(--radius-sm)',
                          border: '1px solid var(--border-subtle)',
                          background: 'rgba(255, 255, 255, 0.05)',
                          color: 'var(--text-secondary)',
                          cursor: 'pointer',
                        }}
                        title="Edit Goal"
                      >
                        <Edit2 size={13} />
                      </button>
                      <button
                        onClick={() => handleDeleteGoal(g.id)}
                        style={{
                          padding: '6px',
                          borderRadius: 'var(--radius-sm)',
                          border: '1px solid rgba(244, 63, 94, 0.2)',
                          background: 'rgba(244, 63, 94, 0.08)',
                          color: '#fb7185',
                          cursor: 'pointer',
                        }}
                        title="Delete Goal"
                      >
                        <Trash2 size={13} />
                      </button>
                    </div>
                  </div>

                  {/* Amounts */}
                  <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between' }}>
                    <div>
                      <div style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '2px' }}>Current Net Savings</div>
                      <div style={{ fontSize: '22px', fontWeight: 800, fontFamily: 'var(--font-display)', color: '#38bdf8' }}>
                        {formatMoney(g.currentSavings?.value, g.targetAmount?.currency)}
                      </div>
                    </div>
                    <div style={{ textAlign: 'right' }}>
                      <div style={{ fontSize: '12px', color: 'var(--text-muted)', marginBottom: '2px' }}>Target</div>
                      <div style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-secondary)' }}>
                        {formatMoney(g.targetAmount?.value, g.targetAmount?.currency)}
                      </div>
                    </div>
                  </div>

                  {/* Progress Bar */}
                  <div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '12px', marginBottom: '6px' }}>
                      <span style={{ color: 'var(--text-muted)' }}>Net Cash Flow Progress</span>
                      <span style={{ fontWeight: 700, color: isAchieved ? '#34d399' : '#38bdf8' }}>{progress.toFixed(1)}%</span>
                    </div>
                    <div
                      style={{
                        width: '100%',
                        height: '8px',
                        borderRadius: '999px',
                        background: 'rgba(255, 255, 255, 0.08)',
                        overflow: 'hidden',
                      }}
                    >
                      <div
                        style={{
                          width: `${Math.min(progress, 100)}%`,
                          height: '100%',
                          background: isAchieved
                            ? 'linear-gradient(90deg, #10b981 0%, #059669 100%)'
                            : 'linear-gradient(90deg, #0284c7 0%, #38bdf8 100%)',
                          borderRadius: '999px',
                          transition: 'width 0.4s ease',
                        }}
                      />
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        )
      )}

      {/* --- Budget Modal --- */}
      {isBudgetModalOpen && (
        <div
          style={{
            position: 'fixed',
            inset: 0,
            background: 'rgba(0, 0, 0, 0.75)',
            backdropFilter: 'blur(8px)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 50,
            padding: '16px',
          }}
        >
          <div className="glass-card" style={{ maxWidth: '480px', width: '100%', padding: '28px' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '20px' }}>
              <h2 style={{ fontSize: '20px', fontWeight: 700 }}>{editingBudget ? 'Edit Budget' : 'Create Monthly Budget'}</h2>
              <button
                onClick={() => setIsBudgetModalOpen(false)}
                style={{ background: 'transparent', border: 'none', color: 'var(--text-muted)', cursor: 'pointer' }}
              >
                <X size={20} />
              </button>
            </div>

            <form onSubmit={handleSaveBudget} style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
              {/* Scope Selection */}
              <div>
                <label style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-secondary)', display: 'block', marginBottom: '8px' }}>
                  Budget Scope
                </label>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '10px' }}>
                  <button
                    type="button"
                    onClick={() => setBudgetForm({ ...budgetForm, scopeType: 'global' })}
                    style={{
                      padding: '10px',
                      borderRadius: 'var(--radius-sm)',
                      border: budgetForm.scopeType === 'global' ? '1px solid #10b981' : '1px solid var(--border-subtle)',
                      background: budgetForm.scopeType === 'global' ? 'rgba(16, 185, 129, 0.1)' : 'rgba(255, 255, 255, 0.03)',
                      color: budgetForm.scopeType === 'global' ? '#34d399' : 'var(--text-secondary)',
                      fontWeight: 600,
                      fontSize: '13px',
                      cursor: 'pointer',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      gap: '6px',
                    }}
                  >
                    <Globe size={14} /> Global (All Expenses)
                  </button>

                  <button
                    type="button"
                    onClick={() => setBudgetForm({ ...budgetForm, scopeType: 'category' })}
                    style={{
                      padding: '10px',
                      borderRadius: 'var(--radius-sm)',
                      border: budgetForm.scopeType === 'category' ? '1px solid #10b981' : '1px solid var(--border-subtle)',
                      background: budgetForm.scopeType === 'category' ? 'rgba(16, 185, 129, 0.1)' : 'rgba(255, 255, 255, 0.03)',
                      color: budgetForm.scopeType === 'category' ? '#34d399' : 'var(--text-secondary)',
                      fontWeight: 600,
                      fontSize: '13px',
                      cursor: 'pointer',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      gap: '6px',
                    }}
                  >
                    <Tag size={14} /> Specific Category
                  </button>
                </div>
              </div>

              {budgetForm.scopeType === 'category' && (
                <div>
                  <label style={{ fontSize: '13px', color: 'var(--text-secondary)', display: 'block', marginBottom: '6px' }}>
                    Category Name
                  </label>
                  <input
                    type="text"
                    value={budgetForm.categoryName}
                    onChange={(e) => setBudgetForm({ ...budgetForm, categoryName: e.target.value })}
                    placeholder="e.g. Food & Dining, Shopping, Utilities"
                    className="input-field"
                    required
                  />
                </div>
              )}

              {/* Amount & Currency */}
              <div>
                <label style={{ fontSize: '13px', color: 'var(--text-secondary)', display: 'block', marginBottom: '6px' }}>
                  Monthly Spending Limit
                </label>
                <div style={{ display: 'flex', gap: '10px' }}>
                  <input
                    type="number"
                    step="any"
                    value={budgetForm.amountValue}
                    onChange={(e) => setBudgetForm({ ...budgetForm, amountValue: e.target.value })}
                    placeholder="e.g. 2500000"
                    className="input-field"
                    style={{ flex: 1 }}
                    required
                  />
                  <select
                    value={budgetForm.currency}
                    onChange={(e) => setBudgetForm({ ...budgetForm, currency: e.target.value })}
                    className="input-field"
                    style={{ width: '90px' }}
                  >
                    <option value="IDR">IDR</option>
                    <option value="USD">USD</option>
                    <option value="SGD">SGD</option>
                    <option value="EUR">EUR</option>
                  </select>
                </div>
              </div>

              {/* Alert Thresholds */}
              <div>
                <label style={{ fontSize: '13px', color: 'var(--text-secondary)', display: 'block', marginBottom: '6px' }}>
                  Alert Notifications Trigger At
                </label>
                <div style={{ display: 'flex', gap: '8px' }}>
                  {[0.5, 0.8, 1.0].map((th) => {
                    const isChecked = budgetForm.alertThresholds.includes(th);
                    return (
                      <button
                        key={th}
                        type="button"
                        onClick={() => {
                          const next = isChecked
                            ? budgetForm.alertThresholds.filter((t) => t !== th)
                            : [...budgetForm.alertThresholds, th].sort();
                          if (next.length > 0) {
                            setBudgetForm({ ...budgetForm, alertThresholds: next });
                          }
                        }}
                        style={{
                          flex: 1,
                          padding: '8px',
                          borderRadius: 'var(--radius-sm)',
                          border: isChecked ? '1px solid #10b981' : '1px solid var(--border-subtle)',
                          background: isChecked ? 'rgba(16, 185, 129, 0.15)' : 'rgba(255, 255, 255, 0.03)',
                          color: isChecked ? '#34d399' : 'var(--text-muted)',
                          fontSize: '12px',
                          fontWeight: 600,
                          cursor: 'pointer',
                        }}
                      >
                        {(th * 100).toFixed(0)}% Limit
                      </button>
                    );
                  })}
                </div>
              </div>

              {/* Active Toggle for editing */}
              {editingBudget && (
                <label style={{ display: 'flex', alignItems: 'center', gap: '8px', fontSize: '13px', color: 'var(--text-secondary)', cursor: 'pointer', marginTop: '6px' }}>
                  <input
                    type="checkbox"
                    checked={budgetForm.active}
                    onChange={(e) => setBudgetForm({ ...budgetForm, active: e.target.checked })}
                    style={{ accentColor: '#10b981' }}
                  />
                  <span>Budget limit active and monitoring spend</span>
                </label>
              )}

              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', marginTop: '12px' }}>
                <button
                  type="button"
                  onClick={() => setIsBudgetModalOpen(false)}
                  className="btn-secondary"
                  style={{ padding: '8px 16px', fontSize: '13px' }}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  className="btn-primary"
                  style={{ padding: '8px 20px', fontSize: '13px' }}
                  disabled={submitting}
                >
                  {submitting ? 'Saving...' : editingBudget ? 'Update Budget' : 'Create Budget'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* --- Savings Goal Modal --- */}
      {isGoalModalOpen && (
        <div
          style={{
            position: 'fixed',
            inset: 0,
            background: 'rgba(0, 0, 0, 0.75)',
            backdropFilter: 'blur(8px)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 50,
            padding: '16px',
          }}
        >
          <div className="glass-card" style={{ maxWidth: '440px', width: '100%', padding: '28px' }}>
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '20px' }}>
              <h2 style={{ fontSize: '20px', fontWeight: 700 }}>{editingGoal ? 'Edit Savings Goal' : 'New Savings Goal'}</h2>
              <button
                onClick={() => setIsGoalModalOpen(false)}
                style={{ background: 'transparent', border: 'none', color: 'var(--text-muted)', cursor: 'pointer' }}
              >
                <X size={20} />
              </button>
            </div>

            <form onSubmit={handleSaveGoal} style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
              <div>
                <label style={{ fontSize: '13px', color: 'var(--text-secondary)', display: 'block', marginBottom: '6px' }}>
                  Target Savings Amount
                </label>
                <div style={{ display: 'flex', gap: '10px' }}>
                  <input
                    type="number"
                    step="any"
                    value={goalForm.targetAmountValue}
                    onChange={(e) => setGoalForm({ ...goalForm, targetAmountValue: e.target.value })}
                    placeholder="e.g. 10000000"
                    className="input-field"
                    style={{ flex: 1 }}
                    required
                  />
                  <select
                    value={goalForm.currency}
                    onChange={(e) => setGoalForm({ ...goalForm, currency: e.target.value })}
                    className="input-field"
                    style={{ width: '90px' }}
                  >
                    <option value="IDR">IDR</option>
                    <option value="USD">USD</option>
                    <option value="SGD">SGD</option>
                    <option value="EUR">EUR</option>
                  </select>
                </div>
              </div>

              <div>
                <label style={{ fontSize: '13px', color: 'var(--text-secondary)', display: 'block', marginBottom: '6px' }}>
                  Target Completion Date
                </label>
                <input
                  type="date"
                  value={goalForm.targetDate}
                  onChange={(e) => setGoalForm({ ...goalForm, targetDate: e.target.value })}
                  className="input-field"
                  required
                />
              </div>

              <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '10px', marginTop: '12px' }}>
                <button
                  type="button"
                  onClick={() => setIsGoalModalOpen(false)}
                  className="btn-secondary"
                  style={{ padding: '8px 16px', fontSize: '13px' }}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  className="btn-primary"
                  style={{ padding: '8px 20px', fontSize: '13px' }}
                  disabled={submitting}
                >
                  {submitting ? 'Saving...' : editingGoal ? 'Update Goal' : 'Create Goal'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
export default BudgetView;
