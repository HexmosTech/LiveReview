// EventLog component - displays review progress events
import { waitForPreact, formatTime, copyToClipboard } from './utils.js';

export async function createEventLog() {
    const { html, useState, useRef } = await waitForPreact();
    
    return function EventLog({ events, status }) {
        const [autoScroll, setAutoScroll] = useState(true);
        const [copied, setCopied] = useState(false);
        const listRef = useRef(null);
        
        const handleCopyLogs = async () => {
            const logsText = events.map((event, index) => {
                const time = formatTime(event.time);
                const type = event.type ? event.type.toUpperCase() : 'LOG';
                return `[${index + 1}] ${time} - ${type}\n  ${event.message}`;
            }).join('\n\n');
            
            try {
                await copyToClipboard(logsText);
                setCopied(true);
                setTimeout(() => setCopied(false), 2000);
            } catch (err) {
                console.error('Failed to copy logs:', err);
            }
        };
        
        const getEventBadge = (event) => {
            if (event.type === 'batch') {
                return html`<span class="event-type batch">BATCH</span>`;
            } else if (event.type === 'completion') {
                return html`<span class="event-type completion">COMPLETE</span>`;
            } else if (event.level === 'error') {
                return html`<span class="event-type error">ERROR</span>`;
            }
            return null;
        };
        
        const getStatusText = () => {
            if (status === 'completed') return '✅ Review completed successfully';
            if (status === 'failed') return '❌ Review completed with errors';
            if (events.length > 0) return `${events.length} events received`;
            return 'Waiting for events...';
        };
        
        return html`
            <div class="events-container">
                <div class="events-header">
                    <div>
                        <h3>Review Progress</h3>
                        <div class="events-status">${getStatusText()}</div>
                    </div>
                    <div class="events-controls">
                        <label class="auto-scroll-label">
                            <input 
                                type="checkbox" 
                                checked=${autoScroll}
                                onChange=${(e) => setAutoScroll(e.target.checked)}
                            />
                            <span>Auto-scroll</span>
                        </label>
                        <button 
                            class="copy-logs-btn"
                            title="Copy all logs to clipboard"
                            onClick=${handleCopyLogs}
                        >
                            <svg width="16" height="16" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                            </svg>
                            ${copied ? 'Copied!' : 'Copy Logs'}
                        </button>
                    </div>
                </div>
                <div class="events-list" ref=${listRef}>
                    ${events.map(event => html`
                        <div class="event-item" data-event-id="${event.id}" data-event-type="${event.type || 'log'}">
                            <span class="event-time">${formatTime(event.time)}</span>
                            <span class="event-message">
                                ${getEventBadge(event)}
                                ${event.message}
                            </span>
                        </div>
                    `)}
                </div>
            </div>
        `;
    };
}

let EventLogComponent = null;
export async function getEventLog() {
    if (!EventLogComponent) {
        EventLogComponent = await createEventLog();
    }
    return EventLogComponent;
}
