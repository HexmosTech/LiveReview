import { formatDistanceToNow, format } from 'date-fns';

/**
 * Humanizes a timestamp to a relative time format (e.g., "2 hours ago")
 * with the full date/time shown on hover
 */
export const humanizeTimestamp = (timestamp: string | Date): { 
  relative: string; 
  absolute: string; 
} => {
  const date = typeof timestamp === 'string' ? new Date(timestamp) : timestamp;
  
  // Check if date is valid
  if (isNaN(date.getTime())) {
    return {
      relative: 'Invalid date',
      absolute: 'Invalid date'
    };
  }

  const relative = formatDistanceToNow(date, { addSuffix: true });
  const absolute = format(date, 'MMM d, yyyy h:mm:ss a');
  
  return { relative, absolute };
};

/**
 * Component for displaying a humanized timestamp with hover tooltip
 */
export interface HumanizedTimestampProps {
  timestamp: string | Date;
  className?: string;
}

export const formatTimestamp = (timestamp: string | Date): string => {
  const date = typeof timestamp === 'string' ? new Date(timestamp) : timestamp;
  
  if (isNaN(date.getTime())) {
    return 'Invalid date';
  }
  
  return formatDistanceToNow(date, { addSuffix: true });
};
