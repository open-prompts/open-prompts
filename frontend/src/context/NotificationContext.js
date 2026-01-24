import React, { createContext, useState, useContext, useCallback } from 'react';
import { ToastNotification } from '@carbon/react';
import './NotificationContext.scss';

const NotificationContext = createContext(null);

export const useNotification = () => {
    const context = useContext(NotificationContext);
    if (!context) {
        throw new Error('useNotification must be used within a NotificationProvider');
    }
    return context;
};

export const NotificationProvider = ({ children }) => {
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
            <div className="notification-container">
                {notifications.map((notification) => (
                    <ToastNotification
                        key={notification.id}
                        kind={notification.kind || 'info'}
                        title={notification.title}
                        subtitle={notification.subtitle}
                        caption={notification.caption}
                        onClose={() => removeNotification(notification.id)}
                        lowContrast={true}
                        className={`notification-toast ${notification.kind || 'info'}`}
                        style={{ marginBottom: '0' }}
                    />
                ))}
            </div>
        </NotificationContext.Provider>
    );
};
