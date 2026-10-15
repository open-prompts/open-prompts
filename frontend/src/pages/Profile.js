import React, { useState, useEffect } from 'react';
import { useSelector, useDispatch } from 'react-redux';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useNotification } from '../context/NotificationContext';
import { EditIcon, SaveIcon, CloseIcon } from '../components/ui/Icons';
import { updateProfile, getProfile } from '../services/api';
import { loginSuccess } from '../store/authSlice';
import Header from '../components/Header';
import ShowWarnedToggle from '../components/warnings/ShowWarnedToggle';
import './Profile.scss';

/**
 * Profile page component.
 * Allows users to view and update their profile.
 */
const Profile = () => {
  const { t } = useTranslation();
  const { user, token } = useSelector((state) => state.auth);
  const dispatch = useDispatch();
  const navigate = useNavigate();
  const { addNotification } = useNotification();

  const [displayName, setDisplayName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [avatar, setAvatar] = useState('');
  const [displayAvatar, setDisplayAvatar] = useState('');
  const [isEditing, setIsEditing] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [showWarnedContent, setShowWarnedContent] = useState(false);
  const [savingWarnedPref, setSavingWarnedPref] = useState(false);

  useEffect(() => {
    if (!token) {
      navigate('/login');
      return;
    }

    const fetchProfile = async () => {
        try {
            const response = await getProfile();
            const data = response.data;
            setDisplayName(data.display_name || '');
            setEmail(data.email || '');
            setAvatar(data.avatar || '');
            setDisplayAvatar(data.avatar || '');
            setShowWarnedContent(!!data.show_warned_content);

            if (user) {
                 dispatch(loginSuccess({
                     user: { ...user, ...data, displayName: data.display_name },
                     token
                 }));
            }
        } catch (err) {
            console.error("Failed to fetch profile", err);
            addNotification({ kind: 'error', title: t('common.error'), subtitle: t('profile.error_fetch') });
        }
    };

    fetchProfile();
    // eslint-disable-next-line
  }, [token, navigate, dispatch]);

  const handleAvatarChange = (e) => {
    const file = e.target.files[0];
    if (file) {
      const reader = new FileReader();
      reader.onloadend = () => {
        setAvatar(reader.result);
        setDisplayAvatar(reader.result);
      };
      reader.readAsDataURL(file);
    }
  };

  const validatePassword = (pwd) => {
    if (pwd.length <= 8) return false;
    let complexity = 0;
    if (/[a-z]/.test(pwd)) complexity++;
    if (/[A-Z]/.test(pwd)) complexity++;
    if (/[0-9]/.test(pwd)) complexity++;
    return complexity >= 2;
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setIsSaving(true);
    try {
      const updatedData = {
        display_name: displayName,
        avatar: avatar,
      };

      if (password) {
        if (!validatePassword(password)) {
          addNotification({ kind: 'error', title: t('common.error'), subtitle: t('register.password_error') });
          setIsSaving(false);
          return;
        }
        updatedData.password = password;
      }

      const response = await updateProfile(updatedData);

      const updatedUser = {
        ...user,
        displayName: response.data.display_name,
        avatar: response.data.avatar,
      };

      dispatch(loginSuccess({ user: updatedUser, token: token }));
      addNotification({ kind: 'success', title: t('common.success'), subtitle: t('profile.success_update') });
      setPassword('');
      setIsEditing(false);
    } catch (err) {
      console.error(err);
      addNotification({ kind: 'error', title: t('common.error'), subtitle: t('profile.error_update') });
    } finally {
      setIsSaving(false);
    }
  };

  const toggleEdit = () => {
      setIsEditing(!isEditing);
  };

  // Persist the show_warned_content viewer preference server-side (FR-5).
  const handleWarnedPrefChange = async (next) => {
    setSavingWarnedPref(true);
    try {
      const response = await updateProfile({ show_warned_content: next });
      setShowWarnedContent(!!response.data.show_warned_content);
      if (user) {
        dispatch(loginSuccess({
          user: { ...user, show_warned_content: !!response.data.show_warned_content },
          token,
        }));
      }
      addNotification({ kind: 'success', title: t('common.success'), subtitle: t('profile.success_update') });
    } catch (err) {
      console.error(err);
      setShowWarnedContent(!next); // revert
      addNotification({ kind: 'error', title: t('common.error'), subtitle: t('profile.error_update') });
    } finally {
      setSavingWarnedPref(false);
    }
  };

  return (
    <div className="layout">
      <Header />
      <div className="profile-page">
        <div className="profile-container">
          <div className="profile-header">
             <h2>{t('header.profile')}</h2>
          </div>

          {!isEditing ? (
              <div className="profile-view card">
                  <div className="avatar-section">
                      <div className="avatar-display">
                          {displayAvatar ? (
                              <img src={displayAvatar} alt={t('common.avatar_alt')} />
                          ) : (
                              <div className="avatar-placeholder">
                                  <span>{displayName ? displayName.charAt(0).toUpperCase() : t('common.initial_fallback')}</span>
                              </div>
                          )}
                      </div>
                  </div>

                  <div className="info-section">
                      <div className="info-group">
                          <label>{t('register.display_name')}</label>
                          <div className="value">{displayName || t('profile.not_set')}</div>
                      </div>
                      <div className="info-group">
                          <label>{t('register.email')}</label>
                          <div className="value">{email}</div>
                      </div>
                  </div>

                  <div className="content-preferences-section">
                      <h3 className="content-preferences-title">{t('warnings.settings.title')}</h3>
                      <ShowWarnedToggle
                        checked={showWarnedContent}
                        saving={savingWarnedPref}
                        onChange={handleWarnedPrefChange}
                      />
                  </div>

                  <div className="action-section">
                      <button className="btn btn-secondary" onClick={toggleEdit}>
                        <EditIcon size={16} />
                        {t('profile.edit_profile')}
                      </button>
                  </div>
              </div>
          ) : (
              <form onSubmit={handleSubmit} className="profile-edit-form card">
                  <div className="form-group avatar-upload-group">
                      <label className="field-label">{t('profile.avatar')}</label>
                      <div className="avatar-upload-container">
                          <div className="avatar-preview">
                              {displayAvatar ? (
                                  <img src={displayAvatar} alt={t('common.avatar_preview_alt')} />
                              ) : (
                                  <div className="avatar-placeholder">
                                      <span>{displayName ? displayName.charAt(0).toUpperCase() : t('common.initial_fallback')}</span>
                                  </div>
                              )}
                          </div>
                          <div className="file-input-wrapper">
                              <input
                                  type="file"
                                  id="avatar-upload"
                                  accept="image/*"
                                  onChange={handleAvatarChange}
                                  className="hidden-file-input"
                              />
                              <label htmlFor="avatar-upload" className="btn btn-secondary btn-sm">
                                  {t('profile.change_avatar')}
                              </label>
                          </div>
                      </div>
                  </div>

                  <div className="form-group">
                      <label className="field-label" htmlFor="displayName">{t('register.display_name')}</label>
                      <input
                          id="displayName"
                          type="text"
                          className="input"
                          value={displayName}
                          onChange={(e) => setDisplayName(e.target.value)}
                          placeholder={t('profile.ph_display_name')}
                      />
                  </div>

                  <div className="form-group">
                      <label className="field-label" htmlFor="password">{t('profile.new_password')}</label>
                      <div className="password-wrap">
                          <input
                              id="password"
                              type={showPassword ? 'text' : 'password'}
                              className="input"
                              value={password}
                              onChange={(e) => setPassword(e.target.value)}
                              placeholder={t('profile.ph_new_password')}
                              autoComplete="new-password"
                          />
                          <button
                            type="button"
                            className="password-toggle"
                            onClick={() => setShowPassword(s => !s)}
                            aria-label={showPassword ? t('common.hide_password') : t('common.show_password')}
                            tabIndex={-1}
                          >
                            {showPassword ? '🙈' : '👁'}
                          </button>
                      </div>
                  </div>

                  <div className="form-actions">
                      <button type="submit" className="btn btn-primary" disabled={isSaving}>
                        <SaveIcon size={16} />
                        {isSaving ? t('common.saving') : t('common.save_changes')}
                      </button>
                      <button type="button" className="btn btn-ghost" onClick={toggleEdit}>
                        <CloseIcon size={16} />
                        {t('common.cancel')}
                      </button>
                  </div>
              </form>
          )}
        </div>
      </div>
    </div>
  );
};

export default Profile;
