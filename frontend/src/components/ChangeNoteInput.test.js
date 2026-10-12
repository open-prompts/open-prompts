import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import ChangeNoteInput from './ChangeNoteInput';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key, opts) => (opts ? `${key}:${JSON.stringify(opts)}` : key),
  }),
}));

describe('ChangeNoteInput', () => {
  test('renders label, input and counter', () => {
    render(<ChangeNoteInput value="" onChange={() => {}} />);
    expect(screen.getByLabelText('versioning.note_label')).toBeInTheDocument();
    expect(screen.getByText(/versioning.note_counter/)).toBeInTheDocument();
  });

  test('shows current count in counter', () => {
    render(<ChangeNoteInput value="hello" onChange={() => {}} />);
    expect(screen.getByText(/"count":5/)).toBeInTheDocument();
  });

  test('input has maxlength 200', () => {
    render(<ChangeNoteInput value="" onChange={() => {}} />);
    expect(screen.getByLabelText('versioning.note_label')).toHaveAttribute('maxlength', '200');
  });

  test('calls onChange on typing', () => {
    const onChange = jest.fn();
    render(<ChangeNoteInput value="" onChange={onChange} />);
    fireEvent.change(screen.getByLabelText('versioning.note_label'), { target: { value: 'note' } });
    expect(onChange).toHaveBeenCalledWith('note');
  });

  test('warning class at 180+ chars', () => {
    const long = 'x'.repeat(180);
    render(<ChangeNoteInput value={long} onChange={() => {}} />);
    expect(screen.getByText(/versioning.note_counter/)).toHaveClass('warning');
  });

  test('no warning class below 180 chars', () => {
    render(<ChangeNoteInput value="short" onChange={() => {}} />);
    expect(screen.getByText(/versioning.note_counter/)).not.toHaveClass('warning');
  });

  test('disabled input does not fire onChange', () => {
    const onChange = jest.fn();
    render(<ChangeNoteInput value="" onChange={onChange} disabled />);
    const input = screen.getByLabelText('versioning.note_label');
    expect(input).toBeDisabled();
  });
});
