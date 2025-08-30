import React from 'react';
import { ErrorBoundary } from '@components/ErrorBoundary';

const Home = () => {
    return (
        <ErrorBoundary>
            <h1>React Redux Boilerplate</h1>
            <p>You can put the components of your app here</p>
        </ErrorBoundary>
    );
};

export default Home;
