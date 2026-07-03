import React from 'react';
import {
    Card,
    Icons
} from '../../../components/UIPrimitives';

interface AdaptiveReviewInfoProps {
    activeRole: 'leader' | 'helper';
    variant?: 'tab' | 'settings';
}

const AdaptiveReviewInfo: React.FC<AdaptiveReviewInfoProps> = ({ activeRole, variant = 'tab' }) => {
    if (variant === 'tab') {
        return (
            <p className="text-sm text-slate-400 mb-4">
                {activeRole === 'leader'
                    ? 'Leader Model: the primary AI that analyzes your code and decides what to flag.'
                    : 'Helper Model: an optional, cheaper AI that expands and polishes the Leader\'s findings into clear review comments.'}
            </p>
        );
    }

    return (
        <Card title="What is Adaptive Review?" className="mb-4">
            <div className="space-y-4">
                <div className="flex items-start">
                    <div className="text-blue-400 mt-1 mr-2 flex-shrink-0">
                        <Icons.Info />
                    </div>
                    <p className="text-sm text-slate-300">
                        Adaptive Review pairs a Leader model (finds and judges issues) with a Helper model
                        (expands the Leader's short notes into clear, polished comments). Splitting the work
                        this way typically cuts review cost 40-50% with no loss in detection quality, since
                        the Leader still decides everything about what's worth flagging.
                    </p>
                </div>
                <div className="flex items-start">
                    <div className="text-blue-400 mt-1 mr-2 flex-shrink-0">
                        <Icons.Info />
                    </div>
                    <p className="text-sm text-slate-300">
                        If the Helper model fails or isn't configured, LiveReview automatically falls back to
                        posting the Leader model's own output &mdash; reviews never fail because of a Helper
                        model issue.
                    </p>
                </div>
                <div className="flex items-start">
                    <div className="text-blue-400 mt-1 mr-2 flex-shrink-0">
                        <Icons.Info />
                    </div>
                    <p className="text-sm text-slate-300">
                        <strong>Concise Then Expand</strong> asks the Leader for terse notes and has the Helper
                        expand them into full comments. <strong>Polish Only</strong> asks the Leader for full
                        comments and has the Helper just clean up the wording.
                    </p>
                </div>
            </div>
        </Card>
    );
};

export default AdaptiveReviewInfo;
