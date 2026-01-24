import React, { useState, useRef, useEffect } from 'react';

const VersionSelect = ({ options = [], value, onChange, disabled = false, renderLabel }) => {
  const [open, setOpen] = useState(false);
  const [highlighted, setHighlighted] = useState(null);
  const rootRef = useRef(null);
  const listRef = useRef(null);

  useEffect(() => {
    const onDocClick = (e) => {
      if (rootRef.current && !rootRef.current.contains(e.target)) setOpen(false);
    };
    document.addEventListener('click', onDocClick);
    return () => document.removeEventListener('click', onDocClick);
  }, []);

  useEffect(() => {
    if (open && listRef.current) {
      const idx = options.findIndex((o) => o.id === value);
      setHighlighted(idx >= 0 ? idx : 0);
    }
  }, [open, options, value]);

  const toggle = () => { if (!disabled) setOpen((s) => !s); };

  const selectIndex = (idx) => {
    const opt = options[idx];
    if (opt) onChange(opt.id);
    setOpen(false);
  };

  const onKeyDown = (e) => {
    if (disabled) return;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setOpen(true);
      setHighlighted((h) => {
        const next = h == null ? 0 : Math.min(options.length - 1, h + 1);
        return next;
      });
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setOpen(true);
      setHighlighted((h) => {
        const prev = h == null ? options.length - 1 : Math.max(0, h - 1);
        return prev;
      });
    } else if (e.key === 'Enter') {
      e.preventDefault();
      if (open && highlighted != null) selectIndex(highlighted);
      else setOpen(true);
    } else if (e.key === 'Escape') {
      setOpen(false);
    } else if (e.key === 'Home') {
      e.preventDefault();
      setHighlighted(0);
    } else if (e.key === 'End') {
      e.preventDefault();
      setHighlighted(options.length - 1);
    }
  };

  const label = () => {
    const sel = options.find((o) => o.id === value);
    if (renderLabel) return renderLabel(sel);
    if (!sel) return '';
    const date = sel.created_at ? new Date(sel.created_at).toLocaleDateString() : '';
    return `v${sel.version} (${date})`;
  };

  return (
    <div className={`custom-select ${disabled ? 'disabled' : ''}`} ref={rootRef}>
      <button
        type="button"
        className="custom-select__toggle"
        aria-haspopup="listbox"
        aria-expanded={open}
        onClick={toggle}
        onKeyDown={onKeyDown}
        disabled={disabled}
      >
        <span className="custom-select__label">{label()}</span>
        <span className="custom-select__caret" aria-hidden>▾</span>
      </button>

      {open && (
        <ul
          className="custom-select__menu"
          role="listbox"
          ref={listRef}
          tabIndex={-1}
          onKeyDown={onKeyDown}
        >
          {options.map((opt, idx) => (
            <li
              key={opt.id}
              role="option"
              aria-selected={opt.id === value}
              className={`custom-select__option ${highlighted === idx ? 'highlighted' : ''} ${opt.id === value ? 'selected' : ''}`}
              onMouseEnter={() => setHighlighted(idx)}
              onClick={() => selectIndex(idx)}
            >
              <div className="option-main">v{opt.version}</div>
              <div className="option-sub">{opt.created_at ? new Date(opt.created_at).toLocaleString() : ''}</div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
};

export default VersionSelect;
