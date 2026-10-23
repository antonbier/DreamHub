# -*- coding: utf-8 -*-
# /dreamhub/settings/* on the box's own web server (WebInterface, port 80):
# the box's setup menus and its config tree, READ ONLY (boxsettings.py,
# docs/BOXSETTINGS_PLAN.md, S2). Registered next to /dreamhub/play, behind
# the same login as /web/*; only the DreamHub backend calls it.
#
#   GET info                          api version, the stores there are
#   GET sections                      the box's setup.xml sections (no values)
#   GET roots?store=                  the top-level groups of the config tree
#   GET items?section=<key>           one setup section, every setting described
#   GET items?store=&root=&q=&offset=&limit=   settings below a root, paged
#   GET item?store=&path=             one setting with its full choice list
#
# Application errors answer HTTP 200 with {"ok": false, "error": code}: the
# backend's HTTP client keeps only 200 bodies. A wrong method is 405, an
# unknown route 404. Nothing here writes to the box.
import json

from twisted.web import resource

from . import boxsettings as bs

# Kept in sync by hand with PluginVersion (__init__.py); imported lazily so a
# problem there cannot keep /dreamhub/play from registering.
API_VERSION = 1


def _clean(o):
    """Python 2: a byte string with invalid UTF-8 would make json.dumps raise."""
    if isinstance(o, dict):
        return dict((_clean(k), _clean(v)) for k, v in o.items())
    if isinstance(o, (list, tuple)):
        return [_clean(x) for x in o]
    if isinstance(o, bytes):  # a Python 2 str; Python 3's bytes never occur here
        try:
            return o.decode("utf-8")
        except UnicodeDecodeError:
            return o.decode("utf-8", "replace")
    return o


def _json(obj):
    try:
        return json.dumps(obj)
    except (UnicodeDecodeError, TypeError, ValueError):
        return json.dumps(_clean(obj))


def _arg(request, name, default=""):
    v = request.args.get(name, [default])[0]
    return v if v is not None else default


class SettingsResource(resource.Resource):
    isLeaf = True

    def __init__(self, session=None):
        resource.Resource.__init__(self)
        self.session = session

    def _reply(self, request, obj, code=200):
        request.setResponseCode(code)
        request.setHeader("Content-Type", "application/json; charset=utf-8")
        request.setHeader("Cache-Control", "no-store")
        return _json(obj)

    def _error(self, request, code, message="", **extra):
        d = {"ok": False, "error": code, "message": message}
        d.update(extra)
        return self._reply(request, d)

    def render_POST(self, request):
        # Read only for now: a write comes as its own step (S3).
        request.setHeader("Allow", "GET")
        return self._reply(request, {"ok": False, "error": "read_only", "message": "this plugin only reads settings"}, 405)

    render_PUT = render_DELETE = render_POST

    def render_GET(self, request):
        route = "/".join(p for p in request.postpath if p)
        try:
            if route == "info":
                from . import PluginVersion
                return self._reply(request, bs.info(PluginVersion))
            if route == "sections":
                return self._sections(request)
            if route == "roots":
                return self._roots(request)
            if route == "items":
                return self._items(request)
            if route == "item":
                return self._item(request)
        except Exception as e:  # a bug here must not become a 500 page that the backend cannot read
            return self._error(request, "internal", "%s: %s" % (type(e).__name__, e))
        request.setResponseCode(404)
        request.setHeader("Content-Type", "application/json; charset=utf-8")
        return json.dumps({"ok": False, "error": "not_found"})

    def _core(self):
        from Screens.Setup import setupdom
        from Components.SystemInfo import SystemInfo
        return bs.core_sections(setupdom, SystemInfo)

    def _sections(self, request):
        secs = self._core()
        return self._reply(request, {"ok": True, "sections": [
            {"key": s["key"], "title": s["title"], "available": s["available"],
             "items": len([i for i in s["items"] if not i["heading"]])} for s in secs]})

    def _roots(self, request):
        store = _arg(request, "store", "settings")
        out = bs.tree_roots(store)
        if out is None:
            return self._error(request, "unknown_store")
        return self._reply(request, {"ok": True, "store": store, "roots": out})

    def _items(self, request):
        section = _arg(request, "section")
        if section:
            res = bs.section_items(self._core(), section)
            if res is None:
                return self._error(request, "not_found", "no such section")
            res["ok"] = True
            return self._reply(request, res)
        root = _arg(request, "root")
        if not root:
            return self._error(request, "bad_request", "section or root is required")
        try:
            offset = int(_arg(request, "offset", "0"))
            limit = int(_arg(request, "limit", "500"))
        except ValueError:
            return self._error(request, "bad_request", "offset and limit are numbers")
        res = bs.tree_items(_arg(request, "store", "settings"), root, _arg(request, "q"), offset, limit)
        if res is None:
            return self._error(request, "not_found", "no such store or root")
        res["ok"] = True
        return self._reply(request, res)

    def _item(self, request):
        store = _arg(request, "store", "settings")
        roots = bs.stores()
        if store not in roots:
            return self._error(request, "unknown_store")
        try:
            elem = bs.resolve(roots[store], _arg(request, "path"))
        except bs.PathError as e:
            return self._error(request, str(e))
        # The whole choice list: a listing caps it (timezone, keymap and satellite lists are long).
        return self._reply(request, {"ok": True, "item": bs.describe_safe(_arg(request, "path"), elem, store, 100000)})
