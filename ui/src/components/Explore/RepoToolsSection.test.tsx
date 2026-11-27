import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import RepoToolsSection from './RepoToolsSection';
import { getRepositoryTools, updateRepositoryTools } from '../../api/repositories';
import { useOrgContext } from '../../hooks/useOrgContext';
import { Repository } from '../../types/explore';
import {
  buildTomlLLMPrompt,
  evaluateToolForPaths,
  parseToolsTomlDetailed,
  upsertToolSectionInToml,
} from './toolsToml';

jest.mock('../../api/repositories', () => ({
  getRepositoryTools: jest.fn(),
  updateRepositoryTools: jest.fn(),
}));

jest.mock('../../hooks/useOrgContext', () => ({
  useOrgContext: jest.fn(),
}));

const mockGetRepositoryTools = getRepositoryTools as jest.Mock;
const mockUpdateRepositoryTools = updateRepositoryTools as jest.Mock;
const mockUseOrgContext = useOrgContext as jest.Mock;

const sampleRepo: Repository = {
  id: 12,
  org_id: 7,
  connector_id: 3,
  provider: 'github',
  provider_repo_id: '123',
  full_name: 'acme/web',
  name: 'web',
  web_url: 'https://github.com/acme/web',
  is_private: true,
  last_sync_status: 'ok',
  open_pr_count: 2,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

const sampleTools = [
  {
    id: 1,
    name: 'ruff',
    description: 'Python linter',
    lambda_arn: 'arn:ruff',
    multiplier: 1.0,
    use_case: 'Python lint',
    category: 'Code Quality',
    docs_url: 'https://github.com/astral-sh/ruff',
    enabled: false,
    org_enabled: false,
  },
  {
    id: 2,
    name: 'gitleaks',
    description: 'Secret scanner',
    lambda_arn: 'arn:gitleaks',
    multiplier: 12.0,
    use_case: 'Secrets',
    category: 'Security',
    docs_url: 'https://github.com/gitleaks/gitleaks',
    enabled: true,
    org_enabled: true,
  },
];

describe('RepoToolsSection', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockUseOrgContext.mockReturnValue({
      currentOrg: { id: 7, name: 'Acme', role: 'owner' },
      canManageCurrentOrg: true,
      isSuperAdmin: false,
    });
    mockGetRepositoryTools.mockResolvedValue({
      repository_id: 12,
      mode: 'inherit',
      tools: sampleTools,
    });
    mockUpdateRepositoryTools.mockResolvedValue({
      repository_id: 12,
      mode: 'custom',
      tools: [
        { ...sampleTools[0], enabled: true, repo_enabled: true },
        { ...sampleTools[1], enabled: false, repo_enabled: false },
      ],
    });
  });

  it('always renders toggle switches in Simple mode without Configuration Source and saves changes', async () => {
    render(
      <MemoryRouter>
        <RepoToolsSection repo={sampleRepo} />
      </MemoryRouter>,
    );

    expect(await screen.findByText('ruff')).toBeInTheDocument();
    expect(screen.queryByText(/CONFIGURATION SOURCE/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/Inherit organization default/i)).not.toBeInTheDocument();

    const ruffToggle = screen.getByRole('switch', { name: 'Toggle ruff' });
    const gitleaksToggle = screen.getByRole('switch', { name: 'Toggle gitleaks' });
    // Tools default to disabled until explicitly configured on the repository
    expect(ruffToggle).toHaveAttribute('aria-checked', 'false');
    expect(gitleaksToggle).toHaveAttribute('aria-checked', 'false');

    fireEvent.click(ruffToggle);
    fireEvent.click(screen.getByRole('button', { name: 'Save Changes' }));

    await waitFor(() => {
      expect(mockUpdateRepositoryTools).toHaveBeenCalledWith(12, {
        mode: 'custom',
        tools: { ruff: true, gitleaks: false },
        tool_configs: {},
      });
    });

    expect(await screen.findByText(/Saved repository tool settings for PR reviews/i)).toBeInTheDocument();
  });

  it('shows read-only warning and disables edits for non-owner members', async () => {
    mockUseOrgContext.mockReturnValue({
      currentOrg: { id: 7, name: 'Acme', role: 'member' },
      canManageCurrentOrg: false,
      isSuperAdmin: false,
    });

    render(
      <MemoryRouter>
        <RepoToolsSection repo={sampleRepo} />
      </MemoryRouter>,
    );

    expect(await screen.findByText(/You need the organization owner role/i)).toBeInTheDocument();
    expect(screen.queryByRole('switch', { name: 'Toggle ruff' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Save Changes' })).not.toBeInTheDocument();
  });

  it('opens Advanced tools.toml editor, supports category popup & include/exclude, updates live table, and saves tool_configs', async () => {
    render(
      <MemoryRouter>
        <RepoToolsSection repo={sampleRepo} />
      </MemoryRouter>,
    );

    expect(await screen.findByText('ruff')).toBeInTheDocument();

    // Open Advanced mode: by default 0 tools are enabled so unconfigured tools are not listed
    fireEvent.click(screen.getByRole('button', { name: /Advanced/i }));

    const editor = screen.getByRole('textbox', { name: 'tools.toml editor' }) as HTMLTextAreaElement;
    expect(editor.value).toContain('No tools enabled yet');
    expect(screen.getByText('VALID')).toBeInTheDocument();
    expect(screen.getByText(/Live table \(0\/2\)/i)).toBeInTheDocument();
    expect(screen.getByText(/No tools configured yet/i)).toBeInTheDocument();

    // Invalid TOML disables Save Changes
    fireEvent.change(editor, { target: { value: '[ruff]\nenabled = maybe\n' } });
    expect(screen.getByText('ERROR')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeDisabled();

    // Toggle quick help sidebar
    fireEvent.click(screen.getByRole('button', { name: /TOML quick help/i }));
    expect(screen.getByText('tools.toml quick help')).toBeInTheDocument();

    // Open Configure tool popup & search by category
    fireEvent.change(editor, { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: /Configure tool\s*Ctrl\+Space/i }));
    const search = screen.getByPlaceholderText(/Search category, tool, or language/i);
    fireEvent.change(search, { target: { value: 'Code Quality' } });
    expect(screen.getByRole('button', { name: /ruff · 1.0×/i })).toBeInTheDocument();

    // Click ruff to open its include/exclude configurator, enable and add include/exclude
    fireEvent.click(screen.getByRole('button', { name: /ruff · 1.0×/i }));
    const toggleSwitch = screen.getByRole('switch', { name: 'Popup toggle ruff' });
    expect(toggleSwitch).toHaveAttribute('aria-checked', 'true');
    fireEvent.click(screen.getByRole('button', { name: '+ **/*.py' }));
    fireEvent.click(screen.getByRole('button', { name: '− tests/**' }));
    fireEvent.click(screen.getByRole('button', { name: 'Apply to tools.toml' }));

    expect(editor.value).toContain('include = ["**/*.py"]');
    expect(editor.value).toContain('exclude = ["tests/**"]');
    expect(screen.getByText('VALID')).toBeInTheDocument();
    expect(screen.getByText('**/*.py')).toBeInTheDocument();
    expect(screen.getByText('tests/**')).toBeInTheDocument();

    // Live table row has a toggle switch that updates tools.toml live
    const liveSwitch = screen.getByRole('switch', { name: 'Live toggle ruff' });
    expect(liveSwitch).toHaveAttribute('aria-checked', 'true');
    fireEvent.click(liveSwitch);
    expect(editor.value).toContain('enabled = false');
    fireEvent.click(screen.getByRole('switch', { name: 'Live toggle ruff' }));
    expect(editor.value).toContain('enabled = true');

    // Test against file paths tab
    fireEvent.click(screen.getByRole('button', { name: /Test file paths/i }));
    expect(screen.getByText(/Will run \(Matched 1 file\)/i)).toBeInTheDocument();

    // Save changes persists both enabled map and include/exclude tool_configs
    const saveBtn = screen.getByRole('button', { name: 'Save Changes' });
    expect(saveBtn).not.toBeDisabled();
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(mockUpdateRepositoryTools).toHaveBeenCalledWith(12, {
        mode: 'custom',
        tools: { ruff: true, gitleaks: false },
        tool_configs: {
          ruff: { include: ['**/*.py'], exclude: ['tests/**'] },
        },
      });
    });
  });

  it('paginates when more than 10 tools are registered', async () => {
    const manyTools = Array.from({ length: 12 }, (_, idx) => ({
      id: idx + 1,
      name: `tool-${String(idx + 1).padStart(2, '0')}`,
      description: `Description ${idx + 1}`,
      lambda_arn: `arn:${idx + 1}`,
      multiplier: 1.0,
      use_case: 'Linting',
      category: 'Code Quality',
      enabled: false,
      org_enabled: false,
    }));
    mockGetRepositoryTools.mockResolvedValueOnce({
      repository_id: 12,
      mode: 'inherit',
      tools: manyTools,
    });

    render(
      <MemoryRouter>
        <RepoToolsSection repo={sampleRepo} />
      </MemoryRouter>,
    );

    expect(await screen.findByText('tool-01')).toBeInTheDocument();
    expect(screen.queryByText('tool-11')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Next page' }));
    expect(screen.getByText('tool-11')).toBeInTheDocument();
    expect(screen.queryByText('tool-01')).not.toBeInTheDocument();
  });
});

