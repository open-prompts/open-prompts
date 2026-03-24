import React, { useEffect } from 'react';
import { useSelector } from 'react-redux';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useNotification } from '../context/NotificationContext';
import Header from '../components/Header';
import APIKeyManager from '../components/APIKeyManager';
import './APIKeys.scss';

const APIKeys = () => {
    const { t } = useTranslation();
    const { token } = useSelector((state) => state.auth);
    const navigate = useNavigate();
    const { addNotification } = useNotification();

    useEffect(() => {
        if (!token) {
            navigate('/login');
        }
    }, [token, navigate]);

    return (
        <React.Fragment>
            <Header />
            <div className="api-keys-page">
                <div className="api-keys-container">
                    <h1>{t('api_keys.title', 'API Keys Management')}</h1>
                    <p className="page-description">
                        {t('api_keys.description', 'Manage your API keys to access the service programmatically.')}
                    </p>
                    <div className="api-key-manager-wrapper">
                        <APIKeyManager notification={addNotification} />
                    </div>
                </div>
            </div>
        </React.Fragment>
    );
};

export default APIKeys;
