# -*- coding: utf-8 -*-
# The read side of the box's settings (docs/BOXSETTINGS_PLAN.md, S2): what
# enigma2 holds under config.* — its setup menus (Screens/Setup.py's own
# setup.xml) and the whole tree behind them, types, limits and choices
# included. The Dreambox's WebControl has no such call, and an outside edit
# of /etc/enigma2/settings is overwritten when enigma2 shuts down, so the
# one place that can tell is a plugin running inside enigma2.
#
# READ ONLY, on purpose: this file has no function that assigns a value or
# saves one. Writing (S3) comes as a separate step; the order it needs is in
# ARCHITECTURE.md, "DreamOS config internals". Nothing here eval()s — a
# dotted path is walked by hand — and no getter with side effects is called
# (ConfigIP6.getValue() rewrites an invalid text, ConfigLocations.getValue()
# rescans the mounts): peek() reads the raw value.
#
# Python 2.7 on the box; also valid Python 3 so the tests can run against the
# box's real Components/config.py off-box (tests/).
import hashlib
import re
import sys

try:
    string_types = basestring  # noqa: F821  (Python 2)
except NameError:
    string_types = str

from Components.config import ConfigElement, ConfigSubDict, ConfigSubList, ConfigSubsection, config

try:
    import __builtin__ as _builtins
except ImportError:  # Python 3 (the tests)
    import builtins as _builtins


def tr(text):
    """enigma2's own translation of a setup.xml text (the TV applies _() to it)."""
    f = getattr(_builtins, "_", None)
    return f(text) if f else text


# ---- what kind of element -------------------------------------------------
# By class NAME along the MRO, as OpenWebif does, so a plugin's subclass
# (TconfigInteger) or a vendored copy (MediaPortal's config.so) still maps.
_KIND = {
    "ConfigPassword": "password", "ConfigPIN": "pin", "ConfigDirectory": "directory",
    "ConfigNumber": "number", "ConfigSearchText": "text", "ConfigIP6": "ip6",
    "ConfigHexSequence": "hex", "ConfigText": "text", "ConfigTextBase": "text",
    "ConfigInteger": "integer", "ConfigFloat": "float", "ConfigIP": "ip", "ConfigMAC": "mac",
    "ConfigClock": "clock", "ConfigPosition": "position", "ConfigSequence": "sequence",
    "ConfigSelectionNumber": "selectionNumber", "ConfigSatlist": "choice",
    "ConfigNothing": "nothing", "ConfigSelection": "choice", "ConfigBoolean": "bool",
    "ConfigSlider": "slider", "ConfigSet": "multichoice", "ConfigLocations": "locations",
    "ConfigDateTime": "datetime",
}
SEQUENCE_KINDS = ("sequence", "ip", "mac", "clock", "position", "float")
# Shown, but not as something to change: an odd value form, or runtime state.
READ_ONLY_KINDS = ("locations", "ip6", "hex", "nothing", "datetime", "unknown")
SECRET_KINDS = ("password", "pin")

# Names that are secrets whatever the class says (ARCHITECTURE.md, "Plugin
# data survey"); the same list the backend keeps in internal/boxsettings.
_SECRET_NAMES = frozenset([
    "config.ParentalControl.setuppin", "config.ParentalControl.servicepin.0",
    "config.ParentalControl.servicepin.1", "config.ParentalControl.servicepin.2",
    "config.ParentalControl.storeservicepin", "config.plugins.serienRec.imap_login",
    "config.plugins.serienRec.imap_password", "config.plugins.serienRec.imap_server",
    "config.libporn.pincode", "config.gzerotier.apikey", "config.serienstream.username",
    "config.serienstream.passwd", "config.tunein.username", "config.gemininetscan.samba_username",
    "config.gemininetscan.samba_passwd", "config.geminiserver.www_user_password",
])
_SECRET_WORDS = ("passwd", "password", "pincode", "apikey", "api_key", "token", "secret", "cookie")
_SECRET_TOKENS = frozenset(["pass", "pin", "login", "username", "user", "credential", "credentials"])
# A yes/no named ...auth or ...usePin is not a secret: only text-like kinds are.
_TEXTISH = ("text", "number", "integer", "directory", "choice")


def kind_of(elem):
    for cls in type(elem).__mro__:
        k = _KIND.get(cls.__name__)
        if k:
            return k
    return "unknown"


def is_secret(path, kind):
    if kind in SECRET_KINDS or path in _SECRET_NAMES:
        return True
    if kind not in _TEXTISH:
        return False
    low = path.lower()
    if any(w in low for w in _SECRET_WORDS):
        return True
    return bool(_SECRET_TOKENS.intersection(re.split(r"[._]", low)[-2:]))


# ---- the tree ---------------------------------------------------------------
def children(node):
    """(name, child) pairs of a container node, else None for a leaf."""
    if isinstance(node, ConfigSubsection):     # Config, the root, is one too
        return sorted(node.dict().items())
    if isinstance(node, ConfigSubList):
        return [(str(i), x) for i, x in enumerate(node)]
    if isinstance(node, ConfigSubDict):
        return sorted((str(k), v) for k, v in node.items())
    return None


