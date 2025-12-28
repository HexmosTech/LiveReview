import React, { useState, useEffect } from 'react';
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
import { triggerReview, TriggerReviewRequest } from '../../api/reviews';
import { getConnectors, ConnectorResponse } from '../../api/connectors';
import { ErrorBoundary } from '../../components/ErrorBoundary';
import { UpgradePromptModal } from '../../components/Subscriptions';
import { SafetyBanner } from '../../components/SafetyBanner/SafetyBanner';

const NewReview: React.FC = () => {
  const navigate = useNavigate();
  const [url, setUrl] = useState('');
  const [connectors, setConnectors] = useState<ConnectorResponse[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadingConnectors, setLoadingConnectors] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [showUpgradeModal, setShowUpgradeModal] = useState(false);
  const [upgradeReason, setUpgradeReason] = useState<'DAILY_LIMIT' | 'NOT_ORG_CREATOR'>('DAILY_LIMIT');
  const [limitInfo, setLimitInfo] = useState<{ used: number; limit: number }>({ used: 3, limit: 3 });

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

  // Handle form submission
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setSuccess(null);

    // Validate URL
    if (!url) {
      setError('Please enter a URL');
      return;
    }

    try {
      // Validate URL format
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

    setLoading(true);

    try {
      const request: TriggerReviewRequest = { url };
      const response = await triggerReview(request);
      
      setSuccess(`Review successfully triggered! Review ID: ${response.reviewId}`);
      setUrl('');
      
      // Navigate directly to the new review's detail page
      setTimeout(() => {
        navigate(`/reviews/${response.reviewId}`);
      }, 1500);
      
    } catch (err: any) {
      console.error('Error triggering review:', err);
      
      // Check for subscription limit errors (HTTP 402)
      if (err.response?.status === 402 || err.statusCode === 402) {
        const errorData = err.response?.data || err;
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
      } else {
        setError(err.message || 'Failed to trigger review. Please try again later.');
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <ErrorBoundary>
      <div className="container mx-auto px-4 py-8">
        <PageHeader
          title="Preview Review Comments"
          description="See what LiveReview would say — safe preview with no comments posted"
          actions={
            <Button
              variant="outline"
              onClick={() => navigate(-1)}
              icon={<svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 19l-7-7m0 0l7-7m-7 7h18" />
              </svg>}
            >
              Back
            </Button>
          }
        />

        <Card className="max-w-3xl mx-auto mt-8">
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
              {/* Safety Banner */}
              <SafetyBanner variant="detailed" className="mb-6" />
              
              <h3 className="text-lg font-medium text-white mb-4">Enter Merge/Pull Request URL</h3>
              
              {error && (
                <Alert 
                  variant="error" 
                  className="mb-4"
                  icon={<Icons.Error />}
                >
                  {error}
                </Alert>
              )}
              
              {success && (
                <Alert 
                  variant="success" 
                  className="mb-4"
                  icon={<Icons.Success />}
                >
                  <div>
                    <div>{success}</div>
                    <div className="text-sm mt-1">Redirecting to reviews list...</div>
                  </div>
                </Alert>
              )}
              
              <div className="mb-4">
                <Input
                  label="URL"
                  placeholder="https://gitlab.com/your-group/your-project/-/merge_requests/123"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  disabled={loading}
                  icon={<svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
                  </svg>}
                  helperText="Enter the URL of the merge/pull request to preview review comments"
                />
              </div>
              
              {/* URL Examples */}
              <div className="mb-6 p-4 bg-slate-700 rounded-lg">
                <h4 className="text-sm font-medium text-white mb-2">Supported URL Examples:</h4>
                <div className="space-y-1 text-xs text-slate-300">
                  <div>• GitLab: https://gitlab.com/group/project/-/merge_requests/123</div>
                  <div>• GitHub: https://github.com/owner/repo/pull/123</div>
                  <div>• Bitbucket: https://bitbucket.org/workspace/repo/pull-requests/123</div>
                </div>
                <div className="mt-3 pt-3 border-t border-slate-600">
                  <p className="text-xs text-green-300 font-medium">
                    🔒 Preview only - No comments will be posted to your PR/MR
                  </p>
                </div>
              </div>
              
              <div className="flex justify-end space-x-3">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => navigate('/reviews')}
                  disabled={loading}
                >
                  Cancel
                </Button>
                <Button
                  type="submit"
                  variant="primary"
                  disabled={loading || !url.trim()}
                  isLoading={loading}
                >
                  Preview Review Comments
                </Button>
              </div>
            </form>
          )}
        </Card>

        {/* Upgrade Modal */}
        <UpgradePromptModal
          isOpen={showUpgradeModal}
          onClose={() => setShowUpgradeModal(false)}
          reason={upgradeReason}
          currentCount={limitInfo.used}
          limit={limitInfo.limit}
        />
      </div>
    </ErrorBoundary>
  );
};

export default NewReview;
