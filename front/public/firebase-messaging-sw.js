/* global firebase, importScripts */
/* Service worker de Firebase Cloud Messaging (push con la app cerrada o en segundo plano).
 * La configuración pública de Firebase llega en la query string (ver src/modules/notificaciones/push.ts).
 * La versión debe coincidir con la de "firebase" en package.json. */
importScripts('https://www.gstatic.com/firebasejs/12.19.0/firebase-app-compat.js');
importScripts('https://www.gstatic.com/firebasejs/12.19.0/firebase-messaging-compat.js');

firebase.initializeApp(Object.fromEntries(new URL(self.location.href).searchParams));
// El SDK muestra la notificación (title/body del push) y, al tocarla, abre el
// link que manda el backend (WEB_APP_URL + deep_link).
firebase.messaging();
