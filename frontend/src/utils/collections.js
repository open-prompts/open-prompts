import api from '../services/api';

// Module-cached probe: whether the collections feature is deployed. The
// dashboard hides the "My Collections" entry when the collections table is
// absent (the overview degrades collection_count to 0). Cached so the probe
// runs at most once per session.
let cached = null;

export const hasCollections = async () => {
  if (cached !== null) return cached;
  try {
    await api.get('/collections', { params: { page_size: 1 } });
    cached = true;
  } catch (e) {
    cached = e.response?.status !== 404;
  }
  return cached;
};