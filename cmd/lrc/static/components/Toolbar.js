// Toolbar component - tabs and action buttons
import { waitForPreact } from './utils.js';

export async function createToolbar() {
    const { html } = await waitForPreact();
    
    return function Toolbar({ 
        activeTab, 
        onTabChange, 
        allExpanded, 
        onToggleAll, 
        onCopyIssues,
        eventCount,
        showEventBadge 
    }) {
        return html`
            <div class="toolbar-row">
                <div class="view-tabs">
                    <button 
                        class="tab-btn ${activeTab === 'files' ? 'active' : ''}"
                        data-tab="files"
                        onClick=${() => onTabChange('files')}
                    >
                        📁 Files & Comments
                    </button>
                    <button 
                        class="tab-btn ${activeTab === 'events' ? 'active' : ''}"
                        data-tab="events"
                        onClick=${() => onTabChange('events')}
                    >
                        📊 Event Log
                        ${showEventBadge && eventCount > 0 && html`
                            <span class="notification-badge">${eventCount}</span>
                        `}
                    </button>
                </div>
                <button class="expand-all" onClick=${onToggleAll}>
                    ${allExpanded ? 'Collapse All Files' : 'Expand All Files'}
                </button>
                <button class="copy-issues-btn" onClick=${onCopyIssues}>
                    Copy Issues
                </button>
            </div>
        `;
    };
}

let ToolbarComponent = null;
export async function getToolbar() {
    if (!ToolbarComponent) {
        ToolbarComponent = await createToolbar();
    }
    return ToolbarComponent;
}
