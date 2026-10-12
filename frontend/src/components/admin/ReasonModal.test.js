import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import ReasonModal from './ReasonModal';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key, opts) => (opts && 'name' in opts ? `${key}:${opts.name}` : key),
  }),
}));

describe('ReasonModal', () => {
  const onConfirm = jest.fn();
  const onClose = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('renders ban title and all three ban consequences', () => {
    render(
      <ReasonModal open action="ban" name="alice" onConfirm={onConfirm} onClose={onClose} />
    );

    expect(screen.getByText('admin.reasonModal.ban_title:alice')).toBeInTheDocument();
    expect(screen.getByText('admin.reasonModal.consequence_ban_1')).toBeInTheDocument();
    expect(screen.getByText('admin.reasonModal.consequence_ban_2')).toBeInTheDocument();
    expect(screen.getByText('admin.reasonModal.consequence_ban_3')).toBeInTheDocument();
  });

  test('confirm button is disabled until reason has at least 3 characters', () => {
    render(
      <ReasonModal open action="ban" name="alice" onConfirm={onConfirm} onClose={onClose} />
    );

    const confirmBtn = screen.getByText('admin.reasonModal.confirm_ban').closest('button');
    expect(confirmBtn).toBeDisabled();

    fireEvent.change(screen.getByLabelText('admin.reasonModal.reason_label'), {
      target: { value: 'ab' },
    });
    expect(confirmBtn).toBeDisabled();

    fireEvent.change(screen.getByLabelText('admin.reasonModal.reason_label'), {
      target: { value: 'abc' },
    });
    expect(confirmBtn).not.toBeDisabled();
  });

  test('shows hint when reason is too short but non-empty', () => {
    render(
      <ReasonModal open action="unban" name="bob" onConfirm={onConfirm} onClose={onClose} />
    );

    fireEvent.change(screen.getByLabelText('admin.reasonModal.reason_label'), {
      target: { value: 'x' },
    });

    expect(screen.getByText('admin.reasonModal.reason_required')).toBeInTheDocument();
  });

  test('confirm passes the trimmed reason', () => {
    render(
      <ReasonModal open action="promote" name="carol" onConfirm={onConfirm} onClose={onClose} />
    );

    fireEvent.change(screen.getByLabelText('admin.reasonModal.reason_label'), {
      target: { value: '  trusted maintainer  ' },
    });
    fireEvent.click(screen.getByText('admin.reasonModal.confirm_promote'));

    expect(onConfirm).toHaveBeenCalledWith('trusted maintainer');
  });

  test('reason is reset when the modal reopens', () => {
    const { rerender } = render(
      <ReasonModal open action="ban" name="alice" onConfirm={onConfirm} onClose={onClose} />
    );

    fireEvent.change(screen.getByLabelText('admin.reasonModal.reason_label'), {
      target: { value: 'previous reason' },
    });

    rerender(
      <ReasonModal open={false} action="ban" name="alice" onConfirm={onConfirm} onClose={onClose} />
    );
    rerender(
      <ReasonModal open action="demote" name="alice" onConfirm={onConfirm} onClose={onClose} />
    );

    expect(screen.getByLabelText('admin.reasonModal.reason_label').value).toBe('');
  });

  test('confirm stays disabled while busy even with a valid reason', () => {
    render(
      <ReasonModal open action="ban" name="alice" onConfirm={onConfirm} onClose={onClose} busy />
    );

    const confirmBtn = screen.getByText('admin.reasonModal.confirm_ban').closest('button');
    expect(confirmBtn).toBeDisabled();
    expect(screen.getByLabelText('admin.reasonModal.reason_label')).toBeDisabled();
  });

  test('demote shows the shared role-change consequence', () => {
    render(
      <ReasonModal open action="demote" name="dave" onConfirm={onConfirm} onClose={onClose} />
    );

    expect(screen.getByText('admin.reasonModal.consequence_role_1')).toBeInTheDocument();
    expect(
      screen.queryByText('admin.reasonModal.consequence_ban_1')
    ).not.toBeInTheDocument();
  });
});
