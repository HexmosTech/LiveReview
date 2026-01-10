// LiveReview App - Main Entry Point
// Fetches data from /api/review and updates reactively

import { waitForPreact, filePathToId, transformEvent, getBadgeClass } from './components/utils.js';
import { getHeader } from './components/Header.js';
import { getSidebar } from './components/Sidebar.js';
import { getSummary } from './components/Summary.js';
import { getStats } from './components/Stats.js';
import { getPrecommitBar } from './components/PrecommitBar.js';
import { getFileBlock } from './components/FileBlock.js';
import { getEventLog } from './components/EventLog.js';
import { getIssuesPanel } from './components/IssuesPanel.js';
import { getToolbar } from './components/Toolbar.js';

// Convert API response to UI data format
// Backend uses snake_case JSON keys (file_path, old_start_line, etc.)
function convertFilesToUIFormat(files) {
    if (!files) return [];
    
    return files.map(file => {
        // Handle snake_case from backend
        const filePath = file.file_path || file.filePath || file.FilePath || '';
        // Use same ID generation as filePathToId in utils.js
        const fileId = 'file_' + filePath.replace(/[^a-zA-Z0-9]/g, '_');
        const comments = file.comments || file.Comments || [];
        const hunks = file.hunks || file.Hunks || [];
        
        // Build comment lookup by line
        const commentsByLine = {};
        comments.forEach(comment => {
            const line = comment.line || comment.Line;
            if (!commentsByLine[line]) {
                commentsByLine[line] = [];
            }
            commentsByLine[line].push({
                Severity: (comment.severity || comment.Severity || 'info').toUpperCase(),
                BadgeClass: getBadgeClass(comment.severity || comment.Severity || 'info'),
                Category: comment.category || comment.Category || '',
                Content: comment.content || comment.Content || '',
                HasCategory: !!(comment.category || comment.Category),
                Line: line,
                FilePath: filePath
            });
        });
        
        // Process hunks
        const processedHunks = hunks.map(hunk => {
            // Handle snake_case keys
            const oldStartLine = hunk.old_start_line || hunk.oldStartLine || hunk.OldStartLine || 1;
            const oldLineCount = hunk.old_line_count || hunk.oldLineCount || hunk.OldLineCount || 0;
            const newStartLine = hunk.new_start_line || hunk.newStartLine || hunk.NewStartLine || 1;
            const newLineCount = hunk.new_line_count || hunk.newLineCount || hunk.NewLineCount || 0;
            const header = hunk.header || hunk.Header || 
                `@@ -${oldStartLine},${oldLineCount} +${newStartLine},${newLineCount} @@`;
            
            // If hunk already has Lines array (pre-processed), use it
            if (hunk.Lines) {
                // Merge comments into existing lines
                const lines = hunk.Lines.map(line => {
                    const newNum = parseInt(line.NewNum) || 0;
                    if (newNum && commentsByLine[newNum]) {
                        return {
                            ...line,
                            IsComment: true,
                            Comments: commentsByLine[newNum]
                        };
                    }
                    return line;
                });
                return { Header: header, Lines: lines };
            }
            
            // Parse hunk content into lines
            const content = hunk.content || hunk.Content || '';
            const contentLines = content.split('\n');
            let oldLine = oldStartLine;
            let newLine = newStartLine;
            
            const lines = [];
            for (const line of contentLines) {
                if (!line || line.startsWith('@@')) continue;
                
                let lineData;
                if (line.startsWith('-')) {
                    lineData = {
                        OldNum: String(oldLine),
                        NewNum: '',
                        Content: line,
                        Class: 'diff-del',
                        IsComment: false,
                        Comments: []
                    };
                    oldLine++;
                } else if (line.startsWith('+')) {
                    const lineComments = commentsByLine[newLine] || [];
                    lineData = {
                        OldNum: '',
                        NewNum: String(newLine),
                        Content: line,
                        Class: 'diff-add',
                        IsComment: lineComments.length > 0,
                        Comments: lineComments
                    };
                    newLine++;
                } else {
                    lineData = {
                        OldNum: String(oldLine),
                        NewNum: String(newLine),
                        Content: ' ' + line,
                        Class: 'diff-context',
                        IsComment: false,
                        Comments: []
                    };
                    oldLine++;
                    newLine++;
                }
                lines.push(lineData);
            }
            
            return { Header: header, Lines: lines };
        });
        
        return {
            ID: fileId,
            FilePath: filePath,
            HasComments: comments.length > 0,
            CommentCount: comments.length,
            Hunks: processedHunks
        };
    });
}

