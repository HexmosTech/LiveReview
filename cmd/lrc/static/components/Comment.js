// Comment component
import { waitForPreact, getBadgeClass, copyToClipboard } from './utils.js';

export async function createComment() {
    const { html, useState } = await waitForPreact();
    
    return function Comment({ comment, filePath, codeExcerpt }) {
        const [copied, setCopied] = useState(false);
        
        const handleCopy = async (e) => {
            e.stopPropagation();
            
            let copyText = '';
            if (filePath) {
                copyText += filePath;
                if (comment.Line) {
                    copyText += ':' + comment.Line;
                }
                copyText += '\n\n';
            }
            
            if (codeExcerpt) {
                copyText += 'Code excerpt:\n  ' + codeExcerpt + '\n\n';
            }
            
            copyText += 'Issue:\n' + comment.Content;
            
            try {
                await copyToClipboard(copyText);
                setCopied(true);
                setTimeout(() => setCopied(false), 2000);
            } catch (err) {
                console.error('Copy failed:', err);
            }
        };
        
        const badgeClass = getBadgeClass(comment.Severity);
        
        return html`
            <tr class="comment-row" data-line="${comment.Line}">
                <td colspan="3">
                    <div 
                        class="comment-container"
                        data-filepath="${filePath}"
                        data-line="${comment.Line}"
                        data-comment="${comment.Content}"
                    >
                        <button 
                            class="comment-copy-btn ${copied ? 'copied' : ''}"
                            title="Copy issue details"
                            onClick=${handleCopy}
                        >
                            ${copied ? 'Copied!' : 'Copy'}
                        </button>
                        <div class="comment-header">
                            <span class="comment-badge ${badgeClass}">${comment.Severity}</span>
                            ${comment.HasCategory && html`
                                <span class="comment-category">${comment.Category}</span>
                            `}
                        </div>
                        <div class="comment-body">${comment.Content}</div>
                    </div>
                </td>
            </tr>
        `;
    };
}

let CommentComponent = null;
export async function getComment() {
    if (!CommentComponent) {
        CommentComponent = await createComment();
    }
    return CommentComponent;
}