def walk(root, prefix="config", maxdepth=12, _seen=None, _depth=0):
    """Yield (dotted path, element) for every leaf below root; guarded against
    cycles and depth."""
    seen = _seen if _seen is not None else set()
    if id(root) in seen or _depth > maxdepth:
        return
    seen.add(id(root))
    for name, child in children(root) or []:
        path = prefix + "." + name
        if children(child) is not None:
            for x in walk(child, path, maxdepth, seen, _depth + 1):
                yield x
        elif isinstance(child, ConfigElement):
            yield path, child


class PathError(Exception):
    pass


_PATH_RE = re.compile(r"^config(\.[A-Za-z0-9_\-]+)+$")  # '-' for config.storage.<uuid>


def resolve(root, path):
    """A dotted path to its element. Never eval(): Setup.py does, this must not."""
    if not isinstance(path, string_types) or not _PATH_RE.match(path):
        raise PathError("bad_path")
    node = root
    for part in path.split(".")[1:]:
        if isinstance(node, ConfigSubsection):
            node = node.dict().get(part)
        elif isinstance(node, ConfigSubList):
            node = node[int(part)] if part.isdigit() and int(part) < len(node) else None
        elif isinstance(node, ConfigSubDict):
            node = dict((str(k), v) for k, v in node.items()).get(part)
        else:
            node = None
        if node is None:
            raise PathError("not_found")
    if children(node) is not None:
        raise PathError("not_an_item")
    return node


# ---- one element ---------------------------------------------------------------
def encode(kind, v):
    """A Python value as JSON."""
    if kind == "bool":
        return bool(v)
    if kind in ("integer", "pin", "number", "slider", "selectionNumber", "datetime"):
        return int(v)
    if kind in SEQUENCE_KINDS:
        return [int(x) for x in v]
    if kind == "multichoice":
        return list(v)
    if kind == "locations":
        return [str(x) for x in v]
    if v is None:  # ConfigDirectory.getValue() is None for ""
        return ""
    return v


def peek(elem, kind=None):
    """The current value without getValue()'s side effects."""
    kind = kind or kind_of(elem)
    if kind == "locations":
        return [x[0] for x in elem.locations]
    if kind == "ip6":
        return elem.text
    return elem.value


def tostr(elem):
    try:
        return elem.tostring(peek(elem))
    except Exception:
        return repr(peek(elem))


def rev_of(elem):
    """An opaque token for the stale check of the write step (S3); the same for a
    secret, it never reveals the value."""
    s = tostr(elem) + "|" + str(getattr(elem, "saved_value", None))
    if not isinstance(s, bytes):
        s = s.encode("utf-8")
    return hashlib.sha1(s).hexdigest()[:16]


def choices_of(elem, limit):
    ids = list(elem.choices)  # choicesList.__iter__ yields the ids
    out = []
    for i in ids[:limit]:
        try:
            label = elem.description[i]  # untranslated; the GUI applies _()
        except Exception:
            label = i
        out.append([i, tr(label) if isinstance(label, string_types) else label])
    return out, len(ids)


def describe(path, elem, store="settings", max_choices=64):
    kind = kind_of(elem)
    secret = is_secret(path, kind)
    d = {"store": store, "path": path, "kind": kind, "class": type(elem).__name__, "secret": secret}
    ro = None
    if kind in READ_ONLY_KINDS:
        ro = "unsupported_kind"
    elif getattr(elem, "save_disabled", False):
        ro = "not_persistent"  # NoSave(): config.timezone.region
    d["readOnly"] = ro
    try:
        d["dirty"] = bool(elem.isChanged())  # a TV screen holds an unsaved edit
    except Exception:
        d["dirty"] = None
    d["notifiers"] = [len(elem.notifiers), len(elem.notifiers_final)]  # [immediate, final]
    d["rev"] = rev_of(elem)
    try:
        d["default"] = None if secret else encode(kind, elem.default)
    except Exception:
        d["default"] = None
    if secret:
        d["value"] = None
        d["isSet"] = bool(tostr(elem)) and tostr(elem) != str(getattr(elem, "default", ""))
    else:
        d["value"] = encode(kind, peek(elem, kind))
        d["valueText"] = tostr(elem)
    if kind in ("choice", "selectionNumber", "multichoice"):
        d["choices"], d["choiceCount"] = choices_of(elem, max_choices)
        d["choicesTruncated"] = d["choiceCount"] > len(d["choices"])
    elif kind in ("integer", "pin"):
        d["limits"] = list(elem.limits[0])
    elif kind == "slider":
        d["limits"] = [elem.min, elem.max]
        d["step"] = elem.increment
    elif kind in SEQUENCE_KINDS:
        d["limits"] = [list(x) for x in elem.limits]
        d["separator"] = elem.seperator  # sic: enigma2's spelling
    return d


