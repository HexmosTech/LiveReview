import React, { useEffect, useRef, useState } from 'react';
import { Alert, Badge, Button, Spinner } from '../UIPrimitives';
import ToolInfoPopover from '../tools/ToolInfoPopover';
import { useOrgContext } from '../../hooks/useOrgContext';
import {
  RepoToolRuleConfig,
  RepoToolView,
  getRepositoryTools,
  updateRepositoryTools,
} from '../../api/repositories';
import { Repository } from '../../types/explore';
import ToolsTomlEditor from './ToolsTomlEditor';

// Repository settings -> Third-Party Tools (Cloud paid plans only; applies only to PR reviews).
const PAGE_SIZE = 10;
type SortField = 'name' | 'use_case' | 'category' | 'multiplier' | 'enabled';

function normalizeRuleConfig(cfg?: RepoToolRuleConfig): RepoToolRuleConfig {
  return {
    ...(cfg?.include?.length ? { include: cfg.include } : {}),
    ...(cfg?.exclude?.length ? { exclude: cfg.exclude } : {}),
  };
}

function configsEqual(a?: RepoToolRuleConfig, b?: RepoToolRuleConfig): boolean {
  const na = normalizeRuleConfig(a);
  const nb = normalizeRuleConfig(b);
  return JSON.stringify(na.include || []) === JSON.stringify(nb.include || [])
    && JSON.stringify(na.exclude || []) === JSON.stringify(nb.exclude || []);
}

