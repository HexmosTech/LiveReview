import React from 'react';
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
                            onClick={() => {/* TODO: Add new code review */}}
                        >
                            New Review
                        </Button>
                    }
                />

                <Section>
                    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-5">
                        <StatCard 
                            title="Connected Services" 
                            value={connectors.length} 
                            icon={<Icons.Git />}
                        />
                        <StatCard 
                            title="AI Comments Posted" 
                            value={aiComments} 
                            icon={<Icons.Info />}
                        />
                        <StatCard 
                            title="Code Reviews by AI" 
                            value={codeReviews} 
                            icon={<Icons.Dashboard />}
                        />
                        <StatCard 
                            title="AI Service" 
                            value={aiService} 
                            icon={<Icons.AI />}
                        />
                    </div>
                </Section>

                {/* Brand showcase */}
                <div className="my-6 bg-gradient-to-r from-blue-900 to-blue-800 rounded-xl p-6 text-white">
                    <div className="flex flex-col md:flex-row items-center">
                        <img src="/assets/logo-mono.svg" alt="LiveReview Logo" className="h-16 w-auto mb-4 md:mb-0 md:mr-6 logo-animation" />
                        <div>
                            <h2 className="text-xl font-semibold mb-2">Welcome to LiveReview</h2>
                            <p className="text-blue-100">
                                Automated code reviews powered by AI. Connect your Git repositories and start receiving 
                                intelligent feedback to improve code quality and development velocity.
                            </p>
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
                            {recentActivity.length > 0 ? (
                                <ul className="divide-y divide-gray-100">
                                    {recentActivity.map((item) => (
                                        <li key={item.id} className="py-3 flex justify-between items-center">
                                            <div>
                                                <p className="text-sm font-medium text-gray-900">{item.action}</p>
                                                <p className="text-sm text-gray-500">{item.repo}</p>
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
                                    <div className="h-10 w-10 rounded-full bg-blue-100 flex items-center justify-center">
                                        <Icons.AI />
                                    </div>
                                </div>
                                <div>
                                    <h4 className="font-medium text-gray-900">{aiService}</h4>
                                    <p className="text-sm text-gray-500">API Key: {apiKey.substring(0, 3)}...{apiKey.substring(apiKey.length - 4)}</p>
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
