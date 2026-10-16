import React from 'react';
import { useTranslation } from 'react-i18next';

/**
 * BasisLabel renders the "Related · {category/tag}" heading for the related
 * rail, based on the server-derived basis ("category" | "tag" | "generic").
 * Category/tag names are proper nouns and are never translated.
 *
 * @param {Object} props
 * @param {string} props.basis - "category" | "tag" | "generic"
 * @param {string} props.category - the anchor template's category
 * @param {string} props.tag - the first shared tag (for the "tag" basis)
 */
const BasisLabel = ({ basis, category, tag }) => {
  const { t } = useTranslation();
  if (basis === 'category' && category) {
    return <>{t('related.title.category', { category })}</>;
  }
  if (basis === 'tag' && tag) {
    return <>{t('related.title.tag', { tag })}</>;
  }
  return <>{t('related.title.generic')}</>;
};

export default BasisLabel;