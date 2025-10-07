import React, { useEffect, useMemo, useState } from 'react';
import apiClient from '../../api/apiClient';
import { Badge, Button, Icons, Input, Card } from '../../components/UIPrimitives';

type ScopeKind = 'org' | 'repo';
type Status = 'active' | 'archived';

export interface Learning {
  id: string;
  short_id: string;
  org_id: number;
  scope: ScopeKind;
  repo_id?: string;
  title: string;
  body: string;
  tags: string[];
  status: Status;
  confidence: number;
  source_urls?: string[];
  created_at: string;
  updated_at: string;
}

interface EditState {
  id: string;
  title: string;
  body: string;
  tagsCsv: string;
  scope: ScopeKind;
  saving: boolean;
}

const ScopeBadge: React.FC<{ scope: ScopeKind }>
  = ({ scope }) => (
    <Badge variant={scope === 'org' ? 'info' : 'primary'}>{scope}</Badge>
  );

const StatusBadge: React.FC<{ status: Status }>
  = ({ status }) => (
    <Badge variant={status === 'active' ? 'success' : 'warning'}>{status}</Badge>
  );

const LearningsTab: React.FC = () => {
  const [items, setItems] = useState<Learning[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [editing, setEditing] = useState<EditState | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const fetchItems = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await apiClient.get<Learning[] | null>('/api/v1/learnings');
      const safe = Array.isArray(data) ? data : [];
      // Sort by updated_at desc
      safe.sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime());
      setItems(safe);
    } catch (e: any) {
      setError(e?.message || 'Failed to load learnings');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchItems();
  }, []);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return items;
    return items.filter(it =>
      it.title.toLowerCase().includes(q) ||
      it.body.toLowerCase().includes(q) ||
      (it.tags || []).some(t => t.toLowerCase().includes(q)) ||
      it.short_id.toLowerCase().includes(q)
    );
  }, [items, search]);

  const beginEdit = (it: Learning) => {
    setEditing({
      id: it.id,
      title: it.title,
      body: it.body,
      tagsCsv: (it.tags || []).join(', '),
      scope: it.scope,
      saving: false,
    });
  };

  const saveEdit = async () => {
    if (!editing) return;
    setEditing({ ...editing, saving: true });
    const tags = editing.tagsCsv
      .split(',')
      .map(t => t.trim())
      .filter(Boolean);
    try {
      // PUT expects optional fields; we'll send those we edit
      const payload: any = {
        title: editing.title,
        body: editing.body,
        tags: tags,
        scope_kind: editing.scope,
      };
      const updated = await apiClient.put<Learning>(`/api/v1/learnings/${editing.id}`, payload);
      setItems(prev => prev.map(p => (p.id === updated.id ? updated : p)));
      setEditing(null);
    } catch (e: any) {
      setError(e?.message || 'Failed to save');
      setEditing({ ...editing, saving: false });
    }
  };

  const deleteLearning = async (id: string) => {
    setDeletingId(id);
    try {
      await apiClient.delete(`/api/v1/learnings/${id}`);
      // Optimistic remove or mark archived if server returns updated entity
      setItems(prev => prev.filter(p => p.id !== id));
    } catch (e: any) {
      setError(e?.message || 'Failed to delete');
    } finally {
      setDeletingId(null);
    }
  };

  return (
    <div className="p-4 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-white text-lg font-medium">Learnings</h3>
          <p className="text-slate-300 text-sm">Org and repo-scoped learnings captured from MR threads.</p>
        </div>
        <div className="flex items-center space-x-2">
          <Button variant="outline" onClick={fetchItems} isLoading={loading}>
            <Icons.Refresh />
            Refresh
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
        <Input
          label="Search"
          placeholder="Search by title, tags, body, or ID"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </div>

      {error && (
        <Card>
          <div className="p-3 text-sm text-red-300 flex items-center space-x-2">
            <Icons.Error />
            <span>{error}</span>
          </div>
        </Card>
      )}

      <div className="overflow-auto rounded-lg border border-slate-700">
        <table className="min-w-full divide-y divide-slate-700">
          <thead className="bg-slate-800/50">
            <tr>
              <th className="px-4 py-2 text-left text-xs font-medium text-slate-300">ID</th>
              <th className="px-4 py-2 text-left text-xs font-medium text-slate-300">Title</th>
              <th className="px-4 py-2 text-left text-xs font-medium text-slate-300">Scope</th>
              <th className="px-4 py-2 text-left text-xs font-medium text-slate-300">Tags</th>
              <th className="px-4 py-2 text-left text-xs font-medium text-slate-300">Status</th>
              <th className="px-4 py-2 text-left text-xs font-medium text-slate-300">Updated</th>
              <th className="px-4 py-2 text-right text-xs font-medium text-slate-300">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-800">
            {filtered.map((it) => (
              <tr key={it.id} className="hover:bg-slate-800/30">
                <td className="px-4 py-2 whitespace-nowrap text-slate-300 font-mono">#{it.short_id}</td>
                <td className="px-4 py-2">
                  <div className="text-white font-medium line-clamp-2">{it.title}</div>
                  <div className="text-slate-400 text-xs line-clamp-1">{it.body}</div>
                </td>
                <td className="px-4 py-2"><ScopeBadge scope={it.scope} /></td>
                <td className="px-4 py-2">
                  <div className="flex flex-wrap gap-1">
                    {(it.tags || []).map(t => (
                      <Badge key={t} variant="default">{t}</Badge>
                    ))}
                  </div>
                </td>
                <td className="px-4 py-2"><StatusBadge status={it.status} /></td>
                <td className="px-4 py-2 text-slate-400 text-sm whitespace-nowrap">{new Date(it.updated_at).toLocaleString()}</td>
                <td className="px-4 py-2">
                  <div className="flex items-center justify-end space-x-2">
                    <Button size="sm" variant="ghost" onClick={() => beginEdit(it)}>
                      <Icons.Edit />
                      Edit
                    </Button>
                    <Button size="sm" variant="danger" onClick={() => deleteLearning(it.id)} isLoading={deletingId === it.id}>
                      <Icons.Delete />
                      Delete
                    </Button>
                  </div>
                </td>
              </tr>
            ))}
            {filtered.length === 0 && !loading && (
              <tr>
                <td className="px-4 py-6 text-center text-slate-400 text-sm" colSpan={7}>No learnings found</td>
              </tr>
            )}
            {loading && (
              <tr>
                <td className="px-4 py-6 text-center text-slate-400 text-sm" colSpan={7}>Loading…</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {editing && (
        <Card>
          <div className="p-4 space-y-3">
            <div className="flex items-center justify-between">
              <div>
                <h4 className="text-white font-medium">Edit Learning #{items.find(x => x.id === editing.id)?.short_id}</h4>
                <p className="text-slate-400 text-xs">Update the content and tags. Changes are saved for this organization.</p>
              </div>
              <div className="flex items-center space-x-2">
                <Button variant="ghost" onClick={() => setEditing(null)} disabled={editing.saving}>
                  Cancel
                </Button>
                <Button variant="primary" onClick={saveEdit} isLoading={editing.saving}>
                  Save
                </Button>
              </div>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              <Input
                label="Title"
                value={editing.title}
                onChange={(e) => setEditing({ ...editing, title: e.target.value })}
              />
              <div>
                <label className="block text-sm text-slate-300 mb-1">Scope</label>
                <select
                  className="w-full bg-slate-800 text-slate-100 border border-slate-600 rounded px-3 py-2"
                  value={editing.scope}
                  onChange={(e) => setEditing({ ...editing, scope: e.target.value as ScopeKind })}
                >
                  <option value="org">org</option>
                  <option value="repo">repo</option>
                </select>
              </div>
            </div>
            <div>
              <label className="block text-sm text-slate-300 mb-1">Body</label>
              <textarea
                className="w-full bg-slate-800 text-slate-100 border border-slate-600 rounded px-3 py-2 h-28"
                value={editing.body}
                onChange={(e) => setEditing({ ...editing, body: e.target.value })}
              />
            </div>
            <Input
              label="Tags (comma separated)"
              value={editing.tagsCsv}
              onChange={(e) => setEditing({ ...editing, tagsCsv: e.target.value })}
            />
          </div>
        </Card>
      )}
    </div>
  );
};

export default LearningsTab;
