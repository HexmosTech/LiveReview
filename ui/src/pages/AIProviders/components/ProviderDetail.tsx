import React from 'react';
import { AIProvider } from '../types';
import { Badge, Button, Icons } from '../../../components/UIPrimitives';

interface ProviderDetailProps {
    provider: AIProvider;
}

const ProviderDetail: React.FC<ProviderDetailProps> = ({ provider }) => {
    return (
        <div className="mt-6 p-4 bg-slate-700 rounded-lg">
            <div className="flex items-center flex-wrap gap-2 mb-2">
                <h3 className="text-lg font-medium text-white">
                    {provider.name}
                </h3>
                {provider.supportLevel === 'recommended' && (
                    <span className="text-[10px] px-2 py-0.5 rounded-full bg-yellow-100 text-yellow-800 font-medium border border-yellow-200 tracking-wide">Recommended</span>
                )}
                {provider.supportLevel === 'experimental' && (
                    <Badge variant="warning" size="sm">Experimental</Badge>
                )}
            </div>
            <p className="text-sm text-slate-300 mb-3 leading-relaxed">
                {provider.description}
            </p>
            <div className="flex flex-col space-y-3">
                <a 
                    href={provider.url} 
                    target="_blank" 
                    rel="noopener noreferrer" 
                    className="text-sm text-blue-400 hover:text-blue-300 flex items-center"
                >
                    Visit Documentation
                </a>
                {provider.supportLevel === 'experimental' && (
                    <div className="p-3 rounded-md border border-slate-500 bg-slate-800/70 shadow-inner">
                        <div className="flex items-start justify-between">
                            <div className="pr-3">
                                <p className="text-xs text-slate-300 leading-relaxed">
                                    This provider is currently <strong>Experimental</strong>. Add your vote to help us prioritize full model coverage, advanced settings, and performance tuning.
                                </p>
                            </div>
                            <a
                                href="https://github.com/HexmosTech/LiveReview/discussions/9"
                                target="_blank"
                                rel="noopener noreferrer"
                                className="ml-2"
                            >
                                <Button variant="outline" size="sm" className="whitespace-nowrap flex items-center">
                                    <Icons.AI />
                                    <span className="ml-1">Vote / Request</span>
                                </Button>
                            </a>
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
};

export default ProviderDetail;
