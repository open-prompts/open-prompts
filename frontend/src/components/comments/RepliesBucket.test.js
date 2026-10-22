import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import RepliesBucket from './RepliesBucket';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key, opts) => (opts && opts.count !== undefined ? `${key}:${opts.count}` : key),
  }),
}));

jest.mock('../../services/api', () => ({
  listCommentReplies: jest.fn(),
}));

const api = require('../../services/api');

describe('RepliesBucket', () => {
  const baseProps = {
    commentId: 'parent-1',
    replyCount: 3,
    templateOwnerId: 'owner-1',
    user: { id: 'user-1' },
    expanded: false,
    cachedReplies: null,
    onToggle: jest.fn(),
    onCacheReplies: jest.fn(),
    onEdit: jest.fn(),
    onDelete: jest.fn(),
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders collapsed count button', () => {
    render(<RepliesBucket {...baseProps} />);
    expect(screen.getByRole('button', { name: 'comments.viewReplies:3' })).toBeInTheDocument();
  });

  test('renders singular label for 1 reply', () => {
    render(<RepliesBucket {...baseProps} replyCount={1} />);
    expect(screen.getByRole('button', { name: 'comments.viewOneReply' })).toBeInTheDocument();
  });

  test('lazily fetches replies on first expand and caches them', async () => {
    api.listCommentReplies.mockResolvedValue({
      data: { replies: [{ id: 'r1', content: 'first' }] },
    });
    const { rerender } = render(<RepliesBucket {...baseProps} />);
    fireEvent.click(screen.getByRole('button', { name: 'comments.viewReplies:3' }));
    await waitFor(() => {
      expect(api.listCommentReplies).toHaveBeenCalledWith('parent-1');
    });
    await waitFor(() => {
      expect(baseProps.onCacheReplies).toHaveBeenCalledWith('parent-1', [{ id: 'r1', content: 'first' }]);
    });
    // Re-render expanded with cached replies → no refetch.
    rerender(<RepliesBucket {...baseProps} expanded cachedReplies={[{ id: 'r1', content: 'first' }]} />);
    expect(api.listCommentReplies).toHaveBeenCalledTimes(1);
  });

  test('shows inline error + retry on fetch failure', async () => {
    api.listCommentReplies.mockRejectedValue(new Error('network'));
    const { rerender } = render(<RepliesBucket {...baseProps} />);
    fireEvent.click(screen.getByRole('button', { name: 'comments.viewReplies:3' }));
    // Parent expands the bucket after the toggle.
    rerender(<RepliesBucket {...baseProps} expanded />);
    await waitFor(() => {
      expect(screen.getByText('comments.repliesError')).toBeInTheDocument();
    });
    expect(screen.getByRole('button', { name: 'admin.common.retry' })).toBeInTheDocument();
  });

  test('toggle calls onToggle', () => {
    render(<RepliesBucket {...baseProps} />);
    fireEvent.click(screen.getByRole('button', { name: 'comments.viewReplies:3' }));
    expect(baseProps.onToggle).toHaveBeenCalledWith('parent-1');
  });
});