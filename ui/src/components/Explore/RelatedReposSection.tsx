import React, { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { LuExternalLink, LuSettings } from 'react-icons/lu';
import { Button, Icons, SingleSelectField, Spinner, Tooltip } from '../UIPrimitives';
import ProviderIcon from '../ProviderIcon';
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

// MAX_CANDIDATE_PAGES bounds the repo-picker's pagination loop (at 200/page,
// 20 pages = 4000 repos) - a safety cap, not an expected org size.
const MAX_CANDIDATE_PAGES = 20;

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
  const [candidatesError, setCandidatesError] = useState(false);
  const [picked, setPicked] = useState('');
  const [adding, setAdding] = useState(false);
  const [removingId, setRemovingId] = useState<number | null>(null);

  // mountedRef guards every setState below that follows an async call (the
  // initial effect, and the add/remove handlers' refresh) so a response that
  // lands after the component unmounts is a no-op instead of a warning.
  const mountedRef = useRef(true);
  useEffect(() => () => { mountedRef.current = false; }, []);

  const loadRelated = () => {
    setError(false);
    getRelatedRepositories(repo.id)
      .then((res) => { if (mountedRef.current) setRelated(res); })
      .catch(() => { if (mountedRef.current) setError(true); });
  };
  useEffect(loadRelated, [repo.id]);

  // Candidate list: every other repo in the org, minus ones already related.
  // The repositories API caps per_page at 200, so this pages through all of
  // them (capped at MAX_CANDIDATE_PAGES as a sanity bound) instead of
  // silently truncating the picker to the first 200 repos in large orgs.
  useEffect(() => {
    setCandidatesError(false);
    (async () => {
      const all: Repository[] = [];
      for (let page = 1; page <= MAX_CANDIDATE_PAGES; page++) {
        const res = await getRepositories({ perPage: 200, page });
        all.push(...(res.repositories || []));
        if (page >= (res.total_pages || 1)) break;
      }
      return all;
    })()
      .then((all) => { if (mountedRef.current) setCandidates(all.filter((r) => r.id !== repo.id)); })
      .catch(() => { if (mountedRef.current) { setCandidates([]); setCandidatesError(true); } });
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
      if (mountedRef.current) setPicked('');
      notify.success('Related repository added');
      loadRelated();
    } catch (e: any) {
      notify.error(e?.message || 'Failed to add related repository');
    } finally {
      if (mountedRef.current) setAdding(false);
    }
  };

  const remove = async (relationId: number) => {
    setRemovingId(relationId);
    try {
      await removeRelatedRepository(repo.id, relationId);
      notify.success('Related repository removed');
      loadRelated();
    } catch (e: any) {
      notify.error(e?.message || 'Failed to remove related repository');
    } finally {
      if (mountedRef.current) setRemovingId(null);
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
              <li key={r.relation_id} className="flex items-center justify-between gap-3 py-3">
                <div className="flex min-w-0 items-center gap-2.5">
                  <span className="flex-shrink-0"><ProviderIcon provider={r.provider} /></span>
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-white">{r.full_name}</p>
                    <p className="text-xs text-slate-400">{r.provider}</p>
                  </div>
                </div>
                <div className="flex flex-shrink-0 items-center gap-1">
                  <Tooltip content="Open on provider">
                    <a
                      href={r.web_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      aria-label="Open on provider"
                      className="flex items-center justify-center rounded-lg p-2 text-slate-400 hover:bg-slate-700 hover:text-blue-300"
                    >
                      <LuExternalLink className="h-4 w-4" />
                    </a>
                  </Tooltip>
                  <Tooltip content="Open repository settings">
                    <Link
                      to={`/explore/repositories/${r.repository_id}/settings`}
                      aria-label="Open repository settings"
                      className="flex items-center justify-center rounded-lg p-2 text-slate-400 hover:bg-slate-700 hover:text-blue-300"
                    >
                      <LuSettings className="h-4 w-4" />
                    </Link>
                  </Tooltip>
                  {canManage && (
                    <Button
                      variant="danger"
                      size="sm"
                      icon={<Icons.Delete />}
                      isLoading={removingId === r.relation_id}
                      disabled={removingId !== null}
                      onClick={() => remove(r.relation_id)}
                    >
                      Remove
                    </Button>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
        {canManage && (
          <div className="mt-4">
            {candidatesError && (
              <p className="mb-2 text-xs text-red-400">Couldn't load the repository list to link from - try reloading this page.</p>
            )}
            <div className="flex items-center gap-2">
              <div className="flex-1">
                <SingleSelectField
                  label="repository"
                  options={options}
                  value={picked}
                  onChange={setPicked}
                  placeholder="Choose a repository to link"
                  searchable
                  allowClear={false}
                />
              </div>
              <Button variant="primary" size="md" icon={<Icons.Add />} onClick={add} isLoading={adding} disabled={!picked || adding}>
                Add
              </Button>
            </div>
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
