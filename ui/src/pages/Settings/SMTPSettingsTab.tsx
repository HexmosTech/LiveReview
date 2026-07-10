import React, { useState, useEffect } from 'react';
import { Button, Input, Icons } from '../../components/UIPrimitives';
import apiClient from '../../api/apiClient';
import toast from 'react-hot-toast';

interface SMTPSettings {
    host: string;
    port: number;
    username: string;
    password: string;
    sender: string;
    sender_name: string;
    skip_tls: boolean;
}

const SMTPSettingsTab: React.FC = () => {
    const [settings, setSettings] = useState<SMTPSettings>({
        host: '',
        port: 587,
        username: '',
        password: '',
        sender: '',
        sender_name: 'LiveReview',
        skip_tls: false,
    });
    const [isLoading, setIsLoading] = useState(true);
    const [isSaving, setIsSaving] = useState(false);
    const [isTesting, setIsTesting] = useState(false);

    useEffect(() => {
        fetchSettings();
    }, []);

    const fetchSettings = async () => {
        try {
            const data = await apiClient.get<SMTPSettings>('/api/v1/admin/settings/smtp');
            if (data && Object.keys(data).length > 0) {
                setSettings(data);
            }
        } catch (error) {
            console.error('Failed to fetch SMTP settings:', error);
            toast.error('Failed to load SMTP settings');
        } finally {
            setIsLoading(false);
        }
    };

    const handleChange = (field: keyof SMTPSettings, value: any) => {
        setSettings(prev => ({ ...prev, [field]: value }));
    };

    const isValidEmail = (email: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);

    const handleSave = async () => {
        if (!settings.sender || !isValidEmail(settings.sender)) {
            toast.error('Please enter a valid Sender Email address');
            return;
        }
        setIsSaving(true);
        try {
            await apiClient.put('/api/v1/admin/settings/smtp', settings);
            toast.success('SMTP settings saved successfully!');
        } catch (error: any) {
            toast.error(error?.message || 'Failed to save SMTP settings');
        } finally {
            setIsSaving(false);
        }
    };

    const handleTest = async () => {
        if (!settings.sender || !isValidEmail(settings.sender)) {
            toast.error('Please enter a valid Sender Email address');
            return;
        }
        setIsTesting(true);
        try {
            const response = await apiClient.post<{message: string}>('/api/v1/admin/settings/smtp/test', settings);
            toast.success(response?.message || 'Test email sent successfully! Please check your inbox.');
        } catch (error: any) {
            toast.error(error?.message || 'Failed to send test email');
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
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                    </svg>
                </div>
                <div>
                    <h3 className="font-medium text-white">SMTP Configuration</h3>
                    <p className="text-sm text-slate-300">Configure global email delivery settings</p>
                </div>
            </div>

            <div className="space-y-6">
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <Input
                        label="SMTP Host"
                        placeholder="smtp.example.com"
                        value={settings.host}
                        onChange={(e) => handleChange('host', e.target.value)}
                    />
                    <Input
                        label="SMTP Port"
                        type="number"
                        placeholder="587"
                        value={settings.port.toString()}
                        onChange={(e) => handleChange('port', parseInt(e.target.value) || 587)}
                    />
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <Input
                        label="Username"
                        placeholder="user@example.com"
                        value={settings.username}
                        onChange={(e) => handleChange('username', e.target.value)}
                    />
                    <Input
                        label="Password"
                        type="password"
                        placeholder="••••••••"
                        value={settings.password}
                        onChange={(e) => handleChange('password', e.target.value)}
                    />
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <Input
                        label="Sender Email (From)"
                        placeholder="noreply@example.com"
                        value={settings.sender}
                        onChange={(e) => handleChange('sender', e.target.value)}
                    />
                    <Input
                        label="Sender Name"
                        placeholder="LiveReview"
                        value={settings.sender_name}
                        onChange={(e) => handleChange('sender_name', e.target.value)}
                    />
                </div>

                <div className="flex items-center space-x-2 bg-slate-800 p-4 rounded-lg">
                    <input
                        type="checkbox"
                        id="skipTls"
                        checked={settings.skip_tls}
                        onChange={(e) => handleChange('skip_tls', e.target.checked)}
                        className="w-4 h-4 text-indigo-600 bg-slate-700 border-slate-600 rounded focus:ring-indigo-500 focus:ring-2"
                    />
                    <label htmlFor="skipTls" className="text-sm font-medium text-slate-300">
                        Skip TLS Verification (Insecure, useful for self-signed certs)
                    </label>
                </div>

                <div className="flex justify-end space-x-3 pt-4 border-t border-slate-700">
                    <Button
                        variant="outline"
                        onClick={handleTest}
                        isLoading={isTesting}
                        disabled={isSaving || isTesting || !settings.host || !settings.sender}
                    >
                        Test Connection
                    </Button>
                    <Button
                        variant="primary"
                        onClick={handleSave}
                        isLoading={isSaving}
                        disabled={isSaving || isTesting || !settings.host || !settings.sender}
                    >
                        Save Settings
                    </Button>
                </div>
            </div>
        </div>
    );
};

export default SMTPSettingsTab;
