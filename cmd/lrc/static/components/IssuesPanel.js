// IssuesPanel component - filterable issues list with copy functionality
import { waitForPreact, getBadgeClass, copyToClipboard } from './utils.js';

export async function createIssuesPanel() {
    const { html, useState, useEffect, useCallback } = await waitForPreact();
    
    return function IssuesPanel({ files, visible, onNavigate, onClose }) {
        // Multi-select filters: Set of active severity types (default: critical + error + warning)
        const [activeFilters, setActiveFilters] = useState(new Set(['critical', 'error', 'warning']));
        const [selectedIndices, setSelectedIndices] = useState(new Set());
        const [copyStatus, setCopyStatus] = useState(null); // null, 'copied', 'error'
        
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
                                fileId: file.ID,
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
        
        // Count issues by severity
        const criticalCount = issues.filter(i => (i.severity || '').toLowerCase() === 'critical').length;
        const errorCount = issues.filter(i => (i.severity || '').toLowerCase() === 'error').length;
        const warningCount = issues.filter(i => (i.severity || '').toLowerCase() === 'warning').length;
        const infoCount = issues.filter(i => (i.severity || '').toLowerCase() === 'info').length;
        
        // Check if severity matches any active filter
        const filterMatches = useCallback((severity) => {
            if (activeFilters.size === 0) return false;
            const sev = (severity || '').toLowerCase();
            return activeFilters.has(sev);
        }, [activeFilters]);
        
        // Initialize: select all issues matching default filters (critical + error + warning)
        useEffect(() => {
            const newSelected = new Set();
            issues.forEach((issue, idx) => {
                const sev = (issue.severity || '').toLowerCase();
                if (activeFilters.has(sev)) {
                    newSelected.add(idx);
                }
            });
            setSelectedIndices(newSelected);
        }, [issues.length]);
        
        if (!visible) return null;
        
        // Visible issues based on active filters
        const visibleIssues = issues.filter(issue => filterMatches(issue.severity));
        
        // Toggle a filter on/off
        const toggleFilter = (type) => {
            setActiveFilters(prev => {
                const next = new Set(prev);
                if (next.has(type)) {
                    next.delete(type);
                } else {
                    next.add(type);
                }
                return next;
            });
        };
        
        // Select all visible issues
        const handleSelectAll = () => {
            const newSelected = new Set(selectedIndices);
            issues.forEach((issue, idx) => {
                if (filterMatches(issue.severity)) {
                    newSelected.add(idx);
                }
            });
            setSelectedIndices(newSelected);
        };
        
        // Deselect all visible issues
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
            const selected = issues.filter((issue, idx) => 
                selectedIndices.has(idx) && filterMatches(issue.severity)
            );
            if (selected.length === 0) {
                setCopyStatus('error');
                setTimeout(() => setCopyStatus(null), 2000);
                return;
            }
            
            const lines = selected.map(issue => {
                const lineSuffix = issue.line ? ':' + issue.line : '';
                const sev = issue.severity ? ` (${issue.severity}${issue.category ? ', ' + issue.category : ''})` : '';
                return `${issue.filePath}${lineSuffix} — ${issue.body}${sev}`;
            });
            
            try {
                await copyToClipboard(lines.join('\n'));
                setCopyStatus('copied');
                setTimeout(() => setCopyStatus(null), 2000);
            } catch (err) {
                setCopyStatus('error');
                setTimeout(() => setCopyStatus(null), 2000);
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
        
        // Handle navigation to comment
        const handleNavigate = (commentId, fileId) => {
            if (onNavigate) {
                onNavigate(commentId, fileId);
            }
        };
        
        // Count selected in current filter view
        const selectedInFilter = issues.filter((issue, idx) => 
            selectedIndices.has(idx) && filterMatches(issue.severity)
        ).length;
        const visibleCount = visibleIssues.length;
        
        return html`
            <div class="issues-panel">
                <div class="issues-header">
                    <div class="issues-actions">
                        <div class="severity-filters">
                            <button 
                                class="severity-filter-btn error ${activeFilters.has('error') ? 'active' : ''}"
                                onClick=${() => toggleFilter('error')}
                                title="Toggle error issues"
                            >
                                ERROR
                                <span class="filter-badge">${errorCount}</span>
                            </button>
                            <button 
                                class="severity-filter-btn critical ${activeFilters.has('critical') ? 'active' : ''}"
                                onClick=${() => toggleFilter('critical')}
                                title="Toggle critical issues"
                            >
                                CRITICAL
                                <span class="filter-badge">${criticalCount}</span>
                            </button>
                            <button 
                                class="severity-filter-btn warning ${activeFilters.has('warning') ? 'active' : ''}"
                                onClick=${() => toggleFilter('warning')}
                                title="Toggle warning issues"
                            >
                                WARNING
                                <span class="filter-badge">${warningCount}</span>
                            </button>
                            <button 
                                class="severity-filter-btn info ${activeFilters.has('info') ? 'active' : ''}"
                                onClick=${() => toggleFilter('info')}
                                title="Toggle info issues"
                            >
                                INFO
                                <span class="filter-badge">${infoCount}</span>
                            </button>
                        </div>
                        <button class="action-btn" onClick=${handleSelectAll} title="Select all visible issues">Select All</button>
                        <button class="action-btn" onClick=${handleDeselectAll} title="Deselect all visible issues">Deselect All</button>
                        <button class="btn btn-primary copy-issues-btn ${copyStatus === 'copied' ? 'copied' : ''} ${copyStatus === 'error' ? 'error-state' : ''}" onClick=${handleCopy}>
                            ${copyStatus === 'copied' ? html`
                                <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
                                </svg>
                                Copied!
                            ` : copyStatus === 'error' ? html`
                                <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                                </svg>
                                None Selected
                            ` : html`
                                <svg width="14" height="14" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                                </svg>
                                Copy Selected
                            `}
                        </button>
                        <span class="issues-count">${selectedInFilter} of ${visibleCount} selected</span>
                    </div>
                    <button class="issues-close-btn" onClick=${onClose} title="Close panel">
                        <svg width="16" height="16" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                        </svg>
                    </button>
                </div>
                <div class="issues-list">
                    ${visibleIssues.length === 0 ? html`
                        <div class="issues-empty">
                            ${activeFilters.size === 0 
                                ? 'Select at least one category filter above'
                                : 'No issues match the selected filters'
                            }
                        </div>
                    ` : issues.map((issue, idx) => {
                        const hidden = !filterMatches(issue.severity);
                        if (hidden) return null;
                        return html`
                            <div class="issue-item" data-severity="${(issue.severity || '').toLowerCase()}">
                                <input 
                                    type="checkbox"
                                    checked=${selectedIndices.has(idx)}
                                    onChange=${() => toggleSelected(idx)}
                                />
                                <div class="issue-content">
                                    <div class="issue-path">
                                        ${issue.filePath}${issue.line ? ':' + issue.line : ''}
                                    </div>
                                    <div class="issue-message">
                                        ${issue.body}${issue.severity ? ` (${issue.severity}${issue.category ? ', ' + issue.category : ''})` : ''}
                                    </div>
                                </div>
                                <button 
                                    class="issue-nav-btn"
                                    type="button"
                                    title="Go to comment"
                                    onClick=${() => handleNavigate(issue.commentId, issue.fileId)}
                                >
                                    <svg width="16" height="16" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14 5l7 7m0 0l-7 7m7-7H3" />
                                    </svg>
                                </button>
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
