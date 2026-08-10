// Cron strings are always UTC; these helpers convert to/from local time via JS Date so DST is handled correctly.

import { getCronText, CronTextResult } from './cronUtils';

export const localTimeZoneName = (): string => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone;
  } catch {
    return 'local time';
  }
};

export interface TimeOfDay {
  hour: number;
  minute: number;
}

/** Local HH:MM -> UTC HH:MM. Day-of-week/month independent, so a same-day reference works. */
export function localTimeToUTC({ hour, minute }: TimeOfDay): TimeOfDay {
  const ref = new Date();
  ref.setHours(hour, minute, 0, 0);
  return { hour: ref.getUTCHours(), minute: ref.getUTCMinutes() };
}

/** UTC HH:MM -> local HH:MM. */
export function utcTimeToLocal({ hour, minute }: TimeOfDay): TimeOfDay {
  const ref = new Date();
  ref.setUTCHours(hour, minute, 0, 0);
  return { hour: ref.getHours(), minute: ref.getMinutes() };
}

/** Local day-of-week + HH:MM -> UTC day-of-week + HH:MM; uses a real Date since the offset can shift the day. */
export function localWeeklyToUTC(dayOfWeek: number, time: TimeOfDay): { dayOfWeek: number } & TimeOfDay {
  const ref = new Date();
  ref.setDate(ref.getDate() + ((dayOfWeek - ref.getDay() + 7) % 7));
  ref.setHours(time.hour, time.minute, 0, 0);
  return { dayOfWeek: ref.getUTCDay(), hour: ref.getUTCHours(), minute: ref.getUTCMinutes() };
}

export function utcWeeklyToLocal(dayOfWeek: number, time: TimeOfDay): { dayOfWeek: number } & TimeOfDay {
  const ref = new Date();
  ref.setUTCDate(ref.getUTCDate() + ((dayOfWeek - ref.getUTCDay() + 7) % 7));
  ref.setUTCHours(time.hour, time.minute, 0, 0);
  return { dayOfWeek: ref.getDay(), hour: ref.getHours(), minute: ref.getMinutes() };
}

/** Local day-of-month + HH:MM -> UTC; a day near month boundary can shift by one, acceptable for this cadence. */
export function localMonthlyDayToUTC(day: number, time: TimeOfDay): { day: number } & TimeOfDay {
  const ref = new Date();
  ref.setMonth(0, day);
  ref.setHours(time.hour, time.minute, 0, 0);
  return { day: ref.getUTCDate(), hour: ref.getUTCHours(), minute: ref.getUTCMinutes() };
}

export function utcMonthlyDayToLocal(day: number, time: TimeOfDay): { day: number } & TimeOfDay {
  const ref = new Date();
  ref.setUTCMonth(0, day);
  ref.setUTCHours(time.hour, time.minute, 0, 0);
  return { day: ref.getDate(), hour: ref.getHours(), minute: ref.getMinutes() };
}

const sortedUnique = (values: number[]): number[] => Array.from(new Set(values)).sort((a, b) => a - b);

/** Converts selected local minutes to UTC for "hourly" - only the minute component matters. */
export function convertHourlyMinutes(minutes: number[], direction: 'toUTC' | 'toLocal'): number[] {
  const convert = direction === 'toUTC' ? localTimeToUTC : utcTimeToLocal;
  return sortedUnique(minutes.map((m) => convert({ hour: 0, minute: m }).minute));
}

/** Converts the cartesian set of local hour/minute combos to UTC for "daily"; can over-include for multi-select. */
export function convertDailyTime(hours: number[], minutes: number[], direction: 'toUTC' | 'toLocal'): { hours: number[]; minutes: number[] } {
  const convert = direction === 'toUTC' ? localTimeToUTC : utcTimeToLocal;
  const hourSet: number[] = [];
  const minuteSet: number[] = [];
  for (const h of hours) {
    for (const m of minutes) {
      const { hour, minute } = convert({ hour: h, minute: m });
      hourSet.push(hour);
      minuteSet.push(minute);
    }
  }
  return { hours: sortedUnique(hourSet), minutes: sortedUnique(minuteSet) };
}

