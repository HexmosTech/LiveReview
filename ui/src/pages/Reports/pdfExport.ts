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
  userEmail: string;
  avgFindingsPerReview: number;
  reposCovered: number;
  coveragePeriodLabel: string;
  isSuperAdmin: boolean;
}

// A single restrained palette, consistent throughout the document.
const DARK: [number, number, number] = [15, 23, 42]; // slate-900 - headings, table headers, primary text
const ACCENT: [number, number, number] = [37, 99, 235]; // blue-600 - the one accent color
const SLATE: [number, number, number] = [100, 116, 139]; // slate-500 - secondary text
const SLATE_LIGHT: [number, number, number] = [148, 163, 184]; // slate-400 - muted header text
const LIGHT_BG: [number, number, number] = [248, 250, 252]; // slate-50 - box fill
const BORDER: [number, number, number] = [226, 232, 240]; // slate-200

const RED: [number, number, number] = [185, 28, 28]; // red-700 - critical
const AMBER: [number, number, number] = [180, 83, 9]; // amber-700 - warning

const PAGE_W = 210;
const PAGE_H = 297;
const MARGIN = 15;
const CONTENT_W = PAGE_W - MARGIN * 2;
const HEADER_H = 28;

const formatDate = (d: string): string => {
  if (!d) return '';
  const dt = new Date(d);
  if (Number.isNaN(dt.getTime())) return d;
  return dt.toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' });
};

const formatRange = (filters: Filters): string => {
  const since = filters.since ? formatDate(filters.since) : '';
  const until = filters.until ? formatDate(filters.until) : '';
  return since && until ? `${since} - ${until}` : since ? `Since ${since}` : until ? `Through ${until}` : 'All time';
};

const formatChartLabel = (bucket: string, grain: string): string => {
  const dt = new Date(bucket);
  if (Number.isNaN(dt.getTime())) return bucket;
  if (grain === 'month') return dt.toLocaleDateString('en-US', { month: 'short', year: '2-digit' });
  return dt.toLocaleDateString('en-US', { month: 'short', day: 'numeric' });
};

// Recreates the LiveReview "eye" mark using vector primitives so it renders crisply at any size.
const drawLogoMark = (doc: jsPDF, cx: number, cy: number, r: number) => {
  doc.setFillColor(17, 24, 39);
  doc.circle(cx, cy, r, 'F');
  doc.setDrawColor(255, 255, 255);
  doc.setLineWidth(r * 0.1);
  doc.circle(cx, cy, r * 0.85, 'S');
  doc.setFillColor(...ACCENT);
  doc.circle(cx, cy, r * 0.48, 'F');
  doc.setFillColor(255, 255, 255);
  doc.circle(cx - r * 0.16, cy - r * 0.16, r * 0.09, 'F');
};

const drawLabeledLine = (doc: jsPDF, label: string, value: string, rightX: number, y: number) => {
  doc.setFont('helvetica', 'normal');
  doc.setFontSize(9);
  const valueText = value || '-';
  doc.setTextColor(255, 255, 255);
  const valueW = doc.getTextWidth(valueText);
  doc.text(valueText, rightX, y, { align: 'right' });
  doc.setFont('helvetica', 'bold');
  doc.setTextColor(...SLATE_LIGHT);
  doc.text(label, rightX - valueW - 1.5, y, { align: 'right' });
};

const drawHeader = (doc: jsPDF, input: PdfExportInput) => {
  doc.setFillColor(...DARK);
  doc.rect(0, 0, PAGE_W, HEADER_H, 'F');

  doc.setFillColor(...ACCENT);
  doc.rect(0, HEADER_H - 1, PAGE_W, 1, 'F');

  drawLogoMark(doc, MARGIN + 4, 12, 4);

  doc.setFont('helvetica', 'bold');
  doc.setFontSize(18);
  doc.setTextColor(255, 255, 255);
  doc.text('LiveReview', MARGIN + 10.5, 13.5);

  doc.setFont('helvetica', 'normal');
  doc.setFontSize(10.5);
  doc.setTextColor(...SLATE_LIGHT);
  doc.text('Impact Report', MARGIN + 10.5, 20.5);

  const range = formatRange(input.filters);

  const rightX = PAGE_W - MARGIN;
  drawLabeledLine(doc, 'Org: ', input.orgName || 'Organization', rightX, 8);
  drawLabeledLine(doc, 'Email: ', input.userEmail || '-', rightX, 13.5);

  doc.setFont('helvetica', 'normal');
  doc.setFontSize(9);
  doc.setTextColor(...SLATE_LIGHT);
  doc.text(range, rightX, 19, { align: 'right' });
  doc.text(`Generated ${formatDate(new Date().toISOString())}`, rightX, 25, { align: 'right' });
};

const drawBarChart = (doc: jsPDF, rows: FilledTrendRow[], x: number, y: number, w: number, h: number, grain: string) => {
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
  doc.setFillColor(...ACCENT);
  rows.forEach((r, i) => {
    const barH = (r.count / maxCount) * h;
    if (barH <= 0) return;
    const bx = x + i * barWidth + barGap / 2;
    const bw = Math.max(0.2, barWidth - barGap);
    doc.rect(bx, y + h - barH, bw, barH, 'F');
  });

  doc.setFont('helvetica', 'normal');
  doc.setFontSize(6.5);
  doc.setTextColor(...SLATE);
  const sampleLabel = formatChartLabel(rows[Math.floor(rows.length / 2)].bucket, grain);
  const labelW = doc.getTextWidth(sampleLabel);
  const maxLabels = Math.max(2, Math.floor(w / (labelW + 4)));
  const step = Math.max(1, Math.ceil(rows.length / maxLabels));
  for (let i = 0; i < rows.length; i += step) {
    const lx = x + i * barWidth + barWidth / 2;
    doc.text(formatChartLabel(rows[i].bucket, grain), lx, y + h + 4, { align: 'center' });
  }
  if ((rows.length - 1) % step !== 0) {
    const lastIdx = rows.length - 1;
    const lx = x + lastIdx * barWidth + barWidth / 2;
    doc.text(formatChartLabel(rows[lastIdx].bucket, grain), lx, y + h + 4, { align: 'center' });
  }
};

