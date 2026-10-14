import React from 'react';
import { render, waitFor, act } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import useTemplateFilters from '../hooks/useTemplateFilters';
import * as api from '../services/api';

// Mock the api service
jest.mock('../services/api', () => ({
  ...jest.requireActual('../services/api'),
  getTemplates: jest.fn(),
}));

// Test harness that exposes the hook's return value.
let hookResult = null;
const Harness = ({ pageSize }) => {
  hookResult = useTemplateFilters({ pageSize, isAuthenticated: false });
  return null;
};

const renderWithRoute = (initialEntry = '/') =>
  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Harness pageSize={5} />
    </MemoryRouter>
  );

const lastCallParams = () => {
  const calls = api.getTemplates.mock.calls;
  return calls[calls.length - 1][0];
};

describe('useTemplateFilters', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    hookResult = null;
    api.getTemplates.mockResolvedValue({
      data: {
        templates: [{ id: 't1' }],
        private_templates: [],
        next_page_token: '',
        total_count: 1,
        private_total_count: 0,
      },
    });
  });

  test('fetches first page with default filters', async () => {
    renderWithRoute();
    await waitFor(() => expect(api.getTemplates).toHaveBeenCalled());
    const params = api.getTemplates.mock.calls[0][0];
    expect(params.page_size).toBe(5);
    expect(params.sort).toBe('newest');
    expect(params.visibility).toBe('VISIBILITY_PUBLIC');
    await waitFor(() => expect(hookResult.totalCount).toBe(1));
  });

  test('restores filters from URL on mount', async () => {
    renderWithRoute('/?q=hello&category=Writing&tags=a,b&sort=trending');
    await waitFor(() => expect(api.getTemplates).toHaveBeenCalled());
    const params = api.getTemplates.mock.calls[0][0];
    expect(params.keyword).toBe('hello');
    expect(params.category).toBe('Writing');
    expect(params.tags).toEqual(['a', 'b']);
    expect(params.sort).toBe('trending');
  });

  test('invalid sort in URL falls back to newest', async () => {
    renderWithRoute('/?sort=bogus');
    await waitFor(() => expect(api.getTemplates).toHaveBeenCalled());
    expect(api.getTemplates.mock.calls[0][0].sort).toBe('newest');
  });

  test('setKeyword is debounced by 300ms', async () => {
    jest.useFakeTimers();
    try {
      renderWithRoute();
      await waitFor(() => expect(api.getTemplates).toHaveBeenCalled());
      const callsBefore = api.getTemplates.mock.calls.length;

      act(() => {
        hookResult.setKeyword('abc');
      });
      // Not yet fired (debounced).
      expect(api.getTemplates.mock.calls.length).toBe(callsBefore);

      act(() => {
        jest.advanceTimersByTime(310);
      });
      await waitFor(() =>
        expect(api.getTemplates.mock.calls.length).toBeGreaterThan(callsBefore)
      );
      expect(lastCallParams().keyword).toBe('abc');
    } finally {
      jest.useRealTimers();
    }
  });

  test('submitKeyword bypasses the debounce', async () => {
    renderWithRoute();
    await waitFor(() => expect(api.getTemplates).toHaveBeenCalled());
    const callsBefore = api.getTemplates.mock.calls.length;

    act(() => {
      hookResult.submitKeyword('immediate');
    });
    await waitFor(() =>
      expect(api.getTemplates.mock.calls.length).toBeGreaterThan(callsBefore)
    );
    expect(lastCallParams().keyword).toBe('immediate');
  });

  test('toggleTag adds and removes tags', async () => {
    renderWithRoute();
    await waitFor(() => expect(api.getTemplates).toHaveBeenCalled());

    act(() => {
      hookResult.toggleTag('go');
    });
    await waitFor(() => expect(lastCallParams().tags).toEqual(['go']));

    act(() => {
      hookResult.toggleTag('go');
    });
    await waitFor(() => expect(lastCallParams().tags).toBeUndefined());
  });

  test('stale responses are discarded (race guard)', async () => {
    let resolveFirst;
    api.getTemplates.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveFirst = resolve;
        })
    );
    renderWithRoute();

    // Fire a second request while the first is pending.
    act(() => {
      hookResult.submitKeyword('second');
    });
    await waitFor(() => expect(api.getTemplates.mock.calls.length).toBe(2));

    // Resolve the stale first request; its data must be discarded.
    await act(async () => {
      resolveFirst({
        data: { templates: [{ id: 'STALE' }], private_templates: [], next_page_token: '' },
      });
    });
    await waitFor(() => expect(hookResult.templates).toEqual([{ id: 't1' }]));
    expect(hookResult.templates).not.toContainEqual({ id: 'STALE' });
  });

  test('error keeps last successful results and exposes retry', async () => {
    renderWithRoute();
    await waitFor(() => expect(hookResult.templates).toEqual([{ id: 't1' }]));

    api.getTemplates.mockRejectedValueOnce(new Error('network down'));
    act(() => {
      hookResult.submitKeyword('fail');
    });
    await waitFor(() => expect(hookResult.error).not.toBeNull());
    // Last successful results stay rendered.
    expect(hookResult.templates).toEqual([{ id: 't1' }]);

    api.getTemplates.mockResolvedValueOnce({
      data: { templates: [{ id: 't2' }], private_templates: [], next_page_token: '' },
    });
    act(() => {
      hookResult.retry();
    });
    await waitFor(() => expect(hookResult.templates).toEqual([{ id: 't2' }]));
  });

  test('clearAll resets keyword/category/tags/sort', async () => {
    renderWithRoute('/?q=hello&category=Writing&tags=a&sort=trending');
    await waitFor(() => expect(api.getTemplates).toHaveBeenCalled());

    act(() => {
      hookResult.clearAll();
    });
    await waitFor(() => expect(lastCallParams().keyword).toBeUndefined());
    const params = lastCallParams();
    expect(params.category).toBeUndefined();
    expect(params.tags).toBeUndefined();
    expect(params.sort).toBe('newest');
  });

  test('loadMore paginates with the stored token', async () => {
    api.getTemplates.mockResolvedValueOnce({
      data: {
        templates: [{ id: 't1' }],
        private_templates: [],
        next_page_token: '5',
        total_count: 10,
      },
    });
    renderWithRoute();
    await waitFor(() => expect(hookResult.hasMore).toBe(true));
    await waitFor(() => expect(hookResult.isFirstLoading).toBe(false));

    act(() => {
      hookResult.loadMore();
    });
    await waitFor(() => expect(api.getTemplates.mock.calls.length).toBe(2));
    expect(api.getTemplates.mock.calls[1][0].page_token).toBe('5');
  });

  test('mixed view combines public and private tokens', async () => {
    api.getTemplates.mockResolvedValueOnce({
      data: {
        templates: [{ id: 'pub' }],
        private_templates: [{ id: 'priv' }],
        next_page_token: '5',
        private_next_page_token: '5',
        total_count: 5,
        private_total_count: 5,
      },
    });
    renderWithRoute();
    await waitFor(() => expect(hookResult.hasMore).toBe(true));
    await waitFor(() => expect(hookResult.isFirstLoading).toBe(false));
    expect(hookResult.totalCount).toBe(10);

    act(() => {
      hookResult.loadMore();
    });
    await waitFor(() => expect(api.getTemplates.mock.calls.length).toBe(2));
    expect(api.getTemplates.mock.calls[1][0].page_token).toBe('5:5');
  });
});
