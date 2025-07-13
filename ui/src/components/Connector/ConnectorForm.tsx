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
        <div className="bg-white shadow-xl rounded-xl p-6 w-full max-w-md border border-gray-100">
            <h2 className="text-xl font-bold mb-6 text-indigo-700">Create New Connector</h2>
            <form onSubmit={handleSubmit} className="space-y-5">
                <div>
                    <label
                        className="block text-gray-700 text-sm font-semibold mb-2"
                        htmlFor="name"
                    >
                        Connector Name
                    </label>
                    <input
                        id="name"
                        name="name"
                        type="text"
                        className="w-full px-4 py-3 rounded-lg border border-gray-200 focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 transition-all duration-200 outline-none"
                        value={formData.name}
                        onChange={handleChange}
                        placeholder="My GitLab Instance"
                        required
                    />
                </div>

                <div>
                    <label
                        className="block text-gray-700 text-sm font-semibold mb-2"
                        htmlFor="type"
                    >
                        Connector Type
                    </label>
                    <div className="relative">
                        <select
                            id="type"
                            name="type"
                            className="w-full px-4 py-3 rounded-lg border border-gray-200 focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 transition-all duration-200 outline-none appearance-none bg-white"
                            value={formData.type}
                            onChange={(e) => handleChange(e)}
                            required
                        >
                            <option value="gitlab">GitLab</option>
                            <option value="github">GitHub</option>
                            <option value="custom">Custom</option>
                        </select>
                        <div className="pointer-events-none absolute inset-y-0 right-0 flex items-center px-2 text-gray-700">
                            <svg className="fill-current h-4 w-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20">
                                <path d="M9.293 12.95l.707.707L15.657 8l-1.414-1.414L10 10.828 5.757 6.586 4.343 8z" />
                            </svg>
                        </div>
                    </div>
                </div>

                <div>
                    <label
                        className="block text-gray-700 text-sm font-semibold mb-2"
                        htmlFor="url"
                    >
                        URL
                    </label>
                    <input
                        id="url"
                        name="url"
                        type="url"
                        className="w-full px-4 py-3 rounded-lg border border-gray-200 focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 transition-all duration-200 outline-none"
                        value={formData.url}
                        onChange={handleChange}
                        placeholder="https://gitlab.com"
                        required
                    />
                </div>

                <div>
                    <label
                        className="block text-gray-700 text-sm font-semibold mb-2"
                        htmlFor="apiKey"
                    >
                        API Key
                    </label>
                    <input
                        id="apiKey"
                        name="apiKey"
                        type="password"
                        className="w-full px-4 py-3 rounded-lg border border-gray-200 focus:border-indigo-500 focus:ring-2 focus:ring-indigo-200 transition-all duration-200 outline-none"
                        value={formData.apiKey}
                        onChange={handleChange}
                        placeholder="Your API Key"
                        required
                    />
                </div>

                <div className="pt-2">
                    <button
                        className="w-full py-3 px-6 text-white bg-gradient-to-r from-indigo-600 to-purple-600 hover:from-indigo-700 hover:to-purple-700 rounded-lg font-medium shadow-md hover:shadow-lg transition-all duration-200 transform hover:-translate-y-0.5"
                        type="submit"
                    >
                        Create Connector
                    </button>
                </div>
            </form>
        </div>
    );
};
