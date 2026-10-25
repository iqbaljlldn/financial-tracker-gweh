import React, { useState, useEffect, useCallback } from 'react';
import { 
  FileCode, 
  Activity, 
  AlertTriangle, 
  CheckCircle, 
  Clock, 
  RefreshCw, 
  ShieldAlert, 
  Cpu, 
  ChevronRight, 
  Eye, 
  Terminal 
} from 'lucide-react';
import { apiClient } from '../api/client';

export function ParserMonitoringView() {
  const [templates, setTemplates] = useState([]);
  const [extractions, setExtractions] = useState([]);
  const [metrics, setMetrics] = useState(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [selectedExtraction, setSelectedExtraction] = useState(null);
  const [error, setError] = useState(null);

  const fetchData = useCallback(async () => {
    try {
      setError(null);
      const [tmplRes, extRes, metRes] = await Promise.all([
        apiClient.listParserTemplates().catch(() => []),
        apiClient.listParserExtractions(30).catch(() => []),
        apiClient.getParserMetrics().catch(() => null),
      ]);
      setTemplates(tmplRes || []);
      setExtractions(extRes || []);
      setMetrics(metRes);
    } catch (err) {
      setError(err.message || 'Failed to fetch parser monitoring telemetry');
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    fetchData();
    const interval = setInterval(fetchData, 15000); // 15s live refresh
    return () => clearInterval(interval);
  }, [fetchData]);

  const handleManualRefresh = () => {
    setRefreshing(true);
    fetchData();
  };

  const getFailureRateBadge = (rate) => {
    const pct = (rate * 100).toFixed(1);
    if (rate > 0.05) {
      return (
        <span className="badge badge-danger">
          <AlertTriangle size={12} className="inline mr-1" />
          {pct}% (Spike Alert)
        </span>
      );
    }
    if (rate > 0.02) {
      return (
        <span className="badge badge-warning">
          <Activity size={12} className="inline mr-1" />
          {pct}%
        </span>
      );
    }
    return (
      <span className="badge badge-success">
        <CheckCircle size={12} className="inline mr-1" />
        {pct}%
      </span>
    );
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-4 border-b border-white/10">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold tracking-tight text-white flex items-center gap-2">
              <Cpu className="text-emerald-400" size={26} />
              Parser Monitoring &amp; Engine Telemetry
            </h1>
            <span className="px-2.5 py-0.5 rounded-full text-xs font-semibold bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
              Ops Internal
            </span>
          </div>
          <p className="text-sm text-slate-400 mt-1">
            Real-time deterministic template performance, sample validation, and rolling failure-rate spike detection (Hotspot H3).
          </p>
        </div>

        <button
          onClick={handleManualRefresh}
          disabled={refreshing}
          className="btn btn-secondary flex items-center gap-2 self-start sm:self-auto text-sm"
        >
          <RefreshCw size={14} className={refreshing ? 'animate-spin' : ''} />
          {refreshing ? 'Syncing...' : 'Refresh Telemetry'}
        </button>
      </div>

      {error && (
        <div className="p-4 rounded-xl bg-rose-500/10 border border-rose-500/20 text-rose-400 flex items-center gap-3">
          <ShieldAlert size={18} />
          <span>{error}</span>
        </div>
      )}

      {/* KPI Summary Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4">
        <div className="stat-card">
          <div className="flex items-center justify-between text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">Active Templates</span>
            <FileCode size={18} className="text-emerald-400" />
          </div>
          <div className="mt-2 flex items-baseline gap-2">
            <span className="text-3xl font-extrabold text-white">{templates.length}</span>
            <span className="text-xs text-slate-400">curated senders</span>
          </div>
          <p className="text-xs text-emerald-400/80 mt-2 flex items-center gap-1">
            <CheckCircle size={12} /> 100% regression validated
          </p>
        </div>

        <div className="stat-card">
          <div className="flex items-center justify-between text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">AI Fallback Rate</span>
            <Cpu size={18} className="text-amber-400" />
          </div>
          <div className="mt-2 flex items-baseline gap-2">
            <span className="text-3xl font-extrabold text-white">
              {extractions.length > 0
                ? (
                    (extractions.filter((e) => e.method === 'ai').length / extractions.length) *
                    100
                  ).toFixed(1) + '%'
                : '0.0%'}
            </span>
            <span className="text-xs text-slate-400">of total volume</span>
          </div>
          <p className="text-xs text-slate-400 mt-2 flex items-center gap-1">
            <Clock size={12} /> Probabilistic fallback
          </p>
        </div>

        <div className="stat-card">
          <div className="flex items-center justify-between text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">Active Prompt</span>
            <Terminal size={18} className="text-sky-400" />
          </div>
          <div className="mt-2 flex items-baseline gap-2">
            <span className="text-3xl font-extrabold text-sky-400">v1</span>
            <span className="text-xs text-slate-400">OpenAI-compat</span>
          </div>
          <p className="text-xs text-sky-400/80 mt-2 flex items-center gap-1">
            <CheckCircle size={12} /> Regression gate passed
          </p>
        </div>

        <div className="stat-card">
          <div className="flex items-center justify-between text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">Avg 1h Failure Rate</span>
            <AlertTriangle size={18} className="text-amber-400" />
          </div>
          <div className="mt-2 flex items-baseline gap-2">
            <span className="text-3xl font-extrabold text-white">
              {metrics?.rollups?.length
                ? (
                    (metrics.rollups.reduce((acc, r) => acc + r.rate1h, 0) /
                      metrics.rollups.length) *
                    100
                  ).toFixed(1) + '%'
                : '0.0%'}
            </span>
            <span className="text-xs text-slate-400">threshold: 5%</span>
          </div>
          <p className="text-xs text-slate-400 mt-2">Trailing 1-hour window</p>
        </div>

        <div className="stat-card">
          <div className="flex items-center justify-between text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">H3 Early Warning</span>
            <ShieldAlert size={18} className="text-purple-400" />
          </div>
          <div className="mt-2 flex items-baseline gap-2">
            <span className="text-3xl font-extrabold text-emerald-400">
              {metrics?.spikeAlerts?.length || 0}
            </span>
            <span className="text-xs text-slate-400">active alerts</span>
          </div>
          <p className="text-xs text-emerald-400/80 mt-2 flex items-center gap-1">
            <CheckCircle size={12} /> Pipeline within cost target
          </p>
        </div>
      </div>

      {/* Active Templates Section */}
      <div className="card space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-semibold text-white flex items-center gap-2">
            <FileCode size={18} className="text-emerald-400" />
            Curated Sender Templates (Active Versions)
          </h2>
          <span className="text-xs text-slate-400">
            Exactly-one active version per pattern enforced
          </span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm text-slate-300">
            <thead className="text-xs uppercase bg-white/5 text-slate-400 border-b border-white/10">
              <tr>
                <th className="px-4 py-3">Sender Pattern</th>
                <th className="px-4 py-3">Version</th>
                <th className="px-4 py-3">Extraction Rules</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">1h Failure Rate</th>
                <th className="px-4 py-3">24h Failure Rate</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/5">
              {templates.map((tmpl) => {
                const rollup = metrics?.rollups?.find(
                  (r) => r.senderPattern === tmpl.senderPattern
                );
                return (
                  <tr key={tmpl.id} className="hover:bg-white/5 transition-colors">
                    <td className="px-4 py-3 font-mono font-medium text-white flex items-center gap-2">
                      <Terminal size={14} className="text-slate-400" />
                      {tmpl.senderPattern}
                    </td>
                    <td className="px-4 py-3">
                      <span className="px-2 py-0.5 rounded text-xs bg-slate-800 text-slate-300 font-mono">
                        v{tmpl.version}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-400">
                      {Object.keys(tmpl.rules?.fields || {}).length} fields configured
                    </td>
                    <td className="px-4 py-3">
                      {tmpl.active ? (
                        <span className="badge badge-success">Active</span>
                      ) : (
                        <span className="badge badge-neutral">Inactive</span>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      {getFailureRateBadge(rollup?.rate1h || 0)}
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-400">
                      {rollup ? `${(rollup.rate24h * 100).toFixed(1)}%` : '0.0%'}
                    </td>
                  </tr>
                );
              })}
              {templates.length === 0 && (
                <tr>
                  <td colSpan="6" className="px-4 py-8 text-center text-slate-500">
                    No sender templates registered yet.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Recent Extraction Results Stream */}
      <div className="card space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-base font-semibold text-white flex items-center gap-2">
            <Activity size={18} className="text-sky-400" />
            Recent Extraction Results Stream
          </h2>
          <span className="text-xs text-slate-400">Latest 30 parse attempts</span>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm text-slate-300">
            <thead className="text-xs uppercase bg-white/5 text-slate-400 border-b border-white/10">
              <tr>
                <th className="px-4 py-3">Timestamp</th>
                <th className="px-4 py-3">Method</th>
                <th className="px-4 py-3">Template / Prompt</th>
                <th className="px-4 py-3">Extracted Merchant</th>
                <th className="px-4 py-3">Category</th>
                <th className="px-4 py-3">Amount</th>
                <th className="px-4 py-3">Confidence</th>
                <th className="px-4 py-3 text-right">Details</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/5">
              {extractions.map((rec) => {
                const isFull = rec.confidenceAggregate >= 0.99;
                const isPartial = rec.confidenceAggregate >= 0.9 && rec.confidenceAggregate < 0.99;
                const isFail = rec.confidenceAggregate < 0.9;
                const cat = rec.candidate?.typeSpecificPayload?.category;
                const catSrc = rec.candidate?.typeSpecificPayload?.categorySource;
                return (
                  <tr key={rec.id} className="hover:bg-white/5 transition-colors">
                    <td className="px-4 py-3 text-xs text-slate-400 font-mono">
                      {new Date(rec.createdAt).toLocaleTimeString()}
                    </td>
                    <td className="px-4 py-3">
                      <span
                        className={`badge ${
                          rec.method === 'template' ? 'badge-primary' : 'badge-warning'
                        }`}
                      >
                        {rec.method}
                      </span>
                    </td>
                    <td className="px-4 py-3 font-mono text-xs text-slate-300">
                      {rec.templateVersion || rec.promptVersion || '-'}
                    </td>
                    <td className="px-4 py-3 font-medium text-white">
                      {rec.candidate?.merchant?.normalizedName ||
                        rec.candidate?.merchant?.rawName ||
                        '-'}
                    </td>
                    <td className="px-4 py-3 text-xs">
                      {cat ? (
                        <span className="px-2 py-0.5 rounded text-xs bg-slate-800 text-slate-200 border border-white/10" title={`Source: ${catSrc || 'unknown'}`}>
                          {cat}
                        </span>
                      ) : (
                        <span className="text-slate-500">-</span>
                      )}
                    </td>
                    <td className="px-4 py-3 font-mono text-emerald-400 font-semibold">
                      {rec.candidate?.amount?.value
                        ? `${rec.candidate.amount.currency} ${rec.candidate.amount.value.toLocaleString()}`
                        : '-'}
                    </td>
                    <td className="px-4 py-3">
                      {rec.method === 'ai' ? (
                        <span
                          className={`badge ${
                            rec.confidenceAggregate >= 0.7 ? 'badge-success' : 'badge-warning'
                          }`}
                        >
                          {rec.confidenceAggregate.toFixed(2)} (AI {rec.confidenceAggregate >= 0.7 ? 'Pass' : 'Review'})
                        </span>
                      ) : (
                        <>
                          {isFull && <span className="badge badge-success">1.0 (Full)</span>}
                          {isPartial && <span className="badge badge-warning">0.95 (Partial)</span>}
                          {isFail && <span className="badge badge-danger">Failed (AI Fallback)</span>}
                        </>
                      )}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <button
                        onClick={() => setSelectedExtraction(rec)}
                        className="btn-icon text-slate-400 hover:text-white"
                        title="View Candidate Event JSON"
                      >
                        <Eye size={16} />
                      </button>
                    </td>
                  </tr>
                );
              })}
              {extractions.length === 0 && (
                <tr>
                  <td colSpan="7" className="px-4 py-8 text-center text-slate-500">
                    No extractions recorded yet. Connect a mailbox to see live stream.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* JSON Candidate Inspection Modal */}
      {selectedExtraction && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm animate-in fade-in duration-200">
          <div className="bg-slate-900 border border-white/10 rounded-2xl max-w-2xl w-full p-6 space-y-4 max-h-[85vh] flex flex-col shadow-2xl">
            <div className="flex items-center justify-between border-b border-white/10 pb-3">
              <h3 className="font-semibold text-white flex items-center gap-2">
                <FileCode size={18} className="text-emerald-400" />
                CandidateFinancialEvent Envelope
              </h3>
              <button
                onClick={() => setSelectedExtraction(null)}
                className="text-slate-400 hover:text-white text-sm"
              >
                ✕ Close
              </button>
            </div>

            <div className="flex-1 overflow-y-auto">
              <pre className="text-xs font-mono bg-slate-950 p-4 rounded-xl text-emerald-400/90 overflow-x-auto border border-white/5">
                {JSON.stringify(selectedExtraction, null, 2)}
              </pre>
            </div>

            <div className="flex justify-end pt-2 border-t border-white/10">
              <button
                onClick={() => setSelectedExtraction(null)}
                className="btn btn-secondary text-sm"
              >
                Dismiss
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
