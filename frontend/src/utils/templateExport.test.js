import {
  EXPORT_FORMAT,
  EXPORT_FORMAT_VERSION,
  MAX_IMPORT_VERSIONS,
  parseTemplateExport,
  buildTemplateExport,
  slugifyTitle,
  fetchVersionsChronological,
  ImportValidationError,
} from './templateExport';

const makeFile = (obj, name = 'export.json') => {
  const text = typeof obj === 'string' ? obj : JSON.stringify(obj);
  const file = new File([text], name, { type: 'application/json' });
  // jsdom's File lacks .text(); polyfill it for the parser.
  if (typeof file.text !== 'function') {
    file.text = () => Promise.resolve(text);
  }
  return file;
};

const validExport = () => ({
  format: EXPORT_FORMAT,
  format_version: EXPORT_FORMAT_VERSION,
  exported_at: '2026-10-10T12:00:00Z',
  source: 'open-prompts',
  template: {
    title: 'My Prompt Template',
    description: 'A description',
    visibility: 'public',
    type: 'system',
    tags: ['prompt', 'writing'],
    category: 'Writing',
    language: 'en',
    versions: [
      { version: 1, content: 'first', change_note: null, created_at: '2026-01-01T00:00:00Z' },
      { version: 2, content: 'second', change_note: 'revised', created_at: '2026-01-02T00:00:00Z' },
    ],
  },
});

describe('parseTemplateExport', () => {
  test('round-trips a valid export preserving all fields', async () => {
    const { data, warnings } = await parseTemplateExport(makeFile(validExport()));
    expect(warnings).toEqual([]);
    expect(data.format).toBe(EXPORT_FORMAT);
    expect(data.format_version).toBe(EXPORT_FORMAT_VERSION);
    expect(data.template.title).toBe('My Prompt Template');
    expect(data.template.description).toBe('A description');
    expect(data.template.tags).toEqual(['prompt', 'writing']);
    expect(data.template.category).toBe('Writing');
    expect(data.template.language).toBe('en');
    expect(data.template.visibility).toBe('public');
    expect(data.template.type).toBe('system');
    expect(data.template.versions).toHaveLength(2);
    expect(data.template.versions[0].version).toBe(1);
    expect(data.template.versions[1].version).toBe(2);
    expect(data.template.versions[0].change_note).toBe('');
    expect(data.template.versions[1].change_note).toBe('revised');
  });

  test('too_large when file exceeds 1 MB', async () => {
    const big = makeFile('x'.repeat(1024 * 1024 + 1), 'big.json');
    await expect(parseTemplateExport(big)).rejects.toMatchObject({ code: 'too_large' });
  });

  test('not_json for unparseable content', async () => {
    await expect(parseTemplateExport(makeFile('not json at all'))).rejects.toMatchObject({ code: 'not_json' });
  });

  test('bad_format for wrong format or missing template', async () => {
    await expect(parseTemplateExport(makeFile({ format: 'other', template: {} }))).rejects.toMatchObject({ code: 'bad_format' });
    await expect(parseTemplateExport(makeFile({ format: EXPORT_FORMAT }))).rejects.toMatchObject({ code: 'bad_format' });
  });

  test('unsupported_version for a newer format version', async () => {
    const ex = validExport();
    ex.format_version = 99;
    await expect(parseTemplateExport(makeFile(ex))).rejects.toMatchObject({
      code: 'unsupported_version',
      params: { version: 99 },
    });
  });

  test('missing_field:title when title absent/empty', async () => {
    const ex = validExport();
    ex.template.title = '';
    await expect(parseTemplateExport(makeFile(ex))).rejects.toMatchObject({ code: 'missing_field:title' });
  });

  test('missing_field:versions when versions absent/empty', async () => {
    const ex = validExport();
    ex.template.versions = [];
    await expect(parseTemplateExport(makeFile(ex))).rejects.toMatchObject({ code: 'missing_field:versions' });
  });

  test('missing_field:versions[i].content when a version has no content', async () => {
    const ex = validExport();
    ex.template.versions[1].content = '';
    await expect(parseTemplateExport(makeFile(ex))).rejects.toMatchObject({ code: 'missing_field:versions[1].content' });
  });

  test('invalid_field:title when title too long', async () => {
    const ex = validExport();
    ex.template.title = 'x'.repeat(201);
    await expect(parseTemplateExport(makeFile(ex))).rejects.toMatchObject({ code: 'invalid_field:title' });
  });

  test('invalid_field:tags for too many tags or too-long tag', async () => {
    const ex = validExport();
    ex.template.tags = Array.from({ length: 21 }, (_, i) => `tag${i}`);
    await expect(parseTemplateExport(makeFile(ex))).rejects.toMatchObject({ code: 'invalid_field:tags' });

    const ex2 = validExport();
    ex2.template.tags = ['x'.repeat(51)];
    await expect(parseTemplateExport(makeFile(ex2))).rejects.toMatchObject({ code: 'invalid_field:tags' });
  });

  test('truncates >50 versions to the most recent 50 with a warning', async () => {
    const ex = validExport();
    ex.template.versions = Array.from({ length: 60 }, (_, i) => ({
      version: i + 1,
      content: `content ${i + 1}`,
      change_note: null,
    }));
    const { data, warnings } = await parseTemplateExport(makeFile(ex));
    expect(data.template.versions).toHaveLength(MAX_IMPORT_VERSIONS);
    expect(data.template.versions[0].version).toBe(11); // kept = last 50 (v11..v60)
    expect(data.template.versions[data.template.versions.length - 1].version).toBe(60);
    expect(warnings).toEqual([{ code: 'truncated_versions', params: { kept: 50, total: 60 } }]);
  });

  test('normalizes description/tags/category/language defaults', async () => {
    const ex = validExport();
    delete ex.template.description;
    delete ex.template.tags;
    delete ex.template.category;
    delete ex.template.language;
    const { data } = await parseTemplateExport(makeFile(ex));
    expect(data.template.description).toBe('');
    expect(data.template.tags).toEqual([]);
    expect(data.template.category).toBe('');
    expect(data.template.language).toBe('en');
  });

  test('deduplicates tags and sorts versions chronologically', async () => {
    const ex = validExport();
    ex.template.tags = ['prompt', 'prompt', 'writing'];
    ex.template.versions = [
      { version: 2, content: 'second', change_note: null },
      { version: 1, content: 'first', change_note: null },
    ];
    const { data } = await parseTemplateExport(makeFile(ex));
    expect(data.template.tags).toEqual(['prompt', 'writing']);
    expect(data.template.versions.map((v) => v.version)).toEqual([1, 2]);
  });
});

