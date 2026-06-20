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
  EmptyState
} from '../../components/UIPrimitives';
import { getConnectors, ConnectorResponse } from '../../api/connectors';
import { getReviewEvents } from '../../api/reviews';
import ReviewTimeline from '../../components/reviews/ReviewTimeline';
import { ReviewEvent } from '../../components/reviews/types';
import apiClient from '../../api/apiClient';

const BetaToolReviewPage: React.FC = () => {
  const navigate = useNavigate();
  const [url, setUrl] = useState('');
  const [connectors, setConnectors] = useState<ConnectorResponse[]>([]);
  const [loadingConnectors, setLoadingConnectors] = useState(true);
  
  // Review execution state
  const [reviewId, setReviewId] = useState<number | null>(null);
  const [isTriggering, setIsTriggering] = useState(false);
  const [reviewStatus, setReviewStatus] = useState<string>('idle');
  const [events, setEvents] = useState<ReviewEvent[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const pollingRef = useRef<NodeJS.Timeout | null>(null);

  // 1. Fetch Git connectors (needed to verify at least one exists)
  useEffect(() => {
    const fetchConnectors = async () => {
      try {
        const data = await getConnectors();
        setConnectors(data);
        setLoadingConnectors(false);
      } catch (err) {
        console.error('Error fetching connectors:', err);
        setError('Failed to load Git connectors.');
        setLoadingConnectors(false);
      }
    };
    fetchConnectors();
  }, []);

  // 2. Poll events when review is in progress
  useEffect(() => {
    if (reviewId === null || reviewStatus !== 'in_progress') {
      if (pollingRef.current) {
        clearInterval(pollingRef.current);
        pollingRef.current = null;
      }
      return;
    }

    const poll = async () => {
      try {
        const response = await getReviewEvents(reviewId, undefined, 100);
        const newEvents = ((response?.events || []) as any[]).map(e => ({
          id: e.id.toString(),
          timestamp: e.time,
          eventType: e.type as any,
          message: e.data?.message || (e.type === 'tool_result' ? `Static Analysis: ${e.data?.tool_name || 'Tool'} completed` : `${e.type} event`),
          details: {
            batchId: e.batchId,
            ...e.data
          },
          severity: e.level as any
        }));
        setEvents(newEvents);

        // Check if review has finished
        const reviewData = await apiClient.get<any>(`/reviews/${reviewId}`);
        if (reviewData && (reviewData.status === 'completed' || reviewData.status === 'failed')) {
          setReviewStatus(reviewData.status);
          if (pollingRef.current) {
            clearInterval(pollingRef.current);
            pollingRef.current = null;
          }
        }
      } catch (err) {
        console.warn('Error polling events:', err);
      }
    };

    // Initial check
    poll();
    pollingRef.current = setInterval(poll, 2000);

    return () => {
      if (pollingRef.current) {
        clearInterval(pollingRef.current);
        pollingRef.current = null;
      }
    };
  }, [reviewId, reviewStatus]);

  // Extract base URL from input URL
  const extractBaseUrl = (inputUrl: string): string => {
    try {
      const parsed = new URL(inputUrl);
      return `${parsed.protocol}//${parsed.hostname}`;
    } catch {
      return '';
    }
  };

  // Check if base URL matches any connected provider
  const isUrlSupported = (inputUrl: string): boolean => {
    if (!inputUrl) return false;
    const baseUrl = extractBaseUrl(inputUrl);
    if (!baseUrl) return false;
    return connectors.some(c =>
      c.provider_url && c.provider_url.includes(baseUrl)
    );
  };

  // 3. Handle submit trigger
  const handleTrigger = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!url) {
      setError('Please enter a PR/MR URL.');
      return;
    }

    // Validate URL format
    try {
      new URL(url);
    } catch {
      setError('Please enter a valid URL.');
      return;
    }

    // Check if URL is from a supported provider
    if (!isUrlSupported(url)) {
      setError('This URL is not from a connected Git provider. Please connect the appropriate Git provider first.');
      return;
    }

    setError(null);
    setSuccess(null);
    setIsTriggering(true);
    setEvents([]);
    setReviewId(null);
    setReviewStatus('idle');

    try {
      const response = await apiClient.post<any>('/reviews/tool-reviews', {
        pr_url: url,
      });

      setSuccess('Tool review successfully enqueued!');
      setReviewId(response.reviewId);
      setReviewStatus('in_progress');
    } catch (err: any) {
      console.error('Error triggering tool review:', err);
      setError(err?.data?.error || err.message || 'Failed to trigger tool review.');
    } finally {
      setIsTriggering(false);
    }
  };

  return (
    <div className="container mx-auto px-4 py-8 max-w-4xl">
      <PageHeader
        title="Beta Static Analysis Review"
        description="Trigger third-party linters and security scanners on any pull/merge request"
        actions={
          <Button
            variant="outline"
            onClick={() => navigate('/reviews')}
            icon={<Icons.Reviews />}
          >
            All Reviews
          </Button>
        }
      />

      <div className="grid grid-cols-1 gap-6 mt-8">
        {/* Form panel */}
        {reviewStatus === 'idle' && (
          <Card className="bg-slate-800/80 border border-slate-700/60 p-6 backdrop-blur-md rounded-xl shadow-lg">
            {loadingConnectors ? (
              <div className="flex justify-center items-center py-12">
                <Spinner size="lg" />
              </div>
            ) : connectors.length === 0 ? (
              <EmptyState
                icon={<Icons.EmptyState />}
                title="No Git Providers Connected"
                description="Please connect a Git provider before triggering a tool review."
                action={
                  <Button variant="primary" onClick={() => navigate('/git')}>
                    Connect Git Provider
                  </Button>
                }
              />
            ) : (
              <form onSubmit={handleTrigger} className="space-y-6">
                <h3 className="text-lg font-medium text-white mb-2">Enter Merge/Pull Request URL</h3>
                <p className="text-sm text-slate-400 mb-4">
                  The Git connector will be automatically detected from your URL — same as AI reviews.
                </p>

                <Input
                  label="Pull / Merge Request URL"
                  placeholder="https://github.com/owner/repo/pull/42"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  disabled={isTriggering}
                  icon={<svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
                  </svg>}
                  helperText="Enter the URL of the merge/pull request to run static analysis tools"
                />

                {/* URL Examples */}
                <div className="p-4 bg-slate-700/50 rounded-lg">
                  <h4 className="text-sm font-medium text-white mb-2">Supported URL Examples:</h4>
                  <div className="space-y-1 text-xs text-slate-300">
                    <div>• GitLab: https://gitlab.com/group/project/-/merge_requests/123</div>
                    <div>• GitHub: https://github.com/owner/repo/pull/123</div>
                    <div>• Bitbucket: https://bitbucket.org/workspace/repo/pull-requests/123</div>
                  </div>
                </div>

                {error && <Alert variant="error">{error}</Alert>}
                {success && <Alert variant="success">{success}</Alert>}

                <div className="flex justify-end pt-2">
                  <Button
                    type="submit"
                    variant="primary"
                    className="bg-gradient-to-r from-violet-600 to-indigo-600 text-white font-semibold shadow-md hover:from-violet-500 hover:to-indigo-500"
                    disabled={isTriggering || !url.trim()}
                    isLoading={isTriggering}
                  >
                    Run Tool Review
                  </Button>
                </div>
              </form>
            )}
          </Card>
        )}

        {/* Live review stream panel */}
        {reviewId !== null && reviewStatus !== 'idle' && (
          <div className="space-y-6">
            <Card className="bg-slate-800/90 border border-slate-700/80 p-5 rounded-xl flex items-center justify-between">
              <div>
                <h3 className="text-sm text-slate-400 font-semibold uppercase tracking-wider">Review Status</h3>
                <p className="text-2xl font-bold text-white mt-1 capitalize flex items-center gap-2">
                  {reviewStatus === 'in_progress' ? (
                    <>
                      <span className="w-2.5 h-2.5 rounded-full bg-blue-500 animate-ping shrink-0" />
                      Running
                    </>
                  ) : reviewStatus === 'completed' ? (
                    <>
                      <span className="w-2.5 h-2.5 rounded-full bg-green-500 shrink-0" />
                      Completed
                    </>
                  ) : (
                    <>
                      <span className="w-2.5 h-2.5 rounded-full bg-red-500 shrink-0" />
                      Failed
                    </>
                  )}
                </p>
              </div>

              {reviewStatus !== 'in_progress' && (
                <Button variant="outline" onClick={() => {
                  setReviewId(null);
                  setReviewStatus('idle');
                  setEvents([]);
                }}>
                  Trigger Another
                </Button>
              )}
            </Card>

            <ReviewTimeline
              reviewId={reviewId}
              events={events}
              isLive={reviewStatus === 'in_progress'}
              className="mt-4"
            />
          </div>
        )}
      </div>
    </div>
  );
};

export default BetaToolReviewPage;
