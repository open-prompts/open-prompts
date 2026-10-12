/**
 * Hand-rolled LCS line diff (no external dependencies).
 * Content is capped at 32 KB server-side, so line counts are small and
 * O(n·m) on lines is trivial.
 *
 * @param {string} aText - the older text
 * @param {string} bText - the newer text
 * @returns {Array<{type: 'add'|'remove'|'context', text: string, aLine: number|null, bLine: number|null}>}
 */
export const diffLines = (aText, bText) => {
  const a = String(aText == null ? '' : aText).split('\n');
  const b = String(bText == null ? '' : bText).split('\n');
  const n = a.length;
  const m = b.length;

  // LCS length table, (n+1) x (m+1), initialized to 0.
  const table = Array.from({ length: n + 1 }, () => new Array(m + 1).fill(0));
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      table[i][j] = a[i] === b[j]
        ? table[i + 1][j + 1] + 1
        : Math.max(table[i + 1][j], table[i][j + 1]);
    }
  }

  // Walk the table emitting removes (with aLine), adds (with bLine), context (both).
  const rows = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      rows.push({ type: 'context', text: a[i], aLine: i + 1, bLine: j + 1 });
      i++;
      j++;
    } else if (table[i + 1][j] >= table[i][j + 1]) {
      rows.push({ type: 'remove', text: a[i], aLine: i + 1, bLine: null });
      i++;
    } else {
      rows.push({ type: 'add', text: b[j], aLine: null, bLine: j + 1 });
      j++;
    }
  }
  while (i < n) {
    rows.push({ type: 'remove', text: a[i], aLine: i + 1, bLine: null });
    i++;
  }
  while (j < m) {
    rows.push({ type: 'add', text: b[j], aLine: null, bLine: j + 1 });
    j++;
  }
  return rows;
};

/**
 * Count added/removed lines in a diff result.
 * @param {Array} rows - output of diffLines
 * @returns {{added: number, removed: number}}
 */
export const diffStats = (rows) => {
  let added = 0;
  let removed = 0;
  (rows || []).forEach((r) => {
    if (r.type === 'add') added++;
    else if (r.type === 'remove') removed++;
  });
  return { added, removed };
};

/**
 * Render a diff as a unified plain-text patch (for copy-to-clipboard).
 * @param {Array} rows - output of diffLines
 * @returns {string}
 */
export const formatDiffText = (rows) => (rows || [])
  .map((r) => (r.type === 'add' ? `+${r.text}` : r.type === 'remove' ? `-${r.text}` : ` ${r.text}`))
  .join('\n');

export default diffLines;
