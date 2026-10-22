import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import ReportModal from './ReportModal';
import { createReport } from '../services/api';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key) => key,
  }),
}));

jest.mock('../services/api', () => ({
  createReport: jest.fn(),
}));

describe('ReportModal', () => {
  test('renders title and reason options', () => {
    render(<ReportModal open itemType="template" itemId="t1" onClose={() => {}} onSuccess={() => {}} />);
    expect(screen.getByText('reports.modal.title.template')).toBeInTheDocument();
    expect(screen.getByText('reports.reason.spam')).toBeInTheDocument();
    expect(screen.getByText('reports.reason.offensive')).toBeInTheDocument();
    expect(screen.getByText('reports.reason.misleading')).toBeInTheDocument();
    expect(screen.getByText('reports.reason.privacy')).toBeInTheDocument();
    expect(screen.getByText('reports.reason.other')).toBeInTheDocument();
  });

  test('renders comment title for comments', () => {
    render(<ReportModal open itemType="comment" itemId="c1" onClose={() => {}} onSuccess={() => {}} />);
    expect(screen.getByText('reports.modal.title.comment')).toBeInTheDocument();
  });

  test('submit is disabled until a reason is chosen', () => {
    render(<ReportModal open itemType="template" itemId="t1" onClose={() => {}} onSuccess={() => {}} />);
    const submit = screen.getByRole('button', { name: 'reports.modal.submit' });
    expect(submit).toBeDisabled();
    fireEvent.click(screen.getByText('reports.reason.spam'));
    expect(submit).not.toBeDisabled();
  });

  test('submits the report with reason and note', async () => {
    createReport.mockResolvedValue({ data: { id: 'r1', reason: 'spam' } });
    const onSuccess = jest.fn();
    const onClose = jest.fn();
    render(<ReportModal open itemType="template" itemId="t1" onClose={onClose} onSuccess={onSuccess} />);
    fireEvent.click(screen.getByText('reports.reason.spam'));
    fireEvent.change(screen.getByLabelText('reports.modal.noteLabel'), { target: { value: 'a note' } });
    fireEvent.click(screen.getByText('reports.modal.submit'));
    await waitFor(() => {
      expect(createReport).toHaveBeenCalledWith({
        item_type: 'template',
        item_id: 't1',
        reason: 'spam',
        note: 'a note',
      });
    });
    await waitFor(() => {
      expect(onSuccess).toHaveBeenCalled();
    });
    await waitFor(() => {
      expect(onClose).toHaveBeenCalled();
    });
  });

  test('shows rate-limit error on 429', async () => {
    createReport.mockRejectedValue({ response: { status: 429 } });
    render(<ReportModal open itemType="template" itemId="t1" onClose={() => {}} onSuccess={() => {}} />);
    fireEvent.click(screen.getByText('reports.reason.spam'));
    fireEvent.click(screen.getByText('reports.modal.submit'));
    await waitFor(() => {
      expect(screen.getByText('reports.rateLimited')).toBeInTheDocument();
    });
  });
});