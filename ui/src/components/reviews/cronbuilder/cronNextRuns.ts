import { CronExpressionParser } from 'cron-parser';

/** Computes the next N run timestamps (UTC); returns [] on invalid/empty input instead of throwing. */
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
