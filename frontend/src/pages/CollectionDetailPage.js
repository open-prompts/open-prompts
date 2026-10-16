import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useSelector } from 'react-redux';
import Layout from '../components/Layout';
import Modal from '../components/ui/Modal';
import Field from '../components/ui/Field';
import PromptCard from '../components/PromptCard';
import CollectionHeader from '../components/collections/CollectionHeader';
import CollectionEmptyState from '../components/collections/CollectionEmptyState';
import ReorderList from '../components/collections/ReorderList';
import CollectionPicker from '../components/collections/CollectionPicker';
import {
  getCollection,
  updateCollection,
  deleteCollection,
  reorderCollectionTemplates,
} from '../services/api';
import { usePageAnnouncement } from '../utils/a11y';
import './CollectionDetailPage.scss';

/**
 * CollectionDetailPage — /collections/:id. Header + member grid; owner
 * actions (Edit, Reorder, Publish/Unpublish, Delete); 404 page on NotFound.
 */
const CollectionDetailPage = () => {
  const { id } = useParams();
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { user } = useSelector((state) => state.auth);
  const [collection, setCollection] = useState(null);
  const [loading, setLoading] = useState(true);
  const [notFound, setNotFound] = useState(false);
  const [error, setError] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [reorderMode, setReorderMode] = useState(false);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [visibility, setVisibility] = useState('private');
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState('');
  const [pickerTemplate, setPickerTemplate] = useState(null);

  usePageAnnouncement('collections.explore_title');

  const isOwner = collection && user && collection.owner_id === user.id;

  const load = () => {
    setLoading(true);
    setNotFound(false);
    setError(false);
    getCollection(id)
      .then((res) => {
        setCollection(res.data);
        setLoading(false);
      })
      .catch((err) => {
        const status = err.response && err.response.status;
        if (status === 404) setNotFound(true);
        else setError(true);
        setLoading(false);
      });
  };

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  const handleEdit = () => {
    setName(collection.name);
    setDescription(collection.description || '');
    setVisibility(collection.visibility);
    setFormError('');
    setEditOpen(true);
  };

  const handleSaveEdit = async () => {
    const trimmed = name.trim();
    if (!trimmed) {
      setFormError(t('collections.error.save'));
      return;
    }
    setSaving(true);
    setFormError('');
    try {
      const resp = await updateCollection(id, { name: trimmed, description, visibility });
      setCollection(resp.data);
      setEditOpen(false);
    } catch (err) {
      setFormError(t('collections.error.save'));
    } finally {
      setSaving(false);
    }
  };

  const handlePublishToggle = async (newVisibility) => {
    try {
      const resp = await updateCollection(id, { visibility: newVisibility });
      setCollection(resp.data);
    } catch (err) {
      setError(true);
    }
  };

  const handleDelete = async () => {
    try {
      await deleteCollection(id);
      navigate('/collections/mine');
    } catch (err) {
      setError(true);
      setDeleteOpen(false);
    }
  };

  const handleReorderSave = async (order) => {
    try {
      await reorderCollectionTemplates(id, order);
      setReorderMode(false);
      load();
    } catch (err) {
      setError(true);
      setReorderMode(false);
    }
  };

  const members = collection ? collection.members || [] : [];

  return (
    <Layout>
      <div className="collection-detail-page">
        {loading && <div className="collection-detail-page__loading">{t('common.loading')}</div>}

        {notFound && (
          <div className="collection-detail-page__not-found">
            <h1>{t('collections.error.load')}</h1>
          </div>
        )}

        {error && !notFound && (
          <p className="collection-detail-page__error">{t('collections.error.load')}</p>
        )}

        {collection && !notFound && (
          <>
            <CollectionHeader
              collection={collection}
              isOwner={isOwner}
              onEdit={handleEdit}
              onPublishToggle={handlePublishToggle}
              onDelete={() => setDeleteOpen(true)}
              onReorder={() => setReorderMode((v) => !v)}
              reorderMode={reorderMode}
            />

            {reorderMode && isOwner ? (
              <ReorderList
                members={members}
                onSave={handleReorderSave}
                onDiscard={() => setReorderMode(false)}
              />
            ) : (
              <>
                {members.length === 0 ? (
                  <CollectionEmptyState variant="collection" />
                ) : (
                  <div className="collection-detail-page__grid">
                    {members.map((m) => (
                      <div key={m.template.id} className="collection-detail-page__member">
                        <PromptCard
                          template={m.template}
                          onAddToCollection={setPickerTemplate}
                        />
                        {isOwner && !m.is_public && (
                          <span className="collection-detail-page__badge">
                            {t('collections.member_badge.private')}
                          </span>
                        )}
                      </div>
                    ))}
                  </div>
                )}
              </>
            )}
          </>
        )}

        <Modal
          open={editOpen}
          title={t('collections.edit')}
          primaryLabel={t('common.save')}
          secondaryLabel={t('common.cancel')}
          primaryDisabled={saving}
          onPrimary={handleSaveEdit}
          onClose={() => setEditOpen(false)}
        >
          <div className="collection-detail-page__form">
            <Field id="edit-name" label={t('create_template.label_title')} required>
              <input
                type="text"
                id="edit-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                maxLength={80}
              />
            </Field>
            <Field id="edit-desc" label={t('create_template.label_description')}>
              <textarea
                id="edit-desc"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                maxLength={500}
                rows={3}
              />
            </Field>
            <Field id="edit-vis" label={t('create_template.label_visibility')}>
              <select id="edit-vis" value={visibility} onChange={(e) => setVisibility(e.target.value)}>
                <option value="private">{t('create_template.visibility_private')}</option>
                <option value="public">{t('create_template.visibility_public')}</option>
              </select>
            </Field>
            {formError && <p className="collection-detail-page__form-error">{formError}</p>}
          </div>
        </Modal>

        <Modal
          open={deleteOpen}
          title={t('collections.delete')}
          primaryLabel={t('common.delete')}
          secondaryLabel={t('common.cancel')}
          danger
          onPrimary={handleDelete}
          onClose={() => setDeleteOpen(false)}
        >
          <p>{t('collections.delete_confirm')}</p>
        </Modal>

        {pickerTemplate && (
          <CollectionPicker template={pickerTemplate} onClose={() => setPickerTemplate(null)} />
        )}
      </div>
    </Layout>
  );
};

export default CollectionDetailPage;