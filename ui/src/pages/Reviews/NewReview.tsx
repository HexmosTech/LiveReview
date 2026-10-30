import React, { useState, useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Card,
  PageHeader,
  Input,
  Button,
  Icons,
  Alert,
  Spinner,
  EmptyState,
  Tooltip
} from '../../components/UIPrimitives';
import ProviderIcon from '../../components/ProviderIcon';
import { triggerReview, formatRelativeTime } from '../../api/reviews';
import { getPullRequests, triggerReviewForPullRequest } from '../../api/pullRequests';
import { PullRequestWithRepo } from '../../types/explore';
import { getConnectors, ConnectorResponse } from '../../api/connectors';
import { ErrorBoundary } from '../../components/ErrorBoundary';
import { UpgradePromptModal } from '../../components/Subscriptions';
import { useQuotaStatusQuery, useBillingStatusQuery } from '../../api/billing';
import { QuotaExhaustedBanner } from '../../components/Dashboard/QuotaExhaustedBanner';
import { QuotaWarningBanner } from '../../components/Dashboard/QuotaWarningBanner';
import GitHubAppNotInstalledModal from '../../components/GitHubApp/GitHubAppNotInstalledModal';
import { getGitHubAppInstallCheck, startGitHubAppConnect } from '../../api/githubApp';
import { notify } from '../../utils/notify';
import { useGitHubAppEnabled } from '../../hooks/useGitHubAppEnabled';
import { isPushSupported } from '../../utils/pushNotifications';
import { PUSH_ENABLE_ACTION_ID } from '../../components/PushNotificationPrompt';

type QuotaStatusResponse = {
  can_trigger_reviews: boolean;
  envelope?: {
    blocked?: boolean;
    trial_readonly?: boolean;
    trial_ends_at?: string;
    usage_percent?: number;
    threshold_state?: string;
    loc_used_month?: number;
    loc_limit_month?: number;
    loc_remaining_month?: number;
    billing_period_end?: string;
    upgrade_url?: string;
  };
};

type TrialBillingStatusResponse = {
  billing?: {
    trial_active?: boolean;
    trial_ends_at?: string | null;
    trial_can_cancel?: boolean;
  };
};

const formatTrialEndsAt = (value?: string | null): string | null => {
  const raw = String(value || '').trim();
  if (!raw) return null;
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return null;
  return date.toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  });
};

const trialDaysRemaining = (value?: string | null): number | null => {
  const raw = String(value || '').trim();
  if (!raw) return null;
  const end = new Date(raw);
  if (Number.isNaN(end.getTime())) return null;
  const diffMs = end.getTime() - Date.now();
  if (diffMs <= 0) return 0;
  return Math.max(1, Math.ceil(diffMs / (24 * 60 * 60 * 1000)));
};

// URL formats shown as provider chips.
const URL_EXAMPLES = [
  { provider: 'github', label: 'GitHub', url: 'https://github.com/owner/repo/pull/123' },
  { provider: 'gitlab', label: 'GitLab', url: 'https://gitlab.com/group/project/-/merge_requests/123' },
  { provider: 'bitbucket', label: 'Bitbucket', url: 'https://bitbucket.org/workspace/repo/pull-requests/123' },
  { provider: 'gitea', label: 'Gitea', url: 'https://gitea.example.com/owner/repo/pulls/123' },
  { provider: 'azuredevops', label: 'Azure DevOps', url: 'https://dev.azure.com/org/project/_git/repo/pullrequest/123' },
];

const isUrlLike = (value: string) => /^https?:\/\//i.test(value.trim());

