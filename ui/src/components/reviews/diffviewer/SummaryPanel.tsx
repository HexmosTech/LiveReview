// Ported from git-lrc:internal/staticserve/static/components/Summary.js (as of the
// git-lrc HEAD current when this port was written). Structure matches exactly:
//   .summary > .summary-header-row
//     .summary-header-left: Slides/Text/Quiz toggle pills
//     .summary-header-center: "Summary" label
//     .summary-actions: "Open Slides" button
// No file/comment/severity counts in the header — git-lrc renders those in a
// separate Stats component between the summary card and the toolbar.
//
// Underneath: the content area renders SummarySlideshow (inline), Markdown (text),
// or QuizPanel depending on view mode. A full-viewport modal slideshow is toggled
// by the "Open Slides" button.
import React, { useEffect, useState } from 'react';
import { DiffReviewFile, DiffReviewQuizQuestion } from '../../../types/reviews';
import { Markdown, OpenFileFromText } from '../../../lib/markdown';
import SummarySlideshow from './SummarySlideshow';
import QuizPanel from './QuizPanel';

interface SummaryPanelProps {
  summary?: string;
  files: DiffReviewFile[];
  quiz: DiffReviewQuizQuestion[];
  onOpenFile?: OpenFileFromText;
}

type ViewMode = 'slides' | 'text' | 'quiz';

const SummaryPanel: React.FC<SummaryPanelProps> = ({ summary, files, quiz, onOpenFile }) => {
  const [viewMode, setViewMode] = useState<ViewMode>('slides');
  const [modalOpen, setModalOpen] = useState(false);
  const hasSummary = Boolean(summary && summary.trim());
  const hasQuiz = Array.isArray(quiz) && quiz.length > 0;

  useEffect(() => {
    if (!modalOpen) return;
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') setModalOpen(false); };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [modalOpen]);

  return (
    <>
      <div className="summary rounded-lg border border-slate-700 bg-slate-800 overflow-hidden">
        {hasSummary && (
          <div className="summary-header-row flex items-center justify-between gap-3 px-4 py-2.5 border-b border-slate-700">
            <div className="summary-header-left">
              <div className="summary-view-toggle flex items-center gap-0.5 rounded-full border border-slate-700 bg-slate-900 p-0.5 text-xs" role="group" aria-label="Summary display mode">
                <button
                  className={`action-btn summary-view-btn rounded-full px-3 py-1 ${viewMode === 'slides' ? 'active bg-slate-700 text-white' : 'text-slate-400 hover:text-slate-200'}`}
                  onClick={() => setViewMode('slides')}
                  title="Show slides view"
                  aria-pressed={viewMode === 'slides'}
                >
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" className="inline-block mr-1"><rect x="2" y="3" width="20" height="14" rx="2" ry="2"></rect><line x1="8" y1="21" x2="16" y2="21"></line><line x1="12" y1="17" x2="12" y2="21"></line></svg>
                  Slides
                </button>
                <button
                  className={`action-btn summary-view-btn rounded-full px-3 py-1 ${viewMode === 'text' ? 'active bg-slate-700 text-white' : 'text-slate-400 hover:text-slate-200'}`}
                  onClick={() => setViewMode('text')}
                  title="Show text view"
                  aria-pressed={viewMode === 'text'}
                >
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" className="inline-block mr-1"><polyline points="4 7 4 4 20 4 20 7"></polyline><line x1="9" y1="20" x2="15" y2="20"></line><line x1="12" y1="4" x2="12" y2="20"></line></svg>
                  Text
                </button>
                {hasQuiz && (
                  <button
                    className={`action-btn summary-view-btn rounded-full px-3 py-1 ${viewMode === 'quiz' ? 'active bg-slate-700 text-white' : 'text-slate-400 hover:text-slate-200'}`}
                    onClick={() => setViewMode('quiz')}
                    title="Show comprehension quiz"
                    aria-pressed={viewMode === 'quiz'}
                  >
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" className="inline-block mr-1"><circle cx="12" cy="12" r="10"></circle><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"></path><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>
                    Quiz
                  </button>
                )}
              </div>
            </div>
            <div className="summary-header-center text-xs font-medium text-slate-500 uppercase tracking-wide" aria-hidden="true">
              Summary
            </div>
            <div className="summary-actions">
              <button
                className="action-btn summary-play-btn rounded-full border border-slate-600 bg-slate-900 px-3 py-1 text-xs text-slate-400 hover:bg-slate-700 hover:text-slate-200 flex items-center gap-1"
                onClick={() => setModalOpen(true)}
                title="Open slides in dialog"
                aria-label="Open slides in dialog"
              >
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect x="2" y="3" width="20" height="14" rx="2" ry="2"></rect><polyline points="8 21 12 17 16 21"></polyline></svg>
                Open Slides
              </button>
            </div>
          </div>
        )}

        <div className="p-4">
          {!hasSummary ? (
            <p className="text-sm text-slate-500">No summary was generated for this review.</p>
          ) : viewMode === 'quiz' ? (
            <QuizPanel quiz={quiz} />
          ) : viewMode === 'slides' ? (
            <SummarySlideshow summary={summary!} hasQuiz={hasQuiz} onTakeQuiz={() => setViewMode('quiz')} onOpenFile={onOpenFile} />
          ) : (
            <Markdown text={summary!} onOpenFile={onOpenFile} />
          )}
        </div>
      </div>

      {modalOpen && (
        <div className="summary-slideshow-modal fixed inset-0 z-50 flex items-center justify-center bg-slate-950/95 backdrop-blur" onClick={() => setModalOpen(false)}>
          <div className="flex h-full w-full max-w-5xl flex-col justify-center p-8" onClick={(e) => e.stopPropagation()}>
            <div className="mb-3 flex items-center justify-between">
              <div className="text-xs text-slate-500">LiveReview Summary</div>
              <button type="button" onClick={() => setModalOpen(false)} className="rounded border border-slate-600 px-3 py-1 text-xs text-slate-400 hover:text-slate-200">Close (Esc)</button>
            </div>
            <div className="flex-1 overflow-y-auto">
              <SummarySlideshow summary={summary!} hasQuiz={hasQuiz} onTakeQuiz={() => { setModalOpen(false); setViewMode('quiz'); }} onOpenFile={onOpenFile} />
            </div>
          </div>
        </div>
      )}
    </>
  );
};

export default SummaryPanel;
