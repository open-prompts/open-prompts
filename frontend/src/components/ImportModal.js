import React, { useState, useRef, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useSelector } from 'react-redux';
import Modal from './ui/Modal';
import Field from './ui/Field';
import Spinner from './ui/Spinner';
import { AlertIcon } from './ui/Icons';
import { useNotification } from '../context/NotificationContext';
import { createTemplate, updateTemplate, getCategories, getTemplates } from '../services/api';
import { parseTemplateExport, MAX_CHANGE_NOTE_LENGTH } from '../utils/templateExport';
import './ImportModal.scss';

const ERROR_KEY_MAP = {
  too_large: 'import_export.error_too_large',
  not_json: 'import_export.error_not_json',
  bad_format: 'import_export.error_bad_format',
  unsupported_version: 'import_export.error_unsupported_version',
};

const mapValidationError = (err, t) => {
  if (err.code && ERROR_KEY_MAP[err.code]) {
    return t(ERROR_KEY_MAP[err.code], err.params);
  }
  if (err.code && err.code.startsWith('missing_field:')) {
    return t('import_export.error_missing_field', { field: err.code.slice('missing_field:'.length) });
  }
  if (err.code && err.code.startsWith('invalid_field:')) {
    return t('import_export.error_invalid_field', { field: err.code.slice('invalid_field:'.length) });
  }
  return t('import_export.error_bad_format');
};

/**
 * ImportModal — two-step import flow (pick → review). Parses an
 * open-prompts template export file client-side, then replays its version
 * history through createTemplate/updateTemplate (earliest-first).
 */