const NewReview: React.FC = () => {
  const navigate = useNavigate();
  const [url, setUrl] = useState('');
  const [connectors, setConnectors] = useState<ConnectorResponse[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadingConnectors, setLoadingConnectors] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [suggestions, setSuggestions] = useState<PullRequestWithRepo[]>([]);
  const [suggestOpen, setSuggestOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const [showOnPRSteps, setShowOnPRSteps] = useState(false);
  const suggestRequestRef = useRef(0);
  const [showUpgradeModal, setShowUpgradeModal] = useState(false);
  const [upgradeReason, setUpgradeReason] = useState<'DAILY_LIMIT' | 'NOT_ORG_CREATOR'>('DAILY_LIMIT');
  const [limitInfo, setLimitInfo] = useState<{ used: number; limit: number }>({ used: 3, limit: 3 });
  // Shared with Navbar/Dashboard/TeamCheckout/SubscriptionTab via react-query's cache.
  const { data: quotaStatus = null } = useQuotaStatusQuery<QuotaStatusResponse>();
  const { data: trialBilling = null } = useBillingStatusQuery<TrialBillingStatusResponse>();

  // A pasted GitHub PR URL whose repository the LiveReview GitHub App can't post to: ask before starting the review.
  const githubAppEnabled = useGitHubAppEnabled();
  const [notInstalled, setNotInstalled] = useState<{ owner: string; repository: string; targetId?: number } | null>(null);
  // The GitHub install URL of the last "Install GitHub App" click, offered as a link if the browser blocked the tab.
  const [installUrl, setInstallUrl] = useState<string | undefined>(undefined);

  // Load connectors when component mounts
  useEffect(() => {
    const fetchConnectors = async () => {
      try {
        const data = await getConnectors();
        setConnectors(data);
        setLoadingConnectors(false);
      } catch (err) {
        console.error('Error fetching connectors:', err);
        setError('Failed to load git connectors. Please try again later.');
        setLoadingConnectors(false);
      }
    };

    fetchConnectors();
  }, []);


  const effectiveTrialEndsAt = String(trialBilling?.billing?.trial_ends_at || quotaStatus?.envelope?.trial_ends_at || '').trim();
  const activeTrialDaysLeft = trialDaysRemaining(effectiveTrialEndsAt);
  const activeTrialLabel = formatTrialEndsAt(effectiveTrialEndsAt);

  // Extract base URL from input URL
  const extractBaseUrl = (url: string): string => {
    try {
      const parsedUrl = new URL(url);
      return `${parsedUrl.protocol}//${parsedUrl.hostname}`;
    } catch (err) {
      return '';
    }
  };

  // Check if base URL is in connectors
  const isUrlSupported = (url: string): boolean => {
    if (!url) return false;

    const baseUrl = extractBaseUrl(url);
    if (!baseUrl) return false;

    return connectors.some(connector =>
      connector.provider_url && connector.provider_url.includes(baseUrl)
    );
  };

  // Provider of the connector matching the typed URL, for the input icon.
  const detectedProvider = (() => {
    const baseUrl = extractBaseUrl(url);
    if (!baseUrl) return '';
    return connectors.find((c) => c.provider_url && c.provider_url.includes(baseUrl))?.provider || '';
  })();

  // Open PRs matching the typed text (repo, title or author).
  useEffect(() => {
    if (isUrlLike(url) || !url.trim()) {
      setSuggestions([]);
      return;
    }
    const requestId = ++suggestRequestRef.current;
    const timer = setTimeout(() => {
      getPullRequests({ state: 'open', search: url.trim(), perPage: 8 })
        .then((res) => {
          if (requestId !== suggestRequestRef.current) return;
          setSuggestions(res.pull_requests || []);
          setActiveIndex(0);
        })
        .catch(() => {
          if (requestId === suggestRequestRef.current) setSuggestions([]);
        });
    }, 300);
    return () => clearTimeout(timer);
  }, [url]);

  // Right after a review is successfully triggered — not before, not on an
  // unrelated page — offer to enable browser push notifications, with the
  // permission grant itself happening inline via the toast's action button
  // (see PushNotificationPrompt), no navigation to Settings required.
  const maybePromptForPushNotifications = () => {
    if (!isPushSupported() || typeof Notification === 'undefined' || Notification.permission !== 'default') {
      return;
    }
    notify.info("Want a browser notification when this review's findings are ready?", {
      source: 'push-notify',
      dedupeKey: 'push-notify-prompt',
      actions: [{ label: 'Enable', actionId: PUSH_ENABLE_ACTION_ID }],
    });
  };

  // Shared plan/quota gating and error handling for both ways of starting a review.
  const startReview = async (run: () => Promise<{ reviewId: string }>) => {
    setError(null);
    if (quotaStatus?.envelope?.trial_readonly) {
      setError('Trial is read-only for this organization. Upgrade your plan to resume review creation.');
      return;
    }
    if (quotaStatus?.envelope?.blocked || quotaStatus?.can_trigger_reviews === false) {
      setError('Review creation is currently blocked by plan/quota limits. Upgrade or wait for usage reset.');
      return;
    }

    setLoading(true);
    try {
      const response = await run();
      maybePromptForPushNotifications();
      navigate(`/reviews/${response.reviewId}`);
    } catch (err: any) {
      console.error('Error triggering review:', err);

      // Check for subscription limit errors (HTTP 402)
      if (err?.status === 402) {
        const errorData = err?.data || err;
        const errorCode = errorData.code;

        if (errorCode === 'DAILY_LIMIT_EXCEEDED') {
          setLimitInfo({
            used: errorData.used || 3,
            limit: errorData.limit || 3,
          });
          setUpgradeReason('DAILY_LIMIT');
          setShowUpgradeModal(true);
        } else if (errorCode === 'NOT_ORG_CREATOR') {
          setUpgradeReason('NOT_ORG_CREATOR');
          setShowUpgradeModal(true);
        } else {
          setError(errorData.message || err.message || 'Failed to trigger review. Please try again later.');
        }
      } else if (err?.status === 403) {
        // LOC quota exceeded or Seat Assignment Error
        const errorData = err?.data || err;
        setError(errorData.error || errorData.message || 'Monthly LOC quota exceeded. Upgrade your plan or wait for reset.');
      } else if (err?.status === 429) {
        const trialReadOnly = Boolean(err?.data?.envelope?.trial_readonly);
        if (trialReadOnly) {
          setError('Trial is read-only for this organization. Upgrade your plan to resume review creation.');
        } else {
          setError(err?.data?.error || 'LOC quota exhausted for this billing period. Upgrade your plan or wait for reset.');
        }
      } else {
        setError(err.message || 'Failed to trigger review. Please try again later.');
      }
    } finally {
      setLoading(false);
    }
  };

  const startForPullRequest = (pr: PullRequestWithRepo) => {
    setSuggestOpen(false);
    startReview(() => triggerReviewForPullRequest(pr.id));
  };

  // Handle form submission: a pasted URL, or the highlighted suggestion.
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    if (!url.trim()) {
      setError('Paste a pull request URL, or type a repository or PR title.');
      return;
    }
    if (!isUrlLike(url)) {
      const pick = suggestions[activeIndex];
      if (pick) startForPullRequest(pick);
      else setError('No open pull requests match. Paste the PR URL instead.');
      return;
    }

    try {
      new URL(url);
    } catch (err) {
      setError('Please enter a valid URL');
      return;
    }

    // Check if URL is from a supported provider
    if (!isUrlSupported(url)) {
      setError(
        'This URL is not from a connected Git provider. Please connect the appropriate Git provider first.'
      );
      return;
    }

    if (githubAppEnabled) {
      setLoading(true);
      let check;
      try {
        check = await getGitHubAppInstallCheck({ url: url.trim() });
      } catch (err) {
        // Only a hint: if it can't be answered, the review itself still reports what is wrong.
        console.error('GitHub App install check failed:', err);
      } finally {
        setLoading(false);
      }
      if (check?.applicable && !check.installed && check.owner && check.repository) {
        setNotInstalled({ owner: check.owner, repository: check.repository, targetId: check.target_id });
        return;
      }
    }

    startReview(() => triggerReview({ url }));
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (!suggestOpen || suggestions.length === 0) return;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setActiveIndex((i) => (i + 1) % suggestions.length);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActiveIndex((i) => (i - 1 + suggestions.length) % suggestions.length);
    } else if (e.key === 'Escape') {
      setSuggestOpen(false);
    }
  };

  const submitBlocked =
    loading ||
    !url.trim() ||
    Boolean(quotaStatus?.envelope?.blocked) ||
    Boolean(quotaStatus?.envelope?.trial_readonly) ||
    (quotaStatus?.envelope?.usage_percent ?? 0) >= 100 ||
    quotaStatus?.can_trigger_reviews === false;

  return (
    <ErrorBoundary>
      <div className="max-w-6xl mx-auto px-4 py-8">
        <Button variant="ghost" onClick={() => navigate(-1)} icon={<Icons.Back />} className="-ml-4 mb-2">
          Back
        </Button>
        <PageHeader
          title="New Review"
          description="Review any pull or merge request."
        />

        <Card className="mt-6">
          {loadingConnectors ? (
            <div className="flex justify-center items-center py-12">
              <Spinner size="lg" />
            </div>
          ) : connectors.length === 0 ? (
            <EmptyState
              icon={<Icons.EmptyState />}
              title="No Git Providers Connected"
              description="Please connect a Git provider before triggering a review"
              action={
                <Button
                  variant="primary"
                  onClick={() => navigate('/git')}
                  icon={<Icons.Git />}
                >
                  Connect Git Provider
                </Button>
              }
            />
          ) : (
            <form onSubmit={handleSubmit}>
              {/* LOC 90% Warning Banner */}
              {/* LOC 100% Exhausted Banner */}
              {!quotaStatus?.envelope?.blocked && !quotaStatus?.envelope?.trial_readonly && (quotaStatus?.envelope?.usage_percent ?? 0) >= 100 && (
                <QuotaExhaustedBanner
                  locUsed={quotaStatus.envelope?.loc_used_month ?? 0}
                  locLimit={quotaStatus.envelope?.loc_limit_month ?? 0}
                  usagePct={quotaStatus.envelope?.usage_percent ?? 0}
                  onUpgrade={() => navigate(quotaStatus.envelope?.upgrade_url || '/settings-subscriptions-overview')}
                />
              )}
              {!quotaStatus?.envelope?.blocked && !quotaStatus?.envelope?.trial_readonly && quotaStatus?.envelope?.threshold_state === '90' && (quotaStatus?.envelope?.usage_percent ?? 0) < 100 && (
                <QuotaWarningBanner
                  locUsed={quotaStatus.envelope?.loc_used_month ?? 0}
                  locLimit={quotaStatus.envelope?.loc_limit_month ?? 0}
                  usagePct={quotaStatus.envelope?.usage_percent ?? 0}
                  onUpgrade={() => navigate(quotaStatus.envelope?.upgrade_url || '/settings-subscriptions-overview')}
                />
              )}

              {trialBilling?.billing?.trial_active && !quotaStatus?.envelope?.trial_readonly && (
                <Alert variant="info" className="mb-4" icon={<Icons.Info />}>
                  <div>
                    <div className="font-medium text-blue-100">
                      Trial active{typeof activeTrialDaysLeft === 'number' ? ` - ${activeTrialDaysLeft} day${activeTrialDaysLeft === 1 ? '' : 's'} left` : ''}
                    </div>
                    <div className="text-sm mt-1 text-blue-200/90">
                      {activeTrialLabel
                        ? `Your trial ends on ${activeTrialLabel}.`
                        : 'Your trial is active and end time is being synchronized.'}
                    </div>
                    <button
                      type="button"
                      onClick={() => navigate(trialBilling?.billing?.trial_can_cancel ? '/settings-subscriptions-assign' : '/settings-subscriptions-overview')}
                      className="mt-2 px-3 py-1 bg-blue-600 hover:bg-blue-500 text-white text-xs font-semibold rounded transition-colors"
                    >
                      {trialBilling?.billing?.trial_can_cancel ? 'Cancel Trial Now' : 'View Trial Details'}
                    </button>
                  </div>
                </Alert>
              )}

              {/* Blocked / Trial Read-Only Banner */}
              {(quotaStatus?.envelope?.trial_readonly || quotaStatus?.envelope?.blocked || quotaStatus?.can_trigger_reviews === false) && (
                <Alert
                  variant="error"
                  className="mb-4"
                  icon={<Icons.Error />}
                >
                  <div>
                    <div className="font-medium text-red-100">
                      {quotaStatus?.envelope?.trial_readonly ? 'Trial Read-Only Active' : 'Monthly LOC Quota Exceeded'}
                    </div>
                    <div className="text-sm mt-1 text-red-200/90">
                      {quotaStatus?.envelope?.trial_readonly
                        ? 'This organization is in trial read-only mode. Upgrade from Subscription Management to continue triggering reviews.'
                        : `You've used ${(quotaStatus?.envelope?.loc_used_month ?? 0).toLocaleString()} of ${(quotaStatus?.envelope?.loc_limit_month ?? 0).toLocaleString()} LOC this month. Reviews are blocked until your quota resets${quotaStatus?.envelope?.billing_period_end ? ` on ${new Date(quotaStatus.envelope.billing_period_end).toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })}` : ''} or you upgrade your plan.`}
                    </div>
                    <button
                      type="button"
                      onClick={() => navigate(quotaStatus?.envelope?.upgrade_url || '/settings-subscriptions-overview')}
                      className="mt-2 px-3 py-1 bg-red-600 hover:bg-red-500 text-white text-xs font-semibold rounded transition-colors"
                    >
                      Upgrade Now
                    </button>
                  </div>
                </Alert>
              )}

              {error && (
                <Alert variant="error" className="mb-4" icon={<Icons.Error />}>
                  {error}
                </Alert>
              )}

              <label htmlFor="pr-search" className="block text-sm font-medium text-slate-200 mb-2">
                Pull request URL, repository or title
              </label>
              <div className="flex flex-col sm:flex-row gap-3">
                <div className="relative flex-1">
                  <Input
                    id="pr-search"
                    role="combobox"
                    aria-expanded={suggestOpen && suggestions.length > 0}
                    aria-controls="pr-suggestions"
                    aria-activedescendant={suggestOpen && suggestions[activeIndex] ? `pr-suggestion-${suggestions[activeIndex].id}` : undefined}
                    placeholder="Paste a PR URL, or type a repo or PR title"
                    value={url}
                    onChange={(e) => { setUrl(e.target.value); setSuggestOpen(e.target.value.trim() !== ''); }}
                    onFocus={() => setSuggestOpen(url.trim() !== '')}
                    onBlur={() => setTimeout(() => setSuggestOpen(false), 150)}
                    onKeyDown={handleKeyDown}
                    disabled={loading}
                    autoFocus
                    icon={detectedProvider ? <ProviderIcon provider={detectedProvider} /> : <Icons.Search />}
                  />
                  {suggestOpen && suggestions.length > 0 && (
                    <ul
                      id="pr-suggestions"
                      role="listbox"
                      className="absolute z-20 mt-1 w-full overflow-hidden rounded-lg border border-slate-600 bg-slate-800 shadow-xl"
                    >
                      <li className="px-3 pt-2 pb-1 text-[11px] uppercase tracking-wide text-slate-500">
                        Matching open pull requests
                      </li>
                      {suggestions.map((pr, i) => (
                        <li
                          key={pr.id}
                          id={`pr-suggestion-${pr.id}`}
                          role="option"
                          aria-selected={i === activeIndex}
                          onMouseDown={(e) => { e.preventDefault(); startForPullRequest(pr); }}
                          onMouseEnter={() => setActiveIndex(i)}
                          className={`flex items-start gap-3 px-3 py-2 cursor-pointer ${i === activeIndex ? 'bg-slate-700' : ''}`}
                        >
                          <span className="mt-0.5 shrink-0 [&_svg]:w-4 [&_svg]:h-4"><ProviderIcon provider={pr.provider} /></span>
                          <span className="min-w-0">
                            <span className="block truncate text-sm text-white">
                              <span className="text-slate-300">{pr.repository_full_name.split('/').pop()}</span>
                              <span className="text-slate-500"> #{pr.number}</span>
                              {'  '}{pr.title}
                            </span>
                            <span className="block text-xs text-slate-400">
                              {pr.author_name || pr.author_username || 'unknown'} · {formatRelativeTime(pr.provider_updated_at || pr.last_synced_at)}
                            </span>
                          </span>
                        </li>
                      ))}
                      <li className="border-t border-slate-700 px-3 py-2 text-right">
                        <button
                          type="button"
                          onMouseDown={(e) => { e.preventDefault(); navigate('/explore/merge-requests'); }}
                          className="inline-flex items-center gap-1 text-xs text-blue-400 hover:text-blue-300 [&_svg]:w-3.5 [&_svg]:h-3.5"
                        >
                          See all in Merge Requests <Icons.ChevronRight />
                        </button>
                      </li>
                    </ul>
                  )}
                </div>
                <Button type="submit" variant="primary" disabled={submitBlocked} isLoading={loading}>
                  Start review
                </Button>
              </div>

              <div className="mt-5 flex flex-wrap items-center gap-2">
                <span className="text-xs text-slate-400 mr-1">Supported:</span>
                {URL_EXAMPLES.map((ex) => (
                  <Tooltip key={ex.provider} content={ex.url}>
                    <span className="inline-flex items-center gap-1.5 rounded-full border border-slate-600 bg-slate-800 px-2.5 py-1 text-xs text-slate-200">
                      <span className="[&_svg]:w-3.5 [&_svg]:h-3.5"><ProviderIcon provider={ex.provider} /></span>
                      {ex.label}
                    </span>
                  </Tooltip>
                ))}
              </div>
            </form>
          )}
        </Card>

        <h2 className="mt-10 mb-3 text-sm font-semibold uppercase tracking-wide text-slate-400">Other ways to review</h2>
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          <div className="rounded-lg border border-slate-700 bg-slate-800/60 p-4">
            <div className="flex items-center gap-2 text-sm font-medium text-white">
              <span className="text-slate-400 [&_svg]:w-4 [&_svg]:h-4"><Icons.Git /></span>
              On the PR
            </div>
            <p className="mt-1 text-xs text-slate-400">Start a review from the pull request itself.</p>
            <button
              type="button"
              onClick={() => setShowOnPRSteps((v) => !v)}
              aria-expanded={showOnPRSteps}
              className="mt-3 inline-flex items-center gap-1 text-xs text-blue-400 hover:text-blue-300 [&_svg]:w-3.5 [&_svg]:h-3.5"
            >
              {showOnPRSteps ? 'Hide steps' : 'How it works'}
              <span className={showOnPRSteps ? 'rotate-180' : ''}><Icons.ChevronDown /></span>
            </button>
            {showOnPRSteps && (
              <ol className="mt-2 list-decimal pl-4 space-y-1 text-xs text-slate-300">
                <li>Assign the LiveReview bot as a reviewer.</li>
                <li>Or @mention the bot in a PR comment.</li>
                <li>Or click <span className="text-white">Start a review</span> in the PR description.</li>
              </ol>
            )}
          </div>
          {[
            { icon: <Icons.Download />, title: 'Pre-commit CLI', text: 'Review staged changes before you commit with git-lrc.', link: 'Install the CLI', to: '/reviews/create-cli' },
            { icon: <Icons.AI />, title: 'IDE / MCP', text: 'Ask your AI coding agent to review changes through LiveReview\'s MCP server.', link: 'See setup details', to: '/reviews/create-mcp' },
            { icon: <Icons.Ready />, title: 'CI/CD & API', text: 'Gate merges in CI, or start reviews from your own scripts with an API key.', link: 'Set up CI rulesets', to: '/ci-rulesets' },
            { icon: <Icons.Chat />, title: 'Chatbot', text: 'Ask Livi to review a pull request in plain language.', link: 'Open the chat', to: '/chat?prefill=' + encodeURIComponent('Trigger a review for <paste PR link here>') },
            { icon: <Icons.Clock />, title: 'Scheduled', text: 'Review whole repositories on a regular schedule.', link: 'Set up a schedule', to: '/reviews/scheduled' },
          ].map((way) => (
            <div key={way.to} className="rounded-lg border border-slate-700 bg-slate-800/60 p-4">
              <div className="flex items-center gap-2 text-sm font-medium text-white">
                <span className="text-slate-400 [&_svg]:w-4 [&_svg]:h-4">{way.icon}</span>
                {way.title}
              </div>
              <p className="mt-1 text-xs text-slate-400">{way.text}</p>
              <button type="button" onClick={() => navigate(way.to)} className="mt-3 inline-flex items-center gap-1 text-xs text-blue-400 hover:text-blue-300 [&_svg]:w-3.5 [&_svg]:h-3.5">
                {way.link} <Icons.ChevronRight />
              </button>
            </div>
          ))}
        </div>

        {/* Upgrade Modal */}
        <UpgradePromptModal
          isOpen={showUpgradeModal}
          onClose={() => setShowUpgradeModal(false)}
          reason={upgradeReason}
          currentCount={limitInfo.used}
          limit={limitInfo.limit}
        />
        
        {notInstalled && (
          <GitHubAppNotInstalledModal
            owner={notInstalled.owner}
            repository={notInstalled.repository}
            continueBusy={loading}
            installUrl={installUrl}
            onInstall={() => {
              // The GitHub round trip runs in its own tab, so this page keeps the review being set up; it ends on the
              // connected GitHub accounts, still signed in as this user. The tab is opened right away (browsers only
              // allow that shortly after a click) and sent to GitHub once the server has started the round trip.
              const tab = window.open('', '_blank');
              // A tab opened this way can reach back to this page through window.opener; cut that link.
              if (tab) tab.opener = null;
              setInstallUrl(undefined);
              startGitHubAppConnect('install', 'new-review', 'git', notInstalled.targetId)
                .then((target) => {
                  setInstallUrl(target);
                  if (tab) tab.location.href = target;
                })
                .catch((err: any) => {
                  tab?.close();
                  notify.error(err?.message || 'Could not start connecting to GitHub. Please try again.');
                });
              return tab !== null;
            }}
            onInstalled={async () => {
              const check = await getGitHubAppInstallCheck({ url: url.trim() });
              if (!check.installed) return false;
              setNotInstalled(null);
              startReview(() => triggerReview({ url }));
              return true;
            }}
            onContinue={async () => {
              await startReview(() => triggerReview({ url }));
              setNotInstalled(null);
            }}
            onCancel={() => setNotInstalled(null)}
          />
        )}
      </div>
    </ErrorBoundary>
  );
};

export default NewReview;
