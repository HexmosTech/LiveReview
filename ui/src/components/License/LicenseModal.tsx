import React, { useState, useEffect } from 'react';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import { submitLicenseToken } from '../../store/License/slice';

interface Props {
  open: boolean;
  onClose: () => void;
  strictMode?: boolean; // if true modal cannot be dismissed until active license
}

const statusRequiresToken = (status: string) => ['missing', 'invalid', 'expired'].includes(status);

export const LicenseModal: React.FC<Props> = ({ open, onClose, strictMode }) => {
  const dispatch = useAppDispatch();
  const { status, updating, lastError } = useAppSelector(s => s.License);
  const [token, setToken] = useState('');
  const [submitted, setSubmitted] = useState(false);

  useEffect(() => {
    if (!open) { setToken(''); setSubmitted(false); }
  }, [open]);

  if (!open) return null;

  const needsBlocking = strictMode && statusRequiresToken(status);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitted(true);
    try {
      const res = await dispatch(submitLicenseToken(token)).unwrap();
      if (res.status === 'active') {
        onClose();
      }
    } catch (err) {
      // lastError handled in slice; we keep UI message reactive
    }
  };

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-slate-900/70">
      <div className="bg-slate-800 rounded-md shadow-xl w-full max-w-lg border border-slate-700">
        <div className="px-5 py-4 border-b border-slate-700 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-white">Enter License Token</h2>
          {!needsBlocking && (
            <button onClick={onClose} className="text-slate-400 hover:text-slate-200" aria-label="Close license modal">✕</button>
          )}
        </div>
        <form onSubmit={handleSubmit} className="p-5 space-y-4">
          <div>
            <label className="block text-sm font-medium text-slate-300 mb-1">Token</label>
            <textarea
              className="w-full h-32 bg-slate-900 border border-slate-700 rounded px-3 py-2 text-slate-200 focus:outline-none focus:ring-2 focus:ring-blue-500"
              value={token}
              onChange={e => setToken(e.target.value.trim())}
              placeholder="Paste license JWT here"
              required
            />
          </div>
          {lastError && submitted && (
            <div className="text-sm text-red-400">{lastError}</div>
          )}
          <div className="flex items-center justify-between">
            <div className="text-xs text-slate-400">
              Status: <span className="font-mono">{status}</span>
            </div>
            <div className="flex gap-2">
              {!needsBlocking && (
                <button type="button" onClick={onClose} className="px-4 py-2 text-sm rounded bg-slate-700 hover:bg-slate-600 text-slate-200 disabled:opacity-50">Cancel</button>
              )}
              <button
                type="submit"
                disabled={updating || !token}
                className="px-4 py-2 text-sm rounded bg-blue-600 hover:bg-blue-500 text-white disabled:opacity-50 flex items-center gap-2"
              >
                {updating && <span className="animate-spin h-4 w-4 border-2 border-white/40 border-t-transparent rounded-full"/>}
                {updating ? 'Saving...' : 'Save Token'}
              </button>
            </div>
          </div>
          {needsBlocking && status !== 'active' && (
            <div className="text-xs text-amber-300 mt-2">
              This application requires a valid license token to proceed. <a href="https://hexmos.com/livereview/access-livereview/" className="underline">Get a Licence here.</a>.
            </div>
          )}
        </form>
      </div>
    </div>
  );
};

export default LicenseModal;
