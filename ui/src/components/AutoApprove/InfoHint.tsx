import React from 'react';
import { Tooltip, Icons } from '../UIPrimitives';

// A small "i" icon with a hover tooltip. Used next to every auto-approve /
// risk-policy control, since these settings are new and not self-explanatory
// to most users. Tooltip text must be written in Simplified Technical
// English (ASD-STE100 style): short sentences, one idea per sentence, plain
// words, active voice, and a concrete example wherever possible.
export const InfoHint: React.FC<{ text: React.ReactNode; className?: string }> = ({ text, className }) => (
  <Tooltip content={<span>{text}</span>} position="auto">
    <span
      className={className ?? 'inline-flex items-center text-slate-400 hover:text-blue-300 cursor-help [&_svg]:w-3.5 [&_svg]:h-3.5'}
      aria-label="Help"
    >
      <Icons.Info />
    </span>
  </Tooltip>
);

// A label plus its info-icon hint, for use as the `label` prop on Input /
// Select (both accept ReactNode), or as plain text next to a Toggle/heading.
export const FieldLabel: React.FC<{ label: React.ReactNode; hint: React.ReactNode }> = ({ label, hint }) => (
  <span className="inline-flex items-center gap-1.5">
    {label}
    <InfoHint text={hint} />
  </span>
);
