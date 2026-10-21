import { useEffect, useState } from 'react';
import { getPublicUIConfig } from '../api/githubApp';
import { isCloudMode } from '../utils/deploymentMode';

// True where the server has the GitHub App configured and this is LiveReview Cloud; null until the server has answered.
export function useGitHubAppEnabled(): boolean | null {
    const [enabled, setEnabled] = useState<boolean | null>(null);

    useEffect(() => {
        let cancelled = false;
        getPublicUIConfig()
            .then((config) => {
                if (!cancelled) setEnabled(Boolean(config.githubAppEnabled) && isCloudMode());
            })
            .catch(() => {
                if (!cancelled) setEnabled(false);
            });
        return () => {
            cancelled = true;
        };
    }, []);

    return enabled;
}
