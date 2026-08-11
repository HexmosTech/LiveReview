import React, { useState } from 'react';
import classNames from 'classnames';
import { Button, Icons } from '../UIPrimitives';
import { SafetyBanner } from '../SafetyBanner/SafetyBanner';

export interface OnboardingStepsProps {
  hasCLI: boolean;
  hasAIProvider: boolean;
  hasRunReview?: boolean;
  installCommand?: string;
  installCommandWindows?: string;
  onConfigureAI: () => void;
  isFreePlan?: boolean;
  onUpgrade?: () => void;
}

const Step: React.FC<{
  title: string;
  description: string | React.ReactNode;
  done?: boolean;
  action?: React.ReactNode;
  isLast?: boolean;
}> = ({ title, description, done = false, action, isLast = false }) => {
  return (
    <div className="flex items-start">
      <div
        className={classNames(
          'flex items-center justify-center w-7 h-7 rounded-full mt-0.5 flex-shrink-0',
          done ? 'bg-green-600 text-white' : 'bg-slate-700 text-slate-200'
        )}
        aria-label={done ? 'Step completed' : 'Step pending'}
      >
        {done ? <Icons.Success /> : <span className="text-xs font-semibold">{/* bullet */}</span>}
      </div>
      <div className="ml-3 flex-1">
        <div className="flex items-center justify-between">
          <h4 className={classNames('text-sm font-semibold', done ? 'text-white' : 'text-slate-200')}>{title}</h4>
          {action && <div className="ml-3">{action}</div>}
        </div>
        <div className="text-sm text-slate-300 mt-0.5">{description}</div>
        {!isLast && <div className="h-5 border-l border-slate-700 ml-3 my-3" />}
      </div>
    </div>
  );
};

const CodeBlock: React.FC<{ code: string; onCopy?: () => void }> = ({ code, onCopy }) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    navigator.clipboard.writeText(code);
    setCopied(true);
    if (onCopy) onCopy();
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="relative mt-2 bg-slate-900/80 rounded border border-slate-700 p-3 pr-12 font-mono text-xs text-slate-200 overflow-x-auto max-w-full">
      <code className="block whitespace-pre-wrap break-all">{code}</code>
      <button
        onClick={handleCopy}
        className="absolute top-2 right-2 p-1.5 rounded bg-slate-700 hover:bg-slate-600 transition-colors flex-shrink-0"
        title="Copy to clipboard"
      >
        {copied ? <Icons.Success /> : <Icons.Copy />}
      </button>
    </div>
  );
};

// The step-by-step "get your first review" content — shared by the dashboard's floating
// onboarding banner and the standalone "Create a Review with CLI" page, so both stay in sync.
export const OnboardingSteps: React.FC<OnboardingStepsProps> = ({
  hasCLI,
  hasAIProvider,
  hasRunReview = false,
  installCommand,
  installCommandWindows,
  onConfigureAI,
  isFreePlan = false,
  onUpgrade,
}) => {
  const allSet = hasCLI && hasAIProvider;

  return (
    <div className="space-y-2">
      <SafetyBanner variant="detailed" className="mb-4" />
      <Step
        title="Step 1: Configure AI"
        description="Add at least one AI provider (e.g., OpenAI, Gemini, or Ollama) to generate code review comments."
        done={hasAIProvider}
        action={
          !hasAIProvider && (
            <Button
              size="sm"
              variant="outline"
              icon={<Icons.AI />}
              onClick={() => {
                if (isFreePlan && onUpgrade) {
                  onUpgrade();
                } else {
                  onConfigureAI();
                }
              }}
              title={isFreePlan ? 'Upgrade your plan to configure AI' : undefined}
            >
              Configure
            </Button>
          )
        }
      />
      <Step
        title="Step 2: Install CLI"
        description={
          <>
            <p>Run this command to install the lrc CLI with pre-configured credentials:</p>
            {installCommand ? (
              <>
                <p className="mt-2 text-xs text-slate-400">Linux/Mac:</p>
                <CodeBlock code={installCommand} />
                <p className="mt-3 text-xs text-slate-400">Windows PowerShell:</p>
                <CodeBlock code={installCommandWindows || ''} />
              </>
            ) : (
              <>
                <p className="mt-2 text-sm text-slate-300">Manual installation:</p>
                <CodeBlock code="curl -fsSL https://hexmos.com/lrc-install.sh | bash" />
                <p className="mt-2 text-xs text-slate-400">
                  Then configure with: echo &apos;api_key = &quot;your-api-key&quot;\napi_url = &quot;http://localhost:8888&quot;&apos; &gt; ~/.lrc.toml
                </p>
              </>
            )}
          </>
        }
        done={hasCLI}
      />
      <Step
        title="Step 3: Preview Review Comments"
        description={
          <>
            <p>Navigate to any git repository with uncommitted changes and run:</p>
            <CodeBlock code="git add ." />
            <CodeBlock code="git lrc review" />
            {!allSet && (
              <p className="mt-2 text-sm text-amber-400">Complete steps 1 and 2 first</p>
            )}
          </>
        }
        done={hasRunReview}
        isLast
      />
    </div>
  );
};

export default OnboardingSteps;
