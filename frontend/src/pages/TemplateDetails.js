import React, { useState, useEffect, useMemo } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useSelector } from 'react-redux';
import { useTranslation } from 'react-i18next';
import {
  getTemplate,
  listTemplateVersions,
  updateTemplate,
  createPrompt,
  listPrompts,
  deletePrompt,
  deleteTemplate,
  toggleTemplateLike,
  toggleTemplateFavorite,
  forkTemplate,
  getCategories
} from '../services/api';
import { Modal } from '@carbon/react';
import { useNotification } from '../context/NotificationContext';
import Layout from '../components/Layout';
import TemplateAliasManager from '../components/TemplateAliasManager';
import './TemplateDetails.scss';
import VersionSelect from '../components/VersionSelect';

const TemplateDetails = () => {
  const { t, i18n } = useTranslation();
  const { id } = useParams();
  const navigate = useNavigate();
  const { user } = useSelector((state) => state.auth);

  const [template, setTemplate] = useState(null);
  const [versions, setVersions] = useState([]);
  const [selectedVersionId, setSelectedVersionId] = useState(null);
  const [prompts, setPrompts] = useState([]);
  const [loading, setLoading] = useState(true);
  const isOwner = user && template && user.id === template.owner_id;

  // Social Stats
  const [isLiked, setIsLiked] = useState(false);
  const [isFavorited, setIsFavorited] = useState(false);
  const [likesCount, setLikesCount] = useState(0);
  const [favoritesCount, setFavoritesCount] = useState(0);

  // Loading States
  const [isSaving, setIsSaving] = useState(false);
  const [isGenerating, setIsGenerating] = useState(false);
  const [isSharing, setIsSharing] = useState(false);
  const [isDeletingTemplate, setIsDeletingTemplate] = useState(false);
  const [isDeletingPrompt, setIsDeletingPrompt] = useState(false);
  const [isForking, setIsForking] = useState(false);

  // Editor state
  const [editContent, setEditContent] = useState('');
  const [isEditing, setIsEditing] = useState(false);

  // Generator state
  const [variableValues, setVariableValues] = useState({});

  // Delete Modal State
  const [isDeleteModalOpen, setIsDeleteModalOpen] = useState(false);
  const [promptToDelete, setPromptToDelete] = useState(null);

  // Template Delete Modal State
  const [isTemplateDeleteModalOpen, setIsTemplateDeleteModalOpen] = useState(false);

  // Template Fork Modal State
  const [isForkModalOpen, setIsForkModalOpen] = useState(false);

  // Metadata Edit State
  const [isEditingMetadata, setIsEditingMetadata] = useState(false);
  const [editCategory, setEditCategory] = useState('');
  const [editTags, setEditTags] = useState([]);
  const [editTitle, setEditTitle] = useState('');
  const [editDescription, setEditDescription] = useState('');
  const [tagInput, setTagInput] = useState('');
  const [categories, setCategories] = useState([]);
  const [customCategory, setCustomCategory] = useState('');

  const { addNotification } = useNotification();

  // Initial Data Fetch
  useEffect(() => {
    const fetchData = async () => {
      setLoading(true);
      try {
        const [tplRes, verRes, pmtRes] = await Promise.all([
          getTemplate(id),
          listTemplateVersions(id, { page_size: 100 }), // Fetch enough versions
          user ? listPrompts({ template_id: id, owner_id: user.id }) : Promise.resolve({ data: { prompts: [] } })
        ]);

        setTemplate(tplRes.data.template);
        setVersions(verRes.data.versions || []);
        setPrompts(pmtRes.data.prompts || []);

        setIsLiked(tplRes.data.template.is_liked || false);
        setIsFavorited(tplRes.data.template.is_favorited || false);
        setLikesCount(tplRes.data.template.likes_count || 0);
        setFavoritesCount(tplRes.data.template.favorites_count || 0);

        // Select latest version by default
        if (tplRes.data.latest_version) {
          setSelectedVersionId(tplRes.data.latest_version.id);
          setEditContent(tplRes.data.latest_version.content);
        } else if (verRes.data.versions && verRes.data.versions.length > 0) {
            // Fallback if latest_version isn't in getTemplate response (e.g. older backend)
            setSelectedVersionId(verRes.data.versions[0].id);
            setEditContent(verRes.data.versions[0].content);
        }
      } catch (error) {
        console.error("Failed to load template data", error);
        addNotification({
          kind: 'error',
          title: t('common.error'),
          subtitle: t('template_details.error_load')
        });
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, [id, user, addNotification, t]);

  // Fetch categories when editing metadata
  useEffect(() => {
    if (isEditingMetadata) {
        const fetchCategories = async () => {
             try {
                 const currentLang = i18n.language ? i18n.language.split('-')[0] : 'en';
                 const res = await getCategories({ language: currentLang });
                 const data = res.data;
                 let cats = [];
                 if (data && Array.isArray(data.categories)) {
                     cats = data.categories.map(c => c.name);
                 } else if (Array.isArray(data)) {
                     cats = data;
                 }
                 // Ensure current template category is in the list if it has one
                 // But wait, user might want to switch.
                 // If we purely use fetched list, and current category is not in it (e.g. cross-language mismatch),
                 // it will show empty in select if we don't have it as an option.
                 // We should probably just render what we get. The user can choose to keep it (if we add it to options) or change it.
                 // If we don't add it, the select value 'editCategory' might not match any option, showing blank.
                 // Let's rely on the user changing it if they want.
                 // Or better yet, append editCategory if not present?
                 // But unique.
                 setCategories(cats);
             } catch (error) {
                 console.error("Failed to load categories", error);
             }
        };
        fetchCategories();
    }
  }, [isEditingMetadata, i18n.language]);

  // Metadata Edit Handlers
  const handleRemoveTag = (tagToRemove) => {
      setEditTags(editTags.filter(tag => tag !== tagToRemove));
  };

  const handleAddTag = () => {
      if (tagInput.trim() && !editTags.includes(tagInput.trim())) {
          setEditTags([...editTags, tagInput.trim()]);
          setTagInput('');
      }
  };

  const handleTagInputKeyDown = (e) => {
      if (e.key === 'Enter') {
          e.preventDefault();
          handleAddTag();
      }
  };

  const handleEditMetadata = () => {
      setEditCategory(template.category || '');
      setCustomCategory('');
      setEditTags(template.tags || []);
      setTagInput('');
      setEditTitle(template.title || '');
      setEditDescription(template.description || '');
      setIsEditingMetadata(true);
  };

  const handleSaveMetadata = async () => {
      setIsSaving(true);
      try {
          // When saving metadata only, always use the latest version's content to avoid creating a new version
          // unless content actually changed on backend check.
          const contentToSend = versions.length > 0 ? versions[0].content : '';

          let finalCategory = editCategory;
          if (editCategory === 'create_new') {
              finalCategory = customCategory.trim();
              if (!finalCategory) {
                  addNotification({ kind: 'error', title: t('common.error'), subtitle: t('create_template.error_required_category') });
                  setIsSaving(false);
                  return;
              }
          }

          // Auto-add pending tag input if user forgot to press Enter
          let finalTags = Array.isArray(editTags) ? [...editTags] : [];
          if (tagInput && tagInput.trim()) {
              const newTag = tagInput.trim();
              if (!finalTags.includes(newTag)) {
                  finalTags.push(newTag);
              }
          }

          const updateData = {
              template_id: template.id,
              owner_id: template.owner_id,
              title: editTitle,
              description: editDescription,
              visibility: template.visibility,
              category: finalCategory,
              tags: finalTags,
              content: contentToSend
          };
            // Preserve language field when updating metadata
            if (template && template.language) {
              updateData.language = template.language;
            }

          const res = await updateTemplate(template.id, updateData);
          setTemplate(res.data.template);

          // If a new version is created (depends on backend logic when content is same), add it
          if (res.data.new_version) {
               const nv = res.data.new_version;
               if (!versions.some(v => v.id === nv.id)) {
                   setVersions([nv, ...versions]);
                   // Optional: switch to new version?
                   // If content didn't change, we might not want to switch version context abruptly?
                   // But if backend created a version, it's the latest.
               }
          }

          setIsEditingMetadata(false);
          addNotification({ kind: 'success', title: t('common.success'), subtitle: t('template_details.success_update') });
      } catch (error) {
          console.error("Failed to update template metadata", error);
          addNotification({ kind: 'error', title: t('common.error'), subtitle: t('template_details.error_update') });
      } finally {
          setIsSaving(false);
      }
  };

  // Handle Edit Save
  const handleSaveContent = async () => {
    setIsSaving(true);
    try {
      // Assuming updateTemplate creates a new version if content changes
      const updateData = {
        template_id: template.id,
        owner_id: template.owner_id,
        title: template.title,
        description: template.description,
        visibility: template.visibility,
        category: template.category,
        tags: template.tags,
        content: editContent
      };
      // Ensure language is preserved when saving content
      if (template && template.language) {
        updateData.language = template.language;
      }

      const res = await updateTemplate(template.id, updateData);

      // Update local state with new version
      const newVersion = res.data.new_version;
      setVersions([newVersion, ...versions]);
      setSelectedVersionId(newVersion.id);
      setIsEditing(false);
      addNotification({ kind: 'success', title: t('common.success'), subtitle: t('template_details.success_update') });
    } catch (error) {
      console.error("Failed to update template", error);
      addNotification({ kind: 'error', title: t('common.error'), subtitle: t('template_details.error_update') });
    } finally {
      setIsSaving(false);
    }
  };

  // Whether the save button should be disabled: saving in progress,
  // no selected version, or edited content equals the selected version content
  const currentVersionContent = versions.find(v => v.id === selectedVersionId)?.content || '';
  const isSaveNewVersionDisabled = isSaving || !selectedVersionId || (editContent === currentVersionContent);

  const handleLike = async () => {
    if (!user) {
        addNotification({ kind: 'error', title: t('common.error'), subtitle: t('template_details.error_login_like') });
        return;
    }
    const oldState = isLiked;
    setIsLiked(!oldState);
    setLikesCount(prev => oldState ? prev - 1 : prev + 1);

    try {
        await toggleTemplateLike(template.id);
    } catch (error) {
        // Revert
        setIsLiked(oldState);
        setLikesCount(prev => oldState ? prev + 1 : prev - 1);
        addNotification({ kind: 'error', title: t('common.error'), subtitle: t('template_details.error_like') });
    }
  };

  const handleFavorite = async () => {
    if (!user) {
        addNotification({ kind: 'error', title: t('common.error'), subtitle: t('template_details.error_login_favorite') });
        return;
    }
    const oldState = isFavorited;
    setIsFavorited(!oldState);
    setFavoritesCount(prev => oldState ? prev - 1 : prev + 1);

    try {
        await toggleTemplateFavorite(template.id);
    } catch (error) {
        // Revert
        setIsFavorited(oldState);
        setFavoritesCount(prev => oldState ? prev + 1 : prev - 1);
        addNotification({ kind: 'error', title: t('common.error'), subtitle: t('template_details.error_favorite') });
    }
  };

  // Parse Content for Placeholders like ${name}
  // We produce: parts (static strings) and placeholders (names in order)
  const { parts: contentParts, placeholders } = useMemo(() => {
    const currentContent = isEditing ? editContent : (versions.find(v => v.id === selectedVersionId)?.content || '');
    const parts = [];
    const names = [];
    let lastIndex = 0;
    const re = /\$\{([^}]+)\}/g;
    let m;
    while ((m = re.exec(currentContent)) !== null) {
      parts.push(currentContent.slice(lastIndex, m.index));
      names.push(m[1]);
      lastIndex = m.index + m[0].length;
    }
    parts.push(currentContent.slice(lastIndex));
    return { parts, placeholders: names };
  }, [editContent, isEditing, versions, selectedVersionId]);

  // Unique variable names (preserve first-seen order)
  const uniqueNames = useMemo(() => {
    const seen = new Set();
    const uniq = [];
    placeholders.forEach((n) => {
      if (!seen.has(n)) {
        seen.add(n);
        uniq.push(n);
      }
    });
    return uniq;
  }, [placeholders]);

  // Initialize variable inputs as a map { name: value }
  useEffect(() => {
    const init = {};
    uniqueNames.forEach(n => { init[n] = ''; });
    setVariableValues(init);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [uniqueNames.join('|')]);

  const handleVariableChange = (name, value) => {
    setVariableValues(prev => ({ ...prev, [name]: value }));
  };

  // Generate Prompt Result
  const generatedContent = useMemo(() => {
    let result = '';
    contentParts.forEach((part, idx) => {
      result += part;
      if (idx < placeholders.length) {
        const name = placeholders[idx];
        result += variableValues[name] || '';
      }
    });
    return result;
  }, [contentParts, placeholders, variableValues]);

  // Handle Create Prompt
  const handleCreatePrompt = async () => {
    if (!user) {
        addNotification({ kind: 'error', title: t('common.error'), subtitle: t('template_details.error_login_save') });
        return;
    }
    setIsGenerating(true);
    try {
      const variablesPayload = {};
      uniqueNames.forEach(name => { variablesPayload[name] = variableValues[name] || ''; });
      
      const promptData = {
        template_id: template.id,
        version_id: selectedVersionId,
        owner_id: user.id,
        variables: variablesPayload
      };
      const res = await createPrompt(promptData);
      setPrompts([res.data.prompt, ...prompts]);
      addNotification({ kind: 'success', title: t('common.success'), subtitle: t('template_details.success_save_prompt') });
    } catch (error) {
      console.error("Failed to create prompt", error);
      addNotification({ kind: 'error', title: t('common.error'), subtitle: t('template_details.error_save_prompt') });
    } finally {
      setIsGenerating(false);
    }
  };

  // Handle Delete Prompt (Open Modal)
  const handleDeletePrompt = (promptId) => {
    setPromptToDelete(promptId);
    setIsDeleteModalOpen(true);
  };

  // Confirm Delete
  const confirmDeletePrompt = async () => {
    if (!promptToDelete) return;
    setIsDeletingPrompt(true);
    try {
      await deletePrompt(promptToDelete);
      setPrompts(prompts.filter(p => p.id !== promptToDelete));
      addNotification({ kind: 'success', title: t('common.success'), subtitle: t('template_details.success_delete_prompt') });
      setIsDeleteModalOpen(false); // Close ONLY on success or handle error carefully
    } catch (error) {
      console.error("Failed to delete prompt", error);
      addNotification({ kind: 'error', title: t('common.error'), subtitle: t('template_details.error_delete_prompt') });
    } finally {
      setIsDeletingPrompt(false);
      if (!isDeleteModalOpen) setPromptToDelete(null); // Clean up if closed
    }
  };

  // Handle Template Actions
  const handleShare = async () => {
    setIsSharing(true);
    try {
      // Create update payload with ALL required fields + new visibility
      const updateData = {
         template_id: template.id,
         owner_id: template.owner_id,
         title: template.title,
         description: template.description,
         category: template.category,
         tags: template.tags,
         visibility: "VISIBILITY_PUBLIC", // Change to public
         content: editContent // Keep current content
      };
        // Preserve language when changing visibility
        if (template && template.language) {
          updateData.language = template.language;
        }
      const res = await updateTemplate(template.id, updateData);
      setTemplate(res.data.template); // Update local state
      addNotification({ kind: 'success', title: t('common.success'), subtitle: t('template_details.success_share') });
    } catch (error) {
      console.error("Failed to share template", error);
      addNotification({ kind: 'error', title: t('common.error'), subtitle: t('template_details.error_share') });
    } finally {
      setIsSharing(false);
    }
  };

  const handleUnshare = async () => {
    setIsSharing(true);
    try {
      const updateData = {
         template_id: template.id,
         owner_id: template.owner_id,
         title: template.title,
         description: template.description,
         category: template.category,
         tags: template.tags,
         visibility: "VISIBILITY_PRIVATE", // Change to private
         content: editContent
      };
        // Preserve language when changing visibility
        if (template && template.language) {
          updateData.language = template.language;
        }
      const res = await updateTemplate(template.id, updateData);
      setTemplate(res.data.template);
      addNotification({ kind: 'success', title: t('common.success'), subtitle: t('template_details.success_unshare') });
    } catch (error) {
      console.error("Failed to unshare template", error);
      addNotification({ kind: 'error', title: t('common.error'), subtitle: t('template_details.error_unshare') });
    } finally {
      setIsSharing(false);
    }
  };

  const handleFork = () => {
    setIsForkModalOpen(true);
  };

  const confirmFork = async () => {
    setIsForking(true);
    try {
        const res = await forkTemplate(template.id);
        setIsForkModalOpen(false);
        addNotification({ kind: 'success', title: t('common.success'), subtitle: t('template_details.success_fork') });
        // Navigate to the new template
        // Assuming API returns { template: { id: ... }, version: ... }
        const newId = res.data.template.id;
        navigate(`/templates/${newId}`);
    } catch (error) {
        console.error("Failed to fork template", error);
        addNotification({ kind: 'error', title: t('common.error'), subtitle: t('template_details.error_fork') });
        setIsForkModalOpen(false); // Close on error too? Or let user retry? Carbon Modal usually closes.
    } finally {
        setIsForking(false);
    }
  };

  const confirmDeleteTemplate = async () => {
      setIsDeletingTemplate(true);
      try {
          await deleteTemplate(template.id);
          addNotification({ kind: 'success', title: t('common.success'), subtitle: t('template_details.success_delete') });
          // Navigate home after a short delay so user sees notification
          setTimeout(() => navigate('/'), 1000);
      } catch (error) {
          console.error("Failed to delete template", error);
          addNotification({ kind: 'error', title: t('common.error'), subtitle: t('template_details.error_delete') });
          setIsTemplateDeleteModalOpen(false); // Close on error
      } finally {
          setIsDeletingTemplate(false);
      }
  };

  // Copy to Clipboard
  const handleCopy = () => {
    navigator.clipboard.writeText(generatedContent).then(() => {
        addNotification({ kind: 'success', title: t('common.success'), subtitle: t('card.copied') });
    });
  };

  // Handle Load Prompt
  const handleLoadPrompt = (prompt) => {
      if (prompt.variables && prompt.variables.length > 0) {
        // Note: Variables may be returned as strings like "key:value" or as objects.
        const map = {};
        prompt.variables.forEach(v => {
          if (typeof v === 'string') {
            const idx = v.indexOf(':');
            if (idx > -1) {
              const key = v.slice(0, idx).trim();
              const val = v.slice(idx + 1).trim();
              map[key] = val;
            }
          } else if (v && typeof v === 'object') {
            // assume single-key object
            const keys = Object.keys(v);
            if (keys.length > 0) {
              map[keys[0]] = v[keys[0]];
            }
          }
        });
        setVariableValues(map);
      }
  };

  if (loading) return <Layout showCreateButton={false}><div className="loading">{t('common.loading')}</div></Layout>;
  if (!template) return <Layout showCreateButton={false}><div className="not-found">{t('template_details.not_found')}</div></Layout>;

  return (
    <Layout showSidebar={false} showCreateButton={false}>
      <div className="template-details-page">
        <div className="header-actions">
          <div className="title-section">
            <div className="title-row">
              <h2>{template.title}</h2>
              <div className="social-actions">
                <button
                  className={`social-btn like ${isLiked ? 'active' : ''}`}
                  onClick={handleLike}
                  title={isLiked ? t('template_details.unlike') : t('template_details.like')}
                >
                  <span className="icon">{isLiked ? '❤️' : '🤍'}</span>
                  <span className="count">{likesCount}</span>
                </button>
                <button
                  className={`social-btn favorite ${isFavorited ? 'active' : ''}`}
                  onClick={handleFavorite}
                  title={isFavorited ? t('template_details.unfavorite') : t('template_details.favorite')}
                >
                  <span className="icon">{isFavorited ? '⭐' : '☆'}</span>
                  <span className="count">{favoritesCount}</span>
                </button>
              </div>
            </div>
            {isEditingMetadata ? (
                <div className="meta-editor">
                    <div className="editor-row">
                        <label className="editor-label">{t('template_details.edit_title_desc') || "Title & Description"}</label>
                        <input
                            type="text"
                            value={editTitle}
                            onChange={e => setEditTitle(e.target.value)}
                            placeholder={t('create_template.title_placeholder') || 'Template Title'}
                            className="meta-input title-input"
                        />
                    </div>
                    <div className="editor-row">
                        <textarea
                            value={editDescription}
                            onChange={e => setEditDescription(e.target.value)}
                            placeholder={t('create_template.description_placeholder') || 'Description'}
                            className="meta-textarea desc-input"
                            rows={3}
                        />
                    </div>
                    <div className="editor-row">
                        <label className="editor-label">{t('template_details.edit_metadata') || "Category & Tags"}</label>
                        <select
                            value={editCategory}
                            onChange={e => setEditCategory(e.target.value)}
                            className="meta-select"
                        >
                            <option value="" disabled>{t('create_template.category_placeholder') || 'Select Category'}</option>
                            <option value="create_new">{t('create_template.create_new_category') || 'Create New...'}</option>
                            {/* Append current category if not in the fetched list (e.g. different language or custom) */}
                            {editCategory && editCategory !== 'create_new' && !categories.includes(editCategory) && (
                                <option key={editCategory} value={editCategory}>{editCategory}</option>
                            )}
                            {categories.map(cat => (
                                <option key={cat} value={cat}>{cat}</option>
                            ))}
                        </select>
                        {editCategory === 'create_new' && (
                             <input
                                 type="text"
                                 value={customCategory}
                                 onChange={e => setCustomCategory(e.target.value)}
                                 placeholder={t('create_template.ph_new_category')}
                                 className="meta-input mt-2"
                                 style={{ marginTop: '0.5rem' }}
                             />
                        )}
                    </div>
                    <div className="editor-row tags-editor">
                        <div className="tags-list">
                            {editTags.map(tag => (
                                <span key={tag} className="tag-edit-chip">
                                    {tag} <button onClick={() => handleRemoveTag(tag)} type="button" aria-label="Remove tag">
                                        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                            <line x1="18" y1="6" x2="6" y2="18"></line>
                                            <line x1="6" y1="6" x2="18" y2="18"></line>
                                        </svg>
                                    </button>
                                </span>
                            ))}
                        </div>
                        <input
                            type="text"
                            value={tagInput}
                            onChange={e => setTagInput(e.target.value)}
                            onKeyDown={handleTagInputKeyDown}
                            placeholder={t('create_template.tags_placeholder') || 'Add tag...'}
                            className="tag-input"
                        />
                        <button onClick={handleAddTag} className="add-tag-btn" type="button" aria-label="Add tag">
                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                <line x1="12" y1="5" x2="12" y2="19"></line>
                                <line x1="5" y1="12" x2="19" y2="12"></line>
                            </svg>
                        </button>
                    </div>
                    <div className="editor-actions">
                        <button className="save-meta-btn" onClick={handleSaveMetadata} disabled={isSaving}>
                            {t('common.save') || 'Save'}
                        </button>
                        <button className="cancel-meta-btn" onClick={() => setIsEditingMetadata(false)} disabled={isSaving}>
                            {t('common.cancel') || 'Cancel'}
                        </button>
                    </div>
                </div>
            ) : (
                <div className="meta">
                    <span>By {template.owner_id}</span>
                    <span>•</span>
                    <span className={`visibility ${template.visibility.toLowerCase()}`}>
                    {template.visibility === 'VISIBILITY_PUBLIC'
                        ? t('create_template.visibility_public')
                        : t('create_template.visibility_private')}
                    </span>
                    {template.category && (
                        <>
                            <span>•</span>
                            <span className="category">{template.category}</span>
                        </>
                    )}
                    {template.tags && template.tags.map(tag => (
                        <span key={tag} className="tag">#{tag}</span>
                    ))}

                    {user && user.id === template.owner_id && (
                       <button onClick={handleEditMetadata} className="icon-btn edit-meta-btn" title={t('template_details.edit_metadata') || "Edit Metadata"}>
                           <span className="icon">
                               <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                   <path d="M12 20h9"></path>
                                   <path d="M16.5 3.5a2.121 2.121 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z"></path>
                               </svg>
                           </span>
                       </button>
                    )}
                </div>
            )}
            <p className="template-description">{template.description}</p>
          </div>

          {user && (
              <div className="template-actions">
                  {user.id === template.owner_id ? (
                      <>
                        {template.visibility === 'VISIBILITY_PRIVATE' ? (
                            <button className="action-btn share" onClick={handleShare} disabled={isSharing}>
                                {isSharing ? 'Processing...' : t('template_details.share_public')}
                            </button>
                        ) : (
                            <button className="action-btn unshare" onClick={handleUnshare} disabled={isSharing}>
                                {isSharing ? 'Processing...' : t('template_details.unshare_private')}
                            </button>
                        )}
                        <button className="action-btn delete" onClick={() => setIsTemplateDeleteModalOpen(true)}>{t('template_details.delete_template')}</button>
                      </>
                  ) : (
                      <button className="action-btn fork" onClick={handleFork}>{t('template_details.fork_template')}</button>
                  )}
              </div>
          )}
        </div>

        <div className="content-layout">
          <div className="main-column">
            {/* Version Selection & Editor */}
            <div className="section template-content">
              <h3>{t('template_details.content_title')}</h3>

              <div className="version-selector">
                <label>{t('template_details.version_label')}</label>
                <VersionSelect
                  options={versions}
                  value={selectedVersionId}
                  onChange={(vId) => {
                    setSelectedVersionId(vId);
                    const version = versions.find(v => v.id === vId);
                    if (version) setEditContent(version.content);
                  }}
                  disabled={isEditing}
                />
              </div>

              {isEditing ? (
                  <textarea
                    value={editContent}
                    onChange={(e) => setEditContent(e.target.value)}
                  />
              ) : (
                  <div className="preview-area">
                      <pre>{versions.find(v => v.id === selectedVersionId)?.content}</pre>
                  </div>
              )}

              <div className="actions">
                {isEditing ? (
                    <>
                        <button className="cancel-btn" onClick={() => setIsEditing(false)} disabled={isSaving}>{t('common.cancel')}</button>
                    <button className="save-btn" onClick={handleSaveContent} disabled={isSaveNewVersionDisabled}>
                      {isSaving ? t('common.saving') : t('template_details.save_new_version')}
                    </button>
                    </>
                ) : (
                    user && user.id === template.owner_id && (
                        <button className="edit-btn" onClick={() => setIsEditing(true)}>{t('template_details.edit_template')}</button>
                    )
                )}
              </div>
            </div>

            {/* Prompt Generator */}
            <div className="section prompt-generator">
                <h3>{t('template_details.generate_prompt')}</h3>

                {uniqueNames.length > 0 ? (
                  <div className="variables-form">
                    {uniqueNames.map((name) => (
                      <div key={name} className="form-group">
                        <label>{name}</label>
                        <input
                          type="text"
                          value={variableValues[name] || ''}
                          onChange={(e) => handleVariableChange(name, e.target.value)}
                          placeholder={t('template_details.enter_value')}
                        />
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="no-vars">{t('template_details.no_vars')}</p>
                )}

                <label className="result-label">{t('template_details.result_preview')}</label>
                <div className="preview-area">
                    <pre>{generatedContent}</pre>
                </div>

                <div className="generator-actions">
                    <button className="secondary" onClick={handleCopy}>{t('card.copy')}</button>
                    <button className="primary" onClick={handleCreatePrompt} disabled={isGenerating || isEditing}>
                      {isGenerating ? t('common.saving') : t('template_details.save_prompt')}
                    </button>
                </div>
            </div>

            {/* Template Alias Manager (Only visible to owner) */}
            {isOwner && (
              <div className="section template-aliases-section">
                <TemplateAliasManager templateId={id} versions={versions} />
              </div>
            )}
          </div>

          <div className="sidebar-column">
            <div className="instruction-card">
                <h4>{t('template_details.how_to_use')}</h4>
                <p dangerouslySetInnerHTML={{ __html: t('template_details.instruction_1') }}></p>
                <p dangerouslySetInnerHTML={{ __html: t('template_details.instruction_2') }}></p>
                <p>{t('template_details.instruction_3')}</p>
            </div>

            <div className="sdk-instruction-card">
                <h4>{t('template_details.sdk_integration') || 'SDK Integration'}</h4>
                <p className="sdk-desc">{t('template_details.sdk_desc') || 'Use this template directly via SDK in your code:'}</p>
                <div className="code-block-wrapper">
                  <pre>
                    <code>
{`from openprompts import OpenPromptsClient

client = OpenPromptsClient(
    base_url="https://api.yourdomain.com",
    api_key="YOUR_API_KEY"
)

prompt = client.get_prompt(
    template_id="${template.id}",
    prompt_tag="latest",
    variables={
${uniqueNames.map(name => `        "${name}": "value"`).join(',\n') || '        # no variables required'}
    }
)
print(prompt)`}
                    </code>
                  </pre>
                  <button className="copy-code-btn" onClick={() => {
                      const codeStr = `from openprompts import OpenPromptsClient\n\nclient = OpenPromptsClient(\n    base_url="https://api.yourdomain.com",\n    api_key="YOUR_API_KEY"\n)\n\nprompt = client.get_prompt(\n    template_id="${template.id}",\n    prompt_tag="latest",\n    variables={\n${uniqueNames.map(name => `        "${name}": "value"`).join(',\n') || '        # no variables required'}\n    }\n)\nprint(prompt)`;
                      navigator.clipboard.writeText(codeStr).then(() => {
                        addNotification({ kind: 'success', title: t('common.success'), subtitle: t('card.copied') });
                      });
                  }} title={t('card.copy') || 'Copy code'}>
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                        <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                        <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
                    </svg>
                  </button>
                </div>
            </div>

            <div className="prompt-history">
                <h3>{t('template_details.saved_prompts')}</h3>
                <div className="prompt-list">
                    {(() => {
                      const filtered = prompts.filter(p => p.version_id === selectedVersionId);
                      if (filtered.length === 0) {
                        return <div className="no-prompts">{t('template_details.no_saved_prompts')}</div>;
                      }
                      return filtered.map(p => {
                        // Parse variables into {key, value} pairs for separate display
                        const parseVars = (vars) => {
                          if (!vars) return [];
                          const pairs = [];
                          try {
                            if (typeof vars === 'object' && !Array.isArray(vars)) {
                                Object.keys(vars).forEach(k => { pairs.push({ key: k, value: vars[k] }); });
                            } else if (Array.isArray(vars)) {
                              vars.forEach((v) => {
                                if (typeof v === 'string') {
                                  const idx = v.indexOf(':');
                                  if (idx > -1) {
                                    pairs.push({ key: v.slice(0, idx).trim(), value: v.slice(idx + 1).trim() });
                                  } else {
                                    pairs.push({ key: v, value: '' });
                                  }
                                } else if (v && typeof v === 'object') {
                                  const k = Object.keys(v)[0];
                                  pairs.push({ key: k, value: v[k] });
                                }
                              });
                            }
                          } catch (e) {
                            // fallback: empty
                          }
                          return pairs;
                        };

                        const varPairs = parseVars(p.variables);

                        return (
                          <div key={p.id} className="prompt-item">
                            <div className="prompt-meta">
                              {new Date(p.created_at).toLocaleDateString()}
                            </div>
                            <div className="prompt-vars">
                              {varPairs.length === 0 ? (
                                <div className="no-vars-small">{t('common.none')}</div>
                              ) : (
                                varPairs.map((pair, i) => (
                                  <div key={i} className="var-chip" title={`${pair.key}: ${pair.value}`}>
                                    <div className="var-key">{pair.key}</div>
                                    <div className="var-sep">:</div>
                                    <div className="var-value">{pair.value}</div>
                                  </div>
                                ))
                              )}
                            </div>
                            <div className="prompt-actions">
                              <button
                                className="load-btn"
                                onClick={() => handleLoadPrompt(p)}
                              >
                                {t('template_details.load')}
                              </button>
                              <button
                                className="delete-btn"
                                onClick={() => handleDeletePrompt(p.id)}
                              >
                                {t('common.delete')}
                              </button>
                            </div>
                          </div>
                        );
                      });
                    })()}
                </div>
            </div>
          </div>
        </div>
      </div>

      <Modal
        open={isDeleteModalOpen}
        className="fork-modal"
        modalHeading={t('template_details.delete_prompt_title')}
        modalLabel={t('common.confirmation')}
        primaryButtonText={isDeletingPrompt ? t('common.deleting') : t('common.delete')}
        primaryButtonDisabled={isDeletingPrompt}
        secondaryButtonText={t('common.cancel')}
        danger
        onRequestClose={() => setIsDeleteModalOpen(false)}
        onRequestSubmit={confirmDeletePrompt}
      >
        <p>{t('template_details.delete_prompt_confirm')}</p>
      </Modal>

      <Modal
        open={isTemplateDeleteModalOpen}
        className="fork-modal"
        modalHeading={t('template_details.delete_template_title')}
        modalLabel={t('common.confirmation')}
        primaryButtonText={isDeletingTemplate ? t('common.deleting') : t('common.delete')}
        primaryButtonDisabled={isDeletingTemplate}
        secondaryButtonText={t('common.cancel')}
        danger
        onRequestClose={() => setIsTemplateDeleteModalOpen(false)}
        onRequestSubmit={confirmDeleteTemplate}
      >
        <p>{t('template_details.delete_template_confirm')}</p>
      </Modal>

      <Modal
        open={isForkModalOpen}
        className="fork-modal"
        modalHeading={t('template_details.fork_template_title')}
        modalLabel={t('common.confirmation')}
        primaryButtonText={isForking ? t('common.saving') : t('template_details.fork')}
        primaryButtonDisabled={isForking}
        secondaryButtonText={t('common.cancel')}
        onRequestClose={() => setIsForkModalOpen(false)}
        onRequestSubmit={confirmFork}
      >
        <p>{t('template_details.fork_template_confirm')}</p>
      </Modal>
    </Layout>
  );
};

export default TemplateDetails;
