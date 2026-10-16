import React from 'react';
import { useTranslation } from 'react-i18next';
import './CollectionEmptyState.scss';

/**
 * Empty state for collections (explore / mine / collection-detail variants).
 *
 * @param {Object} props
 * @param {string} props.variant - "explore" | "mine" | "collection"
 * @param {React.ReactNode} [props.action] - Optional action node (e.g. a button)
 */
const CollectionEmptyState = ({ variant = 'explore', action }) => {
  const { t } = useTranslation();
  const key = `collections.empty.${variant}`;
  return (
    <div className="collection-empty-state">
      <p className="collection-empty-state__text">{t(key)}</p>
      {action && <div className="collection-empty-state__action">{action}</div>}
    </div>
  );
};

export default CollectionEmptyState;