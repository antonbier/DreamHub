import http.server, os, sys
class H(http.server.SimpleHTTPRequestHandler):
    def end_headers(self):
        self.send_header('Cross-Origin-Opener-Policy', 'same-origin')
        self.send_header('Cross-Origin-Embedder-Policy', 'require-corp')
        self.send_header('Cache-Control', 'no-store')
        super().end_headers()
    def log_message(self, *a): pass
    def do_GET(self):
        # /box?file=<path on the box>: pass a recording through from the box,
        # streamed, the way DreamHub's backend would.
        if self.path.startswith('/box?'):
            import urllib.request
            up = urllib.request.urlopen(os.environ.get('BOX', 'http://192.168.1.43') + '/file?' + self.path[5:], timeout=20)
            self.send_response(200); self.send_header('Content-Type', 'video/mp2t'); self.end_headers()
            try:
                while True:
                    b = up.read(65536)
                    if not b: break
                    self.wfile.write(b)
            except (BrokenPipeError, ConnectionResetError):
                pass
            return
        return super().do_GET()
H.extensions_map['.mjs'] = 'text/javascript'; H.extensions_map['.wasm'] = 'application/wasm'
os.chdir(os.path.dirname(os.path.abspath(__file__)))
http.server.ThreadingHTTPServer(('127.0.0.1', int(sys.argv[1]) if len(sys.argv) > 1 else 8099), H).serve_forever()
