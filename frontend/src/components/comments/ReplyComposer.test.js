import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import ReplyComposer from './ReplyComposer';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key, opts) => (opts && opts.count !== undefined ? `${key}:${opts.count}` : key),
  }),
}));

jest.mock('../../context/NotificationContext', () => ({
  useNotification: () => ({ addNotification: jest.fn() }),
}));

jest.mock('../../services/api', () => ({
  createTemplateComment: jest.fn(),
}));

const api = require('../../services/api');

describe('ReplyComposer', () => {
  const baseProps = {
    templateId: 'tpl-1',
    parentCommentId: 'parent-1',
    parentName: 'Alice',
    onPosted: jest.fn(),
    onCancel: jest.fn(),
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders labeled textarea and Reply/Cancel buttons', () => {
    render(<ReplyComposer {...baseProps} />);
    expect(screen.getByLabelText('comments.replyTo')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'comments.reply' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'comments.cancel' })).toBeInTheDocument();
  });

  test('submit is disabled for empty draft', () => {
    render(<ReplyComposer {...baseProps} />);
    expect(screen.getByRole('button', { name: 'comments.reply' })).toBeDisabled();
  });

  test('posts with parent_comment_id and calls onPosted', async () => {
    api.createTemplateComment.mockResolvedValue({ data: { id: 'r1', content: 'hello' } });
    render(<ReplyComposer {...baseProps} />);
    fireEvent.change(screen.getByLabelText('comments.replyTo'), {
      target: { value: 'hello' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'comments.reply' }));
    await waitFor(() => {
      expect(api.createTemplateComment).toHaveBeenCalledWith('tpl-1', 'hello', 'parent-1');
    });
    await waitFor(() => {
      expect(baseProps.onPosted).toHaveBeenCalledWith({ id: 'r1', content: 'hello' });
    });
  });

  test('shows threadFull error on 400 and preserves draft', async () => {
    api.createTemplateComment.mockRejectedValue({ response: { status: 400 } });
    render(<ReplyComposer {...baseProps} />);
    const textarea = screen.getByLabelText('comments.replyTo');
    fireEvent.change(textarea, { target: { value: 'hello' } });
    fireEvent.click(screen.getByRole('button', { name: 'comments.reply' }));
    await waitFor(() => {
      expect(screen.getByText('comments.threadFull')).toBeInTheDocument();
    });
    expect(textarea.value).toBe('hello');
  });

  test('cancel calls onCancel', () => {
    render(<ReplyComposer {...baseProps} />);
    fireEvent.click(screen.getByRole('button', { name: 'comments.cancel' }));
    expect(baseProps.onCancel).toHaveBeenCalled();
  });
});