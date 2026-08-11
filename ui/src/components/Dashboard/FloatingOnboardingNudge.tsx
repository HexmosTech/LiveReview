import React, { useEffect, useRef, useState } from 'react';
import classNames from 'classnames';
import { Button, Icons } from '../UIPrimitives';
import { OnboardingSteps } from './OnboardingSteps';
import { SafetyBanner } from '../SafetyBanner/SafetyBanner';
import { setNudgeOccupying } from '../../store/uiLayout';

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
  // Forces the banner open (expanded, ignoring a prior "Close"/"Never Show Again") — used when
  // deep-linked here from the mega menu's "Trigger via CLI" entry.
  forceOpen?: boolean;
}

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
  forceOpen = false,
}) => {
  const [expanded, setExpanded] = useState(false);
  const [closedForSession, setClosedForSession] = useState(false);
  const visible = useHideOnScrollDown(expanded);

  useEffect(() => {
    if (forceOpen) {
      setExpanded(true);
      setClosedForSession(false);
    }
  }, [forceOpen]);
  // Report whether the bar is currently occupying the bottom of the viewport
  // so the global floating chat button can sit above it (or drop back down
  // when the bar is closed or scrolled away).
  const rendered = !closedForSession;
  useEffect(() => {
    setNudgeOccupying(rendered && visible);
    return () => setNudgeOccupying(false);
  }, [rendered, visible]);

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
            <div className="max-h-[65vh] overflow-y-auto pt-5 pb-2">
              <OnboardingSteps
                hasCLI={hasCLI}
                hasAIProvider={hasAIProvider}
                hasRunReview={hasRunReview}
                installCommand={installCommand}
                installCommandWindows={installCommandWindows}
                onConfigureAI={onConfigureAI}
                isFreePlan={isFreePlan}
                onUpgrade={onUpgrade}
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
              <span className={classNames('flex-shrink-0 text-slate-400 transition-transform', expanded ? '' : 'rotate-180')}>
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