async function initApp() {
    const { h, render, useState, useEffect, useCallback, useRef, html } = await waitForPreact();
    
    // Load all components
    const Header = await getHeader();
    const Sidebar = await getSidebar();
    const Summary = await getSummary();
    const Stats = await getStats();
    const PrecommitBar = await getPrecommitBar();
    const FileBlock = await getFileBlock();
    const EventLog = await getEventLog();
    const IssuesPanel = await getIssuesPanel();
    const Toolbar = await getToolbar();
    
    function App() {
        // Core data state - fetched from API
        const [reviewData, setReviewData] = useState(null);
        const [loading, setLoading] = useState(true);
        const [error, setError] = useState(null);
        
        // UI state
        const [activeTab, setActiveTab] = useState('files');
        const [expandedFiles, setExpandedFiles] = useState(new Set());
        const [allExpanded, setAllExpanded] = useState(false);
        const [activeFileId, setActiveFileId] = useState(null);
        const [issuesPanelVisible, setIssuesPanelVisible] = useState(false);
        const [events, setEvents] = useState([]);
        const [newEventCount, setNewEventCount] = useState(0);
        
        const pollingRef = useRef(null);
        const eventsPollingRef = useRef(null);
        
        // Fetch review data from API
        const fetchReviewData = useCallback(async () => {
            try {
                const response = await fetch('/api/review');
                if (!response.ok) {
                    throw new Error(`Failed to fetch review data: ${response.status}`);
                }
                const data = await response.json();
                
                // Convert files to UI format
                const uiFiles = convertFilesToUIFormat(data.files);
                
                setReviewData(prev => {
                    // Auto-expand files with comments on first load
                    if (!prev) {
                        const expanded = new Set();
                        uiFiles.forEach(file => {
                            if (file.HasComments) {
                                expanded.add(file.ID);
                            }
                        });
                        if (expanded.size > 0) {
                            setExpandedFiles(expanded);
                        }
                    }
                    
                    return {
                        ...data,
                        Files: uiFiles,
                        TotalFiles: uiFiles.length,
                        TotalComments: data.totalComments || 0
                    };
                });
                
                setLoading(false);
                return data;
            } catch (err) {
                console.error('Error fetching review data:', err);
                setError(err.message);
                setLoading(false);
                return null;
            }
        }, []);
        
        // Fetch events for the event log
        const fetchEvents = useCallback(async (reviewID) => {
            if (!reviewID) return;
            
            try {
                const response = await fetch(`/api/v1/diff-review/${reviewID}/events?limit=1000`);
                if (!response.ok) return;
                
                const data = await response.json();
                const backendEvents = data.events || [];
                const transformedEvents = backendEvents.map(transformEvent);
                
                setEvents(prev => {
                    if (transformedEvents.length > prev.length) {
                        const addedCount = transformedEvents.length - prev.length;
                        if (activeTab !== 'events') {
                            setNewEventCount(count => count + addedCount);
                        }
                    }
                    return transformedEvents;
                });
            } catch (err) {
                console.error('Error fetching events:', err);
            }
        }, [activeTab]);
        
        // Initial load and polling setup
        useEffect(() => {
            // Initial fetch
            fetchReviewData().then(data => {
                if (data?.reviewID) {
                    fetchEvents(data.reviewID);
                }
            });
            
            // Poll for updates every 2 seconds
            pollingRef.current = setInterval(async () => {
                const data = await fetchReviewData();
                if (data?.reviewID) {
                    fetchEvents(data.reviewID);
                }
                
                // Stop polling when review is complete
                if (data?.status === 'completed' || data?.status === 'failed') {
                    if (pollingRef.current) {
                        clearInterval(pollingRef.current);
                        pollingRef.current = null;
                    }
                }
            }, 2000);
            
            // Cleanup
            return () => {
                if (pollingRef.current) {
                    clearInterval(pollingRef.current);
                }
            };
        }, [fetchReviewData, fetchEvents]);
        
        // Update page title with friendly name
        useEffect(() => {
            if (reviewData?.friendlyName) {
                document.title = `LiveReview - ${reviewData.friendlyName}`;
            } else {
                document.title = 'LiveReview';
            }
        }, [reviewData?.friendlyName]);
        
        // Toggle single file
        const toggleFile = useCallback((fileId) => {
            setExpandedFiles(prev => {
                const next = new Set(prev);
                if (next.has(fileId)) {
                    next.delete(fileId);
                } else {
                    next.add(fileId);
                }
                return next;
            });
        }, []);
        
        // Toggle all files
        const toggleAll = useCallback(() => {
            if (allExpanded) {
                setExpandedFiles(new Set());
                setAllExpanded(false);
            } else {
                const all = new Set();
                (reviewData?.Files || []).forEach(file => {
                    all.add(file.ID);
                });
                setExpandedFiles(all);
                setAllExpanded(true);
            }
        }, [allExpanded, reviewData?.Files]);
        
        // Handle sidebar file click
        const handleFileClick = useCallback((fileId) => {
            setActiveFileId(fileId);
            setExpandedFiles(prev => {
                const next = new Set(prev);
                next.add(fileId);
                return next;
            });
            
            // Scroll to file
            setTimeout(() => {
                const fileEl = document.getElementById(fileId);
                if (fileEl) {
                    const mainContent = document.querySelector('.main-content');
                    const header = document.querySelector('.header');
                    const headerHeight = header ? header.offsetHeight : 60;
                    const fileRect = fileEl.getBoundingClientRect();
                    const mainContentRect = mainContent.getBoundingClientRect();
                    const scrollTarget = mainContent.scrollTop + fileRect.top - mainContentRect.top - headerHeight - 10;
                    mainContent.scrollTo({ top: scrollTarget, behavior: 'smooth' });
                }
            }, 50);
        }, []);
        
        // Navigate to comment
        const navigateToComment = useCallback((commentId, fileId) => {
            setExpandedFiles(prev => {
                const next = new Set(prev);
                next.add(fileId);
                return next;
            });
            
            setTimeout(() => {
                const comment = document.getElementById(commentId);
                if (comment) {
                    const mainContent = document.querySelector('.main-content');
                    const header = document.querySelector('.header');
                    const headerHeight = header ? header.offsetHeight : 60;
                    const commentRect = comment.getBoundingClientRect();
                    const mainContentRect = mainContent.getBoundingClientRect();
                    const scrollTarget = mainContent.scrollTop + commentRect.top - mainContentRect.top - headerHeight - 20;
                    mainContent.scrollTo({ top: scrollTarget, behavior: 'smooth' });
                    
                    comment.classList.add('highlight');
                    setTimeout(() => comment.classList.remove('highlight'), 1500);
                }
            }, 100);
        }, []);
        
        // Tab change
        const handleTabChange = useCallback((tab) => {
            setActiveTab(tab);
            if (tab === 'events') {
                setNewEventCount(0);
            }
        }, []);
        
        // Loading state
        if (loading && !reviewData) {
            return html`
                <div style="display: flex; align-items: center; justify-content: center; height: 100vh; color: #9ca3af;">
                    <div class="spinner" style="width: 24px; height: 24px; border: 3px solid #374151; border-top-color: #3b82f6; border-radius: 50%; animation: spin 1s linear infinite; margin-right: 12px;"></div>
                    Loading LiveReview...
                </div>
            `;
        }
        
        // Error state
        if (error && !reviewData) {
            return html`
                <div style="display: flex; align-items: center; justify-content: center; height: 100vh; color: #ef4444;">
                    <div>
                        <h2>Error Loading Review</h2>
                        <p>${error}</p>
                    </div>
                </div>
            `;
        }
        
        const status = reviewData?.status || 'in_progress';
        const showLoader = status === 'in_progress';
        const summary = reviewData?.summary || '';
        const totalComments = reviewData?.totalComments || reviewData?.TotalComments || 0;
        const files = reviewData?.Files || [];
        
        // Status display
        const getStatusDisplay = () => {
            if (status === 'failed') {
                return html`
                    <div class="status-container error">
                        <span class="status-icon">❌</span>
                        <span>Review completed with errors</span>
                    </div>
                `;
            }
            if (status === 'completed') {
                return html`
                    <div class="status-container success">
                        <span class="status-icon">✅</span>
                        <span>Review completed successfully</span>
                    </div>
                `;
            }
            return null;
        };
        
        return html`
            <${Sidebar} 
                files=${files}
                totalComments=${totalComments}
                activeFileId=${activeFileId}
                onFileClick=${handleFileClick}
            />
            <div class="main-content">
                <div class="container">
                    <${Header} 
                        generatedTime=${reviewData?.generatedTime || reviewData?.GeneratedTime}
                        friendlyName=${reviewData?.friendlyName || reviewData?.FriendlyName}
                    />
                    
                    ${showLoader && html`
                        <div class="loader-container">
                            <div class="loader-content">
                                <div class="spinner"></div>
                                <span class="loader-message">Review in progress...</span>
                            </div>
                        </div>
                    `}
                    
                    ${getStatusDisplay()}
                    
                    ${summary && summary.trim() && status !== 'in_progress' && html`
                        <${Summary} 
                            markdown=${summary}
                            status=${status}
                            errorSummary=${reviewData?.errorSummary || ''}
                        />
                    `}
                    
                    <${Stats} 
                        totalFiles=${files.length}
                        totalComments=${totalComments}
                    />
                    
                    <${PrecommitBar}
                        interactive=${reviewData?.interactive || reviewData?.Interactive}
                        isPostCommitReview=${reviewData?.isPostCommitReview || reviewData?.IsPostCommitReview}
                        initialMsg=${reviewData?.initialMsg || reviewData?.InitialMsg || ''}
                    />
                    
                    <${Toolbar}
                        activeTab=${activeTab}
                        onTabChange=${handleTabChange}
                        allExpanded=${allExpanded}
                        onToggleAll=${toggleAll}
                        onCopyIssues=${() => setIssuesPanelVisible(!issuesPanelVisible)}
                        eventCount=${newEventCount}
                        showEventBadge=${activeTab !== 'events'}
                    />
                    
                    <div class="issues-toolbar">
                        <${IssuesPanel}
                            files=${files}
                            visible=${issuesPanelVisible}
                            onNavigate=${navigateToComment}
                        />
                    </div>
                    
                    <!-- Files Tab -->
                    <div id="files-tab" class="tab-content ${activeTab === 'files' ? 'active' : ''}" style="display: ${activeTab === 'files' ? 'block' : 'none'}">
                        ${files.length > 0 
                            ? files.map(file => html`
                                <${FileBlock}
                                    key=${file.ID}
                                    file=${file}
                                    expanded=${expandedFiles.has(file.ID)}
                                    onToggle=${toggleFile}
                                />
                            `)
                            : html`
                                <div style="padding: 40px 20px; text-align: center; color: #57606a;">
                                    ${status === 'in_progress' 
                                        ? 'Waiting for review results...' 
                                        : 'No files reviewed or no comments generated.'}
                                </div>
                            `
                        }
                    </div>
                    
                    <!-- Events Tab -->
                    <div id="events-tab" class="tab-content ${activeTab === 'events' ? 'active' : ''}" style="display: ${activeTab === 'events' ? 'block' : 'none'}">
                        <${EventLog}
                            events=${events}
                            status=${status}
                        />
                    </div>
                    
                    <div class="footer">
                        ${status === 'in_progress' 
                            ? `Review in progress: ${totalComments} comment(s) so far`
                            : `Review complete: ${totalComments} total comment(s)`
                        }
                    </div>
                </div>
            </div>
        `;
    }
    
    // Render the app
    render(html`<${App} />`, document.getElementById('app'));
}

// Initialize when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initApp);
} else {
    initApp();
}
