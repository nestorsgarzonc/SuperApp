import { setupServer } from 'msw/node';
import { handlers } from '../features/auth/api/handlers';
export const server = setupServer(...handlers);
