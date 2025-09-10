import React, { useState } from 'react';
import classNames from 'classnames';
import { Button, Icons, Tooltip } from '../UIPrimitives';

interface OnboardingStepperProps {
  hasGitProvider: boolean;
  hasAIProvider: boolean;
  onConnectGit: () => void;
  onConfigureAI: () => void;
  onNewReview: () => void;
  onDismiss?: () => void;
  className?: string;
}

const Step: React.FC<{
  title: string;
  description: string;
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
        <p className="text-sm text-slate-300 mt-0.5">{description}</p>
        {!isLast && <div className="h-5 border-l border-slate-700 ml-3 my-3" />}
      </div>
    </div>
  );
};

export const OnboardingStepper: React.FC<OnboardingStepperProps> = ({
  hasGitProvider,
  hasAIProvider,
  onConnectGit,
  onConfigureAI,
  onNewReview,
  onDismiss,
  className,
}) => {
  const allSet = hasGitProvider && hasAIProvider;
  const [collapsed, setCollapsed] = useState<boolean>(() => {
    try { return localStorage.getItem('lr_get_started_collapsed') === '1'; } catch { return false; }
  });

  const toggleCollapsed = () => {
    setCollapsed(prev => {
      const next = !prev;
      try { localStorage.setItem('lr_get_started_collapsed', next ? '1' : '0'); } catch {}
      return next;
    });
  };

  return (
    <div className={classNames('rounded-lg border border-slate-700 bg-slate-800/60 p-4', className)}>
      <div className="flex items-center justify-between mb-3">
        <div>
          <h3 className="text-base font-semibold text-white">Get started</h3>
          <p className="text-sm text-slate-300">Follow these steps to run your first AI-powered review.</p>
        </div>
        <div className="flex items-center gap-3 flex-wrap">
          {allSet && (
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
        <Step
          title="Step 1: Connect Git Provider"
          description="Link GitHub, GitLab or Bitbucket so LiveReview can access your repos."
          done={hasGitProvider}
          action={
            !hasGitProvider && (
              <Button size="sm" variant="outline" icon={<Icons.Git />} onClick={onConnectGit}>
                Connect
              </Button>
            )
          }
        />
        <Step
          title="Step 2: Configure AI"
          description="Add at least one AI provider (e.g., OpenAI) to generate code review comments."
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
          title="Step 3: Trigger New Review"
          description={
            allSet
              ? 'Everything is connected. Create your first review to see insights here.'
              : 'Once steps 1 and 2 are complete, you can run your first review.'
          }
          done={false}
          isLast
          action={
            allSet ? (
              <Button size="sm" variant="primary" icon={<Icons.Add />} onClick={onNewReview}>
                New Review
              </Button>
            ) : (
              <Tooltip content="Complete steps 1 and 2 to enable this action.">
                <span>
                  <Button size="sm" variant="outline" icon={<Icons.Add />} disabled>
                    New Review
                  </Button>
                </span>
              </Tooltip>
            )
          }
        />
      </div>
      )}
    </div>
  );
};

export default OnboardingStepper;
