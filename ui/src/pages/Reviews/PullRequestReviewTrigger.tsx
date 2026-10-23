import React, { useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Button, Card, EmptyState, Icons, Spinner } from '../../components/UIPrimitives';
import { startOrReuseReviewForPullRequest } from '../../api/pullRequests';
import { useSwitchToPullRequestOrg, PR_NOT_FOUND_MESSAGE } from '../../hooks/useSwitchToPullRequestOrg';
import { GEMINI_API_KEYS_URL, isMissingAIProviderError } from '../../utils/aiProviderErrors';

// Landing page for the "Start a review" link LiveReview posts on new PRs/MRs:
// switches to the PR's org, starts (or reuses a running) review, then opens it.
const PullRequestReviewTrigger: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const switchToPullRequestOrg = useSwitchToPullRequestOrg();
  const [error, setError] = useState<string | null>(null);
  const started = useRef(false);

  const run = async () => {
    const pullRequestId = Number(id);
    if (!Number.isInteger(pullRequestId) || pullRequestId <= 0) {
      setError('This review link is invalid.');
      return;
    }
    try {
      const orgPullRequestId = await switchToPullRequestOrg(pullRequestId);
      const res = await startOrReuseReviewForPullRequest(orgPullRequestId);
      navigate(`/reviews/${res.reviewId}`, { replace: true });
    } catch (err: any) {
      setError(err?.status === 404 ? PR_NOT_FOUND_MESSAGE : err?.message || 'Failed to start the review.');
    }
  };

  useEffect(() => {
    // Guard against StrictMode's double effect firing two reviews.
    if (started.current) return;
    started.current = true;
    void run();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  if (error && isMissingAIProviderError(error)) {
    return (
      <div className="max-w-xl mx-auto px-4 py-12">
        <Card>
          <div className="p-6">
            <h2 className="text-lg font-semibold text-white">Add an AI provider to start this review</h2>
            <p className="mt-2 text-sm text-slate-300 leading-relaxed">
              No AI provider is set up for your organization yet, so LiveReview can&apos;t write review comments. Gemini
              provides a free API key:{' '}
              <a className="text-blue-400 hover:text-blue-300" href={GEMINI_API_KEYS_URL} target="_blank" rel="noopener noreferrer">
                get one from Google AI Studio
              </a>
              .
            </p>
            <ol className="mt-6">
              <li className="flex gap-4">
                <div className="flex flex-col items-center">
                  <span className="flex-shrink-0 flex items-center justify-center w-6 h-6 rounded-full bg-blue-600 text-xs font-semibold text-white">1</span>
                  <div className="w-px flex-1 my-1 bg-slate-700/70" />
                </div>
                <div className="flex-1 pb-6 min-w-0">
                  <h3 className="text-sm font-semibold text-white">Add an AI provider</h3>
                  <p className="mt-1 text-sm text-slate-300">Add your API key under AI Providers. It opens in a new tab.</p>
                  <Button
                    className="mt-2"
                    size="sm"
                    variant="outline"
                    icon={<Icons.AI />}
                    onClick={() => window.open(`${window.location.origin}/#/ai`, '_blank', 'noopener,noreferrer')}
                  >
                    Configure AI
                  </Button>
                </div>
              </li>
              <li className="flex gap-4">
                <div className="flex flex-col items-center">
                  <span className="flex-shrink-0 flex items-center justify-center w-6 h-6 rounded-full bg-blue-600 text-xs font-semibold text-white">2</span>
                </div>
                <div className="flex-1 min-w-0">
                  <h3 className="text-sm font-semibold text-white">Review the PR</h3>
                  <p className="mt-1 text-sm text-slate-300">Once the AI provider is added, click here to review the PR.</p>
                  <Button
                    className="mt-2"
                    size="sm"
                    onClick={() => {
                      setError(null);
                      void run();
                    }}
                  >
                    Review this PR
                  </Button>
                </div>
              </li>
            </ol>
          </div>
        </Card>
      </div>
    );
  }

  if (error) {
    return (
      <EmptyState
        icon={<Icons.Error />}
        title="Couldn't start the review"
        description={error}
        action={<Button variant="primary" onClick={() => navigate('/reviews')}>Go to Reviews</Button>}
      />
    );
  }

  return (
    <div className="flex flex-col items-center justify-center py-20 gap-3 text-slate-300">
      <Spinner size="lg" />
      <p className="text-sm">Starting review…</p>
    </div>
  );
};

export default PullRequestReviewTrigger;
