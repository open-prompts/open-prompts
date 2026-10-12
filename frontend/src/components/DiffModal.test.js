import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import DiffModal from './DiffModal';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key, opts) => (opts ? `${key}:${JSON.stringify(opts)}` : key),
    i18n: { language: 'en' },
  }),
}));

const mkVersion = (id, version, content) => ({
  id,
  version,
  content,
  change_note: '',
  created_at: new Date().toISOString(),
});

const versions = [
  mkVersion(2, 2, 'a\nchanged\nc'),
  mkVersion(1, 1, 'a\nb\nc'),
];

describe('DiffModal', () => {
  test('renders nothing when closed', () => {
    const { container } = render(
      <DiffModal open={false} versions={versions} initialAId={1} initialBId={2} onClose={() => {}} />
    );
    expect(container.innerHTML).toBe('');
  });

  test('renders unified diff with stats', () => {
    render(
      <DiffModal open versions={versions} initialAId={1} initialBId={2} onClose={() => {}} />
    );
    expect(screen.getByTestId('diff-stats')).toHaveTextContent('versioning.stats');
    // removed line b, added line changed
    expect(screen.getByText('b')).toBeInTheDocument();
    expect(screen.getByText('changed')).toBeInTheDocument();
  });

  test('normalizes A=older B=newer regardless of initial order', () => {
    render(
      <DiffModal open versions={versions} initialAId={2} initialBId={1} onClose={() => {}} />
    );
    // Same diff either way: one remove + one add
    expect(screen.getByText('b')).toBeInTheDocument();
    expect(screen.getByText('changed')).toBeInTheDocument();
  });

  test('identical versions produce zero stats', () => {
    render(
      <DiffModal open versions={versions} initialAId={1} initialBId={1} onClose={() => {}} />
    );
    expect(screen.getByTestId('diff-stats')).toHaveTextContent('"added":0');
    expect(screen.getByTestId('diff-stats')).toHaveTextContent('"removed":0');
  });

  test('swap button flips selectors', () => {
    render(
      <DiffModal open versions={versions} initialAId={1} initialBId={2} onClose={() => {}} />
    );
    fireEvent.click(screen.getByLabelText('versioning.swap'));
    // After swap the diff is recomputed (normalized) — still renders
    expect(screen.getByText('changed')).toBeInTheDocument();
  });

  test('close button calls onClose', () => {
    const onClose = jest.fn();
    render(
      <DiffModal open versions={versions} initialAId={1} initialBId={2} onClose={onClose} />
    );
    fireEvent.click(screen.getAllByText('common.close')[0]);
    expect(onClose).toHaveBeenCalled();
  });
});
