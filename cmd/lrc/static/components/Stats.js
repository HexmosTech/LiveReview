// Stats component
import { waitForPreact } from './utils.js';

export async function createStats() {
    const { html } = await waitForPreact();
    
    return function Stats({ totalFiles, totalComments }) {
        return html`
            <div class="stats">
                <div class="stat">Files: <span class="count">${totalFiles}</span></div>
                <div class="stat">Comments: <span class="count">${totalComments}</span></div>
            </div>
        `;
    };
}

let StatsComponent = null;
export async function getStats() {
    if (!StatsComponent) {
        StatsComponent = await createStats();
    }
    return StatsComponent;
}
