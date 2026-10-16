import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { getRelatedTemplates } from '../../services/api';
import PromptCard from '../PromptCard';
import BasisLabel from './BasisLabel';
import './RelatedRail.scss';

/**
 * RelatedRail — a lazy-loaded "Related prompts" rail shown on public template
 * detail pages. Fetches after mount (never blocks first paint), hides
 * silently on error or when fewer than 3 candidates exist.
 *
 * @param {Object} props
 * @param {string} props.templateId
 * @param {string} props.category - anchor template's category
 * @param {string[]} props.tags - anchor template's tags
 */
const RelatedRail = ({ templateId, category, tags }) => {
  const { t } = useTranslation();
  const [items, setItems] = useState(null); // null = loading
  const [basis, setBasis] = useState('generic');

  useEffect(() => {
    let cancelled = false;
    setItems(null);
    getRelatedTemplates(templateId, 8)
      .then((resp) => {
        if (cancelled) return;
        const data = resp.data || {};
        setItems(data.items || []);
        setBasis(data.basis || 'generic');
      })
      .catch(() => {
        if (cancelled) return;
        setItems([]); // silent failure — rails never surface errors
      });
    return () => {
      cancelled = true;
    };
  }, [templateId]);

  // Hidden while loading, on error, or when fewer than 3 candidates.
  if (!items || items.length < 3) {
    return null;
  }

  // First shared tag of the top item drives the "tag" basis label.
  const top = items[0];
  const firstSharedTag = (top.tags || []).find((tg) => (tags || []).includes(tg));

  return (
    <section className="related-rail" aria-label={t('related.aria.rail')}>
      <h2 className="related-rail__title">
        <BasisLabel basis={basis} category={category} tag={firstSharedTag} />
      </h2>
      <div className="related-rail__grid">
        {items.map((item) => (
          <PromptCard key={item.id} template={item} />
        ))}
      </div>
    </section>
  );
};

export default RelatedRail;