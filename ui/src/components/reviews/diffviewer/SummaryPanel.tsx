// Ported from git-lrc:internal/staticserve/static/components/Summary.js (as of HEAD)
import React, { useEffect, useState } from 'react';
import { DiffReviewFile, DiffReviewQuizQuestion } from '../../../types/reviews';
import { Markdown, OpenFileFromText } from '../../../lib/markdown';
import SummarySlideshow from './SummarySlideshow';
import QuizPanel from './QuizPanel';

interface SummaryPanelProps {
  reviewId: number;
  summary?: string;
  files: DiffReviewFile[];
  quiz: DiffReviewQuizQuestion[];
  onOpenFile?: OpenFileFromText;
}

type ViewMode = 'slides' | 'text' | 'quiz';

const SummaryPanel: React.FC<SummaryPanelProps> = ({ reviewId, summary, files, quiz, onOpenFile }) => {
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
      <div className="summary rounded-lg border border-slate-700 bg-slate-800/95 p-3 md:p-4 shadow-lg">
        {hasSummary && (
          <div className="summary-header-row grid grid-cols-[1fr_auto_1fr] items-center gap-3 mb-3">
            <div className="summary-header-left flex items-center justify-self-start z-10">
              <div
                className="summary-view-toggle inline-flex items-center rounded-full border border-[#454545] bg-gradient-to-b from-white/[0.045] to-white/[0.02] shadow-[inset_0_0_0_1px_rgba(255,255,255,0.02)] overflow-hidden"
                role="group"
                aria-label="Summary display mode"
              >
                <button
                  type="button"
                  className={`action-btn summary-view-btn inline-flex items-center gap-1.5 px-3.5 py-1.5 text-xs font-medium transition-colors border-0 outline-none rounded-none ${
                    viewMode === 'slides'
                      ? 'active bg-[rgba(76,175,80,0.2)] text-[#89d185]'
                      : 'bg-transparent text-[#858585] hover:text-[#cccccc] hover:bg-white/[0.06]'
                  }`}
                  onClick={() => setViewMode('slides')}
                  title="Show slides view"
                  aria-label="Show slides view"
                  aria-pressed={viewMode === 'slides'}
                >
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" className="inline-block">
                    <path d="M4 4h10v10H4z" />
                    <path d="M10 10h10v10H10z" />
                  </svg>
                  <span>Slides</span>
                </button>
                <button
                  type="button"
                  className={`action-btn summary-view-btn inline-flex items-center gap-1.5 px-3.5 py-1.5 text-xs font-medium transition-colors border-0 border-l border-[#454545] outline-none rounded-none ${
                    viewMode === 'text'
                      ? 'active bg-[rgba(76,175,80,0.2)] text-[#89d185]'
                      : 'bg-transparent text-[#858585] hover:text-[#cccccc] hover:bg-white/[0.06]'
                  }`}
                  onClick={() => setViewMode('text')}
                  title="Show text view"
                  aria-label="Show text view"
                  aria-pressed={viewMode === 'text'}
                >
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" className="inline-block">
                    <path d="M4 7h16" />
                    <path d="M8 7v10" />
                    <path d="M16 7v10" />
                  </svg>
                  <span>Text</span>
                </button>
                {hasQuiz && (
                  <button
                    type="button"
                    className={`action-btn summary-view-btn inline-flex items-center gap-1.5 px-3.5 py-1.5 text-xs font-medium transition-colors border-0 border-l border-[#454545] outline-none rounded-none ${
                      viewMode === 'quiz'
                        ? 'active bg-[rgba(76,175,80,0.2)] text-[#89d185]'
                        : 'bg-transparent text-[#858585] hover:text-[#cccccc] hover:bg-white/[0.06]'
                    }`}
                    onClick={() => setViewMode('quiz')}
                    title="Show comprehension quiz"
                    aria-label="Show comprehension quiz"
                    aria-pressed={viewMode === 'quiz'}
                  >
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" className="inline-block">
                      <circle cx="12" cy="12" r="10" />
                      <path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3" />
                      <line x1="12" y1="17" x2="12.01" y2="17" />
                    </svg>
                    <span>Quiz</span>
                  </button>
                )}
              </div>
            </div>

            <div
              className="summary-header-center justify-self-center text-[11px] font-semibold text-slate-400/90 uppercase tracking-[0.08em] select-none hidden md:block"
              aria-hidden="true"
            >
              Summary
            </div>

            <div className="summary-actions flex items-center justify-end gap-2 justify-self-end z-10">
              <button
                type="button"
                className="action-btn summary-play-btn inline-flex items-center gap-1.5 rounded-[6px] border border-[rgba(110,118,129,0.3)] bg-[rgba(110,118,129,0.14)] px-3 py-1.5 text-xs font-medium text-[#c9d1d9] hover:bg-[rgba(110,118,129,0.22)] hover:border-[rgba(110,118,129,0.44)] hover:text-white transition-colors"
                onClick={() => setModalOpen(true)}
                title="Open slides in dialog"
                aria-label="Open slides in dialog"
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M4 4h10v10H4z" />
                  <path d="M10 10h10v10H10z" />
                </svg>
                <span>Open Slides</span>
              </button>
            </div>
          </div>
        )}

        {!hasSummary ? (
          <p className="text-sm text-slate-500 py-3">No summary was generated for this review.</p>
        ) : viewMode === 'quiz' ? (
          <div className="summary-quiz-container mb-3">
            <QuizPanel quiz={quiz} />
          </div>
        ) : viewMode === 'slides' ? (
          <div className="summary-embedded-container mb-3">
            <SummarySlideshow
              reviewId={reviewId}
              summary={summary!}
              hasQuiz={hasQuiz}
              isShortcutActive={!modalOpen}
              onTakeQuiz={() => setViewMode('quiz')}
              onOpenFile={onOpenFile}
            />
          </div>
        ) : (
          <div className="summary-text-content text-slate-200 text-sm leading-[1.76] py-1">
            <Markdown text={summary!} onOpenFile={onOpenFile} />
          </div>
        )}
      </div>

      {modalOpen && (
        <div
          className="summary-slideshow-modal fixed inset-0 z-50 flex items-center justify-center bg-slate-950/95 backdrop-blur p-4"
          onClick={() => setModalOpen(false)}
        >
          <div
            className="relative w-full max-w-5xl flex flex-col max-h-[95vh]"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="summary-slideshow-chrome flex items-center justify-between gap-4 px-4 py-2.5 rounded-t-[14px] bg-[#11161d] border-t border-x border-white/10">
              <div className="text-xs font-semibold text-slate-400 tracking-[0.01em]">
                Review slideshow
              </div>
              <button
                type="button"
                onClick={() => setModalOpen(false)}
                className="action-btn summary-slide-btn inline-flex items-center justify-center w-7 h-7 rounded-md border border-[rgba(255,255,255,0.15)] bg-white/5 text-slate-300 hover:bg-white/10 hover:text-white transition-colors"
                title="Close slideshow (Esc)"
                aria-label="Close slideshow"
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
                </svg>
              </button>
            </div>
            <div className="flex-1 overflow-hidden">
              <SummarySlideshow
                reviewId={reviewId}
                summary={summary!}
                hasQuiz={hasQuiz}
                isModal={true}
                isShortcutActive={modalOpen}
                onClose={() => setModalOpen(false)}
                onTakeQuiz={() => { setModalOpen(false); setViewMode('quiz'); }}
                onOpenFile={onOpenFile}
              />
            </div>
          </div>
        </div>
      )}
    </>
  );
};

export default SummaryPanel;
