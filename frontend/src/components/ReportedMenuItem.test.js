import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import ReportedMenuItem from './ReportedMenuItem';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key) => key,
  }),
}));

describe('ReportedMenuItem', () => {
  test('renders reported label and submenu actions', () => {
    render(<ReportedMenuItem onUpdate={() => {}} onWithdraw={() => {}} />);
    expect(screen.getByText('reports.reported')).toBeInTheDocument();
    expect(screen.getByText('reports.update')).toBeInTheDocument();
    expect(screen.getByText('reports.withdraw')).toBeInTheDocument();
  });

  test('calls onUpdate when Update is clicked', () => {
    const onUpdate = jest.fn();
    render(<ReportedMenuItem onUpdate={onUpdate} onWithdraw={() => {}} />);
    fireEvent.click(screen.getByText('reports.update'));
    expect(onUpdate).toHaveBeenCalled();
  });

  test('calls onWithdraw when Withdraw is clicked', () => {
    const onWithdraw = jest.fn();
    render(<ReportedMenuItem onUpdate={() => {}} onWithdraw={onWithdraw} />);
    fireEvent.click(screen.getByText('reports.withdraw'));
    expect(onWithdraw).toHaveBeenCalled();
  });
});