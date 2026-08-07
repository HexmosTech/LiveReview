// Ported from git-lrc:internal/staticserve/static/components/FlameGraph.js (the
// leaf-count-proportional icicle layout algorithm — renderFlameGraph's renderLevel) as
// of the git-lrc HEAD current when this port was written. Rebuilt as plain SVG + React
// state instead of D3 selections/transitions, since LiveReview doesn't otherwise depend
// on D3 and this layout doesn't need d3-hierarchy — it's a direct recursive partition.
import React, { useEffect, useMemo, useState } from 'react';
import { BlastRadiusSymbolContribution } from '../../../types/reviews';
import { buildHierarchy, CallHierarchyNode, emptyCallGraphMessage } from '../../../lib/blastRadius';

const PAD_L = 4;
const PAD_R = 4;
const PAD_TB = 8;
const DEPTH_COLORS = ['#b30000', '#d94f2b', '#e07b39', '#e2a545', '#d9c14a'];

interface Bar {
  node: CallHierarchyNode;
  x: number;
  y: number;
  width: number;
  height: number;
  depth: number;
}

function leafSum(node: CallHierarchyNode): number {
  if (!node.children || node.children.length === 0) return 1;
  return node.children.reduce((s, c) => s + leafSum(c), 0);
}

function maxDepth(node: CallHierarchyNode): number {
  if (!node.children || node.children.length === 0) return 0;
  return 1 + Math.max(...node.children.map(maxDepth));
}

function layoutBars(root: CallHierarchyNode, width: number, height: number): { bars: Bar[]; rootBar: Bar } {
  const depthH = maxDepth(root);
  const totalLevels = depthH + 2;
  const usableH = height - PAD_TB * 2;
  const barGap = Math.max(2, Math.round(usableH * 0.03));
  const barH = Math.max(16, Math.floor((usableH - barGap * totalLevels) / totalLevels));
  const usableW = width - PAD_L - PAD_R;

  const bars: Bar[] = [];

  function renderLevel(nodes: CallHierarchyNode[], depth: number, x0: number, x1: number, bottomY: number) {
    const totalW = x1 - x0;
    if (totalW <= 0 || nodes.length === 0) return;
    const totalLeaves = nodes.reduce((s, n) => s + leafSum(n), 0);
    if (totalLeaves === 0) return;

    let cx = x0;
    const y = bottomY - barH;
    for (const node of nodes) {
      const lc = leafSum(node);
      const w = (lc / totalLeaves) * totalW;
      if (w >= 2) {
        bars.push({ node, x: PAD_L + cx, y, width: w, height: barH, depth });
      }
      if (node.children && node.children.length > 0) {
        renderLevel(node.children, depth + 1, cx, cx + w, y - barGap);
      }
      cx += w;
    }
  }

  const bottomY = height - PAD_TB;
  const rootY = bottomY - barH;
  renderLevel(root.children, 0, 0, usableW, rootY - barGap);

  return { bars, rootBar: { node: root, x: PAD_L, y: rootY, width: usableW, height: barH, depth: 0 } };
}

interface FlameGraphProps {
  symbol?: BlastRadiusSymbolContribution;
  width: number;
  height: number;
  // See SunburstChart's matching prop — controlled hover shared with the
  // caller list for cross-highlighting.
  hovered?: string | null;
  onHover?: (name: string | null) => void;
}

const FlameGraph: React.FC<FlameGraphProps> = ({ symbol, width, height, hovered: hoveredProp, onHover }) => {
  const [internalHovered, setInternalHovered] = useState<string | null>(null);
  const hovered = onHover ? hoveredProp ?? null : internalHovered;
  const setHovered = onHover || setInternalHovered;

  const layout = useMemo(() => {
    if (!symbol || !symbol.Callers || symbol.Callers.length === 0) return null;
    const root = buildHierarchy(symbol);
    return layoutBars(root, width, height);
  }, [symbol, width, height]);

  // Entrance animation — git-lrc's D3 join grows each bar's width from 0 with
  // `.transition().duration(300).delay((d,i) => i*8)` (FlameGraph.js:110), a
  // staggered fade+grow-in from the left edge. Replicated as a CSS
  // transition on width+opacity (no D3 dependency), same per-index 8ms
  // stagger, re-triggered whenever the layout changes (symbol nav, sort mode).
  const [mounted, setMounted] = useState(false);
  useEffect(() => {
    setMounted(false);
    const raf = requestAnimationFrame(() => setMounted(true));
    return () => cancelAnimationFrame(raf);
  }, [layout]);

  if (!symbol || !symbol.Callers || symbol.Callers.length === 0) {
    return <div className="flex items-center justify-center text-xs text-slate-500" style={{ width, height }}>{emptyCallGraphMessage(symbol)}</div>;
  }
  if (!layout) return null;

  const { bars, rootBar } = layout;

  return (
    <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`}>
      {bars.map((bar, i) => {
        const dimmed = hovered !== null && hovered !== bar.node.qualifiedName;
        return (
          <g key={bar.node.qualifiedName + bar.x + bar.depth}>
            <rect
              x={bar.x}
              y={bar.y}
              height={bar.height}
              fill={DEPTH_COLORS[bar.depth % DEPTH_COLORS.length]}
              stroke="#3d0000"
              strokeWidth={1}
              style={{
                width: mounted ? bar.width : 0,
                opacity: mounted ? (dimmed ? 0.25 : 0.92) : 0,
                transition: `width 300ms cubic-bezier(0.22, 1, 0.36, 1) ${i * 8}ms, opacity 300ms ease ${i * 8}ms`,
              }}
              onMouseEnter={() => setHovered(bar.node.qualifiedName)}
              onMouseLeave={() => setHovered(null)}
            >
              <title>{bar.node.qualifiedName}</title>
            </rect>
            {bar.width > 40 && (
              <text
                x={bar.x + bar.width / 2}
                y={bar.y + bar.height / 2 + 4}
                textAnchor="middle"
                fontSize={Math.max(9, Math.min(12, bar.height * 0.55))}
                fill="#f0d8c8"
                pointerEvents="none"
              >
                {bar.node.name}
              </text>
            )}
          </g>
        );
      })}
      <rect x={rootBar.x} y={rootBar.y} width={rootBar.width} height={rootBar.height} fill="#7a0000" stroke="#6b0000" />
      <text
        x={rootBar.x + rootBar.width / 2}
        y={rootBar.y + rootBar.height / 2 + 4}
        textAnchor="middle"
        fontSize={Math.max(9, Math.min(12, rootBar.height * 0.55))}
        fill="#d4a070"
        pointerEvents="none"
      >
        {rootBar.node.name}
      </text>
    </svg>
  );
};

export default FlameGraph;
