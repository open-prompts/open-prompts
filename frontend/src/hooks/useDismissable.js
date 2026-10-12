import { useEffect, useRef } from 'react';

/**
 * Hook for dismissable overlays (dropdowns, popovers).
 * Closes on outside mousedown, Escape key, and route change.
 *
 * @param {boolean} open - whether the overlay is currently open
 * @param {Function} onDismiss - called when a dismiss event occurs
 * @returns {Object} ref to attach to the wrapper element
 */
const useDismissable = (open, onDismiss) => {
  const ref = useRef(null);
  const onDismissRef = useRef(onDismiss);
  onDismissRef.current = onDismiss;

  useEffect(() => {
    if (!open) return undefined;

    const handleMouseDown = (event) => {
      if (ref.current && !ref.current.contains(event.target)) {
        onDismissRef.current();
      }
    };

    const handleKeyDown = (event) => {
      if (event.key === 'Escape') {
        event.stopPropagation();
        onDismissRef.current();
      }
    };

    document.addEventListener('mousedown', handleMouseDown);
    document.addEventListener('keydown', handleKeyDown, true);
    return () => {
      document.removeEventListener('mousedown', handleMouseDown);
      document.removeEventListener('keydown', handleKeyDown, true);
    };
  }, [open]);

  return ref;
};

export default useDismissable;
