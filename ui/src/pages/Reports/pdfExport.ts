import { jsPDF } from 'jspdf';
import autoTable from 'jspdf-autotable';
import type { Summary, DistRow, BreakdownRow, FilledTrendRow, Filters } from './TaxonomyReports';

export interface PdfExportInput {
  summary: Summary;
  filledTrend: FilledTrendRow[];
  categoryDist: DistRow[];
  breakdown: BreakdownRow[];
  filters: Filters;
  orgName: string;
  riskScore: number;
  avgFindingsPerReview: number;
  reposCovered: number;
  coveragePeriodLabel: string;
  isSuperAdmin: boolean;
}

const BRAND_BLUE: [number, number, number] = [59, 130, 246];
const DARK: [number, number, number] = [15, 23, 42];
const SLATE: [number, number, number] = [100, 116, 139];
const LIGHT_BG: [number, number, number] = [241, 245, 249];
const BORDER: [number, number, number] = [226, 232, 240];

const PAGE_W = 210;
const PAGE_H = 297;
const MARGIN = 15;
const CONTENT_W = PAGE_W - MARGIN * 2;

const formatDate = (d: string): string => {
  if (!d) return '';
  const dt = new Date(d);
  if (Number.isNaN(dt.getTime())) return d;
  return dt.toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' });
};

const drawHeader = (doc: jsPDF, input: PdfExportInput) => {
  doc.setFillColor(...DARK);
  doc.rect(0, 0, PAGE_W, 26, 'F');

  doc.setFont('helvetica', 'bold');
  doc.setFontSize(18);
  doc.setTextColor(255, 255, 255);
  doc.text('LiveReview', MARGIN, 12);

  doc.setFont('helvetica', 'normal');
  doc.setFontSize(10);
  doc.setTextColor(203, 213, 225);
  doc.text('Code Quality & Security Impact Report', MARGIN, 19);

  const since = input.filters.since ? formatDate(input.filters.since) : '';
  const until = input.filters.until ? formatDate(input.filters.until) : '';
  const range = since && until ? `${since} - ${until}` : since ? `Since ${since}` : until ? `Through ${until}` : 'All time';

  doc.setFontSize(9);
  doc.setTextColor(255, 255, 255);
  doc.text(input.orgName || 'Organization', PAGE_W - MARGIN, 11, { align: 'right' });
  doc.setTextColor(203, 213, 225);
  doc.text(range, PAGE_W - MARGIN, 17, { align: 'right' });
  doc.text(`Generated ${formatDate(new Date().toISOString())}`, PAGE_W - MARGIN, 23, { align: 'right' });
};

const drawBarChart = (doc: jsPDF, rows: FilledTrendRow[], x: number, y: number, w: number, h: number) => {
  if (rows.length === 0) {
    doc.setFont('helvetica', 'normal');
    doc.setFontSize(9);
    doc.setTextColor(...SLATE);
    doc.text('No trend data available for the selected range.', x, y + h / 2);
    return;
  }

  const maxCount = Math.max(1, ...rows.map((r) => r.count));

  doc.setDrawColor(...BORDER);
  doc.setLineWidth(0.1);
  doc.setFont('helvetica', 'normal');
  for (let i = 0; i <= 4; i++) {
    const gy = y + h - (h * i) / 4;
    doc.line(x, gy, x + w, gy);
    doc.setFontSize(7);
    doc.setTextColor(...SLATE);
    doc.text(String(Math.round((maxCount * i) / 4)), x - 2, gy + 1, { align: 'right' });
  }

  const barWidth = w / rows.length;
  const barGap = Math.min(0.6, barWidth * 0.2);
  doc.setFillColor(...BRAND_BLUE);
  rows.forEach((r, i) => {
    const barH = (r.count / maxCount) * h;
    if (barH <= 0) return;
    const bx = x + i * barWidth + barGap / 2;
    const bw = Math.max(0.2, barWidth - barGap);
    doc.rect(bx, y + h - barH, bw, barH, 'F');
  });

  doc.setFontSize(7);
  doc.setTextColor(...SLATE);
  const labelIdxs = Array.from(new Set([0, Math.floor((rows.length - 1) / 2), rows.length - 1]));
  labelIdxs.forEach((idx) => {
    const lx = x + idx * barWidth + barWidth / 2;
    doc.text(rows[idx].bucket, lx, y + h + 5, { align: 'center' });
  });
};

