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

const APIKeyManager = ({ notification }) => {
  const { t } = useTranslation();
  const [keys, setKeys] = useState([]);
  const [loading, setLoading] = useState(true);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [newKeyName, setNewKeyName] = useState('');
  const [generatedKey, setGeneratedKey] = useState(null);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

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
      if (notification) notification({ kind: 'error', title: 'Error', subtitle: 'Failed to fetch API keys' });
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
      if (notification) notification({ kind: 'error', title: 'Error', subtitle: 'Failed to create API key' });
    }
  };

  const handleDelete = async (id) => {
    try {
      await deleteAPIKey(id);
      fetchKeys();
      if (notification) notification({ kind: 'success', title: 'Success', subtitle: 'API key deleted' });
    } catch (err) {
      if (notification) notification({ kind: 'error', title: 'Error', subtitle: 'Failed to delete API key' });
    }
  };

  const copyToClipboard = (text) => {
    navigator.clipboard.writeText(text);
    if (notification) notification({ kind: 'info', title: 'Copied', subtitle: 'API key copied to clipboard' });
  };

  // Custom Close handler for Modal to clear state when fully closed
  const handleModalClose = () => {
      setIsModalOpen(false);
      // clear generated key after a delay or immediately if user closes
      setGeneratedKey(null);
      setNewKeyName('');
  }

  return (
    <div className="api-key-manager" style={{ marginTop: '2rem' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
          <h3>{t('api_keys.title', 'API Keys')}</h3>
          <Button renderIcon={Add} onClick={() => setIsModalOpen(true)} size="sm">
            {t('api_keys.generate', 'Generate New Key')}
          </Button>
      </div>

      <TableContainer title={t('api_keys.list_title', 'Your API Keys')}>
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
                <TableCell><code>{row.prefix}</code></TableCell>
                <TableCell>{row.created_at}</TableCell>
                <TableCell>{row.last_used_at}</TableCell>
                <TableCell>
                  <Button
                    hasIconOnly
                    renderIcon={TrashCan}
                    kind="ghost"
                    iconDescription="Delete"
                    onClick={() => handleDelete(row.id)}
                    size="sm"
                  />
                </TableCell>
              </TableRow>
            ))}
            {keys.length === 0 && !loading && (
                <TableRow>
                    <TableCell colSpan={5} style={{textAlign: 'center', padding: '1rem'}}>
                        No API keys found.
                    </TableCell>
                </TableRow>
            )}
            {loading && (
                <TableRow>
                    <TableCell colSpan={5} style={{textAlign: 'center', padding: '1rem'}}>
                        Loading...
                    </TableCell>
                </TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>

      {/* Create Modal */}
      <Modal
        open={isModalOpen}
        modalHeading={!generatedKey ? "Generate New API Key" : "API Key Generated"}
        primaryButtonText={!generatedKey ? "Generate" : "Done"}
        secondaryButtonText={!generatedKey ? "Cancel" : ""}
        onRequestClose={handleModalClose}
        onRequestSubmit={!generatedKey ? handleCreate : handleModalClose}
        danger={false}
      >
        {!generatedKey ? (
            <TextInput
                id="key-name"
                labelText="Key Name"
                placeholder="e.g. My App"
                value={newKeyName}
                onChange={(e) => setNewKeyName(e.target.value)}
            />
        ) : (
            <div className="generated-key-display">
                <p style={{marginBottom: '1rem', color: 'red'}}>
                    Please copy your API key now. You won't be able to see it again!
                </p>
                <div style={{ display: 'flex', alignItems: 'flex-end', gap: '1rem' }}>
                    <div style={{flex: 1}}>
                        <TextInput
                            id="generated-key"
                            labelText="API Key"
                            value={generatedKey}
                            readOnly
                        />
                    </div>
                    <Button
                        hasIconOnly
                        renderIcon={Copy}
                        kind="ghost"
                        iconDescription="Copy"
                        onClick={() => copyToClipboard(generatedKey)}
                        style={{marginBottom: '2px'}}
                    />
                </div>
            </div>
        )}
      </Modal>
    </div>
  );
};

export default APIKeyManager;
