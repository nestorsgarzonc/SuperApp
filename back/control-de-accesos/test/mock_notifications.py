#!/usr/bin/env python3
"""Minimal HTTP listener that logs every event posted by internal/notifier.
Not a stand-in for the notificaciones microservice — just a way to see the
outbound POSTs during local testing.

Usage:
    python3 test/mock_notifications.py [port]   # default 9000
    NOTIFICATIONS_URL=http://localhost:9000/events go run ./cmd/api
"""
import http.server
import json
import sys


class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length)
        try:
            print("EVENT RECEIVED:", json.dumps(json.loads(body)), flush=True)
        except Exception:
            print("EVENT RECEIVED (raw):", body, flush=True)
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"ok":true}')

    def log_message(self, fmt, *args):
        pass


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 9000
    print(f"Listening on http://localhost:{port} ...", flush=True)
    http.server.HTTPServer(("localhost", port), Handler).serve_forever()