describe('toolsToml helpers', () => {
  it('parses sections, include/exclude globs, and upserts sections cleanly', () => {
    const src = `
      [ruff] # linter
      enabled = true
      include = ["src/**/*.py", "**/*.pyi"]
      exclude = ["tests/**"]

      [tools]
      gitleaks = false
    `;
    const res = parseToolsTomlDetailed(src);
    expect(res.valid).toBe(true);
    expect(res.enabledMap).toEqual({ ruff: true, gitleaks: false });
    expect(res.ruleConfigs).toEqual({
      ruff: { include: ['src/**/*.py', '**/*.pyi'], exclude: ['tests/**'] },
    });

    const updated = upsertToolSectionInToml('[ruff]\nenabled = false\n\n[gitleaks]\nenabled = true\n', 'ruff', true, {
      include: ['**/*.py'],
    });
    expect(updated).toBe('[ruff]\nenabled = true\ninclude = ["**/*.py"]\n\n[gitleaks]\nenabled = true\n');
  });

  it('evaluates include/exclude globs against sample paths and builds category LLM prompt', () => {
    const evalRes = evaluateToolForPaths(
      true,
      { include: ['**/*.py'], exclude: ['tests/**'] },
      ['src/app.py', 'tests/test_app.py', 'README.md'],
    );
    expect(evalRes.willRun).toBe(true);
    expect(evalRes.matchedFiles).toEqual(['src/app.py']);

    const skipRes = evaluateToolForPaths(
      true,
      { include: ['**/*.py'], exclude: ['tests/**'] },
      ['tests/test_app.py', 'README.md'],
    );
    expect(skipRes.willRun).toBe(false);

    const prompt = buildTomlLLMPrompt(sampleTools);
    expect(prompt).toContain('Category: Code Quality');
    expect(prompt).toContain('Category: Security');
    expect(prompt).toContain('include');
    expect(prompt).toContain('exclude');
  });
});
