import { afterEach, describe, expect, it } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import { AuthProvider } from '../components/AuthContext';
import { LoginPage } from './LoginPage';
import { SelectComplexPage } from './FlowPages';

afterEach(cleanup);
describe('complex selection', () => {
  it('requires one choice and confirms it with the authenticated service', async () => {
    window.history.replaceState({}, '', '/login');
    render(<BrowserRouter><AuthProvider><><LoginPage/><SelectComplexPage/></></AuthProvider></BrowserRouter>);
    fireEvent.change(screen.getByLabelText('Correo electrónico'), { target: { value: 'demo@superapp.co' } });
    fireEvent.change(screen.getByLabelText('Contraseña'), { target: { value: 'Demo123!' } });
    fireEvent.click(screen.getByRole('button', { name: /iniciar sesión/i }));
    await waitFor(() => expect(screen.getByRole('radio', { name: /parques del río/i })).toBeInTheDocument());
    const continueButton = screen.getByRole('button', { name: /continuar/i });
    expect(continueButton).toBeDisabled();
    fireEvent.click(screen.getByRole('radio', { name: /torres de san josé/i }));
    expect(continueButton).toBeEnabled();
    fireEvent.click(continueButton);
    await waitFor(() => expect(window.location.pathname).toBe('/dashboard'));
  });
});
