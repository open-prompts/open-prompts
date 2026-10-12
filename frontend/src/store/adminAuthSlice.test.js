import adminAuthReducer, {
  adminLoginSuccess,
  adminLogout,
} from './adminAuthSlice';

describe('adminAuthSlice', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  test('returns unauthenticated initial state when storage is empty', () => {
    const state = adminAuthReducer(undefined, { type: '@@INIT' });
    expect(state.isAuthenticated).toBe(false);
    expect(state.user).toBeNull();
    expect(state.token).toBeNull();
  });

  test('adminLoginSuccess stores token and user in op_admin_* keys', () => {
    const user = { id: 'admin1', email: 'admin@example.com', role: 'admin' };
    const state = adminAuthReducer(
      undefined,
      adminLoginSuccess({ token: 'tok-1', user })
    );

    expect(state.isAuthenticated).toBe(true);
    expect(state.token).toBe('tok-1');
    expect(state.user).toEqual(user);
    expect(localStorage.getItem('op_admin_token')).toBe('tok-1');
    expect(JSON.parse(localStorage.getItem('op_admin_user'))).toEqual(user);
  });

  test('adminLogout clears state and storage', () => {
    localStorage.setItem('op_admin_token', 'tok-1');
    localStorage.setItem('op_admin_user', JSON.stringify({ id: 'a' }));

    const state = adminAuthReducer(undefined, adminLogout());

    expect(state.isAuthenticated).toBe(false);
    expect(state.user).toBeNull();
    expect(state.token).toBeNull();
    expect(localStorage.getItem('op_admin_token')).toBeNull();
    expect(localStorage.getItem('op_admin_user')).toBeNull();
  });

  test('initial state restores from op_admin_* storage keys', () => {
    const user = { id: 'admin1', role: 'admin' };
    localStorage.setItem('op_admin_token', 'stored-token');
    localStorage.setItem('op_admin_user', JSON.stringify(user));

    // Re-require the module so getInitialState runs against seeded storage.
    jest.resetModules();
    const fresh = require('./adminAuthSlice').default;
    const state = fresh(undefined, { type: '@@INIT' });

    expect(state.isAuthenticated).toBe(true);
    expect(state.token).toBe('stored-token');
    expect(state.user).toEqual(user);
  });

  test('corrupted user JSON is discarded and state stays unauthenticated', () => {
    localStorage.setItem('op_admin_token', 'stored-token');
    localStorage.setItem('op_admin_user', '{not-json');

    jest.resetModules();
    const fresh = require('./adminAuthSlice').default;
    const state = fresh(undefined, { type: '@@INIT' });

    expect(state.isAuthenticated).toBe(false);
    expect(localStorage.getItem('op_admin_token')).toBeNull();
  });

  test('admin session does not touch regular user token keys', () => {
    localStorage.setItem('token', 'user-token');
    localStorage.setItem('user', 'user-payload');

    const user = { id: 'admin1' };
    adminAuthReducer(undefined, adminLoginSuccess({ token: 'admin-tok', user }));

    expect(localStorage.getItem('token')).toBe('user-token');
    expect(localStorage.getItem('user')).toBe('user-payload');
    expect(localStorage.getItem('op_admin_token')).toBe('admin-tok');
  });
});
