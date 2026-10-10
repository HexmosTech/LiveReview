import React from 'react';
import { Link } from 'react-router-dom';
import { BlastRadiusReport, BlastRadiusSkipped, isBlastRadiusSkipped } from '../../types/reviews';
import { blastRadiusTier, blastRadiusTierLabel, BlastRadiusTier } from '../../lib/blastRadius';
import { useOrgContext } from '../../hooks/useOrgContext';
import { isCloudMode } from '../../utils/deploymentMode';

export type RiskPoll = 'checking' | 'waiting' | 'gave_up';
type Kind = 'checking' | 'calculating' | 'ready' | 'failed' | 'skipped' | 'unavailable';

const TIER_TONE: Record<BlastRadiusTier, string> = {
  'blast-radius-high': '!text-red-400',
  'blast-radius-medium': '!text-amber-400',
  'blast-radius-low': '!text-sky-400',
  'blast-radius-none': '!text-slate-400',
};

// Reason codes saved by internal/jobqueue/blast_radius_worker.go, in plain words.
const REASONS: Record<string, { kind: Kind; text: string; fix?: 'cache' | 'connector' }> = {
  repo_too_large: { kind: 'skipped', text: 'This repo is larger than the repo cache.', fix: 'cache' },
  low_disk: { kind: 'skipped', text: 'The server was low on disk space when this review ran.' },
  disabled: { kind: 'skipped', text: 'Server-side risk assessment is turned off.', fix: 'cache' },
  repo_access_failed: { kind: 'failed', text: "Couldn't access the repository. The connector's token may have expired, or the git host was unreachable.", fix: 'connector' },
  repo_lookup_failed: { kind: 'failed', text: "Couldn't find the connector or pull request for this review.", fix: 'connector' },
  engine_missing: { kind: 'failed', text: "The risk scoring engine (codebase-memory-mcp) isn't installed on the server." },
  scoring_failed: { kind: 'failed', text: 'Scoring failed on the server.' },
  diff_failed: { kind: 'failed', text: "Couldn't compute the diff for this PR." },
  timeout: { kind: 'failed', text: 'Scoring took longer than 15 minutes and was stopped.' },
  not_pr: { kind: 'unavailable', text: 'Only PR/MR reviews get a risk assessment.' },
  base_too_old: { kind: 'unavailable', text: "The PR's base is older than the 13 months of history kept for scoring." },
  empty_diff: { kind: 'unavailable', text: 'The PR has no changes to score.' },
  no_source_branch: { kind: 'unavailable', text: "The PR's source branch is unknown." },
};

export interface RiskScore {
  path: string;
  score: number;
}

export interface RiskAssessment {
  kind: Kind;
  value: string; // short text for the header bar
  tone: string;
  text: string; // one-line explanation
  fix?: 'cache' | 'connector';
  report?: BlastRadiusReport;
  scores: RiskScore[];
  top: number;
}

export function riskAssessment(
  report: BlastRadiusReport | BlastRadiusSkipped | null,
  poll: RiskPoll,
  isPR: boolean,
  badgeScores?: RiskScore[], // same numbers as the Findings badges, once that panel has loaded
): RiskAssessment {
  const none: RiskScore[] = [];
  if (report && isBlastRadiusSkipped(report)) {
    const r = REASONS[report.reason] || { kind: 'failed' as Kind, text: 'Risk assessment could not run for this review.' };
    const tone = r.kind === 'failed' ? '!text-red-400' : r.kind === 'skipped' ? '!text-amber-400' : '!text-slate-400';
    const value = r.kind === 'failed' ? 'Failed' : r.kind === 'skipped' ? 'Skipped' : 'Not available';
    return { kind: r.kind, value, tone, text: r.text, fix: r.fix, scores: none, top: 0 };
  }
  const ready = report as BlastRadiusReport | null; // skipped returned above
  if (ready) {
    // Until Findings loads, fall back to the structural score (badges add 10% finding severity).
    const scores = badgeScores?.length
      ? badgeScores
      : ready.Files.flatMap((f) => (f.Hunks || []).map((h) => ({ path: f.Path, score: h.Combined || 0 })));
    const top = Math.max(0, ...scores.map((h) => h.score));
    const text = 'Highest risk score (0-100) across the changed code.';
    return { kind: 'ready', value: blastRadiusTierLabel(top), tone: TIER_TONE[blastRadiusTier(top)], text, report: ready, scores, top };
  }
  if (poll === 'gave_up') {
    return { kind: 'unavailable', value: 'Not available', tone: '!text-slate-400', text: 'No result arrived within 15 minutes.', scores: none, top: 0 };
  }
  if (poll === 'waiting') {
    return isPR
      ? { kind: 'calculating', value: 'Calculating...', tone: '!text-sky-400', text: 'Scoring the changed code. This updates on its own.', scores: none, top: 0 }
      : { kind: 'unavailable', value: 'Not available', tone: '!text-slate-400', text: REASONS.not_pr.text, scores: none, top: 0 };
  }
  return { kind: 'checking', value: '...', tone: '!text-slate-400', text: 'Checking for a risk report.', scores: none, top: 0 };
}

