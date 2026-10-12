import React, { useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Button, EmptyState, Icons, Spinner } from '../../components/UIPrimitives';
import { startOrReuseReviewForPullRequest } from '../../api/pullRequests';
import { useSwitchToPullRequestOrg, PR_NOT_FOUND_MESSAGE } from '../../hooks/useSwitchToPullRequestOrg';

// Landing page for the "Start a review" link LiveReview posts on new PRs/MRs:
// switches to the PR's org, starts (or reuses a running) review, then opens it.
const PullRequestReviewTrigger: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const switchToPullRequestOrg = useSwitchToPullRequestOrg();
  const [error, setError] = useState<string | null>(null);
  const started = useRef(false);

  useEffect(() => {
    // Guard against StrictMode's double effect firing two reviews.
    if (started.current) return;
    started.current = true;

    const pullRequestId = Number(id);
    if (!Number.isInteger(pullRequestId) || pullRequestId <= 0) {
      setError('This review link is invalid.');
      return;
    }

    (async () => {
      try {
        await switchToPullRequestOrg(pullRequestId);
        const res = await startOrReuseReviewForPullRequest(pullRequestId);
        navigate(`/reviews/${res.reviewId}`, { replace: true });
      } catch (err: any) {
        setError(err?.status === 404 ? PR_NOT_FOUND_MESSAGE : err?.message || 'Failed to start the review.');
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

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