/** Same idea as convertDailyTime, for the "weekly" schedule type's day-of-week + time. */
export function convertWeeklyCombos(
  daysOfWeek: number[],
  hours: number[],
  minutes: number[],
  direction: 'toUTC' | 'toLocal'
): { daysOfWeek: number[]; hours: number[]; minutes: number[] } {
  const convert = direction === 'toUTC' ? localWeeklyToUTC : utcWeeklyToLocal;
  const dowSet: number[] = [];
  const hourSet: number[] = [];
  const minuteSet: number[] = [];
  for (const d of daysOfWeek) {
    for (const h of hours) {
      for (const m of minutes) {
        const conv = convert(d, { hour: h, minute: m });
        dowSet.push(conv.dayOfWeek);
        hourSet.push(conv.hour);
        minuteSet.push(conv.minute);
      }
    }
  }
  return { daysOfWeek: sortedUnique(dowSet), hours: sortedUnique(hourSet), minutes: sortedUnique(minuteSet) };
}

/** Same idea as convertDailyTime, for the "monthly" schedule type's day-of-month + time. */
export function convertMonthlyCombos(
  daysOfMonth: number[],
  hours: number[],
  minutes: number[],
  direction: 'toUTC' | 'toLocal'
): { daysOfMonth: number[]; hours: number[]; minutes: number[] } {
  const convert = direction === 'toUTC' ? localMonthlyDayToUTC : utcMonthlyDayToLocal;
  const domSet: number[] = [];
  const hourSet: number[] = [];
  const minuteSet: number[] = [];
  for (const d of daysOfMonth) {
    for (const h of hours) {
      for (const m of minutes) {
        const conv = convert(d, { hour: h, minute: m });
        domSet.push(conv.day);
        hourSet.push(conv.hour);
        minuteSet.push(conv.minute);
      }
    }
  }
  return { daysOfMonth: sortedUnique(domSet), hours: sortedUnique(hourSet), minutes: sortedUnique(minuteSet) };
}

const numbersOrStar = (values: number[], wildcardLength: number): string =>
  values.length === wildcardLength ? '*' : values.join(',');

// A field this module can convert: '*'/'?' or a comma-separated list of integers - not step/range/named syntax.
export const isSimpleCronField = (part: string): boolean => part === '*' || part === '?' || /^\d+(,\d+)*$/.test(part);

/** Rewrites a UTC cron string to local time purely for cronstrue's description; falls back unchanged if it can't confidently re-map. */
export function utcCronToLocalCronString(cronExpr: string): string {
  const cleanExpr = cronExpr.trim();
  const parts = cleanExpr.split(' ');
  if (parts.length !== 5) return cleanExpr;

  const [min, hour, dom, month, dow] = parts;
  // None of the branches below can re-map a month restriction, so bail out rather than drop it.
  if (month !== '*' && month !== '?') return cleanExpr;
  if (![min, hour, dom, dow].every(isSimpleCronField)) return cleanExpr;

  const parseNumbers = (part: string): number[] => {
    if (part === '*' || part === '?') return [];
    return part
      .split(',')
      .map((v) => parseInt(v.trim(), 10))
      .filter((v) => !isNaN(v));
  };

  try {
    if (min !== '*' && hour === '*' && dom === '*' && dow === '*') {
      const minutes = convertHourlyMinutes(parseNumbers(min), 'toLocal');
      return `${numbersOrStar(minutes, 60)} * * * *`;
    }
    if (min !== '*' && hour !== '*' && dom === '*' && dow === '*') {
      const { hours, minutes } = convertDailyTime(parseNumbers(hour), parseNumbers(min), 'toLocal');
      return `${numbersOrStar(minutes, 60)} ${numbersOrStar(hours, 24)} * * *`;
    }
    if (min !== '*' && hour !== '*' && dom === '*' && dow !== '*') {
      const { daysOfWeek, hours, minutes } = convertWeeklyCombos(parseNumbers(dow), parseNumbers(hour), parseNumbers(min), 'toLocal');
      return `${numbersOrStar(minutes, 60)} ${numbersOrStar(hours, 24)} * * ${numbersOrStar(daysOfWeek, 7)}`;
    }
    if (min !== '*' && hour !== '*' && dom !== '*' && dow === '*') {
      const { daysOfMonth, hours, minutes } = convertMonthlyCombos(parseNumbers(dom), parseNumbers(hour), parseNumbers(min), 'toLocal');
      return `${numbersOrStar(minutes, 60)} ${numbersOrStar(hours, 24)} ${numbersOrStar(daysOfMonth, 31)} * *`;
    }
  } catch {
    // fall through to the unchanged-expression return below
  }
  return cleanExpr;
}

/** Human-readable text for a UTC cron string, worded in the viewer's local time. */
export function getLocalCronText(cronExpr: string): CronTextResult {
  return getCronText(utcCronToLocalCronString(cronExpr));
}
