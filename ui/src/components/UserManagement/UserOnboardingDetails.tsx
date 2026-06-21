import React, { useState } from 'react';
import toast from 'react-hot-toast';
import { Member } from '../../api/users';
import { Button } from '../UIPrimitives';

interface UserOnboardingDetailsProps {
  user: Member;
  onContinue: () => void;
}

export const UserOnboardingDetails: React.FC<UserOnboardingDetailsProps> = ({ user, onContinue }) => {
  const [copiedType, setCopiedType] = useState<'linux' | 'windows' | null>(null);
  const [activePlatform, setActivePlatform] = useState<'unix' | 'windows'>('unix');

  const name = `${user.first_name || ''} ${user.last_name || ''}`.trim();
  const installUrl = window.location.origin;
  const installCmdLinux = `curl -fsSL https://hexmos.com/lrc-install.sh | LRC_API_KEY="${user.onboarding_api_key || ''}" LRC_API_URL="${installUrl}" bash`;
  const installCmdWindows = `$env:LRC_API_KEY="${user.onboarding_api_key || ''}"; $env:LRC_API_URL="${installUrl}"; iwr -useb https://hexmos.com/lrc-install.ps1 | iex`;

  const handleCopyCommand = (cmd: string, type: 'linux' | 'windows') => {
    navigator.clipboard.writeText(cmd);
    setCopiedType(type);
    toast.success('Command copied to clipboard!');
    setTimeout(() => setCopiedType(null), 2000);
  };

  const handleDownloadCSV = () => {
    const headers = ['Email', 'Name', 'Linux/Mac Command', 'Windows Command'];
    const row = [user.email, name, installCmdLinux, installCmdWindows];
    
    const escapeCsv = (val: string) => `"${val.replace(/"/g, '""')}"`;
    const csvContent = [
      headers.map(escapeCsv).join(','),
      row.map(escapeCsv).join(',')
    ].join('\n');
    
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.setAttribute('href', url);
    link.setAttribute('download', `${user.email}_git-lrc-setup.csv`);
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    toast.success('git-lrc-setup.csv downloaded successfully!');
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
          <h1 className="text-3xl font-bold text-emerald-400">User Invited Successfully!</h1>
          <p className="text-gray-400 mt-2">
            An invitation email has been sent to <strong>{user.email}</strong>.
          </p>
        </div>

        <div className="space-y-6">
          {/* User Details */}
          <div className="bg-gray-900/50 p-5 rounded-md border border-gray-700">
            <h3 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">User Details</h3>
            <div className="grid grid-cols-2 gap-4 text-sm">
              <div>
                <span className="text-gray-400 block">Name</span>
                <span className="font-medium text-white">{name || 'N/A'}</span>
              </div>
              <div>
                <span className="text-gray-400 block">Role</span>
                <span className="font-medium text-white capitalize">{user.role}</span>
              </div>
            </div>
          </div>

          {/* CLI Installation commands */}
          <div className="bg-gray-900/50 p-5 rounded-md border border-gray-700 space-y-4">
            <div className="flex justify-between items-center border-b border-gray-750 pb-3">
              <h3 className="text-sm font-semibold text-gray-400 uppercase tracking-wider">CLI Installation</h3>
              
              {/* Platform Switcher */}
              <div className="flex bg-gray-950 p-0.5 rounded-lg border border-gray-800">
                <button
                  type="button"
                  onClick={() => setActivePlatform('unix')}
                  className={`px-3 py-1 text-xs font-medium rounded-md transition-all duration-200 ${
                    activePlatform === 'unix'
                      ? 'bg-blue-600 text-white shadow'
                      : 'text-gray-400 hover:text-gray-200'
                  }`}
                >
                  Linux / macOS
                </button>
                <button
                  type="button"
                  onClick={() => setActivePlatform('windows')}
                  className={`px-3 py-1 text-xs font-medium rounded-md transition-all duration-200 ${
                    activePlatform === 'windows'
                      ? 'bg-blue-600 text-white shadow'
                      : 'text-gray-400 hover:text-gray-200'
                  }`}
                >
                  Windows
                </button>
              </div>
            </div>

            <p className="text-xs text-gray-400">
              This command contains the unique onboarding API key for this user. Copy and run it in the terminal to instantly configure the LRC CLI.
            </p>

            {activePlatform === 'unix' ? (
              <div>
                <div className="flex justify-between items-center mb-1.5">
                  <span className="text-xs text-gray-500 font-medium">Shell Command</span>
                  <button
                    onClick={() => handleCopyCommand(installCmdLinux, 'linux')}
                    className="flex items-center text-xs text-blue-400 hover:text-blue-300 font-medium transition"
                  >
                    {copiedType === 'linux' ? (
                      <>
                        <svg className="w-4 h-4 mr-1 text-emerald-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                        </svg>
                        Copied!
                      </>
                    ) : (
                      <>
                        <svg className="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3" />
                        </svg>
                        Copy Command
                      </>
                    )}
                  </button>
                </div>
                <div className="font-mono text-xs bg-gray-950 p-3 rounded border border-gray-800 text-gray-300 overflow-x-auto whitespace-pre-wrap select-all">
                  {installCmdLinux}
                </div>
              </div>
            ) : (
              <div>
                <div className="flex justify-between items-center mb-1.5">
                  <span className="text-xs text-gray-500 font-medium">PowerShell Command</span>
                  <button
                    onClick={() => handleCopyCommand(installCmdWindows, 'windows')}
                    className="flex items-center text-xs text-blue-400 hover:text-blue-300 font-medium transition"
                  >
                    {copiedType === 'windows' ? (
                      <>
                        <svg className="w-4 h-4 mr-1 text-emerald-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                        </svg>
                        Copied!
                      </>
                    ) : (
                      <>
                        <svg className="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3" />
                        </svg>
                        Copy Command
                      </>
                    )}
                  </button>
                </div>
                <div className="font-mono text-xs bg-gray-950 p-3 rounded border border-gray-800 text-gray-300 overflow-x-auto whitespace-pre-wrap select-all">
                  {installCmdWindows}
                </div>
              </div>
            )}
          </div>

          {/* Actions */}
          <div className="flex flex-col sm:flex-row justify-between gap-4 pt-4 border-t border-gray-700">
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
            <Button
              onClick={onContinue}
              className="w-full sm:w-auto"
            >
              Continue to Members
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
};
