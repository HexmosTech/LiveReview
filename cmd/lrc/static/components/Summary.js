// Summary component - renders markdown summary
import { waitForPreact } from './utils.js';

export async function createSummary() {
    const { html, useEffect, useRef } = await waitForPreact();
    
    return function Summary({ markdown, status, errorSummary }) {
        const contentRef = useRef(null);
        
        useEffect(() => {
            if (contentRef.current && markdown && typeof marked !== 'undefined') {
                contentRef.current.innerHTML = marked.parse(markdown);
            }
        }, [markdown]);
        
        const isError = status === 'failed' || errorSummary;
        
        return html`
            <div class="summary" id="summary-content">
                ${isError && html`
                    <div style="padding: 16px; background: #fef2f2; border: 1px solid #fecaca; border-radius: 6px; color: #991b1b; margin-bottom: 16px;">
                        <strong style="display: block; margin-bottom: 8px; font-size: 16px;">⚠️ Error Details:</strong>
                        <pre style="white-space: pre-wrap; font-family: monospace; font-size: 13px; margin: 0;">
                            ${errorSummary || 'Review failed'}
                        </pre>
                    </div>
                `}
                <div ref=${contentRef}></div>
            </div>
        `;
    };
}

let SummaryComponent = null;
export async function getSummary() {
    if (!SummaryComponent) {
        SummaryComponent = await createSummary();
    }
    return SummaryComponent;
}
