import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import Header from '../components/Header';
import Layout from '../components/Layout';
import AppealStateChip from '../components/AppealStateChip';
import { getAppeal } from '../services/api';
import { formatRelativeTime } from '../utils/time';
import './AppealDetailPage.scss';

/**
 * /appeals/:id — the caller's appeal detail: statement + action context +
 * resolution block.
 */
const AppealDetailPage = () => {
  const { t } = useTranslation();
  const { id } = useParams();
  const navigate = useNavigate();
  const [appeal, setAppeal] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const resp = await getAppeal(id);
      setAppeal(resp.data);
    } catch (err) {
      const status = err.response && err.response.status;
      setError(
        status === 404
          ? t('common.notFound')
          : (err.response && err.response.data && err.response.data.error) || ''
      );
    } finally {
      setLoading(false);
    }
  }, [id, t]);

  useEffect(() => {
    load();
  }, [load]);

  const resolved = appeal && (appeal.state === 'resolved_upheld' || appeal.state === 'resolved_reversed');

  return (
    <Layout>
      <Header />
      <div className="appeal-detail">
        <button type="button" className="appeal-detail__back" onClick={() => navigate('/appeals')}>
          ← {t('appeal.list.title')}
        </button>
        {loading && <div className="appeal-detail__loading" role="status">{t('common.loading')}</div>}
        {error && <div className="appeal-detail__error" role="alert">{error}</div>}
        {appeal && (
          <div className="appeal-detail__card">
            <div className="appeal-detail__header">
              <AppealStateChip state={appeal.state} superseded={appeal.superseded} />
              <span className="appeal-detail__time">{formatRelativeTime(appeal.created_at, t)}</span>
            </div>
            <div className="appeal-detail__action">
              <span className="appeal-detail__action-type">{appeal.action_type}</span>
              <span className="appeal-detail__action-reason">"{appeal.action_reason}"</span>
              <span className="appeal-detail__action-meta">
                {appeal.actor_display_name} · {appeal.template_title}
              </span>
            </div>
            <div className="appeal-detail__section">
              <h3 className="appeal-detail__section-title">{t('appeal.detail.statement')}</h3>
              <p className="appeal-detail__statement">{appeal.statement}</p>
            </div>
            <div className="appeal-detail__section">
              <h3 className="appeal-detail__section-title">{t('appeal.detail.resolution')}</h3>
              {resolved ? (
                <p className="appeal-detail__resolution">{appeal.resolution_note}</p>
              ) : (
                <p className="appeal-detail__pending">{t('appeal.detail.pending')}</p>
              )}
            </div>
          </div>
        )}
      </div>
    </Layout>
  );
};

export default AppealDetailPage;