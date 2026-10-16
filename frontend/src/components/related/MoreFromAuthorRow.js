import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { getMoreFromAuthor } from '../../services/api';
import PromptCard from '../PromptCard';
import './MoreFromAuthorRow.scss';

/**
 * MoreFromAuthorRow — a lazy-loaded "More from {author}" row shown on public
 * template detail pages. Fetches after mount, hides silently on error or when
 * the author has no other public templates.
 *
 * @param {Object} props
 * @param {string} props.templateId
 * @param {string} props.authorName - the anchor template's owner display name
 */
const MoreFromAuthorRow = ({ templateId, authorName }) => {
  const { t } = useTranslation();
  const [items, setItems] = useState(null); // null = loading

  useEffect(() => {
    let cancelled = false;
    setItems(null);
    getMoreFromAuthor(templateId, 4)
      .then((resp) => {
        if (cancelled) return;
        setItems(resp.data?.items || []);
      })
      .catch(() => {
        if (cancelled) return;
        setItems([]); // silent failure
      });
    return () => {
      cancelled = true;
    };
  }, [templateId]);

  if (!items || items.length === 0) {
    return null;
  }

  return (
    <section className="more-from-author" aria-label={t('related.aria.authorRow')}>
      <h2 className="more-from-author__title">
        {t('related.moreFromAuthor', { author: authorName })}
      </h2>
      <div className="more-from-author__grid">
        {items.map((item) => (
          <PromptCard key={item.id} template={item} />
        ))}
      </div>
    </section>
  );
};

export default MoreFromAuthorRow;