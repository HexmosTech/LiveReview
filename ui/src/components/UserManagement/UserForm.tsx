import React, { useEffect, useRef, useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import * as z from 'zod';
import { useNavigate, useParams, useLocation } from 'react-router-dom';
import { notify } from '../../utils/notify';
import { useOrgContext } from '../../hooks/useOrgContext';
import apiClient from '../../api/apiClient';
import {
    createOrgUser,
    fetchOrgUser,
    updateOrgUser,
    Member,
    checkUserByEmail,
    bulkCheckUsers,
    BulkCheckResultRow,
    submitBulkInvite,
} from '../../api/users';
import { Button, Input, Select, Icons, Spinner } from '../UIPrimitives';
import { useAppDispatch } from '../../store/configureStore';
import { loadUserOrganizations } from '../../store/Organizations/reducer';
import { UpgradePromptModal } from '../Subscriptions';
import { UserOnboardingDetails } from './UserOnboardingDetails';
import { Table, TableHead, TableHeaderCell, TableBody, TableRow, TableCell } from '../DataTable/SimpleTable';
import { BulkOnboardingDetails, BulkOnboardingRow } from './BulkOnboardingDetails';
import { parseUserInviteCsv } from './bulkInviteCsv';

interface BulkRow extends BulkCheckResultRow {
    id: string;
    password: string;
    confirm_password: string;
    submit_status?: 'invited' | 'updated' | 'unchanged' | 'error';
    submit_message?: string;
}

const CloudUploadIcon: React.FC = () => (
    <svg
        className="w-10 h-10"
        fill="currentColor"
        viewBox="0 0 24 24"
        xmlns="http://www.w3.org/2000/svg"
    >
        <path d="M7 18a5 5 0 01-.4-9.98A6 6 0 0118 8.1 4.5 4.5 0 0117.5 18H7zm5.4-8.4a.6.6 0 00-.8 0l-2.5 2.5a.6.6 0 00.85.85l1.45-1.46V16a.6.6 0 001.2 0v-4.51l1.45 1.46a.6.6 0 00.85-.85l-2.5-2.5z" />
    </svg>
);

const baseSchema = z.object({
    email: z.string().email({ message: 'Invalid email address' }),
    firstName: z.string().optional(),
    lastName: z.string().optional(),
    role: z.enum(['member', 'owner', 'super_admin']),
    password: z.string().optional(),
    password_confirmation: z.string().optional(),
});

type UserFormData = z.infer<typeof baseSchema>;

const UserForm: React.FC = () => {
    const navigate = useNavigate();
    const location = useLocation();
    const dispatch = useAppDispatch();
    const { userId } = useParams<{ userId: string }>();
    const { currentOrgId, currentOrg } = useOrgContext();
    const currentUserRole = currentOrg?.role;

    const isEditMode = !!userId;

    const [existsGlobally, setExistsGlobally] = useState(false);
    const [checkingEmail, setCheckingEmail] = useState(false);

    const userSchema = baseSchema
        .refine(
            (data) => {
                if (!isEditMode && !existsGlobally) {
                    return data.firstName && data.firstName.length > 0;
                }
                return true;
            },
            {
                message: 'First name is required for new users',
                path: ['firstName'],
            }
        )
        .refine(
            (data) => {
                if (!isEditMode && !existsGlobally) {
                    return data.lastName && data.lastName.length > 0;
                }
                return true;
            },
            {
                message: 'Last name is required for new users',
                path: ['lastName'],
            }
        )
        .refine(
            (data) => {
                if (isEditMode) {
                    if (data.password) {
                        return data.password.length >= 8;
                    }
                    return true;
                }
                if (!isEditMode && !existsGlobally) {
                    return data.password && data.password.length >= 8;
                }
                return true;
            },
            {
                message: 'Password must be at least 8 characters',
                path: ['password'],
            }
        )
        .refine(
            (data) => {
                if (isEditMode) {
                    if (data.password || data.password_confirmation) {
                        return data.password === data.password_confirmation;
                    }
                    return true;
                }
                if (!isEditMode && !existsGlobally) {
                    return data.password === data.password_confirmation;
                }
                return true;
            },
            {
                message: 'Passwords do not match',
                path: ['password_confirmation'],
            }
        );

    const [user, setUser] = useState<Member | null>(null);
    const [createdUser, setCreatedUser] = useState<Member | null>(null);
    const [loading, setLoading] = useState(false);
    const [showUpgradeModal, setShowUpgradeModal] = useState(false);
    const [showPassword, setShowPassword] = useState(false);
    const [prerequisiteError, setPrerequisiteError] = useState<string | null>(null);
    const [prerequisiteStatus, setPrerequisiteStatus] = useState<{ productionUrl: boolean; smtp: boolean } | null>(null);

    const [activeTab, setActiveTab] = useState<'single' | 'bulk'>(
        location.pathname.endsWith('/bulk') ? 'bulk' : 'single'
    );

    const selectTab = (tab: 'single' | 'bulk') => {
        setActiveTab(tab);
        navigate(tab === 'bulk' ? '/settings/users/add/bulk' : '/settings/users/add', { replace: true });
    };
    const [bulkFile, setBulkFile] = useState<File | null>(null);
    const [isDragging, setIsDragging] = useState(false);
    const [bulkVerifying, setBulkVerifying] = useState(false);
    const [bulkSubmitting, setBulkSubmitting] = useState(false);
    const [bulkSubmitted, setBulkSubmitted] = useState(false);
    const [bulkRows, setBulkRows] = useState<BulkRow[] | null>(null);
    const [bulkCompletedRows, setBulkCompletedRows] = useState<BulkOnboardingRow[] | null>(null);
    const bulkFileInputRef = useRef<HTMLInputElement>(null);

    const handleDownloadSample = () => {
        const csvContent = [
            'email,first_name,last_name,role,password,confirm_password',
            'jane.doe@example.com,Jane,Doe,owner,Password123,Password123',
            'john.smith@example.com,John,Smith,member,Password123,Password123',
        ].join('\n');
        const blob = new Blob([csvContent], {
            type: 'text/csv;charset=utf-8;',
        });
        const url = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = 'user_invite_sample.csv';
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        URL.revokeObjectURL(url);
    };

    const handleUploadUsersClick = () => bulkFileInputRef.current?.click();

    const processBulkFile = async (file: File) => {
        if (!currentOrgId) {
            notify.error('No organization selected.');
            return;
        }

        const text = await file.text();
        const parsedRows = parseUserInviteCsv(text);
        if (parsedRows.length === 0) {
            notify.error('No valid rows found in that CSV.');
            return;
        }

        setBulkFile(file);
        setBulkRows(null);
        setBulkVerifying(true);
        try {
            const results = await bulkCheckUsers(
                currentOrgId.toString(),
                parsedRows.map((row) => ({
                    email: row.email,
                    first_name: row.first_name,
                    last_name: row.last_name,
                    role: row.role || 'member',
                }))
            );

            const allowedRoles = getRoleOptions().map((opt) => opt.value);

            // bulkCheckUsers returns exactly one result per submitted row, in the same
            // order — zip by index (not by email) so duplicate emails in the CSV don't
            // collapse onto a single password.
            setBulkRows(
                results.map((row, index) => {
                    const csvRow = parsedRows[index];
                    const normalizedRole = row.role.toLowerCase().trim();
                    return {
                        ...row,
                        role: allowedRoles.includes(normalizedRole)
                            ? normalizedRole
                            : 'member',
                        id: `${index}-${row.email}`,
                        password: csvRow?.password || '',
                        confirm_password: csvRow?.confirm_password || '',
                    };
                })
            );
        } catch (error) {
            console.error('Bulk user check failed', error);
            notify.error(
                'Failed to verify users against your organization. Please try again.'
            );
            setBulkFile(null);
        } finally {
            setBulkVerifying(false);
        }
    };

    const handleBulkFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        e.target.value = '';
        if (!file) return;
        processBulkFile(file);
    };

    const handleBulkDragOver = (e: React.DragEvent<HTMLDivElement>) => {
        e.preventDefault();
        setIsDragging(true);
    };

    const handleBulkDragLeave = (e: React.DragEvent<HTMLDivElement>) => {
        e.preventDefault();
        setIsDragging(false);
    };

    const handleBulkDrop = (e: React.DragEvent<HTMLDivElement>) => {
        e.preventDefault();
        setIsDragging(false);
        const file = e.dataTransfer.files?.[0];
        if (!file) return;
        processBulkFile(file);
    };

    const handleBulkReset = () => {
        setBulkFile(null);
        setBulkRows(null);
        setBulkSubmitted(false);
    };

    const updateBulkRow = (id: string, patch: Partial<BulkRow>) => {
        setBulkRows((rows) =>
            rows
                ? rows.map((row) =>
                      row.id === id ? { ...row, ...patch } : row
                  )
                : rows
        );
    };

    const removeBulkRow = (id: string) => {
        setBulkRows((rows) =>
            rows ? rows.filter((row) => row.id !== id) : rows
        );
    };

    const handleBulkSubmit = async () => {
        if (!currentOrgId || !bulkRows || bulkRows.length === 0) return;

        setBulkSubmitting(true);
        try {
            const results = await submitBulkInvite(
                currentOrgId.toString(),
                bulkRows.map((row) => ({
                    email: row.email,
                    first_name: row.first_name,
                    last_name: row.last_name,
                    role: row.role,
                    password: row.password,
                    confirm_password: row.confirm_password,
                }))
            );

            // submitBulkInvite returns exactly one result per submitted row, in the same
            // order — zip by index (not by email) so duplicate emails in the table each
            // get their own outcome instead of collapsing onto one.
            setBulkRows((rows) =>
                rows
                    ? rows.map((row, index) => {
                          const result = results[index];
                          return result
                              ? {
                                    ...row,
                                    submit_status: result.status,
                                    submit_message: result.message,
                                }
                              : row;
                      })
                    : rows
            );

            const counts = results.reduce(
                (acc, r) => {
                    acc[r.status] += 1;
                    return acc;
                },
                { invited: 0, updated: 0, unchanged: 0, error: 0 }
            );
            const parts: string[] = [];
            if (counts.invited > 0)
                parts.push(`${counts.invited} user(s) added`);
            if (counts.updated > 0) parts.push(`${counts.updated} updated`);
            if (counts.unchanged > 0)
                parts.push(`${counts.unchanged} unchanged`);
            const summary =
                parts.length > 0
                    ? `${parts.join(', ')} successfully`
                    : 'No changes made';

            if (counts.error > 0) {
                notify.error(
                    `${summary}, ${counts.error} failed — see table for details.`
                );
                setBulkSubmitted(true);
            } else if (counts.invited > 0 || counts.updated > 0) {
                const completed: BulkOnboardingRow[] = results.reduce<BulkOnboardingRow[]>(
                    (acc, result, index) => {
                        if (result.status !== 'invited' && result.status !== 'updated') return acc;
                        const row = bulkRows[index];
                        acc.push({
                            email: row.email,
                            name: `${row.first_name || ''} ${row.last_name || ''}`.trim(),
                            role: row.role,
                            status: result.status,
                            onboardingApiKey: result.onboarding_api_key,
                        });
                        return acc;
                    },
                    []
                );
                setBulkCompletedRows(completed);
            } else {
                notify.success(summary);
                setBulkSubmitted(true);
            }
            dispatch(loadUserOrganizations());
        } catch (error) {
            console.error('Bulk invite submit failed', error);
            const rawMessage = (error as Error).message || 'Failed to submit bulk invite. Please try again.';
            
            // Check if this is a prerequisite error
            if (rawMessage.includes('invitations require')) {
                setPrerequisiteError(rawMessage);
            } else {
                notify.error('Failed to submit bulk invite. Please try again.');
            }
        } finally {
            setBulkSubmitting(false);
        }
    };

    const {
        register,
        handleSubmit,
        formState: { errors, isSubmitting },
        reset,
        watch,
        setValue,
        trigger,
    } = useForm<UserFormData>({
        resolver: zodResolver(userSchema),
        defaultValues: {
            role: 'member',
        },
    });

    const emailValue = watch('email');

    const handleEmailCheck = async () => {
        if (
            !currentOrgId ||
            isEditMode ||
            !emailValue ||
            !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(emailValue)
        ) {
            return;
        }

        setCheckingEmail(true);
        try {
            const result = await checkUserByEmail(
                currentOrgId.toString(),
                emailValue
            );
            setExistsGlobally(result.exists);
            if (result.exists) {
                setValue('firstName', result.first_name || '');
                setValue('lastName', result.last_name || '');
                // Clear password errors if any
                trigger();
            }
        } catch (error) {
            console.error('Failed to check email', error);
        } finally {
            setCheckingEmail(false);
        }
    };

    useEffect(() => {
        if (userId && currentOrgId) {
            setLoading(true);
            fetchOrgUser(currentOrgId.toString(), userId)
                .then((userData) => {
                    setUser(userData);
                    reset({
                        email: userData.email,
                        firstName: userData.first_name || '',
                        lastName: userData.last_name || '',
                        role:
                            (userData.role as
                                | 'member'
                                | 'owner'
                                | 'super_admin') || 'member',
                    });
                })
                .catch((err) => {
                    notify.error(`Failed to load user: ${err.message}`);
                    navigate('/settings#users');
                })
                .finally(() => setLoading(false));
        }
    }, [userId, currentOrgId, reset, navigate]);

    // Check prerequisites on mount (only for invite mode, not edit)
    useEffect(() => {
        if (!isEditMode && !userId) {
            checkPrerequisites();
        }
    }, [isEditMode, userId]);

    const checkPrerequisites = async () => {
        try {
            const missing: string[] = [];
            let productionUrlOk = false;
            let smtpOk = false;
            
            // Check production URL
            try {
                const prodUrlResponse = await apiClient.get<{ url: string }>('/api/v1/production-url');
                if (prodUrlResponse?.url) {
                    productionUrlOk = true;
                } else {
                    missing.push('Production URL (Settings → Instance)');
                }
            } catch (error) {
                missing.push('Production URL (Settings → Instance)');
            }
            
            // Check SMTP settings
            try {
                const smtpResponse = await apiClient.get<{ host: string }>('/api/v1/admin/settings/smtp');
                if (smtpResponse?.host) {
                    smtpOk = true;
                } else {
                    missing.push('SMTP settings (Settings → SMTP)');
                }
            } catch (error) {
                missing.push('SMTP settings (Settings → SMTP)');
            }
            
            setPrerequisiteStatus({ productionUrl: productionUrlOk, smtp: smtpOk });
            
            if (missing.length > 0) {
                setPrerequisiteError(`Invitations require the following to be configured: ${missing.join(', ')}`);
            } else {
                setPrerequisiteError(null);
            }
        } catch (error) {
            console.error('Failed to check prerequisites', error);
        }
    };

    const getRoleOptions = () => {
        if (currentUserRole === 'super_admin') {
            return [
                { value: 'member', label: 'Member' },
                { value: 'owner', label: 'Owner' },
                // { value: 'super_admin', label: 'Super Admin' },
            ];
        }
        if (currentUserRole === 'owner') {
            return [
                { value: 'member', label: 'Member' },
                { value: 'owner', label: 'Owner' },
            ];
        }
        return [{ value: 'member', label: 'Member' }];
    };

    const roleNameToId = (
        roleName: 'member' | 'owner' | 'super_admin'
    ): number => {
        switch (roleName) {
            case 'super_admin':
                return 1;
            case 'owner':
                return 2;
            case 'member':
                return 3;
            default:
                return 3; // Default to member
        }
    };

    const onSubmit = async (data: UserFormData) => {
        if (!currentOrgId) {
            notify.error('No organization selected.');
            return;
        }

        try {
            if (isEditMode && user) {
                const payload: any = {
                    first_name: data.firstName,
                    last_name: data.lastName,
                    role_id: roleNameToId(data.role),
                };
                if (data.password) {
                    payload.password = data.password;
                }
                const updatedUser = await updateOrgUser(
                    currentOrgId.toString(),
                    user.id.toString(),
                    payload
                );
                notify.success(
                    `User ${updatedUser.email} updated successfully!`
                );
                dispatch(loadUserOrganizations());
            } else {
                if (!existsGlobally && !data.password) {
                    notify.error('Password is required for new users.');
                    return;
                }
                const newUser = await createOrgUser(currentOrgId.toString(), {
                    email: data.email,
                    first_name: data.firstName || '',
                    last_name: data.lastName || '',
                    role_id: roleNameToId(data.role),
                    password: data.password,
                });
                notify.success(`User ${newUser.email} invited successfully!`);
                setCreatedUser(newUser);
                return;
            }
            navigate('/settings#users');
        } catch (error) {
            const action = isEditMode ? 'update' : 'invite';
            const rawMessage =
                (error as Error).message || 'An unknown error occurred.';
            const errorMessage =
                rawMessage
                    .replace(/[\r\n]+/g, ' ')
                    .trim()
                    .slice(0, 200) || 'An unknown error occurred.';
            
            // Check if this is a prerequisite error
            if (errorMessage.includes('invitations require')) {
                setPrerequisiteError(errorMessage);
            } else {
                notify.error(['Failed to', action, 'user:', errorMessage].join(' '));
            }
            console.error('User operation error', { action, error });
        }
    };

    if (loading) {
        return (
            <div className="p-6 bg-gray-900 text-white text-center">
                <h1 className="text-3xl font-bold">Loading User...</h1>
            </div>
        );
    }

    if (createdUser) {
        return (
            <UserOnboardingDetails
                user={createdUser}
                onContinue={() => {
                    if (currentOrg?.plan_type === 'free') {
                        setShowUpgradeModal(true);
                    } else {
                        navigate('/settings#users');
                    }
                }}
            />
        );
    }

    if (bulkCompletedRows) {
        return (
            <BulkOnboardingDetails
                rows={bulkCompletedRows}
                onContinue={() => navigate('/settings#users')}
            />
        );
    }

    // Once the bulk review table is showing, drop the steps panel so the
    // table can use the full width instead of being squeezed into a narrow column.
    const showSteps = !isEditMode && !(activeTab === 'bulk' && bulkRows !== null);

    return (
        <div className="bg-gray-900 text-white">
            <div className="container mx-auto px-4 py-4">
                <h1 className="text-2xl font-bold mb-4">
                    {isEditMode ? 'Edit User' : 'Invite User'}
                </h1>

                {prerequisiteError && (
                    <div className="mb-4 p-4 bg-amber-900/30 border border-amber-600 rounded-lg">
                        <div className="flex items-start">
                            <svg className="w-5 h-5 text-amber-500 mt-0.5 mr-3 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                                <path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
                            </svg>
                            <div className="flex-1">
                                <h3 className="text-amber-500 font-medium">Configuration Required</h3>
                                <p className="text-amber-200/80 mt-1 text-sm">{prerequisiteError}</p>
                                <div className="mt-3 space-y-2">
                                    <div className="flex items-center">
                                        {prerequisiteStatus?.productionUrl ? (
                                            <svg className="w-4 h-4 text-green-500 mr-2" fill="currentColor" viewBox="0 0 20 20">
                                                <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                                            </svg>
                                        ) : (
                                            <svg className="w-4 h-4 text-amber-500 mr-2" fill="currentColor" viewBox="0 0 20 20">
                                                <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                                            </svg>
                                        )}
                                        <a href="/settings#instance" className="text-sm text-amber-400 hover:text-amber-300 underline">
                                            Settings → Instance
                                        </a>
                                    </div>
                                    <div className="flex items-center">
                                        {prerequisiteStatus?.smtp ? (
                                            <svg className="w-4 h-4 text-green-500 mr-2" fill="currentColor" viewBox="0 0 20 20">
                                                <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                                            </svg>
                                        ) : (
                                            <svg className="w-4 h-4 text-amber-500 mr-2" fill="currentColor" viewBox="0 0 20 20">
                                                <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                                            </svg>
                                        )}
                                        <a href="/settings#smtp" className="text-sm text-amber-400 hover:text-amber-300 underline">
                                            Settings → SMTP
                                        </a>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                )}

                <div className="bg-slate-800 rounded-lg border border-slate-700 shadow-xl overflow-hidden">
                    {isEditMode ? (
                        <p className="px-6 pt-6 pb-4 text-sm text-gray-400">{`Update details for ${user?.email}`}</p>
                    ) : (
                        <div className="flex gap-6 px-6 pt-6 border-b border-slate-700">
                            <button
                                type="button"
                                onClick={() => selectTab('single')}
                                className={`pb-3 text-sm font-medium border-b-2 -mb-px transition-colors ${
                                    activeTab === 'single'
                                        ? 'border-blue-500 text-white'
                                        : 'border-transparent text-slate-400 hover:text-slate-200'
                                }`}
                            >
                                Invite User
                            </button>
                            <button
                                type="button"
                                onClick={() => selectTab('bulk')}
                                className={`pb-3 text-sm font-medium border-b-2 -mb-px transition-colors ${
                                    activeTab === 'bulk'
                                        ? 'border-blue-500 text-white'
                                        : 'border-transparent text-slate-400 hover:text-slate-200'
                                }`}
                            >
                                Invite Multiple Users
                            </button>
                        </div>
                    )}

                    <div
                        className={
                            showSteps
                                ? 'grid grid-cols-1 lg:grid-cols-[520px_1fr]'
                                : ''
                        }
                    >
                        {showSteps && (
                            <div className="p-5 bg-slate-900/40 border-b lg:border-b-0 lg:border-r border-slate-700/70">
                                <h2 className="text-lg font-semibold text-white mb-2">
                                    How it works
                                </h2>
                                <ol>
                                    {(() => {
                                        const steps =
                                            activeTab === 'single'
                                                ? [
                                                      <>
                                                          Fill in the
                                                          user&apos;s email,
                                                          name, and role, then
                                                          click{' '}
                                                          <strong className="text-white font-medium">
                                                              Invite User
                                                          </strong>
                                                          .
                                                      </>,
                                                      <>
                                                          Head to{' '}
                                                          <button
                                                              type="button"
                                                              onClick={() =>
                                                                  navigate(
                                                                      '/settings#users'
                                                                  )
                                                              }
                                                              className="text-blue-400 hover:text-blue-300 underline underline-offset-2"
                                                          >
                                                              User Management
                                                          </button>{' '}
                                                          and find the person
                                                          you just invited.
                                                      </>,
                                                      <>
                                                          Select them and click{' '}
                                                          <strong className="text-white font-medium">
                                                              Download
                                                              Onboarding Command
                                                          </strong>{' '}
                                                          — or they can use the
                                                          command from their
                                                          invitation email
                                                          instead.
                                                      </>,
                                                      <>
                                                          Share the command (or
                                                          let them run it
                                                          themselves) to finish
                                                          setting up{' '}
                                                          <code className="px-1 py-0.5 rounded bg-slate-700/80 text-slate-200 text-xs">
                                                              lrc
                                                          </code>
                                                          .
                                                      </>,
                                                  ]
                                                : [
                                                      <>
                                                          Download the sample
                                                          CSV format below.
                                                      </>,
                                                      <>
                                                          Fill it in with your
                                                          users&apos; details
                                                          and save the file.
                                                      </>,
                                                      <>
                                                          Upload it here and
                                                          review the changes in
                                                          the table.
                                                      </>,
                                                      <>
                                                          Click{' '}
                                                          <strong className="text-white font-medium">
                                                              Invite Users
                                                          </strong>{' '}
                                                          to create or update
                                                          the users.
                                                      </>,
                                                      <>
                                                          Head to{' '}
                                                          <button
                                                              type="button"
                                                              onClick={() =>
                                                                  navigate(
                                                                      '/settings#users'
                                                                  )
                                                              }
                                                              className="text-blue-400 hover:text-blue-300 underline underline-offset-2"
                                                          >
                                                              User Management
                                                          </button>
                                                          , select the users,
                                                          and download their
                                                          onboarding commands —
                                                          or they&apos;ll get
                                                          them via email.
                                                      </>,
                                                  ];
                                        return steps.map((step, i) => (
                                            <li
                                                key={i}
                                                className="flex gap-4 mt-2"
                                            >
                                                <div className="flex flex-col items-center">
                                                    <span className="flex-shrink-0 flex items-center justify-center w-6 h-6 rounded-full bg-blue-600 text-xs font-semibold text-white">
                                                        {i + 1}
                                                    </span>
                                                    {i < steps.length - 1 && (
                                                        <div className="w-px flex-1 my-1 bg-slate-700/70" />
                                                    )}
                                                </div>
                                                <p className="text-sm text-slate-300 leading-relaxed pb-8">
                                                    {step}
                                                </p>
                                            </li>
                                        ));
                                    })()}
                                </ol>
                            </div>
                        )}

                        <div className="p-5 min-h-[480px]">
                            {!isEditMode && activeTab === 'bulk' ? (
                                bulkVerifying ? (
                                    <div className="flex flex-col items-center justify-center py-16">
                                        <Spinner size="lg" />
                                        <p className="mt-4 text-sm text-slate-300">
                                            Verifying users against your
                                            organization…
                                        </p>
                                    </div>
                                ) : bulkRows ? (
                                    <div className="space-y-4">
                                        <div className="flex items-center justify-between">
                                            <p className="text-sm text-slate-300">
                                                {bulkRows.length} user
                                                {bulkRows.length !== 1
                                                    ? 's'
                                                    : ''}{' '}
                                                ready to review
                                            </p>
                                            <button
                                                type="button"
                                                onClick={handleBulkReset}
                                                disabled={bulkSubmitting}
                                                className="text-sm text-blue-400 hover:text-blue-300 disabled:opacity-50 disabled:cursor-not-allowed"
                                            >
                                                Upload a different file
                                            </button>
                                        </div>

                                        {bulkRows.length > 0 ? (
                                            <div className="overflow-x-auto border border-slate-700 rounded-lg">
                                                <Table style={{ minWidth: 720 }}>
                                                    <TableHead>
                                                        <TableHeaderCell>Email</TableHeaderCell>
                                                        <TableHeaderCell>First Name</TableHeaderCell>
                                                        <TableHeaderCell>Last Name</TableHeaderCell>
                                                        <TableHeaderCell>Role</TableHeaderCell>
                                                        <TableHeaderCell>Status</TableHeaderCell>
                                                        <TableHeaderCell className="w-10" />
                                                    </TableHead>
                                                    <TableBody>
                                                        {bulkRows.map((row) => {
                                                            const hasGlobalAccount =
                                                                row.exists_globally &&
                                                                !row.exists;
                                                            return (
                                                                <TableRow key={row.id}>
                                                                    <TableCell className="text-slate-200 whitespace-nowrap">
                                                                        {
                                                                            row.email
                                                                        }
                                                                        {row.old_email && (
                                                                            <p className="text-xs text-amber-400 mt-1">
                                                                                was:{' '}
                                                                                {
                                                                                    row.old_email
                                                                                }
                                                                            </p>
                                                                        )}
                                                                    </TableCell>
                                                                    <TableCell className="min-w-[140px]">
                                                                        <input
                                                                            value={
                                                                                row.first_name
                                                                            }
                                                                            onChange={(
                                                                                e
                                                                            ) =>
                                                                                updateBulkRow(
                                                                                    row.id,
                                                                                    {
                                                                                        first_name:
                                                                                            e
                                                                                                .target
                                                                                                .value,
                                                                                    }
                                                                                )
                                                                            }
                                                                            disabled={
                                                                                bulkSubmitting ||
                                                                                hasGlobalAccount
                                                                            }
                                                                            className="w-full bg-slate-900 border border-slate-600 rounded px-2.5 py-1.5 text-white text-sm outline-none focus:border-blue-500 disabled:opacity-50"
                                                                        />
                                                                        {row.old_first_name && (
                                                                            <p className="text-xs text-amber-400 mt-1">
                                                                                was:{' '}
                                                                                {
                                                                                    row.old_first_name
                                                                                }
                                                                            </p>
                                                                        )}
                                                                    </TableCell>
                                                                    <TableCell className="min-w-[140px]">
                                                                        <input
                                                                            value={
                                                                                row.last_name
                                                                            }
                                                                            onChange={(
                                                                                e
                                                                            ) =>
                                                                                updateBulkRow(
                                                                                    row.id,
                                                                                    {
                                                                                        last_name:
                                                                                            e
                                                                                                .target
                                                                                                .value,
                                                                                    }
                                                                                )
                                                                            }
                                                                            disabled={
                                                                                bulkSubmitting ||
                                                                                hasGlobalAccount
                                                                            }
                                                                            className="w-full bg-slate-900 border border-slate-600 rounded px-2.5 py-1.5 text-white text-sm outline-none focus:border-blue-500 disabled:opacity-50"
                                                                        />
                                                                        {row.old_last_name && (
                                                                            <p className="text-xs text-amber-400 mt-1">
                                                                                was:{' '}
                                                                                {
                                                                                    row.old_last_name
                                                                                }
                                                                            </p>
                                                                        )}
                                                                    </TableCell>
                                                                    <TableCell className="min-w-[140px]">
                                                                        <div className="relative">
                                                                            <select
                                                                                value={
                                                                                    row.role
                                                                                }
                                                                                onChange={(
                                                                                    e
                                                                                ) =>
                                                                                    updateBulkRow(
                                                                                        row.id,
                                                                                        {
                                                                                            role: e
                                                                                                .target
                                                                                                .value,
                                                                                        }
                                                                                    )
                                                                                }
                                                                                disabled={
                                                                                    bulkSubmitting
                                                                                }
                                                                                className="w-full appearance-none bg-slate-900 border border-slate-600 rounded pl-2.5 pr-7 py-1.5 text-white text-sm outline-none focus:border-blue-500 disabled:opacity-50"
                                                                            >
                                                                                {getRoleOptions().map(
                                                                                    (
                                                                                        opt
                                                                                    ) => (
                                                                                        <option
                                                                                            key={
                                                                                                opt.value
                                                                                            }
                                                                                            value={
                                                                                                opt.value
                                                                                            }
                                                                                        >
                                                                                            {
                                                                                                opt.label
                                                                                            }
                                                                                        </option>
                                                                                    )
                                                                                )}
                                                                            </select>
                                                                            <svg
                                                                                className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 h-4 w-4 text-slate-400"
                                                                                fill="none"
                                                                                stroke="currentColor"
                                                                                viewBox="0 0 24 24"
                                                                            >
                                                                                <path
                                                                                    strokeLinecap="round"
                                                                                    strokeLinejoin="round"
                                                                                    strokeWidth={
                                                                                        2
                                                                                    }
                                                                                    d="m6 9 6 6 6-6"
                                                                                />
                                                                            </svg>
                                                                        </div>
                                                                        {row.old_role && (
                                                                            <p className="text-xs text-amber-400 mt-1">
                                                                                was:{' '}
                                                                                {
                                                                                    row.old_role
                                                                                }
                                                                            </p>
                                                                        )}
                                                                    </TableCell>
                                                                    <TableCell className="whitespace-nowrap">
                                                                        {row.exists ? (
                                                                            <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-amber-900/20 text-amber-300 whitespace-nowrap">
                                                                                Existing
                                                                                member
                                                                            </span>
                                                                        ) : (
                                                                            <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-900/20 text-green-300 whitespace-nowrap">
                                                                                New
                                                                                invite
                                                                            </span>
                                                                        )}
                                                                        {hasGlobalAccount && (
                                                                            <p className="text-xs text-blue-400 mt-1">
                                                                                Has
                                                                                a
                                                                                LiveReview
                                                                                account
                                                                                —
                                                                                name
                                                                                can&apos;t
                                                                                be
                                                                                edited
                                                                                here
                                                                            </p>
                                                                        )}
                                                                        {row.submit_status && (
                                                                            <div className="mt-1.5">
                                                                                {row.submit_status ===
                                                                                'error' ? (
                                                                                    <>
                                                                                        <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-red-900/20 text-red-300 whitespace-nowrap">
                                                                                            Failed
                                                                                        </span>
                                                                                        <p className="text-xs text-red-400 mt-1">
                                                                                            {
                                                                                                row.submit_message
                                                                                            }
                                                                                        </p>
                                                                                    </>
                                                                                ) : row.submit_status ===
                                                                                  'unchanged' ? (
                                                                                    <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-slate-700/40 text-slate-300 whitespace-nowrap">
                                                                                        No
                                                                                        changes
                                                                                    </span>
                                                                                ) : (
                                                                                    <span className="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-green-900/20 text-green-300 whitespace-nowrap">
                                                                                        {row.submit_status ===
                                                                                        'invited'
                                                                                            ? 'Invited'
                                                                                            : 'Updated'}
                                                                                    </span>
                                                                                )}
                                                                            </div>
                                                                        )}
                                                                    </TableCell>
                                                                    <TableCell>
                                                                        <button
                                                                            type="button"
                                                                            onClick={() =>
                                                                                removeBulkRow(
                                                                                    row.id
                                                                                )
                                                                            }
                                                                            disabled={
                                                                                bulkSubmitting
                                                                            }
                                                                            className="text-slate-400 hover:text-red-400 disabled:opacity-50 disabled:cursor-not-allowed"
                                                                            aria-label="Remove row"
                                                                        >
                                                                            <Icons.Delete />
                                                                        </button>
                                                                    </TableCell>
                                                                </TableRow>
                                                            );
                                                        })}
                                                    </TableBody>
                                                </Table>
                                            </div>
                                        ) : (
                                            <p className="text-sm text-slate-400 text-center py-6 border border-dashed border-slate-700 rounded-lg">
                                                All rows removed. Upload a
                                                different file to continue.
                                            </p>
                                        )}

                                        <div className="flex justify-end space-x-4 pt-4">
                                            <Button
                                                variant={
                                                    bulkSubmitted
                                                        ? 'primary'
                                                        : 'ghost'
                                                }
                                                type="button"
                                                onClick={() =>
                                                    navigate('/settings#users')
                                                }
                                                disabled={bulkSubmitting}
                                            >
                                                {bulkSubmitted
                                                    ? 'Close'
                                                    : 'Cancel'}
                                            </Button>
                                            <Button
                                                variant={
                                                    bulkSubmitted
                                                        ? 'ghost'
                                                        : 'primary'
                                                }
                                                type="button"
                                                onClick={handleBulkSubmit}
                                                isLoading={bulkSubmitting}
                                                disabled={
                                                    bulkRows.length === 0 ||
                                                    bulkSubmitting
                                                }
                                            >
                                                {bulkSubmitting
                                                    ? 'Inviting Users...'
                                                    : 'Invite Users'}
                                            </Button>
                                        </div>
                                    </div>
                                ) : (
                                    <div className="space-y-5">
                                        <p className="text-sm text-slate-400">
                                            Download the sample format, fill it
                                            in with your users, then upload it
                                            here.
                                        </p>

                                        <button
                                            type="button"
                                            onClick={handleDownloadSample}
                                            className="w-full flex items-center gap-2 px-4 py-3 bg-slate-900/40 hover:bg-slate-700/60 border border-slate-700 rounded-lg text-sm text-slate-200 transition-colors"
                                        >
                                            <Icons.Download />
                                            Download sample format
                                        </button>

                                        <div
                                            onClick={handleUploadUsersClick}
                                            onDragOver={handleBulkDragOver}
                                            onDragLeave={handleBulkDragLeave}
                                            onDrop={handleBulkDrop}
                                            className={`border-2 border-dashed rounded-lg py-20 px-6 flex flex-col items-center justify-center text-center cursor-pointer transition-colors ${
                                                isDragging
                                                    ? 'border-blue-500 bg-slate-700/40'
                                                    : 'border-slate-600 hover:border-slate-500 bg-slate-900/30'
                                            }`}
                                        >
                                            <span className="text-slate-200">
                                                <CloudUploadIcon />
                                            </span>
                                            <p className="mt-3 text-base text-slate-200">
                                                Drag &amp; drop or browse
                                            </p>
                                            <input
                                                ref={bulkFileInputRef}
                                                type="file"
                                                accept=".csv"
                                                className="hidden"
                                                onChange={handleBulkFileChange}
                                            />
                                        </div>

                                        <div className="flex justify-end space-x-4 pt-6">
                                            <Button
                                                variant="ghost"
                                                type="button"
                                                onClick={() =>
                                                    navigate('/settings#users')
                                                }
                                            >
                                                Cancel
                                            </Button>
                                            <Button type="button" disabled>
                                                Invite Users
                                            </Button>
                                        </div>
                                    </div>
                                )
                            ) : (
                                <form
                                    onSubmit={handleSubmit(onSubmit)}
                                    className="space-y-6"
                                    autoComplete="off"
                                >
                                    <Input
                                        label="Email Address"
                                        id="email"
                                        type="email"
                                        {...register('email')}
                                        onBlur={handleEmailCheck}
                                        error={errors.email?.message}
                                        required
                                        disabled={isEditMode}
                                        icon={
                                            checkingEmail ? (
                                                <svg
                                                    className="animate-spin h-5 w-5 text-blue-500"
                                                    xmlns="http://www.w3.org/2000/svg"
                                                    fill="none"
                                                    viewBox="0 0 24 24"
                                                >
                                                    <circle
                                                        className="opacity-25"
                                                        cx="12"
                                                        cy="12"
                                                        r="10"
                                                        stroke="currentColor"
                                                        strokeWidth="4"
                                                    ></circle>
                                                    <path
                                                        className="opacity-75"
                                                        fill="currentColor"
                                                        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                                                    ></path>
                                                </svg>
                                            ) : undefined
                                        }
                                        iconPosition="right"
                                    />

                                    {existsGlobally && !isEditMode && (
                                        <div className="bg-blue-900/30 border border-blue-500/50 p-4 rounded-md text-blue-200 text-sm">
                                            This user already has a LiveReview
                                            account. Please select a role.
                                        </div>
                                    )}

                                    {!existsGlobally && (
                                        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                                            <Input
                                                label="First Name"
                                                id="firstName"
                                                {...register('firstName')}
                                                error={
                                                    errors.firstName?.message
                                                }
                                                required
                                            />
                                            <Input
                                                label="Last Name"
                                                id="lastName"
                                                {...register('lastName')}
                                                error={errors.lastName?.message}
                                                required
                                            />
                                        </div>
                                    )}

                                    <Select
                                        label="Role"
                                        id="role"
                                        {...register('role')}
                                        options={getRoleOptions()}
                                        error={errors.role?.message}
                                        required
                                    />

                                    {(!isEditMode && !existsGlobally) ||
                                    (isEditMode &&
                                        currentUserRole === 'super_admin') ? (
                                        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                                            <Input
                                                label={
                                                    isEditMode
                                                        ? 'New Password'
                                                        : 'Password'
                                                }
                                                id="password"
                                                type={
                                                    showPassword
                                                        ? 'text'
                                                        : 'password'
                                                }
                                                autoComplete="new-password"
                                                {...register('password')}
                                                error={errors.password?.message}
                                                required={!isEditMode}
                                                iconPosition="right"
                                                icon={
                                                    <button
                                                        type="button"
                                                        onClick={() =>
                                                            setShowPassword(
                                                                !showPassword
                                                            )
                                                        }
                                                        className="pointer-events-auto text-gray-400 hover:text-white focus:outline-none"
                                                    >
                                                        {showPassword ? (
                                                            <svg
                                                                className="w-5 h-5"
                                                                fill="none"
                                                                viewBox="0 0 24 24"
                                                                stroke="currentColor"
                                                            >
                                                                <path
                                                                    strokeLinecap="round"
                                                                    strokeLinejoin="round"
                                                                    strokeWidth={
                                                                        2
                                                                    }
                                                                    d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 5.411m0 0L21 21"
                                                                />
                                                            </svg>
                                                        ) : (
                                                            <svg
                                                                className="w-5 h-5"
                                                                fill="none"
                                                                viewBox="0 0 24 24"
                                                                stroke="currentColor"
                                                            >
                                                                <path
                                                                    strokeLinecap="round"
                                                                    strokeLinejoin="round"
                                                                    strokeWidth={
                                                                        2
                                                                    }
                                                                    d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
                                                                />
                                                                <path
                                                                    strokeLinecap="round"
                                                                    strokeLinejoin="round"
                                                                    strokeWidth={
                                                                        2
                                                                    }
                                                                    d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"
                                                                />
                                                            </svg>
                                                        )}
                                                    </button>
                                                }
                                            />
                                            <Input
                                                label={
                                                    isEditMode
                                                        ? 'Confirm New Password'
                                                        : 'Confirm Password'
                                                }
                                                id="password_confirmation"
                                                type={
                                                    showPassword
                                                        ? 'text'
                                                        : 'password'
                                                }
                                                autoComplete="new-password"
                                                {...register(
                                                    'password_confirmation'
                                                )}
                                                error={
                                                    errors.password_confirmation
                                                        ?.message
                                                }
                                                required={!isEditMode}
                                            />
                                        </div>
                                    ) : null}

                                    <div className="flex justify-end space-x-4 pt-4">
                                        <Button
                                            variant="ghost"
                                            type="button"
                                            onClick={() =>
                                                navigate('/settings#users')
                                            }
                                            disabled={isSubmitting}
                                        >
                                            Cancel
                                        </Button>
                                        <Button
                                            type="submit"
                                            disabled={isSubmitting}
                                        >
                                            {isSubmitting
                                                ? isEditMode
                                                    ? 'Updating User...'
                                                    : 'Inviting User...'
                                                : isEditMode
                                                ? 'Update User'
                                                : 'Invite User'}
                                        </Button>
                                    </div>
                                </form>
                            )}
                        </div>
                    </div>
                </div>
            </div>

            {/* Upgrade Modal */}
            <UpgradePromptModal
                isOpen={showUpgradeModal}
                onClose={() => {
                    setShowUpgradeModal(false);
                    navigate('/settings#users');
                }}
                reason="MEMBER_ACTIVATION"
            />
        </div>
    );
};

export default UserForm;
