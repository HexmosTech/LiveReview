import React from 'react';
import { humanizeTimestamp } from '../../utils/date';

export interface HumanizedTimestampProps {
  timestamp: string | Date;
  className?: string;
  showIcon?: boolean;
}

export const HumanizedTimestamp: React.FC<HumanizedTimestampProps> = ({ 
  timestamp, 
  className = '', 
  showIcon = false 
}) => {
  const { relative, absolute } = humanizeTimestamp(timestamp);
  
  return (
    <span 
      className={`cursor-help ${className}`}
      title={absolute}
    >
      {showIcon && (
        <svg 
          className="w-3 h-3 inline mr-1" 
          fill="currentColor" 
          viewBox="0 0 20 20"
        >
          <path 
            fillRule="evenodd" 
            d="M10 18a8 8 0 100-16 8 8 0 000 16zm1-12a1 1 0 10-2 0v4a1 1 0 00.293.707l2.828 2.829a1 1 0 101.415-1.415L11 9.586V6z" 
            clipRule="evenodd" 
          />
        </svg>
      )}
      {relative}
    </span>
  );
};
