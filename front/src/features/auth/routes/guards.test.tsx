import { afterEach, describe, expect, it } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { Link, MemoryRouter, Route, Routes } from 'react-router-dom';
import { AuthProvider } from '../components/AuthContext';
import { LoginPage } from '../pages/LoginPage';
import { ProtectedRoute } from './guards';

afterEach(cleanup);
function GuardHarness({ initialPath }: { initialPath: string }) {
  return <MemoryRouter initialEntries={[initialPath]}><AuthProvider><Link to="/protected">Abrir ruta protegida</Link><Routes>
    <Route path="/login" element={<LoginPage/>}/>
    <Route element={<ProtectedRoute permission="access.platform.manage"/>}><Route path="/protected" element={<p>Contenido protegido</p>}/></Route>
    <Route path="/unauthorized" element={<p>Acceso no autorizado</p>}/>
    <Route path="/select-complex" element={<p>Selecciona un conjunto</p>}/>
  </Routes></AuthProvider></MemoryRouter>;
}

describe('route guards', () => {
  it('redirects an unauthenticated visitor to login', async () => {
    render(<GuardHarness initialPath="/protected"/>);
    expect(await screen.findByRole('heading', { name: 'Inicia sesión' })).toBeInTheDocument();
  });
  it('redirects an authenticated user without the permission to unauthorized', async () => {
    render(<GuardHarness initialPath="/login"/>);
    fireEvent.change(screen.getByLabelText('Correo electrónico'), { target: { value: 'demo@superapp.co' } });
    fireEvent.change(screen.getByLabelText('Contraseña'), { target: { value: 'Demo123!' } });
    fireEvent.click(screen.getByRole('button', { name: /iniciar sesión/i }));
    await screen.findByText('Selecciona un conjunto');
    fireEvent.click(screen.getByRole('link', { name: 'Abrir ruta protegida' }));
    expect(await screen.findByText('Acceso no autorizado')).toBeInTheDocument();
    expect(screen.queryByText('Contenido protegido')).not.toBeInTheDocument();
  });
});
