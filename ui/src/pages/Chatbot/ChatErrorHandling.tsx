import React from 'react';



interface ChatErrorBlockquoteProps {
  lines: string[];
  formatLine: (line: string) => React.ReactNode;
}

export function ChatErrorBlockquote({ lines, formatLine }: ChatErrorBlockquoteProps) {
  if (!lines || lines.length === 0) return null;
  
  return (
    <blockquote className="border-l-2 border-indigo-500 text-slate-300 pl-3 pr-3 pt-0 pb-2 rounded-r-md mb-2 mt-1">
      <div className="text-indigo-400 font-bold mb-1">
        {formatLine(lines[0])}
      </div>
      <div className="italic">
        {lines.slice(1).map((bLine, bIdx) => (
          <div key={`${bIdx}-${bLine}`} className={bLine.trim() === '' ? 'h-2' : 'mb-1 leading-relaxed [&>*:first-child]:mt-0'}>
            {formatLine(bLine)}
          </div>
        ))}
      </div>
    </blockquote>
  );
}
