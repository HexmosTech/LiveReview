import React, { useEffect, useState, useMemo, useCallback } from 'react';
import classNames from 'classnames';
import { getCronText } from './cronUtils';
import {
  localTimeZoneName,
  convertHourlyMinutes,
  convertDailyTime,
  convertWeeklyCombos,
  convertMonthlyCombos,
  getLocalCronText,
  isSimpleCronField,
} from './cronTimezone';

// Ported from https://github.com/vpfaiz/cron-builder-ui (MIT), restyled to this app's palette. Operates in local time; crosses to/from UTC only via cronTimezone's converters.

export interface CronBuilderProps {
  onChange: (cronExpression: string) => void;
  defaultValue?: string;
  className?: string;
}

interface ScheduleTypeOption {
  value: string;
  label: string;
}

interface ParsedCron {
  type: string;
  values: {
    minutes?: number[];
    hours?: number[];
    daysOfMonth?: number[];
    daysOfWeek?: number[];
    custom?: string;
  };
}

const SCHEDULE_TYPES: ScheduleTypeOption[] = [
  { value: 'hour', label: 'Hourly' },
  { value: 'day', label: 'Daily' },
  { value: 'week', label: 'Weekly' },
  { value: 'month', label: 'Monthly' },
  { value: 'custom', label: 'Custom' },
];

const MINUTES = Array.from({ length: 60 }, (_, i) => i);
const HOURS = Array.from({ length: 24 }, (_, i) => i);
const DAYS_OF_MONTH = Array.from({ length: 31 }, (_, i) => i + 1);
const COMMON_MINUTES = [0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55];
const DAYS_SHORT = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

const gridButtonClass = (isSelected: boolean): string =>
  classNames(
    'min-w-[36px] px-2 py-1 text-xs rounded border transition-colors',
    isSelected
      ? 'bg-blue-600 text-white border-blue-600'
      : 'bg-slate-800 text-slate-200 border-slate-600 hover:bg-slate-700'
  );

interface GridButtonProps {
  value: number;
  isSelected: boolean;
  onClick: (value: number) => void;
}

const GridButton = React.memo<GridButtonProps>(({ value, isSelected, onClick }) => (
  <button type="button" onClick={() => onClick(value)} className={gridButtonClass(isSelected)}>
    {value.toString().padStart(2, '0')}
  </button>
));
GridButton.displayName = 'GridButton';

// Schedule-type selector — a plain button row standing in for the original's Radix ToggleGroup.
const ScheduleTypeSelector: React.FC<{ value: string; onChange: (value: string) => void }> = ({ value, onChange }) => (
  <div className="inline-flex flex-wrap items-center gap-1 rounded-lg border border-slate-700 bg-slate-900/40 p-1" role="tablist">
    {SCHEDULE_TYPES.map((type) => {
      const active = type.value === value;
      return (
        <button
          key={type.value}
          type="button"
          role="tab"
          aria-selected={active}
          onClick={() => onChange(type.value)}
          className={classNames(
            'rounded-md px-3 py-1.5 text-xs font-medium transition-colors',
            active ? 'bg-blue-600 text-white' : 'text-slate-300 hover:bg-slate-700 hover:text-white'
          )}
        >
          {type.label}
        </button>
      );
    })}
  </div>
);

const cleanArray = (values: number[] | undefined): number[] => (values || []).filter((v) => v !== undefined && v !== null);

