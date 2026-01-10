// Sidebar component
import { waitForPreact, filePathToId } from './utils.js';

export async function createSidebar() {
    const { html } = await waitForPreact();
    
    return function Sidebar({ files, activeFileId, onFileClick }) {
        const totalFiles = files.length;
        // Calculate total comments from actual file data
        const totalComments = files.reduce((sum, file) => sum + (file.CommentCount || 0), 0);
        
        return html`
            <div class="sidebar">
                <div class="sidebar-header">
                    <h2>📂 FILES</h2>
                    <div class="sidebar-stats">
                        ${totalFiles} file${totalFiles !== 1 ? 's' : ''} • ${totalComments} comment${totalComments !== 1 ? 's' : ''}
                    </div>
                </div>
                <div class="sidebar-content">
                    ${files.map(file => {
                        const fileId = filePathToId(file.FilePath);
                        const isActive = activeFileId === fileId;
                        
                        return html`
                            <div 
                                class="sidebar-file ${isActive ? 'active' : ''}"
                                data-file-id="${fileId}"
                                onClick=${() => onFileClick(fileId)}
                            >
                                <span class="sidebar-file-name" title="${file.FilePath}">
                                    ${file.FilePath}
                                </span>
                                ${file.CommentCount > 0 && html`
                                    <span class="sidebar-file-badge">${file.CommentCount}</span>
                                `}
                            </div>
                        `;
                    })}
                </div>
            </div>
        `;
    };
}

let SidebarComponent = null;
export async function getSidebar() {
    if (!SidebarComponent) {
        SidebarComponent = await createSidebar();
    }
    return SidebarComponent;
}
