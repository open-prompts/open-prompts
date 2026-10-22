import React, { useState, useEffect, useMemo, useRef } from 'react';
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
  getCategories,
  recordTemplateEvent
} from '../services/api';
import Modal from '../components/ui/Modal';
import { HeartIcon, StarIcon, ClockIcon, AlertIcon } from '../components/ui/Icons';
import { useNotification } from '../context/NotificationContext';
import Layout from '../components/Layout';
import TemplateAliasManager from '../components/TemplateAliasManager';
import VersionHistoryPanel from '../components/VersionHistoryPanel';
import DiffModal from '../components/DiffModal';
import RestoreConfirmModal from '../components/RestoreConfirmModal';
import ChangeNoteInput from '../components/ChangeNoteInput';
import PublishReviewModal from '../components/review/PublishReviewModal';
import ContentInterstitial, { isWarnedRevealed } from '../components/warnings/ContentInterstitial';
import WarningCheckboxGroup from '../components/warnings/WarningCheckboxGroup';
import OwnerChip from '../components/OwnerChip';
import ExportButton from '../components/ExportButton';
import ReportMenu from '../components/ReportMenu';
import PublicCounters from '../components/analytics/PublicCounters';
import OwnerStatsSection from '../components/analytics/OwnerStatsSection';
import RelatedRail from '../components/related/RelatedRail';
import MoreFromAuthorRow from '../components/related/MoreFromAuthorRow';
import RatingWidget from '../components/RatingWidget';
import RatingHistogram from '../components/RatingHistogram';
import { usePageAnnouncement } from '../utils/a11y';
import './TemplateDetails.scss';
import VersionSelect from '../components/VersionSelect';

