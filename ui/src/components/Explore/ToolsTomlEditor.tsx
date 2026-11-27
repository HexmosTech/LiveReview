import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Badge, Button, Icons, Spinner } from '../UIPrimitives';
import {
  ReferenceChip,
  ScreenPoint,
  getCaretScreenPoint,
} from '../../pages/CiRulesets/shared';
import {
  previewRepositoryTools,
  RepoToolRuleConfig,
  RepoToolView,
} from '../../api/repositories';
import { ToolsTomlQuickHelp } from './ToolsTomlQuickHelp';
import {
  ParsedToolRule,
  buildTomlLLMPrompt,
  evaluateToolForPaths,
  parseToolsTomlDetailed,
  toolsToToml,
  upsertToolSectionInToml,
} from './toolsToml';

const INCLUDE_SUGGESTIONS = [
  'src/**',
  '**/*.py',
  '**/*.ts',
  '**/*.tsx',
  '**/*.go',
  '**/*.tf',
  'Dockerfile*',
  '.github/workflows/**',
];

const EXCLUDE_SUGGESTIONS = [
  'tests/**',
  'node_modules/**',
  'vendor/**',
  'dist/**',
  '**/*.md',
];

const LIVE_TABLE_PAGE_SIZE = 5;

const DEFAULT_SAMPLE_PATHS = [
  'src/api/auth.py',
  'tests/test_auth.py',
  'web/src/App.tsx',
  'Dockerfile',
  'infra/main.tf',
  'README.md',
].join('\n');

// Document-anchored portal popover for SAST Tools so the panel stays attached
// to the control/row where it was opened rather than following viewport scroll.
const ToolsFloatingPanel: React.FC<{
  open: boolean;
  point: ScreenPoint | null;
  onClose: () => void;
  widthPx: number;
  children: React.ReactNode;
}> = ({ open, point, onClose, widthPx, children }) => {
  const panelRef = useRef<HTMLDivElement>(null);
  const [style, setStyle] = useState<{ top: number; left: number }>({ top: -9999, left: -9999 });

  useLayoutEffect(() => {
    if (!open || !point || !panelRef.current) return;
    const rect = panelRef.current.getBoundingClientRect();
    const margin = 12;
    const scrollX = window.scrollX || window.pageXOffset || 0;
    const scrollY = window.scrollY || window.pageYOffset || 0;
    let left = point.x;
    let top = point.y;
    if (left + rect.width > window.innerWidth - margin) left = window.innerWidth - rect.width - margin;
    if (left < margin) left = margin;
    if (top + rect.height > window.innerHeight - margin) {
      const above = point.y - rect.height - 28;
      top = above > margin ? above : Math.max(margin, window.innerHeight - rect.height - margin);
    }
    setStyle({ top: top + scrollY, left: left + scrollX });
  }, [open, point]);

  useEffect(() => {
    if (!open) return;
    const onMouseDown = (e: MouseEvent) => {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) onClose();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('mousedown', onMouseDown);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onMouseDown);
      document.removeEventListener('keydown', onKey);
    };
  }, [open, onClose]);

  if (!open) return null;
  return createPortal(
    <div
      ref={panelRef}
      style={{ position: 'absolute', top: style.top, left: style.left, width: widthPx }}
      className="z-[9999] max-w-[92vw] rounded-lg border border-slate-600 bg-slate-800 shadow-2xl max-h-[70vh] overflow-y-auto"
    >
      {children}
    </div>,
    document.body,
  );
};

// ---- Popup UI: Search by category/tool + Configure enabled/include/exclude ----

