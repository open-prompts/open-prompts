import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import CreateTemplateModal from './CreateTemplateModal';
import * as api from '../services/api';

jest.mock('../context/NotificationContext', () => ({
  useNotification: () => ({ addNotification: jest.fn() }),
  NotificationProvider: ({ children }) => children,
}));


// Mock api service
jest.mock('../services/api');

// Mock react-i18next
jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key) => key,
  }),
}));

describe('CreateTemplateModal Component', () => {
  const mockOnRequestClose = jest.fn();
  const mockOnSuccess = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
    api.getCategories.mockResolvedValue({
      data: {
        categories: [
          { name: 'Coding', count: 10 },
          { name: 'Writing', count: 5 }
        ]
      }
    });
  });

  test('does not render when open is false', () => {
    render(
      <CreateTemplateModal
        open={false}
        onRequestClose={mockOnRequestClose}
      />
    );

    expect(screen.queryByText('create_template.title')).not.toBeInTheDocument();
  });

  test('renders form when open is true', async () => {
    render(
      <CreateTemplateModal
        open={true}
        onRequestClose={mockOnRequestClose}
      />
    );

    expect(screen.getByText('create_template.title')).toBeInTheDocument();
    expect(screen.getByText('create_template.label_title')).toBeInTheDocument();

    // Wait for categories to load
    await waitFor(() => expect(api.getCategories).toHaveBeenCalled());
    // Check if "Create new category..." option is present (implied by rendering logic, but good to check if easy)
    // Note: Checking options in Select might require interacting with it depending on implementation,
    // but at least we know the component rendered without crashing.
  });

  test('validates required fields', async () => {
    render(
      <CreateTemplateModal
        open={true}
        onRequestClose={mockOnRequestClose}
      />
    );

    // Click submit without filling required fields
    fireEvent.click(screen.getByText('common.create'));

    expect(screen.getByText('create_template.error_required_fields')).toBeInTheDocument();
    expect(api.createTemplate).not.toHaveBeenCalled();
  });

  test('validates custom category when "create_new" is selected', async () => {
    render(
      <CreateTemplateModal
        open={true}
        onRequestClose={mockOnRequestClose}
      />
    );

    await waitFor(() => expect(api.getCategories).toHaveBeenCalled());

    // Fill required fields
    fireEvent.change(screen.getByLabelText('create_template.label_title'), {
      target: { value: 'Test' }
    });
    fireEvent.change(screen.getByLabelText('create_template.label_content'), {
      target: { value: 'Content' }
    });

    // Select "Create new category..."
    fireEvent.change(screen.getByLabelText('create_template.label_category'), {
      target: { value: 'create_new' }
    });

    // Submit with empty custom category
    fireEvent.click(screen.getByText('common.create'));

    expect(screen.getByText('create_template.error_required_category')).toBeInTheDocument();
  });

  test('submits form with valid data', async () => {
    api.createTemplate.mockResolvedValue({});

    render(
      <CreateTemplateModal
        open={true}
        onRequestClose={mockOnRequestClose}
        onSuccess={mockOnSuccess}
      />
    );

    // Wait for categories to load
    await waitFor(() => expect(api.getCategories).toHaveBeenCalled());

    // Fill form
    fireEvent.change(screen.getByLabelText('create_template.label_title'), {
      target: { value: 'My Template' },
    });
    fireEvent.change(screen.getByLabelText('create_template.label_content'), {
      target: { value: 'Hello World' },
    });

    // Select Category (might be tricky with Carbon Select, simulating change on underlying select if possible or just assuming text input behavior for test simplicity if Carbon exposes native select)
    // Carbon Select usually hides the native select. We might need to mock or interact with Carbon specific structure.
    // For simplicity, let's just focus on Title and Content which are simple inputs.

    fireEvent.click(screen.getByText('common.create'));

    await waitFor(() => {
      expect(api.createTemplate).toHaveBeenCalledWith(expect.objectContaining({
        title: 'My Template',
        content: 'Hello World',
        type: 'user'
      }));
    });
    expect(mockOnSuccess).toHaveBeenCalled();
    expect(mockOnRequestClose).toHaveBeenCalled();
  });

  test('handles API errors', async () => {
    api.createTemplate.mockRejectedValue(new Error('Failed'));

    render(
      <CreateTemplateModal
        open={true}
        onRequestClose={mockOnRequestClose}
      />
    );

    fireEvent.change(screen.getByLabelText('create_template.label_title'), {
      target: { value: 'My Template' },
    });
    fireEvent.change(screen.getByLabelText('create_template.label_content'), {
      target: { value: 'Hello World' },
    });

    fireEvent.click(screen.getByText('common.create'));

    await waitFor(() => {
      expect(screen.getByText('create_template.error_submit')).toBeInTheDocument();
    });
  });
});
