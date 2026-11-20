import React, { useEffect, useState } from 'react';
import { Button, SingleSelectField, Spinner } from '../UIPrimitives';
import {
  addRelatedRepository,
  getRelatedRepositories,
  getRepositories,
  removeRelatedRepository,
  RelatedRepository,
} from '../../api/repositories';
import { useOrgContext } from '../../hooks/useOrgContext';
import { notify } from '../../utils/notify';
import { Repository } from '../../types/explore';

// Repository settings → Related Repos: other repos (any connected provider)
// whose code graph should be linked with this one for cross-repo blast-radius
// context (see docs/multi-repo-blast-radius-plan.md). Mirrors RepoCacheSection's
// load/error/loading pattern.
const RelatedReposSection: React.FC<{ repo: Repository }> = ({ repo }) => {
  const { isSuperAdmin, currentOrg } = useOrgContext();
  const canManage = isSuperAdmin || currentOrg?.role === 'owner';

  const [related, setRelated] = useState<RelatedRepository[] | null>(null);
  const [error, setError] = useState(false);
  const [candidates, setCandidates] = useState<Repository[]>([]);
  const [picked, setPicked] = useState('');
  const [adding, setAdding] = useState(false);
  const [removingId, setRemovingId] = useState<number | null>(null);

  const load = () => {
    setError(false);
    getRelatedRepositories(repo.id).then(setRelated).catch(() => setError(true));
  };
  useEffect(load, [repo.id]);

  // Candidate list: every other repo in the org, minus ones already related.
  useEffect(() => {
    getRepositories({ perPage: 200 })
      .then((res) => setCandidates((res.repositories || []).filter((r) => r.id !== repo.id)))
      .catch(() => setCandidates([]));
  }, [repo.id]);

  const relatedIds = new Set((related || []).map((r) => r.repository_id));
  const options = candidates
    .filter((c) => !relatedIds.has(c.id))
    .map((c) => ({ value: String(c.id), label: `${c.full_name} (${c.provider})` }));

  const add = async () => {
    if (!picked) return;
    setAdding(true);
    try {
      await addRelatedRepository(repo.id, Number(picked));
      setPicked('');
      notify.success('Related repository added');
      load();
    } catch (e: any) {
      notify.error(e?.message || 'Failed to add related repository');
    } finally {
      setAdding(false);
    }
  };

  const remove = async (relationId: number) => {
    setRemovingId(relationId);
    try {
      await removeRelatedRepository(repo.id, relationId);
      notify.success('Related repository removed');
      load();
    } catch (e: any) {
      notify.error(e?.message || 'Failed to remove related repository');
    } finally {
      setRemovingId(null);
    }
  };

  let body: React.ReactNode;
  if (error) {
    body = <p className="text-sm text-red-400">Couldn't load related repositories.</p>;
  } else if (!related) {
    body = <div className="flex justify-center py-4"><Spinner /></div>;
  } else {
    body = (
      <>
        {related.length === 0 ? (
          <p className="text-sm text-slate-300">
            No related repos yet. Linking a repo here lets blast-radius scoring see calls that cross into it (shared APIs,
            events, imports) instead of only this repo's own code.
          </p>
        ) : (
          <ul className="divide-y divide-slate-700">
            {related.map((r) => (
              <li key={r.relation_id} className="flex items-center justify-between py-2.5">
                <div className="min-w-0">
                  <a href={r.web_url} target="_blank" rel="noopener noreferrer" className="truncate text-sm font-medium text-blue-400 hover:text-blue-300">
                    {r.full_name}
                  </a>
                  <p className="text-xs text-slate-400">{r.provider}</p>
                </div>
                {canManage && (
                  <Button
                    variant="danger"
                    size="sm"
                    isLoading={removingId === r.relation_id}
                    disabled={removingId !== null}
                    onClick={() => remove(r.relation_id)}
                  >
                    Remove
                  </Button>
                )}
              </li>
            ))}
          </ul>
        )}
        {canManage && (
          <div className="mt-4 flex items-end gap-2">
            <div className="flex-1">
              <SingleSelectField label="repository" options={options} value={picked} onChange={setPicked} placeholder="Choose a repository to link" searchable />
            </div>
            <Button variant="primary" size="sm" onClick={add} isLoading={adding} disabled={!picked || adding}>
              Add
            </Button>
          </div>
        )}
      </>
    );
  }

  return (
    <div>
      <div className="mb-5 flex items-center space-x-3">
        <div className="rounded-lg border border-sky-500/20 bg-sky-500/10 p-2 text-sky-400">
          <svg className="h-6 w-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.828 10.172a4 4 0 010 5.656l-1.414 1.414a4 4 0 01-5.656-5.656l1.414-1.414M10.172 13.828a4 4 0 010-5.656l1.414-1.414a4 4 0 015.656 5.656l-1.414 1.414" />
          </svg>
        </div>
        <div>
          <h3 className="text-lg font-semibold text-white">Related Repos</h3>
          <p className="text-sm text-slate-400">Other repositories to include when building cross-repo blast-radius context.</p>
        </div>
      </div>
      {body}
    </div>
  );
};

export default RelatedReposSection;