const RepoToolsSection: React.FC<{ repo: Repository }> = ({ repo }) => {
  const { isSuperAdmin, currentOrg, canManageCurrentOrg } = useOrgContext();
  const canEdit = Boolean(canManageCurrentOrg || currentOrg?.role === 'owner' || isSuperAdmin);

  const [tools, setTools] = useState<RepoToolView[]>([]);
  const [savedEnabled, setSavedEnabled] = useState<Record<string, boolean>>({});
  const [savedConfigs, setSavedConfigs] = useState<Record<string, RepoToolRuleConfig>>({});
  const [draftEnabled, setDraftEnabled] = useState<Record<string, boolean>>({});
  const [draftConfigs, setDraftConfigs] = useState<Record<string, RepoToolRuleConfig>>({});

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const [sortField, setSortField] = useState<SortField>('name');
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc');
  const [currentPage, setCurrentPage] = useState(1);
  const [viewMode, setViewMode] = useState<'simple' | 'advanced'>('simple');
  const [tomlSyncVersion, setTomlSyncVersion] = useState(0);
  const [tomlValid, setTomlValid] = useState(true);

  const [hoveredInfoId, setHoveredInfoId] = useState<number | null>(null);
  const [pinnedInfoId, setPinnedInfoId] = useState<number | null>(null);
  const closeHoverTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const saveSuccessTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const clearHoverTimer = () => {
    if (closeHoverTimerRef.current) {
      clearTimeout(closeHoverTimerRef.current);
      closeHoverTimerRef.current = null;
    }
  };

  useEffect(() => {
    const handleDocMouseDown = (event: MouseEvent) => {
      const target = event.target as HTMLElement | null;
      if (target && target.closest('[data-tool-info-root="true"]')) {
        return;
      }
      setPinnedInfoId(null);
      setHoveredInfoId(null);
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setPinnedInfoId(null);
        setHoveredInfoId(null);
      }
    };
    document.addEventListener('mousedown', handleDocMouseDown);
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      clearHoverTimer();
      if (saveSuccessTimerRef.current) {
        clearTimeout(saveSuccessTimerRef.current);
        saveSuccessTimerRef.current = null;
      }
      document.removeEventListener('mousedown', handleDocMouseDown);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, []);

  const syncFromResponse = (list: RepoToolView[]) => {
    setTools(list);
    const nextEnabled: Record<string, boolean> = {};
    const nextConfigs: Record<string, RepoToolRuleConfig> = {};
    list.forEach((t) => {
      const key = t.name.toLowerCase();
      nextEnabled[key] = typeof t.repo_enabled === 'boolean' ? t.repo_enabled : false;
      const cfg = normalizeRuleConfig(t.config_json);
      if (cfg.include?.length || cfg.exclude?.length) {
        nextConfigs[key] = cfg;
      }
    });
    setSavedEnabled(nextEnabled);
    setSavedConfigs(nextConfigs);
    setDraftEnabled(nextEnabled);
    setDraftConfigs(nextConfigs);
    setTomlValid(true);
    setTomlSyncVersion((v) => v + 1);
  };

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(null);
    setSuccess(null);
    getRepositoryTools(repo.id)
      .then((res) => {
        if (!active) return;
        syncFromResponse(res.tools || []);
      })
      .catch((e: any) => {
        if (!active) return;
        setError(e?.message || 'Failed to load repository tool settings');
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [repo.id]);

  const handleToggleTool = (toolName: string) => {
    if (!canEdit || saving) return;
    const key = toolName.toLowerCase();
    setDraftEnabled((prev) => ({
      ...prev,
      [key]: !prev[key],
    }));
    setTomlSyncVersion((v) => v + 1);
    setSuccess(null);
  };

  const handleTomlStateChange = (
    enabledMap: Record<string, boolean> | null,
    ruleConfigs?: Record<string, RepoToolRuleConfig>,
  ) => {
    setTomlValid(enabledMap !== null);
    if (!enabledMap) return;
    const syncedEnabled: Record<string, boolean> = {};
    const syncedConfigs: Record<string, RepoToolRuleConfig> = {};
    tools.forEach((t) => {
      const key = t.name.toLowerCase();
      syncedEnabled[key] = key in enabledMap ? enabledMap[key] : false;
      if (ruleConfigs && ruleConfigs[key]) {
        const cfg = normalizeRuleConfig(ruleConfigs[key]);
        if (cfg.include?.length || cfg.exclude?.length) {
          syncedConfigs[key] = cfg;
        }
      }
    });
    setDraftEnabled(syncedEnabled);
    setDraftConfigs(syncedConfigs);
    setSuccess(null);
  };

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortField(field);
      setSortDir('asc');
    }
    setCurrentPage(1);
  };

  const renderSortIcon = (field: SortField) => {
    if (sortField !== field) {
      return (
        <svg className="w-3 h-3 ml-1 opacity-30 group-hover:opacity-70 inline" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M8 9l4-4 4 4m0 6l-4 4-4-4" />
        </svg>
      );
    }
    return sortDir === 'asc' ? (
      <svg className="w-3 h-3 ml-1 text-violet-400 inline" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M5 15l7-7 7 7" />
      </svg>
    ) : (
      <svg className="w-3 h-3 ml-1 text-violet-400 inline" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M19 9l-7 7-7-7" />
      </svg>
    );
  };

  const isToolActive = (tool: RepoToolView): boolean => {
    const key = tool.name.toLowerCase();
    return Boolean(draftEnabled[key]);
  };

  const sortedTools = [...tools].sort((a, b) => {
    let valA: string | number;
    let valB: string | number;
    if (sortField === 'name') { valA = a.name.toLowerCase(); valB = b.name.toLowerCase(); }
    else if (sortField === 'use_case') { valA = (a.use_case || '').toLowerCase(); valB = (b.use_case || '').toLowerCase(); }
    else if (sortField === 'category') { valA = (a.category || '').toLowerCase(); valB = (b.category || '').toLowerCase(); }
    else if (sortField === 'multiplier') { valA = Number(a.multiplier); valB = Number(b.multiplier); }
    else { valA = isToolActive(a) ? 1 : 0; valB = isToolActive(b) ? 1 : 0; }
    if (valA < valB) return sortDir === 'asc' ? -1 : 1;
    if (valA > valB) return sortDir === 'asc' ? 1 : -1;
    return 0;
  });

  const totalPages = Math.ceil(sortedTools.length / PAGE_SIZE);
  const activePg = Math.min(currentPage, Math.max(1, totalPages));
  const pagedTools = sortedTools.slice((activePg - 1) * PAGE_SIZE, activePg * PAGE_SIZE);

  const hasChanges = tools.some((t) => {
    const key = t.name.toLowerCase();
    if (Boolean(draftEnabled[key]) !== Boolean(savedEnabled[key])) return true;
    if (!configsEqual(draftConfigs[key], savedConfigs[key])) return true;
    return false;
  });

  const handleSave = async () => {
    if (!canEdit || saving || (viewMode === 'advanced' && !tomlValid)) return;
    setSaving(true);
    setError(null);
    setSuccess(null);
    try {
      const payloadTools = tools.reduce<Record<string, boolean>>((acc, t) => {
        const key = t.name.toLowerCase();
        acc[key] = Boolean(draftEnabled[key]);
        return acc;
      }, {});
      const payloadConfigs = tools.reduce<Record<string, RepoToolRuleConfig>>((acc, t) => {
        const key = t.name.toLowerCase();
        const cfg = normalizeRuleConfig(draftConfigs[key]);
        if (cfg.include?.length || cfg.exclude?.length) {
          acc[key] = cfg;
        }
        return acc;
      }, {});
      const res = await updateRepositoryTools(repo.id, {
        mode: 'custom',
        tools: payloadTools,
        tool_configs: payloadConfigs,
      });
      syncFromResponse(res.tools || []);
      setSuccess('Saved repository tool settings for PR reviews');
      if (saveSuccessTimerRef.current) {
        clearTimeout(saveSuccessTimerRef.current);
      }
      saveSuccessTimerRef.current = setTimeout(() => setSuccess(null), 3000);
    } catch (e: any) {
      setError(e?.message || 'Failed to save repository tool settings');
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <div className="flex justify-center py-16">
        <Spinner />
      </div>
    );
  }

  const activeCount = tools.filter((t) => isToolActive(t)).length;
  const activeMultiplier = tools.reduce((sum, t) => {
    if (!isToolActive(t)) return sum;
    const m = Number(t.multiplier);
    return sum + (Number.isFinite(m) ? m : 0);
  }, 0);

  return (
    <div className="p-6 space-y-6">
      <div className="space-y-1">
        <h2 className="text-lg font-semibold text-white">SAST Tools</h2>
        <p className="text-sm text-slate-400">
          Choose which static analysis tools run during pull request reviews for this repository, and optionally filter paths with include/exclude globs.
        </p>
      </div>

      {!canEdit && (
        <Alert variant="warning">
          You need the organization owner role to edit this repository&apos;s tool settings.
        </Alert>
      )}
      {error && (
        <Alert variant="error" onClose={() => setError(null)}>
          {error}
        </Alert>
      )}
      {success && (
        <Alert variant="success" onClose={() => setSuccess(null)}>
          {success}
        </Alert>
      )}

      {/* Priority A: Simple vs Advanced mode switcher + Summary & Save toolbar */}
      <div className="flex flex-wrap items-center justify-between gap-4 pb-4 border-b border-slate-800">
        <div className="inline-flex items-center gap-1 bg-slate-900/60 border border-slate-700/60 rounded-lg p-1">
          <button
            type="button"
            onClick={() => {
              setViewMode('simple');
              setTomlValid(true);
            }}
            className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
              viewMode === 'simple'
                ? 'bg-violet-600 text-white shadow-sm'
                : 'text-slate-400 hover:text-white'
            }`}
          >
            <span>Simple</span>
            <span className="relative group/simpleinfo inline-flex items-center" onClick={(e) => e.stopPropagation()}>
              <span
                tabIndex={0}
                role="img"
                aria-label="Configure each tool individually using toggles"
                title="Configure each tool individually using toggles"
                className={`inline-flex h-4 w-4 items-center justify-center rounded-full border text-[10px] font-semibold ${
                  viewMode === 'simple'
                    ? 'border-white/40 text-white/90 bg-white/10'
                    : 'border-slate-600 text-slate-400 bg-slate-800/80'
                }`}
              >
                i
              </span>
              <span className="pointer-events-none hidden group-hover/simpleinfo:block group-focus-within/simpleinfo:block absolute left-1/2 -translate-x-1/2 bottom-full mb-2 w-56 rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-1.5 text-xs font-normal text-slate-200 shadow-xl z-30">
                Configure each tool individually with toggle switches.
              </span>
            </span>
          </button>

          <button
            type="button"
            onClick={() => setViewMode('advanced')}
            className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-colors ${
              viewMode === 'advanced'
                ? 'bg-violet-600 text-white shadow-sm'
                : 'text-slate-400 hover:text-white'
            }`}
          >
            <span>Advanced</span>
            <span className="relative group/advinfo inline-flex items-center" onClick={(e) => e.stopPropagation()}>
              <span
                tabIndex={0}
                role="img"
                aria-label="Configure tools using rules editor with include/exclude path globs"
                title="Configure tools using rules editor with include/exclude path globs"
                className={`inline-flex h-4 w-4 items-center justify-center rounded-full border text-[10px] font-semibold ${
                  viewMode === 'advanced'
                    ? 'border-white/40 text-white/90 bg-white/10'
                    : 'border-slate-600 text-slate-400 bg-slate-800/80'
                }`}
              >
                i
              </span>
              <span className="pointer-events-none hidden group-hover/advinfo:block group-focus-within/advinfo:block absolute left-1/2 -translate-x-1/2 bottom-full mb-2 w-64 rounded-lg border border-slate-700 bg-slate-900 px-2.5 py-1.5 text-xs font-normal text-slate-200 shadow-xl z-30">
                Write tools.toml rules with include/exclude globs, category search popup, and live preview table.
              </span>
            </span>
          </button>
        </div>

        <div className="flex flex-wrap items-center gap-5 text-sm">
          <div>
            <span className="text-slate-400">Active: </span>
            <span className="font-semibold text-white tabular-nums">{activeCount}</span>
            <span className="text-slate-400 tabular-nums"> / {tools.length}</span>
          </div>
          <div>
            <span className="text-slate-400">Multiplier: </span>
            <span className="font-semibold text-violet-300 tabular-nums">{activeMultiplier.toFixed(1)}×</span>
          </div>
          <div className="text-xs">
            {hasChanges ? (
              <span className="text-amber-400 font-medium inline-flex items-center">
                <span className="w-2 h-2 rounded-full bg-amber-400 animate-pulse mr-1.5" />
                Unsaved changes
              </span>
            ) : (
              <span className="text-emerald-400 font-medium inline-flex items-center">
                <span className="w-2 h-2 rounded-full bg-emerald-400 mr-1.5" />
                Saved
              </span>
            )}
          </div>
          {canEdit && (
            <Button
              variant="primary"
              size="sm"
              onClick={handleSave}
              disabled={!hasChanges || saving || (viewMode === 'advanced' && !tomlValid)}
              isLoading={saving}
              title={viewMode === 'advanced' && !tomlValid ? 'Fix TOML syntax errors before saving' : undefined}
            >
              {saving ? 'Saving...' : 'Save Changes'}
            </Button>
          )}
        </div>
      </div>

      {/* Priority B: Simple table (with strict toggles) OR Advanced CI/CD-style tools.toml editor */}
      {viewMode === 'advanced' ? (
        <ToolsTomlEditor
          tools={tools}
          draftEnabled={draftEnabled}
          draftConfigs={draftConfigs}
          canEdit={canEdit}
          saving={saving}
          syncVersion={tomlSyncVersion}
          onTomlStateChange={handleTomlStateChange}
        />
      ) : tools.length === 0 ? (
        <div className="text-center py-10 bg-slate-900/40 rounded-xl border border-slate-800">
          <p className="text-slate-300 font-medium">No tools registered</p>
          <p className="text-sm text-slate-400 mt-1">SAST tools are not available at the moment.</p>
        </div>
      ) : (
        <div className="overflow-visible border border-slate-700/80 rounded-xl bg-slate-900/40">
          <table className="w-full text-left border-collapse">
            <thead>
              <tr className="border-b border-slate-700 bg-slate-800/50 select-none">
                <th
                  onClick={() => handleSort('name')}
                  className="first:rounded-tl-xl p-3.5 text-xs font-semibold uppercase tracking-wider text-slate-400 cursor-pointer hover:bg-slate-800/80 hover:text-white transition-colors group"
                >
                  Tool {renderSortIcon('name')}
                </th>
                <th
                  onClick={() => handleSort('use_case')}
                  className="p-3.5 text-xs font-semibold uppercase tracking-wider text-slate-400 cursor-pointer hover:bg-slate-800/80 hover:text-white transition-colors group"
                >
                  Use Case {renderSortIcon('use_case')}
                </th>
                <th
                  onClick={() => handleSort('category')}
                  className="p-3.5 text-xs font-semibold uppercase tracking-wider text-slate-400 cursor-pointer hover:bg-slate-800/80 hover:text-white transition-colors group"
                >
                  Category {renderSortIcon('category')}
                </th>
                <th
                  onClick={() => handleSort('multiplier')}
                  className="p-3.5 text-xs font-semibold uppercase tracking-wider text-slate-400 cursor-pointer hover:bg-slate-800/80 hover:text-white transition-colors group"
                >
                  Multiplier {renderSortIcon('multiplier')}
                </th>
                <th
                  onClick={() => handleSort('enabled')}
                  className="last:rounded-tr-xl p-3.5 text-xs font-semibold uppercase tracking-wider text-slate-400 text-right cursor-pointer hover:bg-slate-800/80 hover:text-white transition-colors group"
                >
                  Status {renderSortIcon('enabled')}
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800">
              {pagedTools.map((tool, index) => {
                const enabled = isToolActive(tool);
                const openUpward = pagedTools.length > 3 && index >= pagedTools.length - 3;
                const toolCfg = draftConfigs[tool.name.toLowerCase()];
                const hasFilters = Boolean(toolCfg?.include?.length || toolCfg?.exclude?.length);
                return (
                  <tr key={tool.id} className="group/row hover:bg-slate-800/40 transition-colors">
                    <td className="p-3.5">
                      <div className="flex items-center gap-2">
                        <span className="font-medium text-white text-sm">{tool.name}</span>
                        <ToolInfoPopover
                          tool={tool}
                          openUpward={openUpward}
                          isHovered={hoveredInfoId === tool.id}
                          isPinned={pinnedInfoId === tool.id}
                          onHoverStart={() => {
                            clearHoverTimer();
                            setHoveredInfoId(tool.id);
                          }}
                          onHoverEnd={() => {
                            clearHoverTimer();
                            closeHoverTimerRef.current = setTimeout(() => {
                              setHoveredInfoId((prev) => (prev === tool.id ? null : prev));
                            }, 150);
                          }}
                          onTogglePin={() => setPinnedInfoId((prev) => (prev === tool.id ? null : tool.id))}
                        />
                        {hasFilters && (
                          <button
                            type="button"
                            onClick={() => setViewMode('advanced')}
                            className="text-[11px] rounded-full border border-blue-500/40 bg-blue-500/10 px-2 py-0.5 text-blue-300 hover:bg-blue-500/20"
                            title="Path globs active — click to edit in Advanced mode"
                          >
                            filtered
                          </button>
                        )}
                      </div>
                    </td>
                    <td className="p-3.5 text-sm text-slate-300">{tool.use_case || 'General Analysis'}</td>
                    <td className="p-3.5 text-sm text-slate-300">
                      {tool.category ? (
                        <span className="inline-flex items-center rounded-full border border-slate-700 bg-slate-800/80 px-2.5 py-0.5 text-xs font-medium text-slate-300">
                          {tool.category}
                        </span>
                      ) : (
                        <span className="text-slate-500">—</span>
                      )}
                    </td>
                    <td className="p-3.5 text-sm font-semibold text-slate-300 tabular-nums">
                      {Number(tool.multiplier).toFixed(1)}×
                    </td>
                    <td className="p-3.5 text-right">
                      {canEdit ? (
                        <button
                          type="button"
                          role="switch"
                          aria-checked={enabled}
                          aria-label={`Toggle ${tool.name}`}
                          onClick={() => handleToggleTool(tool.name)}
                          disabled={saving}
                          className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors duration-200 focus:outline-none focus:ring-2 focus:ring-violet-500 focus:ring-offset-2 focus:ring-offset-slate-900 ${
                            enabled ? 'bg-violet-600' : 'bg-slate-700'
                          } ${saving ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}`}
                        >
                          <span
                            className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform duration-200 ${
                              enabled ? 'translate-x-6' : 'translate-x-1'
                            }`}
                          />
                        </button>
                      ) : (
                        <Badge variant={enabled ? 'success' : 'default'}>
                          {enabled ? 'Enabled' : 'Disabled'}
                        </Badge>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>

          {/* Pagination footer */}
          {totalPages > 1 && (
            <div className="flex items-center justify-between rounded-b-xl border-t border-slate-700 bg-slate-900/60 px-4 py-3">
              <p className="text-sm text-slate-400">
                Showing <span className="font-medium text-white">{(activePg - 1) * PAGE_SIZE + 1}</span>–
                <span className="font-medium text-white">{Math.min(activePg * PAGE_SIZE, sortedTools.length)}</span>
                {' '}of <span className="font-medium text-white">{sortedTools.length}</span> tools
              </p>
              <nav className="isolate inline-flex -space-x-px rounded-md shadow-sm">
                <button
                  onClick={() => setCurrentPage((p) => Math.max(p - 1, 1))}
                  disabled={activePg === 1}
                  className="relative inline-flex items-center rounded-l-md px-2 py-2 text-slate-400 ring-1 ring-inset ring-slate-700 hover:bg-slate-800 disabled:opacity-30 disabled:cursor-not-allowed"
                  aria-label="Previous page"
                >
                  <svg className="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M12.79 5.23a.75.75 0 01-.02 1.06L8.832 10l3.938 3.71a.75.75 0 11-1.04 1.08l-4.5-4.25a.75.75 0 010-1.08l4.5-4.25a.75.75 0 011.06.02z" clipRule="evenodd" />
                  </svg>
                </button>
                {Array.from({ length: totalPages }, (_, i) => i + 1).map((pg) => (
                  <button
                    key={pg}
                    onClick={() => setCurrentPage(pg)}
                    className={`relative inline-flex items-center px-3.5 py-2 text-sm font-semibold ${
                      pg === activePg
                        ? 'z-10 bg-violet-600 text-white'
                        : 'text-slate-300 ring-1 ring-inset ring-slate-700 hover:bg-slate-800'
                    }`}
                  >
                    {pg}
                  </button>
                ))}
                <button
                  onClick={() => setCurrentPage((p) => Math.min(p + 1, totalPages))}
                  disabled={activePg === totalPages}
                  className="relative inline-flex items-center rounded-r-md px-2 py-2 text-slate-400 ring-1 ring-inset ring-slate-700 hover:bg-slate-800 disabled:opacity-30 disabled:cursor-not-allowed"
                  aria-label="Next page"
                >
                  <svg className="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M7.21 14.77a.75.75 0 01.02-1.06L11.168 10 7.23 6.29a.75.75 0 111.04-1.08l4.5 4.25a.75.75 0 010 1.08l-4.5 4.25a.75.75 0 01-1.06-.02z" clipRule="evenodd" />
                  </svg>
                </button>
              </nav>
            </div>
          )}
        </div>
      )}

      <p className="text-xs text-slate-500">
        If the repository also contains .lrc/policy/tools.toml in git, its entries and file filters overlay on top of these settings.
      </p>
    </div>
  );
};

export default RepoToolsSection;
