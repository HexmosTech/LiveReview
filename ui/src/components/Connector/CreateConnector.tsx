import React, { useState } from 'react';
import { Card, Button } from '../UIPrimitives';
import GitLabConnector from './GitLabConnector';
import DomainValidator from './DomainValidator';

// This is an example component that would be used to create a new connector
// It demonstrates how to use the DomainValidationWrapper at the "Create connector" level
const CreateConnector: React.FC = () => {
  const [activeTab, setActiveTab] = useState<string>('gitlab-com');

  const handleConnectorSubmit = (data: any) => {
    console.log('Connector data submitted:', data);
    // Handle the connector creation logic here
  };

  return (
    <div className="container mx-auto px-4 py-8">
      <h1 className="text-2xl font-bold text-white mb-6">Create New Connector</h1>
      
      {/* Wrap the entire connector creation UI with DomainValidator */}
      <DomainValidator>
        <Card className="overflow-hidden">
          <div className="flex border-b border-slate-700">
            <button
              className={`px-4 py-3 text-sm font-medium ${activeTab === 'gitlab-com' ? 'bg-slate-700 text-white' : 'text-slate-400 hover:text-slate-200'}`}
              onClick={() => setActiveTab('gitlab-com')}
            >
              GitLab.com
            </button>
            <button
              className={`px-4 py-3 text-sm font-medium ${activeTab === 'gitlab-self-hosted' ? 'bg-slate-700 text-white' : 'text-slate-400 hover:text-slate-200'}`}
              onClick={() => setActiveTab('gitlab-self-hosted')}
            >
              Self-hosted GitLab
            </button>
            <button
              className={`px-4 py-3 text-sm font-medium ${activeTab === 'github' ? 'bg-slate-700 text-white' : 'text-slate-400 hover:text-slate-200'}`}
              onClick={() => setActiveTab('github')}
            >
              GitHub
            </button>
          </div>
          
          <div className="p-4">
            {activeTab === 'gitlab-com' && (
              <GitLabConnector 
                type="gitlab-com" 
                onSubmit={handleConnectorSubmit}
              />
            )}
            
            {activeTab === 'gitlab-self-hosted' && (
              <GitLabConnector 
                type="gitlab-self-hosted" 
                onSubmit={handleConnectorSubmit}
              />
            )}
            
            {activeTab === 'github' && (
              <div>GitHub connector component would go here</div>
            )}
          </div>
        </Card>
      </DomainValidator>
    </div>
  );
};

export default CreateConnector;
