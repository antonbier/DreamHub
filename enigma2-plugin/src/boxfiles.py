# -*- coding: utf-8 -*-
# The one file write the plugin does for the backend (docs/NETCAST_PLAN.md,
# N3): Netcast's source lists in /etc/enigma2. Neither WebControl nor
# OpenWebif can write a file, and the Gemini Netcast plugin reads these
# files itself, so DreamHub puts a changed list there and nothing else.
#
# Pure Python (no enigma2, no Twisted) so it can be tested off the box.
# Deliberately narrow: one folder, two name patterns, valid JSON, a size cap,
# atomic replace. Which file, what is in it and the preview/confirm step are
# the backend's; this only refuses what no Netcast list could ever be.
import json
import os
import re
import socket

DIR = "/etc/enigma2"
# netcastserver_<id>.json / netcastepg_<id>.json and the copy of the old file
# the backend keeps before it changes one (<name>.dreamhub-backup).
NAME = re.compile(r"^netcast(server|epg)_[A-Za-z0-9_-]{1,64}\.json(\.dreamhub-backup)?$")
MAX_BYTES = 1 << 20  # these are small lists

_IPV4 = re.compile(r"^\d{1,3}(\.\d{1,3}){3}$")


class Refused(Exception):
    def __init__(self, code, message=""):
        Exception.__init__(self, code)
        self.code = code
        self.message = message


def check_path(path, directory=DIR):
    """The file name if `path` is one of the lists this route may write."""
    if not path or "\x00" in path:
        raise Refused("bad_path", "no file given")
    if os.path.dirname(path) != directory or os.path.normpath(path) != path:
        raise Refused("bad_path", "only files in %s" % directory)
    name = os.path.basename(path)
    if not NAME.match(name):
        raise Refused("bad_path", "not a Netcast source list")
    return name


def host_ok(host_header, own_names=None):
    """DNS rebinding: a page on another origin can make the browser send a
    request here under its own name. Accept an IP literal, localhost or the
    box's own name — what DreamHub's backend and a browser on the LAN use."""
    host = (host_header or "").strip().lower()
    if host.startswith("["):  # [::1]:80
        return host.find("]") > 0
    host = host.split(":")[0]
    if not host:
        return False
    if _IPV4.match(host) or host == "localhost":
        return True
    names = own_names if own_names is not None else _own_names()
    return host in names


def _own_names():
    try:
        n = socket.gethostname().lower()
    except Exception:
        return set()
    return set([n, n + ".local", n + ".lan", n + ".fritz.box"])


def write(path, data, directory=DIR):
    """Replace the file with `data` (a byte string), atomically. Returns the
    number of bytes written; raises Refused."""
    name = check_path(path, directory)
    if len(data) > MAX_BYTES:
        raise Refused("too_large", "more than %d bytes" % MAX_BYTES)
    try:
        parsed = json.loads(data.decode("utf-8"))
    except (ValueError, UnicodeDecodeError):
        raise Refused("not_json", "the content is not valid JSON")
    if not isinstance(parsed, dict):
        raise Refused("not_json", "a Netcast list is a JSON object")
    mode = 0o644
    try:
        mode = os.stat(path).st_mode & 0o777
    except OSError:
        pass
    tmp = os.path.join(directory, ".%s.dreamhub-tmp" % name)
    try:
        fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, mode)
        try:
            view = data
            while view:
                view = view[os.write(fd, view):]
            os.fsync(fd)
        finally:
            os.close(fd)
        os.chmod(tmp, mode)
        os.rename(tmp, path)
    except (IOError, OSError) as e:
        try:
            os.unlink(tmp)
        except OSError:
            pass
        raise Refused("write_failed", "%s" % e)
    return len(data)
