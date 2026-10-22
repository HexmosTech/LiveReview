import React from 'react';
import { Button, Icons } from '../UIPrimitives';
import DomainValidator from './DomainValidator';
import { useNavigate } from 'react-router-dom';
import { useGitHubAppEnabled } from '../../hooks/useGitHubAppEnabled';

const ProviderSelection: React.FC = () => {
    const navigate = useNavigate();
    const githubAppEnabled = useGitHubAppEnabled();
    return (
        <DomainValidator>
            <div className="space-y-5">
                <h3 className="text-lg font-medium text-white">Select Git Provider</h3>
                <p className="text-slate-300 text-sm">Choose a Git provider to connect with LiveReview</p>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-3 pt-2">
                    {githubAppEnabled && (
                        <Button
                            variant="outline"
                            icon={<Icons.GitHub />}
                            className="h-24 flex-col md:col-span-2"
                            onClick={() => navigate('/git/github-app')}
                        >
                            <span className="text-base mt-2">
                                GitHub App <span className="ml-1 rounded bg-blue-600 px-1.5 py-0.5 text-xs font-medium text-white">Recommended</span>
                            </span>
                        </Button>
                    )}
                    <Button variant="outline" icon={<Icons.GitLab />} className="h-24 flex-col" onClick={() => navigate('/git/gitlab-com/manual')}>
                        <span className="text-base mt-2">GitLab.com</span>
                    </Button>
                    <Button variant="outline" icon={<Icons.GitLab />} className="h-24 flex-col" onClick={() => navigate('/git/gitlab-self-hosted/manual')}>
                        <span className="text-base mt-2">Self-Hosted GitLab</span>
                    </Button>
                    <Button variant="outline" icon={<Icons.GitHub />} className="h-24 flex-col" onClick={() => navigate('/git/github/manual')}>
                        <span className="text-base mt-2">GitHub</span>
                    </Button>
                    <Button variant="outline" icon={<Icons.Bitbucket />} className="h-24 flex-col" onClick={() => navigate('/git/bitbucket/manual')}>
                        <span className="text-base mt-2">Bitbucket</span>
                    </Button>
                    <Button variant="outline" icon={<Icons.Gitea />} className="h-24 flex-col" onClick={() => navigate('/git/gitea/manual')}>
                        <span className="text-base mt-2">Gitea</span>
                    </Button>
                    <Button variant="outline" icon={<Icons.AzureDevOps />} className="h-24 flex-col" onClick={() => navigate('/git/azuredevops/manual')}>
                        <span className="text-base mt-2">Azure DevOps</span>
                    </Button>
                </div>
            </div>
        </DomainValidator>
    );
};

export default ProviderSelection;
