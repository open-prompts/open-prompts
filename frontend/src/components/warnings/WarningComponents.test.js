import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import WarningCheckboxGroup from './WarningCheckboxGroup';
import ContentInterstitial, { isWarnedRevealed } from './ContentInterstitial';
import WarningChip from './WarningChip';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key, opts) => {
      // Minimal resolver for the keys used in these components.
      const map = {
        'warnings.group.title': 'Content warnings (optional)',
        'warnings.group.help': 'Help viewers know what to expect before opening.',
        'warnings.type.mature': 'Mature themes',
        'warnings.type.violence': 'Graphic violence',
        'warnings.type.profanity': 'Strong language',
        'warnings.type.sensitive': 'Sensitive topics',
        'warnings.locked.label': 'Applied by moderation (locked)',
        'warnings.chip.multiple': '{{first}} +{{count}} more',
        'warnings.interstitial.title': 'Content warning',
        'warnings.interstitial.body': 'This template covers: {{reasons}}',
        'warnings.interstitial.subtext': 'You can view it — we just want you to know first.',
        'warnings.interstitial.show': 'Show content',
        'warnings.interstitial.back': 'Go back',
      };
      if (map[key]) {
        let out = map[key];
        if (opts) {
          Object.entries(opts).forEach(([k, v]) => {
            out = out.replace(`{{${k}}}`, String(v));
          });
        }
        return out;
      }
      return key;
    },
  }),
}));

describe('WarningCheckboxGroup', () => {
  it('renders taxonomy options and toggles selection', () => {
    const onChange = jest.fn();
    render(<WarningCheckboxGroup value={['mature']} onChange={onChange} />);
    const mature = screen.getByLabelText('Mature themes');
    expect(mature.checked).toBe(true);
    fireEvent.click(mature);
    expect(onChange).toHaveBeenCalledWith([]);
  });

  it('disables locked warnings', () => {
    const onChange = jest.fn();
    render(<WarningCheckboxGroup value={['violence']} locked={['violence']} onChange={onChange} />);
    const violence = screen.getByLabelText('Graphic violence');
    expect(violence.disabled).toBe(true);
    fireEvent.click(violence);
    expect(onChange).not.toHaveBeenCalled();
  });
});

describe('WarningChip', () => {
  it('renders a single warning name', () => {
    render(<WarningChip warnings={['mature']} />);
    expect(screen.getByText('Mature themes')).toBeInTheDocument();
  });

  it('renders multiple with +N more', () => {
    render(<WarningChip warnings={['mature', 'violence']} />);
    expect(screen.getByText(/\+1 more/)).toBeInTheDocument();
  });

  it('renders nothing when empty', () => {
    render(<WarningChip warnings={[]} />);
    expect(screen.queryByText(/Mature themes/)).not.toBeInTheDocument();
  });
});

describe('ContentInterstitial', () => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  it('shows reasons and reveal/back buttons', () => {
    const onReveal = jest.fn();
    const onBack = jest.fn();
    render(
      <ContentInterstitial templateId="t1" warnings={['mature']} onReveal={onReveal} onBack={onBack} />
    );
    expect(screen.getByText('Content warning')).toBeInTheDocument();
    expect(screen.getByText(/Mature themes/)).toBeInTheDocument();
    fireEvent.click(screen.getByText('Show content'));
    expect(onReveal).toHaveBeenCalled();
    expect(isWarnedRevealed('t1')).toBe(true);
  });

  it('Escape triggers back', () => {
    const onBack = jest.fn();
    render(
      <ContentInterstitial templateId="t1" warnings={['mature']} onReveal={() => {}} onBack={onBack} />
    );
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' });
    expect(onBack).toHaveBeenCalled();
  });
});