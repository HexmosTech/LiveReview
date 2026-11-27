import React from 'react';
import { isSafeExternalUrl } from '../../pages/Reviews/reviewDetailHelpers';

export interface ToolInfoItem {
	id: number;
	name: string;
	description: string;
	use_case?: string;
	category?: string;
	docs_url?: string;
}

export interface ToolInfoPopoverProps {
	tool: ToolInfoItem;
	openUpward: boolean;
	isHovered: boolean;
	isPinned: boolean;
	onHoverStart: () => void;
	onHoverEnd: () => void;
	onTogglePin: () => void;
}

const ToolInfoPopover: React.FC<ToolInfoPopoverProps> = ({
	tool,
	openUpward,
	isHovered,
	isPinned,
	onHoverStart,
	onHoverEnd,
	onTogglePin,
}) => {
	const isOpen = isHovered || isPinned;
	const hasSafeDocsUrl = isSafeExternalUrl(tool.docs_url);
	const popoverId = `tool-info-popover-${tool.id}`;

	return (
		<div
			className="relative inline-flex items-center"
			data-tool-info-root="true"
			onMouseEnter={onHoverStart}
			onMouseLeave={onHoverEnd}
		>
			<button
				type="button"
				aria-label={`Inspect ${tool.name} details`}
				aria-expanded={isOpen}
				aria-controls={popoverId}
				onClick={(e) => {
					e.stopPropagation();
					onTogglePin();
				}}
				onFocus={onHoverStart}
				onBlur={(e) => {
					const related = e.relatedTarget as Node | null;
					if (!e.currentTarget.parentElement?.contains(related)) {
						onHoverEnd();
					}
				}}
				className={`inline-flex h-5 w-5 items-center justify-center rounded-full border text-[11px] font-semibold transition-colors cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-violet-500 ${
					isOpen
						? 'bg-violet-500/20 text-violet-200 border-violet-400/60'
						: 'bg-slate-800/70 text-slate-400 border-slate-700/80 hover:text-white hover:border-slate-500 group-hover/row:border-slate-500 group-hover/row:text-slate-200'
				}`}
				title={`About ${tool.name}`}
			>
				i
			</button>

			{isOpen && (
				<div
					id={popoverId}
					role="dialog"
					aria-label={`${tool.name} information`}
					className={`absolute left-0 z-30 w-80 sm:w-96 rounded-xl border border-slate-700 bg-slate-900/95 p-4 shadow-2xl backdrop-blur-md text-left ${
						openUpward ? 'bottom-full mb-2' : 'top-full mt-2'
					}`}
				>
					<div className="flex flex-wrap items-center gap-2">
						<span className="text-sm font-semibold text-white">
							{tool.name}
						</span>
						{tool.category && (
							<span className="rounded-full border border-slate-700 bg-slate-800 px-2 py-0.5 text-[11px] font-medium text-slate-300">
								{tool.category}
							</span>
						)}
					</div>
					<p className="mt-0.5 text-[11px] text-slate-400">
						{tool.use_case || 'General Analysis'}
					</p>

					<p className="mt-2.5 text-xs leading-relaxed text-slate-300">
						{tool.description || 'Static analysis tool configured in catalog.'}
					</p>

					{hasSafeDocsUrl && (
						<div className="mt-3 flex items-center justify-end border-t border-slate-800 pt-2.5 text-[11px]">
							<a
								href={tool.docs_url}
								target="_blank"
								rel="noopener noreferrer"
								className="inline-flex items-center gap-1.5 font-medium text-violet-400 hover:text-violet-300 hover:underline"
							>
								<svg className="w-3.5 h-3.5 shrink-0" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
									<path fillRule="evenodd" clipRule="evenodd" d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.53 1.032 1.53 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
								</svg>
								<span>Official GitHub repository</span>
								<svg className="w-3 h-3 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
									<path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
								</svg>
							</a>
						</div>
					)}
				</div>
			)}
		</div>
	);
};

export default ToolInfoPopover;
