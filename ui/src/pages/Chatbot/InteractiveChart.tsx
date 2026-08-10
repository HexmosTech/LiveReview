import React, { useCallback, useMemo } from 'react';
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

// The backend's spec assembly (internal/mcpagent/analytics.go, shared with the
// PNG path Slack/Discord/Teams still use) forces a white background as a
// workaround for vl-convert's standalone PNG rendering (see
// internal/vlrender's injectSafeFonts comment) - irrelevant here, since the
// chart renders directly on the page's own dark background. Strip it so our
// own transparent config wins.
function forBrowser(spec: Record<string, unknown>): Record<string, unknown> {
  const { background: _background, ...rest } = spec;
  return rest;
}

// Renders a Vega-Lite spec live in the browser (tooltips, hover, legend
// filtering) instead of a backend-rendered PNG. actions is disabled because
// the surrounding chat UI supplies its own download affordance.
export const InteractiveChart: React.FC<InteractiveChartProps> = ({ spec, width, height, className, onViewReady }) => {
  const patchedSpec = useMemo(() => forBrowser(spec), [spec]);

  const options: EmbedOptions = useMemo(
    () => ({
      actions: false,
      renderer: 'svg',
      tooltip: true,
      // mark.tooltip: true makes Vega-Lite auto-populate the hover tooltip from
      // every encoded channel - the backend's spec doesn't declare an explicit
      // `tooltip` encoding, so without this override hovering a mark shows
      // nothing and the chart reads as a static picture.
      config: { background: 'transparent', mark: { tooltip: true } },
      ...(width ? { width } : {}),
      ...(height ? { height } : {}),
    }),
    [width, height],
  );

  const handleEmbed = useCallback(
    (result: Result) => {
      onViewReady?.(result.view);
    },
    [onViewReady],
  );

  return <VegaEmbed spec={patchedSpec as any} options={options} onEmbed={handleEmbed} className={className} />;
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
