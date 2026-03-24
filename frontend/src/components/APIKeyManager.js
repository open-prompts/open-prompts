/* eslint-disable react-hooks/exhaustive-deps */
import React, { useState, useEffect } from 'react';
import {
  Button,
  Table,
  TableHead,
  TableRow,
  TableHeader,
  TableBody,
  TableCell,
  TableContainer,
  Modal,
  TextInput,
} from '@carbon/react';
import { Add, TrashCan, Copy } from '@carbon/icons-react';
import { useTranslation } from 'react-i18next';
import { listAPIKeys, createAPIKey, deleteAPIKey } from '../services/api';
import './APIKeyManager.scss';

const APIKeyManager = ({ notification }) => {
  const { t } = useTranslation();
  const [keys, setKeys] = useState([]);
  const [loading, setLoading] = useState(true);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [keyToDelete, setKeyToDelete] = useState(null);
  const [newKeyName, setNewKeyName] = useState('');
  const [generatedKey, setGeneratedKey] = useState(null);
  const [page] = useState(1);
  const [pageSize] = useState(10);

  const headers = [
    { key: 'name', header: t('api_keys.name', 'Name') },
    { key: 'prefix', header: t('api_keys.key', 'Key Prefix') },
    { key: 'created_at', header: t('api_keys.created_at', 'Created At') },
    { key: 'last_used_at', header: t('api_keys.last_used_at', 'Last Used') },
    { key: 'actions', header: '' },
  ];

  const fetchKeys = async () => {
    setLoading(true);
    try {
      const res = await listAPIKeys({ page_size: pageSize, page: page });
      const data = res.data.api_keys || [];
      const formatted = data.map(k => ({
        id: k.id,
        name: k.name,
        prefix: k.prefix,
        created_at: new Date(k.created_at).toLocaleString(),
        last_used_at: k.last_used_at ? new Date(k.last_used_at).toLocaleString() : '-',
      }));
      setKeys(formatted);
    } catch (err) {
      if (notification) notification({ kind: 'error', title: t('common.error'), subtitle: t('api_keys.error_fetch') });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchKeys();
  }, [page, pageSize]);

  const handleCreate = async () => {
    if (!newKeyName.trim()) return;
    try {
      const res = await createAPIKey({ name: newKeyName });
      setGeneratedKey(res.data.api_key);
      setNewKeyName('');
      fetchKeys(); // Refresh list
    } catch (err) {
      if (notification) notification({ kind: 'error', title: t('common.error'), subtitle: t('api_keys.error_create') });
    }
  };

  const handleDelete = async (id) => {
    try {
      await deleteAPIKey(id);
      fetchKeys();
      if (notification) notification({ kind: 'success', title: t('common.success'), subtitle: t('api_keys.success_delete') });
    } catch (err) {
      if (notification) notification({ kind: 'error', title: t('common.error'), subtitle: t('api_keys.error_delete') });
    }
  };

  const copyToClipboard = (text) => {
    navigator.clipboard.writeText(text);
    if (notification) notification({ kind: 'info', title: t('common.success'), subtitle: t('api_keys.success_copy') });
  };

  // Custom Close handler for Modal to clear state when fully closed
  const handleModalClose = () => {
      setIsModalOpen(false);
      // clear generated key after a delay or immediately if user closes
      setGeneratedKey(null);
      setNewKeyName('');
  }

  return (
    <div className="api-key-manager">
      <div className="header-section">
          <h3>{t('api_keys.list_title', 'Your API Keys')}</h3>
          <Button renderIcon={Add} onClick={() => setIsModalOpen(true)} size="sm">
            {t('api_keys.generate_new', 'Generate New Key')}
          </Button>
      </div>

      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              {headers.map((header) => (
                <TableHeader key={header.key}>{header.header}</TableHeader>
              ))}
            </TableRow>
          </TableHead>
          <TableBody>
            {keys.map((row) => (
              <TableRow key={row.id}>
                <TableCell>{row.name}</TableCell>
                <TableCell><span className="key-prefix">{row.prefix}...</span></TableCell>
                <TableCell>{row.created_at}</TableCell>
                <TableCell>{row.last_used_at}</TableCell>
                <TableCell>
                  <Button
                    hasIconOnly
                    renderIcon={TrashCan}
                    kind="ghost"
                    iconDescription={t('api_keys.delete')}
                    onClick={() => {
                        setKeyToDelete(row.id);
                        setDeleteModalOpen(true);
                    }}
                    size="sm"
                  />
                </TableCell>
              </TableRow>
            ))}
            {keys.length === 0 && !loading && (
                <TableRow>
                    <TableCell colSpan={5}>
                        <div className="empty-state">{t('api_keys.no_keys')}</div>
                    </TableCell>
                </TableRow>
            )}
            {loading && (
                <TableRow>
                    <TableCell colSpan={5}>
                        <div className="loading-state">{t('common.loading')}</div>
                    </TableCell>
                </TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>

      {/* Create Modal */}
      <Modal
        open={isModalOpen}
        modalHeading={!generatedKey ? t('api_keys.modal_create_title') : t('api_keys.modal_created_title')}
        primaryButtonText={!generatedKey ? t('api_keys.btn_generate') : t('api_keys.btn_done')}
        secondaryButtonText={!generatedKey ? t('common.cancel') : ""}
        onRequestClose={handleModalClose}
        onRequestSubmit={!generatedKey ? handleCreate : handleModalClose}
        danger={false}
        className="api-key-modal"
      >
        {!generatedKey ? (
            <TextInput
                id="key-name"
                labelText={t('api_keys.label_name')}
                placeholder={t('api_keys.placeholder_name')}
                value={newKeyName}
                onChange={(e) => setNewKeyName(e.target.value)}
            />
        ) : (
            <div className="generated-key-display">
                <p className="warning-text">
                    {t('api_keys.generated_warning')}
                </p>
                <div className="key-copy-row">
                    <div className="key-input-wrapper">
                        <TextInput
                            id="generated-key"
                            labelText={t('api_keys.label_key')}
                            value={generatedKey}
                            readOnly
                        />
                    </div>
                    <Button
                        hasIconOnly
                        renderIcon={Copy}
                        kind="ghost"
                        iconDescription={t('api_keys.copy_desc', 'Copy')}
                        onClick={() => copyToClipboard(generatedKey)}
                        className="copy-btn"
                    />
                </div>
            </div>
        )}
      </Modal>

      {/* Delete Confirmation Modal */}
      <Modal
        open={deleteModalOpen}
        modalHeading={t('api_keys.delete')}
        primaryButtonText={t('common.delete')}
        secondaryButtonText={t('common.cancel')}
        onRequestClose={() => {
            setDeleteModalOpen(false);
            setKeyToDelete(null);
        }}
        onRequestSubmit={() => {
            if (keyToDelete) {
                handleDelete(keyToDelete);
            }
            setDeleteModalOpen(false);
            setKeyToDelete(null);
        }}
        danger={true}
        className="api-key-modal"
      >
        <p className="delete-confirmation-text">
            {t('api_keys.confirm_delete', 'Are you sure you want to delete this API Key? This action cannot be undone.')}
        </p>
      </Modal>
    </div>
  );
};

export default APIKeyManager;
