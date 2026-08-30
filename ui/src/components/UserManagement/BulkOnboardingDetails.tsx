import React, { useState } from 'react';
import { notify } from '../../utils/notify';
import { Button } from '../UIPrimitives';

export interface BulkOnboardingRow {
  email: string;
  name: string;
  role: string;
  status: 'invited' | 'updated';
  onboardingApiKey?: string;
}

interface BulkOnboardingDetailsProps {
  rows: BulkOnboardingRow[];
  onContinue: () => void;
}

const DEFAULT_VISIBLE_ROWS = 5;

export const BulkOnboardingDetails: React.FC<BulkOnboardingDetailsProps> = ({ rows, onContinue }) => {
  const [showAll, setShowAll] = useState(false);
  const visibleRows = showAll ? rows : rows.slice(0, DEFAULT_VISIBLE_ROWS);
  const hiddenCount = rows.length - visibleRows.length;

  const handleDownloadCSV = () => {
    const installUrl = window.location.origin;
    const headers = ['Email', 'Name', 'Linux/Mac Command', 'Windows Command'];
    const csvRows = rows.map((row) => {
      const installCmdLinux = `curl -fsSL https://hexmos.com/lrc-install.sh | LRC_API_KEY="${row.onboardingApiKey || ''}" LRC_API_URL="${installUrl}" bash`;
      const installCmdWindows = `$env:LRC_API_KEY="${row.onboardingApiKey || ''}"; $env:LRC_API_URL="${installUrl}"; iwr -useb https://hexmos.com/lrc-install.ps1 | iex`;
      return [row.email, row.name || row.email, installCmdLinux, installCmdWindows];
    });

    const escapeCsv = (val: string) => `"${val.replace(/"/g, '""')}"`;
    const csvContent = [
      headers.map(escapeCsv).join(','),
      ...csvRows.map((row) => row.map(escapeCsv).join(',')),
    ].join('\n');

    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.setAttribute('href', url);
    link.setAttribute('download', 'git-lrc-setup.csv');
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    notify.success(`git-lrc-setup.csv downloaded successfully for ${rows.length} user(s)!`);
  };

  return (
    <div className="p-6 bg-gray-900 text-white min-h-screen">
      <div className="max-w-2xl mx-auto bg-gray-800 p-8 rounded-lg border border-emerald-500/30 shadow-xl shadow-emerald-950/20">
        <div className="text-center mb-8">
          <div className="inline-flex items-center justify-center w-16 h-16 bg-emerald-500/10 text-emerald-400 rounded-full mb-4">
            <svg className="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
          </div>
          <h1 className="text-3xl font-bold text-emerald-400">
            {rows.length} User{rows.length !== 1 ? 's' : ''} Invited Successfully!
          </h1>
          <p className="text-gray-400 mt-2">
            An invitation email has been sent to {rows.length} user{rows.length !== 1 ? 's' : ''}. Optionally,
            download the Onboarding Command below and share it with them.
          </p>
        </div>

        <div className="space-y-6">
          <div className="bg-gray-900/50 p-5 rounded-md border border-gray-700">
            <h3 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">User Details</h3>
            <div className="divide-y divide-gray-700">
              {visibleRows.map((row) => (
                <div key={row.email} className="flex items-center justify-between py-2.5 text-sm gap-4">
                  <div className="min-w-0">
                    <p className="font-medium text-white truncate">{row.name || row.email}</p>
                    <p className="text-gray-400 text-xs truncate">{row.email}</p>
                  </div>
                  <div className="flex items-center gap-3 flex-shrink-0">
                    <span className="text-gray-300 capitalize">{row.role}</span>
                    <span className="text-xs px-2 py-0.5 rounded-full bg-emerald-900/30 text-emerald-300 capitalize">
                      {row.status}
                    </span>
                  </div>
                </div>
              ))}
            </div>

            {rows.length > DEFAULT_VISIBLE_ROWS && (
              <button
                type="button"
                onClick={() => setShowAll((prev) => !prev)}
                className="mt-3 text-sm text-blue-400 hover:text-blue-300"
              >
                {showAll ? 'Show less' : `Show all (${hiddenCount} more)`}
              </button>
            )}
          </div>
        </div>

        <div className="flex flex-col sm:flex-row justify-between gap-4 pt-6 mt-6 border-t border-gray-700">
          <Button
            variant="secondary"
            onClick={handleDownloadCSV}
            className="flex items-center justify-center space-x-2 w-full sm:w-auto"
          >
            <svg className="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
            </svg>
            Download Onboarding Command
          </Button>
          <Button onClick={onContinue} className="w-full sm:w-auto">
            Continue to Members
          </Button>
        </div>
      </div>
    </div>
  );
};