def describe_safe(path, elem, store="settings", max_choices=64):
    """One broken element (ConfigNumber.getValue() raises on a corrupt text) must
    not fail the list."""
    try:
        return describe(path, elem, store, max_choices)
    except Exception as e:
        return {"store": store, "path": path, "kind": "unknown", "class": type(elem).__name__,
                "secret": is_secret(path, "unknown"), "readOnly": "unreadable", "error": type(e).__name__}


# ---- stores -------------------------------------------------------------------
GEMINI_MODULE = "Plugins.GP4.geminicomm.gcommtools"


def stores():
    """The config roots there are: enigma2's own, and the Gemini plugins' second one
    (gpconfset, saved to /etc/enigma2/gpsettings at shutdown) when Gemini is
    loaded. Never imported from here: a second import would create a second
    root that diverges from the live one."""
    s = {"settings": config}
    mod = sys.modules.get(GEMINI_MODULE)
    if mod is not None and hasattr(mod, "gpconfset"):
        s["gp"] = mod.gpconfset
    return s


# ---- the setup menus (Screens/Setup.py's setup.xml) --------------------------------
def core_sections(setupdom, system_info):
    """Every <setup> of the box's setup.xml, items in order: label (translated), level,
    the config path (None for a heading), whether the box offers it (requires=)."""
    out = []
    for s in setupdom.getroot().findall("setup"):
        req = s.get("requires")
        sec = {"key": s.get("key", ""), "title": tr(s.get("title", "")),
               "available": (not req) or bool(system_info.get(req, False)), "items": []}
        for x in s.findall("item"):
            path = (x.text or "").strip()
            r = x.get("requires")
            try:
                level = int(x.get("level", 0))
            except ValueError:
                level = 0
            sec["items"].append({
                "label": tr(x.get("text", "")), "level": level, "path": path or None,
                "heading": not path, "requires": r,
                "available": (not r) or bool(system_info.get(r, False)),
                "notify": x.get("notify") == "true"})
        out.append(sec)
    return out


def section_items(sections, key, roots=None):
    """One section's items with each element described. A path that no element answers
    to is listed as missing, not dropped."""
    roots = roots or stores()
    for sec in sections:
        if sec["key"] != key:
            continue
        items = []
        for it in sec["items"]:
            if it["heading"]:
                items.append({"heading": True, "label": it["label"], "level": it["level"]})
                continue
            row = {"heading": False, "label": it["label"], "level": it["level"], "path": it["path"],
                   "requires": it["requires"], "available": it["available"], "notify": it["notify"]}
            try:
                row["item"] = describe_safe(it["path"], resolve(roots["settings"], it["path"]), "settings")
            except PathError as e:
                row["missing"] = str(e)
            items.append(row)
        return {"key": sec["key"], "title": sec["title"], "available": sec["available"], "items": items}
    return None


def tree_roots(store, roots=None):
    """The top-level groups of a store with the number of settings in each:
    config.plugins.<name> one by one, every other config.<name> as it is."""
    roots = roots or stores()
    if store not in roots:
        return None
    base = roots[store]
    out = []
    for name, child in children(base) or []:
        if children(child) is None:
            continue
        if store == "settings" and name == "plugins":
            for sub, c2 in children(child) or []:
                if children(c2) is not None:
                    out.append(("config.plugins." + sub, sum(1 for _ in walk(c2))))
        else:
            out.append(("config." + name, sum(1 for _ in walk(child, "config." + name))))
    return [{"store": store, "root": r, "count": n} for r, n in out if n]


def tree_items(store, root, q="", offset=0, limit=500, roots=None):
    """Settings below root (a dotted path), described, filtered by q (every word
    in the path), paged. root "config" is the whole store."""
    roots = roots or stores()
    if store not in roots:
        return None
    base = roots[store]
    if root != "config" and not _PATH_RE.match(root):
        return None
    if root != "config":
        try:
            node = base
            for part in root.split(".")[1:]:
                nxt = None
                if isinstance(node, ConfigSubsection):
                    nxt = node.dict().get(part)
                elif isinstance(node, ConfigSubList) and part.isdigit() and int(part) < len(node):
                    nxt = node[int(part)]
                elif isinstance(node, ConfigSubDict):
                    nxt = dict((str(k), v) for k, v in node.items()).get(part)
                if nxt is None:
                    raise PathError("not_found")
                node = nxt
        except PathError:
            return None
    else:
        node = base
    words = [w for w in q.lower().split() if w]
    rows = []
    start = max(0, int(offset))
    for path, elem in walk(node, root):
        if words and not all(w in path.lower() for w in words):
            continue
        rows.append((path, elem))
    page = rows[start:start + max(1, min(int(limit), 1000))]
    return {"store": store, "root": root, "total": len(rows), "offset": start,
            "items": [describe_safe(p, e, store) for p, e in page]}


def info(plugin_version):
    s = stores()
    try:
        level = config.usage.setup_level.index
    except Exception:
        level = None
    return {"ok": True, "api": 1, "plugin": plugin_version, "writes": False, "level": level,
            "stores": [{"id": k, "present": True} for k in sorted(s)]}
