# -*- coding: utf-8 -*-
# The launcher's main (and, for v1, only from-the-remote) screen: a QR code
# to open DreamHub on a phone, and whether the backend is reachable —
# CONCEPT.md's "a launcher, not a second UI". Everything else lives in the
# web UI.
import json
import re
import traceback

from Components.ActionMap import ActionMap
from Components.Label import Label
from Components.Pixmap import Pixmap
from Screens.Screen import Screen
from Tools.LoadPixmap import LoadPixmap

from . import PluginVersion, _
from .backend import fetchHealth, fetchQR

# Written fresh on every open; nothing sensitive in it (the QR just encodes
# this box's own URL), and no persistent state the plugin needs to manage.
QR_TMP_PATH = "/tmp/dreamhub_qr.png"

# Verified live 2026-09-25 on the reference box: the status check failed
# inside the real Enigma2 process (showed "not reachable") even though the
# server log showed a real 200 response, and the exact same getPage()+
# json.loads() code run standalone against the same backend worked
# perfectly — something about the full Enigma2/Twisted environment
# (heavily loaded with third-party plugins, unlike a plain `python2 -c`
# process) behaves differently in a way that wasn't pinned down. The old
# bare `except: self.healthFailed(None)` swallowed whatever the real cause
# was; this logs it instead, and falls back to a regex-extracted version
# number so the screen still shows something useful even if json.loads
# itself turns out to be the part that's failing.
LOG_PATH = "/tmp/dreamhub-plugin.log"


def _log(message):
    try:
        with open(LOG_PATH, "a") as f:
            f.write(message + "\n")
    except Exception:
        pass  # logging is best-effort; must never itself break the screen


# Root cause found live, 2026-09-25 (the failure the logging above was
# added to catch): eLabel's SWIG-wrapped setText() takes a C++
# std::string, and raises "TypeError: argument 2 of type 'std::string
# const &'" if handed a Python `unicode` object instead of `str` --
# json.loads() always returns `unicode` for JSON strings in Python 2, and
# "%s" % a_unicode_value silently promotes the whole formatted string to
# `unicode` too. gettext's own dgettext() already returns plain `str`
# (encoded per the .mo file), which is why the *static* messages
# (healthFailed's, the hint/key labels) never hit this — only text built
# from the backend's JSON did. Route every dynamic setText() through this.
def _toStr(value):
    if isinstance(value, unicode):  # noqa: F821 -- Python 2 builtin
        return value.encode("utf-8")
    return value


class DreamHubScreen(Screen):
    skin = """
    <screen name="DreamHubScreen" position="center,center" size="560,420" title="DreamHub">
        <widget name="qr" position="20,20" size="220,220" alphatest="blend" />
        <widget name="status" position="260,20" size="280,180" font="Regular;22" valign="top" />
        <widget name="hint" position="20,260" size="520,60" font="Regular;20" valign="top" />
        <widget name="key_red" position="20,370" size="240,40" font="Regular;20" halign="left" valign="center" foregroundColor="red" />
        <widget name="key_green" position="280,370" size="260,40" font="Regular;20" halign="left" valign="center" foregroundColor="green" />
    </screen>
    """

    def __init__(self, session):
        Screen.__init__(self, session)
        self.setTitle(_("DreamHub"))
        self["qr"] = Pixmap()
        self["status"] = Label(_("Checking backend…"))
        self["hint"] = Label(_("Scan this code with your phone's camera to open DreamHub."))
        self["key_red"] = Label(_("Close"))
        self["key_green"] = Label(_("Settings"))
        self["actions"] = ActionMap(
            ["OkCancelActions", "ColorActions"],
            {
                "cancel": self.close,
                "red": self.close,
                "green": self.openSetup,
            },
            -1,
        )
        self.onLayoutFinish.append(self.start)

    def start(self):
        fetchHealth(self.gotHealth, self.healthFailed)
        fetchQR(self.gotQR, self.qrFailed)

    def gotHealth(self, raw):
        version = None
        try:
            data = json.loads(raw)
            version = data.get("version", "?")
        except Exception:
            _log("gotHealth: json.loads failed on %r\n%s" % (raw, traceback.format_exc()))
            match = re.search(r'"version"\s*:\s*"([^"]+)"', raw or "")
            if match:
                version = match.group(1)
        if version is None:
            self.healthFailed(None)
            return
        if version != PluginVersion:
            self["status"].setText(_toStr(_("Backend running (v%s)\nThis plugin is v%s — versions differ, some things may not match.") % (version, PluginVersion)))
        else:
            self["status"].setText(_toStr(_("Backend running (v%s)") % version))

    def healthFailed(self, failure):
        _log("healthFailed: %r" % (failure,))
        self["status"].setText(_("Backend not reachable on this box.\nIs DreamHub installed and running?"))

    def gotQR(self, raw):
        try:
            f = open(QR_TMP_PATH, "wb")
            try:
                f.write(raw)
            finally:
                f.close()
        except Exception:
            _log("gotQR: writing %s failed\n%s" % (QR_TMP_PATH, traceback.format_exc()))
            return
        pixmap = LoadPixmap(QR_TMP_PATH, cached=False)
        if pixmap:
            self["qr"].instance.setPixmap(pixmap)

    def qrFailed(self, failure):
        _log("qrFailed: %r" % (failure,))  # the status text already explains an unreachable backend

    def openSetup(self):
        from .DreamHubSetup import DreamHubSetup

        self.session.open(DreamHubSetup)
