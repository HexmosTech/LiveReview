import React, { useEffect, useRef, useState } from 'react';
import { useLocation } from 'react-router-dom';

let startNavigationProgress: () => void = () => {};

export function triggerNavigationProgress(): void {
    startNavigationProgress();
}

export const NavigationProgressBar: React.FC = () => {
    const location = useLocation();
    const [active, setActive] = useState(false);
    const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

    useEffect(() => {
        startNavigationProgress = () => {
            if (timerRef.current) clearTimeout(timerRef.current);
            setActive(true);
        };
        return () => {
            startNavigationProgress = () => {};
        };
    }, []);

    useEffect(() => {
        if (active) {
            timerRef.current = setTimeout(() => setActive(false), 200);
        }
        return () => {
            if (timerRef.current) clearTimeout(timerRef.current);
        };
    }, [location.pathname]);

    if (!active) return null;

    return (
        <div className="fixed top-0 left-0 right-0 z-[200] h-0.5">
            <div className="h-full bg-blue-500 animate-[nav-progress_1.5s_ease-in-out_infinite]" />
            <style>{`
                @keyframes nav-progress {
                    0% { width: 0%; margin-left: 0%; }
                    50% { width: 60%; margin-left: 20%; }
                    100% { width: 0%; margin-left: 100%; }
                }
            `}</style>
        </div>
    );
};
