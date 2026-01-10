// IssuesPanel component - filterable issues list with copy functionality
import { waitForPreact, getBadgeClass, copyToClipboard } from './utils.js';

export async function createIssuesPanel() {
    const { html, useState, useEffect } = await waitForPreact();
    
    return function IssuesPanel({ files, visible, onNavigate }) {
        const [filter, setFilter] = useState('error,warning');
        const [selectedIndices, setSelectedIndices] = useState(new Set());
        const [status, setStatus] = useState('');
        
        // Collect issues from files
        const issues = [];
        files.forEach(file => {
            if (!file.HasComments) return;
            file.Hunks.forEach(hunk => {
                hunk.Lines.forEach((line, lineIdx) => {
                    if (line.IsComment && line.Comments) {
                        line.Comments.forEach((comment, commentIdx) => {
                            issues.push({
                                filePath: file.FilePath,
                                fileId: 'file_' + file.FilePath.replace(/[^a-zA-Z0-9]/g, '_'),
                                line: comment.Line,
                                body: comment.Content,
                                severity: comment.Severity,
                                category: comment.Category,
                                commentId: `comment-${file.ID}-${comment.Line}-${commentIdx}`
                            });
                        });
                    }
                });
            });
        });
        
        // Initialize selected based on filter
        useEffect(() => {
            const newSelected = new Set();
            issues.forEach((issue, idx) => {
                const sev = (issue.severity || '').toLowerCase();
                if (sev === 'error' || sev === 'warning' || sev === 'critical') {
                    newSelected.add(idx);
                }
            });
            setSelectedIndices(newSelected);
        }, [issues.length]);
        
        if (!visible) return null;
        
        const filterMatches = (severity) => {
            const sev = (severity || '').toLowerCase();
            if (filter === 'all') return true;
            return filter.includes(sev);
        };
        
        const visibleIssues = issues.filter(issue => filterMatches(issue.severity));
        
        const handleSelectAll = () => {
            const newSelected = new Set(selectedIndices);
            issues.forEach((issue, idx) => {
                if (filterMatches(issue.severity)) {
                    newSelected.add(idx);
                }
            });
            setSelectedIndices(newSelected);
        };
        
        const handleDeselectAll = () => {
            const newSelected = new Set(selectedIndices);
            issues.forEach((issue, idx) => {
                if (filterMatches(issue.severity)) {
                    newSelected.delete(idx);
                }
            });
            setSelectedIndices(newSelected);
        };
        
        const handleCopy = async () => {
            const selected = issues.filter((_, idx) => selectedIndices.has(idx));
            if (selected.length === 0) {
                setStatus('Nothing selected to copy');
                return;
            }
            
            const lines = selected.map(issue => {
                const lineSuffix = issue.line ? ':' + issue.line : '';
                const sev = issue.severity ? ` (${issue.severity}${issue.category ? ', ' + issue.category : ''})` : '';
                return `${issue.filePath}${lineSuffix} — ${issue.body}${sev}`;
            });
            
            try {
                await copyToClipboard(lines.join('\n'));
                setStatus(`Copied ${selected.length} issue(s)`);
            } catch (err) {
                setStatus('Copy failed: ' + err.message);
            }
        };
        
        const toggleSelected = (idx) => {
            const newSelected = new Set(selectedIndices);
            if (newSelected.has(idx)) {
                newSelected.delete(idx);
            } else {
                newSelected.add(idx);
            }
            setSelectedIndices(newSelected);
        };
        
        const setFilterType = (type) => {
            if (type === 'all') {
                setFilter('all');
            } else {
                setFilter(type);
            }
        };
        
        return html`
            <div class="issues-panel">
                <div class="issues-actions">
                    <div class="severity-filters">
                        <button 
                            class="severity-filter-btn all ${filter === 'all' ? 'active' : ''}"
                            onClick=${() => setFilterType('all')}
                        >All</button>
                        <button 
                            class="severity-filter-btn error ${filter.includes('error') ? 'active' : ''}"
                            onClick=${() => setFilterType('error')}
                        >Error</button>
                        <button 
                            class="severity-filter-btn warning ${filter.includes('warning') ? 'active' : ''}"
                            onClick=${() => setFilterType('warning')}
                        >Warning</button>
                        <button 
                            class="severity-filter-btn info ${filter === 'info' ? 'active' : ''}"
                            onClick=${() => setFilterType('info')}
                        >Info</button>
                    </div>
                    <button class="btn-ghost" onClick=${handleSelectAll}>Select All</button>
                    <button class="btn-ghost" onClick=${handleDeselectAll}>Deselect All</button>
                    <button class="btn-primary" onClick=${handleCopy}>Copy Selected</button>
                    <span class="issues-status">${status}</span>
                </div>
                <div class="issues-list">
                    ${issues.map((issue, idx) => {
                        const hidden = !filterMatches(issue.severity);
                        return html`
                            <div class="issue-item ${hidden ? 'hidden' : ''}" data-severity="${(issue.severity || '').toLowerCase()}">
                                <input 
                                    type="checkbox"
                                    checked=${selectedIndices.has(idx)}
                                    onChange=${() => toggleSelected(idx)}
                                />
                                <div>
                                    <div class="issue-path">
                                        ${issue.filePath}${issue.line ? ':' + issue.line : ''}
                                    </div>
                                    <div class="issue-message">
                                        ${issue.body}${issue.severity ? ` (${issue.severity}${issue.category ? ', ' + issue.category : ''})` : ''}
                                    </div>
                                </div>
                                <a 
                                    class="issue-nav-link"
                                    href="#${issue.commentId}"
                                    title="Navigate to comment"
                                    onClick=${(e) => {
                                        e.preventDefault();
                                        onNavigate(issue.commentId, issue.fileId);
                                    }}
                                >→</a>
                            </div>
                        `;
                    })}
                </div>
            </div>
        `;
    };
}

let IssuesPanelComponent = null;
export async function getIssuesPanel() {
    if (!IssuesPanelComponent) {
        IssuesPanelComponent = await createIssuesPanel();
    }
    return IssuesPanelComponent;
}