const buildTakeaways = (input: PdfExportInput): string[] => {
  const { summary, filledTrend, categoryDist, reposCovered } = input;
  const takeaways: string[] = [];
  const total = summary.total_findings;

  if (total > 0) {
    const pct = Math.round((summary.critical_count / total) * 100);
    takeaways.push(
      `${pct}% of all findings (${summary.critical_count.toLocaleString()} of ${total.toLocaleString()}) were Critical severity -- each one a potential issue caught before reaching production.`,
    );
  } else {
    takeaways.push('No findings were recorded in the selected period -- a strong signal of code health.');
  }

  if (filledTrend.length >= 2) {
    const mid = Math.ceil(filledTrend.length / 2);
    const firstHalf = filledTrend.slice(0, mid).reduce((s, r) => s + r.count, 0);
    const secondHalf = filledTrend.slice(mid).reduce((s, r) => s + r.count, 0);
    if (firstHalf === 0 && secondHalf === 0) {
      takeaways.push('Finding volume remained at zero throughout the period.');
    } else if (secondHalf < firstHalf) {
      const pct = Math.round(((firstHalf - secondHalf) / Math.max(1, firstHalf)) * 100);
      takeaways.push(
        `Finding volume decreased by ${pct}% in the latter half of the period, indicating improving code health as issues are caught and resolved earlier.`,
      );
    } else if (secondHalf > firstHalf) {
      const pct = Math.round(((secondHalf - firstHalf) / Math.max(1, firstHalf)) * 100);
      takeaways.push(
        `Finding volume increased by ${pct}% as review coverage expanded, surfacing more issues before they reach production.`,
      );
    } else {
      takeaways.push('Finding volume remained stable across the period.');
    }
  }

  if (categoryDist.length > 0 && total > 0) {
    const top = categoryDist[0];
    const pct = Math.round((top.count / total) * 100);
    takeaways.push(
      `"${top.value || 'Uncategorized'}" was the most common issue category (${pct}% of findings, ${top.count.toLocaleString()} issues) -- a focused target for remediation.`,
    );
  }

  if (summary.total_reviews > 0) {
    takeaways.push(
      `${summary.total_reviews.toLocaleString()} automated reviews were completed${reposCovered > 0 ? ` across ${reposCovered} repositor${reposCovered === 1 ? 'y' : 'ies'}` : ''}, providing continuous oversight without adding manual review burden.`,
    );
  }

  return takeaways;
};

const sectionHeading = (doc: jsPDF, text: string, y: number) => {
  doc.setFont('helvetica', 'bold');
  doc.setFontSize(13);
  doc.setTextColor(...DARK);
  doc.text(text, MARGIN, y);
};

const drawMetricBoxes = (doc: jsPDF, boxes: { label: string; value: string }[], y: number): number => {
  const gap = 4;
  const boxW = (CONTENT_W - gap * (boxes.length - 1)) / boxes.length;
  const boxH = 20;
  boxes.forEach((b, i) => {
    const x = MARGIN + i * (boxW + gap);
    doc.setFillColor(...LIGHT_BG);
    doc.setDrawColor(...BORDER);
    doc.roundedRect(x, y, boxW, boxH, 1.5, 1.5, 'FD');
    doc.setFont('helvetica', 'bold');
    doc.setFontSize(14);
    doc.setTextColor(...DARK);
    doc.text(b.value, x + 3, y + 11);
    doc.setFont('helvetica', 'normal');
    doc.setFontSize(7.5);
    doc.setTextColor(...SLATE);
    doc.text(b.label.toUpperCase(), x + 3, y + 16);
  });
  return y + boxH + 4;
};

