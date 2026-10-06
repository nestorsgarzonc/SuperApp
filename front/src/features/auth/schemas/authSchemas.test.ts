import { describe, expect, it } from 'vitest';
import { loginSchema, passwordSchema } from './authSchemas';

describe('auth schemas', () => {
  it('validates email and required password', () => {
    expect(loginSchema.safeParse({ email: 'persona@correo.co', password: 'a', remember: false }).success).toBe(true);
    expect(loginSchema.safeParse({ email: 'no-es-correo', password: '', remember: false }).success).toBe(false);
  });
  it('requires a strong password and matching confirmation', () => {
    expect(passwordSchema.safeParse({ password: 'CasaSegura1!', confirmPassword: 'CasaSegura1!' }).success).toBe(true);
    expect(passwordSchema.safeParse({ password: 'casasegura', confirmPassword: 'casasegura' }).success).toBe(false);
    expect(passwordSchema.safeParse({ password: 'CasaSegura1!', confirmPassword: 'CasaSegura2!' }).success).toBe(false);
  });
});
