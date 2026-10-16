import React from 'react';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import CollectionCard from './CollectionCard';
import CollectionEmptyState from './CollectionEmptyState';
import CopyLinkButton from './CopyLinkButton';
import PublishToggle from './PublishToggle';
import ReorderList from './ReorderList';
import CollectionPicker from './CollectionPicker';
import {
  listMyCollections,
  listCollectionsContainingTemplate,
  addTemplateToCollection,
  removeTemplateFromCollection,
  createCollection,
} from '../../services/api';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key, opts) => (opts ? `${key}:${JSON.stringify(opts)}` : key),
  }),
}));

jest.mock('../../services/api', () => ({
  listMyCollections: jest.fn(),
  listCollectionsContainingTemplate: jest.fn(),
  addTemplateToCollection: jest.fn(),
  removeTemplateFromCollection: jest.fn(),
  createCollection: jest.fn(),
}));

jest.mock('../ui/Modal', () => (props) => {
  if (!props.open) return null;
  return (
    <div data-testid="modal">
      <h3>{props.title}</h3>
      <div>{props.children}</div>
      <button type="button" onClick={props.onPrimary}>{props.primaryLabel}</button>
      <button type="button" onClick={props.onClose}>{props.secondaryLabel}</button>
    </div>
  );
});

const makeCollection = (id, name, count = 0, members = []) => ({
  id,
  name,
  template_count: count,
  members,
  owner: { id: 'owner-1', display_name: 'Ada' },
});

describe('CollectionCard', () => {
  test('renders name, count, and curator', () => {
    render(
      <MemoryRouter>
        <CollectionCard collection={makeCollection('c1', 'Best prompts', 3)} />
      </MemoryRouter>
    );
    expect(screen.getByText('Best prompts')).toBeInTheDocument();
    expect(screen.getByText(/collections.items/)).toBeInTheDocument();
    expect(screen.getByText(/collections.by/)).toBeInTheDocument();
  });

  test('links to the detail page', () => {
    render(
      <MemoryRouter>
        <CollectionCard collection={makeCollection('c1', 'Best prompts')} />
      </MemoryRouter>
    );
    expect(screen.getByRole('link')).toHaveAttribute('href', '/collections/c1');
  });
});

describe('CollectionEmptyState', () => {
  test('renders the variant text', () => {
    render(<CollectionEmptyState variant="mine" />);
    expect(screen.getByText('collections.empty.mine')).toBeInTheDocument();
  });
});

describe('CopyLinkButton', () => {
  test('copies the URL and shows feedback', async () => {
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: jest.fn().mockResolvedValue() },
      configurable: true,
    });
    render(<CopyLinkButton url="http://example.com/collections/c1" />);
    fireEvent.click(screen.getByRole('button'));
    await waitFor(() => expect(navigator.clipboard.writeText).toHaveBeenCalledWith('http://example.com/collections/c1'));
    expect(await screen.findByText('collections.link_copied')).toBeInTheDocument();
  });
});

describe('PublishToggle', () => {
  test('shows Publish for private and confirms', () => {
    const onToggle = jest.fn();
    render(<PublishToggle isPublic={false} onToggle={onToggle} />);
    fireEvent.click(screen.getByText('collections.publish'));
    expect(screen.getByText('collections.publish_confirm')).toBeInTheDocument();
    fireEvent.click(screen.getByText('common.confirmation'));
    expect(onToggle).toHaveBeenCalledWith('public');
  });

  test('shows Unpublish for public', () => {
    render(<PublishToggle isPublic onToggle={jest.fn()} />);
    expect(screen.getByText('collections.unpublish')).toBeInTheDocument();
  });
});

describe('ReorderList', () => {
  const members = [
    { template: { id: 't1', title: 'One' }, position: 0 },
    { template: { id: 't2', title: 'Two' }, position: 1 },
  ];

  test('saves the ordered ids', () => {
    const onSave = jest.fn();
    render(<ReorderList members={members} onSave={onSave} onDiscard={jest.fn()} />);
    // Move the second row up.
    const downButtons = screen.getAllByText('↑');
    fireEvent.click(downButtons[1]);
    fireEvent.click(screen.getByText('collections.picker.save'));
    expect(onSave).toHaveBeenCalledWith(['t2', 't1']);
  });

  test('discards without saving', () => {
    const onDiscard = jest.fn();
    render(<ReorderList members={members} onSave={jest.fn()} onDiscard={onDiscard} />);
    fireEvent.click(screen.getByText('collections.picker.cancel'));
    expect(onDiscard).toHaveBeenCalled();
  });
});

describe('CollectionPicker', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    listMyCollections.mockResolvedValue({
      data: { collections: [makeCollection('c1', 'Best prompts', 1)] },
    });
    listCollectionsContainingTemplate.mockResolvedValue({
      data: { collections: [makeCollection('c1', 'Best prompts', 1)] },
    });
  });

  test('loads collections and shows checkmarks', async () => {
    render(<CollectionPicker template={{ id: 't1' }} onClose={jest.fn()} />);
    await waitFor(() => expect(listMyCollections).toHaveBeenCalled());
    expect(await screen.findByText('Best prompts')).toBeInTheDocument();
    const checkbox = screen.getByRole('checkbox');
    expect(checkbox.checked).toBe(true); // already containing
  });

  test('stages an add and commits on save', async () => {
    addTemplateToCollection.mockResolvedValue({ data: {} });
    // Collection does NOT contain the template -> checkbox unchecked.
    listCollectionsContainingTemplate.mockResolvedValue({ data: { collections: [] } });
    render(<CollectionPicker template={{ id: 't1' }} onClose={jest.fn()} />);
    await waitFor(() => expect(listMyCollections).toHaveBeenCalled());
    const checkbox = await screen.findByRole('checkbox');
    expect(checkbox.checked).toBe(false);
    fireEvent.click(checkbox); // stage add
    fireEvent.click(screen.getByText('collections.picker.save'));
    await waitFor(() => expect(addTemplateToCollection).toHaveBeenCalledWith('c1', 't1'));
  });

  test('stages a remove and commits on save', async () => {
    removeTemplateFromCollection.mockResolvedValue({ data: {} });
    render(<CollectionPicker template={{ id: 't1' }} onClose={jest.fn()} />);
    await waitFor(() => expect(listMyCollections).toHaveBeenCalled());
    const checkbox = await screen.findByRole('checkbox');
    expect(checkbox.checked).toBe(true); // already containing
    fireEvent.click(checkbox); // stage remove
    fireEvent.click(screen.getByText('collections.picker.save'));
    await waitFor(() => expect(removeTemplateFromCollection).toHaveBeenCalledWith('c1', 't1'));
  });

  test('creates a new collection inline', async () => {
    createCollection.mockResolvedValue({ data: makeCollection('c2', 'New col') });
    render(<CollectionPicker template={{ id: 't1' }} onClose={jest.fn()} />);
    await waitFor(() => expect(listMyCollections).toHaveBeenCalled());
    await screen.findByRole('checkbox');
    fireEvent.change(screen.getByPlaceholderText('collections.picker.create_new'), {
      target: { value: 'New col' },
    });
    fireEvent.click(screen.getByText('collections.new'));
    await waitFor(() => expect(createCollection).toHaveBeenCalledWith({ name: 'New col' }));
  });
});