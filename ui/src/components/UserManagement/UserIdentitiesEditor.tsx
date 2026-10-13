import React, { useEffect, useState } from 'react';
import { Button, Card, Input, Select, Spinner } from '../UIPrimitives';
import { notify } from '../../utils/notify';
import {
  listUserIdentities,
  addUserIdentity,
  removeUserIdentity,
  UserIdentity,
} from '../../api/riskPolicy';

const UserIdentitiesEditor: React.FC<{ orgId: number; userId: number }> = ({ orgId, userId }) => {
  const [identities, setIdentities] = useState<UserIdentity[]>([]);
  const [loading, setLoading] = useState(true);
  const [kind, setKind] = useState<'email' | 'username'>('username');
  const [value, setValue] = useState('');
  const [busy, setBusy] = useState(false);

  const load = () => {
    listUserIdentities(orgId, userId)
      .then((res) => setIdentities(res.rows || []))
      .catch((e) => notify.error(String(e?.message || e)))
      .finally(() => setLoading(false));
  };

  useEffect(load, [orgId, userId]);

  const add = async () => {
    if (!value.trim()) return;
    setBusy(true);
    try {
      await addUserIdentity(orgId, userId, kind, value.trim());
      setValue('');
      await listUserIdentities(orgId, userId).then((res) => setIdentities(res.rows || []));
      notify.success('Identity added.');
    } catch (e: any) {
      notify.error(String(e?.message || e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (id: number) => {
    setBusy(true);
    try {
      await removeUserIdentity(orgId, userId, id);
      setIdentities((prev) => prev.filter((i) => i.id !== id));
      notify.success('Identity removed.');
    } catch (e: any) {
      notify.error(String(e?.message || e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card className="p-4 mt-6">
      <h3 className="text-sm font-semibold text-white mb-1">Alternative identities</h3>
      <p className="text-xs text-slate-400 mb-3">
        Match this user's Git provider handles/emails to their account for auto-approve eligibility.
      </p>

      {loading ? (
        <Spinner />
      ) : (
        <div className="space-y-2 mb-3">
          {identities.length === 0 && <p className="text-xs text-slate-500">No identities yet.</p>}
          {identities.map((i) => (
            <div key={i.id} className="flex items-center justify-between text-sm text-slate-200">
              <span>
                <span className="text-slate-400">{i.kind}:</span> {i.value}
              </span>
              <Button variant="ghost" size="sm" onClick={() => remove(i.id)} disabled={busy} className="text-red-400">
                Remove
              </Button>
            </div>
          ))}
        </div>
      )}

      <div className="flex items-end gap-2">
        <Select
          label="Kind"
          value={kind}
          onChange={(e) => setKind(e.target.value as 'email' | 'username')}
          options={[
            { value: 'username', label: 'Username / handle' },
            { value: 'email', label: 'Email' },
          ]}
          className="w-40"
        />
        <Input label="Value" value={value} onChange={(e) => setValue(e.target.value)} placeholder="e.g. octocat or user@example.com" />
        <Button variant="secondary" onClick={add} disabled={busy || !value.trim()}>
          Add
        </Button>
      </div>
    </Card>
  );
};

export default UserIdentitiesEditor;
