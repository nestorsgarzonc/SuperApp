import { setupWorker } from 'msw/browser';
import { handlers } from '../features/auth/api/handlers';
export const worker = setupWorker(...handlers);