const sectionHeading = (doc: jsPDF, text: string, y: number) => {
  doc.setFillColor(...ACCENT);
  doc.rect(MARGIN, y - 3.6, 1.2, 4.2, 'F');
  doc.setFont('helvetica', 'bold');
  doc.setFontSize(13);
  doc.setTextColor(...DARK);
  doc.text(text, MARGIN + 4, y);
};

const drawPeriodBanner = (doc: jsPDF, input: PdfExportInput, y: number): number => {
  const bannerH = 14;
  doc.setFillColor(...LIGHT_BG);
  doc.setDrawColor(...BORDER);
  doc.roundedRect(MARGIN, y, CONTENT_W, bannerH, 1.5, 1.5, 'FD');
  doc.setFillColor(...ACCENT);
  doc.rect(MARGIN, y, 1.2, bannerH, 'F');

  doc.setFont('helvetica', 'normal');
  doc.setFontSize(7.5);
  doc.setTextColor(...SLATE);
  doc.text('REPORTING PERIOD', MARGIN + 5, y + 5.5);

  doc.setFont('helvetica', 'bold');
  doc.setFontSize(13);
  doc.setTextColor(...DARK);
  doc.text(formatRange(input.filters), MARGIN + 5, y + 11);

  doc.setFont('helvetica', 'normal');
  doc.setFontSize(9.5);
  doc.setTextColor(...SLATE);
  doc.text(`${input.coveragePeriodLabel} covered`, PAGE_W - MARGIN - 5, y + 8.5, { align: 'right' });

  return y + bannerH + 8;
};

const drawMetricBoxes = (doc: jsPDF, boxes: { label: string; value: string }[], y: number): number => {
  const gap = 4;
  const boxW = (CONTENT_W - gap * (boxes.length - 1)) / boxes.length;
  const boxH = 21;
  boxes.forEach((b, i) => {
    const x = MARGIN + i * (boxW + gap);
    doc.setFillColor(...LIGHT_BG);
    doc.setDrawColor(...BORDER);
    doc.roundedRect(x, y, boxW, boxH, 1.5, 1.5, 'FD');
    doc.setFont('helvetica', 'bold');
    doc.setFontSize(15);
    doc.setTextColor(...DARK);
    doc.text(b.value, x + 3, y + 12);
    doc.setFont('helvetica', 'normal');
    doc.setFontSize(7.5);
    doc.setTextColor(...SLATE);
    doc.text(b.label.toUpperCase(), x + 3, y + 17.5);
  });
  return y + boxH + 4;
};

export const generateImpactReportPdf = (input: PdfExportInput) => {
  const doc = new jsPDF({ orientation: 'portrait', unit: 'mm', format: 'a4' });
  drawHeader(doc, input);

  let y = HEADER_H + 8;
  y = drawPeriodBanner(doc, input, y);

  sectionHeading(doc, 'Executive Summary', y);
  y += 6;

  const totalFindings = input.summary.total_findings;
  const criticalCount = input.summary.critical_count;
  const criticalPct = totalFindings > 0 ? Math.round((criticalCount / totalFindings) * 100) : 0;

  const headlineBoxes = [
    { label: 'Total Findings', value: totalFindings.toLocaleString() },
    { label: 'Critical', value: totalFindings > 0 ? `${criticalCount.toLocaleString()} (${criticalPct}%)` : criticalCount.toLocaleString() },
    { label: 'Avg Findings / Review', value: input.avgFindingsPerReview.toLocaleString() },
  ];
  y = drawMetricBoxes(doc, headlineBoxes, y);

  const coverageBoxes = [
    { label: 'Reviews Performed', value: input.summary.total_reviews.toLocaleString() },
    { label: 'Repos Covered', value: input.reposCovered.toLocaleString() },
    { label: 'Period Covered', value: input.coveragePeriodLabel },
  ];
  y = drawMetricBoxes(doc, coverageBoxes, y);
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
    didParseCell: (data) => {
      if (data.section === 'body' && data.column.index === 0) {
        const val = String(data.cell.raw);
        if (val === 'Critical') data.cell.styles.textColor = RED;
        else if (val.startsWith('Medium')) data.cell.styles.textColor = AMBER;
        else if (val === 'Info') data.cell.styles.textColor = SLATE;
        data.cell.styles.fontStyle = 'bold';
      }
    },
  });
  y = (doc as any).lastAutoTable.finalY + 12;

  if (y > PAGE_H - 70) {
    doc.addPage();
    y = MARGIN;
  }
  sectionHeading(doc, `Finding Volume Trend (by ${input.filters.grain || 'day'})`, y);
  y += 6;
  const chartH = 45;
  drawBarChart(doc, input.filledTrend, MARGIN + 6, y, CONTENT_W - 6, chartH, input.filters.grain || 'day');
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
    headStyles: { fillColor: DARK, textColor: 255, fontSize: 9 },
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
    headStyles: { fillColor: DARK, textColor: 255, fontSize: 9 },
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
