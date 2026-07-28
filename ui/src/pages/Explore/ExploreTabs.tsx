import React from 'react';
import { Link } from 'react-router-dom';

interface ExploreTabsProps {
  active: 'repositories' | 'merge-requests';
}

const tabs: { key: ExploreTabsProps['active']; label: string; path: string }[] = [
  { key: 'repositories', label: 'Repositories', path: '/explore/repositories' },
  { key: 'merge-requests', label: 'Merge Requests', path: '/explore/merge-requests' },
];

const ExploreTabs: React.FC<ExploreTabsProps> = ({ active }) => {
  return (
    <div className="border-b border-slate-700 mb-6">
      <nav className="flex space-x-6" aria-label="Explore sections">
        {tabs.map((tab) => (
          <Link
            key={tab.key}
            to={tab.path}
            className={`py-3 px-1 border-b-2 text-sm font-medium transition-colors ${
              active === tab.key
                ? 'border-blue-500 text-white'
                : 'border-transparent text-slate-400 hover:text-slate-200 hover:border-slate-600'
            }`}
          >
            {tab.label}
          </Link>
        ))}
      </nav>
    </div>
  );
};

export default ExploreTabs;
