import React from 'react';
import { useNavigate } from 'react-router-dom';
import { Button, Card, Icons, PageHeader, Spinner } from '../../components/UIPrimitives';
import { useDashboardQuery } from '../../api/dashboard';
import { OnboardingSteps } from '../../components/Dashboard/OnboardingSteps';
import { ErrorBoundary } from '../../components/ErrorBoundary';
import { getApiUrl } from '../../utils/apiUrl';

// Standalone page for the mega menu's "Create via CLI" entry — same onboarding content the
// dashboard's floating banner shows (via the shared OnboardingSteps component), just landed on
// directly instead of forced open over the dashboard.
const CreateReviewCLI: React.FC = () => {
  const navigate = useNavigate();
  // Shares the same cached query as Dashboard.tsx - arriving here right after visiting the
  // dashboard reuses that result instead of firing another request.
  const { data: dashboardData = null, isLoading: loading } = useDashboardQuery();

  const codeReviews = dashboardData?.total_reviews || 0;
  const aiConnectors = dashboardData?.active_ai_connectors || 0;
  const hasCLI = dashboardData?.cli_installed || false;
  const hasAIProvider = aiConnectors > 0;
  const hasRunReview = codeReviews > 0;

  const apiKey = dashboardData?.onboarding_api_key || '';
  const apiUrl = getApiUrl();
  const installCommand = apiKey
    ? `curl -fsSL https://hexmos.com/lrc-install.sh | LRC_API_KEY="${apiKey}" LRC_API_URL="${apiUrl}" bash`
    : '';
  const installCommandWindows = apiKey
    ? `$env:LRC_API_KEY="${apiKey}"; $env:LRC_API_URL="${apiUrl}"; iwr -useb https://hexmos.com/lrc-install.ps1 | iex`
    : '';

  return (
    <ErrorBoundary>
      <div className="container mx-auto px-4 py-8">
        <Button variant="ghost" onClick={() => navigate(-1)} icon={<Icons.Back />} className="-ml-4 mb-2">
          Back
        </Button>
        <PageHeader
          title="Create a Review with CLI"
          description="Install git-lrc and trigger a review straight from your terminal under 2 minutes."
        />
        {loading ? (
          <div className="flex justify-center py-12">
            <Spinner />
          </div>
        ) : (
          <Card>
            <div className="p-4 sm:p-6">
              <OnboardingSteps
                hasCLI={hasCLI}
                hasAIProvider={hasAIProvider}
                hasRunReview={hasRunReview}
                installCommand={installCommand}
                installCommandWindows={installCommandWindows}
                onConfigureAI={() => navigate('/ai')}
              />
            </div>
          </Card>
        )}
      </div>
    </ErrorBoundary>
  );
};

export default CreateReviewCLI;
