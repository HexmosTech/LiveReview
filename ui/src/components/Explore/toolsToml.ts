import { RepoToolRuleConfig, RepoToolView } from '../../api/repositories';

export interface ParsedToolRule {
  enabled?: boolean;
  include?: string[];
  exclude?: string[];
  category?: string;
}

export interface ToolsTomlParseResult {
  valid: boolean;
  error?: string;
  errorLine?: number;
  rules: Record<string, ParsedToolRule>;
  /** Explicit enabled states resolved against TOML semantics:
   *  - If `enabled` is set, use it.
   *  - If `[tool]` block omits `enabled` (e.g. only `include`/`exclude`), treat as `true`
   *    (matching internal/jobqueue/tool_dispatcher.go).
   */
  enabledMap: Record<string, boolean>;
  /** Extracted per-tool include/exclude configs ready to persist in repositories.settings->'tools'. */
  ruleConfigs: Record<string, RepoToolRuleConfig>;
  hasFileFilters: boolean;
}

/** Strip unquoted `# ...` comments from a single line. */
function stripTomlComment(line: string): string {
  let quote: '"' | "'" | null = null;
  for (let i = 0; i < line.length; i++) {
    const ch = line[i];
    if (quote) {
      if (ch === '\\' && quote === '"' && i + 1 < line.length) {
        i++;
        continue;
      }
      if (ch === quote) quote = null;
    } else {
      if (ch === '"' || ch === "'") quote = ch;
      else if (ch === '#') return line.slice(0, i);
    }
  }
  return line;
}

/** Count unescaped quote balance and bracket depth outside quotes. */
function scanArrayState(text: string): { depth: number; openQuote: '"' | "'" | null } {
  let depth = 0;
  let openQuote: '"' | "'" | null = null;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    if (openQuote) {
      if (ch === '\\' && openQuote === '"' && i + 1 < text.length) {
        i++;
        continue;
      }
      if (ch === openQuote) openQuote = null;
    } else {
      if (ch === '"' || ch === "'") openQuote = ch;
      else if (ch === '[') depth++;
      else if (ch === ']') depth--;
    }
  }
  return { depth, openQuote };
}

const BASIC_ESCAPES: Record<string, string> = {
  '"': '"',
  '\\': '\\',
  n: '\n',
  r: '\r',
  t: '\t',
  b: '\b',
  f: '\f',
};

/** Parse a TOML array of strings like `["*.py", 'tests/**']`. Returns null on syntax error. */
function parseStringArray(raw: string): string[] | null {
  const trimmed = raw.trim();
  if (!trimmed.startsWith('[') || !trimmed.endsWith(']')) return null;
  const inner = trimmed.slice(1, -1).trim();
  if (!inner) return [];

  const items: string[] = [];
  let i = 0;
  while (i < inner.length) {
    while (i < inner.length && /[\s,]/.test(inner[i])) i++;
    if (i >= inner.length) break;

    const quote = inner[i];
    if (quote !== '"' && quote !== "'") return null;
    i++;
    let value = '';
    let closed = false;
    while (i < inner.length) {
      const ch = inner[i];
      if (ch === '\\' && quote === '"') {
        if (i + 1 >= inner.length) return null;
        const mapped = BASIC_ESCAPES[inner[i + 1]];
        if (mapped === undefined) return null;
        value += mapped;
        i += 2;
        continue;
      }
      if (ch === quote) {
        closed = true;
        i++;
        break;
      }
      value += ch;
      i++;
    }
    if (!closed) return null;
    items.push(value);

    while (i < inner.length && /\s/.test(inner[i])) i++;
    if (i < inner.length) {
      if (inner[i] !== ',') return null;
      i++;
    }
  }

  return items;
}

