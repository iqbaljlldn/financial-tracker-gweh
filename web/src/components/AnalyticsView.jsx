import React, { useState, useEffect, useMemo, useCallback } from 'react';
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
  Area,
  AreaChart,
} from 'recharts';
import {
  TrendingUp,
  PieChart as PieChartIcon,
  BarChart3,
  ShoppingBag,
  Calendar,
  RefreshCw,
  Coins,
  AlertCircle,
} from 'lucide-react';
import { apiClient } from '../api/client';

const CATEGORY_COLORS = [
  '#10b981', // emerald
  '#06b6d4', // cyan
  '#6366f1', // indigo
  '#f59e0b', // amber
  '#ec4899', // pink
  '#8b5cf6', // purple
  '#14b8a6', // teal
  '#f97316', // orange
  '#3b82f6', // blue
  '#ef4444', // red
];

function formatCurrency(amountStr, currency) {
  const num = parseFloat(amountStr) || 0;
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency: currency || 'USD',
      maximumFractionDigits: 2,
    }).format(num);
  } catch {
    return `${currency || 'USD'} ${num.toFixed(2)}`;
  }
}

function formatRelativeTime(dateString) {
  if (!dateString) return 'Never';
  const date = new Date(dateString);
  const now = new Date();
  const diffSec = Math.floor((now - date) / 1000);

  if (diffSec < 5) return 'just now';
  if (diffSec < 60) return `${diffSec}s ago`;
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m ago`;
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 24) return `${diffHr}h ago`;
  return date.toLocaleDateString();
}

export default function AnalyticsView() {
  const [activeSection, setActiveSection] = useState('cashflow'); // 'cashflow' | 'categories' | 'merchants' | 'trends'
  const [granularity, setGranularity] = useState('monthly'); // 'monthly' | 'daily'
  const [selectedCurrency, setSelectedCurrency] = useState('ALL');
  const [merchantSortBy, setMerchantSortBy] = useState('totalValue'); // 'totalValue' | 'transactionCount'

  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState(null);

  const [cashFlowData, setCashFlowData] = useState([]);
  const [categoryData, setCategoryData] = useState([]);
  const [merchantData, setMerchantData] = useState([]);
  const [trendData, setTrendData] = useState([]);
  const [freshness, setFreshness] = useState(null);

  const loadData = useCallback(async (isSilent = false) => {
    if (!isSilent) setLoading(true);
    else setRefreshing(true);
    setError(null);

    try {
      const [cfRes, catRes, merchRes, trRes] = await Promise.all([
        apiClient.getCashFlow({ granularity }),
        apiClient.getCategoryBreakdown(),
        apiClient.getMerchantRanking({ limit: 20 }),
        apiClient.getTrend({ granularity }),
      ]);

      setCashFlowData(cfRes.buckets || []);
      setCategoryData(catRes.items || []);
      setMerchantData(merchRes.items || []);
      setTrendData(trRes.items || []);

      // Freshness from any endpoint (backed by projection_checkpoint)
      setFreshness(cfRes.projectionFreshness || catRes.projectionFreshness || trRes.projectionFreshness);
    } catch (err) {
      setError(err.message || 'Failed to fetch analytics data');
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [granularity]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  // Discover all unique currencies across projections
  const availableCurrencies = useMemo(() => {
    const set = new Set();
    cashFlowData.forEach((d) => d.currency && set.add(d.currency));
    categoryData.forEach((d) => d.currency && set.add(d.currency));
    merchantData.forEach((d) => d.currency && set.add(d.currency));
    trendData.forEach((d) => d.currency && set.add(d.currency));
    return Array.from(set).sort();
  }, [cashFlowData, categoryData, merchantData, trendData]);

  // If a specific currency was selected that no longer exists, reset to ALL
  useEffect(() => {
    if (selectedCurrency !== 'ALL' && !availableCurrencies.includes(selectedCurrency)) {
      setSelectedCurrency('ALL');
    }
  }, [availableCurrencies, selectedCurrency]);

  // Filtered views by currency (strictly partitioned, never blended)
  const filteredCashFlow = useMemo(() => {
    if (selectedCurrency === 'ALL') return cashFlowData;
    return cashFlowData.filter((d) => d.currency === selectedCurrency);
  }, [cashFlowData, selectedCurrency]);

  const filteredCategories = useMemo(() => {
    if (selectedCurrency === 'ALL') return categoryData;
    return categoryData.filter((d) => d.currency === selectedCurrency);
  }, [categoryData, selectedCurrency]);

  const filteredMerchants = useMemo(() => {
    let list = selectedCurrency === 'ALL' ? merchantData : merchantData.filter((d) => d.currency === selectedCurrency);
    return [...list].sort((a, b) => {
      if (merchantSortBy === 'transactionCount') {
        return (b.transactionCount || 0) - (a.transactionCount || 0);
      }
      return (parseFloat(b.totalValue) || 0) - (parseFloat(a.totalValue) || 0);
    });
  }, [merchantData, selectedCurrency, merchantSortBy]);

  const filteredTrends = useMemo(() => {
    if (selectedCurrency === 'ALL') return trendData;
    return trendData.filter((d) => d.currency === selectedCurrency);
  }, [trendData, selectedCurrency]);

  // Totals per currency for KPI cards
  const currencyTotals = useMemo(() => {
    const totals = {};
    cashFlowData.forEach((bucket) => {
      const c = bucket.currency || 'USD';
      if (!totals[c]) totals[c] = { income: 0, expense: 0, net: 0 };
      totals[c].income += parseFloat(bucket.totalIncome) || 0;
      totals[c].expense += parseFloat(bucket.totalExpense) || 0;
      totals[c].net = totals[c].income - totals[c].expense;
    });
    return totals;
  }, [cashFlowData]);

  // Prepare chart data for Cash Flow
  const chartCashFlowData = useMemo(() => {
    // When a single currency is selected, simple timeline
    if (selectedCurrency !== 'ALL') {
      return filteredCashFlow.map((item) => ({
        period: item.periodBucket,
        income: parseFloat(item.totalIncome) || 0,
        expense: parseFloat(item.totalExpense) || 0,
        net: (parseFloat(item.totalIncome) || 0) - (parseFloat(item.totalExpense) || 0),
        currency: item.currency,
      }));
    }

    // When ALL currencies are selected, partition label with currency
    return filteredCashFlow.map((item) => ({
      period: `${item.periodBucket} (${item.currency})`,
      income: parseFloat(item.totalIncome) || 0,
      expense: parseFloat(item.totalExpense) || 0,
      net: (parseFloat(item.totalIncome) || 0) - (parseFloat(item.totalExpense) || 0),
      currency: item.currency,
    }));
  }, [filteredCashFlow, selectedCurrency]);

  // Prepare chart data for Categories
  const chartCategoryData = useMemo(() => {
    // Group by category if multi-period
    const catMap = {};
    filteredCategories.forEach((item) => {
      const key = `${item.category}-${item.currency}`;
      if (!catMap[key]) {
        catMap[key] = {
          name: item.category,
          currency: item.currency,
          value: 0,
          percentageShare: item.percentageShare || 0,
        };
      }
      catMap[key].value += parseFloat(item.totalValue) || 0;
    });
    return Object.values(catMap).sort((a, b) => b.value - a.value);
  }, [filteredCategories]);

  // Prepare chart data for Trend
  const chartTrendData = useMemo(() => {
    return filteredTrends.map((item) => ({
      period: selectedCurrency === 'ALL' ? `${item.periodBucket} (${item.currency})` : item.periodBucket,
      netFlow: parseFloat(item.netFlow) || 0,
      currency: item.currency,
    }));
  }, [filteredTrends, selectedCurrency]);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
      {/* Top Header & Controls */}
      <div
        style={{
          display: 'flex',
          flexWrap: 'wrap',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: '16px',
        }}
      >
        <div>
          <h1 style={{ fontSize: '24px', fontWeight: 700, marginBottom: '4px', display: 'flex', alignItems: 'center', gap: '10px' }}>
            <BarChart3 size={24} color="#10b981" />
            Analytics & Ledger Insights
          </h1>
          <p style={{ color: 'var(--text-secondary)', fontSize: '14px', margin: 0 }}>
            Read-optimized CQRS projections computed directly from your finalized ledger events.
          </p>
        </div>

        {/* Right side freshness & sync */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
          {/* Subtle Projection Freshness Badge */}
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              padding: '6px 14px',
              borderRadius: 'var(--radius-full)',
              background: 'rgba(16, 185, 129, 0.08)',
              border: '1px solid rgba(16, 185, 129, 0.25)',
              fontSize: '12px',
              color: '#34d399',
            }}
            title={freshness ? `Last projection checkpoint: ${new Date(freshness).toLocaleString()}` : 'No projection events yet'}
          >
            <span
              style={{
                width: '8px',
                height: '8px',
                borderRadius: '50%',
                background: '#10b981',
                boxShadow: '0 0 8px #10b981',
                display: 'inline-block',
              }}
            />
            <span style={{ fontWeight: 500 }}>
              Updated {freshness ? formatRelativeTime(freshness) : 'realtime'}
            </span>
          </div>

          {/* Refresh Button */}
          <button
            onClick={() => loadData(true)}
            disabled={refreshing || loading}
            className="btn-secondary"
            style={{ padding: '8px 14px', display: 'flex', alignItems: 'center', gap: '6px', fontSize: '13px' }}
          >
            <RefreshCw size={14} className={refreshing ? 'spin' : ''} />
            <span>Sync</span>
          </button>
        </div>
      </div>

      {error && (
        <div
          style={{
            padding: '12px 16px',
            borderRadius: 'var(--radius-sm)',
            background: 'rgba(239, 68, 68, 0.1)',
            border: '1px solid rgba(239, 68, 68, 0.3)',
            color: '#f87171',
            fontSize: '13px',
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
          }}
        >
          <AlertCircle size={16} />
          <span>{error}</span>
        </div>
      )}

      {/* Currency Switcher & Granularity Filters */}
      <div
        className="glass-card"
        style={{
          padding: '12px 18px',
          display: 'flex',
          flexWrap: 'wrap',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: '12px',
        }}
      >
        {/* Currency Filter Tabs */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <span style={{ fontSize: '13px', color: 'var(--text-secondary)', display: 'flex', alignItems: 'center', gap: '4px' }}>
            <Coins size={14} /> Currency:
          </span>
          <button
            onClick={() => setSelectedCurrency('ALL')}
            style={{
              padding: '5px 12px',
              borderRadius: 'var(--radius-sm)',
              border: selectedCurrency === 'ALL' ? '1px solid rgba(16, 185, 129, 0.5)' : '1px solid transparent',
              background: selectedCurrency === 'ALL' ? 'rgba(16, 185, 129, 0.15)' : 'transparent',
              color: selectedCurrency === 'ALL' ? '#34d399' : 'var(--text-secondary)',
              fontSize: '13px',
              fontWeight: selectedCurrency === 'ALL' ? 600 : 500,
              cursor: 'pointer',
              transition: 'all 0.15s ease',
            }}
          >
            All Currencies {availableCurrencies.length > 1 && `(${availableCurrencies.length})`}
          </button>
          {availableCurrencies.map((c) => (
            <button
              key={c}
              onClick={() => setSelectedCurrency(c)}
              style={{
                padding: '5px 12px',
                borderRadius: 'var(--radius-sm)',
                border: selectedCurrency === c ? '1px solid rgba(16, 185, 129, 0.5)' : '1px solid transparent',
                background: selectedCurrency === c ? 'rgba(16, 185, 129, 0.15)' : 'transparent',
                color: selectedCurrency === c ? '#34d399' : 'var(--text-secondary)',
                fontSize: '13px',
                fontWeight: selectedCurrency === c ? 600 : 500,
                cursor: 'pointer',
                transition: 'all 0.15s ease',
              }}
            >
              {c}
            </button>
          ))}
        </div>

        {/* Granularity Toggle */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <span style={{ fontSize: '13px', color: 'var(--text-secondary)', display: 'flex', alignItems: 'center', gap: '4px' }}>
            <Calendar size={14} /> Timeframe:
          </span>
          <div
            style={{
              display: 'inline-flex',
              background: 'rgba(255, 255, 255, 0.04)',
              border: '1px solid var(--border-subtle)',
              borderRadius: 'var(--radius-sm)',
              padding: '2px',
            }}
          >
            <button
              onClick={() => setGranularity('monthly')}
              style={{
                padding: '4px 10px',
                border: 'none',
                borderRadius: 'var(--radius-sm)',
                background: granularity === 'monthly' ? 'rgba(16, 185, 129, 0.2)' : 'transparent',
                color: granularity === 'monthly' ? '#34d399' : 'var(--text-secondary)',
                fontSize: '12px',
                fontWeight: granularity === 'monthly' ? 600 : 500,
                cursor: 'pointer',
              }}
            >
              Monthly
            </button>
            <button
              onClick={() => setGranularity('daily')}
              style={{
                padding: '4px 10px',
                border: 'none',
                borderRadius: 'var(--radius-sm)',
                background: granularity === 'daily' ? 'rgba(16, 185, 129, 0.2)' : 'transparent',
                color: granularity === 'daily' ? '#34d399' : 'var(--text-secondary)',
                fontSize: '12px',
                fontWeight: granularity === 'daily' ? 600 : 500,
                cursor: 'pointer',
              }}
            >
              Daily
            </button>
          </div>
        </div>
      </div>

      {/* Multi-Currency KPI Summary Cards */}
      {availableCurrencies.length === 0 ? (
        <div className="glass-card" style={{ padding: '32px', textAlign: 'center' }}>
          <p style={{ color: 'var(--text-secondary)', fontSize: '14px', margin: 0 }}>
            No finalized transactions have been projected into Analytics yet.
            Once you confirm or finalize transactions in the Ledger, read models will project here automatically.
          </p>
        </div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))', gap: '16px' }}>
          {(selectedCurrency === 'ALL' ? availableCurrencies : [selectedCurrency]).map((curr) => {
            const totals = currencyTotals[curr] || { income: 0, expense: 0, net: 0 };
            return (
              <div
                key={curr}
                className="glass-card"
                style={{
                  padding: '20px',
                  display: 'flex',
                  flexDirection: 'column',
                  gap: '12px',
                  position: 'relative',
                  overflow: 'hidden',
                }}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <span
                    style={{
                      fontSize: '11px',
                      fontWeight: 700,
                      padding: '2px 8px',
                      borderRadius: '4px',
                      background: 'rgba(255, 255, 255, 0.08)',
                      color: 'var(--text-secondary)',
                      letterSpacing: '0.05em',
                    }}
                  >
                    CURRENCY: {curr}
                  </span>
                  <span style={{ fontSize: '12px', color: totals.net >= 0 ? '#34d399' : '#f87171', fontWeight: 600 }}>
                    {totals.net >= 0 ? '+ Savings' : '- Deficit'}
                  </span>
                </div>

                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px', marginTop: '4px' }}>
                  <div>
                    <div style={{ fontSize: '11px', color: 'var(--text-secondary)', marginBottom: '2px' }}>Total Inflow</div>
                    <div style={{ fontSize: '18px', fontWeight: 700, color: '#34d399' }}>
                      {formatCurrency(totals.income, curr)}
                    </div>
                  </div>
                  <div>
                    <div style={{ fontSize: '11px', color: 'var(--text-secondary)', marginBottom: '2px' }}>Total Outflow</div>
                    <div style={{ fontSize: '18px', fontWeight: 700, color: '#f87171' }}>
                      {formatCurrency(totals.expense, curr)}
                    </div>
                  </div>
                </div>

                <div
                  style={{
                    paddingTop: '10px',
                    borderTop: '1px solid var(--border-subtle)',
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                  }}
                >
                  <span style={{ fontSize: '12px', color: 'var(--text-secondary)' }}>Net Balance:</span>
                  <span style={{ fontSize: '15px', fontWeight: 700, color: totals.net >= 0 ? '#38bdf8' : '#fb923c' }}>
                    {formatCurrency(totals.net, curr)}
                  </span>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Navigation Sub-Tabs */}
      <div
        style={{
          display: 'flex',
          gap: '4px',
          borderBottom: '1px solid var(--border-subtle)',
          paddingBottom: '8px',
        }}
      >
        <button
          onClick={() => setActiveSection('cashflow')}
          style={{
            padding: '8px 16px',
            border: 'none',
            borderRadius: 'var(--radius-sm)',
            background: activeSection === 'cashflow' ? 'rgba(16, 185, 129, 0.15)' : 'transparent',
            color: activeSection === 'cashflow' ? '#34d399' : 'var(--text-secondary)',
            fontWeight: activeSection === 'cashflow' ? 600 : 500,
            fontSize: '14px',
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
          }}
        >
          <BarChart3 size={16} /> Cash Flow
        </button>

        <button
          onClick={() => setActiveSection('categories')}
          style={{
            padding: '8px 16px',
            border: 'none',
            borderRadius: 'var(--radius-sm)',
            background: activeSection === 'categories' ? 'rgba(16, 185, 129, 0.15)' : 'transparent',
            color: activeSection === 'categories' ? '#34d399' : 'var(--text-secondary)',
            fontWeight: activeSection === 'categories' ? 600 : 500,
            fontSize: '14px',
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
          }}
        >
          <PieChartIcon size={16} /> Categories
        </button>

        <button
          onClick={() => setActiveSection('merchants')}
          style={{
            padding: '8px 16px',
            border: 'none',
            borderRadius: 'var(--radius-sm)',
            background: activeSection === 'merchants' ? 'rgba(16, 185, 129, 0.15)' : 'transparent',
            color: activeSection === 'merchants' ? '#34d399' : 'var(--text-secondary)',
            fontWeight: activeSection === 'merchants' ? 600 : 500,
            fontSize: '14px',
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
          }}
        >
          <ShoppingBag size={16} /> Top Merchants
        </button>

        <button
          onClick={() => setActiveSection('trends')}
          style={{
            padding: '8px 16px',
            border: 'none',
            borderRadius: 'var(--radius-sm)',
            background: activeSection === 'trends' ? 'rgba(16, 185, 129, 0.15)' : 'transparent',
            color: activeSection === 'trends' ? '#34d399' : 'var(--text-secondary)',
            fontWeight: activeSection === 'trends' ? 600 : 500,
            fontSize: '14px',
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
          }}
        >
          <TrendingUp size={16} /> Trend Series
        </button>
      </div>

      {/* SECTION 1: Cash Flow Chart & Table */}
      {activeSection === 'cashflow' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
          <div className="glass-card" style={{ padding: '24px' }}>
            <h3 style={{ fontSize: '16px', fontWeight: 600, marginBottom: '4px' }}>
              Inflow vs Outflow Over Time ({granularity})
            </h3>
            <p style={{ color: 'var(--text-secondary)', fontSize: '13px', marginBottom: '20px' }}>
              Comparison of total income and expenses partitioned strictly by currency.
            </p>

            {chartCashFlowData.length === 0 ? (
              <div style={{ padding: '40px', textAlign: 'center', color: 'var(--text-secondary)' }}>
                No cash flow projection data available for the selected currency filter.
              </div>
            ) : (
              <div style={{ width: '100%', height: 320 }}>
                <ResponsiveContainer width="100%" height="100%">
                  <BarChart data={chartCashFlowData} margin={{ top: 10, right: 20, left: 10, bottom: 20 }}>
                    <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.08)" />
                    <XAxis dataKey="period" stroke="var(--text-secondary)" fontSize={12} tickLine={false} />
                    <YAxis stroke="var(--text-secondary)" fontSize={12} tickLine={false} />
                    <Tooltip
                      contentStyle={{
                        background: '#0e1424',
                        border: '1px solid var(--border-subtle)',
                        borderRadius: '8px',
                        boxShadow: '0 8px 24px rgba(0,0,0,0.5)',
                      }}
                      formatter={(val, name, item) => [formatCurrency(val, item.payload.currency), name === 'income' ? 'Total Income' : 'Total Expense']}
                    />
                    <Legend wrapperStyle={{ paddingTop: '10px' }} />
                    <Bar dataKey="income" name="Income" fill="#10b981" radius={[4, 4, 0, 0]} />
                    <Bar dataKey="expense" name="Expense" fill="#f43f5e" radius={[4, 4, 0, 0]} />
                  </BarChart>
                </ResponsiveContainer>
              </div>
            )}
          </div>

          {/* Cash Flow Detailed Table */}
          <div className="glass-card" style={{ padding: '20px' }}>
            <h3 style={{ fontSize: '15px', fontWeight: 600, marginBottom: '14px' }}>
              Cash Flow Breakdown Table
            </h3>
            <div style={{ overflowX: 'auto' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontSize: '13px' }}>
                <thead>
                  <tr style={{ borderBottom: '1px solid var(--border-subtle)', color: 'var(--text-secondary)' }}>
                    <th style={{ padding: '10px' }}>Period</th>
                    <th style={{ padding: '10px' }}>Granularity</th>
                    <th style={{ padding: '10px' }}>Currency</th>
                    <th style={{ padding: '10px', textAlign: 'right' }}>Total Income</th>
                    <th style={{ padding: '10px', textAlign: 'right' }}>Total Expense</th>
                    <th style={{ padding: '10px', textAlign: 'right' }}>Net Flow</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredCashFlow.length === 0 ? (
                    <tr>
                      <td colSpan={6} style={{ padding: '20px', textAlign: 'center', color: 'var(--text-secondary)' }}>
                        No records found.
                      </td>
                    </tr>
                  ) : (
                    filteredCashFlow.map((row, idx) => {
                      const inc = parseFloat(row.totalIncome) || 0;
                      const exp = parseFloat(row.totalExpense) || 0;
                      const net = inc - exp;
                      return (
                        <tr key={idx} style={{ borderBottom: '1px solid rgba(255,255,255,0.04)' }}>
                          <td style={{ padding: '10px', fontWeight: 500 }}>{row.periodBucket}</td>
                          <td style={{ padding: '10px', textTransform: 'capitalize', color: 'var(--text-secondary)' }}>{row.granularity}</td>
                          <td style={{ padding: '10px', fontWeight: 600 }}>{row.currency}</td>
                          <td style={{ padding: '10px', textAlign: 'right', color: '#34d399', fontWeight: 600 }}>{formatCurrency(row.totalIncome, row.currency)}</td>
                          <td style={{ padding: '10px', textAlign: 'right', color: '#f87171', fontWeight: 600 }}>{formatCurrency(row.totalExpense, row.currency)}</td>
                          <td style={{ padding: '10px', textAlign: 'right', fontWeight: 700, color: net >= 0 ? '#38bdf8' : '#fb923c' }}>
                            {formatCurrency(net, row.currency)}
                          </td>
                        </tr>
                      );
                    })
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {/* SECTION 2: Category Breakdown */}
      {activeSection === 'categories' && (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(360px, 1fr))', gap: '20px' }}>
          {/* Pie Chart Card */}
          <div className="glass-card" style={{ padding: '24px' }}>
            <h3 style={{ fontSize: '16px', fontWeight: 600, marginBottom: '4px' }}>
              Spending by Category
            </h3>
            <p style={{ color: 'var(--text-secondary)', fontSize: '13px', marginBottom: '20px' }}>
              Distribution of expenses across identified spending categories.
            </p>

            {chartCategoryData.length === 0 ? (
              <div style={{ padding: '40px', textAlign: 'center', color: 'var(--text-secondary)' }}>
                No category breakdown data available.
              </div>
            ) : (
              <div style={{ width: '100%', height: 300 }}>
                <ResponsiveContainer width="100%" height="100%">
                  <PieChart>
                    <Pie
                      data={chartCategoryData}
                      dataKey="value"
                      nameKey="name"
                      cx="50%"
                      cy="50%"
                      outerRadius={100}
                      innerRadius={50}
                      paddingAngle={3}
                    >
                      {chartCategoryData.map((entry, index) => (
                        <Cell key={`cell-${index}`} fill={CATEGORY_COLORS[index % CATEGORY_COLORS.length]} />
                      ))}
                    </Pie>
                    <Tooltip
                      contentStyle={{
                        background: '#0e1424',
                        border: '1px solid var(--border-subtle)',
                        borderRadius: '8px',
                      }}
                      formatter={(val, name, item) => [formatCurrency(val, item.payload.currency), item.payload.name]}
                    />
                    <Legend />
                  </PieChart>
                </ResponsiveContainer>
              </div>
            )}
          </div>

          {/* Percentage Share Table */}
          <div className="glass-card" style={{ padding: '24px' }}>
            <h3 style={{ fontSize: '16px', fontWeight: 600, marginBottom: '4px' }}>
              Category Share Details
            </h3>
            <p style={{ color: 'var(--text-secondary)', fontSize: '13px', marginBottom: '16px' }}>
              Calculated dynamically at projection time per currency partition.
            </p>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
              {chartCategoryData.length === 0 ? (
                <div style={{ padding: '20px', textAlign: 'center', color: 'var(--text-secondary)' }}>
                  No categories recorded.
                </div>
              ) : (
                chartCategoryData.map((cat, idx) => {
                  const color = CATEGORY_COLORS[idx % CATEGORY_COLORS.length];
                  return (
                    <div key={idx} style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: '13px' }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                          <span style={{ width: '10px', height: '10px', borderRadius: '50%', background: color }} />
                          <span style={{ fontWeight: 600 }}>{cat.name}</span>
                          <span style={{ fontSize: '11px', color: 'var(--text-secondary)', background: 'rgba(255,255,255,0.06)', padding: '1px 6px', borderRadius: '3px' }}>
                            {cat.currency}
                          </span>
                        </div>
                        <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                          <span style={{ fontWeight: 600 }}>{formatCurrency(cat.value, cat.currency)}</span>
                          <span style={{ fontWeight: 700, color, minWidth: '48px', textAlign: 'right' }}>
                            {cat.percentageShare?.toFixed(1)}%
                          </span>
                        </div>
                      </div>

                      {/* Visual progress bar */}
                      <div style={{ width: '100%', height: '6px', background: 'rgba(255,255,255,0.06)', borderRadius: '3px', overflow: 'hidden' }}>
                        <div
                          style={{
                            height: '100%',
                            width: `${Math.min(100, Math.max(0, cat.percentageShare || 0))}%`,
                            background: color,
                            borderRadius: '3px',
                            transition: 'width 0.4s ease',
                          }}
                        />
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </div>
        </div>
      )}

      {/* SECTION 3: Merchant Ranking */}
      {activeSection === 'merchants' && (
        <div className="glass-card" style={{ padding: '24px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px', flexWrap: 'wrap', gap: '12px' }}>
            <div>
              <h3 style={{ fontSize: '16px', fontWeight: 600, marginBottom: '4px' }}>
                Top Merchants Ranking
              </h3>
              <p style={{ color: 'var(--text-secondary)', fontSize: '13px', margin: 0 }}>
                Aggregated spend and transaction frequency per normalized merchant.
              </p>
            </div>

            {/* Sort toggle */}
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <span style={{ fontSize: '12px', color: 'var(--text-secondary)' }}>Sort By:</span>
              <button
                onClick={() => setMerchantSortBy('totalValue')}
                className="btn-secondary"
                style={{
                  padding: '4px 10px',
                  fontSize: '12px',
                  background: merchantSortBy === 'totalValue' ? 'rgba(16, 185, 129, 0.2)' : 'transparent',
                  color: merchantSortBy === 'totalValue' ? '#34d399' : 'var(--text-secondary)',
                }}
              >
                Spend Volume
              </button>
              <button
                onClick={() => setMerchantSortBy('transactionCount')}
                className="btn-secondary"
                style={{
                  padding: '4px 10px',
                  fontSize: '12px',
                  background: merchantSortBy === 'transactionCount' ? 'rgba(16, 185, 129, 0.2)' : 'transparent',
                  color: merchantSortBy === 'transactionCount' ? '#34d399' : 'var(--text-secondary)',
                }}
              >
                Frequency
              </button>
            </div>
          </div>

          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', fontSize: '13px' }}>
              <thead>
                <tr style={{ borderBottom: '1px solid var(--border-subtle)', color: 'var(--text-secondary)' }}>
                  <th style={{ padding: '10px' }}>Rank</th>
                  <th style={{ padding: '10px' }}>Merchant</th>
                  <th style={{ padding: '10px' }}>Currency</th>
                  <th style={{ padding: '10px', textAlign: 'center' }}>Transactions</th>
                  <th style={{ padding: '10px', textAlign: 'right' }}>Total Spend</th>
                  <th style={{ padding: '10px', textAlign: 'right' }}>Avg / Txn</th>
                </tr>
              </thead>
              <tbody>
                {filteredMerchants.length === 0 ? (
                  <tr>
                    <td colSpan={6} style={{ padding: '30px', textAlign: 'center', color: 'var(--text-secondary)' }}>
                      No merchant data projected yet.
                    </td>
                  </tr>
                ) : (
                  filteredMerchants.map((merch, idx) => {
                    const spend = parseFloat(merch.totalValue) || 0;
                    const count = merch.transactionCount || 1;
                    const avg = spend / count;
                    return (
                      <tr key={idx} style={{ borderBottom: '1px solid rgba(255,255,255,0.04)' }}>
                        <td style={{ padding: '10px', fontWeight: 700, color: idx < 3 ? '#10b981' : 'var(--text-secondary)' }}>
                          #{idx + 1}
                        </td>
                        <td style={{ padding: '10px', fontWeight: 600 }}>{merch.merchantNormalizedName}</td>
                        <td style={{ padding: '10px', fontWeight: 500 }}>{merch.currency}</td>
                        <td style={{ padding: '10px', textAlign: 'center' }}>
                          <span
                            style={{
                              padding: '2px 8px',
                              borderRadius: 'var(--radius-full)',
                              background: 'rgba(255, 255, 255, 0.05)',
                              fontSize: '12px',
                            }}
                          >
                            {count} {count === 1 ? 'txn' : 'txns'}
                          </span>
                        </td>
                        <td style={{ padding: '10px', textAlign: 'right', fontWeight: 700, color: '#f87171' }}>
                          {formatCurrency(merch.totalValue, merch.currency)}
                        </td>
                        <td style={{ padding: '10px', textAlign: 'right', color: 'var(--text-secondary)' }}>
                          {formatCurrency(avg, merch.currency)}
                        </td>
                      </tr>
                    );
                  })
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* SECTION 4: Trend View */}
      {activeSection === 'trends' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
          <div className="glass-card" style={{ padding: '24px' }}>
            <h3 style={{ fontSize: '16px', fontWeight: 600, marginBottom: '4px' }}>
              Net Cash Flow Trend Series ({granularity})
            </h3>
            <p style={{ color: 'var(--text-secondary)', fontSize: '13px', marginBottom: '20px' }}>
              Trajectory of net savings (income minus expenses) across time periods.
            </p>

            {chartTrendData.length === 0 ? (
              <div style={{ padding: '40px', textAlign: 'center', color: 'var(--text-secondary)' }}>
                No trend projection data available.
              </div>
            ) : (
              <div style={{ width: '100%', height: 320 }}>
                <ResponsiveContainer width="100%" height="100%">
                  <AreaChart data={chartTrendData} margin={{ top: 10, right: 20, left: 10, bottom: 20 }}>
                    <defs>
                      <linearGradient id="netFlowGrad" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="5%" stopColor="#10b981" stopOpacity={0.4} />
                        <stop offset="95%" stopColor="#10b981" stopOpacity={0.0} />
                      </linearGradient>
                    </defs>
                    <CartesianGrid strokeDasharray="3 3" stroke="rgba(255,255,255,0.08)" />
                    <XAxis dataKey="period" stroke="var(--text-secondary)" fontSize={12} tickLine={false} />
                    <YAxis stroke="var(--text-secondary)" fontSize={12} tickLine={false} />
                    <Tooltip
                      contentStyle={{
                        background: '#0e1424',
                        border: '1px solid var(--border-subtle)',
                        borderRadius: '8px',
                      }}
                      formatter={(val, name, item) => [formatCurrency(val, item.payload.currency), 'Net Flow']}
                    />
                    <Area
                      type="monotone"
                      dataKey="netFlow"
                      name="Net Flow"
                      stroke="#10b981"
                      strokeWidth={2}
                      fillOpacity={1}
                      fill="url(#netFlowGrad)"
                    />
                  </AreaChart>
                </ResponsiveContainer>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
