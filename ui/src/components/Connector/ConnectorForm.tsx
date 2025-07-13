import React, { useState } from 'react';

type ConnectorType = 'gitlab' | 'github' | 'custom';

type ConnectorFormProps = {
    onSubmit: (connector: ConnectorData) => void;
};

export type ConnectorData = {
    name: string;
    type: ConnectorType;
    url: string;
    apiKey: string;
};

export const ConnectorForm: React.FC<ConnectorFormProps> = ({ onSubmit }) => {
    const [formData, setFormData] = useState<ConnectorData>({
        name: '',
        type: 'gitlab',
        url: '',
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
        onSubmit(formData);
        // Reset form after submission
        setFormData({
            name: '',
            type: 'gitlab',
            url: '',
            apiKey: '',
        });
    };

    return (
        <div className="bg-white shadow-md rounded-lg p-6 w-full max-w-md">
            <h2 className="text-xl font-semibold mb-4">Create New Connector</h2>
            <form onSubmit={handleSubmit}>                <div className="mb-4">
                    <label
                      className="block text-gray-700 text-sm font-bold mb-2"
                      htmlFor="name"
                    >
                        Connector Name
                    </label>
                    <input
                        id="name"
                        name="name"
                        type="text"
                        className="shadow appearance-none border rounded w-full py-2 px-3 text-gray-700 leading-tight focus:outline-none focus:shadow-outline"
                        value={formData.name}
                        onChange={handleChange}
                        placeholder="My GitLab Instance"
                        required
                    />
                </div>

                <div className="mb-4">
                    <label
                        className="block text-gray-700 text-sm font-bold mb-2"
                        htmlFor="type"
                    >
                        Connector Type
                    </label>
                    <select
                        id="type"
                        name="type"
                        className="shadow appearance-none border rounded w-full py-2 px-3 text-gray-700 leading-tight focus:outline-none focus:shadow-outline"
                        value={formData.type}
                        onChange={(e) => handleChange(e)}
                        required
                    >
                        <option value="gitlab">GitLab</option>
                        <option value="github">GitHub</option>
                        <option value="custom">Custom</option>
                    </select>
                </div>

                <div className="mb-4">
                    <label
                        className="block text-gray-700 text-sm font-bold mb-2"
                        htmlFor="url"
                    >
                        URL
                    </label>
                    <input
                        id="url"
                        name="url"
                        type="url"
                        className="shadow appearance-none border rounded w-full py-2 px-3 text-gray-700 leading-tight focus:outline-none focus:shadow-outline"
                        value={formData.url}
                        onChange={handleChange}
                        placeholder="https://gitlab.com"
                        required
                    />
                </div>

                <div className="mb-6">
                    <label
                        className="block text-gray-700 text-sm font-bold mb-2"
                        htmlFor="apiKey"
                    >
                        API Key
                    </label>
                    <input
                        id="apiKey"
                        name="apiKey"
                        type="password"
                        className="shadow appearance-none border rounded w-full py-2 px-3 text-gray-700 leading-tight focus:outline-none focus:shadow-outline"
                        value={formData.apiKey}
                        onChange={handleChange}
                        placeholder="Your API Key"
                        required
                    />
                </div>

                <div className="flex items-center justify-end">
                    <button
                        className="bg-slate-800 hover:bg-slate-700 text-white font-bold py-2 px-4 rounded focus:outline-none focus:shadow-outline transition"
                        type="submit"
                    >
                        Create Connector
                    </button>
                </div>
            </form>
        </div>
    );
};