const ConfigureToolPopup: React.FC<{
  tools: RepoToolView[];
  initialTool?: RepoToolView | null;
  currentRules: Record<string, ParsedToolRule>;
  currentEnabledMap: Record<string, boolean>;
  onApplyTool: (toolName: string, enabled: boolean, config: RepoToolRuleConfig) => void;
  onClose: () => void;
}> = ({
  tools,
  initialTool = null,
  currentRules,
  currentEnabledMap,
  onApplyTool,
  onClose,
}) => {
  const [filter, setFilter] = useState('');
  const [selectedCategory, setSelectedCategory] = useState<string | null>(null);
  const selectedTool = initialTool;

  const [editEnabled, setEditEnabled] = useState(true);
  const [includeInput, setIncludeInput] = useState('');
  const [excludeInput, setExcludeInput] = useState('');

  // Initialize form inputs only when the selected tool changes, not on parent re-renders.
  useEffect(() => {
    if (!selectedTool) return;
    const key = selectedTool.name.toLowerCase();
    const existingRule = currentRules[key];
    const isEnabled =
      key in currentEnabledMap
        ? currentEnabledMap[key]
        : typeof existingRule?.enabled === 'boolean'
          ? existingRule.enabled
          : true;
    setEditEnabled(isEnabled);
    setIncludeInput((existingRule?.include || []).join(', '));
    setExcludeInput((existingRule?.exclude || []).join(', '));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedTool?.id]);

  const categories = useMemo(() => {
    const set = new Set<string>();
    tools.forEach((t) => set.add(t.category?.trim() || 'General'));
    return Array.from(set).sort((a, b) => a.localeCompare(b));
  }, [tools]);

  const q = filter.trim().toLowerCase();
  const filtered = useMemo(() => {
    return tools.filter((t) => {
      const cat = t.category?.trim() || 'General';
      if (selectedCategory && cat !== selectedCategory) return false;
      if (!q) return true;
      return (
        t.name.toLowerCase().includes(q) ||
        cat.toLowerCase().includes(q) ||
        (t.use_case || '').toLowerCase().includes(q) ||
        (t.description || '').toLowerCase().includes(q)
      );
    });
  }, [tools, selectedCategory, q]);

  const grouped = useMemo(() => {
    const map: Record<string, RepoToolView[]> = {};
    filtered.forEach((t) => {
      const cat = t.category?.trim() || 'General';
      if (!map[cat]) map[cat] = [];
      map[cat].push(t);
    });
    return Object.entries(map).sort(([a], [b]) => a.localeCompare(b));
  }, [filtered]);

  const parseCsvGlobs = (raw: string): string[] => {
    const items: string[] = [];
    let cur = '';
    let braceDepth = 0;
    for (let i = 0; i < raw.length; i++) {
      const ch = raw[i];
      if (ch === '{') braceDepth++;
      else if (ch === '}' && braceDepth > 0) braceDepth--;
      if ((ch === ',' && braceDepth === 0) || ch === '\n') {
        const trimmed = cur.trim();
        if (trimmed) items.push(trimmed);
        cur = '';
      } else {
        cur += ch;
      }
    }
    const tail = cur.trim();
    if (tail) items.push(tail);
    return items;
  };

  const toggleGlobChip = (
    currentRaw: string,
    setter: (next: string) => void,
    glob: string,
  ) => {
    const list = parseCsvGlobs(currentRaw);
    const exists = list.includes(glob);
    const next = exists ? list.filter((x) => x !== glob) : [...list, glob];
    setter(next.join(', '));
  };

  if (selectedTool) {
    const incList = parseCsvGlobs(includeInput);
    const excList = parseCsvGlobs(excludeInput);

    return (
      <div className="p-4 space-y-4">
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2 min-w-0">
            <h3 className="text-sm font-semibold text-white truncate">{selectedTool.name}</h3>
            {selectedTool.category && (
              <span className="rounded-full border border-slate-700 bg-slate-900 px-2 py-0.5 text-[11px] text-slate-300">
                {selectedTool.category}
              </span>
            )}
          </div>
          <button
            type="button"
            onClick={onClose}
            className="text-slate-400 hover:text-white"
            aria-label="Close configure tool panel"
          >
            <Icons.Close />
          </button>
        </div>

        <p className="text-xs text-slate-400">
          {selectedTool.use_case || selectedTool.description} ·{' '}
          <span className="text-violet-300 font-medium tabular-nums">
            {Number(selectedTool.multiplier).toFixed(1)}× cost
          </span>
        </p>

        {/* Enabled toggle */}
        <div className="flex items-center justify-between rounded-lg border border-slate-700 bg-slate-900/50 px-3 py-2.5">
          <div>
            <div className="text-xs font-semibold text-white">Enable {selectedTool.name}</div>
            <div className="text-[11px] text-slate-400">Run during pull request reviews on this repository</div>
          </div>
          <button
            type="button"
            role="switch"
            aria-checked={editEnabled}
            aria-label={`Popup toggle ${selectedTool.name}`}
            onClick={() => setEditEnabled((v) => !v)}
            className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
              editEnabled ? 'bg-violet-600' : 'bg-slate-700'
            }`}
          >
            <span
              className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                editEnabled ? 'translate-x-6' : 'translate-x-1'
              }`}
            />
          </button>
        </div>

        {/* Include globs */}
        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-slate-300">
            Include paths <span className="text-slate-500 font-normal">(comma-separated globs, optional)</span>
          </label>
          <input
            type="text"
            className="w-full rounded-lg border border-slate-600 bg-slate-900 text-xs text-white px-3 py-2 outline-none focus:border-blue-500 placeholder:text-slate-500"
            placeholder='e.g. src/**/*.py, **/*.pyi'
            value={includeInput}
            onChange={(e) => setIncludeInput(e.target.value)}
          />
          <div className="flex flex-wrap gap-1.5">
            {INCLUDE_SUGGESTIONS.map((glob) => {
              const active = incList.includes(glob);
              return (
                <button
                  key={glob}
                  type="button"
                  onClick={() => toggleGlobChip(includeInput, setIncludeInput, glob)}
                  className={`text-[11px] px-2 py-0.5 rounded-full border transition-colors ${
                    active
                      ? 'bg-blue-600 border-blue-500 text-white'
                      : 'bg-slate-900/70 border-slate-700 text-slate-300 hover:border-slate-500'
                  }`}
                >
                  + {glob}
                </button>
              );
            })}
          </div>
        </div>

        {/* Exclude globs */}
        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-slate-300">
            Exclude paths <span className="text-slate-500 font-normal">(comma-separated globs, optional)</span>
          </label>
          <input
            type="text"
            className="w-full rounded-lg border border-slate-600 bg-slate-900 text-xs text-white px-3 py-2 outline-none focus:border-blue-500 placeholder:text-slate-500"
            placeholder='e.g. tests/**, vendor/**'
            value={excludeInput}
            onChange={(e) => setExcludeInput(e.target.value)}
          />
          <div className="flex flex-wrap gap-1.5">
            {EXCLUDE_SUGGESTIONS.map((glob) => {
              const active = excList.includes(glob);
              return (
                <button
                  key={glob}
                  type="button"
                  onClick={() => toggleGlobChip(excludeInput, setExcludeInput, glob)}
                  className={`text-[11px] px-2 py-0.5 rounded-full border transition-colors ${
                    active
                      ? 'bg-amber-600/80 border-amber-500 text-white'
                      : 'bg-slate-900/70 border-slate-700 text-slate-300 hover:border-slate-500'
                  }`}
                >
                  − {glob}
                </button>
              );
            })}
          </div>
        </div>

        <div className="flex justify-end gap-2 pt-2 border-t border-slate-700">
          <Button size="sm" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            size="sm"
            variant="primary"
            onClick={() => {
              onApplyTool(selectedTool.name, editEnabled, {
                ...(incList.length ? { include: incList } : {}),
                ...(excList.length ? { exclude: excList } : {}),
              });
              onClose();
            }}
          >
            Apply to tools.toml
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="p-4">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-sm font-semibold text-white">Configure tool by category</h3>
        <button
          type="button"
          onClick={onClose}
          className="text-slate-400 hover:text-white"
          aria-label="Close configure tool panel"
        >
          <Icons.Close />
        </button>
      </div>

      <div className="relative">
        <span className="pointer-events-none absolute inset-y-0 left-3 flex items-center text-slate-400">
          <Icons.Search />
        </span>
        <input
          autoFocus
          className="w-full rounded-lg border border-slate-600 bg-slate-900 text-sm text-white pl-9 pr-3 py-2 outline-none focus:border-blue-500 placeholder:text-slate-500"
          placeholder="Search category, tool, or language (e.g. Security, Python, ruff)…"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
      </div>

      {/* Category filter chips */}
      {categories.length > 1 && (
        <div className="flex flex-wrap gap-1.5 mt-3">
          <button
            type="button"
            onClick={() => setSelectedCategory(null)}
            className={`text-xs px-2.5 py-1 rounded-full border transition-colors ${
              selectedCategory === null
                ? 'bg-violet-600 border-violet-500 text-white'
                : 'bg-slate-900/60 border-slate-700 text-slate-300 hover:border-slate-500'
            }`}
          >
            All ({tools.length})
          </button>
          {categories.map((cat) => (
            <button
              key={cat}
              type="button"
              onClick={() => setSelectedCategory((prev) => (prev === cat ? null : cat))}
              className={`text-xs px-2.5 py-1 rounded-full border transition-colors ${
                selectedCategory === cat
                  ? 'bg-violet-600 border-violet-500 text-white'
                  : 'bg-slate-900/60 border-slate-700 text-slate-300 hover:border-slate-500'
              }`}
            >
              {cat}
            </button>
          ))}
        </div>
      )}

      <div className="mt-4 space-y-4">
        {grouped.map(([category, items]) => (
          <div key={category}>
            <div className="flex items-center justify-between mb-2">
              <h4 className="text-xs font-semibold text-slate-400 uppercase tracking-wide">
                {category}
              </h4>
            </div>
            <div className="flex flex-wrap gap-2">
              {items.map((tool) => {
                const key = tool.name.toLowerCase();
                const isActive = Boolean(currentEnabledMap[key]);
                const existingRule = currentRules[key];
                return (
                  <ReferenceChip
                    key={tool.id}
                    active={isActive}
                    label={`${tool.name} · ${Number(tool.multiplier).toFixed(1)}×`}
                    title={`${isActive ? 'Disable' : 'Enable'} ${tool.name} (${tool.use_case || tool.description})`}
                    onClick={() => {
                      onApplyTool(tool.name, !isActive, {
                        ...(existingRule?.include?.length ? { include: existingRule.include } : {}),
                        ...(existingRule?.exclude?.length ? { exclude: existingRule.exclude } : {}),
                      });
                    }}
                  />
                );
              })}
            </div>
          </div>
        ))}

        {grouped.length === 0 && (
          <p className="text-sm text-slate-500">
            {tools.length === 0 ? 'No tools registered in catalog yet.' : 'No tools match your search.'}
          </p>
        )}
      </div>
    </div>
  );
};

