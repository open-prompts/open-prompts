import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import StarRating from '../StarRating';
import RatingHistogram from '../RatingHistogram';
import RatingWidget from '../RatingWidget';
import { rateTemplate, deleteRating } from '../../services/api';

const mockAddNotification = jest.fn();

jest.mock('../../context/NotificationContext', () => ({
  useNotification: () => ({ addNotification: mockAddNotification }),
  NotificationProvider: ({ children }) => children,
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key, opts) => (opts ? `${key}:${JSON.stringify(opts)}` : key),
  }),
}));

jest.mock('../../services/api', () => ({
  rateTemplate: jest.fn(),
  deleteRating: jest.fn(),
}));

jest.mock('react-redux', () => ({
  useSelector: () => ({ id: 'user-1' }),
}));

const renderWidget = (props) =>
  render(
    <MemoryRouter>
      <RatingWidget templateId="t1" {...props} />
    </MemoryRouter>
  );

describe('StarRating', () => {
  test('renders nothing at zero ratings', () => {
    render(<StarRating ratingAvg={0} ratingCount={0} />);
    expect(screen.queryByRole('img')).toBeNull();
  });

  test('renders value and count when rated', () => {
    render(<StarRating ratingAvg={4.6} ratingCount={128} />);
    expect(screen.getByText('4.6')).toBeInTheDocument();
    expect(screen.getByText('ratings.ratings_count:{"count":128}')).toBeInTheDocument();
    expect(screen.getByLabelText('ratings.aria_label:{"avg":4.6,"count":128}')).toBeInTheDocument();
  });
});

describe('RatingHistogram', () => {
  test('renders nothing at zero total', () => {
    render(<RatingHistogram distribution={[0, 0, 0, 0, 0]} />);
    expect(screen.queryByRole('list')).toBeNull();
  });

  test('renders 5 rows with percentages', () => {
    render(<RatingHistogram distribution={[1, 1, 1, 1, 1]} />);
    const rows = screen.getAllByRole('listitem');
    expect(rows).toHaveLength(5);
    expect(screen.getAllByText('20%')).toHaveLength(5);
  });
});

describe('RatingWidget', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  test('shows owner hint for owner', () => {
    renderWidget({ isOwner: true });
    expect(screen.getByText('ratings.owner_hint')).toBeInTheDocument();
  });

  test('commits a rating on star click', async () => {
    rateTemplate.mockResolvedValue({ data: { rating_avg: 4, rating_count: 1, my_rating: 4 } });
    renderWidget({ ratingAvg: 0, ratingCount: 0, myRating: 0 });
    const stars = screen.getAllByRole('button');
    fireEvent.click(stars[3]); // 4th star = rating 4
    await waitFor(() => expect(rateTemplate).toHaveBeenCalledWith('t1', 4));
    expect(mockAddNotification).toHaveBeenCalled();
  });

  test('clears rating via confirm popover', async () => {
    deleteRating.mockResolvedValue({ data: { rating_avg: 0, rating_count: 0, my_rating: 0 } });
    renderWidget({ ratingAvg: 4, ratingCount: 1, myRating: 4 });
    fireEvent.click(screen.getByText('ratings.clear'));
    fireEvent.click(screen.getByText('common.confirmation'));
    await waitFor(() => expect(deleteRating).toHaveBeenCalledWith('t1'));
  });

  test('reverts on failure', async () => {
    rateTemplate.mockRejectedValue({ response: { status: 500 } });
    renderWidget({ ratingAvg: 0, ratingCount: 0, myRating: 0 });
    const stars = screen.getAllByRole('button');
    fireEvent.click(stars[2]); // rating 3
    await waitFor(() => expect(mockAddNotification).toHaveBeenCalled());
    // value reverted to 0 -> no clear button
    expect(screen.queryByText('ratings.clear')).toBeNull();
  });

  test('keyboard arrow adjusts value and enter commits', async () => {
    rateTemplate.mockResolvedValue({ data: { rating_avg: 2, rating_count: 1, my_rating: 2 } });
    renderWidget({ ratingAvg: 0, ratingCount: 0, myRating: 0 });
    const slider = screen.getByRole('slider');
    fireEvent.keyDown(slider, { key: 'ArrowRight' });
    fireEvent.keyDown(slider, { key: 'ArrowRight' });
    fireEvent.keyDown(slider, { key: 'Enter' });
    await waitFor(() => expect(rateTemplate).toHaveBeenCalledWith('t1', 2));
  });
});