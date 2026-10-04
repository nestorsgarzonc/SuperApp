import { useEffect, useState } from 'react';
import QRCode from 'qrcode';

/**
 * Imagen del QR (data URL) generada en el navegador a partir del código del
 * pase. Es el mismo contenido que el ms codifica en `qr_code_url`, pero
 * `GET /access-grants/{id}` no lo devuelve, y así funciona también sin red.
 */
export function useQrDataUrl(value?: string | null) {
  const [url, setUrl] = useState<string | null>(null);
  useEffect(() => {
    let alive = true;
    setUrl(null);
    if (value) {
      QRCode.toDataURL(value, { errorCorrectionLevel: 'M', margin: 1, width: 440 })
        .then((u) => alive && setUrl(u))
        .catch(() => alive && setUrl(null));
    }
    return () => {
      alive = false;
    };
  }, [value]);
  return url;
}
