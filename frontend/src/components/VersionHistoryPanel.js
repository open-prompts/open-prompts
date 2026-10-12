import React from 'react';
import { useTranslation } from 'react-i18next';
import { formatRelativeTime } from '../utils/time';

/**
 * Inline version row (not exported; part of the panel).
 * Newest-first list row: number, Latest badge, note, relative time,
 * chars/lines/prompts meta, Compare + Restore actions.
 */
const VersionRow = ({ version, isLatest, isSelected, isOwner, onSelect, onCompare, onRestore }) => {
  const { t } = useTranslation();
  const note = version.change_note || '';
  const chars = (version.content || '').length;
  const lines = (version.content || '').split('\n').length;
  const hasUsage = typeof version.prompt_count === 'number';
  const absTime = version.created_at ? new Date(version.created_at).toLocaleString() : '';

  return (
    <li className={`version-row ${isSelected ? 'selected' : ''}`}>
      <button
        type="button"
        className="version-row__main"
        onClick={() => onSelect(version)}
        aria-label={`v${version.version}`}
      >
        <span className="version-row__number">v{version.version}</span>
        {isLatest && <span className="version-row__latest-badge">{t('versioning.latest')}</span>}
        <span className={`version-row__note ${note ? '' : 'empty'}`}>
          {note || t('versioning.no_note')}
        </span>
        <span className="version-row__time" title={absTime}>
          {formatRelativeTime(version.created_at, t)}
        </span>
        <span className="version-row__meta">
          {hasUsage
            ? t('versioning.meta', { chars, lines, prompts: version.prompt_count })
            : t('versioning.meta_no_usage', { chars, lines })}
        </span>
      </button>
      <div className="version-row__actions">
        <button
          type="button"
          className="btn-ghost btn-sm"
          onClick={() => onCompare(version)}
          aria-label={`${t('versioning.compare')} v${version.version}`}
        >
          {t('versioning.compare')}
        </button>
        {isOwner && !isLatest && (
          <button
            type="button"
            className="btn-ghost btn-sm version-row__restore"
            onClick={() => onRestore(version)}
            aria-label={`${t('versioning.restore')} v${version.version}`}
          >
            {t('versioning.restore')}
          </button>
        )}
      </div>
    </li>
  );
};

/**
 * Collapsible version history panel (in-place on the template details page).
 * Lists versions newest-first, 20/page with Load more.
 */
const VersionHistoryPanel = ({
  versions,
  loading,
  error,
  selectedVersionId,
  isOwner,
  hasMore,
  onLoadMore,
  onSelect,
  onCompare,
  onRestore,
  onRetry,
  onClose
}) => {
  const { t } = useTranslation();

  return (
    <div className="version-history-panel" aria-labelledby="version-history-title">
      <div className="version-history-panel__header">
        <h4 id="version-history-title">{t('versioning.panel_title')}</h4>
        <button type="button" className="btn-ghost btn-sm" onClick={onClose}>
          {t('common.close')}
        </button>
      </div>

      {error ? (
        <div className="version-history-panel__error">
          <p>{t('versioning.error.fetch')}</p>
          <button type="button" className="btn-ghost btn-sm" onClick={onRetry}>
            {t('versioning.error.retry')}
          </button>
        </div>
      ) : loading && versions.length === 0 ? (
        <ul className="version-history-panel__list" aria-hidden>
          {[0, 1, 2, 3].map((i) => (
            <li key={i} className="version-row skeleton" data-testid="version-skeleton-row" />
          ))}
        </ul>
      ) : versions.length <= 1 ? (
        <p className="version-history-panel__hint">{t('versioning.single_version')}</p>
      ) : (
        <>
          <ul className="version-history-panel__list">
            {versions.map((v, idx) => (
              <VersionRow
                key={v.id}
                version={v}
                isLatest={idx === 0}
                isSelected={v.id === selectedVersionId}
                isOwner={isOwner}
                onSelect={onSelect}
                onCompare={onCompare}
                onRestore={onRestore}
              />
            ))}
          </ul>
          {hasMore && (
            <button type="button" className="btn-ghost btn-sm version-history-panel__more" onClick={onLoadMore}>
              {t('versioning.load_more')}
            </button>
          )}
        </>
      )}
    </div>
  );
};

export default VersionHistoryPanel;
