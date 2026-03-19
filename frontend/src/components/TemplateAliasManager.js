import React, { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, TextInput, Select, SelectItem, InlineLoading } from '@carbon/react';
import { TrashCan } from '@carbon/icons-react';
import { listTemplateAliases, createTemplateAlias, updateTemplateAlias, deleteTemplateAlias } from '../services/api';
import './TemplateAliasManager.scss';
import { useNotification } from '../context/NotificationContext';

const TemplateAliasManager = ({ templateId, versions = [] }) => {
  const { t } = useTranslation();
  const { addNotification } = useNotification();
  const [aliases, setAliases] = useState([]);
  const [loading, setLoading] = useState(true);
  const [newAliasName, setNewAliasName] = useState('');
  const [newAliasVersionId, setNewAliasVersionId] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    if (templateId) {
       fetchAliases();
    }
  }, [templateId]);

  const fetchAliases = async () => {
    try {
      setLoading(true);
      const res = await listTemplateAliases(templateId);
      setAliases(res.data.aliases || []);
    } catch (err) {
      console.error('Failed to fetch aliases', err);
    } finally {
      setLoading(false);
    }
  };

  const handleCreateOrUpdate = async () => {
    if (!newAliasName || !newAliasVersionId) return;
    
    try {
      setIsSubmitting(true);
      const existing = aliases.find(a => a.alias_name === newAliasName);
      
      if (existing) {
        await updateTemplateAlias(templateId, newAliasName, { version_id: parseInt(newAliasVersionId) });
        addNotification({ kind: 'success', title: t('common.success'), subtitle: 'Alias updated' });
      } else {
        await createTemplateAlias(templateId, { alias_name: newAliasName, version_id: parseInt(newAliasVersionId) });
        addNotification({ kind: 'success', title: t('common.success'), subtitle: 'Alias created' });
      }
      
      setNewAliasName('');
      setNewAliasVersionId('');
      fetchAliases();
    } catch (err) {
      addNotification({
        kind: 'error',
        title: t('common.error'),
        subtitle: 'Failed to save alias'
      });
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleDelete = async (aliasName) => {
    if (!window.confirm('Are you sure you want to delete this alias?')) return;
    
    try {
      await deleteTemplateAlias(templateId, aliasName);
      addNotification({ kind: 'success', title: t('common.success'), subtitle: 'Alias deleted' });
      fetchAliases();
    } catch (err) {
      addNotification({ kind: 'error', title: t('common.error'), subtitle: 'Failed to delete alias' });
    }
  };

  if (loading) return <InlineLoading description={t('common.loading')} />;

  return (
    <div className="template-alias-manager">
      <h4 className="alias-manager-title">Environment / API Aliases</h4>
      <p className="alias-manager-desc">
        Map strings like "prod" to versions so API clients can fetch the latest mapped prompts without changing code.
      </p>

      <div className="alias-list">
        {aliases.length === 0 && (
          <div className="no-aliases">No aliases mapped yet</div>
        )}
        {aliases.map(alias => {
          const v = versions.find(v => v.id === alias.version_id);
          const vName = v ? `v${v.version}` : `ID: ${alias.version_id}`;
          return (
            <div key={alias.alias_name} className="alias-item">
              <div className="alias-info">
                <span className="alias-badge">{alias.alias_name}</span>
                <span className="icon-arrow"> → </span>
                <span className="alias-version">{vName}</span>
              </div>
              <Button
                hasIconOnly
                renderIcon={TrashCan}
                iconDescription={t('common.delete')}
                kind="ghost"
                size="sm"
                onClick={() => handleDelete(alias.alias_name)}
                disabled={alias.alias_name === 'latest'}
              />
            </div>
          );
        })}
      </div>

      <div className="alias-form">
        <TextInput
          id="alias-name"
          labelText="Alias Name"
          value={newAliasName}
          onChange={(e) => setNewAliasName(e.target.value)}
          placeholder="e.g. prod, dev"
        />
        <Select
          id="alias-version"
          labelText="Target Version"
          value={newAliasVersionId}
          onChange={(e) => setNewAliasVersionId(e.target.value)}
        >
          <SelectItem value="" text={t('common.select', 'Select...')} disabled hidden />
          {versions.map(v => (
            <SelectItem key={v.id} value={v.id} text={`v${v.version}`} />
          ))}
        </Select>
        <Button 
          size="md" 
          onClick={handleCreateOrUpdate}
          disabled={!newAliasName || !newAliasVersionId || isSubmitting}
          className="alias-btn"
        >
          {aliases.some(a => a.alias_name === newAliasName) 
            ? 'Update Alias' 
            : 'Create Alias'}
        </Button>
      </div>
    </div>
  );
};

export default TemplateAliasManager;
