import { z } from 'zod';

export const loginSchema = z.object({
  email: z.string().trim().email('Ingresa un correo electrónico válido.'),
  password: z.string().min(1, 'Ingresa tu contraseña.'),
  remember: z.boolean(),
});
export type LoginValues = z.infer<typeof loginSchema>;

export const emailSchema = z.object({ email: z.string().trim().email('Ingresa un correo electrónico válido.') });
export const passwordSchema = z.object({
  password: z.string().min(8, 'Usa al menos 8 caracteres.').regex(/[A-ZÁÉÍÓÚÑ]/, 'Incluye al menos una mayúscula.')
    .regex(/[0-9]/, 'Incluye al menos un número.').regex(/[^A-Za-z0-9]/, 'Incluye al menos un símbolo.'),
  confirmPassword: z.string(),
}).refine((values) => values.password === values.confirmPassword, { path: ['confirmPassword'], message: 'Las contraseñas no coinciden.' });
