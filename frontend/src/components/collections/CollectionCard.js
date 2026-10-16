import React from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import './CollectionCard.scss';

// Deterministic background colors for the initials collage.
const COLLAGE_COLORS = ['#5b8def', '#7c5ce0', '#00b894', '#e17055', '#0984e3', '#e84393'];

function colorFor(index) {
  return COLLAGE_COLORS[index % COLLAGE_COLORS.length];
}

function initials(name) {
  const parts = (name || '').trim().split(/\s+/);
  if (parts.length === 0) return '?';
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[1][0]).toUpperCase();
}

/**
 * CollectionCard — a 2×2 CSS initials collage, name, template count, curator
 * and updated date. Links to the collection detail page.
 *
 * @param {Object} props
 * @param {Object} props.collection - The collection summary
 */
const CollectionCard = ({ collection }) => {
  const { t } = useTranslation();
  const members = collection.members || [];
  const tiles = members.slice(0, 4);
  const count = collection.template_count || 0;

  return (
    <Link to={`/collections/${collection.id}`} className="collection-card">
      <div className="collection-card__collage" aria-hidden="true">
        {tiles.length === 0 && (
          <div className="collection-card__tile collection-card__tile--empty">
            <span className="collection-card__initials">?</span>
          </div>
        )}
        {tiles.map((m, i) => (
          <div
            key={i}
            className="collection-card__tile"
            style={{ backgroundColor: colorFor(i) }}
          >
            <span className="collection-card__initials">{initials(m.template && m.template.title)}</span>
          </div>
        ))}
      </div>
      <div className="collection-card__body">
        <h3 className="collection-card__name">{collection.name}</h3>
        <p className="collection-card__meta">
          {t('collections.items', { count })}
          {collection.owner && collection.owner.display_name
            ? ` · ${t('collections.by', { name: collection.owner.display_name })}`
            : ''}
        </p>
      </div>
    </Link>
  );
};

export default CollectionCard;