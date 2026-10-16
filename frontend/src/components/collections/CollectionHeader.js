import React from 'react';
import { useTranslation } from 'react-i18next';
import CopyLinkButton from './CopyLinkButton';
import PublishToggle from './PublishToggle';
import './CollectionHeader.scss';

/**
 * CollectionHeader — title, description, curator line, count, and an action
 * bar (Copy link for public; Edit/Reorder/Delete for the owner).
 *
 * @param {Object} props
 * @param {Object} props.collection - The collection
 * @param {boolean} props.isOwner - Whether the viewer owns it
 * @param {Function} props.onEdit - Open edit modal
 * @param {Function} props.onPublishToggle - Toggle visibility
 * @param {Function} props.onDelete - Delete collection
 * @param {Function} props.onReorder - Toggle reorder mode
 * @param {boolean} props.reorderMode - Whether reorder mode is active
 */
const CollectionHeader = ({
  collection,
  isOwner,
  onEdit,
  onPublishToggle,
  onDelete,
  onReorder,
  reorderMode = false,
}) => {
  const { t } = useTranslation();
  const isPublic = collection.visibility === 'public';
  const url = typeof window !== 'undefined' ? window.location.href : '';

  return (
    <div className="collection-header">
      <h1 className="collection-header__title">{collection.name}</h1>
      {collection.description && (
        <p className="collection-header__desc">{collection.description}</p>
      )}
      <p className="collection-header__meta">
        {t('collections.items', { count: collection.template_count || 0 })}
        {collection.owner && collection.owner.display_name
          ? ` · ${t('collections.by', { name: collection.owner.display_name })}`
          : ''}
      </p>
      <div className="collection-header__actions">
        {isPublic && <CopyLinkButton url={url} />}
        {isOwner && (
          <>
            <button type="button" className="collection-header__btn" onClick={onEdit}>
              {t('collections.edit')}
            </button>
            <button type="button" className="collection-header__btn" onClick={onReorder}>
              {reorderMode ? t('collections.picker.cancel') : t('collections.reorder')}
            </button>
            <PublishToggle isPublic={isPublic} onToggle={onPublishToggle} />
            <button type="button" className="collection-header__btn collection-header__btn--danger" onClick={onDelete}>
              {t('collections.delete')}
            </button>
          </>
        )}
      </div>
    </div>
  );
};

export default CollectionHeader;