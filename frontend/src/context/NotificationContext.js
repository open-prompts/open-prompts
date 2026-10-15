import React, { createContext, useState, useContext, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { CheckIcon, AlertIcon, InfoIcon, CloseIcon } from '../components/ui/Icons';
import './NotificationContext.scss';

const NotificationContext = createContext(null);

export const useNotification = () => {
    const context = useContext(NotificationContext);
    if (!context) {
        throw new Error('useNotification must be used within a NotificationProvider');
    }
    return context;
};

const ICONS = {
    success: CheckIcon,
    error: AlertIcon,
    warning: AlertIcon,
    info: InfoIcon,
};

export const NotificationProvider = ({ children }) => {
    const { t } = useTranslation();
    const [notifications, setNotifications] = useState([]);

    const removeNotification = useCallback((id) => {
        setNotifications((prev) => prev.filter((n) => n.id !== id));
    }, []);

    const addNotification = useCallback((notification) => {
        const id = Date.now().toString() + Math.random().toString(36).substr(2, 9);
        const newNotification = { ...notification, id };

        setNotifications((prev) => [...prev, newNotification]);

        // Auto-hide
        const timeout = notification.timeout || 3000;
        if (timeout > 0) {
            setTimeout(() => {
                removeNotification(id);
            }, timeout);
        }
    }, [removeNotification]);

    return (
        <NotificationContext.Provider value={{ addNotification }}>
            {children}
            <div
                className="notification-container"
                role="region"
                aria-label={t('a11y.notifications_region')}
                aria-live="polite"
            >
                {notifications.map((notification) => {
                    const kind = notification.kind || 'info';
                    const IconComponent = ICONS[kind] || InfoIcon;
                    return (
                        <div
                            key={notification.id}
                            className={`notification-toast ${kind}`}
                            role={kind === 'error' ? 'alert' : 'status'}
                        >
                            <span className="notification-icon">
                                <IconComponent size={18} />
                            </span>
                            <div className="notification-content">
                                <div className="notification-title">{notification.title}</div>
                                {notification.subtitle && (
                                    <div className="notification-subtitle">{notification.subtitle}</div>
                                )}
                                {notification.caption && (
                                    <div className="notification-subtitle">{notification.caption}</div>
                                )}
                            </div>
                            <button
                                type="button"
                                className="notification-close"
                                onClick={() => removeNotification(notification.id)}
                                aria-label={t('common.close_notification')}
                            >
                                <CloseIcon size={16} />
                            </button>
                        </div>
                    );
                })}
            </div>
        </NotificationContext.Provider>
    );
};
