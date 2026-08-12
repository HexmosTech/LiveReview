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

// The backend's spec assembly forces a white background for PNG rendering;
// we keep it here as-is since we also want a white background in the browser.
// In fill-container mode we also inject autosize:{ type:'fit', contains:'padding' }
// so the measured pixel width is treated as the TOTAL chart width (including
// Vega's own axis/label padding) — this prevents the chart from overflowing
// its container and causing a horizontal scrollbar.
function forBrowser(
  spec: Record<string, unknown>,
  fillContainer: boolean,
): Record<string, unknown> {
  return fillContainer
    ? { ...spec, background: 'white', autosize: { type: 'fit', contains: 'padding' } }
    : { ...spec, background: 'white' };
}

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

  // Effective width: caller-supplied > measured container width > skip (let Vega default).
  const effectiveWidth = width ?? measuredWidth ?? undefined;

  const options: EmbedOptions = useMemo(
    () => ({
      actions: false,
      renderer: 'svg',
      tooltip: true,
      // mark.tooltip: true makes Vega-Lite auto-populate the hover tooltip from
      // every encoded channel - the backend's spec doesn't declare an explicit
      // `tooltip` encoding, so without this override hovering a mark shows
      // nothing and the chart reads as a static picture.
      config: { background: 'white', mark: { tooltip: true } },
      ...(effectiveWidth ? { width: effectiveWidth } : {}),
      ...(height ? { height } : {}),
    }),
    [effectiveWidth, height],
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
