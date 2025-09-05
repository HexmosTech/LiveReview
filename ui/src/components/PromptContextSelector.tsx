import React from 'react';

export type PromptContext = {
  ai_connector_id?: number;
  integration_token_id?: number;
  repository?: string;
};

type Props = {
  value: PromptContext;
  onChange: (ctx: PromptContext) => void;
};

export const PromptContextSelector: React.FC<Props> = ({ value, onChange }) => {
  return (
    <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
      <div>
        <label className="block text-xs text-slate-400 mb-1">AI Connector ID (optional)</label>
        <input
          type="number"
          value={value.ai_connector_id ?? ''}
          onChange={(e) => onChange({ ...value, ai_connector_id: e.target.value ? Number(e.target.value) : undefined })}
          className="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1 text-slate-200"
          placeholder="e.g., 1"
        />
      </div>
      <div>
        <label className="block text-xs text-slate-400 mb-1">Integration Token ID (optional)</label>
        <input
          type="number"
          value={value.integration_token_id ?? ''}
          onChange={(e) => onChange({ ...value, integration_token_id: e.target.value ? Number(e.target.value) : undefined })}
          className="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1 text-slate-200"
          placeholder="e.g., 5"
        />
      </div>
      <div>
        <label className="block text-xs text-slate-400 mb-1">Repository (optional)</label>
        <input
          type="text"
          value={value.repository ?? ''}
          onChange={(e) => onChange({ ...value, repository: e.target.value || undefined })}
          className="w-full bg-slate-800 border border-slate-700 rounded px-2 py-1 text-slate-200"
          placeholder="group/repo"
        />
      </div>
    </div>
  );
};

export default PromptContextSelector;
