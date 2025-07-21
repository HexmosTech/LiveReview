import React from 'react';
import { useNavigate } from 'react-router-dom';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import { 
    StatCard, 
    Section, 
    PageHeader, 
    Card, 
    Badge, 
    EmptyState, 
    Button, 
    Icons 
} from '../UIPrimitives';

export const Dashboard: React.FC = () => {
    const dispatch = useAppDispatch();
    const navigate = useNavigate();
    const connectors = useAppSelector((state) => state.Connector.connectors);
    
    // Placeholder stats
    const aiComments = 0;
    const codeReviews = 0;
    const aiService = 'Gemini';
    const apiKey = 'sk-xxxxxxx';

    // Mock recent activity
    const recentActivity = [
        { id: 1, action: 'Code review', repo: 'frontend/main', date: '2h ago' },
        { id: 2, action: 'Comment added', repo: 'api/feature-branch', date: '5h ago' },
        { id: 3, action: 'Connected', repo: 'GitLab', date: '1d ago' }
    ];

    return (
        <div className="min-h-screen">
            <main className="container mx-auto px-4 py-8">
                <PageHeader 
                    title="Dashboard" 
                    description="Monitor your code review activity and connected services"
                    actions={
                        <Button 
                            variant="primary" 
                            icon={<Icons.Add />}
                            onClick={() => navigate('/reviews/new')}
                        >
                            New Review
                        </Button>
                    }
                />

                {/* Primary Metrics - The most important ones */}
                <Section>
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-5 mb-6">
                        <StatCard 
                            variant="primary"
                            title="Code Reviews by AI" 
                            value={codeReviews} 
                            icon={
                                <svg className="w-6 h-6" fill="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                    <path d="M14.72,8.79l-4.29,4.3L8.78,11.44a1,1,0,1,0-1.41,1.41l2.35,2.36a1,1,0,0,0,.71.29,1,1,0,0,0,.7-.29l5-5a1,1,0,0,0,0-1.42A1,1,0,0,0,14.72,8.79ZM12,2A10,10,0,1,0,22,12,10,10,0,0,0,12,2Zm0,18a8,8,0,1,1,8-8A8,8,0,0,1,12,20Z"/>
                                </svg>
                            }
                            description="Total number of code reviews automated by AI"
                        />
                        <StatCard 
                            variant="primary"
                            title="AI Comments Posted" 
                            value={aiComments} 
                            icon={
                                <svg className="w-6 h-6" fill="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                    <path d="M12,2A10,10,0,0,0,2,12a9.89,9.89,0,0,0,2.26,6.33l-2,2a1,1,0,0,0-.21,1.09A1,1,0,0,0,3,22h9A10,10,0,0,0,12,2Zm0,18H5.41l.93-.93a1,1,0,0,0,0-1.41A8,8,0,1,1,12,20Z"/>
                                </svg>
                            }
                            description="Total comments posted by AI across all repositories"
                        />
                    </div>
                    
                    {/* Secondary Metrics */}
                    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-2 gap-5">
                        <StatCard 
                            title="Connected Git Providers" 
                            value={connectors.length} 
                            icon={<Icons.Git />}
                        />
                        <StatCard 
                            title="AI Service" 
                            value={aiService} 
                            icon={<Icons.AI />}
                        />
                    </div>
                </Section>

                {/* Brand showcase */}
                <div className="my-6 bg-gradient-to-r from-blue-800 to-blue-600 rounded-xl p-6 text-white shadow-lg">
                    <div className="flex flex-col md:flex-row items-center">
                        <img src="assets/logo-mono.svg" alt="LiveReview Logo" className="h-20 w-auto mb-4 md:mb-0 md:mr-8 logo-animation" />
                        <div>
                            <h2 className="text-2xl font-bold mb-3">Welcome to LiveReview</h2>
                            <p className="text-blue-50 text-base leading-relaxed mb-4">
                                Automated code reviews powered by AI. Connect your Git repositories and start receiving 
                                intelligent feedback to improve code quality and development velocity.
                            </p>
                            <div className="flex flex-wrap gap-6 mt-2">
                                <div className="flex items-center">
                                    <div className="w-2 h-2 bg-blue-300 rounded-full mr-2"></div>
                                    <span className="text-blue-100 font-semibold">{codeReviews} Code Reviews</span>
                                </div>
                                <div className="flex items-center">
                                    <div className="w-2 h-2 bg-blue-300 rounded-full mr-2"></div>
                                    <span className="text-blue-100 font-semibold">{aiComments} AI Comments</span>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>

                <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
                    <div className="lg:col-span-2">
                        <Card 
                            title="Recent Activity" 
                            badge={`${recentActivity.length}`}
                            badgeColor="bg-blue-100 text-blue-800"
                            className="card-brand"
                        >
                            <div className="mb-4 p-3 bg-blue-700 bg-opacity-30 rounded-lg border border-blue-600">
                                <div className="flex justify-between items-center">
                                    <div className="flex items-center">
                                        <svg className="w-5 h-5 text-blue-300 mr-2" fill="currentColor" viewBox="0 0 20 20" xmlns="http://www.w3.org/2000/svg">
                                            <path fillRule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clipRule="evenodd"></path>
                                        </svg>
                                        <p className="text-sm font-medium text-blue-100">Your AI-powered code review metrics are highlighted above</p>
                                    </div>
                                    <div>
                                        <Button 
                                            variant="outline" 
                                            size="sm"
                                            className="border-blue-400 text-blue-200"
                                        >
                                            View Analytics
                                        </Button>
                                    </div>
                                </div>
                            </div>
                            {recentActivity.length > 0 ? (
                                <ul className="divide-y divide-slate-700">
                                    {recentActivity.map((item) => (
                                        <li key={item.id} className="py-3 flex justify-between items-center">
                                            <div>
                                                <p className="text-sm font-medium text-slate-100">{item.action}</p>
                                                <p className="text-sm text-slate-300">{item.repo}</p>
                                            </div>
                                            <Badge variant="default" size="sm">{item.date}</Badge>
                                        </li>
                                    ))}
                                </ul>
                            ) : (
                                <EmptyState
                                    icon={<Icons.EmptyState />}
                                    title="No recent activity"
                                    description="Your recent code review activity will appear here"
                                />
                            )}
                        </Card>
                    </div>

                    <div>
                        <Card 
                            title="Quick Actions" 
                            subtitle="Common tasks and shortcuts"
                        >
                            <div className="space-y-3">
                                <Button 
                                    variant="outline" 
                                    fullWidth 
                                    className="justify-start" 
                                    icon={<Icons.Git />}
                                >
                                    Connect Git Provider
                                </Button>
                                <Button 
                                    variant="outline" 
                                    fullWidth 
                                    className="justify-start" 
                                    icon={<Icons.AI />}
                                >
                                    Configure AI Service
                                </Button>
                                <Button 
                                    variant="outline" 
                                    fullWidth 
                                    className="justify-start" 
                                    icon={<Icons.Settings />}
                                >
                                    Review Settings
                                </Button>
                            </div>
                        </Card>

                        <Card 
                            title="AI Service Status" 
                            className="mt-6"
                        >
                            <div className="flex items-center mb-4">
                                <div className="flex-shrink-0 mr-3">
                                    <div className="h-10 w-10 rounded-full bg-blue-600 text-white flex items-center justify-center">
                                        <Icons.AI />
                                    </div>
                                </div>
                                <div>
                                    <h4 className="font-medium text-white">{aiService}</h4>
                                    <p className="text-sm text-slate-300">API Key: {apiKey.substring(0, 3)}...{apiKey.substring(apiKey.length - 4)}</p>
                                </div>
                            </div>
                            <Badge variant="success" className="w-full justify-center py-1">
                                Active
                            </Badge>
                        </Card>
                    </div>
                </div>
            </main>
        </div>
    );
};
