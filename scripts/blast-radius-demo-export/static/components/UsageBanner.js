import { normalizeUsagePayload } from './usage_chip_model.js';
import { renderIcon } from './icons.js';

const { html, useEffect, useState } = window.preact;

const SESSION_REVIEW_ID = new URLSearchParams(window.location.search).get('r') || '';

function withSession(endpoint) {
    if (!SESSION_REVIEW_ID) return endpoint;
    const sep = endpoint.includes('?') ? '&' : '?';
    return `${endpoint}${sep}r=${SESSION_REVIEW_ID}`;
}

export function UsageBanner({ endpoint }) {
    const [chip, setChip] = useState(null);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        let cancelled = false;
        const load = async () => {
            try {
                const response = await fetch(withSession(endpoint));
                if (!response.ok) return;
                const data = await response.json();
                if (!cancelled) {
                    setChip(normalizeUsagePayload(data));
                }
            } catch (err) {
                console.error('Failed to fetch usage for banner:', err);
            } finally {
                if (!cancelled) setLoading(false);
            }
        };
        load();
        return () => { cancelled = true; };
    }, [endpoint]);

    if (loading || !chip || !chip.available) return '';

    const upgradeURL = `${chip.cloudURL}/#/settings-subscriptions-overview`;

    if (chip.blocked || chip.usagePct >= 100) {
        const limitStr = chip.locLimit > 0 ? chip.locLimit.toLocaleString() : 'N/A';

        return html`
            <div class="quota-banner-slate">
                <div class="qbs-flex">
                    <div class="qbs-icon-wrap">
                        ${renderIcon(html, 'issueWarning', { size: 20 })}
                    </div>
                    <div class="qbs-content">
                        <p class="qbs-title">You've reached your monthly limit</p>
                        <p class="qbs-text">
                            Your team used all <strong>${limitStr} LOC</strong> this month. 
                            Upgrade to a higher tier and continue reviewing code without any interruption to your workflow.
                        </p>
                        <a href="${upgradeURL}" target="_blank" class="qbs-btn">
                            Upgrade plan
                        </a>
                    </div>
                </div>
            </div>
        `;
    }

    if (chip.usagePct >= 90) {
        return html`
            <div class="main-alert main-alert-warn">
                <div class="main-alert-content">
                    <div class="main-alert-text">
                        <span class="main-alert-title">${renderIcon(html, 'issueWarning', { className: 'btn-icon', size: 14 })}LOC Usage Nearing Limit</span>
                        <span class="main-alert-sub">
                            You've used ${chip.locUsed.toLocaleString()} of ${chip.locLimit > 0 ? chip.locLimit.toLocaleString() : 'N/A'} LOC (${chip.usagePct}%). Upgrade to avoid interruption.
                        </span>
                    </div>
                    <a href="${upgradeURL}" target="_blank" class="main-alert-btn main-alert-btn-warn">
                        Upgrade Plan
                    </a>
                </div>
            </div>
        `;
    }

    return '';
}
