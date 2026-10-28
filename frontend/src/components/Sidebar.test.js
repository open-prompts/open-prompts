import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import { Provider } from 'react-redux';
import { configureStore } from '@reduxjs/toolkit';
import authReducer from '../store/authSlice';
import { NotificationProvider } from '../context/NotificationContext';
import Sidebar from './Sidebar';

jest.mock('../services/api', () => ({
  getCategories: jest.fn(() => Promise.resolve({ data: { categories: [
    { name: 'Coding', count: 3 },
    { name: 'Writing', count: 2 },
  ] } })),
  getTags: jest.fn(() => Promise.resolve({ data: { tags: [{ name: 'python', count: 2 }] } })),
}));

const LocationProbe = () => {
  const location = useLocation();
  return <div data-testid="location">{location.pathname + location.search}</div>;
};

const renderSidebar = (props = {}, initialEntry = '/dashboard') => {
  const store = configureStore({
    reducer: { auth: authReducer },
    preloadedState: { auth: { isAuthenticated: false, user: null } },
  });
  return render(
    <Provider store={store}>
      <NotificationProvider>
        <MemoryRouter initialEntries={[initialEntry]}>
          <Routes>
            <Route
              path="*"
              element={(
                <>
                  <LocationProbe />
                  <Sidebar
                    categories={[
                      { name: 'Coding', count: 3 },
                      { name: 'Writing', count: 2 },
                    ]}
                    availableTags={[{ name: 'python', count: 2 }]}
                    {...props}
                  />
                </>
              )}
            />
          </Routes>
        </MemoryRouter>
      </NotificationProvider>
    </Provider>
  );
};

test('category click calls onFilterChange when a handler is provided', () => {
  const onFilterChange = jest.fn();
  renderSidebar({ onFilterChange });

  fireEvent.click(screen.getByText(/Coding/));
  expect(onFilterChange).toHaveBeenCalledWith(
    expect.objectContaining({ category: 'Coding', visibility: 'VISIBILITY_PUBLIC', tags: [] })
  );
});

test('tag click calls onFilterChange with the toggled tag', () => {
  const onFilterChange = jest.fn();
  renderSidebar({ onFilterChange, currentFilters: { tags: [] } });

  fireEvent.click(screen.getByLabelText('python'));
  expect(onFilterChange).toHaveBeenCalledWith(
    expect.objectContaining({ tags: ['python'] })
  );
});

test('renders a tag checkbox for each tag', () => {
  renderSidebar({ onFilterChange: jest.fn() });
  expect(screen.getByLabelText('python')).toBeInTheDocument();
});

test('without onFilterChange a category click navigates back to the browse page with the filter', () => {
  renderSidebar({}, '/dashboard');

  fireEvent.click(screen.getByText(/Coding/));
  expect(screen.getByTestId('location').textContent).toBe(
    '/?category=Coding&visibility=VISIBILITY_PUBLIC'
  );
});

test('without onFilterChange a tag click navigates back to the browse page with the tag', () => {
  renderSidebar({ currentFilters: { tags: [] } }, '/analytics');

  fireEvent.click(screen.getByLabelText('python'));
  expect(screen.getByTestId('location').textContent).toBe('/?tags=python');
});
