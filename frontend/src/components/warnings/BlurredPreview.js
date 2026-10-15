import React from 'react';
import WarningChip from './WarningChip';
import './BlurredPreview.scss';

/**
 * BlurredPreview — blurs a card's preview area and overlays a warning chip.
 * The title stays outside the blur (readable). Used on PromptCard when a
 * template has warnings and the viewer hasn't opted out / revealed.
 *
 * @param {Object} props
 * @param {string[]} props.warnings - taxonomy keys
 * @param {React.ReactNode} props.children - the preview content to blur
 */
const BlurredPreview = ({ warnings = [], children }) => (
  <div className="blurred-preview">
    <div className="blurred-preview__blur" aria-hidden="true">
      {children}
    </div>
    <div className="blurred-preview__chip">
      <WarningChip warnings={warnings} />
    </div>
  </div>
);

export default BlurredPreview;