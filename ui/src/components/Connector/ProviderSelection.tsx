import React from 'react';
import { Card, Button, Icons, Alert } from '../UIPrimitives';
import DomainValidator from './DomainValidator';
import { useNavigate } from 'react-router-dom';

const ProviderSelection: React.FC = () => {
    const navigate = useNavigate();
    return (
        <DomainValidator>
            <Card title="Create New Connector">
                <div className="space-y-5">
                    <h3 className="text-lg font-medium text-white">Select Git Provider</h3>
                    <p className="text-slate-300 text-sm">Choose a Git provider to connect with LiveReview</p>
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-3 pt-2">
                        <Button variant="outline" icon={<Icons.GitLab />} className="h-24 flex-col" onClick={() => navigate('/git/gitlab-com/manual')}>
                            <span className="text-base mt-2">GitLab.com</span>
                        </Button>
                        <Button variant="outline" icon={<Icons.GitLab />} className="h-24 flex-col" onClick={() => navigate('/git/gitlab-self-hosted/manual')}>
                            <span className="text-base mt-2">Self-Hosted GitLab</span>
                        </Button>
                        <Button variant="outline" icon={<Icons.GitHub />} className="h-24 flex-col" onClick={() => navigate('/git/github/manual')}>
                            <span className="text-base mt-2">GitHub</span>
                        </Button>
                        <Button variant="outline" icon={<Icons.Git />} className="h-24 flex-col" disabled>
                            <span className="text-base mt-2">Custom</span>
                            <span className="text-xs mt-1">Coming Soon</span>
                        </Button>
                    </div>
                </div>
            </Card>
        </DomainValidator>
    );
};

export default ProviderSelection;
