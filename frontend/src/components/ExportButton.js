import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNotification } from '../context/NotificationContext';
import { getTemplate, listTemplateVersions } from '../services/api';
import { getAdminTemplate } from '../services/adminApi';
import Spinner from './ui/Spinner';
import {
  buildTemplateExport,
  downloadJSON,
  slugifyTitle,
  fetchVersionsChronological,
} from '../utils/templateExport';
import './ExportButton.scss';

/**
 * ExportButton downloads a template (with full version history) as a portable
 * .json file. Mounted on TemplateDetails (owner) and the admin template detail
 * page. The admin path flattens the admin response into the same field set.
 *
 * @param {Object} props
 * @param {Object} props.template - the template object (user or admin shape)
 * @param {string} [props.dataSource] - 'user' (default) | 'admin'
 */
const ExportButton = ({ template, dataSource = 'user' }) => {
  const { t } = useTranslation();
  const { addNotification } = useNotification();
  const [exporting, setExporting] = useState(false);

  const handleExport = async () => {
    setExporting(true);
    try {
      let flat;
      if (dataSource === 'admin') {
        const resp = await getAdminTemplate(template.id);
        const d = resp.data;
        flat = {
          title: d.template.title,
          description: d.description,
          visibility: d.template.visibility,
          type: d.template.type,
          tags: d.template.tags || [],
          category: d.template.category || '',
          language: d.template.language,
          versions: [...(d.versions || [])].reverse(),
        };
      } else {
        const [tplRes, versions] = await Promise.all([
          getTemplate(template.id),
          fetchVersionsChronological(template.id, listTemplateVersions),
        ]);
        const tpl = tplRes.data.template;
        flat = {
          title: tpl.title,
          description: tpl.description,
          visibility: tpl.visibility,
          type: tpl.type,
          tags: tpl.tags || [],
          category: tpl.category || '',
          language: tpl.language,
          versions,
        };
      }
      const text = buildTemplateExport(flat, flat.versions);
      downloadJSON(`${slugifyTitle(flat.title, template.id)}.json`, text);
      addNotification({
        kind: 'success',
        title: t('common.success'),
        subtitle: t('import_export.export_success', { title: flat.title, count: flat.versions.length }),
      });
    } catch (err) {
      console.error('Export failed:', err);
      addNotification({ kind: 'error', title: t('common.error'), subtitle: t('import_export.export_failed') });
    } finally {
      setExporting(false);
    }
  };

  return (
    <button
      type="button"
      className="btn-ghost btn-sm export-btn"
      onClick={handleExport}
      disabled={exporting}
    >
      {exporting ? (
        <Spinner size={14} />
      ) : (
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
          <polyline points="7 10 12 15 17 10" />
          <line x1="12" y1="15" x2="12" y2="3" />
        </svg>
      )}
      <span>{t('import_export.export')}</span>
    </button>
  );
};

export default ExportButton;