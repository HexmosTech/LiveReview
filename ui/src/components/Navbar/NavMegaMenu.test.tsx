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
  });
});