const TemplateDetails = () => {
  const { t, i18n } = useTranslation();
  const { id } = useParams();
  const navigate = useNavigate();
  const { user } = useSelector((state) => state.auth);

  usePageAnnouncement('template_details.title');

  const [template, setTemplate] = useState(null);
  const [owner, setOwner] = useState(null);
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

  // Ratings
  const [ratingAvg, setRatingAvg] = useState(0);
  const [ratingCount, setRatingCount] = useState(0);
  const [myRating, setMyRating] = useState(0);
  const [ratingDistribution, setRatingDistribution] = useState([]);

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
  const [editNote, setEditNote] = useState('');

  // Version history state
  const [isHistoryOpen, setIsHistoryOpen] = useState(false);
  const [historyPageToken, setHistoryPageToken] = useState('');
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyError, setHistoryError] = useState(false);
  const [diffState, setDiffState] = useState(null); // { aId, bId } while modal open
  const [restoreTarget, setRestoreTarget] = useState(null); // version being restored
  const [isRestoring, setIsRestoring] = useState(false);

  // Generator state
  const [variableValues, setVariableValues] = useState({});

  // Delete Modal State
  const [isDeleteModalOpen, setIsDeleteModalOpen] = useState(false);
  const [promptToDelete, setPromptToDelete] = useState(null);

  // Template Delete Modal State
  const [isTemplateDeleteModalOpen, setIsTemplateDeleteModalOpen] = useState(false);

  // Template Fork Modal State
  const [isForkModalOpen, setIsForkModalOpen] = useState(false);

  // Pre-publication review: shown when a publish entered review
  const [isReviewModalOpen, setIsReviewModalOpen] = useState(false);

  // Metadata Edit State
  const [isEditingMetadata, setIsEditingMetadata] = useState(false);
  const [editCategory, setEditCategory] = useState('');
  const [editTags, setEditTags] = useState([]);
  const [editTitle, setEditTitle] = useState('');
  const [editDescription, setEditDescription] = useState('');
  const [tagInput, setTagInput] = useState('');
  const [categories, setCategories] = useState([]);
  const [customCategory, setCustomCategory] = useState('');

  // Content warnings: interstitial reveal state (session-scoped) + edit form.
  const [warnedRevealed, setWarnedRevealed] = useState(() => isWarnedRevealed(id));
  const [editWarnings, setEditWarnings] = useState([]);

  const { addNotification } = useNotification();

  // Fetch the first page of versions (server-truthful numbering after writes).
  const fetchVersionsFirstPage = async () => {
    setHistoryLoading(true);
    setHistoryError(false);
    try {
      const verRes = await listTemplateVersions(id, { page_size: 20 });
      const list = verRes.data.versions || [];
      setVersions(list);
      setHistoryPageToken(verRes.data.next_page_token || '');
      return list;
    } catch (error) {
      console.error('Failed to load version history', error);
      setHistoryError(true);
      return null;
    } finally {
      setHistoryLoading(false);
    }
  };

  // Initial Data Fetch
  useEffect(() => {
    const fetchData = async () => {
      setLoading(true);
      try {
        const [tplRes, verRes, pmtRes] = await Promise.all([
          getTemplate(id),
          listTemplateVersions(id, { page_size: 20 }),
          user ? listPrompts({ template_id: id, owner_id: user.id }) : Promise.resolve({ data: { prompts: [] } })
        ]);

        setTemplate(tplRes.data.template);
        setOwner(tplRes.data.owner || null);
        setVersions(verRes.data.versions || []);
        setHistoryPageToken(verRes.data.next_page_token || '');
        setPrompts(pmtRes.data.prompts || []);
        setEditWarnings(tplRes.data.template.content_warnings || []);

        setIsLiked(tplRes.data.template.is_liked || false);
        setIsFavorited(tplRes.data.template.is_favorited || false);
        setLikesCount(tplRes.data.template.like_count || 0);
        setFavoritesCount(tplRes.data.template.favorite_count || 0);

        setRatingAvg(tplRes.data.template.rating_avg || 0);
        setRatingCount(tplRes.data.template.rating_count || 0);
        setMyRating(tplRes.data.template.my_rating || 0);
        setRatingDistribution(tplRes.data.rating_distribution || []);

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

  // Prompt analytics: fire a view beacon once per mount (StrictMode-safe).
  const viewFiredRef = useRef(false);
  useEffect(() => {
    if (viewFiredRef.current) return;
    viewFiredRef.current = true;
    let sid = localStorage.getItem('op_analytics_sid');
    if (!sid) {
      const g = typeof crypto !== 'undefined' && crypto.randomUUID ? crypto.randomUUID() : '';
      sid = g || `sid-${Date.now()}-${Math.random()}`;
      localStorage.setItem('op_analytics_sid', sid);
    }
    recordTemplateEvent(id, { sessionId: sid }).catch(() => {});
  }, [id]);

  // Fetch categories when editing metadata
  useEffect(() => {
    if (isEditingMetadata) {
        const fetchCategories = async () => {
             try {
                 // Note: no language filter on purpose — categories are shared
                 // across UI languages so a template's category always shows up.
                 const res = await getCategories();
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
  }, [isEditingMetadata]);

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
              content: contentToSend,
              content_warnings: editWarnings
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
        content: editContent,
        change_note: editNote
      };
      // Ensure language is preserved when saving content
      if (template && template.language) {
        updateData.language = template.language;
      }

      const res = await updateTemplate(template.id, updateData);

      // Refresh from server so numbering and Latest badge stay truthful
      const list = await fetchVersionsFirstPage();
      const newVersion = res.data.new_version;
      if (newVersion) {
        setSelectedVersionId(newVersion.id);
      } else if (list && list.length > 0) {
        setSelectedVersionId(list[0].id);
      }
      setEditNote('');
      setIsEditing(false);
      addNotification({ kind: 'success', title: t('common.success'), subtitle: t('template_details.success_update') });
    } catch (error) {
      console.error("Failed to update template", error);
      addNotification({ kind: 'error', title: t('common.error'), subtitle: t('template_details.error_update') });
    } finally {
      setIsSaving(false);
    }
  };

  // Version history: load next page (append)
  const handleLoadMoreVersions = async () => {
    if (!historyPageToken) return;
    setHistoryLoading(true);
    try {
      const res = await listTemplateVersions(id, { page_size: 20, page_token: historyPageToken });
      setVersions((prev) => [...prev, ...(res.data.versions || [])]);
      setHistoryPageToken(res.data.next_page_token || '');
    } catch (error) {
      console.error('Failed to load more versions', error);
      addNotification({ kind: 'error', title: t('common.error'), subtitle: t('versioning.error.fetch') });
    } finally {
      setHistoryLoading(false);
    }
  };

  // Version history: select a version (single source of truth)
  const handleSelectVersion = (version) => {
    setSelectedVersionId(version.id);
    setEditContent(version.content);
  };

  // Restore flow: confirm modal -> PUT old content + canonical note
  const handleRestoreConfirm = async () => {
    if (!restoreTarget) return;
    setIsRestoring(true);
    try {
      const updateData = {
        template_id: template.id,
        owner_id: template.owner_id,
        title: template.title,
        description: template.description,
        visibility: template.visibility,
        category: template.category,
        tags: template.tags,
        content: restoreTarget.content,
        change_note: `Restored from v${restoreTarget.version}`
      };
      if (template && template.language) {
        updateData.language = template.language;
      }
      const res = await updateTemplate(template.id, updateData);
      const newVersion = res.data.new_version;

      const list = await fetchVersionsFirstPage();
      if (newVersion) {
        setSelectedVersionId(newVersion.id);
        setEditContent(newVersion.content);
      } else if (list && list.length > 0) {
        setSelectedVersionId(list[0].id);
        setEditContent(list[0].content);
      }
      setRestoreTarget(null);
      addNotification({
        kind: 'success',
        title: t('common.success'),
        subtitle: newVersion
          ? t('versioning.toast.restored', { version: newVersion.version })
          : t('template_details.success_update')
      });
    } catch (error) {
      console.error('Failed to restore version', error);
      addNotification({ kind: 'error', title: t('common.error'), subtitle: t('versioning.error.restore') });
    } finally {
      setIsRestoring(false);
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
      if (res.data.template && res.data.template.moderation_status === 'in_review') {
        // Gated creator: the publish entered pre-publication review.
        setIsReviewModalOpen(true);
      } else {
        addNotification({ kind: 'success', title: t('common.success'), subtitle: t('template_details.success_share') });
      }
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
      if (!prompt.variables) return;
      const map = {};
      // Backend returns variables as a JSON object (map<string, string>),
      // e.g. {"name": "John", "age": "30"}. Also handle legacy array format.
      if (typeof prompt.variables === 'object' && !Array.isArray(prompt.variables)) {
        Object.keys(prompt.variables).forEach(k => {
          map[k] = prompt.variables[k];
        });
      } else if (Array.isArray(prompt.variables)) {
        prompt.variables.forEach(v => {
          if (typeof v === 'string') {
            const idx = v.indexOf(':');
            if (idx > -1) {
              const key = v.slice(0, idx).trim();
              const val = v.slice(idx + 1).trim();
              map[key] = val;
            }
          } else if (v && typeof v === 'object') {
            const keys = Object.keys(v);
            if (keys.length > 0) {
              map[keys[0]] = v[keys[0]];
            }
          }
        });
      }
      if (Object.keys(map).length > 0) {
        setVariableValues(map);
      }
  };

  if (loading) return <Layout showCreateButton={false}><div className="loading">{t('common.loading')}</div></Layout>;
  if (!template) return <Layout showCreateButton={false}><div className="not-found">{t('template_details.not_found')}</div></Layout>;

  return (
    <Layout showSidebar={false} showCreateButton={false}>
      <div className="template-details-page">
        {isOwner && template.moderation_status === 'in_review' && (
          <div className="review-owner-banner review-owner-banner--in-review" role="status">
            <ClockIcon size={16} />
            <span>{t('review.banner.in_review')}</span>
          </div>
        )}
        {isOwner && template.moderation_status === 'rejected' && (
          <div className="review-owner-banner review-owner-banner--rejected" role="alert">
            <AlertIcon size={16} />
            <span>{t('review.banner.rejected')}</span>
            <button
              type="button"
              className="review-owner-banner__cta"
              onClick={() => setIsEditingMetadata(true)}
            >
              {t('review.cta.resubmit')}
            </button>
          </div>
        )}
        <div className="header-actions">
          <div className="title-section">
            <div className="title-row">
              <h1>{template.title}</h1>
              <div className="social-actions">
                <button
                  className={`social-btn like ${isLiked ? 'active' : ''}`}
                  onClick={handleLike}
                  aria-pressed={!!isLiked}
                  title={isLiked ? t('template_details.unlike') : t('template_details.like')}
                >
                  <HeartIcon size={17} filled={isLiked} />
                  <span className="count">{likesCount}</span>
                </button>
                <button
                  className={`social-btn favorite ${isFavorited ? 'active' : ''}`}
                  onClick={handleFavorite}
                  aria-pressed={!!isFavorited}
                  title={isFavorited ? t('template_details.unfavorite') : t('template_details.favorite')}
                >
                  <StarIcon size={17} filled={isFavorited} />
                  <span className="count">{favoritesCount}</span>
                </button>
                {user && (
                  <ReportMenu itemType="template" itemId={template.id} isOwner={isOwner} />
                )}
              </div>
              <PublicCounters views={template.view_count} uses={template.use_count} />
            </div>
            <div className="rating-block">
              <RatingWidget
                templateId={template.id}
                ratingAvg={ratingAvg}
                ratingCount={ratingCount}
                myRating={myRating}
                isOwner={isOwner}
                onChange={(data) => {
                  setRatingAvg(data.rating_avg);
                  setRatingCount(data.rating_count);
                  setMyRating(data.my_rating);
                }}
              />
              <RatingHistogram distribution={ratingDistribution} />
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
                                    {tag} <button onClick={() => handleRemoveTag(tag)} type="button" aria-label={t('common.remove_tag')}>
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
                        <button onClick={handleAddTag} className="add-tag-btn" type="button" aria-label={t('common.add_tag')}>
                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                                <line x1="12" y1="5" x2="12" y2="19"></line>
                                <line x1="5" y1="12" x2="19" y2="12"></line>
                            </svg>
                        </button>
                    </div>
                    <WarningCheckboxGroup
                      value={editWarnings}
                      locked={template.locked_warnings || []}
                      onChange={setEditWarnings}
                    />
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
                    {owner ? (
                      <OwnerChip owner={owner} locale={i18n.language || 'en'} />
                    ) : (
                      <span>{t('template_details.by', { owner: template.owner_id })}</span>
                    )}
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
                       <button onClick={handleEditMetadata} className="icon-btn edit-meta-btn" title={t('template_details.edit_metadata') || "Edit Metadata"} aria-label={t('template_details.edit_metadata') || "Edit Metadata"}>
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
                                {isSharing ? t('common.processing') : t('template_details.share_public')}
                            </button>
                        ) : (
                            <button className="action-btn unshare" onClick={handleUnshare} disabled={isSharing}>
                                {isSharing ? t('common.processing') : t('template_details.unshare_private')}
                            </button>
                        )}
                        <button className="action-btn delete" onClick={() => setIsTemplateDeleteModalOpen(true)}>{t('template_details.delete_template')}</button>
                        <ExportButton template={template} />
                      </>
                  ) : (
                      <button className="action-btn fork" onClick={handleFork}>{t('template_details.fork_template')}</button>
                  )}
              </div>
          )}
        </div>

        <div className="content-layout">
          <div className="main-column">
            {/* Content warnings interstitial: shown before content when the
                template has warnings and the viewer hasn't opted out/revealed. */}
            {template.content_warnings &&
              template.content_warnings.length > 0 &&
              !(user && user.show_warned_content) &&
              !warnedRevealed && (
                <ContentInterstitial
                  templateId={template.id}
                  warnings={template.content_warnings}
                  onReveal={() => setWarnedRevealed(true)}
                  onBack={() => navigate(-1)}
                />
              )}

            {/* Version Selection & Editor */}
            <div className="section template-content">
              <h3>{t('template_details.content_title')}</h3>

              <div className="version-selector">
                <label>{t('template_details.version_label')}</label>
                <VersionSelect
                  options={versions}
                  value={selectedVersionId}
                  onChange={(vId) => {
                    const version = versions.find(v => v.id === vId);
                    if (version) handleSelectVersion(version);
                  }}
                  disabled={isEditing}
                />
                <button
                  type="button"
                  className="btn-ghost btn-sm history-toggle"
                  aria-expanded={isHistoryOpen}
                  onClick={() => setIsHistoryOpen((s) => !s)}
                >
                  {isHistoryOpen ? t('versioning.hide_history') : t('versioning.history')}
                </button>
              </div>

              {isHistoryOpen ? (
                <VersionHistoryPanel
                  versions={versions}
                  loading={historyLoading}
                  error={historyError}
                  selectedVersionId={selectedVersionId}
                  isOwner={isOwner}
                  hasMore={!!historyPageToken}
                  onLoadMore={handleLoadMoreVersions}
                  onSelect={handleSelectVersion}
                  onCompare={(version) => {
                    const selected = versions.find(v => v.id === selectedVersionId);
                    const other = selected && selected.id !== version.id ? selected : versions.find(v => v.id !== version.id);
                    setDiffState({
                      aId: other ? other.id : version.id,
                      bId: version.id
                    });
                  }}
                  onRestore={(version) => setRestoreTarget(version)}
                  onRetry={fetchVersionsFirstPage}
                  onClose={() => setIsHistoryOpen(false)}
                />
              ) : isEditing ? (
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
                        <ChangeNoteInput value={editNote} onChange={setEditNote} disabled={isSaving} />
                        <button className="cancel-btn" onClick={() => setIsEditing(false)} disabled={isSaving}>{t('common.cancel')}</button>
                    <button className="save-btn" onClick={handleSaveContent} disabled={isSaveNewVersionDisabled}>
                      {isSaving ? t('common.saving') : t('template_details.save_new_version')}
                    </button>
                    </>
                ) : (
                    user && user.id === template.owner_id && (
                        <button className="edit-btn" onClick={() => { setEditNote(''); setIsEditing(true); }}>{t('template_details.edit_template')}</button>
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
                        <textarea
                          value={variableValues[name] || ''}
                          onChange={(e) => handleVariableChange(name, e.target.value)}
                          placeholder={t('template_details.enter_value')}
                          rows={3}
                          className="var-textarea"
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
                    <button className="secondary" onClick={handleCopy} aria-label={t('template_details.copy_code')}>{t('card.copy')}</button>
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
    base_url="https://awsomeprompt.top",
    api_key="YOUR_API_KEY"
)

prompt = client.get_prompt(
    template_id="${template.id}",
    prompt_tag="latest",
    variables={
${uniqueNames.map(name => `        "${name}": "value"`).join(',\n') || '        ' + t('template_details.sdk_no_vars_comment')}
    }
)
print(prompt)`}
                    </code>
                  </pre>
                  <button className="copy-code-btn" onClick={() => {
                      const codeStr = `from openprompts import OpenPromptsClient\n\nclient = OpenPromptsClient(\n    base_url="https://awsomeprompt.top",\n    api_key="YOUR_API_KEY"\n)\n\nprompt = client.get_prompt(\n    template_id="${template.id}",\n    prompt_tag="latest",\n    variables={\n${uniqueNames.map(name => `        "${name}": "value"`).join(',\n') || '        ' + t('template_details.sdk_no_vars_comment')}\n    }\n)\nprint(prompt)`;
                      navigator.clipboard.writeText(codeStr).then(() => {
                        addNotification({ kind: 'success', title: t('common.success'), subtitle: t('card.copied') });
                      });
                  }} title={t('card.copy') || 'Copy code'} aria-label={t('template_details.copy_code')}>
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
                              {new Date(p.created_at).toLocaleDateString(i18n.language)}
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

            {isOwner && <OwnerStatsSection templateId={id} />}

            {template.visibility === 'VISIBILITY_PUBLIC' && (
              <>
                <RelatedRail
                  templateId={id}
                  category={template.category}
                  tags={template.tags}
                />
                <MoreFromAuthorRow
                  templateId={id}
                  authorName={template.owner_display_name || template.owner_id}
                />
              </>
            )}
          </div>
        </div>
      </div>

      <Modal
        open={isDeleteModalOpen}
        title={t('template_details.delete_prompt_title')}
        subtitle={t('common.confirmation')}
        primaryLabel={isDeletingPrompt ? t('common.deleting') : t('common.delete')}
        primaryDisabled={isDeletingPrompt}
        secondaryLabel={t('common.cancel')}
        danger
        onClose={() => setIsDeleteModalOpen(false)}
        onPrimary={confirmDeletePrompt}
      >
        <p className="modal-confirm-text">{t('template_details.delete_prompt_confirm')}</p>
      </Modal>

      <Modal
        open={isTemplateDeleteModalOpen}
        title={t('template_details.delete_template_title')}
        subtitle={t('common.confirmation')}
        primaryLabel={isDeletingTemplate ? t('common.deleting') : t('common.delete')}
        primaryDisabled={isDeletingTemplate}
        secondaryLabel={t('common.cancel')}
        danger
        onClose={() => setIsTemplateDeleteModalOpen(false)}
        onPrimary={confirmDeleteTemplate}
      >
        <p className="modal-confirm-text">{t('template_details.delete_template_confirm')}</p>
      </Modal>

      <Modal
        open={isForkModalOpen}
        title={t('template_details.fork_template_title')}
        subtitle={t('common.confirmation')}
        primaryLabel={isForking ? t('common.saving') : t('template_details.fork')}
        primaryDisabled={isForking}
        secondaryLabel={t('common.cancel')}
        onClose={() => setIsForkModalOpen(false)}
        onPrimary={confirmFork}
      >
        <p className="modal-confirm-text">{t('template_details.fork_template_confirm')}</p>
      </Modal>

      <PublishReviewModal
        open={isReviewModalOpen}
        onViewStatus={() => {
          setIsReviewModalOpen(false);
          navigate('/');
        }}
        onDone={() => setIsReviewModalOpen(false)}
      />

      <DiffModal
        key={diffState ? `${diffState.aId}-${diffState.bId}` : 'closed'}
        open={!!diffState}
        versions={versions}
        initialAId={diffState?.aId}
        initialBId={diffState?.bId}
        onClose={() => setDiffState(null)}
        onCopy={() => addNotification({ kind: 'success', title: t('common.success'), subtitle: t('versioning.toast.diff_copied') })}
      />

      <RestoreConfirmModal
        open={!!restoreTarget}
        version={restoreTarget}
        nextVersion={restoreTarget ? (versions[0]?.version || 0) + 1 : 0}
        inFlight={isRestoring}
        onCancel={() => setRestoreTarget(null)}
        onConfirm={handleRestoreConfirm}
      />
    </Layout>
  );
};

export default TemplateDetails;