export const generateImpactReportPdf = (input: PdfExportInput) => {
  const doc = new jsPDF({ orientation: 'portrait', unit: 'mm', format: 'a4' });
  drawHeader(doc, input);

  let y = 36;

  sectionHeading(doc, 'Executive Summary', y);
  y += 6;

  const totalFindings = input.summary.total_findings;
  const criticalCount = input.summary.critical_count;

  const coverageBoxes = [
    { label: 'Total Findings', value: totalFindings.toLocaleString() },
    { label: 'Automated Reviews', value: input.summary.total_reviews.toLocaleString() },
    { label: 'Repos Covered', value: input.reposCovered.toLocaleString() },
    { label: 'Period Covered', value: input.coveragePeriodLabel },
  ];
  y = drawMetricBoxes(doc, coverageBoxes, y);

  const riskBoxes = [
    { label: 'Avg Findings / Review', value: input.avgFindingsPerReview.toLocaleString() },
    {
      label: 'Critical',
      value: totalFindings > 0
        ? `${criticalCount.toLocaleString()} (${Math.round((criticalCount / totalFindings) * 100)}%)`
        : criticalCount.toLocaleString(),
    },
    { label: 'Risk Score', value: `${input.riskScore} / 100` },
  ];
  y = drawMetricBoxes(doc, riskBoxes, y);
  y += 4;

  const sevRows: [string, number][] = [
    ['Critical', input.summary.critical_count],
    ['Medium / Warning', input.summary.medium_count],
    ['Info', input.summary.info_count],
  ];
  autoTable(doc, {
    startY: y,
    margin: { left: MARGIN, right: MARGIN },
    head: [['Severity', 'Findings', '% of Total']],
    body: sevRows.map(([label, count]) => [
      label,
      count.toLocaleString(),
      totalFindings > 0 ? `${Math.round((count / totalFindings) * 100)}%` : '0%',
    ]),
    theme: 'grid',
    headStyles: { fillColor: DARK, textColor: 255, fontSize: 9 },
    bodyStyles: { fontSize: 9, textColor: DARK },
    columnStyles: { 1: { halign: 'right' }, 2: { halign: 'right' } },
  });
  y = (doc as any).lastAutoTable.finalY + 10;

  sectionHeading(doc, 'Key Takeaways', y);
  y += 6;

  doc.setFont('helvetica', 'normal');
  doc.setFontSize(9.5);
  doc.setTextColor(51, 65, 85);
  buildTakeaways(input).forEach((line) => {
    const wrapped = doc.splitTextToSize(line, CONTENT_W - 6);
    doc.text('-', MARGIN, y);
    doc.text(wrapped, MARGIN + 4, y);
    y += wrapped.length * 4.5 + 2;
  });
  y += 4;

  if (y > PAGE_H - 70) {
    doc.addPage();
    y = MARGIN;
  }
  sectionHeading(doc, `Finding Volume Trend (by ${input.filters.grain || 'day'})`, y);
  y += 6;
  const chartH = 45;
  drawBarChart(doc, input.filledTrend, MARGIN + 6, y, CONTENT_W - 6, chartH);
  y += chartH + 12;

  if (y > PAGE_H - 60) {
    doc.addPage();
    y = MARGIN;
  }
  sectionHeading(doc, 'Findings by Category', y);
  y += 6;
  autoTable(doc, {
    startY: y,
    margin: { left: MARGIN, right: MARGIN },
    head: [['Category', 'Findings', '% of Total']],
    body: input.categoryDist.slice(0, 10).map((r) => [
      r.value || '(empty)',
      r.count.toLocaleString(),
      totalFindings > 0 ? `${Math.round((r.count / totalFindings) * 100)}%` : '0%',
    ]),
    theme: 'striped',
    headStyles: { fillColor: BRAND_BLUE, textColor: 255, fontSize: 9 },
    bodyStyles: { fontSize: 9 },
    columnStyles: { 1: { halign: 'right' }, 2: { halign: 'right' } },
  });
  y = (doc as any).lastAutoTable.finalY + 10;

  if (y > PAGE_H - 60) {
    doc.addPage();
    y = MARGIN;
  }
  sectionHeading(doc, `Breakdown by ${input.isSuperAdmin ? 'Organization / ' : ''}Repository / Provider`, y);
  y += 6;
  const breakdownHead = input.isSuperAdmin
    ? ['Org', 'Repository', 'Provider', 'Findings', 'Reviews']
    : ['Repository', 'Provider', 'Findings', 'Reviews'];
  const breakdownBody = input.breakdown.slice(0, 25).map((r) => {
    const row = input.isSuperAdmin
      ? [r.org_name || '-', r.repository || '-', r.provider || '-']
      : [r.repository || '-', r.provider || '-'];
    return [...row, r.count.toLocaleString(), (r.review_count || 0).toLocaleString()];
  });
  autoTable(doc, {
    startY: y,
    margin: { left: MARGIN, right: MARGIN },
    head: [breakdownHead],
    body: breakdownBody,
    theme: 'striped',
    headStyles: { fillColor: BRAND_BLUE, textColor: 255, fontSize: 9 },
    bodyStyles: { fontSize: 8.5 },
  });

  const pageCount = doc.getNumberOfPages();
  for (let i = 1; i <= pageCount; i++) {
    doc.setPage(i);
    doc.setFont('helvetica', 'normal');
    doc.setFontSize(8);
    doc.setTextColor(...SLATE);
    doc.text('Generated by LiveReview - Confidential', MARGIN, PAGE_H - 8);
    doc.text(`Page ${i} of ${pageCount}`, PAGE_W - MARGIN, PAGE_H - 8, { align: 'right' });
  }

  const since = input.filters.since || 'all';
  const until = input.filters.until || new Date().toISOString().slice(0, 10);
  doc.save(`livereview-impact-report-${since}-to-${until}.pdf`);
};
