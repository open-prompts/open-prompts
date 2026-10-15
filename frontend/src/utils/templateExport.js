/**
 * Template import/export utilities (pure functions, no React imports).
 * Export assembles a portable .json file from the existing getTemplate /
 * listTemplateVersions responses; import parses and validates such a file
 * client-side before replaying it through createTemplate/updateTemplate.
 */

export const EXPORT_FORMAT = 'open-prompts-template';
export const EXPORT_FORMAT_VERSION = 1;
export const MAX_IMPORT_FILE_SIZE = 1024 * 1024; // 1 MB
export const MAX_IMPORT_VERSIONS = 50;
export const MAX_CHANGE_NOTE_LENGTH = 200;
export const MAX_TITLE_LENGTH = 200;
export const MAX_TAGS = 20;
export const MAX_TAG_LENGTH = 50;

/**
 * Typed validation error thrown by parseTemplateExport.
 * `code` is a stable machine key (e.g. "too_large", "missing_field:title");
 * `params` carries interpolation values for the i18n string.
 */
export class ImportValidationError extends Error {
  constructor(code, params) {
    super(code);
    this.name = 'ImportValidationError';
    this.code = code;
    this.params = params || {};
  }
}

const isNonEmptyString = (v) => typeof v === 'string' && v.trim().length > 0;

/**
 * Parse and validate an open-prompts template export file.
 * @param {File} file - the selected/dropped file
 * @returns {Promise<{data: Object, warnings: Array<{code: string, params: Object}>}>}
 *   `data` is the validated export object (versions chronological, possibly
 *   truncated to MAX_IMPORT_VERSIONS); `warnings` lists non-blocking issues.
 * @throws {ImportValidationError} on any FR-11 violation.
 */
export const parseTemplateExport = async (file) => {
  // Step 1: size cap, checked before parsing.
  if (file.size > MAX_IMPORT_FILE_SIZE) {
    throw new ImportValidationError('too_large');
  }

  // Step 2: must be parseable JSON.
  let data;
  try {
    data = JSON.parse(await file.text());
  } catch (e) {
    throw new ImportValidationError('not_json');
  }

  // Step 3: format + template object.
  if (data.format !== EXPORT_FORMAT || !data.template || typeof data.template !== 'object') {
    throw new ImportValidationError('bad_format');
  }

  // Step 4: format version.
  if (data.format_version !== EXPORT_FORMAT_VERSION) {
    throw new ImportValidationError('unsupported_version', { version: data.format_version });
  }

  const template = data.template;

  // Step 5: title.
  if (!isNonEmptyString(template.title)) {
    throw new ImportValidationError('missing_field:title');
  }

  // Step 6: versions array, at least one.
  if (!Array.isArray(template.versions) || template.versions.length < 1) {
    throw new ImportValidationError('missing_field:versions');
  }

  // Step 7: every version has non-empty content.
  for (let i = 0; i < template.versions.length; i++) {
    if (!isNonEmptyString(template.versions[i].content)) {
      throw new ImportValidationError(`missing_field:versions[${i}].content`);
    }
  }

  // Step 8: title length.
  if (template.title.length > MAX_TITLE_LENGTH) {
    throw new ImportValidationError('invalid_field:title');
  }

  // Step 9: tags count/length.
  if (template.tags != null) {
    if (!Array.isArray(template.tags)) {
      throw new ImportValidationError('invalid_field:tags');
    }
    if (template.tags.length > MAX_TAGS) {
      throw new ImportValidationError('invalid_field:tags');
    }
    for (const tag of template.tags) {
      if (typeof tag !== 'string' || tag.length > MAX_TAG_LENGTH) {
        throw new ImportValidationError('invalid_field:tags');
      }
    }
  }

  // Step 10: truncate to the most recent MAX_IMPORT_VERSIONS (warning).
  const warnings = [];
  let versions = template.versions;
  if (versions.length > MAX_IMPORT_VERSIONS) {
    versions = versions.slice(versions.length - MAX_IMPORT_VERSIONS);
    warnings.push({ code: 'truncated_versions', params: { kept: MAX_IMPORT_VERSIONS, total: template.versions.length } });
  }

  // Normalization.
  const normalizedVersions = versions
    .map((v) => ({
      version: v.version,
      content: String(v.content),
      change_note: String(v.change_note == null ? '' : v.change_note),
      created_at: v.created_at,
    }))
    .sort((a, b) => a.version - b.version);

  const tags = Array.isArray(template.tags)
    ? [...new Set(template.tags.map((tag) => String(tag).trim()).filter(Boolean))]
    : [];

  return {
    data: {
      format: EXPORT_FORMAT,
      format_version: EXPORT_FORMAT_VERSION,
      exported_at: data.exported_at,
      source: data.source,
      template: {
        title: template.title,
        description: String(template.description == null ? '' : template.description),
        tags,
        category: String(template.category == null ? '' : template.category),
        language: String(template.language == null ? 'en' : template.language),
        visibility: template.visibility, // display only; importer forces private
        type: template.type, // display only; importer forces user
        versions: normalizedVersions,
      },
    },
    warnings,
  };
};

