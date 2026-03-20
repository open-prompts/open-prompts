import React, { useState, useEffect } from 'react';
import {
  Modal,
  TextInput,
  TextArea,
  Dropdown
} from '@carbon/react';
import { useNotification } from '../context/NotificationContext';
import { useTranslation } from 'react-i18next';
import { createTemplate, getCategories } from '../services/api';
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
  const { t, i18n } = useTranslation();
  const [formData, setFormData] = useState({
    title: '',
    description: '',
    content: '',
    visibility: 'private',
    category: '',
    tags: '',
  });
  const [categories, setCategories] = useState([]);
  const [customCategory, setCustomCategory] = useState('');
  const [loading, setLoading] = useState(false);
  const [formErrors, setFormErrors] = useState({});
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
        setCategories(['General', 'Writing', 'Coding', 'Business']);
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
      });
      setCustomCategory('');
      setFormErrors({});
    }
  }, [open, addNotification, t]);

  const handleChange = (e) => {
    const { id, value } = e.target;
    setFormData((prev) => ({
      ...prev,
      [id]: value,
    }));
    if (formErrors[id]) {
        setFormErrors(prev => ({...prev, [id]: ''}));
    }
  };

  const handleSubmit = async () => {
    setFormErrors({});
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
        return;
    }

    setLoading(true);

    try {
      // Prepare payload
      const lang = (i18n && (i18n.language === 'zh' || i18n.language.startsWith('zh'))) ? 'zh' : 'en';

      const payload = {
        ...formData,
        category: formData.category === 'create_new' ? customCategory.trim() : formData.category,
        tags: formData.tags.split(',').map(tag => tag.trim()).filter(tag => tag),
        type: 'user', // Default to user type
        language: lang,
      };

      await createTemplate(payload);
      setLoading(false);
      // Notify user of success (localized)
      addNotification({ kind: 'success', title: t('common.success'), subtitle: t('create_template.success_created'), timeout: 4000 });
      onRequestClose();
      if (onSuccess) onSuccess();
    } catch (err) {
      console.error('Create template error:', err);
      setLoading(false);
      addNotification({ kind: 'error', title: t('common.error'), subtitle: t('create_template.error_submit') });
    }
  };

  const categoryItems = [
    ...categories.map(c => ({ id: c, text: c })),
    { id: 'create_new', text: t('create_template.create_new_category') }
  ];

  const visibilityItems = [
    { id: 'public', text: t('create_template.visibility_public') },
    { id: 'private', text: t('create_template.visibility_private') }
  ];

  return (
    <Modal
      open={open}
      className="create-template-modal"
      modalHeading={t('create_template.title')}
      primaryButtonText={loading ? t('common.saving') : t('common.create')}
      primaryButtonDisabled={loading}
      secondaryButtonText={t('common.cancel')}
      onRequestClose={onRequestClose}
      onRequestSubmit={handleSubmit}
      danger={false}
      selectorPrimaryFocus="#title"
    >
      <div className="create-template-form">
        <TextInput
          id="title"
          labelText={t('create_template.label_title')}
          placeholder={t('create_template.ph_title')}
          value={formData.title}
          onChange={handleChange}
          required // Carbon styling for required
          className="form-field"
          invalid={!!formErrors.title}
          invalidText={formErrors.title}
        />

        <Dropdown
          id="category"
          titleText={t('create_template.label_category')}
          label={t('create_template.choose_category')}
          items={categoryItems}
          itemToString={(item) => (item ? item.text : '')}
          selectedItem={categoryItems.find(c => c.id === formData.category) || null}
          onChange={({ selectedItem }) => {
            setFormData(prev => ({ ...prev, category: selectedItem.id }));
            // Clear category-related validation errors when user selects a value
            setFormErrors(prev => ({ ...prev, category: '', customCategory: '' }));
          }}
          className="form-field"
          invalid={!!formErrors.category}
          invalidText={formErrors.category}
        />

        {formData.category === 'create_new' && (
          <TextInput
            id="customCategory"
            labelText={t('create_template.label_new_category')}
            placeholder={t('create_template.ph_new_category')}
            value={customCategory}
            onChange={(e) => {
              setCustomCategory(e.target.value);
              if (formErrors.customCategory) setFormErrors(prev => ({ ...prev, customCategory: '' }));
            }}
            className="form-field"
            style={{ marginTop: '0.5rem' }}
            invalid={!!formErrors.customCategory}
            invalidText={formErrors.customCategory}
          />
        )}

        <Dropdown
          id="visibility"
          titleText={t('create_template.label_visibility')}
          items={visibilityItems}
          itemToString={(item) => (item ? item.text : '')}
          selectedItem={visibilityItems.find(v => v.id === formData.visibility)}
          onChange={({ selectedItem }) => {
            setFormData(prev => ({ ...prev, visibility: selectedItem.id }));
            if (formErrors.visibility) setFormErrors(prev => ({ ...prev, visibility: '' }));
          }}
          className="form-field"
        />

        <TextInput
          id="tags"
          labelText={t('create_template.label_tags')}
          placeholder={t('create_template.ph_tags')}
          value={formData.tags}
          onChange={handleChange}
          className="form-field"
        />

        <TextArea
          id="description"
          labelText={t('create_template.label_description')}
          placeholder={t('create_template.ph_description')}
          value={formData.description}
          onChange={handleChange}
          className="form-field"
        />

        <TextArea
          id="content"
          labelText={t('create_template.label_content')}
          placeholder={t('create_template.ph_content')}
          value={formData.content}
          onChange={handleChange}
          rows={10}
          className="form-field"
          invalid={!!formErrors.content}
          invalidText={formErrors.content}
        />
      </div>
    </Modal>
  );
};

export default CreateTemplateModal;
