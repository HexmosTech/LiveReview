import React from 'react';
import { Badge } from './UIPrimitives';

interface CompactTagsProps {
  tags: string[];
  maxVisible?: number;
  className?: string;
}

const CompactTags: React.FC<CompactTagsProps> = ({ 
  tags = [], 
  maxVisible = 3, 
  className = "" 
}) => {
  if (!tags || tags.length === 0) {
    return <span className="text-slate-500 text-xs">no tags</span>;
  }

  const visibleTags = tags.slice(0, maxVisible);
  const hiddenCount = tags.length - maxVisible;

  return (
    <div className={`flex flex-wrap gap-1 ${className}`}>
      {visibleTags.map((tag, index) => (
        <Badge 
          key={`${tag}-${index}`} 
          variant="default" 
          className="text-xs px-2 py-0.5 leading-tight"
        >
          {tag}
        </Badge>
      ))}
      {hiddenCount > 0 && (
        <div title={`+${hiddenCount} more tags: ${tags.slice(maxVisible).join(', ')}`}>
          <Badge 
            variant="info" 
            className="text-xs px-2 py-0.5 leading-tight text-slate-400"
          >
            +{hiddenCount}
          </Badge>
        </div>
      )}
    </div>
  );
};

export default CompactTags;