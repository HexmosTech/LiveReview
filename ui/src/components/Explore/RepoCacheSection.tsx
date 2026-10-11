import React, { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Button, Spinner } from '../UIPrimitives';
import { ConfirmModal } from '../ConfirmModal';
import { clearRepositoryCache, getRepositoryCache, RepositoryCache } from '../../api/repositories';
import { formatRelativeTime } from '../../api/reviews';
import { useOrgContext } from '../../hooks/useOrgContext';
import { isCloudMode } from '../../utils/deploymentMode';
import { notify } from '../../utils/notify';
import { Repository } from '../../types/explore';

const formatBytes = (bytes: number): string =>
  bytes >= 1 << 30 ? `${(bytes / (1 << 30)).toFixed(1)} GB` : `${Math.max(1, Math.round(bytes / (1 << 20)))} MB`;

// Repository settings → Repo cache: this repo's cached copy on the server, with a clear button for owners.
const RepoCacheSection: React.FC<{ repo: Repository }> = ({ repo }) => {
  const { isSuperAdmin, currentOrg } = useOrgContext();
  const canClear = isSuperAdmin || currentOrg?.role === 'owner';
  const canOpenSettings = isSuperAdmin || (currentOrg?.role === 'owner' && !isCloudMode()); // Settings → Storage gate
  const [cache, setCache] = useState<RepositoryCache | null>(null);
  const [error, setError] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [clearing, setClearing] = useState(false);

  const load = () => {
    setError(false);
    getRepositoryCache(repo.id).then(setCache).catch(() => setError(true));
  };
  useEffect(load, [repo.id]);

  const clear = async () => {
    setConfirming(false);
    setClearing(true);
    try {
      await clearRepositoryCache(repo.id);
      notify.success(`Repo cache cleared for ${repo.full_name}`);
      load();
    } catch (e: any) {
      notify.error(e?.message || 'Failed to clear the repo cache');
    } finally {
      setClearing(false);
    }
  };

  let body: React.ReactNode;
  if (error) {
    body = <p className="text-sm text-red-400">Couldn't load the repo cache details.</p>;
  } else if (!cache) {
    body = <div className="flex justify-center py-4"><Spinner /></div>;
  } else if (!cache.enabled) {
    body = (
      <p className="text-sm text-slate-300">
        The repo cache is turned off for this server, so PR reviews don't get risk scores.{' '}
        {canOpenSettings && (
          <Link to="/settings?section=repo-cache#storage" className="font-semibold text-blue-400 hover:text-blue-300">
            Open Repo Cache settings
          </Link>
        )}
      </p>
    );
  } else if (!cache.cached) {
    body = <p className="text-sm text-slate-300">Not cached yet. It's cloned on the next PR review.</p>;
  } else {
    body = (
      <>
        <dl className="grid grid-cols-[6.5rem_1fr] gap-x-3 gap-y-1 text-sm">
          <dt className="text-slate-400">Status</dt>
          <dd className="text-emerald-400">Cached</dd>
          <dt className="text-slate-400">Size</dt>
          <dd className="text-white">{formatBytes(cache.bytes)}</dd>
          <dt className="text-slate-400">Last used</dt>
          <dd className="text-white">{cache.last_used ? formatRelativeTime(cache.last_used) : '—'}</dd>
        </dl>
        <p className="mt-4 text-xs text-slate-400">
          The server keeps a recent copy of this repo to score PR risk quickly. Clearing it is safe; it's cloned again on the next PR review.
        </p>
        {canClear && (
          <div className="mt-4 flex justify-end">
            <Button variant="danger" size="sm" onClick={() => setConfirming(true)} isLoading={clearing} disabled={clearing}>
              Clear cache
            </Button>
          </div>
        )}
      </>
    );
  }

  return (
    <div>
      <div className="mb-5 flex items-center space-x-3">
        <div className="rounded-lg border border-violet-500/20 bg-violet-500/10 p-2 text-violet-400">
          <svg className="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
              d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4" />
          </svg>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white">Repo Cache</h3>
          <p className="text-sm text-slate-400">A shallow clone and code graph of this repo.</p>
        </div>
      </div>
      {body}
      <ConfirmModal
        show={confirming}
        type="danger"
        title="Clear repo cache?"
        message={`The cached copy of ${repo.full_name} will be deleted. It's cloned again on its next PR review.`}
        confirmText="Clear cache"
        onConfirm={clear}
        onCancel={() => setConfirming(false)}
      />
    </div>
  );
};

export default RepoCacheSection;