/**
 * Build the format-version-1 export JSON string from a template + versions.
 * Field whitelist only — owner identity is structurally impossible to include.
 * @param {Object} template - shape from getTemplate (or flattened admin shape)
 * @param {Array} versions - chronological array of {version, content, change_note, created_at}
 * @returns {string} pretty-printed JSON with a trailing newline
 */
export const buildTemplateExport = (template, versions) => {
  const obj = {
    format: EXPORT_FORMAT,
    format_version: EXPORT_FORMAT_VERSION,
    exported_at: new Date().toISOString(),
    source: 'open-prompts',
    template: {
      title: template.title,
      description: template.description == null ? '' : template.description,
      visibility: template.visibility,
      type: template.type,
      tags: Array.isArray(template.tags) ? template.tags : [],
      category: template.category == null ? '' : template.category,
      language: template.language == null ? 'en' : template.language,
      versions: versions.map((v) => ({
        version: v.version,
        content: v.content,
        change_note: v.change_note == null ? null : v.change_note,
        created_at: v.created_at,
      })),
    },
  };
  return JSON.stringify(obj, null, 2) + '\n';
};

/**
 * Trigger a browser download of `text` as `filename`.
 */
export const downloadJSON = (filename, text) => {
  const blob = new Blob([text], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
};

/**
 * Slugify a title for the export filename, capped at 60 chars, with an 8-char
 * id suffix. Falls back to "template" when the title slugifies to empty.
 * @returns {string} e.g. "my-prompt-template-a1b2c3d4"
 */
export const slugifyTitle = (title, id) => {
  const slug = String(title)
    .toLowerCase()
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');

  let capped = slug;
  if (capped.length > 60) {
    const cut = capped.slice(0, 60);
    const lastDash = cut.lastIndexOf('-');
    capped = lastDash > 0 ? cut.slice(0, lastDash) : cut;
  }

  const id8 = String(id).replace(/-/g, '').slice(0, 8);
  const base = capped || 'template';
  return `${base}-${id8}`;
};

/**
 * Fetch all versions of a template in chronological (earliest-first) order.
 * The API returns newest-first; this reverses the paginated result.
 * @param {string} templateId
 * @param {Function} listFn - listTemplateVersions (injectable for tests)
 * @returns {Promise<Array>} chronological versions
 */
export const fetchVersionsChronological = async (templateId, listFn) => {
  const versions = [];
  let pageToken = '';
  do {
    const resp = await listFn(templateId, { page_size: 50, page_token: pageToken });
    const data = resp.data || {};
    versions.push(...(data.versions || []));
    pageToken = data.next_page_token || '';
  } while (pageToken);
  return versions.reverse();
};