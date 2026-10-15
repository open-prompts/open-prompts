import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import ImportModal from './ImportModal';
import './ImportButton.scss';

/**
 * ImportButton opens the ImportModal. Mounted on Home's search row, rendered
 * only when the user is authenticated.
 */
const ImportButton = () => {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  return (
    <>
      <button
        type="button"
        className="btn-ghost btn-sm import-btn"
        onClick={() => setOpen(true)}
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
          <polyline points="17 8 12 3 7 8" />
          <line x1="12" y1="3" x2="12" y2="15" />
        </svg>
        <span>{t('import_export.import')}</span>
      </button>
      <ImportModal open={open} onRequestClose={() => setOpen(false)} />
    </>
  );
};

export default ImportButton;