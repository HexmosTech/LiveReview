import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button, Icons } from '../../components/UIPrimitives';
import { getApiUrl } from '../../utils/apiUrl';

interface MCPIntegrationTabProps {
    // Settings shows its own tab heading; pages that already have their own section
    // heading (e.g. the "Create via MCP" page) can suppress this one.
    showHeading?: boolean;
}

const MCPIntegrationTab: React.FC<MCPIntegrationTabProps> = ({ showHeading = true }) => {
    const navigate = useNavigate();
    const [copied, setCopied] = useState(false);

    const mcpServerUrl = `${getApiUrl()}/api/mcp`;
    const configSnippet = `{
  "mcpServers": {
    "livereview": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote",
        "${mcpServerUrl}",
        "--header",
        "X-API-KEY: \${LIVEREVIEW_API_KEY}"
      ],
      "env": {
        "LIVEREVIEW_API_KEY": "<YOUR_LIVEREVIEW_API_KEY>"
      }
    }
  }
}`;

    const copyConfig = () => {
        navigator.clipboard.writeText(configSnippet);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
    };

    return (
        <div className="space-y-6">
            {showHeading && (
                <div>
                    <h3 className="text-lg font-medium text-white mb-1">Model Context Protocol (MCP) Server</h3>
                    <p className="text-sm text-slate-300">
                        Integrate LiveReview natively into your favorite AI-powered IDEs and clients, including Cursor, Claude Desktop, Windsurf, and VS Code.
                    </p>
                </div>
            )}

            <div className="bg-slate-800 rounded-lg border border-slate-700 p-5">
                <h4 className="text-white font-medium mb-3">1. Get your API key</h4>
                <p className="text-sm text-slate-300 mb-3">
                    Open the API Keys page and generate a new key — you'll need it in step 2 below.
                </p>
                <Button
                    size="sm"
                    variant="outline"
                    onClick={() => navigate('/settings#api-keys')}
                >
                    Go to API Keys
                </Button>
            </div>

            <div className="bg-slate-800 rounded-lg border border-slate-700 p-5">
                <h4 className="text-white font-medium mb-2">2. Configure your MCP client</h4>
                <p className="text-sm text-slate-300 mb-1">
                    Add the following block to your MCP client's configuration file.
                </p>
                <p className="text-xs text-slate-400 mb-3">
                    For example, Claude Desktop uses <code className="bg-slate-700 px-1 rounded">claude_desktop_config.json</code>. For other clients, check the client's documentation for the equivalent file.
                </p>
                <div className="relative">
                    <pre className="bg-slate-900 rounded p-3 text-xs text-slate-200 font-mono overflow-x-auto whitespace-pre">{configSnippet}</pre>
                    <Button
                        size="sm"
                        variant="ghost"
                        className="absolute top-2 right-2"
                        onClick={copyConfig}
                        icon={<Icons.Copy />}
                    >
                        {copied ? 'Copied!' : 'Copy'}
                    </Button>
                </div>
                <p className="text-xs text-slate-400 mt-3">
                    Replace <code className="bg-slate-700 px-1 rounded">&lt;YOUR_LIVEREVIEW_API_KEY&gt;</code> with your actual LiveReview API key.
                </p>
            </div>
        </div>
    );
};

export default MCPIntegrationTab;
