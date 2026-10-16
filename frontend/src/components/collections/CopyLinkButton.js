import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CopyIcon } from '../ui/Icons';
import './CopyLinkButton.scss';

/**
 * CopyLinkButton — copies the current collection URL to the clipboard with an
 * execCommand fallback for non-secure contexts. Shows "Copied!" feedback.
 *
 * @param {Object} props
 * @param {string} props.url - The URL to copy
 */
const CopyLinkButton = ({ url }) => {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(url);
      } else {
        const ta = document.createElement('textarea');
        ta.value = url;
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
      }
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      // Clipboard unavailable — ignore silently.
    }
  };

  return (
    <button type="button" className="copy-link-button" onClick={handleCopy}>
      <CopyIcon size={15} />
      <span>{copied ? t('collections.link_copied') : t('collections.copy_link')}</span>
    </button>
  );
};

export default CopyLinkButton;