export function CronBuilder({ onChange, defaultValue, className }: CronBuilderProps) {
  const defaultSchedule = defaultValue || '0 9 * * *';
  const zoneName = useMemo(() => localTimeZoneName(), []);

  // Cron fields on the wire are always UTC - convert to local right away.
  const parseCronExpression = (cronExpr: string): ParsedCron => {
    if (!cronExpr || cronExpr === '') return { type: 'day', values: {} };

    const cleanExpr = cronExpr.trim();
    const parts = cleanExpr.split(' ');
    if (parts.length !== 5) return { type: 'custom', values: { custom: cleanExpr } };

    const [min, hour, dom, month, dow] = parts;
    // No friendly-picker type can represent a month restriction, so treat it as Custom.
    if (month !== '*' && month !== '?') return { type: 'custom', values: { custom: cleanExpr } };
    // Step/range/named syntax (e.g. */5) isn't representable either - same reason.
    if (![min, hour, dom, dow].every(isSimpleCronField)) return { type: 'custom', values: { custom: cleanExpr } };

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
        return { type: 'hour', values: { minutes } };
      } else if (min !== '*' && hour !== '*' && dom === '*' && dow === '*') {
        const { hours, minutes } = convertDailyTime(parseNumbers(hour), parseNumbers(min), 'toLocal');
        return { type: 'day', values: { hours, minutes } };
      } else if (min !== '*' && hour !== '*' && dom === '*' && dow !== '*') {
        const { daysOfWeek, hours, minutes } = convertWeeklyCombos(parseNumbers(dow), parseNumbers(hour), parseNumbers(min), 'toLocal');
        return { type: 'week', values: { daysOfWeek, hours, minutes } };
      } else if (min !== '*' && hour !== '*' && dom !== '*' && dow === '*') {
        const { daysOfMonth, hours, minutes } = convertMonthlyCombos(parseNumbers(dom), parseNumbers(hour), parseNumbers(min), 'toLocal');
        return { type: 'month', values: { daysOfMonth, hours, minutes } };
      } else {
        return { type: 'custom', values: { custom: cleanExpr } };
      }
    } catch (error) {
      return { type: 'custom', values: { custom: cleanExpr } };
    }
  };

  const initialParsed = parseCronExpression(defaultSchedule);

  const [scheduleType, setScheduleType] = useState(initialParsed.type);
  const [minutes, setMinutes] = useState<number[]>(initialParsed.values.minutes || [0]);
  const [hours, setHours] = useState<number[]>(initialParsed.values.hours || [9]);
  const [daysOfMonth, setDaysOfMonth] = useState<number[]>(initialParsed.values.daysOfMonth || [1]);
  const [daysOfWeek, setDaysOfWeek] = useState<number[]>(initialParsed.values.daysOfWeek || [1]);
  const [custom, setCustom] = useState<string>(initialParsed.values.custom || defaultSchedule);
  const [showAllMinutes, setShowAllMinutes] = useState(false);

  function loadDefaults() {
    setMinutes([0]);
    setHours([9]);
    setDaysOfMonth([1]);
    setDaysOfWeek([1]);
  }

  // Local picker state -> UTC cron string; Custom mode is untouched (already raw UTC cron).
  const buildExpression = useMemo((): string => {
    if (scheduleType === 'custom') return custom || '';

    const cleanMinutes = cleanArray(minutes);
    const cleanHours = cleanArray(hours);
    const cleanDaysOfMonth = cleanArray(daysOfMonth);
    const cleanDaysOfWeek = cleanArray(daysOfWeek);

    switch (scheduleType) {
      case 'hour': {
        const utcMinutes = convertHourlyMinutes(cleanMinutes, 'toUTC');
        const minutesCSV = utcMinutes.length === 60 ? '*' : utcMinutes.join(',');
        return `${minutesCSV} * * * *`;
      }
      case 'day': {
        const { hours: utcHours, minutes: utcMinutes } = convertDailyTime(cleanHours, cleanMinutes, 'toUTC');
        const hoursCSV = utcHours.length === 24 ? '*' : utcHours.join(',');
        const minutesCSV = utcMinutes.length === 60 ? '*' : utcMinutes.join(',');
        return `${minutesCSV} ${hoursCSV} * * *`;
      }
      case 'week': {
        const { daysOfWeek: utcDow, hours: utcHours, minutes: utcMinutes } = convertWeeklyCombos(cleanDaysOfWeek, cleanHours, cleanMinutes, 'toUTC');
        const dowCSV = utcDow.length === 7 ? '*' : utcDow.join(',');
        const hoursCSV = utcHours.length === 24 ? '*' : utcHours.join(',');
        const minutesCSV = utcMinutes.length === 60 ? '*' : utcMinutes.join(',');
        return `${minutesCSV} ${hoursCSV} * * ${dowCSV}`;
      }
      case 'month': {
        const { daysOfMonth: utcDom, hours: utcHours, minutes: utcMinutes } = convertMonthlyCombos(cleanDaysOfMonth, cleanHours, cleanMinutes, 'toUTC');
        const domCSV = utcDom.length === 31 ? '*' : utcDom.join(',');
        const hoursCSV = utcHours.length === 24 ? '*' : utcHours.join(',');
        const minutesCSV = utcMinutes.length === 60 ? '*' : utcMinutes.join(',');
        return `${minutesCSV} ${hoursCSV} ${domCSV} * *`;
      }
      default:
        return '';
    }
  }, [scheduleType, minutes, hours, daysOfMonth, daysOfWeek, custom]);

  useEffect(() => {
    onChange(getCronText(buildExpression).status ? buildExpression : '');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [buildExpression]);

  const handleDayOfWeekToggle = useCallback((dayIndex: number) => {
    setDaysOfWeek((prev) => (prev.includes(dayIndex) ? prev.filter((d) => d !== dayIndex) : [...prev, dayIndex]));
  }, []);

  const renderDaysOfWeekList = () => {
    const weekendDays = [0, 6];
    return (
      <div className="flex flex-col gap-2">
        <label className="text-xs font-medium text-slate-300">Days of Week</label>
        <div className="flex flex-row flex-wrap gap-1">
          {DAYS_SHORT.map((day, index) => {
            const isWeekend = weekendDays.includes(index);
            const isSelected = daysOfWeek.includes(index);
            return (
              <button
                key={index}
                type="button"
                onClick={() => handleDayOfWeekToggle(index)}
                className={classNames(
                  'min-w-[50px] px-3 py-1 text-center text-xs rounded border transition-colors',
                  isSelected
                    ? 'bg-blue-600 text-white border-blue-600'
                    : classNames('bg-slate-800 border-slate-600 hover:bg-slate-700', isWeekend ? 'text-orange-400' : 'text-slate-200')
                )}
              >
                {day}
              </button>
            );
          })}
        </div>
      </div>
    );
  };

  const handleDayOfMonthToggle = useCallback((day: number) => {
    setDaysOfMonth((prev) => (prev.includes(day) ? prev.filter((d) => d !== day) : [...prev, day]));
  }, []);

  const renderDaysOfMonthGrid = () => (
    <div className="flex flex-col gap-2">
      <label className="text-xs font-medium text-slate-300">Days of Month</label>
      <div className="grid grid-cols-7 gap-1 w-fit">
        {DAYS_OF_MONTH.map((day) => (
          <GridButton key={day} value={day} isSelected={daysOfMonth.includes(day)} onClick={handleDayOfMonthToggle} />
        ))}
      </div>
    </div>
  );

  const handleHourToggle = useCallback((hour: number) => {
    setHours((prev) => (prev.includes(hour) ? prev.filter((h) => h !== hour) : [...prev, hour]));
  }, []);

  const renderHoursGrid = () => (
    <div className="flex flex-col gap-2">
      <label className="text-xs font-medium text-slate-300">Hours</label>
      <div className="grid grid-cols-6 gap-1 w-fit">
        {HOURS.map((hour) => (
          <GridButton key={hour} value={hour} isSelected={hours.includes(hour)} onClick={handleHourToggle} />
        ))}
      </div>
    </div>
  );

  const handleMinuteToggle = useCallback((minute: number) => {
    setMinutes((prev) => (prev.includes(minute) ? prev.filter((m) => m !== minute) : [...prev, minute]));
  }, []);

  const minutesToShow = useMemo(() => (showAllMinutes ? MINUTES : COMMON_MINUTES), [showAllMinutes]);

  const renderMinutesGrid = () => (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2">
        <label className="text-xs font-medium text-slate-300">Minutes</label>
        <span className="text-xs text-slate-500">({showAllMinutes ? 'All' : 'Common'})</span>
        <button
          type="button"
          onClick={() => setShowAllMinutes(!showAllMinutes)}
          className="rounded border border-slate-600 px-1.5 text-xs text-slate-300 hover:bg-slate-700"
          aria-label={showAllMinutes ? 'Show fewer minutes' : 'Show more minutes'}
        >
          {showAllMinutes ? '−' : '+'}
        </button>
      </div>
      <div className={classNames('grid gap-1 w-fit', showAllMinutes ? 'grid-cols-12' : 'grid-cols-6')}>
        {minutesToShow.map((minute) => (
          <GridButton key={minute} value={minute} isSelected={minutes.includes(minute)} onClick={handleMinuteToggle} />
        ))}
      </div>
    </div>
  );

  const renderScheduleFields = () => {
    if (scheduleType === 'week') {
      return (
        <div className="flex flex-col gap-4">
          {renderDaysOfWeekList()}
          <div className="flex flex-wrap gap-6">
            {renderHoursGrid()}
            {renderMinutesGrid()}
          </div>
        </div>
      );
    }
    if (scheduleType === 'month') {
      return (
        <div className="flex flex-col gap-4">
          <div className="flex flex-wrap gap-6">{renderDaysOfMonthGrid()}</div>
          <div className="flex flex-wrap gap-6">
            {renderHoursGrid()}
            {renderMinutesGrid()}
          </div>
        </div>
      );
    }
    if (scheduleType === 'day') {
      return (
        <div className="flex flex-wrap gap-6">
          {renderHoursGrid()}
          {renderMinutesGrid()}
        </div>
      );
    }
    if (scheduleType === 'hour') {
      return <div className="flex flex-wrap gap-6">{renderMinutesGrid()}</div>;
    }
    return null;
  };

  return (
    <div className={classNames('flex flex-col gap-3', className)}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <ScheduleTypeSelector
          value={scheduleType}
          onChange={(value) => {
            loadDefaults();
            setScheduleType(value);
          }}
        />
        {scheduleType !== 'custom' && <span className="text-xs text-slate-500">All times in {zoneName}</span>}
      </div>

      <div className="flex flex-col gap-4">
        {scheduleType === 'custom' ? (
          <div className="flex flex-col gap-2 rounded-md border border-slate-700 bg-slate-900/40 p-3">
            <label htmlFor="custom-schedule" className="text-xs font-medium text-slate-300">
              Cron Expression (UTC)
            </label>
            <input
              type="text"
              id="custom-schedule"
              value={custom}
              onChange={(event) => setCustom(event.target.value)}
              className="h-9 w-full max-w-xs rounded-md border border-slate-600 bg-slate-800 px-3 py-1 text-sm text-slate-100 placeholder:text-slate-500 focus:outline-none focus:ring-1 focus:ring-blue-500 sm:w-auto"
              placeholder="0 9 * * *"
            />
          </div>
        ) : (
          renderScheduleFields()
        )}

        {(() => {
          if (!getCronText(buildExpression).status) {
            return <p className="rounded-md bg-red-950/40 p-3 text-sm text-red-300">Invalid cron expression</p>;
          }
          // Described in local time, not the underlying UTC field values.
          return <p className="rounded-md bg-slate-900/40 p-3 text-sm text-slate-200">{getLocalCronText(buildExpression).value}</p>;
        })()}
      </div>
    </div>
  );
}

export default CronBuilder;
