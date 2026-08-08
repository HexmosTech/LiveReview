import { CronExpressionParser } from 'cron-parser';

/**
 * Computes the next N run timestamps (UTC) for a cron expression, for the "preview upcoming
 * runs" list in the schedule editor. Returns an empty array for an invalid/empty expression
 * rather than throwing, so callers can render "—" without a try/catch of their own.
 */
export function getNextRuns(cronExpression: string, count = 7): Date[] {
  const expr = cronExpression.trim();
  if (!expr) return [];
  try {
    const interval = CronExpressionParser.parse(expr, { tz: 'UTC' });
    const runs: Date[] = [];
    for (let i = 0; i < count; i++) {
      runs.push(interval.next().toDate());
    }
    return runs;
  } catch {
    return [];
  }
}
