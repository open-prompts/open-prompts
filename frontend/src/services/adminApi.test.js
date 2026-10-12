import { decodeAdminToken } from './adminApi';

describe('adminApi', () => {
  describe('decodeAdminToken', () => {
    test('decodes a valid base64url JWT payload', () => {
      // header.payload.signature with payload {"sub":"admin1","role":"admin"}
      const token = 'eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJhZG1pbjEiLCJyb2xlIjoiYWRtaW4ifQ.sig';
      expect(decodeAdminToken(token)).toEqual({ sub: 'admin1', role: 'admin' });
    });

    test('returns null for garbage input', () => {
      expect(decodeAdminToken('not-a-jwt')).toBeNull();
    });

    test('returns null for empty input', () => {
      expect(decodeAdminToken('')).toBeNull();
    });

    test('returns null when payload is not valid JSON', () => {
      const badPayload = btoa('not-json').replace(/\+/g, '-').replace(/\//g, '_');
      expect(decodeAdminToken(`header.${badPayload}.sig`)).toBeNull();
    });
  });
});
