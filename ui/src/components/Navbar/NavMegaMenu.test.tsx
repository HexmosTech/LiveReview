import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import { NavMegaMenu } from './NavMegaMenu';
import { MegaMenuSection } from './megaMenuData';

const sections: MegaMenuSection[] = [
  {
    key: 'reviews',
    name: 'Reviews',
    icon: <span />,
    path: '/reviews',
    items: [
      { kind: 'link', name: 'All Reviews', icon: <span />, path: '/reviews/all' },
      {
        kind: 'group',
        name: 'Settings',
        icon: <span />,
        path: '/reviews/settings',
        children: [
          { kind: 'link', name: 'Policies', icon: <span />, path: '/reviews/settings/policies' },
        ],
      },
      {
        kind: 'group',
        name: 'Findings',
        icon: <span />,
        children: [
          { kind: 'link', name: 'All Findings', icon: <span />, path: '/reports?mode=explore' },
        ],
      },
    ],
  },
];

describe('NavMegaMenu links', () => {
  it('renders destinations as real anchors so they can open in a new tab', () => {
    render(
      <MemoryRouter>
        <NavMegaMenu isOpen onClose={() => {}} sections={sections} />
      </MemoryRouter>
    );

    const section = screen.getByRole('link', { name: 'Reviews' });
    expect(section.tagName).toBe('A');
    expect(section).toHaveAttribute('href', '/reviews');

    const link = screen.getByRole('link', { name: /All Reviews/ });
    expect(link.tagName).toBe('A');
    expect(link).toHaveAttribute('href', '/reviews/all');

    const group = screen.getByRole('link', { name: /Settings/ });
    expect(group.tagName).toBe('A');
    expect(group).toHaveAttribute('href', '/reviews/settings');

    // A group with no landing page of its own still renders an anchor, using
    // its first navigable child, so right-click → open-in-new-tab works.
    const fallbackGroup = screen.getByRole('link', { name: /Findings/ });
    expect(fallbackGroup.tagName).toBe('A');
    expect(fallbackGroup).toHaveAttribute('href', '/reports?mode=explore');
  });
});
