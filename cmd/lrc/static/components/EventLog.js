// EventLog component - displays review progress events
import { waitForPreact, formatTime } from './utils.js';

export async function createEventLog() {
    const { html, useRef, useEffect } = await waitForPreact();
    
    return function EventLog({ events, status, isTailing, listRef }) {
        // Scroll to bottom when tailing is enabled or when new events arrive while tailing
        useEffect(() => {
            if (isTailing && listRef?.current) {
                listRef.current.scrollTop = listRef.current.scrollHeight;
            }
        }, [events, isTailing, listRef]);
        
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
