import React, { useState } from 'react';
import classNames from 'classnames';
import { Button, Icons, Tooltip } from '../UIPrimitives';
import { SafetyBanner } from '../SafetyBanner/SafetyBanner';

interface OnboardingStepperProps {
  hasCLI: boolean;
  hasAIProvider: boolean;
  hasRunReview?: boolean;
  installCommand?: string;
  installCommandWindows?: string;
  onConfigureAI: () => void;
  onNewReview: () => void;
  onDismiss?: () => void;
  className?: string;
  userId?: number | string; // For scoping localStorage to user
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
    <div className="relative mt-2 bg-slate-900/80 rounded border border-slate-700 p-3 pr-12 font-mono text-xs text-slate-200 overflow-x-auto">
      <code>{code}</code>
      <button
        onClick={handleCopy}
        className="absolute top-2 right-2 p-1.5 rounded bg-slate-700 hover:bg-slate-600 transition-colors"
        title="Copy to clipboard"
      >
        {copied ? <Icons.Success /> : <Icons.Copy />}
      </button>
    </div>
  );
};

export const OnboardingStepper: React.FC<OnboardingStepperProps> = ({
  hasCLI,
  hasAIProvider,
  hasRunReview = false,
  installCommand,
  installCommandWindows,
  onConfigureAI,
  onNewReview,
  onDismiss,
  className,
  userId,
}) => {
  const allSet = hasCLI && hasAIProvider;
  const [collapsed, setCollapsed] = useState<boolean>(() => {
    try { 
      const key = userId ? `lr_get_started_collapsed_${userId}` : 'lr_get_started_collapsed';
      return localStorage.getItem(key) === '1'; 
    } catch { 
      return false; 
    }
  });

  const toggleCollapsed = () => {
    setCollapsed(prev => {
      const next = !prev;
      try { 
        const key = userId ? `lr_get_started_collapsed_${userId}` : 'lr_get_started_collapsed';
        localStorage.setItem(key, next ? '1' : '0'); 
      } catch {}
      return next;
    });
  };

  return (
    <div className={classNames('rounded-lg border border-blue-500/30 bg-gradient-to-br from-slate-800/80 to-slate-900/80 p-6 shadow-xl', className)}>
      <div className="flex items-center justify-between mb-4">
        <div>
          <h3 className="text-2xl font-extrabold bg-gradient-to-r from-blue-400 via-cyan-400 to-emerald-400 bg-clip-text text-transparent mb-1.5">
            🚀 Get your first preview in 2 minutes
          </h3>
          <p className="text-base text-slate-300 font-medium">Three simple steps to safe, preview-only AI code reviews.</p>
        </div>
        <div className="flex items-center gap-3 flex-wrap">
          {allSet && collapsed && (
            <Button variant="primary" size="sm" icon={<Icons.Add />} onClick={onNewReview}>
              New Review
            </Button>
          )}
          <Button
            variant="ghost"
            size="sm"
            onClick={toggleCollapsed}
            aria-label={collapsed ? 'Expand getting started' : 'Minimize getting started'}
            title={collapsed ? 'Expand' : 'Minimize'}
            icon={
              <span className={classNames('inline-block transition-transform', collapsed ? 'rotate-0' : 'rotate-180')}>
                <Icons.ChevronDown />
              </span>
            }
          >
            {collapsed ? 'Expand' : 'Minimize'}
          </Button>
          {onDismiss && (
            <Button
              variant="ghost"
              size="sm"
              onClick={onDismiss}
              aria-label="Hide get started forever"
              title="Don't show again"
              icon={<Icons.Delete />}
            >
              Don't show again
            </Button>
          )}
        </div>
      </div>

      {!collapsed && (
      <div className="space-y-2">
        {/* Safety Banner */}
        <SafetyBanner variant="detailed" className="mb-4" />
        
        <Step
          title="Step 1: Configure AI"
          description="Add at least one AI provider (e.g., OpenAI, Gemini, or Ollama) to generate code review comments."
          done={hasAIProvider}
          action={
            !hasAIProvider && (
              <Button size="sm" variant="outline" icon={<Icons.AI />} onClick={onConfigureAI}>
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
                  <CodeBlock code={installCommandWindows} />
                </>
              ) : (
                <>
                  <p className="mt-2 text-sm text-slate-300">Manual installation:</p>
                  <CodeBlock code="curl -fsSL https://hexmos.com/lrc-install.sh | bash" />
                  <p className="mt-2 text-xs text-slate-400">Then configure with: echo 'api_key = \"your-api-key\"\napi_url = \"http://localhost:8888\"' > ~/.lrc.toml</p>
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
              <CodeBlock code="lrc review" />
              {!allSet && (
                <p className="mt-2 text-sm text-amber-400">Complete steps 1 and 2 first</p>
              )}
            </>
          }
          done={hasRunReview}
          isLast
        />
      </div>
      )}
    </div>
  );
};

export default OnboardingStepper;
