import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import RestoreConfirmModal from './RestoreConfirmModal';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key, opts) => (opts ? `${key}:${JSON.stringify(opts)}` : key),
  }),
}));

const version = { id: 1, version: 3, content: 'x', change_note: '' };

describe('RestoreConfirmModal', () => {
  test('renders nothing when closed', () => {
    const { container } = render(
      <RestoreConfirmModal open={false} version={version} nextVersion={5} inFlight={false} onCancel={() => {}} onConfirm={() => {}} />
    );
    expect(container.innerHTML).toBe('');
  });

  test('renders nothing without a version', () => {
    const { container } = render(
      <RestoreConfirmModal open version={null} nextVersion={5} inFlight={false} onCancel={() => {}} onConfirm={() => {}} />
    );
    expect(container.innerHTML).toBe('');
  });

  test('shows body with version and next version numbers', () => {
    render(
      <RestoreConfirmModal open version={version} nextVersion={5} inFlight={false} onCancel={() => {}} onConfirm={() => {}} />
    );
    expect(screen.getByText(/versioning.restore_body/)).toHaveTextContent('"version":3');
    expect(screen.getByText(/versioning.restore_body/)).toHaveTextContent('"next":5');
  });

  test('confirm button labeled with version and fires onConfirm', () => {
    const onConfirm = jest.fn();
    render(
      <RestoreConfirmModal open version={version} nextVersion={5} inFlight={false} onCancel={() => {}} onConfirm={onConfirm} />
    );
    const btn = screen.getByText('versioning.restore_confirm:{"version":3}');
    fireEvent.click(btn);
    expect(onConfirm).toHaveBeenCalled();
  });

  test('in-flight disables confirm', () => {
    render(
      <RestoreConfirmModal open version={version} nextVersion={5} inFlight onCancel={() => {}} onConfirm={() => {}} />
    );
    expect(screen.getByText('versioning.restore_confirm:{"version":3}')).toBeDisabled();
  });

  test('cancel fires onCancel', () => {
    const onCancel = jest.fn();
    render(
      <RestoreConfirmModal open version={version} nextVersion={5} inFlight={false} onCancel={onCancel} onConfirm={() => {}} />
    );
    fireEvent.click(screen.getByText('common.cancel'));
    expect(onCancel).toHaveBeenCalled();
  });
});
