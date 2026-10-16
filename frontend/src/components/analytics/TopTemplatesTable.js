import React from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import './TopTemplatesTable.scss';

/**
 * TopTemplatesTable — the creator's top templates by uses in the range.
 *
 * @param {Object} props
 * @param {Array} props.rows - [{ id, title, views, uses, likes, favorites, rate }]
 */
const TopTemplatesTable = ({ rows = [] }) => {
  const { t } = useTranslation();
  if (rows.length === 0) {
    return (
      <div className="top-templates">
        <h3>{t('analytics.top.title')}</h3>
        <p className="top-templates__empty">{t('analytics.top.empty')}</p>
      </div>
    );
  }
  return (
    <div className="top-templates">
      <h3>{t('analytics.top.title')}</h3>
      <table className="top-templates__table">
        <thead>
          <tr>
            <th scope="col">{t('analytics.top.title')}</th>
            <th scope="col">{t('analytics.top.views')}</th>
            <th scope="col">{t('analytics.top.uses')}</th>
            <th scope="col">{t('analytics.top.likes')}</th>
            <th scope="col">{t('analytics.top.favorites')}</th>
            <th scope="col">{t('analytics.top.rate')}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.id}>
              <td><Link to={`/templates/${row.id}`} className="top-templates__link">{row.title}</Link></td>
              <td>{row.views}</td>
              <td>{row.uses}</td>
              <td>{row.likes}</td>
              <td>{row.favorites}</td>
              <td className="top-templates__rate">{row.rate != null ? `${(row.rate * 100).toFixed(1)}%` : '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

export default TopTemplatesTable;