// ---- LLM helper panel (mirrors LLMHelperContent in CiRulesetEditor) ----------

const LLMHelperContent: React.FC<{
  prompt: string;
  requirements: string;
  onRequirementsChange: (next: string) => void;
  onCopy: () => void;
  onClose: () => void;
}> = ({ prompt, requirements, onRequirementsChange, onCopy, onClose }) => (
  <div className="p-4">
    <div className="flex items-center justify-between mb-3">
      <h3 className="text-sm font-semibold text-white">Ask an LLM to write it</h3>
      <button
        type="button"
        onClick={onClose}
        className="text-slate-400 hover:text-white"
        aria-label="Close Ask LLM panel"
      >
        <Icons.Close />
      </button>
    </div>
    <p className="text-xs text-slate-400 mb-2.5">
      Describe your tech stack or rules (optional), copy the prompt, then open a chat and paste it with Ctrl+V / Cmd+V.
    </p>
    <textarea
      rows={2}
      aria-label="Describe what you want to configure"
      className="w-full rounded-lg border border-slate-600 bg-slate-900 text-xs text-white px-3 py-2 outline-none focus:border-blue-500 placeholder:text-slate-500 mb-3 resize-y"
      placeholder='e.g. Python API + React TS, run ruff/eslint/bandit excluding tests/** and always scan secrets'
      value={requirements}
      onChange={(e) => onRequirementsChange(e.target.value)}
    />
    <div className="mb-3">
      <Button size="sm" variant="primary" icon={<Icons.Copy />} onClick={onCopy}>
        Copy prompt
      </Button>
    </div>
    <div className="flex flex-wrap gap-2">
      <Button size="sm" variant="outline" onClick={() => window.open('https://chatgpt.com/', '_blank', 'noopener,noreferrer')}>
        Open ChatGPT
      </Button>
      <Button size="sm" variant="outline" onClick={() => window.open('https://gemini.google.com/app', '_blank', 'noopener,noreferrer')}>
        Open Gemini
      </Button>
      <Button size="sm" variant="outline" onClick={() => window.open('https://chat.deepseek.com/', '_blank', 'noopener,noreferrer')}>
        Open DeepSeek
      </Button>
    </div>
    <details className="mt-3">
      <summary className="text-xs text-slate-400 cursor-pointer hover:text-slate-300">Preview the exact prompt</summary>
      <pre className="mt-2 text-xs bg-slate-900/70 border border-slate-700 rounded px-3 py-2 text-slate-300 overflow-auto max-h-64 whitespace-pre-wrap">
        {prompt}
      </pre>
    </details>
  </div>
);

