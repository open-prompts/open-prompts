import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useSelector } from 'react-redux';
import Layout from '../components/Layout';
import Modal from '../components/ui/Modal';
import Field from '../components/ui/Field';
import CollectionCard from '../components/collections/CollectionCard';
import CollectionEmptyState from '../components/collections/CollectionEmptyState';
import { listMyCollections, createCollection, deleteCollection } from '../services/api';
import { usePageAnnouncement } from '../utils/a11y';
import './MyCollectionsPage.scss';

/**
 * MyCollectionsPage — auth-guarded /collections/mine. "New collection" button
 * opens a create modal; grid of own collections with edit/delete.
 */
const MyCollectionsPage = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { token } = useSelector((state) => state.auth);
  const [collections, setCollections] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [visibility, setVisibility] = useState('private');
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState('');
  const [deleteTarget, setDeleteTarget] = useState(null);

  usePageAnnouncement('collections.mine_title');

  const load = () => {
    setLoading(true);
    setError(false);
    listMyCollections()
      .then((res) => {
        setCollections(res.data.collections || []);
        setLoading(false);
      })
      .catch(() => {
        setError(true);
        setLoading(false);
      });
  };

  useEffect(() => {
    if (!token) {
      navigate('/login');
      return undefined;
    }
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token]);

  const handleCreate = async () => {
    const trimmed = name.trim();
    if (!trimmed) {
      setFormError(t('collections.error.save'));
      return;
    }
    setSaving(true);
    setFormError('');
    try {
      await createCollection({ name: trimmed, description, visibility });
      setCreateOpen(false);
      setName('');
      setDescription('');
      setVisibility('private');
      load();
    } catch (err) {
      const status = err.response && err.response.status;
      if (status === 429) setFormError(t('collections.picker.limit_reached'));
      else setFormError(t('collections.error.save'));
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    try {
      await deleteCollection(deleteTarget.id);
      setDeleteTarget(null);
      load();
    } catch (err) {
      setError(true);
      setDeleteTarget(null);
    }
  };

  return (
    <Layout>
      <div className="my-collections-page">
        <div className="my-collections-page__header">
          <h1 className="my-collections-page__title">{t('collections.mine_title')}</h1>
          <button type="button" className="btn btn-primary" onClick={() => setCreateOpen(true)}>
            {t('collections.new')}
          </button>
        </div>

        {error && <p className="my-collections-page__error">{t('collections.error.load')}</p>}

        {!error && collections.length === 0 && !loading && (
          <CollectionEmptyState variant="mine" />
        )}

        <div className="my-collections-page__grid">
          {collections.map((c) => (
            <div key={c.id} className="my-collections-page__card">
              <CollectionCard collection={c} />
              <button
                type="button"
                className="my-collections-page__delete"
                onClick={() => setDeleteTarget(c)}
              >
                {t('collections.delete')}
              </button>
            </div>
          ))}
        </div>

        {loading && <div className="my-collections-page__loading">{t('common.loading')}</div>}

        <Modal
          open={createOpen}
          title={t('collections.new')}
          primaryLabel={t('common.create')}
          secondaryLabel={t('common.cancel')}
          primaryDisabled={saving}
          onPrimary={handleCreate}
          onClose={() => setCreateOpen(false)}
        >
          <div className="my-collections-page__form">
            <Field id="coll-name" label={t('create_template.label_title')} required>
              <input
                type="text"
                id="coll-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                maxLength={80}
              />
            </Field>
            <Field id="coll-desc" label={t('create_template.label_description')}>
              <textarea
                id="coll-desc"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                maxLength={500}
                rows={3}
              />
            </Field>
            <Field id="coll-vis" label={t('create_template.label_visibility')}>
              <select id="coll-vis" value={visibility} onChange={(e) => setVisibility(e.target.value)}>
                <option value="private">{t('create_template.visibility_private')}</option>
                <option value="public">{t('create_template.visibility_public')}</option>
              </select>
            </Field>
            {formError && <p className="my-collections-page__form-error">{formError}</p>}
          </div>
        </Modal>

        <Modal
          open={!!deleteTarget}
          title={t('collections.delete')}
          primaryLabel={t('common.delete')}
          secondaryLabel={t('common.cancel')}
          danger
          onPrimary={handleDelete}
          onClose={() => setDeleteTarget(null)}
        >
          <p>{t('collections.delete_confirm')}</p>
        </Modal>
      </div>
    </Layout>
  );
};

export default MyCollectionsPage;