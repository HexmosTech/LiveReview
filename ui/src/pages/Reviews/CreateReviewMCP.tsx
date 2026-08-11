import React from 'react';
import { PageHeader, Icons } from '../../components/UIPrimitives';
import { ErrorBoundary } from '../../components/ErrorBoundary';
import MCPIntegrationTab from '../Settings/MCPIntegrationTab';

// icon is omitted for platforms with no available brand mark; those fall back to a generic icon.
const SUPPORTED_PLATFORMS: { name: string; icon?: React.ReactNode }[] = [
  { name: 'Claude Code', icon: <Icons.ClaudeCode /> },
  { name: 'Cursor', icon: <Icons.Cursor /> },
  { name: 'Codex', icon: <Icons.OpenAI /> },
  { name: 'Antigravity', icon: <Icons.Google /> },
  { name: 'Claude Desktop', icon: <Icons.Claude /> },
  { name: 'Windsurf', icon: <Icons.Windsurf /> },
  { name: 'VS Code', icon: <Icons.VSCode /> },
];

const DOC_LINKS = [
  {
    label: 'API Documentation',
    description: 'Full reference for the LiveReview REST API.',
    href: 'https://hexmos.com/livereview/docs/livereview/api/',
  },
  {
    label: 'MCP Use Cases',
    description: 'Example prompts and workflows for triggering reviews over MCP.',
    href: 'https://hexmos.com/livereview/docs/livereview/mcp/usecases/',
  },
];

// Standalone page for the mega menu's "Create via MCP" entry — reuses the same MCP setup
// component shown at /settings#mcp so both stay in sync, with added context (supported
// platforms, docs links) that doesn't belong in the settings tab itself.
const CreateReviewMCP: React.FC = () => {
  return (
    <ErrorBoundary>
      <div className="container mx-auto px-4 py-8">
        <PageHeader
          title="Create a Review with MCP"
          description="Connect LiveReview to your AI coding assistant over MCP and trigger reviews without leaving your editor."
        />

        <section className="mb-8">
          <h3 className="text-xs font-semibold text-slate-400 uppercase tracking-wide mb-3">
            Supported platforms
          </h3>
          <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
            {SUPPORTED_PLATFORMS.map((platform) => (
              <div key={platform.name} className="flex items-center gap-2">
                <span className="shrink-0 text-slate-300">{platform.icon ?? <Icons.AI />}</span>
                <span className="text-sm font-medium text-slate-200">{platform.name}</span>
              </div>
            ))}
            <span className="text-sm text-slate-500">+ any MCP-compatible client</span>
          </div>
        </section>

        <section className="mb-8">
          <h3 className="text-xs font-semibold text-slate-400 uppercase tracking-wide mb-3">
            Setup Instructions
          </h3>
          <MCPIntegrationTab showHeading={false} />
        </section>

        <section>
          <h3 className="text-xs font-semibold text-slate-400 uppercase tracking-wide mb-3">
            Documentation
          </h3>
          <div className="flex flex-col sm:flex-row gap-3">
            {DOC_LINKS.map((doc) => (
              <a
                key={doc.href}
                href={doc.href}
                target="_blank"
                rel="noopener noreferrer"
                className="group flex-1 flex items-center justify-between gap-3 rounded-lg border border-slate-700/80 hover:border-cyan-400/60 hover:bg-slate-800/40 transition-colors px-4 py-3"
              >
                <div>
                  <p className="text-sm font-medium text-slate-100 group-hover:text-cyan-300">{doc.label}</p>
                  <p className="text-xs text-slate-400 mt-0.5">{doc.description}</p>
                </div>
                <span className="text-slate-500 group-hover:text-cyan-300 shrink-0">
                  <Icons.FolderOpen />
                </span>
              </a>
            ))}
          </div>
        </section>
      </div>
    </ErrorBoundary>
  );
};

export default CreateReviewMCP;
