import React, { useCallback, useEffect, useRef, useState, useMemo } from 'react';
import { VegaEmbed } from 'react-vega';
import type { EmbedOptions, Result } from 'vega-embed';
import type { View } from 'vega';

interface InteractiveChartProps {
  spec: Record<string, unknown>;
  width?: number;
  height?: number;
  className?: string;
  onViewReady?: (view: View) => void;
}

// The backend's spec assembly forces a white background, meant for PNGs that
// may be shared onto a light surface (Slack, a downloaded file opened
// elsewhere). Rendered live inside this app's dark UI that reads as a stray
// white card, so here we swap it for a solid dark card color instead - not
// transparent, so the chart (and any PNG downloaded from this view, since
// download reads the already-rendered Vega view) stays legible regardless of
// what it ends up on top of. DARK_CHART_CONFIG below supplies the matching
// axis/legend/title colors. In fill-container mode we also inject
// autosize:{ type:'fit', contains:'padding' } so the measured pixel width is
// treated as the TOTAL chart width (including Vega's own axis/label
// padding) — this prevents the chart from overflowing its container and
// causing a horizontal scrollbar.
const CHART_BACKGROUND = '#1e293b'; // slate-800, matches the app's card surfaces

function forBrowser(
  spec: Record<string, unknown>,
  fillContainer: boolean,
): Record<string, unknown> {
  return fillContainer
    ? { ...spec, background: CHART_BACKGROUND, autosize: { type: 'fit', contains: 'padding' } }
    : { ...spec, background: CHART_BACKGROUND };
}

// Mirrors the app's slate/indigo dark theme (see tailwind slate-* classes
// used throughout Chatbot.tsx) so the chart reads as part of the page
// instead of a pasted-in light-mode picture. This only sets Vega-Lite
// *defaults* (config) - any color/format the model's spec sets explicitly on
// an encoding channel still wins.
const DARK_CHART_CONFIG = {
  background: CHART_BACKGROUND,
  view: { stroke: 'transparent' },
  axis: {
    domainColor: '#475569', // slate-600
    gridColor: '#334155', // slate-700
    tickColor: '#475569',
    labelColor: '#cbd5e1', // slate-300
    titleColor: '#e2e8f0', // slate-200
  },
  legend: {
    labelColor: '#cbd5e1',
    titleColor: '#e2e8f0',
  },
  title: { color: '#e2e8f0' },
  header: { labelColor: '#cbd5e1', titleColor: '#e2e8f0' },
  range: {
    category: ['#818cf8', '#34d399', '#fbbf24', '#f87171', '#38bdf8', '#c084fc', '#fb923c', '#4ade80'],
  },
  mark: { tooltip: true, color: '#818cf8' }, // indigo-400, matches the app's accent
};

// Renders a Vega-Lite spec live in the browser (tooltips, hover, legend
// filtering) instead of a backend-rendered PNG. actions is disabled because
// the surrounding chat UI supplies its own download affordance.
export const InteractiveChart: React.FC<InteractiveChartProps> = ({
  spec,
  width,
  height,
  className,
  onViewReady,
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  // measuredWidth is null until the ResizeObserver fires.
  // We hold off rendering until we have a real non-zero pixel value so Vega
  // never gets width=0 (which would produce a blank chart).
  const [measuredWidth, setMeasuredWidth] = useState<number | null>(null);

  const fillContainer = !width;

  useEffect(() => {
    if (!fillContainer) return; // explicit width provided (expanded modal) – skip
    const el = containerRef.current;
    if (!el) return;

    const observer = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (entry) {
        const w = Math.floor(entry.contentRect.width);
        if (w > 0) setMeasuredWidth(w);
      }
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, [fillContainer]);

  const patchedSpec = useMemo(
    () => forBrowser(spec, fillContainer),
    [spec, fillContainer],
  );

  // The spec's own authored width/height (600x340 from the backend) define
  // its intended aspect ratio. Without this, only width ever got resized
  // (to the container, or to the expand-modal box) while height stayed
  // fixed at the spec's original value - stretching the chart instead of
  // scaling it, which is what made it look squished.
  const specAspectRatio =
    typeof spec.width === 'number' && typeof spec.height === 'number' && spec.height > 0
      ? spec.width / spec.height
      : undefined;

  // Effective size: derive whichever of width/height isn't pinned from the
  // spec's aspect ratio, so the chart scales instead of stretching.
  //  - Fill-container (inline chat card): width comes from the ResizeObserver;
  //    height is computed to match, so the whole box fills width but keeps shape.
  //  - Explicit width+height (expand modal): contain-fit within that box
  //    instead of stretching to exactly fill it.
  //  - Anything else (aspect ratio unknown, or only one dimension given):
  //    fall back to the previous caller-supplied/undefined behavior.
  const { effectiveWidth, effectiveHeight } = useMemo(() => {
    if (fillContainer) {
      if (measuredWidth == null) return { effectiveWidth: undefined, effectiveHeight: undefined };
      const h = specAspectRatio ? Math.round(measuredWidth / specAspectRatio) : height;
      return { effectiveWidth: measuredWidth, effectiveHeight: h };
    }
    if (width && height && specAspectRatio) {
      const heightAtFullWidth = width / specAspectRatio;
      if (heightAtFullWidth <= height) {
        return { effectiveWidth: width, effectiveHeight: Math.round(heightAtFullWidth) };
      }
      const widthAtFullHeight = height * specAspectRatio;
      return { effectiveWidth: Math.round(widthAtFullHeight), effectiveHeight: height };
    }
    return { effectiveWidth: width, effectiveHeight: height };
  }, [fillContainer, measuredWidth, width, height, specAspectRatio]);

  const options: EmbedOptions = useMemo(
    () => ({
      actions: false,
      renderer: 'svg',
      tooltip: true,
      // mark.tooltip: true makes Vega-Lite auto-populate the hover tooltip from
      // every encoded channel - the backend's spec doesn't declare an explicit
      // `tooltip` encoding, so without this override hovering a mark shows
      // nothing and the chart reads as a static picture.
      config: DARK_CHART_CONFIG,
      ...(effectiveWidth ? { width: effectiveWidth } : {}),
      ...(effectiveHeight ? { height: effectiveHeight } : {}),
    }),
    [effectiveWidth, effectiveHeight],
  );

  const handleEmbed = useCallback(
    (result: Result) => {
      onViewReady?.(result.view);
    },
    [onViewReady],
  );

  return (
    <div ref={containerRef} style={fillContainer ? { width: '100%' } : undefined}>
      {/* Only render once we have a real container width; avoids Vega measuring 0px */}
      {effectiveWidth !== undefined && (
        <VegaEmbed
          spec={patchedSpec as any}
          options={options}
          onEmbed={handleEmbed}
          className={className}
        />
      )}
    </div>
  );
};

export async function downloadChartView(view: View | null, filename: string): Promise<void> {
  if (!view) {
    alert('Chart is still loading — try again in a moment.');
    return;
  }
  try {
    const url = await view.toImageURL('png', 2);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    a.remove();
  } catch {
    alert('Could not download the chart.');
  }
}
