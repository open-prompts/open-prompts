import React, { useState, useEffect } from 'react';
import Modal from './ui/Modal';
import Field from './ui/Field';
import { useNotification } from '../context/NotificationContext';
import { useTranslation } from 'react-i18next';
import { createTemplate, getCategories } from '../services/api';
import { AlertIcon } from './ui/Icons';
import PublishReviewModal from './review/PublishReviewModal';
import WarningCheckboxGroup from './warnings/WarningCheckboxGroup';
import './CreateTemplateModal.scss';

/**
 * CreateTemplateModal Component
 * A modal form for creating a new prompt template.
 * @param {Object} props - Component properties
 * @param {boolean} props.open - Whether the modal is open
 * @param {Function} props.onRequestClose - Function to call when closing the modal
 * @param {Function} props.onSuccess - Function to call when creation is successful
 */
const CreateTemplateModal = ({ open, onRequestClose, onSuccess }) => {
  const { t } = useTranslation();
  const [formData, setFormData] = useState({
    title: '',
    description: '',
    content: '',
    visibility: 'private',
    category: '',
    tags: '',
    language: 'en',
    content_warnings: [],
  });
  const [categories, setCategories] = useState([]);
  const [customCategory, setCustomCategory] = useState('');
  const [loading, setLoading] = useState(false);
  const [formErrors, setFormErrors] = useState({});
  const [generalError, setGeneralError] = useState('');
  const [reviewModalOpen, setReviewModalOpen] = useState(false);
  const { addNotification } = useNotification();

  // Fetch categories when the modal opens
  useEffect(() => {
    const loadCategories = async () => {
      try {
        const response = await getCategories();
        const data = response.data;
        if (data && Array.isArray(data.categories)) {
          // Extract names from CategoryStats objects
          setCategories(data.categories.map(c => c.name));
        } else if (Array.isArray(data)) {
          setCategories(data);
        } else {
          setCategories([]);
        }
      } catch (err) {
        console.error('Failed to load categories', err);
        addNotification({ kind: 'warning', title: t('common.warning'), subtitle: t('create_template.error_load_categories') });
        // Fallback categories if API fails
        const fallbackCats = t('create_template.default_categories', { returnObjects: true });
        setCategories(Array.isArray(fallbackCats) ? fallbackCats : ['General', 'Writing', 'Coding', 'Business']);
      }
    };

    if (open) {
      loadCategories();
      // Reset form
      setFormData({
        title: '',
        description: '',
        content: '',
        visibility: 'private',
        category: '',
        tags: '',
        language: 'en',
        content_warnings: [],
      });
      setCustomCategory('');
      setFormErrors({});
      setGeneralError('');
    }
    // Note: `t` is intentionally excluded from deps — with react-i18next it is
    // memoized, but test mocks recreate it each render, which would cause an
    // infinite effect loop.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, addNotification]);

  const handleChange = (e) => {
    const { id, value } = e.target;
    setFormData((prev) => ({
      ...prev,
      [id]: value,
    }));
    if (formErrors[id]) {
        setFormErrors(prev => ({ ...prev, [id]: '' }));
    }
  };

  const handleSubmit = async () => {
    setFormErrors({});
    setGeneralError('');
    const errors = {};

    // Basic validation
    if (!formData.title) errors.title = t('create_template.error_required_title');
    if (!formData.category) errors.category = t('create_template.error_required_category_select');
    if (!formData.content) errors.content = t('create_template.error_required_content');

    // Validate custom category if selected
    if (formData.category === 'create_new' && !customCategory.trim()) {
      errors.customCategory = t('create_template.error_required_category');
    }

    if (Object.keys(errors).length > 0) {
        setFormErrors(errors);
        setGeneralError(t('create_template.error_required_fields'));
        return;
    }

    setLoading(true);

    try {
      // Prepare payload. The template language is an explicit attribute of the
      // template itself (chosen in the form), NOT the UI display language.
      const payload = {
        ...formData,
        category: formData.category === 'create_new' ? customCategory.trim() : formData.category,
        tags: formData.tags.split(',').map(tag => tag.trim()).filter(tag => tag),
        type: 'user', // Default to user type
        language: formData.language || 'en',
      };

      const resp = await createTemplate(payload);
      setLoading(false);
      const created = resp.data && resp.data.template;
      if (created && created.moderation_status === 'in_review') {
        // Gated creator: the publish entered pre-publication review — show
        // the review confirmation instead of the plain success toast.
        onRequestClose();
        if (onSuccess) onSuccess();
        setReviewModalOpen(true);
        return;
      }
      // Notify user of success (localized)
      addNotification({ kind: 'success', title: t('common.success'), subtitle: t('create_template.success_created'), timeout: 4000 });
      onRequestClose();
      if (onSuccess) onSuccess();
    } catch (err) {
      console.error('Create template error:', err);
      setLoading(false);
      setGeneralError(t('create_template.error_submit'));
      addNotification({ kind: 'error', title: t('common.error'), subtitle: t('create_template.error_submit') });
    }
  };

  return (
    <>
    <Modal
      open={open}
      className="create-template-modal"
      title={t('create_template.title')}
      primaryLabel={loading ? t('common.saving') : t('common.create')}
      primaryDisabled={loading}
      secondaryLabel={t('common.cancel')}
      onPrimary={handleSubmit}
      onClose={onRequestClose}
      size="lg"
    >
      <div className="create-template-form">
        {generalError && (
          <div className="form-alert">
            <AlertIcon size={16} />
            <span>{generalError}</span>
          </div>
        )}

        <Field id="title" label={t('create_template.label_title')} error={formErrors.title} required>
          <input
            type="text"
            className="input"
            placeholder={t('create_template.ph_title')}
            value={formData.title}
            onChange={handleChange}
          />
        </Field>

        <Field id="category" label={t('create_template.label_category')} error={formErrors.category}>
          <select
            className="select"
            value={formData.category}
            onChange={(e) => {
              setFormData(prev => ({ ...prev, category: e.target.value }));
              // Clear category-related validation errors when user selects a value
              setFormErrors(prev => ({ ...prev, category: '', customCategory: '' }));
            }}
          >
            <option value="" disabled>{t('create_template.choose_category')}</option>
            {categories.map((cat) => (
              <option key={cat} value={cat}>{cat}</option>
            ))}
            <option value="create_new">{t('create_template.create_new_category')}</option>
          </select>
        </Field>

        {formData.category === 'create_new' && (
          <Field id="customCategory" label={t('create_template.label_new_category')} error={formErrors.customCategory}>
            <input
              type="text"
              className="input"
              placeholder={t('create_template.ph_new_category')}
              value={customCategory}
              onChange={(e) => {
                setCustomCategory(e.target.value);
                if (formErrors.customCategory) setFormErrors(prev => ({ ...prev, customCategory: '' }));
              }}
            />
          </Field>
        )}

        <Field id="visibility" label={t('create_template.label_visibility')}>
          <select
            className="select"
            value={formData.visibility}
            onChange={(e) => {
              setFormData(prev => ({ ...prev, visibility: e.target.value }));
              if (formErrors.visibility) setFormErrors(prev => ({ ...prev, visibility: '' }));
            }}
          >
            <option value="public">{t('create_template.visibility_public')}</option>
            <option value="private">{t('create_template.visibility_private')}</option>
          </select>
        </Field>

        <Field id="language" label={t('create_template.label_language')}>
          <select
            className="select"
            value={formData.language}
            onChange={(e) => {
              setFormData(prev => ({ ...prev, language: e.target.value }));
            }}
          >
            <option value="en">{t('template.en')}</option>
            <option value="zh">{t('template.zh')}</option>
          </select>
        </Field>

        <Field id="tags" label={t('create_template.label_tags')}>
          <input
            type="text"
            className="input"
            placeholder={t('create_template.ph_tags')}
            value={formData.tags}
            onChange={handleChange}
          />
        </Field>

        <Field id="description" label={t('create_template.label_description')}>
          <textarea
            className="textarea"
            placeholder={t('create_template.ph_description')}
            value={formData.description}
            onChange={handleChange}
            rows={3}
          />
        </Field>

        <Field id="content" label={t('create_template.label_content')} error={formErrors.content} required>
          <div className="content-variable-hint">
            <span className="content-variable-hint-text">{t('create_template.content_variable_hint')}</span>
            <code className="content-variable-hint-example">{t('create_template.content_variable_example')}</code>
          </div>
          <textarea
            className="textarea"
            placeholder={t('create_template.ph_content')}
            value={formData.content}
            onChange={handleChange}
            rows={10}
          />
        </Field>

        <WarningCheckboxGroup
          value={formData.content_warnings}
          onChange={(warnings) => setFormData((prev) => ({ ...prev, content_warnings: warnings }))}
        />
      </div>
    </Modal>
    <PublishReviewModal
      open={reviewModalOpen}
      onViewStatus={() => setReviewModalOpen(false)}
      onDone={() => setReviewModalOpen(false)}
    />
    </>
  );
};

export default CreateTemplateModal;