/** Parse a quoted string literal `"..."` or `'...'`. Returns null if invalid. */
function parseQuotedString(raw: string): string | null {
  const trimmed = raw.trim();
  if (trimmed.length < 2) return null;
  const quote = trimmed[0];
  if ((quote !== '"' && quote !== "'") || trimmed[trimmed.length - 1] !== quote) return null;
  let out = '';
  for (let i = 1; i < trimmed.length - 1; i++) {
    const ch = trimmed[i];
    if (ch === '\\' && quote === '"') {
      if (i + 1 >= trimmed.length - 1) return null;
      const mapped = BASIC_ESCAPES[trimmed[i + 1]];
      if (mapped === undefined) return null;
      out += mapped;
      i++;
      continue;
    }
    if (ch === quote) return null;
    out += ch;
  }
  return out;
}

/** Normalize section name like `ruff`, `tools.ruff`, `"detect-secrets"`, `tools."detect-secrets"`. */
function parseSectionHeader(rawHeader: string): { isToolsRoot?: boolean; toolName?: string } | null {
  const inner = rawHeader.trim();
  if (!inner) return null;

  const stripPart = (p: string): string | null => {
    const t = p.trim();
    if (!t) return null;
    if ((t.startsWith('"') && t.endsWith('"')) || (t.startsWith("'") && t.endsWith("'"))) {
      return parseQuotedString(t);
    }
    if (/^[a-zA-Z0-9_-]+$/.test(t)) return t;
    return null;
  };

  const parts: string[] = [];
  let cur = '';
  let q: '"' | "'" | null = null;
  for (let i = 0; i < inner.length; i++) {
    const ch = inner[i];
    if (q) {
      cur += ch;
      if (ch === q) q = null;
    } else if (ch === '"' || ch === "'") {
      cur += ch;
      q = ch;
    } else if (ch === '.') {
      parts.push(cur);
      cur = '';
    } else {
      cur += ch;
    }
  }
  if (q) return null;
  parts.push(cur);

  if (parts.length === 1) {
    const name = stripPart(parts[0]);
    if (!name) return null;
    if (name.toLowerCase() === 'tools') return { isToolsRoot: true };
    return { toolName: name.toLowerCase() };
  }
  if (parts.length === 2) {
    const root = stripPart(parts[0]);
    const sub = stripPart(parts[1]);
    if (!root || !sub || root.toLowerCase() !== 'tools') return null;
    return { toolName: sub.toLowerCase() };
  }
  return null;
}

