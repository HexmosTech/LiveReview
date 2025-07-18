import React, { useState } from 'react';
import { Card, Input, Select, Button, Icons } from '../UIPrimitives';
import { ConnectorType } from '../../store/Connector/reducer';

type ConnectorFormProps = {
    onSubmit: (connector: ConnectorData) => void;
};

export type ConnectorData = {
    name: string;
    type: ConnectorType;
    url: string;
    apiKey: string;
    id?: string;
    createdAt?: number;
};

export const ConnectorForm: React.FC<ConnectorFormProps> = ({ onSubmit }) => {
    const [formData, setFormData] = useState<ConnectorData>({
        name: '',
        type: 'gitlab-com',
        url: 'https://gitlab.com',
        apiKey: '',
    });

    const handleChange = (
        e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>
    ) => {
        const { name, value } = e.target;
        setFormData((prev) => ({
            ...prev,
            [name]: value,
        }));
    };

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault();
        
        // Add ID and timestamp
        const connectorWithMeta = {
            ...formData,
            id: `connector-${Date.now()}`,
            createdAt: Date.now(),
        };
        
        onSubmit(connectorWithMeta);
        
        // Reset form after submission
        setFormData({
            name: '',
            type: 'gitlab-com',
            url: 'https://gitlab.com',
            apiKey: '',
        });
    };

    const getConnectorIcon = () => {
        switch (formData.type) {
            case 'gitlab-com':
            case 'gitlab-self-hosted':
                return <Icons.GitLab />;
            case 'github':
                return <Icons.GitHub />;
            default:
                return <Icons.Git />;
        }
    };

    return (
        <Card title="Create New Connector">
            <form onSubmit={handleSubmit} className="space-y-5">
                <Input
                    id="name"
                    name="name"
                    label="Connector Name"
                    type="text"
                    value={formData.name}
                    onChange={handleChange}
                    placeholder="My GitLab Instance"
                    icon={getConnectorIcon()}
                    required
                />

                <Select
                    id="type"
                    name="type"
                    label="Connector Type"
                    value={formData.type}
                    onChange={(e) => {
                        const newType = e.target.value as ConnectorType;
                        setFormData((prev) => ({
                            ...prev,
                            type: newType,
                            // Set URL field for GitLab.com, clear for other types
                            url: newType === 'gitlab-com' ? 'https://gitlab.com' : '',
                        }));
                    }}
                    options={[
                        { value: 'gitlab-com', label: 'GitLab.com' },
                        { value: 'gitlab-self-hosted', label: 'Self-Hosted GitLab' },
                        { value: 'github', label: 'GitHub' },
                        { value: 'custom', label: 'Custom' },
                    ]}
                    required
                />

                <Input
                    id="url"
                    name="url"
                    label="URL"
                    type="url"
                    value={formData.url}
                    onChange={handleChange}
                    placeholder={formData.type === 'gitlab-self-hosted' ? "https://gitlab.mycompany.com" : "https://gitlab.com"}
                    required
                    disabled={formData.type === 'gitlab-com'}
                />

                <Input
                    id="apiKey"
                    name="apiKey"
                    label="API Key"
                    type="password"
                    value={formData.apiKey}
                    onChange={handleChange}
                    placeholder="Your API Key"
                    helperText="Your API key will be stored securely"
                    required
                />

                <Button
                    type="submit"
                    variant="primary"
                    fullWidth
                    icon={<Icons.Add />}
                >
                    Create Connector
                </Button>
            </form>
        </Card>
    );
};
