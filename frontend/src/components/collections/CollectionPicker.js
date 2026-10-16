import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import Modal from '../ui/Modal';
import {
  listMyCollections,
  listCollectionsContainingTemplate,
  addTemplateToCollection,
  removeTemplateFromCollection,
  createCollection,
} from '../../services/api';
import './CollectionPicker.scss';

/**
 * CollectionPicker — modal listing the caller's collections with checkmarks
 * from ListCollectionsContainingTemplate. Staging semantics: checking adds,
 * unchecking removes, commit on Save. Supports inline create and filtering.
 *
 * @param {Object} props
 * @param {Object} props.template - The template being added
 * @param {Function} props.onClose - Close the modal
 */
const CollectionPicker = ({ template, onClose }) => {
  const { t } = useTranslation();
  const [myCollections, setMyCollections] = useState([]);
  const [containing, setContaining] = useState(new Set());
  const [stagedAdd, setStagedAdd] = useState(new Set());
  const [stagedRemove, setStagedRemove] = useState(new Set());
  const [filter, setFilter] = useState('');
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState('');
  const [error, setError] = useState(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let cancelled = false;
    Promise.all([listMyCollections(), listCollectionsContainingTemplate(template.id)])
      .then(([mine, containingResp]) => {
        if (cancelled) return;
        setMyCollections(mine.data.collections || []);
        setContaining(new Set((containingResp.data.collections || []).map((c) => c.id)));
      })
      .catch(() => {
        if (!cancelled) setError(t('collections.error.load'));
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [template.id]);

  const isChecked = (id) => {
    if (stagedRemove.has(id)) return false;
    if (stagedAdd.has(id)) return true;
    return containing.has(id);
  };

  const toggle = (id) => {
    if (isChecked(id)) {
      if (stagedAdd.has(id)) {
        const next = new Set(stagedAdd);
        next.delete(id);
        setStagedAdd(next);
      } else {
        setStagedRemove((prev) => new Set(prev).add(id));
      }
    } else {
      if (stagedRemove.has(id)) {
        const next = new Set(stagedRemove);
        next.delete(id);
        setStagedRemove(next);
      } else {
        setStagedAdd((prev) => new Set(prev).add(id));
      }
    }
  };

  const handleCreate = async () => {
    const name = newName.trim();
    if (!name) return;
    setCreating(true);
    setError(null);
    try {
      const resp = await createCollection({ name });
      const col = resp.data;
      setMyCollections((prev) => [col, ...prev]);
      setStagedAdd((prev) => new Set(prev).add(col.id));
      setNewName('');
    } catch (err) {
      const status = err.response && err.response.status;
      if (status === 429) setError(t('collections.picker.limit_reached'));
      else setError(t('collections.error.save'));
    } finally {
      setCreating(false);
    }
  };

  const handleSave = async () => {
    setSaving(true);
    setError(null);
    // Sequential; first error shown inline, already-applied changes kept.
    for (const id of stagedAdd) {
      try {
        await addTemplateToCollection(id, template.id);
      } catch (err) {
        const status = err.response && err.response.status;
        if (status === 429) {
          const col = myCollections.find((c) => c.id === id);
          setError(t('collections.picker.member_limit_reached', { name: col ? col.name : '' }));
        } else {
          setError(t('collections.error.save'));
        }
        setSaving(false);
        return;
      }
    }
    for (const id of stagedRemove) {
      try {
        await removeTemplateFromCollection(id, template.id);
      } catch (err) {
        setError(t('collections.error.save'));
        setSaving(false);
        return;
      }
    }
    setSaving(false);
    onClose();
  };

  const filtered = myCollections.filter((c) =>
    c.name.toLowerCase().includes(filter.toLowerCase())
  );

  return (
    <Modal
      open
      title={t('collections.picker.title')}
      subtitle={t('collections.picker.subtitle')}
      primaryLabel={t('collections.picker.save')}
      secondaryLabel={t('collections.picker.cancel')}
      primaryDisabled={saving || (stagedAdd.size === 0 && stagedRemove.size === 0)}
      onPrimary={handleSave}
      onClose={onClose}
    >
      <div className="collection-picker">
        <input
          type="text"
          className="collection-picker__filter"
          placeholder={t('collections.picker.filter_placeholder')}
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
        <ul className="collection-picker__list">
          {filtered.map((c) => (
            <li key={c.id} className="collection-picker__item">
              <label className="collection-picker__label">
                <input
                  type="checkbox"
                  checked={isChecked(c.id)}
                  onChange={() => toggle(c.id)}
                />
                <span className="collection-picker__name">{c.name}</span>
                <span className="collection-picker__count">
                  {t('collections.items', { count: c.template_count || 0 })}
                </span>
              </label>
            </li>
          ))}
          {filtered.length === 0 && (
            <li className="collection-picker__empty">{t('collections.empty.mine')}</li>
          )}
        </ul>
        <div className="collection-picker__create">
          <input
            type="text"
            className="collection-picker__new-name"
            placeholder={t('collections.picker.create_new')}
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
          />
          <button
            type="button"
            className="btn btn-ghost"
            onClick={handleCreate}
            disabled={creating || !newName.trim()}
          >
            {t('collections.new')}
          </button>
        </div>
        {error && <p className="collection-picker__error">{error}</p>}
      </div>
    </Modal>
  );
};

export default CollectionPicker;