/** Parse `.lrc/policy/tools.toml` following internal/lrcconfig/lrcconfig.go rules. */
export function parseToolsTomlDetailed(text: string): ToolsTomlParseResult {
  const lines = text.split(/\r?\n/);
  const rules: Record<string, ParsedToolRule> = {};
  let currentSection: { isToolsRoot?: boolean; toolName?: string } | null = null;
  let hasFileFilters = false;

  const fail = (line: number, message: string): ToolsTomlParseResult => ({
    valid: false,
    error: `Line ${line}: ${message}`,
    errorLine: line,
    rules: {},
    enabledMap: {},
    ruleConfigs: {},
    hasFileFilters: false,
  });

  for (let i = 0; i < lines.length; i++) {
    const lineNum = i + 1;
    const cleaned = stripTomlComment(lines[i]).trim();
    if (!cleaned) continue;

    if (cleaned.startsWith('[')) {
      if (!cleaned.endsWith(']') || cleaned.startsWith('[[')) {
        return fail(lineNum, 'Malformed section header — expected [tool-name]');
      }
      const parsedHeader = parseSectionHeader(cleaned.slice(1, -1));
      if (!parsedHeader) {
        return fail(lineNum, `Invalid section name ${cleaned}`);
      }
      currentSection = parsedHeader;
      if (parsedHeader.toolName && !(parsedHeader.toolName in rules)) {
        rules[parsedHeader.toolName] = {};
      }
      continue;
    }

    const eqIdx = cleaned.indexOf('=');
    if (eqIdx === -1) {
      return fail(lineNum, 'Expected `[tool-name]` section or `key = value` assignment');
    }

    const rawKey = cleaned.slice(0, eqIdx).trim();
    const key = parseQuotedString(rawKey) ?? (/^[a-zA-Z0-9_-]+$/.test(rawKey) ? rawKey : null);
    if (!key) {
      return fail(lineNum, `Invalid key "${rawKey}"`);
    }

    let rawValue = cleaned.slice(eqIdx + 1).trim();
    if (!rawValue) {
      return fail(lineNum, `Missing value for "${key}"`);
    }

    if (rawValue.startsWith('[')) {
      let state = scanArrayState(rawValue);
      while ((state.depth > 0 || state.openQuote !== null) && i + 1 < lines.length) {
        i++;
        const nextLine = stripTomlComment(lines[i]).trim();
        if (nextLine) rawValue += ' ' + nextLine;
        state = scanArrayState(rawValue);
      }
      if (state.depth !== 0 || state.openQuote !== null) {
        return fail(lineNum, `Unclosed array for "${key}"`);
      }
    }

    if (currentSection === null || currentSection.isToolsRoot) {
      if (rawValue !== 'true' && rawValue !== 'false') {
        return fail(
          lineNum,
          currentSection === null
            ? `Key "${key}" appears before any [tool-name] section`
            : `Expected true or false for "${key}" under [tools]`,
        );
      }
      const toolKey = key.toLowerCase();
      rules[toolKey] = { ...(rules[toolKey] || {}), enabled: rawValue === 'true' };
      continue;
    }

    const toolName = currentSection.toolName!;
    const rule = (rules[toolName] = rules[toolName] || {});
    const lowerKey = key.toLowerCase();

    if (lowerKey === 'enabled') {
      if (rawValue !== 'true' && rawValue !== 'false') {
        return fail(lineNum, '`enabled` must be `true` or `false`');
      }
      rule.enabled = rawValue === 'true';
    } else if (lowerKey === 'include' || lowerKey === 'exclude') {
      const arr = parseStringArray(rawValue);
      if (!arr) {
        return fail(lineNum, `\`${lowerKey}\` must be an array of strings, e.g. ["**/*.py"]`);
      }
      rule[lowerKey] = arr;
      if (arr.length > 0) hasFileFilters = true;
    } else if (lowerKey === 'category') {
      const cat = parseQuotedString(rawValue);
      if (cat === null) {
        return fail(lineNum, '`category` must be a quoted string, e.g. "security"');
      }
      rule.category = cat;
    } else {
      return fail(lineNum, `Unknown key "${key}" (supported keys: enabled, include, exclude, category)`);
    }
  }

  const enabledMap: Record<string, boolean> = {};
  const ruleConfigs: Record<string, RepoToolRuleConfig> = {};
  Object.entries(rules).forEach(([name, rule]) => {
    enabledMap[name] = typeof rule.enabled === 'boolean' ? rule.enabled : true;
    if ((rule.include && rule.include.length > 0) || (rule.exclude && rule.exclude.length > 0)) {
      ruleConfigs[name] = {
        ...(rule.include && rule.include.length > 0 ? { include: rule.include } : {}),
        ...(rule.exclude && rule.exclude.length > 0 ? { exclude: rule.exclude } : {}),
      };
    }
  });

  return {
    valid: true,
    rules,
    enabledMap,
    ruleConfigs,
    hasFileFilters,
  };
}

function formatTomlStringArray(items: string[]): string {
  return '[' + items.map((s) => JSON.stringify(s)).join(', ') + ']';
}

export function formatToolBlock(
  toolName: string,
  enabled: boolean,
  config?: RepoToolRuleConfig,
): string {
  const lines = [`[${toolName}]`, `enabled = ${enabled}`];
  if (config?.include?.length) lines.push(`include = ${formatTomlStringArray(config.include)}`);
  if (config?.exclude?.length) lines.push(`exclude = ${formatTomlStringArray(config.exclude)}`);
  return lines.join('\n');
}

