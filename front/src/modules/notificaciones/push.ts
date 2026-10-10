import type { FirebaseOptions } from 'firebase/app';
import type { Messaging } from 'firebase/messaging';
import { notificationsApi } from './api';
import { refreshUnread } from './unread';

// Push del navegador con Firebase Cloud Messaging. Sin las variables
// VITE_FIREBASE_* todo es no-op y la app funciona solo con la bandeja.

const env = import.meta.env;
const config: FirebaseOptions = {
  apiKey: env.VITE_FIREBASE_API_KEY,
  authDomain: env.VITE_FIREBASE_AUTH_DOMAIN,
  projectId: env.VITE_FIREBASE_PROJECT_ID,
  messagingSenderId: env.VITE_FIREBASE_MESSAGING_SENDER_ID,
  appId: env.VITE_FIREBASE_APP_ID,
};
const vapidKey: string | undefined = env.VITE_FIREBASE_VAPID_KEY;

export const pushConfigured = Boolean(
  config.apiKey && config.projectId && config.messagingSenderId && config.appId && vapidKey,
);

// Scope propio: no choca con el service worker de MSW (mock) que usa "/".
const SW_SCOPE = '/firebase-cloud-messaging-push-scope';
const TOKEN_KEY = 'superapp:push-token';

const storage = {
  get: () => {
    try {
      return localStorage.getItem(TOKEN_KEY);
    } catch {
      return null;
    }
  },
  set: (token: string | null) => {
    try {
      if (token) localStorage.setItem(TOKEN_KEY, token);
      else localStorage.removeItem(TOKEN_KEY);
    } catch {
      // Sin almacenamiento solo se pierde el unregister al cerrar sesión.
    }
  },
};

type Sdk = typeof import('firebase/messaging');
let messagingPromise: Promise<{ m: Messaging; sdk: Sdk } | null> | null = null;

// El SDK se descarga solo si push está configurado (no pesa en el bundle inicial).
function messaging() {
  messagingPromise ??= (async () => {
    if (!pushConfigured) return null;
    const [{ initializeApp }, sdk] = await Promise.all([import('firebase/app'), import('firebase/messaging')]);
    if (!(await sdk.isSupported())) return null;
    const m = sdk.getMessaging(initializeApp(config));
    // Con la app abierta FCM no muestra nada: basta con actualizar la campana.
    sdk.onMessage(m, () => void refreshUnread());
    return { m, sdk };
  })().catch(() => null);
  return messagingPromise;
}

export type PushState = 'unsupported' | NotificationPermission;

export async function pushState(): Promise<PushState> {
  return (await messaging()) ? Notification.permission : 'unsupported';
}

function deviceName() {
  const ua = navigator.userAgent;
  const browser = /Edg\//.test(ua) ? 'Edge' : /Chrome\//.test(ua) ? 'Chrome' : /Firefox\//.test(ua) ? 'Firefox' : /Safari\//.test(ua) ? 'Safari' : 'Navegador';
  const os = /Android/.test(ua) ? 'Android' : /iPhone|iPad/.test(ua) ? 'iOS' : /Mac OS X/.test(ua) ? 'macOS' : /Windows/.test(ua) ? 'Windows' : /Linux/.test(ua) ? 'Linux' : '';
  return os ? `${browser} · ${os}` : browser;
}

/**
 * La suscripción push exige un worker activo; en la primera visita aún se está
 * instalando. `navigator.serviceWorker.ready` no sirve: espera al worker del
 * scope de la página, y este usa SW_SCOPE.
 */
function activated(reg: ServiceWorkerRegistration) {
  const worker = reg.installing ?? reg.waiting;
  if (reg.active || !worker) return Promise.resolve(reg);
  return new Promise<ServiceWorkerRegistration>((resolve) => {
    worker.addEventListener('statechange', () => {
      if (worker.state === 'activated') resolve(reg);
    });
  });
}

async function register({ m, sdk }: { m: Messaging; sdk: Sdk }) {
  // El service worker no ve import.meta.env: la configuración viaja en la URL.
  const params = new URLSearchParams(Object.entries(config).filter((e): e is [string, string] => !!e[1]));
  const sw = await activated(await navigator.serviceWorker.register(`/firebase-messaging-sw.js?${params}`, { scope: SW_SCOPE }));
  const token = await sdk.getToken(m, { vapidKey, serviceWorkerRegistration: sw });
  await notificationsApi.registerDevice({ platform: 'WEB', push_token: token, device_name: deviceName() });
  storage.set(token);
}

/** Pide permiso (debe llamarse desde un clic) y registra este navegador. */
export async function enablePush(): Promise<PushState> {
  const fcm = await messaging();
  if (!fcm) return 'unsupported';
  const permission = await Notification.requestPermission();
  if (permission === 'granted') await register(fcm);
  return permission;
}

/** Al iniciar sesión: si ya hay permiso, renueva el registro (el token puede cambiar o ser de otro usuario). */
export async function syncPush() {
  const fcm = await messaging();
  if (fcm && Notification.permission === 'granted') await register(fcm).catch(() => undefined);
}

/** Al cerrar sesión: este navegador deja de recibir los push del usuario. */
export async function disablePush() {
  const token = storage.get();
  if (!token) return;
  storage.set(null);
  await notificationsApi.unregisterDevice(token).catch(() => undefined);
  const fcm = await messaging();
  if (fcm) await fcm.sdk.deleteToken(fcm.m).catch(() => undefined);
}
