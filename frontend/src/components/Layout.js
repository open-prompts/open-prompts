import React, { useState } from 'react';
import { useSelector } from 'react-redux';
import { useTranslation } from 'react-i18next';
import { AddIcon } from './ui/Icons';
import Header from './Header';
import Sidebar from './Sidebar';
import SkipLink from './SkipLink';
import CreateTemplateModal from './CreateTemplateModal';
import './Layout.scss';

/**
 * Layout component.
 * Wraps the application content with a Header and Sidebar.
 * Displays a floating create button for authenticated users.
 * @param {Object} props - Component props
 * @param {React.ReactNode} props.children - The content to display
 * @param {Function} props.onFilterChange - Callback for sidebar filter changes
 * @param {boolean} props.showSidebar - Whether to show the sidebar (default: true)
 * @param {boolean} props.showCreateButton - Whether to show the create template button (default: true)
 * @param {Function} props.onCreateSuccess - Callback when a template is successfully created
 */
const Layout = ({
  children,
  onFilterChange,
  currentFilters,
  showSidebar = true,
  showCreateButton = true,
  onCreateSuccess,
  availableTags,
  categories
}) => {
  const { t } = useTranslation();
  const { isAuthenticated } = useSelector((state) => state.auth);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);

  const handleCreateClick = () => {
    setIsModalOpen(true);
  };

  const handleModalClose = () => {
    setIsModalOpen(false);
  };

  const handleModalSuccess = () => {
    console.log("Template created successfully");
    if (onCreateSuccess) {
      onCreateSuccess();
    }
  };

  const toggleMobileSidebar = () => {
    setMobileSidebarOpen(!mobileSidebarOpen);
  };

  return (
    <div className="app-layout">
      <SkipLink />
      <Header onMenuClick={toggleMobileSidebar} />
      <div className="app-body">
        {showSidebar && (
          <>
            <div
              className={`sidebar-backdrop ${mobileSidebarOpen ? 'visible' : ''}`}
              onClick={() => setMobileSidebarOpen(false)}
            />
            <Sidebar
              onFilterChange={onFilterChange}
              currentFilters={currentFilters}
              availableTags={availableTags}
              categories={categories}
              mobileOpen={mobileSidebarOpen}
              onClose={() => setMobileSidebarOpen(false)}
            />
          </>
        )}
        <main className="app-content" id="main-content" tabIndex={-1}>
          {children}
        </main>
      </div>

      {isAuthenticated && showCreateButton && (
        <>
          <button
            className="floating-create-btn"
            onClick={handleCreateClick}
            aria-label={t('header.create_template')}
            title={t('header.create_template')}
          >
            <AddIcon size={28} />
          </button>

          <CreateTemplateModal
            open={isModalOpen}
            onRequestClose={handleModalClose}
            onSuccess={handleModalSuccess}
          />
        </>
      )}
    </div>
  );
};

export default Layout;
