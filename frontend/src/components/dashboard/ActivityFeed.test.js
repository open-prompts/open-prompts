import React from 'react';
import { render, screen } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import ActivityFeed from './ActivityFeed';

// Mock react-i18next
jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key, opts) => (opts && opts.title ? `${key}:${opts.title}` : key),
  }),
}));

// Mock formatRelativeTime to keep the test deterministic
jest.mock('../../utils/time', () => ({
  formatRelativeTime: () => '2h ago',
}));

const renderFeed = (items) =>
  render(
    <BrowserRouter>
      <ActivityFeed items={items} />
    </BrowserRouter>
  );

describe('ActivityFeed', () => {
  test('renders the empty state when items is empty', () => {
    renderFeed([]);
    expect(screen.getByText('dashboard_v2.activity.empty')).toBeInTheDocument();
  });

  test('renders the empty state when items is undefined', () => {
    renderFeed(undefined);
    expect(screen.getByText('dashboard_v2.activity.empty')).toBeInTheDocument();
  });

  test('renders activity items from snake_case API fields', () => {
    const items = [
      {
        type: 'DASHBOARD_ACTIVITY_TYPE_TEMPLATE',
        entity_id: 'tpl-1',
        entity_title: 'My Template',
        timestamp: '2026-10-20T10:00:00Z',
        template_id: '',
      },
      {
        type: 'DASHBOARD_ACTIVITY_TYPE_PROMPT',
        entity_id: 'prm-1',
        entity_title: 'Source Template',
        timestamp: '2026-10-20T09:00:00Z',
        template_id: 'tpl-2',
      },
    ];

    renderFeed(items);

    const list = screen.getByRole('list');
    expect(list).toHaveClass('activity-feed');
    expect(screen.getAllByRole('listitem').length).toBe(2);

    // Template item links to its own entity id.
    const templateLink = screen.getByText('dashboard_v2.activity.template_created:My Template');
    expect(templateLink.getAttribute('href')).toBe('/templates/tpl-1');

    // PROMPT item links to the source template id.
    const promptLink = screen.getByText('dashboard_v2.activity.prompt_used:Source Template');
    expect(promptLink.getAttribute('href')).toBe('/templates/tpl-2');
  });

  test('renders LIKE and FAVORITE activity types', () => {
    const items = [
      {
        type: 'DASHBOARD_ACTIVITY_TYPE_LIKE',
        entity_id: 'tpl-3',
        entity_title: 'Liked Template',
        timestamp: '2026-10-20T08:00:00Z',
        template_id: '',
      },
      {
        type: 'DASHBOARD_ACTIVITY_TYPE_FAVORITE',
        entity_id: 'tpl-4',
        entity_title: 'Favorited Template',
        timestamp: '2026-10-20T07:00:00Z',
        template_id: '',
      },
    ];

    renderFeed(items);

    expect(
      screen.getByText('dashboard_v2.activity.liked:Liked Template')
    ).toBeInTheDocument();
    expect(
      screen.getByText('dashboard_v2.activity.favorited:Favorited Template')
    ).toBeInTheDocument();
  });
});