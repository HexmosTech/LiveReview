// Ported from git-lrc:internal/staticserve/static/components/SunburstChart.js (the
// leaf-count-proportional radial partition — same layout algorithm as FlameGraph.tsx,
// just polar instead of Cartesian) as of the git-lrc HEAD current when this port was
// written. Rebuilt as plain SVG + React state instead of D3 selections/arc generators.
import React, { useMemo, useState } from 'react';
import { BlastRadiusSymbolContribution } from '../../../types/reviews';
import { buildHierarchy, CallHierarchyNode, emptyCallGraphMessage } from '../../../lib/blastRadius';

const DEPTH_COLORS = ['#b30000', '#d94f2b', '#e07b39', '#e2a545', '#d9c14a'];
const ROOT_RADIUS = 34;

interface Segment {
  node: CallHierarchyNode;
  depth: number;
  startAngle: number;
  endAngle: number;
  innerR: number;
  outerR: number;
}

function leafSum(node: CallHierarchyNode): number {
  if (!node.children || node.children.length === 0) return 1;
  return node.children.reduce((s, c) => s + leafSum(c), 0);
}

function maxDepth(node: CallHierarchyNode): number {
  if (!node.children || node.children.length === 0) return 0;
  return 1 + Math.max(...node.children.map(maxDepth));
}

function polarToCartesian(cx: number, cy: number, r: number, angle: number) {
  return { x: cx + r * Math.sin(angle), y: cy - r * Math.cos(angle) };
}

// A full-circle (>=360°) segment's start and end points are the same
// coordinate, which SVG treats as a zero-length arc and draws nothing — the
// most common trigger is a single-child node at some depth (a linear caller
// chain), which is a very ordinary shape for a call hierarchy, not an edge
// case. Split anything at or near a full turn into two half-circle arcs,
// the standard SVG workaround, before falling through to the normal path.
function arcPath(cx: number, cy: number, innerR: number, outerR: number, startAngle: number, endAngle: number): string {
  const FULL_TURN = Math.PI * 2;
  if (endAngle - startAngle >= FULL_TURN - 1e-6) {
    const mid = startAngle + Math.PI;
    return [arcPath(cx, cy, innerR, outerR, startAngle, mid), arcPath(cx, cy, innerR, outerR, mid, startAngle + FULL_TURN)].join(' ');
  }
  const startOuter = polarToCartesian(cx, cy, outerR, startAngle);
  const endOuter = polarToCartesian(cx, cy, outerR, endAngle);
  const startInner = polarToCartesian(cx, cy, innerR, endAngle);
  const endInner = polarToCartesian(cx, cy, innerR, startAngle);
  const largeArc = endAngle - startAngle > Math.PI ? 1 : 0;
  return [
    `M ${startOuter.x} ${startOuter.y}`,
    `A ${outerR} ${outerR} 0 ${largeArc} 1 ${endOuter.x} ${endOuter.y}`,
    `L ${startInner.x} ${startInner.y}`,
    `A ${innerR} ${innerR} 0 ${largeArc} 0 ${endInner.x} ${endInner.y}`,
    'Z',
  ].join(' ');
}

function layoutSegments(root: CallHierarchyNode, maxRadius: number): { segments: Segment[]; ringWidth: number } {
  const depthH = Math.max(1, maxDepth(root));
  const ringWidth = (maxRadius - ROOT_RADIUS) / depthH;
  const segments: Segment[] = [];

  function renderLevel(nodes: CallHierarchyNode[], depth: number, startAngle: number, endAngle: number) {
    const span = endAngle - startAngle;
    if (span <= 0 || nodes.length === 0) return;
    const totalLeaves = nodes.reduce((s, n) => s + leafSum(n), 0);
    if (totalLeaves === 0) return;

    let a = startAngle;
    const innerR = ROOT_RADIUS + ringWidth * depth;
    const outerR = innerR + ringWidth;
    for (const node of nodes) {
      const lc = leafSum(node);
      const nodeSpan = (lc / totalLeaves) * span;
      segments.push({ node, depth, startAngle: a, endAngle: a + nodeSpan, innerR, outerR });
      if (node.children && node.children.length > 0) {
        renderLevel(node.children, depth + 1, a, a + nodeSpan);
      }
      a += nodeSpan;
    }
  }

  renderLevel(root.children, 0, 0, Math.PI * 2);
  return { segments, ringWidth };
}

interface SunburstChartProps {
  symbol?: BlastRadiusSymbolContribution;
  width: number;
  height: number;
  // Controlled hover, shared with the caller list outside this chart (see
  // BlastRadiusPanel's hoveredCaller) — git-lrc cross-highlights a caller
  // chip and its chart segment together in both directions. Falls back to
  // uncontrolled internal state when omitted.
  hovered?: string | null;
  onHover?: (name: string | null) => void;
}

const SunburstChart: React.FC<SunburstChartProps> = ({ symbol, width, height, hovered: hoveredProp, onHover }) => {
  const [internalHovered, setInternalHovered] = useState<string | null>(null);
  const hovered = onHover ? hoveredProp ?? null : internalHovered;
  const setHovered = onHover || setInternalHovered;
  const cx = width / 2;
  const cy = height / 2;
  const maxRadius = Math.min(width, height) / 2 - 8;

  const layout = useMemo(() => {
    if (!symbol || !symbol.Callers || symbol.Callers.length === 0) return null;
    const root = buildHierarchy(symbol);
    return layoutSegments(root, maxRadius);
  }, [symbol, maxRadius]);

  if (!symbol || !symbol.Callers || symbol.Callers.length === 0) {
    return <div className="flex items-center justify-center text-xs text-slate-500" style={{ width, height }}>{emptyCallGraphMessage(symbol)}</div>;
  }
  if (!layout) return null;

  const rootName = symbol.Name || symbol.QualifiedName;

  return (
    <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`}>
      {layout.segments.map((seg, i) => {
        const dimmed = hovered !== null && hovered !== seg.node.qualifiedName;
        return (
          <path
            key={`${seg.node.qualifiedName}-${i}`}
            d={arcPath(cx, cy, seg.innerR, seg.outerR, seg.startAngle, seg.endAngle)}
            fill={DEPTH_COLORS[seg.depth % DEPTH_COLORS.length]}
            opacity={dimmed ? 0.25 : 0.9}
            stroke="#1a1a1a"
            strokeWidth={0.75}
            onMouseEnter={() => setHovered(seg.node.qualifiedName)}
            onMouseLeave={() => setHovered(null)}
          >
            <title>{seg.node.qualifiedName}</title>
          </path>
        );
      })}
      <circle cx={cx} cy={cy} r={ROOT_RADIUS} fill="#7a0000" stroke="#6b0000" />
      <text x={cx} y={cy + 4} textAnchor="middle" fontSize={11} fill="#d4a070" pointerEvents="none">
        {rootName.length > 12 ? `${rootName.slice(0, 11)}…` : rootName}
      </text>
    </svg>
  );
};

export default SunburstChart;
