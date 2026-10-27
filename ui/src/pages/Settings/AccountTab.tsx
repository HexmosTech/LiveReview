import React, { useState } from 'react';
import { Button, Input } from '../../components/UIPrimitives';
import { notify } from '../../utils/notify';
import { updateProfile, changePassword } from '../../api/profile';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import { fetchUser } from '../../store/Auth/reducer';
import { Avatar } from '../../components/Navbar/Avatar';

const splitName = (name?: string): { first: string; last: string } => {
    const parts = (name || '').trim().split(/\s+/).filter(Boolean);
    if (parts.length === 0) return { first: '', last: '' };
    if (parts.length === 1) return { first: parts[0], last: '' };
    return { first: parts[0], last: parts.slice(1).join(' ') };
};

const AccountTab: React.FC = () => {
    const dispatch = useAppDispatch();
    const user = useAppSelector((state) => state.Auth.user);
    const { first: initialFirst, last: initialLast } = splitName(user?.name);

    const [firstName, setFirstName] = useState(initialFirst);
    const [lastName, setLastName] = useState(initialLast);
    const [isSavingProfile, setIsSavingProfile] = useState(false);

    const [currentPassword, setCurrentPassword] = useState('');
    const [newPassword, setNewPassword] = useState('');
    const [confirmPassword, setConfirmPassword] = useState('');
    const [isSavingPassword, setIsSavingPassword] = useState(false);

    if (!user) return null;

    const handleSaveProfile = async (e: React.FormEvent) => {
        e.preventDefault();
        setIsSavingProfile(true);
        try {
            await updateProfile({ first_name: firstName.trim(), last_name: lastName.trim() });
            await dispatch(fetchUser());
            notify.success('Profile updated.');
        } catch (err) {
            notify.error(err instanceof Error ? err.message : 'Failed to update profile.');
        } finally {
            setIsSavingProfile(false);
        }
    };

    const handleChangePassword = async (e: React.FormEvent) => {
        e.preventDefault();
        if (newPassword.length < 8) {
            notify.error('New password must be at least 8 characters long.');
            return;
        }
        if (newPassword !== confirmPassword) {
            notify.error('New password and confirmation do not match.');
            return;
        }
        setIsSavingPassword(true);
        try {
            await changePassword({ current_password: currentPassword, new_password: newPassword });
            notify.success('Password changed successfully.');
            setCurrentPassword('');
            setNewPassword('');
            setConfirmPassword('');
        } catch (err) {
            notify.error(err instanceof Error ? err.message : 'Failed to change password.');
        } finally {
            setIsSavingPassword(false);
        }
    };

    return (
        <div className="space-y-6">
            <div className="flex items-center gap-4">
                <Avatar identity={user} size={56} />
                <div>
                    <div className="text-base font-semibold text-slate-100">{user.name || user.email}</div>
                    <div className="text-sm text-slate-400">{user.email}</div>
                </div>
            </div>

            <form onSubmit={handleSaveProfile} className="space-y-4 border-t border-slate-700/60 pt-6">
                <h3 className="text-sm font-semibold text-slate-200">Profile and preferences</h3>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    <div>
                        <label className="block text-sm text-slate-400 mb-1">First name</label>
                        <Input value={firstName} onChange={(e) => setFirstName(e.target.value)} placeholder="First name" />
                    </div>
                    <div>
                        <label className="block text-sm text-slate-400 mb-1">Last name</label>
                        <Input value={lastName} onChange={(e) => setLastName(e.target.value)} placeholder="Last name" />
                    </div>
                </div>
                <Button type="submit" variant="primary" disabled={isSavingProfile}>
                    {isSavingProfile ? 'Saving…' : 'Save profile'}
                </Button>
            </form>

            <form onSubmit={handleChangePassword} className="space-y-4 border-t border-slate-700/60 pt-6">
                <h3 className="text-sm font-semibold text-slate-200">Change password</h3>
                <div>
                    <label className="block text-sm text-slate-400 mb-1">Current password</label>
                    <Input
                        type="password"
                        value={currentPassword}
                        onChange={(e) => setCurrentPassword(e.target.value)}
                        autoComplete="current-password"
                    />
                </div>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    <div>
                        <label className="block text-sm text-slate-400 mb-1">New password</label>
                        <Input
                            type="password"
                            value={newPassword}
                            onChange={(e) => setNewPassword(e.target.value)}
                            autoComplete="new-password"
                        />
                    </div>
                    <div>
                        <label className="block text-sm text-slate-400 mb-1">Confirm new password</label>
                        <Input
                            type="password"
                            value={confirmPassword}
                            onChange={(e) => setConfirmPassword(e.target.value)}
                            autoComplete="new-password"
                        />
                    </div>
                </div>
                <Button type="submit" variant="primary" disabled={isSavingPassword}>
                    {isSavingPassword ? 'Changing…' : 'Change password'}
                </Button>
            </form>
        </div>
    );
};

export default AccountTab;
