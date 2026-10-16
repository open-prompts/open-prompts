import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import './ReorderList.scss';

/**
 * ReorderList — rows with up/down buttons + explicit position input; local
 * order state with a Save/Discard bar.
 *
 * @param {Object} props
 * @param {Array} props.members - CollectionMember[] (template + position)
 * @param {Function} props.onSave - Called with the ordered template-id array
 * @param {Function} props.onDiscard - Called to exit edit mode
 */
const ReorderList = ({ members, onSave, onDiscard }) => {
  const { t } = useTranslation();
  const [order, setOrder] = useState(() => members.map((m) => m.template.id));
  const dirty = JSON.stringify(order) !== JSON.stringify(members.map((m) => m.template.id));

  const move = (index, delta) => {
    const next = [...order];
    const target = index + delta;
    if (target < 0 || target >= next.length) return;
    [next[index], next[target]] = [next[target], next[index]];
    setOrder(next);
  };

  const setPosition = (index, value) => {
    const pos = Number(value);
    if (Number.isNaN(pos) || pos < 0 || pos >= order.length) return;
    const next = [...order];
    const [item] = next.splice(index, 1);
    next.splice(pos, 0, item);
    setOrder(next);
  };

  return (
    <div className="reorder-list">
      <ul className="reorder-list__rows">
        {order.map((id, index) => {
          const member = members.find((m) => m.template.id === id);
          return (
            <li key={id} className="reorder-list__row">
              <span className="reorder-list__index">{index + 1}</span>
              <span className="reorder-list__title">{member ? member.template.title : id}</span>
              <div className="reorder-list__controls">
                <button
                  type="button"
                  className="reorder-list__btn"
                  onClick={() => move(index, -1)}
                  disabled={index === 0}
                  aria-label={t('collections.reorder')}
                >
                  ↑
                </button>
                <button
                  type="button"
                  className="reorder-list__btn"
                  onClick={() => move(index, 1)}
                  disabled={index === order.length - 1}
                  aria-label={t('collections.reorder')}
                >
                  ↓
                </button>
                <input
                  type="number"
                  className="reorder-list__pos"
                  value={index + 1}
                  min={1}
                  max={order.length}
                  onChange={(e) => setPosition(index, e.target.value)}
                  aria-label={t('collections.reorder')}
                />
              </div>
            </li>
          );
        })}
      </ul>
      <div className="reorder-list__bar">
        <button
          type="button"
          className="btn btn-primary"
          disabled={!dirty}
          onClick={() => onSave(order)}
        >
          {t('collections.picker.save')}
        </button>
        <button type="button" className="btn btn-ghost" onClick={onDiscard}>
          {t('collections.picker.cancel')}
        </button>
      </div>
    </div>
  );
};

export default ReorderList;