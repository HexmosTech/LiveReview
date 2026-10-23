import React from 'react';
import { useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { Button, Icons } from '../UIPrimitives';
import { useDashboardQuery } from '../../api/dashboard';
import { getAIConnectors } from '../../api/connectors';
import { GEMINI_API_KEYS_URL } from '../../utils/aiProviderErrors';

// The steps after a GitHub account is connected: an AI provider, then the first review. Shared by get-started and
// the GitHub App card on Git Providers, so an org that connects from either place sees what to do next.

export const Step: React.FC<{
    index: number;
    title: string;
    done?: boolean;
    isLast?: boolean;
    children: React.ReactNode;
}> = ({ index, title, done = false, isLast = false, children }) => (
    <li className="flex gap-4">
        <div className="flex flex-col items-center">
            <span
                className={`flex-shrink-0 flex items-center justify-center w-6 h-6 rounded-full text-xs font-semibold text-white ${
                    done ? 'bg-green-600' : 'bg-blue-600'
                }`}
            >
                {done ? <Icons.Success /> : index}
            </span>
            {!isLast && <div className="w-px flex-1 my-1 bg-slate-700/70" />}
        </div>
        <div className="flex-1 pb-6 min-w-0">
            <h3 className="text-sm font-semibold text-white">{title}</h3>
            <div className="mt-1 text-sm text-slate-300">{children}</div>
        </div>
    </li>
);

// The AI provider and "Review a pull request" steps, numbered from `startIndex`.
const GitHubAppNextSteps: React.FC<{ startIndex: number }> = ({ startIndex }) => {
    const navigate = useNavigate();
    const { data: dashboardData = null } = useDashboardQuery();
    const hasAIProvider = (dashboardData?.active_ai_connectors || 0) > 0;
    // Launch-trial sign-ups get the LiveReview AI Model connector instead of having to add a key.
    const { data: aiConnectors = [] } = useQuery({ queryKey: ['ai-connectors'], queryFn: getAIConnectors, enabled: hasAIProvider });
    const usesLiveReviewModel = aiConnectors.some((c: any) => c?.provider_name === 'livereview-default-ai' && (c?.role || 'leader') === 'leader');

    return (
        <>
            <Step index={startIndex} title="AI provider" done={hasAIProvider}>
                {hasAIProvider && usesLiveReviewModel ? (
                    <>
                        <p>
                            LiveReview AI is connected for you, so there&apos;s nothing to set up: reviews run on our model, up to
                            30,000 reviewed lines a month on the Free plan.
                        </p>
                        <Button className="mt-2" size="sm" variant="ghost" icon={<Icons.AI />} onClick={() => window.open(`${window.location.origin}/#/ai`, '_blank', 'noopener,noreferrer')}>
                            Use your own AI key instead
                        </Button>
                    </>
                ) : hasAIProvider ? (
                    <p>An AI provider is configured.</p>
                ) : (
                    <>
                        <p>
                            Add an AI provider so LiveReview can write review comments. Gemini provides a free API key:{' '}
                            <a className="text-blue-400 hover:text-blue-300" href={GEMINI_API_KEYS_URL} target="_blank" rel="noopener noreferrer">
                                get one from Google AI Studio
                            </a>
                            .
                        </p>
                        <Button className="mt-2" size="sm" variant="outline" icon={<Icons.AI />} onClick={() => window.open(`${window.location.origin}/#/ai`, '_blank', 'noopener,noreferrer')}>
                            Configure AI
                        </Button>
                    </>
                )}
            </Step>

            <Step index={startIndex + 1} title="Review a pull request" isLast>
                <p>Pick one of your open merge requests / pull requests, or trigger a review with a URL.</p>
                <div className="mt-2 flex flex-wrap gap-2">
                    <Button size="sm" onClick={() => navigate('/explore/merge-requests')}>
                        See open MRs/PRs and trigger a review
                    </Button>
                    <Button size="sm" variant="outline" onClick={() => navigate('/reviews/new')}>
                        Trigger a review via URL
                    </Button>
                </div>
            </Step>
        </>
    );
};

export default GitHubAppNextSteps;
