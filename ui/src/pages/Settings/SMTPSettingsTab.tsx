import React, { useState, useEffect } from 'react';
import { Button, Input, Icons } from '../../components/UIPrimitives';
import apiClient from '../../api/apiClient';
import { notify } from '../../utils/notify';

interface SMTPSettings {
    host: string;
    port: number;
    username: string;
    password: string;
    sender: string;
    sender_name: string;
    skip_tls: boolean;
}

const ResultBanner: React.FC<{ type: 'success' | 'error'; message: string }> = ({ type, message }) => (
    <div className={`mb-4 p-4 rounded-lg flex items-center ${
        type === 'success'
            ? 'bg-green-900/30 border border-green-600'
            : 'bg-red-900/30 border border-red-600'
    }`}>
        {type === 'success' ? (
            <svg className="w-5 h-5 text-green-500 mr-3 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
            </svg>
        ) : (
            <svg className="w-5 h-5 text-red-500 mr-3 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
            </svg>
        )}
        <span className={type === 'success' ? 'text-green-200' : 'text-red-200'}>
            {message}
        </span>
    </div>
);

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
    const [testResult, setTestResult] = useState<{ type: 'success' | 'error'; message: string } | null>(null);
    const [saveResult, setSaveResult] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

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
            notify.error('Failed to load SMTP settings');
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
            notify.error('Please enter a valid Sender Email address');
            return;
        }
        setIsSaving(true);
        setSaveResult(null);
        setTestResult(null);
        try {
            await apiClient.put('/api/v1/admin/settings/smtp', settings);
            setSaveResult({ type: 'success', message: 'SMTP settings saved successfully!' });
            notify.success('SMTP settings saved successfully!');
        } catch (error: any) {
            const errorMessage = error?.message || 'Failed to save SMTP settings';
            setSaveResult({ type: 'error', message: errorMessage });
            notify.error(errorMessage);
        } finally {
            setIsSaving(false);
        }
    };

    const handleTest = async () => {
        if (!settings.sender || !isValidEmail(settings.sender)) {
            notify.error('Please enter a valid Sender Email address');
            return;
        }
        setIsTesting(true);
        setTestResult(null);
        try {
            const response = await apiClient.post<{message: string}>('/api/v1/admin/settings/smtp/test', settings);
            const successMessage = response?.message || 'Test email sent successfully! Please check your inbox.';
            setTestResult({ type: 'success', message: successMessage });
            notify.success(successMessage);
        } catch (error: any) {
            const errorMessage = error?.message || 'Failed to send test email';
            setTestResult({ type: 'error', message: errorMessage });
            notify.error(errorMessage);
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

            {testResult && <ResultBanner type={testResult.type} message={testResult.message} />}

            {saveResult && <ResultBanner type={saveResult.type} message={saveResult.message} />}

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