/** Generate TOML text from a tool catalog, active states, and optional include/exclude configs. */
export function toolsToToml(
  tools: RepoToolView[],
  enabledMap: Record<string, boolean>,
  configsMap?: Record<string, RepoToolRuleConfig>,
  options?: { configuredOnly?: boolean },
): string {
  if (tools.length === 0) {
    return '# No tools registered yet.\n';
  }
  const rows = options?.configuredOnly
    ? tools.filter((t) => {
        const key = t.name.toLowerCase();
        const active = Boolean(enabledMap[key]);
        const cfg = configsMap ? configsMap[key] : t.config_json;
        return active || Boolean(cfg?.include?.length || cfg?.exclude?.length);
      })
    : tools;

  if (options?.configuredOnly && rows.length === 0) {
    return '# No tools enabled yet. Use "Configure tool" (Ctrl+Space) to add one.\n';
  }

  return (
    rows
      .map((t) => {
        const key = t.name.toLowerCase();
        const active = Boolean(enabledMap[key]);
        const cfg = configsMap ? configsMap[key] : t.config_json;
        return formatToolBlock(t.name, active, cfg);
      })
      .join('\n\n') + '\n'
  );
}

/** Insert or replace a `[toolName]` section cleanly inside existing TOML text. */
export function upsertToolSectionInToml(
  currentText: string,
  toolName: string,
  enabled: boolean,
  config?: RepoToolRuleConfig,
): string {
  const block = formatToolBlock(toolName, enabled, config);
  const normalized = currentText.replace(/\r\n/g, '\n');
  const lines = normalized.split('\n');
  const targetLower = toolName.toLowerCase();

  let startIdx = -1;
  let endIdx = lines.length;
  for (let i = 0; i < lines.length; i++) {
    const trimmed = stripTomlComment(lines[i]).trim();
    if (trimmed.startsWith('[') && trimmed.endsWith(']')) {
      const parsed = parseSectionHeader(trimmed.slice(1, -1));
      if (!parsed) continue;
      if (startIdx !== -1) {
        endIdx = i;
        break;
      }
      if (parsed.toolName === targetLower) {
        startIdx = i;
      }
    }
  }

  if (startIdx !== -1) {
    while (endIdx > startIdx + 1 && lines[endIdx - 1].trim() === '') {
      endIdx--;
    }
    const nextLines = [...lines.slice(0, startIdx), block, ...lines.slice(endIdx)];
    return nextLines.join('\n').trimEnd() + '\n';
  }

  // Strip empty placeholder banner before appending the first real section
  const nonPlaceholder = lines.filter((l) => !l.trim().startsWith('# No tools ')).join('\n').trimEnd();
  return (nonPlaceholder ? nonPlaceholder + '\n\n' + block : block) + '\n';
}

/** Lightweight gitignore-like glob matcher for client-side live "Test file paths" preview. */
export function matchesGlobPattern(pattern: string, filePath: string): boolean {
  const cleanPattern = pattern
    .trim()
    .replace(/^\/+/, '')
    .replace(/(\*\*\/)+/g, '**/')
    .replace(/\*{3,}/g, '**');
  const cleanPath = filePath.trim().replace(/^\/+/, '');
  if (!cleanPattern || !cleanPath || cleanPattern.length > 256 || cleanPath.length > 1024) return false;

  // Escape regex specials except * and ?, then translate ** / * / ?
  let regexStr = '';
  for (let i = 0; i < cleanPattern.length; i++) {
    const ch = cleanPattern[i];
    if (ch === '*') {
      if (cleanPattern[i + 1] === '*') {
        if (cleanPattern[i + 2] === '/') {
          regexStr += '(?:.+/)?';
          i += 2;
        } else {
          regexStr += '.*';
          i += 1;
        }
      } else {
        regexStr += '[^/]*';
      }
    } else if (ch === '?') {
      regexStr += '[^/]';
    } else if (/[.+^${}()|[\]\\]/.test(ch)) {
      regexStr += '\\' + ch;
    } else {
      regexStr += ch;
    }
  }

  const re = new RegExp(`^${regexStr}$`, 'i');
  if (re.test(cleanPath)) return true;
  // Basename match when pattern has no slash (like `*.py` matching `src/app.py`)
  if (!cleanPattern.includes('/')) {
    const base = cleanPath.split('/').pop() || cleanPath;
    if (re.test(base)) return true;
  }
  return false;
}

