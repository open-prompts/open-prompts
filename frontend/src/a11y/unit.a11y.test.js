import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { Provider } from 'react-redux';
import { configureStore } from '@reduxjs/toolkit';
import authReducer from '../store/authSlice';
import Modal from '../components/ui/Modal';
import { usePageAnnouncement } from '../utils/a11y';

// Mock react-i18next so translations resolve to their keys.
jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key) => key,
    i18n: { language: 'en' },
  }),
}));

const renderWithRedux = (component) => {
  const store = configureStore({ reducer: { auth: authReducer } });
  return render(<Provider store={store}>{component}</Provider>);
};

describe('Modal focus trap and restore', () => {
  test('focuses the dialog on open', async () => {
    renderWithRedux(
      <Modal open title="Test modal" onClose={() => {}}>
        <button type="button">Inside</button>
      </Modal>
    );
    const dialog = screen.getByRole('dialog');
    await waitFor(() => expect(dialog).toHaveFocus());
  });

  test('wraps Tab from last to first focusable element', async () => {
    renderWithRedux(
      <Modal open title="Test modal" onClose={() => {}}>
        <button type="button">First</button>
        <button type="button">Last</button>
      </Modal>
    );
    const dialog = screen.getByRole('dialog');
    const closeBtn = screen.getByLabelText('common.close');
    const last = screen.getByText('Last');
    await waitFor(() => expect(dialog).toHaveFocus());
    last.focus();
    fireEvent.keyDown(document, { key: 'Tab' });
    expect(closeBtn).toHaveFocus();
  });

  test('wraps Shift+Tab from first to last focusable element', async () => {
    renderWithRedux(
      <Modal open title="Test modal" onClose={() => {}}>
        <button type="button">First</button>
        <button type="button">Last</button>
      </Modal>
    );
    const dialog = screen.getByRole('dialog');
    const closeBtn = screen.getByLabelText('common.close');
    const last = screen.getByText('Last');
    await waitFor(() => expect(dialog).toHaveFocus());
    closeBtn.focus();
    fireEvent.keyDown(document, { key: 'Tab', shiftKey: true });
    expect(last).toHaveFocus();
  });

  test('restores focus to the trigger on close', async () => {
    const trigger = document.createElement('button');
    trigger.textContent = 'Trigger';
    document.body.appendChild(trigger);
    trigger.focus();

    const { rerender } = renderWithRedux(
      <Modal open title="Test modal" onClose={() => {}}>
        <button type="button">Inside</button>
      </Modal>
    );
    await waitFor(() => expect(screen.getByRole('dialog')).toHaveFocus());

    rerender(
      <Provider store={configureStore({ reducer: { auth: authReducer } })}>
        <Modal open={false} title="Test modal" onClose={() => {}}>
          <button type="button">Inside</button>
        </Modal>
      </Provider>
    );
    await waitFor(() => expect(trigger).toHaveFocus());
    document.body.removeChild(trigger);
  });

  test('Escape calls onClose', () => {
    const onClose = jest.fn();
    renderWithRedux(
      <Modal open title="Test modal" onClose={onClose}>
        <button type="button">Inside</button>
      </Modal>
    );
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' });
    expect(onClose).toHaveBeenCalled();
  });
});

describe('usePageAnnouncement', () => {
  const Page = () => {
    usePageAnnouncement('home.title');
    return (
      <main>
        <h1>Home</h1>
      </main>
    );
  };

  test('sets document.title and focuses the h1 on pathname change', async () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <Routes>
          <Route path="/" element={<Page />} />
        </Routes>
      </MemoryRouter>
    );
    expect(document.title).toBe('home.title');
    const h1 = screen.getByRole('heading', { level: 1 });
    await waitFor(() => expect(h1).toHaveFocus());
  });
});