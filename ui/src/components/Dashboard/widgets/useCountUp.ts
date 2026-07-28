import { useEffect, useRef, useState } from 'react';

const easeOutCubic = (t: number): number => 1 - Math.pow(1 - t, 3);

/**
 * Animates a numeric value from 0 up to `target` once, on mount / whenever `target` changes.
 * Used by KPI-style widgets for the "count-up" first-glance effect.
 */
export function useCountUp(target: number, durationMs = 900): number {
    const [value, setValue] = useState(0);
    const startRef = useRef<number | null>(null);
    const frameRef = useRef<number | null>(null);

    useEffect(() => {
        startRef.current = null;
        if (frameRef.current) cancelAnimationFrame(frameRef.current);

        const step = (timestamp: number) => {
            if (startRef.current === null) startRef.current = timestamp;
            const elapsed = timestamp - startRef.current;
            const progress = Math.min(1, elapsed / durationMs);
            setValue(target * easeOutCubic(progress));
            if (progress < 1) {
                frameRef.current = requestAnimationFrame(step);
            }
        };

        frameRef.current = requestAnimationFrame(step);
        return () => {
            if (frameRef.current) cancelAnimationFrame(frameRef.current);
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [target, durationMs]);

    return value;
}
