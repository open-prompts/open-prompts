import React from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import RelatedRail from './RelatedRail';
import MoreFromAuthorRow from './MoreFromAuthorRow';
import { getRelatedTemplates, getMoreFromAuthor } from '../../services/api';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key, opts) => (opts ? `${key}:${JSON.stringify(opts)}` : key),
  }),
}));

jest.mock('../../services/api', () => ({
  getRelatedTemplates: jest.fn(),
  getMoreFromAuthor: jest.fn(),
}));

jest.mock('../PromptCard', () => () => <div data-testid="prompt-card" />);

const makeItem = (id, title, category, tags) => ({
  id,
  title,
  category,
  tags,
  visibility: 'VISIBILITY_PUBLIC',
  owner_id: 'owner-1',
  owner_display_name: 'Alice',
});

describe('RelatedRail', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders nothing while loading', () => {
    getRelatedTemplates.mockReturnValue(new Promise(() => {}));
    render(<RelatedRail templateId="t1" category="Cat" tags={['a']} />);
    expect(screen.queryByRole('region')).toBeNull();
  });

  test('renders nothing when fewer than 3 items', async () => {
    getRelatedTemplates.mockResolvedValue({ data: { items: [makeItem('a', 'A', 'Cat', ['a'])], basis: 'category' } });
    render(<RelatedRail templateId="t1" category="Cat" tags={['a']} />);
    await waitFor(() => expect(getRelatedTemplates).toHaveBeenCalled());
    expect(screen.queryByRole('region')).toBeNull();
  });

  test('renders cards and category basis label', async () => {
    getRelatedTemplates.mockResolvedValue({
      data: {
        items: [
          makeItem('a', 'A', 'Cat', ['a']),
          makeItem('b', 'B', 'Cat', ['a']),
          makeItem('c', 'C', 'Cat', ['a']),
        ],
        basis: 'category',
      },
    });
    render(<RelatedRail templateId="t1" category="Cat" tags={['a']} />);
    await waitFor(() => expect(screen.getAllByTestId('prompt-card')).toHaveLength(3));
    expect(screen.getByText(/related.title.category/)).toBeInTheDocument();
    expect(screen.getByLabelText('related.aria.rail')).toBeInTheDocument();
  });

  test('renders tag basis label with first shared tag', async () => {
    getRelatedTemplates.mockResolvedValue({
      data: {
        items: [
          makeItem('a', 'A', 'Other', ['writing']),
          makeItem('b', 'B', 'Other', ['x']),
          makeItem('c', 'C', 'Other', ['y']),
        ],
        basis: 'tag',
      },
    });
    render(<RelatedRail templateId="t1" category="Cat" tags={['writing', 'email']} />);
    await waitFor(() => expect(screen.getAllByTestId('prompt-card')).toHaveLength(3));
    expect(screen.getByText(/related.title.tag/)).toBeInTheDocument();
  });

  test('hides silently on error', async () => {
    getRelatedTemplates.mockRejectedValue(new Error('boom'));
    render(<RelatedRail templateId="t1" category="Cat" tags={['a']} />);
    await waitFor(() => expect(getRelatedTemplates).toHaveBeenCalled());
    expect(screen.queryByRole('region')).toBeNull();
  });
});

describe('MoreFromAuthorRow', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders nothing when empty', async () => {
    getMoreFromAuthor.mockResolvedValue({ data: { items: [] } });
    render(<MoreFromAuthorRow templateId="t1" authorName="Alice" />);
    await waitFor(() => expect(getMoreFromAuthor).toHaveBeenCalled());
    expect(screen.queryByRole('region')).toBeNull();
  });

  test('renders cards and author title', async () => {
    getMoreFromAuthor.mockResolvedValue({
      data: {
        items: [
          makeItem('a', 'A', 'Cat', ['a']),
          makeItem('b', 'B', 'Cat', ['a']),
        ],
      },
    });
    render(<MoreFromAuthorRow templateId="t1" authorName="Alice" />);
    await waitFor(() => expect(screen.getAllByTestId('prompt-card')).toHaveLength(2));
    expect(screen.getByText(/related.moreFromAuthor/)).toBeInTheDocument();
    expect(screen.getByLabelText('related.aria.authorRow')).toBeInTheDocument();
  });

  test('hides silently on error', async () => {
    getMoreFromAuthor.mockRejectedValue(new Error('boom'));
    render(<MoreFromAuthorRow templateId="t1" authorName="Alice" />);
    await waitFor(() => expect(getMoreFromAuthor).toHaveBeenCalled());
    expect(screen.queryByRole('region')).toBeNull();
  });
});