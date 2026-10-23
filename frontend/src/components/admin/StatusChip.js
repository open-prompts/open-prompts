import React from 'react';
import { useTranslation } from 'react-i18next';
import Badge from './Badge';
import './StatusChip.scss';

const STATUS_TONES = {
  normal: 'success',
  locked: 'warning',
  unpublished_by_admin: 'accent',
  soft_deleted: 'danger',
  in_review: 'neutral',
  rejected: 'danger',
};

/**
 * Colored chip per moderation_status. The label is localized; unknown
 * statuses fall back to the raw value.
 */
const StatusChip = ({ status }) => {
  const { t } = useTranslation();
  const tone = STATUS_TONES[status] || 'neutral';
  const label = t(`mod.status.${status}`, { defaultValue: status });
  return (
    <Badge tone={tone} dot>
      {label}
    </Badge>
  );
};

export default StatusChip;