/** Evaluate whether a tool with `include` / `exclude` rules matches at least one changed file path. */
export function evaluateToolForPaths(
  enabled: boolean,
  rule: ParsedToolRule | undefined,
  paths: string[],
): { willRun: boolean; matchedFiles: string[]; reason: string } {
  if (!enabled) {
    return { willRun: false, matchedFiles: [], reason: 'Disabled' };
  }
  const inc = rule?.include?.filter(Boolean) || [];
  const exc = rule?.exclude?.filter(Boolean) || [];
  if (inc.length === 0 && exc.length === 0) {
    return { willRun: true, matchedFiles: paths, reason: 'Runs on all files' };
  }

  const matched = paths.filter((p) => {
    if (exc.some((pat) => matchesGlobPattern(pat, p))) return false;
    if (inc.length > 0) return inc.some((pat) => matchesGlobPattern(pat, p));
    return true;
  });

  return matched.length > 0
    ? { willRun: true, matchedFiles: matched, reason: `Matched ${matched.length} file${matched.length === 1 ? '' : 's'}` }
    : { willRun: false, matchedFiles: [], reason: 'No matching files (0 credits)' };
}

/** Build a rich, static-tool-specific LLM prompt grouped by category with include/exclude examples. */
export function buildTomlLLMPrompt(tools: RepoToolView[]): string {
  const byCategory: Record<string, RepoToolView[]> = {};
  tools.forEach((t) => {
    const cat = t.category?.trim() || 'General';
    if (!byCategory[cat]) byCategory[cat] = [];
    byCategory[cat].push(t);
  });

  const catalogLines = Object.keys(byCategory).length
    ? Object.entries(byCategory)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([cat, list]) => {
          const items = list
            .map(
              (t) =>
                `  - ${t.name} (${Number(t.multiplier).toFixed(1)}x cost): ${t.use_case || t.description}`,
            )
            .join('\n');
          return `Category: ${cat}\n${items}`;
        })
        .join('\n\n')
    : '(no tools registered in catalog yet)';

  return [
    "I'm configuring static analysis tools for a LiveReview repository using tools.toml format.",
    '',
    'Available static analysis tools by category:',
    catalogLines,
    '',
    'Supported schema (per-tool TOML sections):',
    '[tool-name]',
    'enabled = true                          # true to run, false to skip',
    'include = ["src/**/*.py", "**/*.pyi"]   # optional glob list — run only if matching files changed',
    'exclude = ["tests/**", "migrations/**"] # optional glob list — ignore matching paths',
    '',
    'Rules & best practices:',
    '- Use ONLY exact tool names listed above.',
    '- For beta, each tool supports `enabled`, `include`, and `exclude` only (no extra rule keys).',
    '- If a PR only touches files that fail `include` or match `exclude`, LiveReview skips that tool and charges 0 credits.',
    '- Keep high-cost tools (e.g. 12.0x scanners) scoped to relevant source paths and exclude generated/test/vendor directories when appropriate.',
    '- Any tool omitted from the configuration stays disabled for this repository.',
    '',
    'My repository & requirements:',
    '<describe your tech stack, directories, and which scanners/linters you want, e.g.:',
    '  "Python backend under api/, React TypeScript frontend under web/, Terraform under infra/.',
    '   Run ruff/bandit on api/ excluding tests, eslint on web/ excluding dist/, tfsec on infra/, and gitleaks everywhere">',
    '',
    'Please return ONLY valid TOML (no markdown code fences, no explanation).',
  ].join('\n');
}
