import React, { useEffect, useMemo, useState } from 'react';
import apiClient from '../../api/apiClient';
import { Badge, Button, Icons, Input, Card } from '../../components/UIPrimitives';
import { HumanizedTimestamp } from '../../components/HumanizedTimestamp/HumanizedTimestamp';
import CompactTags from '../../components/CompactTags';

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

interface PaginationInfo {
  page: number;
  limit: number;
  total: number;
  total_pages: number;
  has_next: boolean;
  has_prev: boolean;
}

interface LearningsResponse {
  items: Learning[];
  pagination: PaginationInfo;
}

const ScopeBadge: React.FC<{ scope: ScopeKind }> = ({ scope }) => (
  <Badge variant={scope === 'org' ? 'info' : 'primary'} className="text-xs">{scope}</Badge>
);

const StatusBadge: React.FC<{ status: Status }> = ({ status }) => (
  <Badge 
    variant={status === 'active' ? 'success' : 'warning'} 
    className="text-xs"
  >
    {status}
  </Badge>
);

const LearningsTab: React.FC = () => {
  const [items, setItems] = useState<Learning[]>([]);
  const [pagination, setPagination] = useState<PaginationInfo>({
    page: 1,
    limit: 20,
    total: 0,
    total_pages: 0,
    has_next: false,
    has_prev: false,
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [hideArchived, setHideArchived] = useState(true); // Default to hiding archived
  const [editing, setEditing] = useState<EditState | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  // Handle ESC key to close edit dialog
  useEffect(() => {
    const handleEscKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && editing) {
        setEditing(null);
      }
    };

    document.addEventListener('keydown', handleEscKey);
    return () => {
      document.removeEventListener('keydown', handleEscKey);
    };
  }, [editing]);

  const fetchItems = async (page: number = 1, searchQuery?: string) => {
    setLoading(true);
    setError(null);
    try {
      const params = new URLSearchParams({
        page: page.toString(),
        limit: pagination.limit.toString(),
        include_archived: (!hideArchived).toString(),
      });
      
      if (searchQuery !== undefined) {
        if (searchQuery.trim()) {
          params.set('search', searchQuery.trim());
        }
      } else if (search.trim()) {
        params.set('search', search.trim());
      }

      const data = await apiClient.get<LearningsResponse>(`/api/v1/learnings?${params}`);
      
      if (data && data.items && data.pagination) {
        setItems(data.items);
        setPagination(data.pagination);
      } else {
        // Fallback for old API format
        const items: Learning[] = Array.isArray(data) ? data : [];
        setItems(items);
        setPagination({
          page: 1,
          limit: 20,
          total: items.length,
          total_pages: 1,
          has_next: false,
          has_prev: false,
        });
      }
    } catch (e: any) {
      setError(e?.message || 'Failed to load learnings');
      setItems([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchItems();
  }, [hideArchived]);

  // Debounced search
  useEffect(() => {
    const timer = setTimeout(() => {
      fetchItems(1, search);
    }, 300);
    return () => clearTimeout(timer);
  }, [search]);

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
      setItems(prev => prev.filter(p => p.id !== id));
      // Update pagination total
      setPagination(prev => ({ ...prev, total: prev.total - 1 }));
    } catch (e: any) {
      setError(e?.message || 'Failed to delete');
    } finally {
      setDeletingId(null);
    }
  };

  const handlePageChange = (newPage: number) => {
    fetchItems(newPage);
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-white text-lg font-medium">Learnings</h3>
          <p className="text-slate-300 text-sm">Org and repo-scoped learnings captured from MR threads.</p>
        </div>
        <div className="flex items-center space-x-3">
          <Button variant="outline" onClick={() => fetchItems(pagination.page)} isLoading={loading} size="sm">
            <Icons.Refresh />
            Refresh
          </Button>
        </div>
      </div>

      {/* Filters */}
      <div className="flex flex-col sm:flex-row gap-3">
        <div className="flex-1">
          <Input
            placeholder="Search by title, tags, body, or ID"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full"
          />
        </div>
        <div className="flex items-center space-x-2">
          <label className="flex items-center space-x-2 text-sm text-slate-300 whitespace-nowrap">
            <input
              type="checkbox"
              checked={hideArchived}
              onChange={(e) => setHideArchived(e.target.checked)}
              className="rounded border-slate-600 bg-slate-800 text-blue-500 focus:ring-blue-500 focus:ring-2"
            />
            <span>Hide archived</span>
          </label>
        </div>
      </div>

      {/* Error Display */}
      {error && (
        <Card>
          <div className="p-3 text-sm text-red-300 flex items-center space-x-2">
            <Icons.Error />
            <span>{error}</span>
            <Button size="sm" variant="ghost" onClick={() => setError(null)}>
              <Icons.Delete />
            </Button>
          </div>
        </Card>
      )}

      {/* Table */}
      <div className="bg-slate-900 rounded-lg border border-slate-700 overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full divide-y divide-slate-700">
            <thead className="bg-slate-800/50">
              <tr>
                <th className="px-3 py-3 text-left text-xs font-medium text-slate-300 uppercase tracking-wider">ID</th>
                <th className="px-3 py-3 text-left text-xs font-medium text-slate-300 uppercase tracking-wider">Title</th>
                <th className="px-3 py-3 text-left text-xs font-medium text-slate-300 uppercase tracking-wider">Updated</th>
                <th className="px-3 py-3 text-left text-xs font-medium text-slate-300 uppercase tracking-wider">Status</th>
                <th className="px-3 py-3 text-left text-xs font-medium text-slate-300 uppercase tracking-wider">Scope</th>
                <th className="px-3 py-3 text-left text-xs font-medium text-slate-300 uppercase tracking-wider">Tags</th>
                <th className="px-3 py-3 text-right text-xs font-medium text-slate-300 uppercase tracking-wider">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800">
              {items.map((it) => (
                <tr key={it.id} className="hover:bg-slate-800/30 transition-colors">
                  <td className="px-3 py-3 whitespace-nowrap">
                    <span className="text-slate-300 font-mono text-sm">#{it.short_id}</span>
                  </td>
                  <td className="px-3 py-3">
                    <div className="max-w-sm cursor-pointer" onClick={() => beginEdit(it)} title={`${it.title}\n\n${it.body}`}>
                      <div className="text-white font-medium text-sm line-clamp-1 mb-1 hover:text-blue-300 transition-colors">
                        {it.title}
                      </div>
                      <div className="text-slate-400 text-xs line-clamp-2 leading-relaxed hover:text-slate-300 transition-colors">
                        {it.body}
                      </div>
                    </div>
                  </td>
                  <td className="px-3 py-3 whitespace-nowrap">
                    <HumanizedTimestamp timestamp={it.updated_at} className="text-slate-400 text-sm" />
                  </td>
                  <td className="px-3 py-3 whitespace-nowrap">
                    <StatusBadge status={it.status} />
                  </td>
                  <td className="px-3 py-3 whitespace-nowrap">
                    <ScopeBadge scope={it.scope} />
                  </td>
                  <td className="px-3 py-3">
                    <div className="max-w-32">
                      <CompactTags tags={it.tags || []} maxVisible={2} />
                    </div>
                  </td>
                  <td className="px-3 py-3 whitespace-nowrap text-right">
                    <div className="flex items-center justify-end space-x-1">
                      <Button size="sm" variant="ghost" onClick={() => beginEdit(it)} className="opacity-70 hover:opacity-100">
                        <Icons.Edit />
                      </Button>
                      <Button 
                        size="sm" 
                        variant="danger" 
                        onClick={() => deleteLearning(it.id)} 
                        isLoading={deletingId === it.id}
                        className="opacity-70 hover:opacity-100"
                      >
                        <Icons.Delete />
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
              
              {/* Empty/Loading states */}
              {items.length === 0 && !loading && (
                <tr>
                  <td className="px-3 py-8 text-center text-slate-400 text-sm" colSpan={7}>
                    {search || !hideArchived ? 'No learnings found' : 'No active learnings found'}
                  </td>
                </tr>
              )}
              {loading && (
                <tr>
                  <td className="px-3 py-8 text-center text-slate-400 text-sm" colSpan={7}>
                    <div className="flex items-center justify-center space-x-2">
                      <Icons.Refresh />
                      <span>Loading…</span>
                    </div>
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        {/* Pagination */}
        {pagination.total_pages > 1 && (
          <div className="bg-slate-800/30 px-4 py-3 border-t border-slate-700">
            <div className="flex items-center justify-between">
              <div className="text-sm text-slate-400">
                Showing {items.length} of {pagination.total} learnings
              </div>
              <div className="flex items-center space-x-2">
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => handlePageChange(pagination.page - 1)}
                  disabled={!pagination.has_prev}
                >
                  <span>←</span>
                  Previous
                </Button>
                
                <div className="flex items-center space-x-1">
                  {/* Show page numbers */}
                  {Array.from({ length: Math.min(5, pagination.total_pages) }, (_, i) => {
                    const pageNum = Math.max(1, pagination.page - 2) + i;
                    if (pageNum > pagination.total_pages) return null;
                    
                    return (
                      <Button
                        key={pageNum}
                        size="sm"
                        variant={pageNum === pagination.page ? "primary" : "ghost"}
                        onClick={() => handlePageChange(pageNum)}
                        className="w-8 h-8 p-0"
                      >
                        {pageNum}
                      </Button>
                    );
                  })}
                </div>

                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => handlePageChange(pagination.page + 1)}
                  disabled={!pagination.has_next}
                >
                  Next
                  <span>→</span>
                </Button>
              </div>
            </div>
          </div>
        )}
      </div>

      {/* Edit Modal */}
      {editing && (
        <>
          <div className="fixed inset-0 bg-black/50 z-40" onClick={() => setEditing(null)} />
          <div className="fixed inset-0 flex items-center justify-center p-4 z-50 pointer-events-none">
            <div className="w-full max-w-2xl pointer-events-auto">
              <Card className="w-full">
              {/* Fixed Header */}
              <div className="flex items-center justify-between p-4 border-b border-slate-700">
                <div>
                  <h4 className="text-white font-medium">Edit Learning #{items.find(x => x.id === editing.id)?.short_id}</h4>
                  <p className="text-slate-400 text-sm">Update the content and tags. Changes are saved for this organization.</p>
                </div>
                <Button variant="ghost" onClick={() => setEditing(null)} disabled={editing.saving} className="text-slate-400 hover:text-white">
                  ×
                </Button>
              </div>
              
              {/* Content */}
              <div className="p-4 space-y-4">
                <div>
                  <label className="block text-sm text-slate-300 mb-2">Title</label>
                  <textarea
                    className="w-full bg-slate-800 text-slate-100 border border-slate-600 rounded-md px-3 py-2 h-16 resize-none"
                    value={editing.title}
                    onChange={(e) => setEditing({ ...editing, title: e.target.value })}
                  />
                </div>
                
                <div>
                  <label className="block text-sm text-slate-300 mb-2">Scope</label>
                  <select
                    className="w-full bg-slate-800 text-slate-100 border border-slate-600 rounded-md px-3 py-2"
                    value={editing.scope}
                    onChange={(e) => setEditing({ ...editing, scope: e.target.value as ScopeKind })}
                  >
                    <option value="org">Organization</option>
                    <option value="repo">Repository</option>
                  </select>
                </div>
                
                <div>
                  <label className="block text-sm text-slate-300 mb-2">Content</label>
                  <textarea
                    className="w-full bg-slate-800 text-slate-100 border border-slate-600 rounded-md px-3 py-2 h-24 resize-none"
                    value={editing.body}
                    onChange={(e) => setEditing({ ...editing, body: e.target.value })}
                  />
                </div>
                
                <div>
                  <label className="block text-sm text-slate-300 mb-2">Tags (comma separated)</label>
                  <input
                    type="text"
                    className="w-full bg-slate-800 text-slate-100 border border-slate-600 rounded-md px-3 py-2"
                    value={editing.tagsCsv}
                    onChange={(e) => setEditing({ ...editing, tagsCsv: e.target.value })}
                    placeholder="assertions, team_policy, error_handling"
                  />
                </div>
              </div>
              
              {/* Fixed Footer */}
              <div className="flex items-center justify-between p-4 border-t border-slate-700">
                <Button 
                  variant="danger" 
                  onClick={() => {
                    if (confirm('Are you sure you want to delete this learning? This action cannot be undone.')) {
                      deleteLearning(editing.id);
                      setEditing(null);
                    }
                  }} 
                  disabled={editing.saving}
                  className="text-red-400 hover:text-red-300 hover:bg-red-900/20"
                >
                  <Icons.Delete />
                  Delete
                </Button>
                <div className="flex items-center space-x-3">
                  <Button variant="ghost" onClick={() => setEditing(null)} disabled={editing.saving}>
                    Cancel
                  </Button>
                  <Button variant="primary" onClick={saveEdit} isLoading={editing.saving}>
                    Save Changes
                  </Button>
                </div>
              </div>
            </Card>
          </div>
        </div>
        </>
      )}
    </div>
  );
};

export default LearningsTab;