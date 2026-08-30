import React from 'react';
import { Alert, Icons } from './UIPrimitives';
import { useProductionUrlCheck } from '../hooks/useProductionUrlCheck';

interface ProductionUrlWarningProps {
  floating?: boolean;
  onClose?: () => void;
}

const ProductionUrlWarning: React.FC<ProductionUrlWarningProps> = ({ floating, onClose }) => {
    const { isConfigured, isLoading } = useProductionUrlCheck();

    if (isLoading || isConfigured) return null;

    return (
        <Alert variant="warning" icon={<Icons.Warning />} className={floating ? '' : 'mb-4 w-full'} floating={floating} onClose={onClose}>
            <p className="font-medium">Production URL Required</p>
            <p className="text-sm mt-1">
                Configure your production URL before using this feature.
            </p>
            <a
                href="/settings#instance"
                className="mt-2 inline-block text-sm font-medium underline"
            >
                Settings → Instance
            </a>
        </Alert>
    );
};

export default ProductionUrlWarning;
