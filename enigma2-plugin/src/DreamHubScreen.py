# -*- coding: utf-8 -*-
# The launcher's main (and, for v1, only from-the-remote) screen: a QR code
# to open DreamHub on a phone, and whether the backend is reachable —
# CONCEPT.md's "a launcher, not a second UI". Everything else lives in the
# web UI.
import json

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
        try:
            data = json.loads(raw)
            version = data.get("version", "?")
        except Exception:
            self.healthFailed(None)
            return
        if version != PluginVersion:
            self["status"].setText(_("Backend running (v%s)\nThis plugin is v%s — versions differ, some things may not match.") % (version, PluginVersion))
        else:
            self["status"].setText(_("Backend running (v%s)") % version)

    def healthFailed(self, failure):
        self["status"].setText(_("Backend not reachable on this box.\nIs DreamHub installed and running?"))

    def gotQR(self, raw):
        try:
            f = open(QR_TMP_PATH, "wb")
            try:
                f.write(raw)
            finally:
                f.close()
        except Exception:
            return
        pixmap = LoadPixmap(QR_TMP_PATH, cached=False)
        if pixmap:
            self["qr"].instance.setPixmap(pixmap)

    def qrFailed(self, failure):
        pass  # the status text already explains an unreachable backend

    def openSetup(self):
        from .DreamHubSetup import DreamHubSetup

        self.session.open(DreamHubSetup)