const TIER_ORDER: { tier: BlastRadiusTier; label: string }[] = [
  { tier: 'blast-radius-high', label: 'High' },
  { tier: 'blast-radius-medium', label: 'Moderate' },
  { tier: 'blast-radius-low', label: 'Low' },
  { tier: 'blast-radius-none', label: 'Minimal' },
];

// Expanded-panel column: the breakdown when ready, otherwise why not and how to fix it.
export const RiskAssessmentDetails: React.FC<{ risk: RiskAssessment; connectorId?: number }> = ({ risk, connectorId }) => {
  const { isSuperAdmin, currentOrg } = useOrgContext();
  const isOwner = isSuperAdmin || currentOrg?.role === 'owner';
  const canManageCache = isSuperAdmin || (currentOrg?.role === 'owner' && !isCloudMode()); // Settings → Storage gate

  if (risk.kind !== 'ready' || !risk.report) {
    const link = 'font-semibold text-blue-400 hover:text-blue-300';
    let fix: React.ReactNode = null;
    if (risk.fix === 'cache') {
      fix = canManageCache
        ? <Link to="/settings?section=repo-cache#storage" className={link}>Open Repo Cache settings</Link>
        : 'Ask an admin to check Settings → Storage → Repo Cache.';
    } else if (risk.fix === 'connector' && connectorId) {
      fix = isOwner ? <Link to={`/git/connector/${connectorId}`} className={link}>Check connector</Link> : 'Ask an admin to check the git connector.';
    } else if (risk.kind === 'failed') {
      fix = isOwner ? 'Check the worker logs for details.' : 'Ask an admin to check the server.';
    }
    return (
      <div className="space-y-1">
        <p className={`font-medium ${risk.tone}`}>{risk.value}</p>
        <p className="text-slate-300">{risk.text}</p>
        {fix && <p className="text-slate-400">{fix}</p>}
      </div>
    );
  }

  const hunks = risk.scores;
  const counts = TIER_ORDER.map(({ tier, label }) => ({ tier, label, n: hunks.filter((h) => blastRadiusTier(h.score) === tier).length }));
  const fileTop = new Map<string, number>();
  hunks.forEach((h) => fileTop.set(h.path, Math.max(fileTop.get(h.path) ?? 0, h.score)));
  const byFile = [...fileTop].map(([path, score]) => ({ path, score })).sort((a, b) => b.score - a.score).slice(0, 3);

  // Same layout as the Review details column: key/value grid, then labelled groups 16px apart.
  return (
    <div>
      <dl className="grid grid-cols-[6.5rem_1fr] gap-x-3 gap-y-1">
        <dt className="text-slate-400">Highest score</dt>
        <dd className={risk.tone}>{Math.round(risk.top)}</dd>
        <dt className="text-slate-400">Changes scored</dt>
        <dd className="text-white">{hunks.length} in {fileTop.size} file{fileTop.size === 1 ? '' : 's'}</dd>
      </dl>
      <div className="mt-4">
        <p className="text-slate-400 mb-1">Changes by risk</p>
        <div className="flex gap-4">
          {counts.map((c) => (
            <span key={c.tier} className={TIER_TONE[c.tier]}>{c.label} {c.n}</span>
          ))}
        </div>
      </div>
      {byFile.length > 1 && (
        <div className="mt-4">
          <p className="text-slate-400 mb-1">Riskiest files</p>
          <ul className="space-y-1">
            {byFile.map((f) => (
              <li key={f.path} className="flex items-center gap-3">
                <span className="truncate font-mono text-slate-300" title={f.path}>{f.path}</span>
                <span className={TIER_TONE[blastRadiusTier(f.score)]}>{Math.round(f.score)}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
};
