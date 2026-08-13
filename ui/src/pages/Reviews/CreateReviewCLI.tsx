import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Card, PageHeader, Spinner } from '../../components/UIPrimitives';
import { getDashboardData, DashboardData } from '../../api/dashboard';
import { OnboardingSteps } from '../../components/Dashboard/OnboardingSteps';
import { ErrorBoundary } from '../../components/ErrorBoundary';
import { useOrgContext } from '../../hooks/useOrgContext';
import { getApiUrl } from '../../utils/apiUrl';
import LicenseUpgradeDialog from '../../components/License/LicenseUpgradeDialog';

// Standalone page for the mega menu's "Create via CLI" entry — same onboarding content the
// dashboard's floating banner shows (via the shared OnboardingSteps component), just landed on
// directly instead of forced open over the dashboard.
const CreateReviewCLI: React.FC = () => {
  const navigate = useNavigate();
  const { isFreePlan } = useOrgContext();
  const [dashboardData, setDashboardData] = useState<DashboardData | null>(null);
  const [loading, setLoading] = useState(true);
  const [showUpgradeDialog, setShowUpgradeDialog] = useState(false);

  useEffect(() => {
    let cancelled = false;
    getDashboardData()
      .then((data) => {
        if (!cancelled) setDashboardData(data);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

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
                isFreePlan={isFreePlan}
                onUpgrade={() => setShowUpgradeDialog(true)}
              />
            </div>
          </Card>
        )}

        <LicenseUpgradeDialog
          open={showUpgradeDialog}
          onClose={() => setShowUpgradeDialog(false)}
          requiredTier="team"
          featureName="Review Creation From Dashboard"
          featureDescription="Unlock AI-powered code reviews by upgrading to a paid plan. Your current plan is read-only."
        />
      </div>
    </ErrorBoundary>
  );
};

export default CreateReviewCLI;
