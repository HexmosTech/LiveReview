import React, { useState } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import Reviews from './Reviews';
import NewReview from './NewReview';
import ReviewDetail from './ReviewDetail';
import ScheduledReviews from './ScheduledReviews';
import ScheduledReviewRuns from './ScheduledReviewRuns';
import CreateReviewCLI from './CreateReviewCLI';
import CreateReviewMCP from './CreateReviewMCP';

const ReviewsRoutes: React.FC = () => {
    return (
        <Routes>
            <Route index element={<Reviews />} />
            <Route path="/" element={<Reviews />} />
            <Route path="/new" element={<NewReview />} />
            <Route path="/create-cli" element={<CreateReviewCLI />} />
            <Route path="/create-mcp" element={<CreateReviewMCP />} />
            <Route path="/scheduled" element={<ScheduledReviews />} />
            <Route path="/scheduled/:repositoryId/runs" element={<ScheduledReviewRuns />} />
            <Route path="/:id" element={<ReviewDetail />} />
            <Route path="*" element={<Navigate to="/reviews" replace />} />
        </Routes>
    );
};

export default ReviewsRoutes;