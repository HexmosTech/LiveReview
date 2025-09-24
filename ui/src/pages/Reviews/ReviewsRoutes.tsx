import React, { useState } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import Reviews from './Reviews';
import NewReview from './NewReview';
import ReviewDetail from './ReviewDetail';

const ReviewsRoutes: React.FC = () => {
    return (
        <Routes>
            <Route index element={<Reviews />} />
            <Route path="/" element={<Reviews />} />
            <Route path="/new" element={<NewReview />} />
            <Route path="/:id" element={<ReviewDetail />} />
            <Route path="*" element={<Navigate to="/reviews" replace />} />
        </Routes>
    );
};

export default ReviewsRoutes;