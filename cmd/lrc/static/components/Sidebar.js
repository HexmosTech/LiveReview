// Sidebar component
import { waitForPreact, filePathToId } from './utils.js';

export async function createSidebar() {
    const { html } = await waitForPreact();
    
    return function Sidebar({ files, totalComments, activeFileId, onFileClick }) {
        const totalFiles = files.length;
        
        return html`
            <div class="sidebar">
                <div class="sidebar-header">
                    <h2>📂 Files</h2>
                    <div class="sidebar-stats">
                        ${totalFiles} files • ${totalComments} comments
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
                                ${file.HasComments && html`
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
