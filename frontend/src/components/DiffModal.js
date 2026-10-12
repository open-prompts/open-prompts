import React, { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import Modal from './ui/Modal';
import VersionSelect from './VersionSelect';
import { diffLines, diffStats, formatDiffText } from '../utils/diff';

/**
 * Client-side unified diff between two template versions.
 * A = older, B = newer (normalized); Swap flips them. Falls back to
 * side-by-side raw panes if diff computation throws.
 */
const DiffModal = ({ open, versions, initialAId, initialBId, onClose, onCopy }) => {
  const { t } = useTranslation();
  const [aId, setAId] = useState(initialAId);
  const [bId, setBId] = useState(initialBId);

  // Normalize A = older, B = newer.
  const aVersion = versions.find((v) => v.id === aId);
  const bVersion = versions.find((v) => v.id === bId);
  const [normA, normB] = useMemo(() => {
    if (!aVersion || !bVersion) return [aVersion, bVersion];
    return aVersion.version <= bVersion.version
      ? [aVersion, bVersion]
      : [bVersion, aVersion];
  }, [aVersion, bVersion]);

  const { rows, stats, failed } = useMemo(() => {
    if (!normA || !normB) return { rows: [], stats: { added: 0, removed: 0 }, failed: false };
    try {
      const r = diffLines(normA.content, normB.content);
      return { rows: r, stats: diffStats(r), failed: false };
    } catch (e) {
      return { rows: [], stats: { added: 0, removed: 0 }, failed: true };
    }
  }, [normA, normB]);

  const handleSwap = () => {
    setAId(bId);
    setBId(aId);
  };

  const handleCopy = () => {
    const text = formatDiffText(rows);
    navigator.clipboard.writeText(text).then(() => {
      if (onCopy) onCopy();
    });
  };

  if (!open) return null;

  return (
    <Modal open={open} title={t('versioning.compare_title')} onClose={onClose} size="lg">
      <div className="diff-modal">
        <div className="diff-modal__selectors">
          <div className="diff-modal__selector">
            <label id="diff-a-label">{t('versioning.version_a')}</label>
            <VersionSelect
              options={versions}
              value={aId}
              onChange={setAId}
              aria-labelledby="diff-a-label"
            />
          </div>
          <button
            type="button"
            className="btn-ghost btn-sm diff-modal__swap"
            onClick={handleSwap}
            aria-label={t('versioning.swap')}
          >
            ⇄ {t('versioning.swap')}
          </button>
          <div className="diff-modal__selector">
            <label id="diff-b-label">{t('versioning.version_b')}</label>
            <VersionSelect
              options={versions}
              value={bId}
              onChange={setBId}
              aria-labelledby="diff-b-label"
            />
          </div>
        </div>

        {failed ? (
          <div className="diff-modal__fallback">
            <p className="diff-modal__notice">{t('versioning.diff_fallback_notice')}</p>
            <div className="diff-modal__panes">
              <pre>{normA ? normA.content : ''}</pre>
              <pre>{normB ? normB.content : ''}</pre>
            </div>
          </div>
        ) : (
          <>
            <div className="diff-modal__stats" data-testid="diff-stats">
              {t('versioning.stats', { added: stats.added, removed: stats.removed })}
            </div>
            <div className="diff-modal__body">
              {rows.map((r, idx) => (
                <div
                  key={idx}
                  className={`diff-line diff-line--${r.type}`}
                  aria-label={
                    r.type === 'remove'
                      ? `${t('versioning.removed_line')}: ${r.text}`
                      : r.type === 'add'
                        ? `${t('versioning.added_line')}: ${r.text}`
                        : undefined
                  }
                >
                  <span className="diff-line__num">{r.aLine || ''}</span>
                  <span className="diff-line__num">{r.bLine || ''}</span>
                  <span className="diff-line__marker">{r.type === 'add' ? '+' : r.type === 'remove' ? '−' : ' '}</span>
                  <span className="diff-line__text">{r.text}</span>
                </div>
              ))}
            </div>
          </>
        )}

        <div className="diff-modal__footer">
          <button type="button" className="btn-ghost btn-sm" onClick={handleCopy} disabled={failed}>
            {t('versioning.copy_diff')}
          </button>
          <button type="button" className="btn-ghost btn-sm" onClick={onClose}>
            {t('common.close')}
          </button>
        </div>
      </div>
    </Modal>
  );
};

export default DiffModal;
