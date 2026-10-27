import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import Layout from '../components/Layout';
import AppealStateChip from '../components/AppealStateChip';
import { listAppeals } from '../services/api';
import { formatRelativeTime } from '../utils/time';
import './AppealsPage.scss';

const PAGE_SIZE = 20;

/**
 * /appeals — the caller's own appeals, newest first, with state chips,
 * action context, timestamps, and a resolution note excerpt when resolved.
 */
const AppealsPage = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const load = useCallback(async (p) => {
    setLoading(true);
    setError('');
    try {
      const resp = await listAppeals({ page: p, per_page: PAGE_SIZE });
      setItems(resp.data.appeals || []);
      setTotal(resp.data.total || 0);
    } catch (err) {
      setError((err.response && err.response.data && err.response.data.error) || '');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load(page);
  }, [load, page]);

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <Layout>
      <div className="appeals-page">
        <h1 className="appeals-page__title">{t('appeal.list.title')}</h1>
        {error && <div className="appeals-page__error" role="alert">{error}</div>}
        {loading && <div className="appeals-page__loading" role="status">{t('common.loading')}</div>}
        {!loading && items.length === 0 && (
          <div className="appeals-page__empty">{t('appeal.list.empty')}</div>
        )}
        {!loading && items.length > 0 && (
          <ul className="appeals-page__list">
            {items.map((a) => (
              <li key={a.id}>
                <button
                  type="button"
                  className="appeal-card"
                  onClick={() => navigate(`/appeals/${a.id}`)}
                >
                  <div className="appeal-card__top">
                    <AppealStateChip state={a.state} superseded={a.superseded} />
                    <span className="appeal-card__time">{formatRelativeTime(a.created_at, t)}</span>
                  </div>
                  <div className="appeal-card__title">
                    {a.action_type} · {a.template_title}
                  </div>
                  {a.resolution_note && (
                    <div className="appeal-card__note">{a.resolution_note}</div>
                  )}
                </button>
              </li>
            ))}
          </ul>
        )}
        {!loading && totalPages > 1 && (
          <div className="appeals-page__pagination">
            <button disabled={page <= 1} onClick={() => setPage(page - 1)}>
              {t('common.prev')}
            </button>
            <span>{t('common.page', { page, total: totalPages })}</span>
            <button disabled={page >= totalPages} onClick={() => setPage(page + 1)}>
              {t('common.next')}
            </button>
          </div>
        )}
      </div>
    </Layout>
  );
};

export default AppealsPage;