describe('buildTemplateExport', () => {
  test('produces a parseable format-version-1 file', async () => {
    const template = {
      title: 'My Prompt Template',
      description: 'A description',
      visibility: 'public',
      type: 'user',
      tags: ['prompt'],
      category: 'Writing',
      language: 'en',
    };
    const versions = [
      { version: 1, content: 'first', change_note: null, created_at: '2026-01-01T00:00:00Z' },
      { version: 2, content: 'second', change_note: 'revised', created_at: '2026-01-02T00:00:00Z' },
    ];
    const text = buildTemplateExport(template, versions);
    expect(text.endsWith('\n')).toBe(true);
    const { data, warnings } = await parseTemplateExport(makeFile(text));
    expect(warnings).toEqual([]);
    expect(data.template.title).toBe('My Prompt Template');
    expect(data.template.versions).toHaveLength(2);
    expect(data.template.versions[0].change_note).toBe('');
    expect(data.template.versions[1].change_note).toBe('revised');
  });

  test('omits owner identity entirely', () => {
    const text = buildTemplateExport({ title: 'T', tags: [] }, [{ version: 1, content: 'c' }]);
    expect(text).not.toMatch(/owner|email|display_name|user_id/i);
  });
});

describe('slugifyTitle', () => {
  test('slugifies a normal title with id suffix', () => {
    expect(slugifyTitle('My Prompt Template', 'a1b2c3d4-e5f6')).toBe('my-prompt-template-a1b2c3d4');
  });

  test('caps at 60 chars cutting at a dash boundary', () => {
    const long = 'word '.repeat(30).trim(); // 149 chars
    const slug = slugifyTitle(long, 'id');
    expect(slug.length).toBeLessThanOrEqual(60 + 1 + 8);
    expect(slug.endsWith('-id')).toBe(true);
  });

  test('falls back to template-{id8} for all-CJK titles', () => {
    expect(slugifyTitle('中文模板标题', 'a1b2c3d4')).toBe('template-a1b2c3d4');
  });

  test('empty title falls back to template-{id8}', () => {
    expect(slugifyTitle('', 'id')).toBe('template-id');
  });
});

describe('fetchVersionsChronological', () => {
  test('follows next_page_token and reverses to chronological', async () => {
    const listFn = jest.fn((id, params) => {
      if (params.page_token === '') {
        return Promise.resolve({ data: { versions: [{ version: 3 }, { version: 2 }], next_page_token: 'p2' } });
      }
      return Promise.resolve({ data: { versions: [{ version: 1 }], next_page_token: '' } });
    });
    const versions = await fetchVersionsChronological('t1', listFn);
    expect(versions.map((v) => v.version)).toEqual([1, 2, 3]);
    expect(listFn).toHaveBeenCalledTimes(2);
  });
});

describe('ImportValidationError', () => {
  test('carries code and params', () => {
    const err = new ImportValidationError('missing_field:title', { field: 'title' });
    expect(err).toBeInstanceOf(Error);
    expect(err.code).toBe('missing_field:title');
    expect(err.params).toEqual({ field: 'title' });
  });
});