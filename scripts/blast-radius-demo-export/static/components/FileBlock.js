// FileBlock component - collapsible file with diff
import { waitForPreact, filePathToId } from './utils.js';
import { countFileVisibleIssues } from './issue_filter_state.mjs';
import { getDiffTable } from './DiffTable.js';

export async function createFileBlock() {
    const { html } = await waitForPreact();
    const DiffTable = await getDiffTable();
    
    return function FileBlock({
        file,
        expanded,
        onToggle,
        issueFilters,
        hiddenCommentKeys,
        onToggleCommentVisibility,
        reviewStartMs,
        commentRenderTimes,
        onCommentRendered,
        commentVotes,
        onVote
    }) {
        // Use file.ID if available (set by convertFilesToUIFormat), otherwise generate
        const fileId = file.ID || filePathToId(file.FilePath);
        
        const visibleCount = countFileVisibleIssues(file, issueFilters, hiddenCommentKeys);
        
        return html`
            <div 
                class="file ${expanded ? 'expanded' : 'collapsed'}"
                id="${fileId}"
                data-has-comments="${file.HasComments}"
                data-filepath="${file.FilePath}"
            >
                <div class="file-header" onClick=${() => onToggle(fileId)}>
                    <span class="toggle"></span>
                    <span class="filename">${file.FilePath}</span>
                    ${visibleCount > 0 && html`
                        <span class="comment-count">${visibleCount}</span>
                    `}
                </div>
                <div class="file-content">
                    <${DiffTable}
                        hunks=${file.Hunks}
                        filePath=${file.FilePath}
                        fileId=${fileId}
                        issueFilters=${issueFilters}
                        hiddenCommentKeys=${hiddenCommentKeys}
                        onToggleCommentVisibility=${onToggleCommentVisibility}
                        reviewStartMs=${reviewStartMs}
                        commentRenderTimes=${commentRenderTimes}
                        onCommentRendered=${onCommentRendered}
                        commentVotes=${commentVotes}
                        onVote=${onVote}
                    />
                </div>
            </div>
        `;
    };
}

let FileBlockComponent = null;
export async function getFileBlock() {
    if (!FileBlockComponent) {
        FileBlockComponent = await createFileBlock();
    }
    return FileBlockComponent;
}
