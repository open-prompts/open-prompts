import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import AppealFormModal from './AppealFormModal';
import * as api from '../services/api';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key) => key,
  }),
}));

jest.mock('../services/api');

const action = {
  id: 'act1',
  action_type: 'soft_delete',
  reason: 'spam',
  actor_display_name: 'admin@example.com',
  template_title: 'My Template',
};

describe('AppealFormModal', () => {
  const onClose = jest.fn();
  const onSuccess = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('does not render when closed', () => {
    render(<AppealFormModal open={false} action={action} onClose={onClose} onSuccess={onSuccess} />);
    expect(screen.queryByText('appeal.form.title')).not.toBeInTheDocument();
  });

  test('renders form with action context', () => {
    render(<AppealFormModal open action={action} onClose={onClose} onSuccess={onSuccess} />);
    expect(screen.getByText('appeal.form.title')).toBeInTheDocument();
    expect(screen.getByText('appeal.form.statement')).toBeInTheDocument();
  });

  test('submit disabled when statement too short', () => {
    render(<AppealFormModal open action={action} onClose={onClose} onSuccess={onSuccess} />);
    const textarea = screen.getByLabelText('appeal.form.statement');
    fireEvent.change(textarea, { target: { value: 'short' } });
    expect(screen.getByText('appeal.form.tooShort')).toBeInTheDocument();
  });

  test('submits appeal on valid statement', async () => {
    api.createAppeal.mockResolvedValue({ data: { id: 'appeal1', state: 'open' } });
    render(<AppealFormModal open action={action} onClose={onClose} onSuccess={onSuccess} />);
    const statement = 'This is a valid statement that is long enough to pass the fifty character minimum requirement for an appeal.';
    const textarea = screen.getByLabelText('appeal.form.statement');
    fireEvent.change(textarea, { target: { value: statement } });
    fireEvent.click(screen.getByText('appeal.form.submit'));
    await waitFor(() => expect(api.createAppeal).toHaveBeenCalledWith({
      moderation_action_id: 'act1',
      statement,
    }));
    expect(onSuccess).toHaveBeenCalledWith({ id: 'appeal1', state: 'open' });
  });

  test('shows conflict card on 409', async () => {
    api.createAppeal.mockRejectedValue({ response: { status: 409 } });
    render(<AppealFormModal open action={action} onClose={onClose} onSuccess={onSuccess} />);
    const statement = 'This is a valid statement that is long enough to pass the fifty character minimum requirement for an appeal.';
    fireEvent.change(screen.getByLabelText('appeal.form.statement'), { target: { value: statement } });
    fireEvent.click(screen.getByText('appeal.form.submit'));
    expect(await screen.findByText('appeal.form.conflict')).toBeInTheDocument();
  });
});