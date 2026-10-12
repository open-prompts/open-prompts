import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import VersionHistoryPanel from './VersionHistoryPanel';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key, opts) => (opts ? `${key}:${JSON.stringify(opts)}` : key),
  }),
}));

const baseVersion = (over = {}) => ({
  id: 1,
  version: 1,
  content: 'line1\nline2',
  change_note: '',
  prompt_count: 0,
  created_at: new Date().toISOString(),
  ...over,
});

const versions = [
  baseVersion({ id: 2, version: 2, change_note: 'second revision', prompt_count: 3 }),
  baseVersion({ id: 1, version: 1 }),
];

describe('VersionHistoryPanel', () => {
  const noop = () => {};

  test('renders rows newest-first with notes and meta', () => {
    render(
      <VersionHistoryPanel
        versions={versions}
        loading={false}
        error={false}
        selectedVersionId={2}
        isOwner
        hasMore={false}
        onLoadMore={noop}
        onSelect={noop}
        onCompare={noop}
        onRestore={noop}
        onRetry={noop}
        onClose={noop}
      />
    );
    expect(screen.getByText('v2')).toBeInTheDocument();
    expect(screen.getByText('v1')).toBeInTheDocument();
    expect(screen.getByText('second revision')).toBeInTheDocument();
    expect(screen.getByText('versioning.latest')).toBeInTheDocument();
    expect(screen.getAllByText(/versioning.meta/).length).toBe(2);
  });

  test('shows "No note" for noteless versions', () => {
    render(
      <VersionHistoryPanel
        versions={versions}
        loading={false}
        error={false}
        selectedVersionId={1}
        isOwner={false}
        hasMore={false}
        onLoadMore={noop}
        onSelect={noop}
        onCompare={noop}
        onRestore={noop}
        onRetry={noop}
        onClose={noop}
      />
    );
    expect(screen.getByText('versioning.no_note')).toBeInTheDocument();
  });

  test('hides usage segment when prompt_count absent', () => {
    const noCount = versions.map((v) => {
      const { prompt_count, ...rest } = v;
      return rest;
    });
    render(
      <VersionHistoryPanel
        versions={noCount}
        loading={false}
        error={false}
        selectedVersionId={1}
        isOwner={false}
        hasMore={false}
        onLoadMore={noop}
        onSelect={noop}
        onCompare={noop}
        onRestore={noop}
        onRetry={noop}
        onClose={noop}
      />
    );
    expect(screen.getAllByText(/versioning.meta_no_usage/).length).toBe(2);
    expect(screen.queryByText(/versioning.meta"/)).not.toBeInTheDocument();
  });

  test('row click calls onSelect with the version', () => {
    const onSelect = jest.fn();
    render(
      <VersionHistoryPanel
        versions={versions}
        loading={false}
        error={false}
        selectedVersionId={2}
        isOwner={false}
        hasMore={false}
        onLoadMore={noop}
        onSelect={onSelect}
        onCompare={noop}
        onRestore={noop}
        onRetry={noop}
        onClose={noop}
      />
    );
    fireEvent.click(screen.getByText('v1'));
    expect(onSelect).toHaveBeenCalledWith(versions[1]);
  });

  test('owner sees Restore on non-latest rows only', () => {
    render(
      <VersionHistoryPanel
        versions={versions}
        loading={false}
        error={false}
        selectedVersionId={2}
        isOwner
        hasMore={false}
        onLoadMore={noop}
        onSelect={noop}
        onCompare={noop}
        onRestore={noop}
        onRetry={noop}
        onClose={noop}
      />
    );
    // v1 (non-latest) has a Restore action
    expect(screen.getByLabelText('versioning.restore v1')).toBeInTheDocument();
    // v2 (latest) does not
    expect(screen.queryByLabelText('versioning.restore v2')).not.toBeInTheDocument();
  });

  test('non-owner sees no Restore actions', () => {
    render(
      <VersionHistoryPanel
        versions={versions}
        loading={false}
        error={false}
        selectedVersionId={2}
        isOwner={false}
        hasMore={false}
        onLoadMore={noop}
        onSelect={noop}
        onCompare={noop}
        onRestore={noop}
        onRetry={noop}
        onClose={noop}
      />
    );
    expect(screen.queryByLabelText(/versioning.restore/)).not.toBeInTheDocument();  });

  test('Compare action fires with the version', () => {
    const onCompare = jest.fn();
    render(
      <VersionHistoryPanel
        versions={versions}
        loading={false}
        error={false}
        selectedVersionId={2}
        isOwner={false}
        hasMore={false}
        onLoadMore={noop}
        onSelect={noop}
        onCompare={onCompare}
        onRestore={noop}
        onRetry={noop}
        onClose={noop}
      />
    );
    fireEvent.click(screen.getByLabelText('versioning.compare v1'));
    expect(onCompare).toHaveBeenCalledWith(versions[1]);
  });

  test('Load more button shown when hasMore', () => {
    const onLoadMore = jest.fn();
    render(
      <VersionHistoryPanel
        versions={versions}
        loading={false}
        error={false}
        selectedVersionId={2}
        isOwner={false}
        hasMore
        onLoadMore={onLoadMore}
        onSelect={noop}
        onCompare={noop}
        onRestore={noop}
        onRetry={noop}
        onClose={noop}
      />
    );
    fireEvent.click(screen.getByText('versioning.load_more'));
    expect(onLoadMore).toHaveBeenCalled();
  });

  test('error state shows message and Retry', () => {
    const onRetry = jest.fn();
    render(
      <VersionHistoryPanel
        versions={[]}
        loading={false}
        error
        selectedVersionId={null}
        isOwner={false}
        hasMore={false}
        onLoadMore={noop}
        onSelect={noop}
        onCompare={noop}
        onRestore={noop}
        onRetry={onRetry}
        onClose={noop}
      />
    );
    expect(screen.getByText('versioning.error.fetch')).toBeInTheDocument();
    fireEvent.click(screen.getByText('versioning.error.retry'));
    expect(onRetry).toHaveBeenCalled();
  });

  test('single version shows hint instead of list', () => {
    render(
      <VersionHistoryPanel
        versions={[baseVersion()]}
        loading={false}
        error={false}
        selectedVersionId={1}
        isOwner
        hasMore={false}
        onLoadMore={noop}
        onSelect={noop}
        onCompare={noop}
        onRestore={noop}
        onRetry={noop}
        onClose={noop}
      />
    );
    expect(screen.getByText('versioning.single_version')).toBeInTheDocument();
  });

  test('loading with empty list shows skeleton rows', () => {
    render(
      <VersionHistoryPanel
        versions={[]}
        loading
        error={false}
        selectedVersionId={null}
        isOwner={false}
        hasMore={false}
        onLoadMore={noop}
        onSelect={noop}
        onCompare={noop}
        onRestore={noop}
        onRetry={noop}
        onClose={noop}
      />
    );
    // 4 skeleton rows rendered while the first page loads
    expect(screen.getAllByTestId('version-skeleton-row')).toHaveLength(4);
  });
});
