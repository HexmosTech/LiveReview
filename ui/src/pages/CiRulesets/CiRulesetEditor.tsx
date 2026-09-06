import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import apiClient from '../../api/apiClient';
import { PageHeader, Card, Button, Input, Icons, Badge, Spinner } from '../../components/UIPrimitives';
import { useToast } from '../../components/NotificationToast';
import { getReviews } from '../../api/reviews';
import { Review } from '../../types/reviews';
import { getPrimaryTitle, reviewStatusBadge, SourceIcon } from '../../utils/reviewDisplay';
import {
  CIRuleset,
  CanonicalDoc,
  PreviewOutcome,
  Taxonomy,
  EMPTY_TAXONOMY,
  EXAMPLE_EXPRESSIONS,
  JQ_MANUAL_URL,
  GOJQ_REPO_URL,
  titleCase,
  buildLLMPrompt,
  jqQuote,
  ScreenPoint,
  getCaretScreenPoint,
  FloatingPanel,
  ReferenceChip,
  Breadcrumb,
  GateResultPill,
} from './shared';
import { JqQuickHelp } from './jqQuickHelp';

// ---- insert reference panel: Severity / Type / Confidence / Classification, git-lrc style

const InsertReferenceContent: React.FC<{ taxonomy: Taxonomy; onInsert: (text: string) => void; onClose: () => void }> = ({
  taxonomy,
  onInsert,
  onClose,
}) => {
  const [filter, setFilter] = useState('');
  const q = filter.trim().toLowerCase();
  const matches = (s: string) => !q || s.toLowerCase().includes(q);

  // Inserts a complete, valid jq comparison -- both the field (left) and the
  // value (right) -- e.g. `.severity == "critical"`, not just the bare value.
  const insertComparison = (field: string, raw: string) => {
    onInsert(`.${field} == ${jqQuote(raw)}`);
    onClose();
  };

  const filteredSeverities = taxonomy.severities.filter(matches);
  const filteredTypes = taxonomy.types.filter(matches);
  const filteredConfidences = taxonomy.confidences.filter(matches);
  const filteredTree = q
    ? taxonomy.category_tree
        .map((c) => ({ ...c, subcategories: c.subcategories.filter((s) => matches(s.subcategory)) }))
        .filter((c) => matches(c.category) || c.subcategories.length > 0)
    : taxonomy.category_tree;

  const nothing = filteredSeverities.length === 0 && filteredTypes.length === 0 && filteredConfidences.length === 0 && filteredTree.length === 0;

  return (
    <div className="p-4">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-sm font-semibold text-white">Insert reference</h3>
        <button onClick={onClose} className="text-slate-400 hover:text-white"><Icons.Close /></button>
      </div>
      <Input
        placeholder="Search severity, type, confidence, category, subcategory…"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        icon={<Icons.Search />}
        autoFocus
      />
      <div className="mt-4 space-y-5">
        {filteredSeverities.length > 0 && (
          <div>
            <h4 className="text-xs font-semibold text-slate-400 uppercase tracking-wide mb-2">Severity</h4>
            <div className="flex flex-wrap gap-2">
              {filteredSeverities.map((v) => (
                <ReferenceChip key={v} label={titleCase(v)} title={`.severity == "${v}"`} onClick={() => insertComparison('severity', v)} />
              ))}
            </div>
          </div>
        )}

        {filteredTypes.length > 0 && (
          <div>
            <h4 className="text-xs font-semibold text-slate-400 uppercase tracking-wide mb-2">Type</h4>
            <div className="flex flex-wrap gap-2">
              {filteredTypes.map((v) => (
                <ReferenceChip key={v} label={titleCase(v)} title={`.type == "${v}"`} onClick={() => insertComparison('type', v)} />
              ))}
            </div>
          </div>
        )}

        {filteredConfidences.length > 0 && (
          <div>
            <h4 className="text-xs font-semibold text-slate-400 uppercase tracking-wide mb-2">Confidence</h4>
            <div className="flex flex-wrap gap-2">
              {filteredConfidences.map((v) => (
                <ReferenceChip key={v} label={titleCase(v)} title={`.confidence == "${v}"`} onClick={() => insertComparison('confidence', v)} />
              ))}
            </div>
          </div>
        )}

        {filteredTree.length > 0 && (
          <div>
            <h4 className="text-xs font-semibold text-slate-400 uppercase tracking-wide mb-2">Classification</h4>
            <div className="space-y-3">
              {filteredTree.map((node) => (
                <div key={node.category}>
                  <ReferenceChip label={titleCase(node.category)} title={`.category == "${node.category}"`} onClick={() => insertComparison('category', node.category)} />
                  {node.subcategories.length > 0 && (
                    <div className="flex flex-wrap gap-2 mt-2 ml-5 pl-3 border-l-2 border-slate-700">
                      {node.subcategories.map((s) => (
                        <ReferenceChip
                          key={s.subcategory}
                          label={titleCase(s.subcategory)}
                          title={`.subcategory == "${s.subcategory}"`}
                          onClick={() => insertComparison('subcategory', s.subcategory)}
                        />
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}

        {nothing && <p className="text-sm text-slate-500">No matches.</p>}
        {taxonomy.category_tree.length === 0 && taxonomy.types.length === 0 && !q && (
          <p className="text-xs text-slate-500">
            This org has no completed reviews yet. Type, Confidence and Classification will appear here automatically once reviews complete.
          </p>
        )}
      </div>
    </div>
  );
};

// ---- LLM helper panel --------------------------------------------------------
// Neither ChatGPT nor Gemini support a reliable, documented URL parameter to
// prefill the chat box (an old undocumented `?q=` on chatgpt.com stopped
// working; Gemini never supported one). So "Copy prompt" is its own,
// separate action -- it's the one thing that actually always works -- and
// each "Ask <provider>" button just opens that site in a new tab; the two
// aren't bundled into a single click.

const LLMHelperContent: React.FC<{ prompt: string; onCopy: () => void; onClose: () => void }> = ({ prompt, onCopy, onClose }) => (
  <div className="p-4">
    <div className="flex items-center justify-between mb-3">
      <h3 className="text-sm font-semibold text-white">Ask an LLM to write it</h3>
      <button onClick={onClose} className="text-slate-400 hover:text-white"><Icons.Close /></button>
    </div>
    <p className="text-xs text-slate-400 mb-3">
      Copy the prompt (JSON shape, full taxonomy, and a rule template), then open a chat and paste it with Ctrl+V / Cmd+V.
    </p>
    <div className="mb-3">
      <Button size="sm" variant="primary" icon={<Icons.Copy />} onClick={onCopy}>
        Copy prompt
      </Button>
    </div>
    <div className="flex flex-wrap gap-2">
      <Button size="sm" variant="outline" onClick={() => window.open('https://chatgpt.com/', '_blank')}>
        Open ChatGPT
      </Button>
      <Button size="sm" variant="outline" onClick={() => window.open('https://gemini.google.com/app', '_blank')}>
        Open Gemini
      </Button>
      <Button size="sm" variant="outline" onClick={() => window.open('https://chat.deepseek.com/', '_blank')}>
        Open DeepSeek
      </Button>
    </div>
    <details className="mt-3">
      <summary className="text-xs text-slate-400 cursor-pointer hover:text-slate-300">Preview the exact prompt</summary>
      <pre className="mt-2 text-xs bg-slate-900/70 border border-slate-700 rounded px-3 py-2 text-slate-300 overflow-auto max-h-64 whitespace-pre-wrap">{prompt}</pre>
    </details>
  </div>
);

// ---- add-review-tab dialog ---------------------------------------------------

// ---- unsaved-changes prompt --------------------------------------------------

const UnsavedChangesDialog: React.FC<{
  open: boolean;
  saving: boolean;
  onSaveAndContinue: () => void;
  onDiscard: () => void;
  onKeepEditing: () => void;
}> = ({ open, saving, onSaveAndContinue, onDiscard, onKeepEditing }) => {
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={onKeepEditing} />
      <div className="relative bg-slate-800 rounded-lg border border-slate-600 shadow-2xl max-w-md w-full p-5">
        <h3 className="text-base font-semibold text-white mb-2">You have unsaved changes</h3>
        <p className="text-sm text-slate-400 mb-5">Leaving now will discard your edits to this ruleset's name, description, or jq expression.</p>
        <div className="flex flex-col gap-2">
          <Button onClick={onSaveAndContinue} isLoading={saving}>Save &amp; continue</Button>
          <Button variant="danger" onClick={onDiscard} disabled={saving}>Discard changes</Button>
          <Button variant="ghost" onClick={onKeepEditing} disabled={saving}>Keep editing</Button>
        </div>
      </div>
    </div>
  );
};

const AddReviewTabDialog: React.FC<{
  open: boolean;
  reviews: Review[];
  alreadyAdded: number[];
  onAdd: (id: number) => void;
  onClose: () => void;
}> = ({ open, reviews, alreadyAdded, onAdd, onClose }) => {
  if (!open) return null;
  const available = reviews.filter((r) => !alreadyAdded.includes(r.id));
  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/70 backdrop-blur-sm" onClick={onClose} />
      <div className="relative bg-slate-800 rounded-lg border border-slate-600 shadow-2xl max-w-xl w-full max-h-[80vh] flex flex-col">
        <div className="p-4 border-b border-slate-700 flex items-center justify-between">
          <h3 className="text-base font-semibold text-white">Add a review tab</h3>
          <button onClick={onClose} className="text-slate-400 hover:text-white"><Icons.Close /></button>
        </div>
        <div className="p-4 overflow-y-auto space-y-2">
          {available.length === 0 ? (
            <p className="text-sm text-slate-400">No more reviews to add.</p>
          ) : (
            available.map((rv) => {
              const badge = reviewStatusBadge(rv.status);
              return (
                <button
                  key={rv.id}
                  type="button"
                  onClick={() => { onAdd(rv.id); onClose(); }}
                  className="w-full rounded-lg border border-slate-700 hover:border-blue-500 hover:bg-slate-700/40 px-3 py-2.5 text-left"
                >
                  <div className="flex items-center justify-between gap-2 mb-1">
                    <div className="flex items-center gap-2 min-w-0">
                      <span className="flex-shrink-0 text-slate-300"><SourceIcon provider={rv.provider} triggerType={rv.triggerType} /></span>
                      <Badge variant="info" className="text-xs flex-shrink-0">#{rv.id}</Badge>
                      <span className="text-xs text-slate-400 truncate">{rv.repository}</span>
                    </div>
                    <span
                      className="text-xs font-medium px-2 py-0.5 rounded-full border flex-shrink-0"
                      style={{ color: badge.color, borderColor: badge.borderColor }}
                    >
                      {rv.status}
                    </span>
                  </div>
                  <p className="text-sm text-slate-100 truncate">{getPrimaryTitle(rv)}</p>
                  {rv.authorName && <p className="text-xs text-slate-500 truncate mt-0.5">by {rv.authorName}</p>}
                </button>
              );
            })
          )}
        </div>
      </div>
    </div>
  );
};

// ---- component --------------------------------------------------------------

const CiRulesetEditor: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const rulesetId = id ? Number(id) : undefined;
  const isNew = !rulesetId;
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const { showToast, ToastContainer } = useToast();

  const [loadingRuleset, setLoadingRuleset] = useState(!isNew);
  const initialDraft = { name: '', description: '', jq_expr: EXAMPLE_EXPRESSIONS[0].expr };
  const [editing, setEditing] = useState<Partial<CIRuleset> | null>(isNew ? initialDraft : null);
  const [saving, setSaving] = useState(false);

  // The last-saved (or, for a brand-new ruleset, the initial-default) values
  // -- compared against `editing` to know whether there are unsaved changes
  // to warn about before navigating away.
  const [savedSnapshot, setSavedSnapshot] = useState<{ name: string; description: string; jq_expr: string }>(initialDraft);
  const dirty = !!editing && (
    (editing.name || '') !== savedSnapshot.name ||
    (editing.description || '') !== savedSnapshot.description ||
    (editing.jq_expr || '') !== savedSnapshot.jq_expr
  );
  const dirtyRef = useRef(dirty);
  dirtyRef.current = dirty;

  // Where to go once the user resolves the "unsaved changes" prompt --
  // either a path (breadcrumb / Cancel / Get integration code) or the
  // sentinel 'BACK' for a real browser back-button press.
  const [pendingNav, setPendingNav] = useState<string | 'BACK' | null>(null);

  const [sampleDoc, setSampleDoc] = useState<CanonicalDoc | null>(null);
  const [taxonomy, setTaxonomy] = useState<Taxonomy>(EMPTY_TAXONOMY);
  const [recentReviews, setRecentReviews] = useState<Review[]>([]);

  // Tab state lives in the URL: ?tab=sample|<reviewId>&reviews=1,2,3 so back/
  // forward and a copy-pasted link restore exactly what was open.
  const reviewTabs = useMemo(() => {
    const raw = searchParams.get('reviews');
    if (!raw) return [] as number[];
    return raw.split(',').map((s) => Number(s)).filter((n) => Number.isFinite(n) && n > 0);
  }, [searchParams]);
  const tabParam = searchParams.get('tab');
  const activeTab: 'sample' | number = tabParam && tabParam !== 'sample' && Number.isFinite(Number(tabParam)) ? Number(tabParam) : 'sample';

  const setUrlTabs = useCallback((tabs: number[], active: 'sample' | number) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (tabs.length) next.set('reviews', tabs.join(',')); else next.delete('reviews');
      next.set('tab', String(active));
      return next;
    }, { replace: true });
  }, [setSearchParams]);

  const [addTabOpen, setAddTabOpen] = useState(false);
  const [quickHelpOpen, setQuickHelpOpen] = useState(false);

  const [samplePreview, setSamplePreview] = useState<PreviewOutcome>({ loading: false });
  const [reviewPreviews, setReviewPreviews] = useState<Record<number, PreviewOutcome>>({});

  const [insertPoint, setInsertPoint] = useState<ScreenPoint | null>(null);
  const [llmPoint, setLlmPoint] = useState<ScreenPoint | null>(null);

  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const llmButtonRef = useRef<HTMLSpanElement>(null);

  useEffect(() => {
    if (isNew || !rulesetId) return;
    setLoadingRuleset(true);
    apiClient.get<{ ruleset: CIRuleset }>(`/ci-rulesets/${rulesetId}`)
      .then((res) => {
        setEditing({ ...res.ruleset });
        setSavedSnapshot({ name: res.ruleset.name, description: res.ruleset.description || '', jq_expr: res.ruleset.jq_expr });
      })
      .catch((err: any) => {
        showToast(err?.message || 'Failed to load ruleset', 'error');
        navigate('/ci-rulesets');
      })
      .finally(() => setLoadingRuleset(false));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rulesetId, isNew]);

  useEffect(() => {
    apiClient.get<{ document?: CanonicalDoc }>('/ci-rulesets/sample-document')
      .then((res) => setSampleDoc(res.document || null))
      .catch((): void => undefined);
    apiClient.get<Taxonomy>('/ci-rulesets/taxonomy')
      .then((res) => setTaxonomy({
        severities: res.severities || [],
        confidences: res.confidences || [],
        categories: res.categories || [],
        subcategories: res.subcategories || [],
        types: res.types || [],
        category_tree: res.category_tree || [],
      }))
      .catch((): void => undefined);
    getReviews({ perPage: 20 })
      .then((res) => setRecentReviews(res.reviews || []))
      .catch((): void => undefined);
  }, []);

  // Warn on tab close / refresh / typing a new URL while there are unsaved
  // changes -- the one case in-app navigation guards below can't cover.
  useEffect(() => {
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      if (!dirtyRef.current) return;
      e.preventDefault();
      e.returnValue = '';
    };
    window.addEventListener('beforeunload', onBeforeUnload);
    return () => window.removeEventListener('beforeunload', onBeforeUnload);
  }, []);

  // Best-effort guard for the browser/OS Back button (HashRouter has no
  // built-in navigation blocker outside a data router). On a real back
  // press we restore the current hash to cancel it, which -- because
  // assigning location.hash pushes a new history entry -- leaves the
  // original back target exactly one `history.back()` away, so Discard
  // below can still land the user where they meant to go.
  const originalHashRef = useRef(window.location.hash);
  const restoringRef = useRef(false);
  const allowNextPopRef = useRef(false);
  useEffect(() => {
    const onPopState = () => {
      if (restoringRef.current) {
        restoringRef.current = false;
        return;
      }
      if (allowNextPopRef.current) {
        allowNextPopRef.current = false;
        return;
      }
      if (!dirtyRef.current) return;
      restoringRef.current = true;
      window.location.hash = originalHashRef.current;
      setPendingNav('BACK');
    };
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  const guardedNavigate = (to: string): boolean => {
    if (dirtyRef.current) {
      setPendingNav(to);
      return false;
    }
    navigate(to);
    return true;
  };

  const discardAndLeave = () => {
    const target = pendingNav;
    setPendingNav(null);
    if (target === 'BACK') {
      allowNextPopRef.current = true;
      window.history.back();
    } else if (target) {
      navigate(target);
    }
  };

  const saveAndLeave = async () => {
    const target = pendingNav;
    const ok = await save({ suppressAutoNavigate: true });
    if (!ok) return; // save failed -- stay put, keep the prompt dismissed but changes intact
    setPendingNav(null);
    if (target === 'BACK') {
      allowNextPopRef.current = true;
      window.history.back();
    } else if (target) {
      navigate(target);
    }
  };

  const insertAtCursor = (text: string) => {
    const current = editing?.jq_expr || '';
    const el = textareaRef.current;
    if (!el) {
      setEditing((prev) => ({ ...(prev || {}), jq_expr: current + text }));
      return;
    }
    const start = el.selectionStart ?? current.length;
    const end = el.selectionEnd ?? current.length;
    const next = current.slice(0, start) + text + current.slice(end);
    setEditing((prev) => ({ ...(prev || {}), jq_expr: next }));
    requestAnimationFrame(() => {
      el.focus();
      const pos = start + text.length;
      el.setSelectionRange(pos, pos);
    });
  };

  const openInsertReference = () => {
    const el = textareaRef.current;
    if (el) {
      el.focus();
      setInsertPoint(getCaretScreenPoint(el));
    } else {
      setInsertPoint({ x: 100, y: 200 });
    }
  };

  const openLlmHelper = () => {
    const btn = llmButtonRef.current;
    if (btn) {
      const rect = btn.getBoundingClientRect();
      setLlmPoint({ x: rect.left, y: rect.bottom + 6 });
    }
  };

  const copyPrompt = (prompt: string) => {
    navigator.clipboard?.writeText(prompt).then(
      () => showToast('Prompt copied — paste it with Ctrl+V / Cmd+V', 'success'),
      () => showToast('Could not copy automatically — use "Preview the exact prompt" below and copy manually', 'error')
    );
  };

  // ---- live preview: re-runs against the sample doc + every open review tab
  // whenever jq_expr changes, debounced -- results update together as you type.
  useEffect(() => {
    if (!editing || !editing.jq_expr) return;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    const expr = editing.jq_expr;
    debounceRef.current = setTimeout(() => {
      runPreviewAll(expr);
    }, 350);
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [editing?.jq_expr, reviewTabs.join(',')]);

  const runPreviewAll = (expr: string) => {
    if (sampleDoc) {
      setSamplePreview((prev) => ({ ...prev, loading: true }));
      apiClient
        .post<{ block?: boolean; result?: unknown }>('/ci-rulesets/preview', { jq_expr: expr, document: sampleDoc })
        .then((res) => setSamplePreview({ loading: false, block: res.block, result: res.result, document: sampleDoc }))
        .catch((err) => setSamplePreview({ loading: false, error: err?.message || 'preview failed' }));
    }
    reviewTabs.forEach((reviewId) => {
      setReviewPreviews((prev) => ({ ...prev, [reviewId]: { ...(prev[reviewId] || {}), loading: true } }));
      apiClient
        .post<{ block?: boolean; result?: unknown; document?: CanonicalDoc }>('/ci-rulesets/preview', {
          jq_expr: expr,
          review_id: reviewId,
        })
        .then((res) =>
          setReviewPreviews((prev) => ({
            ...prev,
            [reviewId]: { loading: false, block: res.block, result: res.result, document: res.document },
          }))
        )
        .catch((err) =>
          setReviewPreviews((prev) => ({
            ...prev,
            [reviewId]: { loading: false, error: err?.message || 'preview failed' },
          }))
        );
    });
  };

  const addReviewTab = (reviewId: number) => {
    const next = reviewTabs.includes(reviewId) ? reviewTabs : [...reviewTabs, reviewId];
    setUrlTabs(next, reviewId);
  };

  const closeReviewTab = (reviewId: number) => {
    const next = reviewTabs.filter((x) => x !== reviewId);
    setReviewPreviews((prev) => {
      const copy = { ...prev };
      delete copy[reviewId];
      return copy;
    });
    setUrlTabs(next, activeTab === reviewId ? 'sample' : activeTab);
  };

  // Returns whether the save succeeded, so callers (the unsaved-changes
  // prompt's "Save & continue") can decide whether it's safe to navigate
  // away afterward. `suppressAutoNavigate` skips the usual "jump straight to
  // the integration code" redirect on first create -- used when the caller
  // (saveAndLeave) already has its own destination in mind.
  const save = async (opts?: { suppressAutoNavigate?: boolean }): Promise<boolean> => {
    if (!editing || !editing.name?.trim() || !editing.jq_expr?.trim()) {
      showToast('Name and jq expression are required', 'error');
      return false;
    }
    setSaving(true);
    try {
      const body = { name: editing.name.trim(), description: editing.description || '', jq_expr: editing.jq_expr.trim() };
      if (editing.id) {
        const res = await apiClient.put<{ ruleset: CIRuleset }>(`/ci-rulesets/${editing.id}`, body);
        setEditing({ ...res.ruleset });
        setSavedSnapshot({ name: res.ruleset.name, description: res.ruleset.description || '', jq_expr: res.ruleset.jq_expr });
        showToast('Ruleset updated', 'success');
      } else {
        const res = await apiClient.post<{ ruleset: CIRuleset }>('/ci-rulesets', body);
        setEditing({ ...res.ruleset });
        setSavedSnapshot({ name: res.ruleset.name, description: res.ruleset.description || '', jq_expr: res.ruleset.jq_expr });
        showToast('Ruleset created', 'success');
        if (!opts?.suppressAutoNavigate) {
          // First save: jump straight to the integration code so it's visible
          // immediately, rather than leaving the user on the edit page.
          navigate(`/ci-rulesets/${res.ruleset.id}/integration`, { replace: true });
        }
      }
      return true;
    } catch (err: any) {
      showToast(err?.message || 'Save failed', 'error');
      return false;
    } finally {
      setSaving(false);
    }
  };

  const sampleDocJson = useMemo(() => (sampleDoc ? JSON.stringify(sampleDoc, null, 2) : ''), [sampleDoc]);
  const llmPrompt = useMemo(
    () => buildLLMPrompt(sampleDocJson || '{ ... loading sample ... }', taxonomy),
    [sampleDocJson, taxonomy]
  );

  const activeOutcome: PreviewOutcome = activeTab === 'sample' ? samplePreview : reviewPreviews[activeTab] || { loading: false };
  const activeReview = activeTab === 'sample' ? null : recentReviews.find((r) => r.id === activeTab);
  const activeJson = activeOutcome.document ? JSON.stringify(activeOutcome.document, null, 2) : '';

  // Gate saving on the expression actually validating (evaluated against the
  // sample doc) -- a syntax error in samplePreview.error means the CI/CD
  // endpoint would also 500 on every call, so we refuse to save it.
  const jqInvalid = !!samplePreview.error;
  const canSave = !!editing?.name?.trim() && !!editing?.jq_expr?.trim() && !jqInvalid;

  const TabStatusIndicator: React.FC<{ outcome?: PreviewOutcome }> = ({ outcome }) => {
    if (!outcome) return null;
    if (outcome.loading) return <Spinner size="sm" />;
    if (outcome.error) return <span className="w-2 h-2 rounded-full bg-amber-500" title={outcome.error} />;
    if (outcome.result === undefined) return null;
    return (
      <span
        className={`w-2 h-2 rounded-full ${outcome.block ? 'bg-red-500' : 'bg-green-500'}`}
        title={outcome.block ? 'Would block' : 'Would allow'}
      />
    );
  };

  const gateHttpStatus = (outcome: PreviewOutcome): { code: number; label: string } | null => {
    const docStatus = outcome.document?.status;
    if (docStatus && !['completed', 'failed', 'error'].includes(docStatus)) {
      return { code: 202, label: 'Accepted — review still running' };
    }
    if (outcome.result === undefined) return null;
    return outcome.block ? { code: 422, label: 'Unprocessable Entity — blocked' } : { code: 200, label: 'OK — allowed' };
  };

  if (loadingRuleset || !editing) {
    return (
      <div className="container mx-auto px-4 py-6">
        <div className="flex justify-center py-24"><Spinner /></div>
      </div>
    );
  }

  return (
    <div className="container mx-auto px-4 py-6 space-y-6">
      <ToastContainer />
      <Breadcrumb
        items={[
          { label: 'CI/CD Gates', to: '/ci-rulesets' },
          { label: editing.id ? (editing.name || `Ruleset #${editing.id}`) : 'New ruleset' },
        ]}
        onBeforeNavigate={guardedNavigate}
      />
      <PageHeader
        title={editing.id ? `Edit ruleset: ${editing.name}` : 'New ruleset'}
        description="Write a jq rule against a review's findings, save it, and call one URL from any CI/CD pipeline to block or allow the build."
      />

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className={quickHelpOpen ? 'lg:col-span-2 space-y-4' : 'lg:col-span-3 space-y-4'}>
          <Card>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <Input
                label="Name"
                value={editing.name || ''}
                onChange={(e) => setEditing({ ...editing, name: e.target.value })}
                placeholder="e.g. Block on critical or security findings"
              />
              <Input
                label="Description (optional)"
                value={editing.description || ''}
                onChange={(e) => setEditing({ ...editing, description: e.target.value })}
                placeholder="What this rule is for"
              />
            </div>

            <div className="mt-4">
              <label className="block text-sm font-medium text-slate-300 mb-1">jq expression</label>

              {/* Toolbar: grouped with the editor as one visual block, buttons on the left
                  where writing starts. */}
              <div className="flex items-center gap-2 bg-slate-900/60 border border-slate-600 border-b-0 rounded-t-lg px-3 py-2 flex-wrap">
                <button
                  type="button"
                  onClick={openInsertReference}
                  className="inline-flex items-center rounded-md border border-slate-600 bg-slate-700 hover:bg-slate-600 text-slate-100 overflow-hidden transition-colors"
                >
                  <span className="flex items-center gap-1.5 px-3 py-1.5 text-sm"><Icons.List /> Insert reference</span>
                  <span className="px-2 py-1.5 text-[10px] font-mono text-slate-400 bg-slate-800/70 border-l border-slate-600">Ctrl+Space</span>
                </button>
                <span ref={llmButtonRef} className="inline-flex">
                  <Button size="sm" variant="secondary" icon={<Icons.AI />} onClick={openLlmHelper}>
                    Ask LLM
                  </Button>
                </span>
                <Button size="sm" variant={quickHelpOpen ? 'primary' : 'secondary'} icon={<Icons.Info />} onClick={() => setQuickHelpOpen((v) => !v)}>
                  jq quick help
                </Button>
                <span className="ml-auto flex items-center gap-3">
                  <a href={JQ_MANUAL_URL} target="_blank" rel="noopener noreferrer" className="text-xs text-blue-400 hover:underline">jq manual</a>
                  <a href={GOJQ_REPO_URL} target="_blank" rel="noopener noreferrer" className="text-xs text-blue-400 hover:underline">gojq</a>
                </span>
              </div>
              <textarea
                ref={textareaRef}
                className={`w-full rounded-b-lg border bg-slate-700 text-white outline-none px-4 py-3 font-mono text-base resize-y ${
                  jqInvalid ? 'border-red-500 focus:border-red-400 focus:ring-2 focus:ring-red-400' : 'border-slate-600 focus:border-blue-500 focus:ring-2 focus:ring-blue-400'
                }`}
                style={{ minHeight: '4.5rem' }}
                rows={3}
                value={editing.jq_expr || ''}
                onChange={(e) => setEditing({ ...editing, jq_expr: e.target.value })}
                onKeyDown={(e) => {
                  if (e.ctrlKey && e.code === 'Space') {
                    e.preventDefault();
                    openInsertReference();
                  }
                }}
                spellCheck={false}
              />
              <p className={`text-xs mt-1 ${jqInvalid ? 'text-red-400' : 'text-slate-500'}`}>
                {jqInvalid ? `Invalid jq expression: ${samplePreview.error}` : 'Truthy result blocks the build. Drag the bottom-right corner to resize. Live results below update as you type.'}
              </p>
            </div>

            <div className="flex flex-wrap gap-2 mt-3">
              {EXAMPLE_EXPRESSIONS.map((ex) => (
                <button
                  key={ex.label}
                  type="button"
                  onClick={() => setEditing({ ...editing, jq_expr: ex.expr })}
                  className="text-xs bg-slate-700/70 hover:bg-slate-600 text-slate-200 px-2.5 py-1.5 rounded-md border border-slate-600"
                  title={ex.expr}
                >
                  {ex.label}
                </button>
              ))}
            </div>
          </Card>

          {/* Live Results -- immediately below the jq editor, tabbed, with big
              pass/fail emphasis visible at both the tab and panel level. */}
          <Card title="Live results">
            <div className="flex items-center gap-1 border-b border-slate-700 -mt-2 mb-3 flex-wrap">
              <button
                type="button"
                onClick={() => setUrlTabs(reviewTabs, 'sample')}
                className={`flex items-center gap-2 px-3 py-2 text-sm font-medium border-b-2 -mb-px ${
                  activeTab === 'sample' ? 'border-blue-500 text-white' : 'border-transparent text-slate-400 hover:text-slate-200'
                }`}
              >
                <TabStatusIndicator outcome={samplePreview} />
                Sample
              </button>
              {reviewTabs.map((rid) => {
                const outcome = reviewPreviews[rid];
                return (
                  <span
                    key={rid}
                    className={`flex items-center gap-2 px-3 py-2 text-sm font-medium border-b-2 -mb-px cursor-pointer ${
                      activeTab === rid ? 'border-blue-500 text-white' : 'border-transparent text-slate-400 hover:text-slate-200'
                    }`}
                    onClick={() => setUrlTabs(reviewTabs, rid)}
                  >
                    <TabStatusIndicator outcome={outcome} />
                    #{rid}
                    <button
                      type="button"
                      onClick={(e) => { e.stopPropagation(); closeReviewTab(rid); }}
                      className="text-slate-500 hover:text-white ml-1"
                      title="Remove this tab"
                    >
                      ×
                    </button>
                  </span>
                );
              })}
              <button
                type="button"
                onClick={() => setAddTabOpen(true)}
                className="px-3 py-2 text-sm font-medium text-blue-400 hover:text-blue-300"
                title="Add a past review as a tab"
              >
                + Add review
              </button>
            </div>

            <div className="flex items-center justify-between gap-3 mb-3">
              <div className="min-w-0">
                <h4 className="text-sm font-semibold text-slate-100 truncate">
                  {activeTab === 'sample' ? 'Synthetic sample document' : `#${activeTab} ${activeReview ? getPrimaryTitle(activeReview) : ''}`}
                </h4>
                {activeReview && <p className="text-xs text-slate-500 truncate">{activeReview.repository}</p>}
              </div>
              {activeOutcome.loading && <Spinner size="sm" />}
            </div>

            {/* Compact, tasteful result summary: badge + HTTP status the real
                gate would return + the raw jq result value. */}
            {!activeOutcome.loading && activeOutcome.error && (
              <div className="flex items-center gap-2 mb-3">
                <Badge variant="warning">ERROR</Badge>
                <span className="text-sm text-amber-300">{activeOutcome.error}</span>
              </div>
            )}
            {!activeOutcome.loading && !activeOutcome.error && activeOutcome.result !== undefined && (() => {
              const status = gateHttpStatus(activeOutcome);
              return (
                <div className="flex items-center gap-3 mb-3 flex-wrap">
                  <GateResultPill block={!!activeOutcome.block} />
                  {status && (
                    <span className="text-xs font-mono bg-slate-900 border border-slate-700 rounded px-2 py-1 text-slate-300" title={status.label}>
                      HTTP {status.code}
                    </span>
                  )}
                  <span className="text-xs text-slate-500 font-mono">jq result: {JSON.stringify(activeOutcome.result)}</span>
                </div>
              );
            })()}

            <pre className="text-xs bg-slate-900/70 border border-slate-700 rounded px-3 py-3 text-slate-300 overflow-auto" style={{ height: '48vh', minHeight: '320px' }}>
              {activeJson || 'Loading…'}
            </pre>
          </Card>

          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => guardedNavigate('/ci-rulesets')}>Cancel</Button>
            <Button
              variant="outline"
              icon={<Icons.Ready />}
              disabled={!editing.id}
              title={!editing.id ? 'Save this ruleset first to get its integration code' : undefined}
              onClick={() => editing.id && guardedNavigate(`/ci-rulesets/${editing.id}/integration`)}
            >
              Get integration code
            </Button>
            <Button onClick={() => save()} isLoading={saving} disabled={!canSave} title={jqInvalid ? 'Fix the jq expression before saving' : undefined}>
              {editing.id ? 'Save changes' : 'Create ruleset'}
            </Button>
          </div>
        </div>

        {quickHelpOpen && (
          <div className="lg:col-span-1">
            <div className="sticky top-4 bg-slate-800 border border-slate-700 rounded-lg max-h-[85vh] overflow-y-auto">
              <div className="flex justify-end p-2">
                <button onClick={() => setQuickHelpOpen(false)} className="text-slate-400 hover:text-white"><Icons.Close /></button>
              </div>
              <div className="-mt-10">
                <JqQuickHelp />
              </div>
            </div>
          </div>
        )}
      </div>

      <FloatingPanel open={!!insertPoint} point={insertPoint} onClose={() => setInsertPoint(null)} widthPx={520}>
        <InsertReferenceContent taxonomy={taxonomy} onInsert={insertAtCursor} onClose={() => setInsertPoint(null)} />
      </FloatingPanel>

      <FloatingPanel open={!!llmPoint} point={llmPoint} onClose={() => setLlmPoint(null)} widthPx={420}>
        <LLMHelperContent prompt={llmPrompt} onCopy={() => copyPrompt(llmPrompt)} onClose={() => setLlmPoint(null)} />
      </FloatingPanel>

      <AddReviewTabDialog
        open={addTabOpen}
        reviews={recentReviews}
        alreadyAdded={reviewTabs}
        onAdd={addReviewTab}
        onClose={() => setAddTabOpen(false)}
      />

      <UnsavedChangesDialog
        open={!!pendingNav}
        saving={saving}
        onSaveAndContinue={saveAndLeave}
        onDiscard={discardAndLeave}
        onKeepEditing={() => setPendingNav(null)}
      />
    </div>
  );
};

export default CiRulesetEditor;