const ImportModal = ({ open, onRequestClose }) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { addNotification } = useNotification();
  const user = useSelector((state) => state.auth.user);

  const [step, setStep] = useState('pick');
  const [parsed, setParsed] = useState(null);
  const [parsing, setParsing] = useState(false);
  const [validationError, setValidationError] = useState('');
  const [dragOver, setDragOver] = useState(false);
  const [shake, setShake] = useState(false);
  const fileInputRef = useRef(null);

  // Review-form state
  const [title, setTitle] = useState('');
  const [category, setCategory] = useState('');
  const [visibility, setVisibility] = useState('private');
  const [language, setLanguage] = useState('en');
  const [tags, setTags] = useState('');
  const [description, setDescription] = useState('');
  const [content, setContent] = useState('');
  const [categories, setCategories] = useState([]);
  const [titleCollision, setTitleCollision] = useState(false);
  const [formErrors, setFormErrors] = useState({});
  const [generalError, setGeneralError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const reset = useCallback(() => {
    setStep('pick');
    setParsed(null);
    setParsing(false);
    setValidationError('');
    setDragOver(false);
    setTitle('');
    setCategory('');
    setVisibility('private');
    setLanguage('en');
    setTags('');
    setDescription('');
    setContent('');
    setTitleCollision(false);
    setFormErrors({});
    setGeneralError('');
    setSubmitting(false);
  }, []);

  useEffect(() => {
    if (open) {
      reset();
      getCategories()
        .then((resp) => {
          const data = resp.data;
          if (data && Array.isArray(data.categories)) {
            setCategories(data.categories.map((c) => c.name));
          } else if (Array.isArray(data)) {
            setCategories(data);
          } else {
            setCategories([]);
          }
        })
        .catch(() => setCategories([]));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const handleFile = async (file) => {
    if (!file) return;
    setParsing(true);
    setValidationError('');
    try {
      const result = await parseTemplateExport(file);
      const tpl = result.data.template;
      setParsed(result);
      setTitle(tpl.title);
      setCategory(tpl.category && categories.includes(tpl.category) ? tpl.category : '');
      setVisibility('private');
      setLanguage(tpl.language || 'en');
      setTags(tpl.tags.join(', '));
      setDescription(tpl.description);
      const latest = tpl.versions[tpl.versions.length - 1];
      setContent(latest ? latest.content : '');
      setStep('review');
    } catch (err) {
      setValidationError(mapValidationError(err, t));
    } finally {
      setParsing(false);
    }
  };

  const onDrop = (e) => {
    e.preventDefault();
    setDragOver(false);
    const file = e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files[0];
    if (!file) return;
    if (!file.name.toLowerCase().endsWith('.json')) {
      triggerShake();
      setValidationError(t('import_export.error_bad_format'));
      return;
    }
    handleFile(file);
  };

  const triggerShake = () => {
    setShake(true);
    setTimeout(() => setShake(false), 400);
  };

  const openPicker = () => {
    if (fileInputRef.current) fileInputRef.current.click();
  };

  // Title collision check (non-blocking, advisory).
  const checkTitleCollision = useCallback(
    async (value) => {
      if (!value || !user) return;
      try {
        const resp = await getTemplates({ owner_id: user.id, page_size: 100 });
        const list = resp.data && resp.data.templates ? resp.data.templates : [];
        const hit = list.some(
          (tpl) => String(tpl.title || '').toLowerCase() === String(value).toLowerCase()
        );
        setTitleCollision(hit);
      } catch (e) {
        setTitleCollision(false);
      }
    },
    [user]
  );

  const handleTitleChange = (e) => {
    const value = e.target.value;
    setTitle(value);
    if (formErrors.title) setFormErrors((prev) => ({ ...prev, title: '' }));
    checkTitleCollision(value);
  };

  const useSuggestedTitle = () => {
    const suffix = t('import_export.suggested_suffix');
    setTitle(`${title} ${suffix}`);
    setTitleCollision(false);
  };

  const handleSubmit = async () => {
    setFormErrors({});
    setGeneralError('');
    const errors = {};
    if (!title.trim()) errors.title = t('create_template.error_required_title');
    if (!category) errors.category = t('create_template.error_required_category_select');
    if (!content.trim()) errors.content = t('create_template.error_required_content');
    if (Object.keys(errors).length > 0) {
      setFormErrors(errors);
      setGeneralError(t('create_template.error_required_fields'));
      return;
    }

    setSubmitting(true);
    const versions = parsed.data.template.versions; // chronological, ≤ 50
    const base = {
      title: title.trim(),
      description,
      category,
      tags: tags.split(',').map((tag) => tag.trim()).filter(Boolean),
      language: language || 'en',
      visibility,
      type: 'user',
    };

    try {
      // 1. create with EARLIEST content
      const created = await createTemplate({ ...base, content: versions[0].content });
      const newId = created.data.template.id;
      // 2. replay v2..vN, earliest → latest
      for (let i = 1; i < versions.length; i++) {
        const isLast = i === versions.length - 1;
        const note = String(versions[i].change_note || '').slice(0, MAX_CHANGE_NOTE_LENGTH - 3);
        await updateTemplate(newId, {
          ...base,
          content: isLast ? content : versions[i].content,
          change_note: note,
        });
      }
      onRequestClose();
      addNotification({ kind: 'success', title: t('common.success'), subtitle: t('import_export.import_success') });
      navigate(`/templates/${newId}`);
    } catch (err) {
      console.error('Import replay failed:', err);
      setSubmitting(false);
      setGeneralError(t('import_export.import_partial', { imported: 1, total: versions.length }));
    }
  };

  const handleKeyDown = (e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      openPicker();
    }
  };

  return (
    <Modal
      open={open}
      className="import-modal"
      title={t('import_export.import_title')}
      primaryLabel={step === 'review' ? (submitting ? t('common.saving') : t('import_export.confirm')) : undefined}
      primaryDisabled={step !== 'review' || submitting}
      onPrimary={step === 'review' ? handleSubmit : undefined}
      secondaryLabel={t('common.cancel')}
      onClose={onRequestClose}
      size="lg"
    >
      {step === 'pick' ? (
        <div className="import-pick">
          <div
            className={`drop-zone ${dragOver ? 'drop-zone--over' : ''} ${shake ? 'drop-zone--shake' : ''}`}
            onDragOver={(e) => {
              e.preventDefault();
              setDragOver(true);
            }}
            onDragLeave={() => setDragOver(false)}
            onDrop={onDrop}
            onClick={openPicker}
            onKeyDown={handleKeyDown}
            role="button"
            tabIndex={0}
            aria-label={t('import_export.drop_hint')}
          >
            <input
              ref={fileInputRef}
              type="file"
              accept=".json,application/json"
              className="drop-zone__input"
              onChange={(e) => {
                const file = e.target.files && e.target.files[0];
                if (file) handleFile(file);
                e.target.value = '';
              }}
            />
            <p className="drop-zone__hint">{t('import_export.drop_hint')}</p>
            <button type="button" className="btn-secondary btn-sm" onClick={(e) => { e.stopPropagation(); openPicker(); }}>
              {t('import_export.browse')}
            </button>
            <p className="drop-zone__note">{t('import_export.drop_note')}</p>
          </div>
          {parsing && (
            <div className="import-parsing">
              <Spinner size={18} />
              <span>{t('common.loading')}</span>
            </div>
          )}
          {validationError && (
            <div className="form-alert" role="alert">
              <AlertIcon size={16} />
              <span>{validationError}</span>
            </div>
          )}
        </div>
      ) : (
        <div className="import-review">
          <p className="import-review__summary">
            {t('import_export.review_summary', {
              file: parsed.data.template.title,
              count: parsed.data.template.versions.length,
            })}
          </p>
          {parsed.warnings.map((w) => (
            <p key={w.code} className="import-review__warning" role="status">
              {t('import_export.warning_truncated_versions', w.params)}
            </p>
          ))}

          {generalError && (
            <div className="form-alert" role="alert">
              <AlertIcon size={16} />
              <span>{generalError}</span>
            </div>
          )}

          <Field id="import-title" label={t('create_template.label_title')} error={formErrors.title} required>
            <input
              type="text"
              className="input"
              value={title}
              onChange={handleTitleChange}
            />
          </Field>
          {titleCollision && (
            <div className="import-collision" role="status">
              <span>{t('import_export.title_collision')}</span>
              <button type="button" className="btn-ghost btn-sm" onClick={useSuggestedTitle}>
                {t('import_export.use_suggested')}
              </button>
            </div>
          )}

          <Field id="import-category" label={t('create_template.label_category')} error={formErrors.category} required>
            <select
              className="select"
              value={category}
              onChange={(e) => {
                setCategory(e.target.value);
                if (formErrors.category) setFormErrors((prev) => ({ ...prev, category: '' }));
              }}
            >
              <option value="" disabled>{t('create_template.choose_category')}</option>
              {categories.map((cat) => (
                <option key={cat} value={cat}>{cat}</option>
              ))}
            </select>
          </Field>

          <Field id="import-visibility" label={t('create_template.label_visibility')}>
            <select
              className="select"
              value={visibility}
              onChange={(e) => setVisibility(e.target.value)}
            >
              <option value="private">{t('create_template.visibility_private')}</option>
              <option value="public">{t('create_template.visibility_public')}</option>
            </select>
          </Field>

          <Field id="import-language" label={t('create_template.label_language')}>
            <select
              className="select"
              value={language}
              onChange={(e) => setLanguage(e.target.value)}
            >
              <option value="en">{t('template.en')}</option>
              <option value="zh">{t('template.zh')}</option>
            </select>
          </Field>

          <Field id="import-tags" label={t('create_template.label_tags')}>
            <input
              type="text"
              className="input"
              placeholder={t('create_template.ph_tags')}
              value={tags}
              onChange={(e) => setTags(e.target.value)}
            />
          </Field>

          <Field id="import-description" label={t('create_template.label_description')}>
            <textarea
              className="textarea"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
            />
          </Field>

          <Field id="import-content" label={t('create_template.label_content')} error={formErrors.content} required>
            <textarea
              className="textarea"
              value={content}
              onChange={(e) => {
                setContent(e.target.value);
                if (formErrors.content) setFormErrors((prev) => ({ ...prev, content: '' }));
              }}
              rows={10}
            />
          </Field>
        </div>
      )}
    </Modal>
  );
};

export default ImportModal;