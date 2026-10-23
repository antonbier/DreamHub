import os
SCOPE_CONFIG = "config"
CONFDIR = os.environ.get("STUB_CONFDIR", "/tmp")
def resolveFilename(scope, name): return os.path.join(CONFDIR, name)
def fileExists(p): return os.path.exists(p)
