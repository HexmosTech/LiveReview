import React, { useEffect, useMemo, useState } from 'react';
import { Button, Icons } from '../../UIPrimitives';
import { CronBuilder } from './CronBuilder';
import { getNextRuns } from './cronNextRuns';
import { localTimeZoneName } from './cronTimezone';

interface EditScheduleModalProps {
  /** Single repo's full_name, or a summary like "4 repositories" for a bulk edit. */
  subtitle: string;
  initialCronExpression: string;
  onClose: () => void;
  onSave: (cronExpression: string) => void;
  saving?: boolean;
}

// getNextRuns computes real absolute timestamps (parsed as UTC, since that's what the cron
// fields represent on the wire). Formatting here deliberately omits a `timeZone` override so
// the browser renders each one in the viewer's own local timezone - the IANA zone name (e.g.
// "Asia/Kolkata") is appended manually rather than via toLocaleString's `timeZoneName`
// option, which for many zones (India included) only has a "GMT+5:30"-style short form.
const formatRunDate = (date: Date, zone: string): string =>
  `${date.toLocaleString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })} ${zone}`;

// The parent (ScheduledReviews.tsx) shows/hides this via conditional rendering
// ({editingRepos && <EditScheduleModal .../>}), not by toggling a persistent `open` prop -
// so mount = opened, unmount = closed. That's why there's no `open` prop here: `initialCronExpression`
// is only ever read once, at mount, via useState's initializer, and the fade-in below just
// runs on mount. (There's deliberately no fade-out - closing unmounts immediately.)
export const EditScheduleModal: React.FC<EditScheduleModalProps> = ({
  subtitle,
  initialCronExpression,
  onClose,
  onSave,
  saving = false,
}) => {
  const [cronExpression, setCronExpression] = useState(initialCronExpression);
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    const id = requestAnimationFrame(() => setMounted(true));
    return () => cancelAnimationFrame(id);
  }, []);

  const nextRuns = useMemo(() => getNextRuns(cronExpression, 7), [cronExpression]);
  const zoneName = useMemo(() => localTimeZoneName(), []);

  return (
    <div
      className={`fixed inset-0 z-40 flex items-center justify-center bg-slate-900/70 p-4 transition-opacity duration-200 ${mounted ? 'opacity-100' : 'opacity-0'}`}
      onClick={onClose}
    >
      <div
        className={`w-full max-w-2xl transform rounded-md border border-slate-700 bg-slate-800 shadow-xl transition-all duration-200 ${mounted ? 'translate-y-0 opacity-100' : '-translate-y-1 opacity-0'}`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-slate-700 px-5 py-4">
          <div>
            <h2 className="text-lg font-semibold text-white">Edit Schedule</h2>
            <p className="text-sm text-slate-400">{subtitle}</p>
          </div>
          <button onClick={onClose} className="text-slate-400 hover:text-slate-200" aria-label="Close">
            <Icons.Close />
          </button>
        </div>

        <div className="max-h-[70vh] overflow-y-auto p-5">
          <CronBuilder defaultValue={cronExpression} onChange={setCronExpression} />

          <div className="mt-5">
            <h3 className="mb-2 text-xs font-medium uppercase tracking-wide text-slate-400">Next 7 Runs</h3>
            {nextRuns.length === 0 ? (
              <p className="text-sm text-slate-500">
                {cronExpression ? 'Unable to compute upcoming runs for this expression.' : 'No schedule configured.'}
              </p>
            ) : (
              <ul className="space-y-1">
                {nextRuns.map((date, i) => (
                  <li key={i} className="flex items-center gap-2 text-sm text-slate-300">
                    <span className="text-slate-500"><Icons.Clock /></span>
                    {formatRunDate(date, zoneName)}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-slate-700 px-5 py-4">
          <Button variant="ghost" size="sm" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" size="sm" isLoading={saving} onClick={() => onSave(cronExpression)} disabled={!cronExpression}>
            Save Schedule
          </Button>
        </div>
      </div>
    </div>
  );
};

export default EditScheduleModal;