// ---- Main component ---------------------------------------------------------

interface ToolsTomlEditorProps {
  repoId?: number;
  repoName?: string;
  tools: RepoToolView[];
  draftEnabled: Record<string, boolean>;
  draftConfigs: Record<string, RepoToolRuleConfig>;
  canEdit: boolean;
  saving: boolean;
  syncVersion: number;
  onTomlStateChange: (
    enabledMap: Record<string, boolean> | null,
    ruleConfigs?: Record<string, RepoToolRuleConfig>,
  ) => void;
}

const ToolsTomlEditor: React.FC<ToolsTomlEditorProps> = ({
  repoId,
  repoName,
  tools,
  draftEnabled,
  draftConfigs,
  canEdit,
  saving,
  syncVersion,
  onTomlStateChange,
}) => {
  const [tomlText, setTomlText] = useState<string>(() =>
    toolsToToml(tools, draftEnabled, draftConfigs, { configuredOnly: true }),
  );
  const [quickHelpOpen, setQuickHelpOpen] = useState(false);
  const [copiedPrompt, setCopiedPrompt] = useState(false);
  const [llmRequirements, setLlmRequirements] = useState('');
  const [copiedToml, setCopiedToml] = useState(false);
  const [resultTab, setResultTab] = useState<'table' | 'paths'>('table');
  const [livePage, setLivePage] = useState(1);
  const [samplePathsText, setSamplePathsText] = useState(DEFAULT_SAMPLE_PATHS);
  const [backendPreview, setBackendPreview] = useState<{
    loading: boolean;
    error?: string;
    unknownTools?: string[];
  }>({ loading: false });

  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const previewSeqRef = useRef(0);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const llmBtnRef = useRef<HTMLSpanElement>(null);
  const [popupState, setPopupState] = useState<{
    point: ScreenPoint;
    tool: RepoToolView | null;
  } | null>(null);
  const [llmPoint, setLlmPoint] = useState<ScreenPoint | null>(null);

  // Sync TOML text only when parent reloads or toggles externally.
  useEffect(() => {
    setTomlText(toolsToToml(tools, draftEnabled, draftConfigs, { configuredOnly: true }));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [syncVersion]);

  const parsed = useMemo(() => parseToolsTomlDetailed(tomlText), [tomlText]);
  const llmPrompt = useMemo(
    () => buildTomlLLMPrompt(tools, { requirements: llmRequirements, repoName, currentToml: tomlText }),
    [tools, llmRequirements, repoName, tomlText],
  );

  // Debounced backend TOML validation matching CiRulesetEditor's 350ms jq checker.
  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    let cancelled = false;
    if (!repoId || typeof previewRepositoryTools !== 'function') {
      setBackendPreview({ loading: false });
      return;
    }
    if (!parsed.valid) {
      previewSeqRef.current++;
      setBackendPreview({ loading: false });
      return;
    }

    setBackendPreview((prev) => (prev.loading || prev.error ? { loading: false } : prev));
    const seq = ++previewSeqRef.current;
    debounceRef.current = setTimeout(() => {
      if (cancelled || previewSeqRef.current !== seq) return;
      setBackendPreview((prev) => ({ ...prev, loading: true }));
      Promise.resolve(previewRepositoryTools(repoId, tomlText))
        .then((res) => {
          if (cancelled || previewSeqRef.current !== seq) return;
          if (!res) {
            setBackendPreview({ loading: false });
            return;
          }
          if (!res.valid) {
            setBackendPreview({
              loading: false,
              error: res.error || 'Invalid tools.toml syntax',
              unknownTools: res.unknown_tools,
            });
            onTomlStateChange(null);
          } else {
            setBackendPreview({
              loading: false,
              unknownTools: res.unknown_tools,
            });
          }
        })
        .catch(() => {
          if (cancelled || previewSeqRef.current !== seq) return;
          setBackendPreview({ loading: false });
        });
    }, 350);

    return () => {
      cancelled = true;
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [repoId, tomlText, parsed.valid]);

  const catalogByLower = useMemo(() => {
    const m = new Map<string, RepoToolView>();
    tools.forEach((t) => m.set(t.name.toLowerCase(), t));
    return m;
  }, [tools]);

  const unknownTools = useMemo(
    () => backendPreview.unknownTools ?? Object.keys(parsed.rules).filter((name) => !catalogByLower.has(name)),
    [backendPreview.unknownTools, parsed.rules, catalogByLower],
  );

  const samplePaths = useMemo(
    () =>
      samplePathsText
        .split(/\r?\n/)
        .map((s) => s.trim())
        .filter(Boolean),
    [samplePathsText],
  );

  // Only show tools that are actually configured in tools.toml (or enabled)
  const liveRows = useMemo(() => {
    return tools
      .map((tool) => {
        const key = tool.name.toLowerCase();
        const rule = parsed.valid ? parsed.rules[key] : undefined;
        const enabled = parsed.valid ? Boolean(parsed.enabledMap[key]) : Boolean(draftEnabled[key]);
        const inToml = Boolean(rule) || enabled;
        const pathEval = evaluateToolForPaths(enabled, rule, samplePaths);
        return {
          tool,
          rule,
          enabled,
          inToml,
          pathEval,
        };
      })
      .filter((r) => r.inToml);
  }, [tools, parsed, draftEnabled, samplePaths]);

  const activeRows = liveRows.filter((r) => r.enabled);
  const activeMultiplier = activeRows.reduce((sum, r) => sum + Number(r.tool.multiplier), 0);
  const totalLivePages = Math.ceil(liveRows.length / LIVE_TABLE_PAGE_SIZE);
  const activeLivePage = Math.min(livePage, Math.max(1, totalLivePages));
  const pagedLiveRows = liveRows.slice(
    (activeLivePage - 1) * LIVE_TABLE_PAGE_SIZE,
    activeLivePage * LIVE_TABLE_PAGE_SIZE,
  );

  const updateText = (next: string) => {
    setTomlText(next);
    const res = parseToolsTomlDetailed(next);
    if (res.valid) {
      onTomlStateChange(res.enabledMap, res.ruleConfigs);
    } else {
      onTomlStateChange(null);
    }
  };

  const openPopup = (tool: RepoToolView | null = null, anchorEl?: HTMLElement | null) => {
    if (!canEdit || saving) return;
    const viewportW = typeof window !== 'undefined' ? window.innerWidth : 1200;
    const viewportH = typeof window !== 'undefined' ? window.innerHeight : 800;
    if (anchorEl) {
      const rect = anchorEl.getBoundingClientRect();
      setPopupState({
        point: {
          x: Math.max(12, Math.min(rect.left, viewportW - 12)),
          y: Math.max(12, Math.min(rect.bottom + 6, viewportH - 12)),
        },
        tool,
      });
      return;
    }
    const el = textareaRef.current;
    if (el) {
      el.focus();
      const elRect = el.getBoundingClientRect();
      const pt = getCaretScreenPoint(el);
      setPopupState({
        point: {
          x: Math.max(12, Math.min(pt.x, viewportW - 12)),
          y: Math.max(Math.max(12, elRect.top + 24), Math.min(pt.y, Math.min(elRect.bottom, viewportH - 12))),
        },
        tool,
      });
    } else {
      setPopupState({ point: { x: 100, y: 200 }, tool });
    }
  };

  const openLlmHelper = () => {
    const btn = llmBtnRef.current;
    if (btn) {
      const rect = btn.getBoundingClientRect();
      setLlmPoint({ x: rect.left, y: rect.bottom + 6 });
    }
  };

  const copyToClipboard = (text: string, onCopied: () => void) => {
    const writeText = navigator.clipboard?.writeText;
    if (typeof writeText !== 'function') return;
    Promise.resolve(writeText.call(navigator.clipboard, text)).then(
      (): void => onCopied(),
      (): void => undefined,
    );
  };

  const copyPrompt = () => {
    copyToClipboard(llmPrompt, () => {
      setCopiedPrompt(true);
      setTimeout(() => setCopiedPrompt(false), 2000);
    });
  };

  const copyToml = () => {
    copyToClipboard(tomlText, () => {
      setCopiedToml(true);
      setTimeout(() => setCopiedToml(false), 2000);
    });
  };

  const handleApplyToolFromPopup = (
    toolName: string,
    enabled: boolean,
    config: RepoToolRuleConfig,
  ) => {
    const next = upsertToolSectionInToml(tomlText, toolName, enabled, config);
    updateText(next);
  };

  const applyPreset = (
    preset: 'security' | 'secrets' | 'linters' | 'exclude_tests' | 'all_on' | 'clear',
  ) => {
    if (!canEdit || saving) return;
    if (preset === 'clear') {
      const map = Object.fromEntries(tools.map((t) => [t.name.toLowerCase(), false]));
      updateText(toolsToToml(tools, map, {}, { configuredOnly: true }));
      return;
    }
    if (preset === 'all_on') {
      const map = Object.fromEntries(tools.map((t) => [t.name.toLowerCase(), true]));
      updateText(toolsToToml(tools, map, parsed.valid ? parsed.ruleConfigs : draftConfigs));
      return;
    }
    if (preset === 'exclude_tests') {
      if (liveRows.length === 0) return;
      let next = tomlText;
      liveRows.forEach(({ tool: t }) => {
        const key = t.name.toLowerCase();
        const existing = parsed.valid ? parsed.rules[key] : undefined;
        const exc = existing?.exclude ? [...existing.exclude] : [];
        if (!exc.includes('tests/**')) exc.push('tests/**');
        next = upsertToolSectionInToml(next, t.name, parsed.valid ? Boolean(parsed.enabledMap[key]) : true, {
          ...(existing?.include ? { include: existing.include } : {}),
          exclude: exc,
        });
      });
      updateText(next);
      return;
    }
    const map: Record<string, boolean> = {};
    tools.forEach((t) => {
      const text = `${t.name} ${t.category || ''} ${t.use_case || ''}`.toLowerCase();
      if (preset === 'security') {
        map[t.name.toLowerCase()] = /secret|security|vuln|cve|sast|supply|terraform|container/.test(text);
      } else if (preset === 'secrets') {
        map[t.name.toLowerCase()] = /gitleaks|trufflehog|detect-secrets|secret/.test(text);
      } else {
        map[t.name.toLowerCase()] = /ruff|eslint|golangci|hadolint|actionlint|spectral|openapi/.test(text);
      }
    });
    updateText(toolsToToml(tools, map, parsed.valid ? parsed.ruleConfigs : draftConfigs, { configuredOnly: true }));
  };

  const effectiveError = !parsed.valid ? parsed.error : backendPreview.error;
  const isInvalid = Boolean(effectiveError);

  return (
    <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <div className={quickHelpOpen ? 'lg:col-span-2 space-y-5' : 'lg:col-span-3 space-y-5'}>
        {/* Editor block — matches CiRulesetEditor visual structure */}
        <div>
          <label className="block text-sm font-medium text-slate-300 mb-1.5">
            SAST tools configuration
          </label>
          <div className="flex items-center gap-2 bg-slate-900/80 border border-slate-700 border-b-0 rounded-t-lg px-3 py-2 flex-wrap">
            <button
              type="button"
              disabled={!canEdit || saving}
              onClick={(e) => openPopup(null, e.currentTarget)}
              className="inline-flex items-center rounded-md border border-slate-600 bg-slate-700 hover:bg-slate-600 disabled:opacity-50 disabled:cursor-not-allowed text-slate-100 overflow-hidden transition-colors"
            >
              <span className="flex items-center gap-1.5 px-3 py-1.5 text-sm">
                <Icons.List /> Configure tool
              </span>
              <span className="px-2 py-1.5 text-[10px] text-slate-400 bg-slate-800/70 border-l border-slate-600 tabular-nums">
                Ctrl+Space
              </span>
            </button>

            <span ref={llmBtnRef} className="inline-flex">
              <Button size="sm" variant="secondary" icon={<Icons.AI />} onClick={openLlmHelper}>
                {copiedPrompt ? 'Copied prompt!' : 'Ask LLM'}
              </Button>
            </span>

            <Button
              size="sm"
              variant={quickHelpOpen ? 'primary' : 'secondary'}
              icon={<Icons.Info />}
              onClick={() => setQuickHelpOpen((v) => !v)}
            >
              TOML quick help
            </Button>

            <span className="ml-auto flex items-center gap-2">
              <Button
                size="sm"
                variant="ghost"
                icon={<Icons.Copy />}
                onClick={copyToml}
                title="Copy as .lrc/policy/tools.toml"
              >
                {copiedToml ? 'Copied!' : 'Copy tools.toml'}
              </Button>
            </span>
          </div>

          <textarea
            ref={textareaRef}
            readOnly={!canEdit || saving}
            aria-label="tools.toml editor"
            aria-invalid={isInvalid}
            className={`w-full rounded-b-lg border bg-slate-950/90 text-slate-100 leading-relaxed caret-violet-400 outline-none px-4 py-3 font-mono text-sm resize-y ${
              !canEdit ? 'opacity-80 cursor-not-allowed' : ''
            } ${
              isInvalid
                ? 'border-red-500 focus:border-red-400 focus:ring-2 focus:ring-red-400/50'
                : 'border-slate-700 focus:border-blue-500 focus:ring-1 focus:ring-blue-500/40'
            }`}
            style={{ minHeight: '11rem' }}
            rows={10}
            value={tomlText}
            onChange={(e) => updateText(e.target.value)}
            onKeyDown={(e) => {
              if (e.ctrlKey && e.code === 'Space') {
                e.preventDefault();
                openPopup(null);
              }
            }}
            spellCheck={false}
          />

          <p className={`text-xs mt-1.5 ${isInvalid ? 'text-red-400' : 'text-slate-500'}`}>
            {isInvalid
              ? `Invalid TOML — ${effectiveError}`
              : 'Configure enabled tools and optional include/exclude globs. Live table below updates as you type.'}
          </p>

          {canEdit && tools.length > 0 && (
            <div className="flex flex-wrap gap-2 mt-3">
              <button
                type="button"
                onClick={() => applyPreset('security')}
                className="text-xs bg-slate-700/70 hover:bg-slate-600 text-slate-200 px-2.5 py-1.5 rounded-md border border-slate-600"
              >
                Security scanners
              </button>
              <button
                type="button"
                onClick={() => applyPreset('secrets')}
                className="text-xs bg-slate-700/70 hover:bg-slate-600 text-slate-200 px-2.5 py-1.5 rounded-md border border-slate-600"
              >
                Secrets only
              </button>
              <button
                type="button"
                onClick={() => applyPreset('linters')}
                className="text-xs bg-slate-700/70 hover:bg-slate-600 text-slate-200 px-2.5 py-1.5 rounded-md border border-slate-600"
              >
                Linters &amp; Docker/CI
              </button>
              <button
                type="button"
                onClick={() => applyPreset('exclude_tests')}
                className="text-xs bg-slate-700/70 hover:bg-slate-600 text-slate-200 px-2.5 py-1.5 rounded-md border border-slate-600"
              >
                Exclude tests/**
              </button>
              <button
                type="button"
                onClick={() => applyPreset('all_on')}
                className="text-xs bg-slate-700/70 hover:bg-slate-600 text-slate-200 px-2.5 py-1.5 rounded-md border border-slate-600"
              >
                Enable all
              </button>
              <button
                type="button"
                onClick={() => applyPreset('clear')}
                className="text-xs bg-slate-700/70 hover:bg-slate-600 text-slate-200 px-2.5 py-1.5 rounded-md border border-slate-600"
              >
                Clear
              </button>
            </div>
          )}
        </div>

        {/* Bottom Live results — mirrors CiRulesetEditor's tabbed bottom panel with authentic Live Table */}
        <div className="pt-4 border-t border-slate-800 space-y-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex items-center gap-4 border-b border-slate-700">
              <button
                type="button"
                onClick={() => setResultTab('table')}
                className={`flex items-center gap-2 pb-2 text-sm font-medium border-b-2 -mb-px transition-colors ${
                  resultTab === 'table'
                    ? 'border-blue-500 text-white'
                    : 'border-transparent text-slate-400 hover:text-slate-200'
                }`}
              >
                {backendPreview.loading ? (
                  <Spinner size="sm" />
                ) : (
                  <span
                    className={`w-2 h-2 rounded-full ${isInvalid ? 'bg-amber-500' : 'bg-green-500'}`}
                    title={isInvalid ? effectiveError : 'Valid tools.toml'}
                  />
                )}
                Live table ({activeRows.length}/{tools.length})
              </button>
              <button
                type="button"
                onClick={() => setResultTab('paths')}
                className={`pb-2 text-sm font-medium border-b-2 -mb-px transition-colors ${
                  resultTab === 'paths'
                    ? 'border-blue-500 text-white'
                    : 'border-transparent text-slate-400 hover:text-slate-200'
                }`}
              >
                Test file paths
              </button>
            </div>

            <div className="flex items-center gap-3">
              {backendPreview.loading && <Spinner size="sm" />}
              <span className="text-xs text-slate-400 tabular-nums">
                {activeMultiplier.toFixed(1)}× multiplier
              </span>
              <span
                className="text-xs font-semibold tracking-wide px-2.5 py-0.5 rounded-full border"
                style={
                  isInvalid
                    ? { color: '#f87171', borderColor: 'rgba(239,68,68,0.5)', backgroundColor: 'rgba(239,68,68,0.12)' }
                    : { color: '#4ade80', borderColor: 'rgba(34,197,94,0.5)', backgroundColor: 'rgba(34,197,94,0.12)' }
                }
              >
                {isInvalid ? 'ERROR' : 'VALID'}
              </span>
            </div>
          </div>

          {!isInvalid && unknownTools.length > 0 && (
            <p className="text-xs text-amber-300">
              Unknown tool section{unknownTools.length > 1 ? 's' : ''}: {unknownTools.join(', ')} (not in catalog — ignored).
            </p>
          )}

          {isInvalid ? (
            <div className="flex items-center gap-2 rounded-lg border border-red-500/40 bg-red-950/30 px-3 py-2.5">
              <Badge variant="warning">ERROR</Badge>
              <span className="text-sm text-amber-300">{effectiveError}</span>
            </div>
          ) : resultTab === 'table' ? (
            liveRows.length === 0 ? (
              <div className="rounded-xl border border-slate-700/80 bg-slate-900/40 px-4 py-6 text-center text-sm text-slate-400">
                No tools configured yet. Click <span className="text-slate-200 font-medium">Configure tool</span> (Ctrl+Space) or pick a preset above.
              </div>
            ) : (
              <div className="border border-slate-700/80 rounded-xl bg-slate-900/40 overflow-hidden">
                <div className="overflow-x-auto">
                  <table className="w-full text-left border-collapse">
                    <thead className="bg-slate-800/95 border-b border-slate-700">
                      <tr>
                        <th className="px-3.5 py-2.5 text-xs font-semibold uppercase tracking-wider text-slate-400">Tool</th>
                        <th className="px-3.5 py-2.5 text-xs font-semibold uppercase tracking-wider text-slate-400">Category</th>
                        <th className="px-3.5 py-2.5 text-xs font-semibold uppercase tracking-wider text-slate-400">Include</th>
                        <th className="px-3.5 py-2.5 text-xs font-semibold uppercase tracking-wider text-slate-400">Exclude</th>
                        <th className="px-3.5 py-2.5 text-xs font-semibold uppercase tracking-wider text-slate-400 text-right">Status</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-slate-800 text-sm">
                      {pagedLiveRows.map(({ tool, rule, enabled }) => (
                        <tr
                          key={tool.id}
                          onClick={(e) => canEdit && openPopup(tool, e.currentTarget)}
                          className={`transition-colors ${
                            canEdit ? 'cursor-pointer hover:bg-slate-800/50' : ''
                          } ${!enabled ? 'opacity-60' : ''}`}
                          title={canEdit ? `Click to configure ${tool.name} include/exclude globs` : undefined}
                        >
                          <td className="px-3.5 py-2.5 font-medium text-white">
                            {tool.name}
                          </td>
                          <td className="px-3.5 py-2.5 text-xs text-slate-300">
                            {tool.category || '—'}
                          </td>
                          <td className="px-3.5 py-2.5 text-xs text-slate-300">
                            {rule?.include?.length ? (
                              <span className="inline-flex flex-wrap gap-1">
                                {rule.include.map((g) => (
                                  <span key={g} className="rounded bg-blue-500/15 border border-blue-500/30 px-1.5 py-0.5 text-blue-200">
                                    {g}
                                  </span>
                                ))}
                              </span>
                            ) : (
                              <span className="text-slate-500">All files</span>
                            )}
                          </td>
                          <td className="px-3.5 py-2.5 text-xs text-slate-300">
                            {rule?.exclude?.length ? (
                              <span className="inline-flex flex-wrap gap-1">
                                {rule.exclude.map((g) => (
                                  <span key={g} className="rounded bg-amber-500/15 border border-amber-500/30 px-1.5 py-0.5 text-amber-200">
                                    {g}
                                  </span>
                                ))}
                              </span>
                            ) : (
                              <span className="text-slate-500">—</span>
                            )}
                          </td>
                          <td className="px-3.5 py-2.5 text-right">
                            {canEdit ? (
                              <button
                                type="button"
                                role="switch"
                                aria-checked={enabled}
                                aria-label={`Live toggle ${tool.name}`}
                                onClick={(e) => {
                                  e.stopPropagation();
                                  handleApplyToolFromPopup(tool.name, !enabled, {
                                    ...(rule?.include?.length ? { include: rule.include } : {}),
                                    ...(rule?.exclude?.length ? { exclude: rule.exclude } : {}),
                                  });
                                }}
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
                      ))}
                    </tbody>
                  </table>
                </div>
                {totalLivePages > 1 && (
                  <div className="flex items-center justify-between border-t border-slate-700 bg-slate-900/60 px-4 py-3">
                    <p className="text-xs text-slate-400">
                      Showing <span className="font-medium text-white">{(activeLivePage - 1) * LIVE_TABLE_PAGE_SIZE + 1}</span>–
                      <span className="font-medium text-white">{Math.min(activeLivePage * LIVE_TABLE_PAGE_SIZE, liveRows.length)}</span>
                      {' '}of <span className="font-medium text-white">{liveRows.length}</span> tools
                    </p>
                    <nav className="isolate inline-flex -space-x-px rounded-md shadow-sm" aria-label="Live table pagination">
                      <button
                        type="button"
                        onClick={() => setLivePage((p) => Math.max(p - 1, 1))}
                        disabled={activeLivePage === 1}
                        className="relative inline-flex items-center rounded-l-md px-2 py-1.5 text-slate-400 ring-1 ring-inset ring-slate-700 hover:bg-slate-800 disabled:opacity-30 disabled:cursor-not-allowed"
                        aria-label="Previous page"
                      >
                        <svg className="h-4 w-4" viewBox="0 0 20 20" fill="currentColor">
                          <path fillRule="evenodd" d="M12.79 5.23a.75.75 0 01-.02 1.06L8.832 10l3.938 3.71a.75.75 0 11-1.04 1.08l-4.5-4.25a.75.75 0 010-1.08l4.5-4.25a.75.75 0 011.06.02z" clipRule="evenodd" />
                        </svg>
                      </button>
                      {Array.from({ length: totalLivePages }, (_, i) => i + 1).map((pg) => (
                        <button
                          key={pg}
                          type="button"
                          onClick={() => setLivePage(pg)}
                          className={`relative inline-flex items-center px-3 py-1.5 text-xs font-semibold ${
                            pg === activeLivePage
                              ? 'z-10 bg-violet-600 text-white'
                              : 'text-slate-300 ring-1 ring-inset ring-slate-700 hover:bg-slate-800'
                          }`}
                        >
                          {pg}
                        </button>
                      ))}
                      <button
                        type="button"
                        onClick={() => setLivePage((p) => Math.min(p + 1, totalLivePages))}
                        disabled={activeLivePage === totalLivePages}
                        className="relative inline-flex items-center rounded-r-md px-2 py-1.5 text-slate-400 ring-1 ring-inset ring-slate-700 hover:bg-slate-800 disabled:opacity-30 disabled:cursor-not-allowed"
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
            )
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="block text-xs font-medium text-slate-400 mb-1.5">
                  Sample changed PR file paths (one per line)
                </label>
                <textarea
                  rows={6}
                  value={samplePathsText}
                  onChange={(e) => setSamplePathsText(e.target.value)}
                  className="w-full rounded-lg border border-slate-700 bg-slate-900/70 px-3 py-2 font-mono text-xs text-slate-200 outline-none focus:border-blue-500"
                  spellCheck={false}
                />
              </div>
              <div className="border border-slate-700/80 rounded-xl bg-slate-900/40 max-h-48 overflow-y-auto">
                <table className="w-full text-left border-collapse text-xs">
                  <thead className="sticky top-0 bg-slate-800/95 border-b border-slate-700">
                    <tr>
                      <th className="px-3 py-2 font-semibold uppercase tracking-wider text-slate-400">Tool</th>
                      <th className="px-3 py-2 font-semibold uppercase tracking-wider text-slate-400">Outcome</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-800">
                    {liveRows.map(({ tool, pathEval }) => (
                      <tr key={tool.id}>
                        <td className="px-3 py-1.5 text-white font-medium">{tool.name}</td>
                        <td className="px-3 py-1.5">
                          {pathEval.willRun ? (
                            <span className="text-emerald-400 font-medium">Will run ({pathEval.reason})</span>
                          ) : (
                            <span className="text-slate-500">Skipped · {pathEval.reason}</span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Sticky Quick Help Sidebar (mirrors CiRulesetEditor) */}
      {quickHelpOpen && (
        <div className="lg:col-span-1">
          <div className="sticky top-4 bg-slate-800 border border-slate-700 rounded-lg max-h-[85vh] overflow-y-auto">
            <div className="flex justify-end p-2">
              <button
                type="button"
                onClick={() => setQuickHelpOpen(false)}
                className="text-slate-400 hover:text-white"
                aria-label="Close TOML quick help"
              >
                <Icons.Close />
              </button>
            </div>
            <div className="-mt-10">
              <ToolsTomlQuickHelp />
            </div>
          </div>
        </div>
      )}

      {/* Popup UI for Category/Tool Search & Include/Exclude Configuration */}
      <ToolsFloatingPanel
        open={!!popupState}
        point={popupState?.point || null}
        onClose={() => setPopupState(null)}
        widthPx={540}
      >
        <ConfigureToolPopup
          tools={tools}
          initialTool={popupState?.tool || null}
          currentRules={parsed.valid ? parsed.rules : {}}
          currentEnabledMap={parsed.valid ? parsed.enabledMap : draftEnabled}
          onApplyTool={handleApplyToolFromPopup}
          onClose={() => setPopupState(null)}
        />
      </ToolsFloatingPanel>

      {/* Ask LLM Popup */}
      <ToolsFloatingPanel open={!!llmPoint} point={llmPoint} onClose={() => setLlmPoint(null)} widthPx={440}>
        <LLMHelperContent
          prompt={llmPrompt}
          requirements={llmRequirements}
          onRequirementsChange={setLlmRequirements}
          onCopy={copyPrompt}
          onClose={() => setLlmPoint(null)}
        />
      </ToolsFloatingPanel>
    </div>
  );
};

export default ToolsTomlEditor;
