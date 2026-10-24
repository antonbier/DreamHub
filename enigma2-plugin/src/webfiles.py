# -*- coding: utf-8 -*-
# /dreamhub/files/* on the box's own web server (WebInterface, port 80): the
# write side of Netcast's source lists (boxfiles.py, docs/NETCAST_PLAN.md, N3).
# Registered next to /dreamhub/play and /dreamhub/settings, behind the same
# login as /web/*; only the DreamHub backend calls it.
#
#   GET  info    {"ok": true, "api": 1, "writes": <switch>}
#   POST write   form fields `path` and `data` (the whole new content, UTF-8)
#
# A write needs, all of them: the switch config.plugins.dreamhub.fileWrite
# (off by default, turned on in DreamHub's settings on the TV), POST, the
# header X-DreamHub-Request (a page on another origin cannot send it without
# the preflight this resource never grants), and a Host the box answers to
# (DNS rebinding). Application errors answer HTTP 200 {"ok": false, "error":
# code}: the backend's client keeps only 200 bodies.
import json

from twisted.web import resource

from . import boxfiles

API_VERSION = 1
HEADER = "X-DreamHub-Request"


def _switch_on():
    from Components.config import config
    return bool(config.plugins.dreamhub.fileWrite.value)


class FilesResource(resource.Resource):
    isLeaf = True

    def _reply(self, request, obj, code=200):
        request.setResponseCode(code)
        request.setHeader("Content-Type", "application/json; charset=utf-8")
        request.setHeader("Cache-Control", "no-store")
        return json.dumps(obj)

    def _error(self, request, code, message=""):
        return self._reply(request, {"ok": False, "error": code, "message": message})

    def render_GET(self, request):
        route = "/".join(p for p in request.postpath if p)
        if route == "info":
            try:
                on = _switch_on()
            except Exception:
                on = False
            return self._reply(request, {"ok": True, "api": API_VERSION, "writes": on})
        return self._reply(request, {"ok": False, "error": "not_found"}, 404)

    def render_POST(self, request):
        route = "/".join(p for p in request.postpath if p)
        if route != "write":
            return self._reply(request, {"ok": False, "error": "not_found"}, 404)
        try:
            if not _switch_on():
                return self._error(request, "writes_off", "turn on file writing in DreamHub's settings on the TV")
            if request.getHeader(HEADER) != "1":
                return self._error(request, "bad_request", "missing %s header" % HEADER)
            if not boxfiles.host_ok(request.getHeader("Host")):
                return self._error(request, "bad_host", "unexpected Host header")
            path = request.args.get("path", [""])[0]
            data = request.args.get("data", [None])[0]
            if data is None:
                return self._error(request, "bad_request", "no data")
            if not isinstance(data, bytes):  # Python 3 (tests)
                data = data.encode("utf-8")
            n = boxfiles.write(path, data)
            return self._reply(request, {"ok": True, "bytes": n})
        except boxfiles.Refused as e:
            return self._error(request, e.code, e.message)
        except Exception as e:  # never a 500 page the backend cannot read
            return self._error(request, "internal", "%s: %s" % (type(e).__name__, e))

    def render_PUT(self, request):
        request.setHeader("Allow", "GET, POST")
        return self._reply(request, {"ok": False, "error": "method"}, 405)

    render_DELETE = render_PUT
