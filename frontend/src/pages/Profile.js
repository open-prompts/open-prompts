import React, { useState, useEffect } from 'react';
import { useSelector, useDispatch } from 'react-redux';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { TextInput, PasswordInput, Button } from '@carbon/react';
import { useNotification } from '../context/NotificationContext';
import { Edit, Save, Close } from '@carbon/icons-react';
import { updateProfile, getProfile } from '../services/api';
import { loginSuccess } from '../store/authSlice';
import Header from '../components/Header';
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
  const [avatar, setAvatar] = useState('');
  const [displayAvatar, setDisplayAvatar] = useState('');
  const [isEditing, setIsEditing] = useState(false);
  const [isSaving, setIsSaving] = useState(false);

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

  return (
    <div className="layout">
      <Header />
      <div className="profile-page">
        <div className="profile-container">
          <div className="profile-header">
             <h2>{t('header.profile')}</h2>
          </div>

          {!isEditing ? (
              <div className="profile-view">
                  <div className="avatar-section">
                      <div className="avatar-display">
                          {displayAvatar ? (
                              <img src={displayAvatar} alt="Avatar" />
                          ) : (
                              <div className="avatar-placeholder">
                                  <span>{displayName ? displayName.charAt(0).toUpperCase() : 'U'}</span>
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

                  <div className="action-section">
                      <Button renderIcon={Edit} onClick={toggleEdit}>{t('profile.edit_profile')}</Button>
                  </div>
              </div>
          ) : (
              <form onSubmit={handleSubmit} className="profile-edit-form">
                  <div className="form-group avatar-upload-group">
                      <label>{t('profile.avatar')}</label>
                      <div className="avatar-upload-container">
                          <div className="avatar-preview">
                              {displayAvatar ? (
                                  <img src={displayAvatar} alt="Avatar Preview" />
                              ) : (
                                  <div className="avatar-placeholder">
                                      <span>{displayName ? displayName.charAt(0).toUpperCase() : 'U'}</span>
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
                              <label htmlFor="avatar-upload" className="cds--btn cds--btn--secondary">
                                  {t('profile.change_avatar')}
                              </label>
                          </div>
                      </div>
                  </div>

                  <div className="form-group">
                      <TextInput
                          id="displayName"
                          labelText={t('register.display_name')}
                          value={displayName}
                          onChange={(e) => setDisplayName(e.target.value)}
                          placeholder={t('profile.ph_display_name')}
                      />
                  </div>

                  <div className="form-group">
                      <PasswordInput
                          id="password"
                          labelText={t('profile.new_password')}
                          value={password}
                          onChange={(e) => setPassword(e.target.value)}
                          placeholder={t('profile.ph_new_password')}
                          autoComplete="new-password"
                      />
                  </div>

                  <div className="form-actions">
                      <Button type="submit" renderIcon={Save} disabled={isSaving}>
                        {isSaving ? t('common.saving') : t('common.save_changes')}
                      </Button>
                      <Button kind="ghost" renderIcon={Close} onClick={toggleEdit}>{t('common.cancel')}</Button>
                  </div>
              </form>
          )}
        </div>
      </div>
    </div>
  );
};

export default Profile;
