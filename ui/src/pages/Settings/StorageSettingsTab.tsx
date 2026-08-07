import React, { useState, useEffect } from 'react';
import { Button, Input } from '../../components/UIPrimitives';
import apiClient from '../../api/apiClient';
import toast from 'react-hot-toast';

interface StorageSettings {
    backend: 'filesystem' | 's3';
    local_dir?: string;
    bucket?: string;
    endpoint?: string;
    region?: string;
    access_key_id?: string;
    secret_access_key?: string;
}

const DEFAULT_SETTINGS: StorageSettings = {
    backend: 'filesystem',
    local_dir: '',
    bucket: '',
    endpoint: '',
    region: '',
    access_key_id: '',
    secret_access_key: '',
};

// Where git-lrc-computed review artifacts (e.g. blast-radius reports) are
// stored - see internal/blobstore and internal/api/diff_review.go's
// Put/GetDiffReviewArtifact. Filesystem is the zero-config default; the
// S3-compatible backend covers both real AWS S3 and Backblaze B2 (B2 speaks
// the S3 API over a custom endpoint).
const StorageSettingsTab: React.FC = () => {
    const [settings, setSettings] = useState<StorageSettings>(DEFAULT_SETTINGS);
    const [isLoading, setIsLoading] = useState(true);
    const [isSaving, setIsSaving] = useState(false);
    const [isTesting, setIsTesting] = useState(false);

    useEffect(() => {
        fetchSettings();
    }, []);

    const fetchSettings = async () => {
        try {
            const data = await apiClient.get<StorageSettings>('/api/v1/admin/settings/storage');
            if (data && Object.keys(data).length > 0) {
                setSettings({ ...DEFAULT_SETTINGS, ...data });
            }
        } catch (error) {
            console.error('Failed to fetch storage settings:', error);
            toast.error('Failed to load storage settings');
        } finally {
            setIsLoading(false);
        }
    };

    const handleChange = (field: keyof StorageSettings, value: string) => {
        setSettings(prev => ({ ...prev, [field]: value }));
    };

    const isS3 = settings.backend === 's3';
    const canSubmit = !isS3 || !!settings.bucket;

    const handleSave = async () => {
        if (isS3 && !settings.bucket) {
            toast.error('Please enter a bucket name');
            return;
        }
        setIsSaving(true);
        try {
            await apiClient.put('/api/v1/admin/settings/storage', settings);
            toast.success('Storage settings saved successfully!');
        } catch (error: any) {
            toast.error(error?.message || 'Failed to save storage settings');
        } finally {
            setIsSaving(false);
        }
    };

    const handleTest = async () => {
        if (isS3 && !settings.bucket) {
            toast.error('Please enter a bucket name');
            return;
        }
        setIsTesting(true);
        try {
            const response = await apiClient.post<{ message: string }>('/api/v1/admin/settings/storage/test', settings);
            toast.success(response?.message || 'Storage connection succeeded');
        } catch (error: any) {
            toast.error(error?.message || 'Storage connection failed');
        } finally {
            setIsTesting(false);
        }
    };

    if (isLoading) {
        return (
            <div className="flex justify-center p-8">
                <div className="w-8 h-8 border-2 border-indigo-500 border-t-transparent rounded-full animate-spin"></div>
            </div>
        );
    }

    return (
        <div>
            <div className="flex items-center mb-6">
                <div className="text-indigo-400 mr-3">
                    <svg className="w-8 h-8" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0H4m8-9v9" />
                    </svg>
                </div>
                <div>
                    <h3 className="font-medium text-white">Blob Storage</h3>
                    <p className="text-sm text-slate-300">Where git-lrc review artifacts (e.g. blast-radius reports) are stored</p>
                </div>
            </div>

            <div className="space-y-6">
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div>
                        <label className="block text-sm font-medium text-slate-300 mb-1">Backend</label>
                        <select
                            className="w-full bg-slate-800 border border-slate-600 rounded-lg px-3 py-2 text-white focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500"
                            value={settings.backend}
                            onChange={(e) => handleChange('backend', e.target.value)}
                        >
                            <option value="filesystem">Filesystem (local disk)</option>
                            <option value="s3">S3-compatible (AWS S3 / Backblaze B2)</option>
                        </select>
                    </div>
                </div>

                {!isS3 && (
                    <div className="grid grid-cols-1 gap-4">
                        <Input
                            label="Local directory"
                            placeholder="./data/blobs"
                            value={settings.local_dir || ''}
                            onChange={(e) => handleChange('local_dir', e.target.value)}
                        />
                    </div>
                )}

                {isS3 && (
                    <>
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                            <Input
                                label="Bucket"
                                placeholder="livereview-private"
                                value={settings.bucket || ''}
                                onChange={(e) => handleChange('bucket', e.target.value)}
                            />
                            <Input
                                label="Region"
                                placeholder="us-east-1"
                                value={settings.region || ''}
                                onChange={(e) => handleChange('region', e.target.value)}
                            />
                        </div>
                        <div className="grid grid-cols-1 gap-4">
                            <Input
                                label="Custom endpoint (leave blank for real AWS S3)"
                                placeholder="https://s3.us-east-005.backblazeb2.com"
                                value={settings.endpoint || ''}
                                onChange={(e) => handleChange('endpoint', e.target.value)}
                            />
                        </div>
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                            <Input
                                label="Access Key ID"
                                value={settings.access_key_id || ''}
                                onChange={(e) => handleChange('access_key_id', e.target.value)}
                            />
                            <Input
                                label="Secret Access Key"
                                type="password"
                                placeholder="••••••••"
                                value={settings.secret_access_key || ''}
                                onChange={(e) => handleChange('secret_access_key', e.target.value)}
                            />
                        </div>
                    </>
                )}

                <div className="flex justify-end space-x-3 pt-4 border-t border-slate-700">
                    <Button
                        variant="outline"
                        onClick={handleTest}
                        isLoading={isTesting}
                        disabled={isSaving || isTesting || !canSubmit}
                    >
                        Test Connection
                    </Button>
                    <Button
                        variant="primary"
                        onClick={handleSave}
                        isLoading={isSaving}
                        disabled={isSaving || isTesting || !canSubmit}
                    >
                        Save Settings
                    </Button>
                </div>
            </div>
        </div>
    );
};

export default StorageSettingsTab;
