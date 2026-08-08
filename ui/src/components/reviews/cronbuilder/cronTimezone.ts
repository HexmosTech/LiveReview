// The cron strings this app builds/stores are always UTC (that's what the backend worker
// evaluates against), but the picker itself should let the user think in their own local
// time. These helpers convert between the two, leaning on the JS Date engine's own
// local<->UTC handling rather than manually computing offsets (so DST is handled correctly).

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

/**
 * Local day-of-week (0=Sun..6=Sat) + HH:MM -> the UTC day-of-week + HH:MM. Crossing midnight
 * against the local offset can shift the day-of-week by one - that's the whole reason this
 * needs a real Date instead of just converting the hour.
 */
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

/**
 * Local day-of-month (1-31) + HH:MM -> UTC day-of-month + HH:MM. Uses a 31-day reference
 * month so every day value is valid; a day near a month boundary can shift by one when
 * converted (e.g. local day 1 just after midnight can land on UTC day 28-31 of the prior
 * month) - acceptable for a monthly review cadence, where exact-midnight precision doesn't
 * matter as much as it would for, say, billing.
 */
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

/**
 * Converts every selected local minute to its UTC equivalent for the "hourly" schedule type
 * (fires every hour, so only the offset's minute component matters - the hour component
 * doesn't change "every hour").
 */
export function convertHourlyMinutes(minutes: number[], direction: 'toUTC' | 'toLocal'): number[] {
  const convert = direction === 'toUTC' ? localTimeToUTC : utcTimeToLocal;
  return sortedUnique(minutes.map((m) => convert({ hour: 0, minute: m }).minute));
}

/**
 * Converts the full cartesian set of selected local hour/minute combinations (the "daily"
 * schedule type) to their UTC equivalents. Multiple hours and minutes are independent
 * multi-selects in the UI, so each combination is converted individually and the resulting
 * hour/minute values are re-collapsed into two flat lists - exact for a single selected
 * hour+minute, a safe (if occasionally over-inclusive) approximation for multi-select
 * combined with a fractional-hour timezone offset.
 */
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

/**
 * Rewrites a UTC cron string's minute/hour/day fields into their local-time equivalents,
 * purely so cronstrue's description reads in local time - never used for actual scheduling
 * (that stays UTC). Falls back to returning the input unchanged for anything it can't
 * confidently re-map (e.g. a hand-typed Custom expression), so the description just
 * describes the literal (UTC) field values in that case instead of guessing wrong.
 */
export function utcCronToLocalCronString(cronExpr: string): string {
  const cleanExpr = cronExpr.trim();
  const parts = cleanExpr.split(' ');
  if (parts.length !== 5) return cleanExpr;

  const [min, hour, dom, month, dow] = parts;
  // None of the branches below know how to re-map a month restriction (and the friendly
  // picker never produces one), so bail out rather than silently drop it from the description.
  if (month !== '*' && month !== '?') return cleanExpr;

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
