import React, { useEffect, useRef, useState } from 'react';
import classNames from 'classnames';
import { Button, Icons } from '../UIPrimitives';
import { SafetyBanner } from '../SafetyBanner/SafetyBanner';

interface FloatingOnboardingNudgeProps {
  hasCLI: boolean;
  hasAIProvider: boolean;
  hasRunReview?: boolean;
  installCommand?: string;
  installCommandWindows?: string;
  onConfigureAI: () => void;
  onNewReview: () => void;
  onDismiss: () => void;
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

// Hides the band on scroll-down (it's chrome, not content) and brings it
// back on scroll-up, so it never sits on top of the graphs while reading.
const useHideOnScrollDown = (paused: boolean): boolean => {
  const [visible, setVisible] = useState(true);
  const lastY = useRef(0);

  useEffect(() => {
    lastY.current = window.scrollY;
    let ticking = false;

    const onScroll = () => {
      if (ticking) return;
      ticking = true;
      requestAnimationFrame(() => {
        const y = window.scrollY;
        if (paused) {
          setVisible(true);
        } else if (y <= 24) {
          setVisible(true);
        } else if (y > lastY.current + 4) {
          setVisible(false);
        } else if (y < lastY.current - 4) {
          setVisible(true);
        }
        lastY.current = y;
        ticking = false;
      });
    };

    window.addEventListener('scroll', onScroll, { passive: true });
    return () => window.removeEventListener('scroll', onScroll);
  }, [paused]);

  return paused || visible;
};

export const FloatingOnboardingNudge: React.FC<FloatingOnboardingNudgeProps> = ({
  hasCLI,
  hasAIProvider,
  hasRunReview = false,
  installCommand,
  installCommandWindows,
  onConfigureAI,
  onDismiss,
  isFreePlan = false,
  onUpgrade,
}) => {
  const [expanded, setExpanded] = useState(false);
  const [closedForSession, setClosedForSession] = useState(false);
  const allSet = hasCLI && hasAIProvider;
  const visible = useHideOnScrollDown(expanded);

  if (closedForSession) return null;

  return (
    <div
      className={classNames(
        'fixed inset-x-0 bottom-0 z-40 transition-transform duration-300',
        visible ? 'translate-y-0' : 'translate-y-full'
      )}
    >
      <div className="border-t-2 border-blue-400/60 bg-gradient-to-r from-blue-950/95 via-slate-800/95 to-blue-950/95 backdrop-blur-md shadow-[0_-8px_30px_-6px_rgba(59,130,246,0.45)]">
        <div className="container mx-auto px-4">
          {expanded && (
            <div className="max-h-[65vh] overflow-y-auto pt-5 pb-2 space-y-2">
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
          )}

          <div className="flex flex-wrap items-center justify-between gap-3 py-3">
            <button
              type="button"
              onClick={() => setExpanded((prev) => !prev)}
              className="flex min-w-0 flex-1 items-center gap-2 text-left"
            >
              <span className="truncate text-sm font-bold text-transparent bg-clip-text bg-gradient-to-r from-blue-400 via-cyan-400 to-emerald-400 sm:text-base">
                🚀 Get your first review in 2 minutes
              </span>
              <span className={classNames('flex-shrink-0 text-slate-400 transition-transform', expanded ? 'rotate-180' : '')}>
                <Icons.ChevronDown />
              </span>
            </button>
            <div className="flex flex-shrink-0 items-center gap-2">
              <Button variant="ghost" size="sm" icon={<Icons.Delete />} onClick={onDismiss} title="Never show this again">
                Never Show Again
              </Button>
              <Button
                variant="secondary"
                size="sm"
                icon={<Icons.Close />}
                onClick={() => setClosedForSession(true)}
                title="Hide for now — reappears next time you load this page"
              >
                Close
              </Button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};

export default FloatingOnboardingNudge;
