import React, { useState } from 'react';
import classNames from 'classnames';
import { Icons } from '../UIPrimitives';

interface SafetyBannerProps {
  variant?: 'compact' | 'detailed';
  className?: string;
}

export const SafetyBanner: React.FC<SafetyBannerProps> = ({ 
  variant = 'detailed', 
  className 
}) => {
  const [expandedImage, setExpandedImage] = useState<string | null>(null);
  
  const images = ['/assets/lr_cli1.png', '/assets/lr_cli2.png'];

  // Handle escape key to close modal
  React.useEffect(() => {
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && expandedImage) {
        setExpandedImage(null);
      }
    };
    window.addEventListener('keydown', handleEscape);
    return () => window.removeEventListener('keydown', handleEscape);
  }, [expandedImage]);

  if (variant === 'compact') {
    return (
      <div className={classNames(
        'flex items-center gap-2.5 px-4 py-2.5 rounded-lg bg-cyan-900/20 border border-cyan-400/30',
        className
      )}>
        <svg className="w-5 h-5 text-cyan-400 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
        </svg>
        <span className="text-sm font-medium text-cyan-200">
          ⚡ Instant CLI Mode • One command • ~30sec results • Nothing posted
        </span>
      </div>
    );
  }

  return (
    <div className={classNames(
      'rounded-lg bg-gradient-to-br from-blue-900/30 via-purple-900/20 to-cyan-900/30 border border-blue-400/30 p-4 sm:p-5 w-full',
      className
    )}>
      <div className="flex flex-col lg:flex-row lg:items-start gap-4 lg:gap-5">
        {/* Column 1 - Description (50%) */}
        <div className="w-full lg:basis-1/2 flex items-start gap-3 sm:gap-4">
          <div className="flex-shrink-0 mt-1">
            <svg className="w-7 h-7 text-cyan-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
            </svg>
          </div>
          <div className="flex-1">
            <div className="flex items-start flex-wrap gap-2 mb-3">
              <h4 className="text-lg font-bold text-cyan-300">
                ⚡ Instant CLI Mode
              </h4>
              <span className="text-xs font-semibold px-2 py-0.5 rounded-full bg-cyan-500/20 text-cyan-300 border border-cyan-400/30">
                Get started in 2 minutes
              </span>
            </div>
            
            <div className="space-y-2.5 text-sm text-slate-200">
              <div className="flex items-start gap-2.5">
                <svg className="w-4 h-4 text-cyan-400 mt-0.5 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                </svg>
                <span><strong className="text-white">One command</strong> to review your working git changes</span>
              </div>
              
              <div className="flex items-start gap-2.5">
                <svg className="w-4 h-4 text-cyan-400 mt-0.5 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                </svg>
                <span><strong className="text-white">Get comments in ~30 seconds</strong> in a custom-built UI</span>
              </div>
              
              <div className="flex items-start gap-2.5">
                <svg className="w-4 h-4 text-cyan-400 mt-0.5 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                </svg>
                <span><strong className="text-white">Nothing is posted</strong> — safe preview only, you decide</span>
              </div>
              
              <div className="flex items-start gap-2.5">
                <svg className="w-4 h-4 text-cyan-400 mt-0.5 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                </svg>
                <span><strong className="text-white">Optional:</strong> Connect git providers later for MR commenting, conversations & learning</span>
              </div>
            </div>
            
            <p className="mt-3 text-xs text-slate-300 italic">
              Minimal effort to start. Full features when you're ready.
            </p>
          </div>
        </div>

        {/* Column 2 - Image 1 (25%) */}
        <div className="w-full lg:basis-1/4 flex-shrink-0">
          <button
            type="button"
            className="group w-full rounded border border-slate-500/70 hover:border-cyan-400 transition-colors bg-slate-800/60 hover:bg-slate-800 p-3 h-32 sm:h-36 lg:h-40 flex items-center justify-center relative"
            onClick={() => setExpandedImage(images[0])}
            aria-label="Enlarge CLI command screenshot"
          >
            <img
              src={images[0]}
              alt="CLI Review Command"
              className="max-h-full w-auto rounded"
            />
            <div className="absolute inset-0 rounded pointer-events-none bg-gradient-to-t from-black/20 via-transparent to-transparent opacity-0 group-hover:opacity-100 transition-opacity" />
            <div className="absolute bottom-1 right-2 text-[10px] uppercase tracking-wide text-slate-200/80 group-hover:text-white bg-black/50 px-1.5 py-0.5 rounded-sm">
              Click to enlarge
            </div>
          </button>
        </div>

        {/* Column 3 - Image 2 (25%) */}
        <div className="w-full lg:basis-1/4 flex-shrink-0">
          <button
            type="button"
            className="group w-full rounded border border-slate-500/70 hover:border-cyan-400 transition-colors bg-slate-800/60 hover:bg-slate-800 p-3 h-32 sm:h-36 lg:h-40 flex items-center justify-center relative"
            onClick={() => setExpandedImage(images[1])}
            aria-label="Enlarge CLI output screenshot"
          >
            <img
              src={images[1]}
              alt="CLI Review Output"
              className="max-h-full w-auto rounded"
            />
            <div className="absolute inset-0 rounded pointer-events-none bg-gradient-to-t from-black/20 via-transparent to-transparent opacity-0 group-hover:opacity-100 transition-opacity" />
            <div className="absolute bottom-1 right-2 text-[10px] uppercase tracking-wide text-slate-200/80 group-hover:text-white bg-black/50 px-1.5 py-0.5 rounded-sm">
              Click to enlarge
            </div>
          </button>
        </div>
      </div>
      
      {/* Image Modal */}
      {expandedImage && (
        <div 
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4"
          onClick={() => setExpandedImage(null)}
        >
          <div className="relative max-w-6xl max-h-[90vh] w-full">
            <button
              onClick={() => setExpandedImage(null)}
              className="absolute -top-12 right-0 text-white hover:text-cyan-400 transition-colors"
              aria-label="Close (or press Escape)"
            >
              <svg className="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
            <img 
              src={expandedImage} 
              alt="Expanded view" 
              className="w-full h-auto rounded-lg shadow-2xl border border-cyan-400/50"
              onClick={(e) => e.stopPropagation()}
            />
            <p className="text-center text-slate-300 text-sm mt-3">Press ESC to close</p>
          </div>
        </div>
      )}
    </div>
  );
};

export default SafetyBanner;
