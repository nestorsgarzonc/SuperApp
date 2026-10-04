import { forwardRef, useState } from 'react';
import { Eye, EyeOff } from 'lucide-react';
import { Input, type InputProps } from './Input';
export const PasswordInput = forwardRef<HTMLInputElement, InputProps>(function PasswordInput(props, ref) {
  const [visible, setVisible] = useState(false);
  return <Input {...props} ref={ref} type={visible ? 'text' : 'password'} trailing={<button className="icon-button password-toggle" type="button" aria-label={visible ? 'Ocultar contraseña' : 'Mostrar contraseña'} aria-pressed={visible} onClick={() => setVisible((value) => !value)}>{visible ? <EyeOff size={19} /> : <Eye size={19} />}</button>} />;
});
