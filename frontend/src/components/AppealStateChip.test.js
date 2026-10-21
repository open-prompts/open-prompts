import React from 'react';
import { render, screen } from '@testing-library/react';
import AppealStateChip from './AppealStateChip';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key) => key,
  }),
}));

describe('AppealStateChip', () => {
  test('renders open state', () => {
    render(<AppealStateChip state="open" />);
    expect(screen.getByText('appeal.state.open')).toBeInTheDocument();
  });

  test('renders under_review state', () => {
    render(<AppealStateChip state="under_review" />);
    expect(screen.getByText('appeal.state.under_review')).toBeInTheDocument();
  });

  test('renders resolved_reversed state', () => {
    render(<AppealStateChip state="resolved_reversed" />);
    expect(screen.getByText('appeal.state.resolved_reversed')).toBeInTheDocument();
  });

  test('renders superseded tag', () => {
    render(<AppealStateChip state="open" superseded />);
    expect(screen.getByText('appeal.superseded')).toBeInTheDocument();
  });
});