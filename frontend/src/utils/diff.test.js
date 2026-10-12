import { diffLines, diffStats, formatDiffText } from './diff';

describe('diffLines', () => {
  test('identical texts produce only context rows', () => {
    const rows = diffLines('a\nb\nc', 'a\nb\nc');
    expect(rows).toHaveLength(3);
    expect(rows.every((r) => r.type === 'context')).toBe(true);
    expect(rows[0]).toEqual({ type: 'context', text: 'a', aLine: 1, bLine: 1 });
  });

  test('empty to text produces only adds', () => {
    const rows = diffLines('', 'x\ny');
    // '' splits to [''], 'x\ny' splits to ['x','y'] -> '' removed, x/y added
    expect(rows.some((r) => r.type === 'add' && r.text === 'x')).toBe(true);
    expect(rows.some((r) => r.type === 'add' && r.text === 'y')).toBe(true);
  });

  test('text to empty produces only removes', () => {
    const rows = diffLines('x\ny', '');
    expect(rows.some((r) => r.type === 'remove' && r.text === 'x')).toBe(true);
    expect(rows.some((r) => r.type === 'remove' && r.text === 'y')).toBe(true);
  });

  test('both empty produce single context row', () => {
    const rows = diffLines('', '');
    expect(rows).toHaveLength(1);
    expect(rows[0].type).toBe('context');
  });

  test('mixed add/remove/context with line numbers', () => {
    const rows = diffLines('line1\nline2\nline3', 'line1\nchanged\nline3\nline4');
    const types = rows.map((r) => r.type);
    expect(types).toContain('context');
    expect(types).toContain('remove');
    expect(types).toContain('add');

    const removed = rows.find((r) => r.type === 'remove');
    expect(removed.text).toBe('line2');
    expect(removed.aLine).toBe(2);
    expect(removed.bLine).toBeNull();

    const added = rows.find((r) => r.type === 'add' && r.text === 'changed');
    expect(added.bLine).toBe(2);
    expect(added.aLine).toBeNull();

    const added4 = rows.find((r) => r.type === 'add' && r.text === 'line4');
    expect(added4.bLine).toBe(4);
  });

  test('null/undefined inputs are treated as empty', () => {
    expect(() => diffLines(null, undefined)).not.toThrow();
    const rows = diffLines(null, undefined);
    expect(rows).toHaveLength(1);
  });

  test('preserves order: removes before adds for replaced block', () => {
    const rows = diffLines('a\nb', 'a\nc');
    expect(rows.map((r) => `${r.type}:${r.text}`)).toEqual(['context:a', 'remove:b', 'add:c']);
  });
});

describe('diffStats', () => {
  test('counts added and removed', () => {
    const rows = diffLines('a\nb\nc', 'a\nx\nc\nd');
    const stats = diffStats(rows);
    expect(stats.added).toBe(2);
    expect(stats.removed).toBe(1);
  });

  test('zero for identical', () => {
    expect(diffStats(diffLines('a', 'a'))).toEqual({ added: 0, removed: 0 });
  });

  test('handles null input', () => {
    expect(diffStats(null)).toEqual({ added: 0, removed: 0 });
  });
});

describe('formatDiffText', () => {
  test('renders unified patch markers', () => {
    const rows = diffLines('a\nb', 'a\nc');
    expect(formatDiffText(rows)).toBe(' a\n-b\n+c');
  });

  test('handles null input', () => {
    expect(formatDiffText(null)).toBe('');
  });
});
