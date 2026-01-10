// Header component
import { waitForPreact, LOGO_DATA_URI } from './utils.js';

export async function createHeader() {
    const { html } = await waitForPreact();
    
    return function Header({ generatedTime, friendlyName }) {
        return html`
            <div class="header">
                <div class="brand">
                    <div class="logo-wrap">
                        <img alt="LiveReview" src="${LOGO_DATA_URI}" />
                    </div>
                    <div class="brand-text">
                        <h1>LiveReview Results</h1>
                        <div class="meta">Generated: ${generatedTime}</div>
                        ${friendlyName && html`
                            <div class="run-name-pill">
                                <span class="dot"></span>
                                Run: ${friendlyName}
                            </div>
                        `}
                    </div>
                </div>
            </div>
        `;
    };
}

// Export a loader that will be resolved when preact is ready
let HeaderComponent = null;
export async function getHeader() {
    if (!HeaderComponent) {
        HeaderComponent = await createHeader();
    }
    return HeaderComponent;
}
