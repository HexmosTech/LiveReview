// PrecommitBar component - commit/push/skip actions
import { waitForPreact } from './utils.js';

export async function createPrecommitBar() {
    const { html, useState } = await waitForPreact();
    
    return function PrecommitBar({ interactive, isPostCommitReview, initialMsg }) {
        const [message, setMessage] = useState(initialMsg || '');
        const [status, setStatus] = useState('');
        const [disabled, setDisabled] = useState(false);
        
        if (!interactive) return null;
        
        // Post-commit review mode - just show info
        if (isPostCommitReview) {
            return html`
                <div class="precommit-bar">
                    <div style="padding: 16px; color: #9ca3af;">
                        <p>📖 Viewing historical commit review. Press <strong>Ctrl-C</strong> in the terminal to exit.</p>
                    </div>
                </div>
            `;
        }
        
        const postDecision = async (path, successText, requireMessage) => {
            if (requireMessage && !message.trim()) {
                setStatus('Commit message is required');
                return;
            }
            
            setDisabled(true);
            setStatus('Sending decision...');
            
            try {
                const res = await fetch(path, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ message })
                });
                
                if (!res.ok) throw new Error('Request failed: ' + res.status);
                setStatus(successText + ' — you can now return to the terminal.');
            } catch (err) {
                setStatus('Failed: ' + err.message);
                setDisabled(false);
            }
        };
        
        return html`
            <div class="precommit-bar">
                <div class="precommit-bar-left">
                    <div class="precommit-bar-title">Pre-commit action</div>
                    <div class="precommit-actions">
                        <button 
                            class="btn-primary"
                            disabled=${disabled}
                            onClick=${() => postDecision('/commit', 'Commit requested', true)}
                        >
                            Commit
                        </button>
                        <button 
                            class="btn-primary"
                            disabled=${disabled}
                            onClick=${() => postDecision('/commit-push', 'Commit and push requested', true)}
                        >
                            Commit and Push
                        </button>
                        <button 
                            class="btn-ghost"
                            disabled=${disabled}
                            onClick=${() => postDecision('/skip', 'Skip requested', false)}
                        >
                            Skip Commit
                        </button>
                    </div>
                    <div class="precommit-status">${status}</div>
                </div>
                <div class="precommit-message">
                    <label for="commit-message">Commit message</label>
                    <textarea 
                        id="commit-message"
                        placeholder="Enter your commit message (required)"
                        value=${message}
                        disabled=${disabled}
                        onInput=${(e) => setMessage(e.target.value)}
                    ></textarea>
                    <div class="precommit-message-hint">Required for commit actions; ignored on Skip.</div>
                </div>
            </div>
        `;
    };
}

let PrecommitBarComponent = null;
export async function getPrecommitBar() {
    if (!PrecommitBarComponent) {
        PrecommitBarComponent = await createPrecommitBar();
    }
    return PrecommitBarComponent;
}
