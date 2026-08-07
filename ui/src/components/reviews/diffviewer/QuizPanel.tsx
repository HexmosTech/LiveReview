// Ported from git-lrc:internal/staticserve/static/components/Quiz.js (as of the
// git-lrc HEAD current when this port was written) — a comprehension check generated
// alongside the summary. Static per review: answers aren't re-fetched, just cleared
// on retry.
import React, { useState } from 'react';
import classNames from 'classnames';
import { Button, EmptyState, Icons } from '../../UIPrimitives';
import { DiffReviewQuizQuestion } from '../../../types/reviews';

const OPTION_LETTERS = ['A', 'B', 'C', 'D'];

interface QuizPanelProps {
  quiz: DiffReviewQuizQuestion[];
}

const QuizPanel: React.FC<QuizPanelProps> = ({ quiz }) => {
  const [selected, setSelected] = useState<Record<number, number>>({});
  const [submitted, setSubmitted] = useState(false);

  if (!quiz || quiz.length === 0) {
    return <EmptyState icon={<Icons.Info />} title="No quiz was generated for this review" />;
  }

  const answeredCount = Object.keys(selected).length;
  const allAnswered = answeredCount === quiz.length;
  const score = submitted ? quiz.reduce((sum, q, idx) => sum + (selected[idx] === q.correctIndex ? 1 : 0), 0) : 0;

  const choose = (questionIdx: number, optionIdx: number) => {
    if (submitted) return;
    setSelected((prev) => ({ ...prev, [questionIdx]: optionIdx }));
  };

  const handleRetry = () => {
    setSelected({});
    setSubmitted(false);
  };

  return (
    <div className="space-y-4">
      {submitted && (
        <div className="flex items-center justify-between rounded-lg border border-slate-700 bg-slate-800 px-4 py-3">
          <span className="text-sm text-slate-200">You scored <strong className="text-white">{score}/{quiz.length}</strong></span>
          <Button variant="outline" onClick={handleRetry}>Try Again</Button>
        </div>
      )}
      {quiz.map((q, qIdx) => (
        <div key={qIdx} className="rounded-lg border border-slate-700 bg-slate-800 p-4">
          <p className="mb-3 text-sm font-medium text-slate-100">{qIdx + 1}. {q.question}</p>
          <div className="space-y-2">
            {(q.options || []).map((opt, oIdx) => {
              const isChosen = selected[qIdx] === oIdx;
              const isCorrect = oIdx === q.correctIndex;
              return (
                <button
                  key={oIdx}
                  type="button"
                  disabled={submitted}
                  onClick={() => choose(qIdx, oIdx)}
                  className={classNames(
                    'flex w-full items-start gap-2 rounded-md border px-3 py-2 text-left text-sm',
                    submitted && isCorrect && 'border-emerald-600 bg-emerald-900/30 text-emerald-200',
                    submitted && isChosen && !isCorrect && 'border-red-600 bg-red-900/30 text-red-200',
                    !submitted && isChosen && 'border-blue-600 bg-blue-900/20 text-white',
                    !submitted && !isChosen && 'border-slate-700 text-slate-300 hover:border-slate-600'
                  )}
                >
                  <span className="shrink-0 font-mono text-slate-500">{OPTION_LETTERS[oIdx] || oIdx + 1}</span>
                  <span>{opt}</span>
                </button>
              );
            })}
          </div>
          {submitted && q.explanation && (
            <p className="mt-3 text-xs text-slate-400">{q.explanation}</p>
          )}
        </div>
      ))}
      {!submitted && (
        <Button variant="primary" disabled={!allAnswered} onClick={() => setSubmitted(true)}>
          Check My Answers ({answeredCount}/{quiz.length} answered)
        </Button>
      )}
    </div>
  );
};

export default QuizPanel;
