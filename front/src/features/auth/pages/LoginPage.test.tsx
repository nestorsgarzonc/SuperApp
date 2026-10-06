import { afterEach, describe, expect, it } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import { AuthProvider } from '../components/AuthContext';
import { LoginPage } from './LoginPage';

afterEach(cleanup);
function renderLogin() { return render(<BrowserRouter><AuthProvider><LoginPage /></AuthProvider></BrowserRouter>); }

describe('LoginPage', () => {
  it('shows one generic credential message when login returns 401', async () => {
    renderLogin();
    fireEvent.change(screen.getByLabelText('Correo electrónico'), { target: { value: 'persona@correo.co' } });
    fireEvent.change(screen.getByLabelText('Contraseña'), { target: { value: 'incorrecta' } });
    fireEvent.click(screen.getByRole('button', { name: /iniciar sesión/i }));
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('No pudimos validar tus credenciales. Verifica tus datos e inténtalo nuevamente.'));
    expect(screen.queryByText(/correo no existe|contraseña incorrecta/i)).not.toBeInTheDocument();
  });
});
