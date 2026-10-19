# -*- coding: utf-8 -*-
# The launcher's "a few basic settings" (CONCEPT.md): which port DreamHub's
# backend listens on, and whether it has its own auth switched on. Anything
# beyond this — recordings, timers, everything else — is the web UI's job,
# not this screen's (PRINCIPLES.md, "Settings: one central menu").
from Components.ActionMap import ActionMap
from Components.config import ConfigInteger, config, getConfigListEntry
from Components.ConfigList import ConfigListScreen
from Components.Label import Label
from enigma import eConsoleAppContainer
from Screens.MessageBox import MessageBox
from Screens.Screen import Screen

from . import _
from .portcfg import RESTART_CMD, readPort, writePort


class DreamHubSetup(Screen, ConfigListScreen):
    skin = """
    <screen name="DreamHubSetup" position="center,center" size="560,300" title="DreamHub Settings">
        <widget name="config" position="20,20" size="520,220" scrollbarMode="showOnDemand" />
        <widget name="key_red" position="20,260" size="240,30" font="Regular;20" foregroundColor="red" />
        <widget name="key_green" position="280,260" size="240,30" font="Regular;20" foregroundColor="green" />
    </screen>
    """

    def __init__(self, session):
        Screen.__init__(self, session)
        self.setTitle(_("DreamHub Settings"))
        self.oldPort = readPort()
        # Not an enigma2 setting: the port is read from and written to
        # /etc/default/dreamhub (portcfg.py), where the backend reads it too.
        self.port = ConfigInteger(default=self.oldPort, limits=(1024, 65535))
        self.list = [
            getConfigListEntry(_("Backend port"), self.port),
            getConfigListEntry(_("Backend requires a password"), config.plugins.dreamhub.authEnabled),
        ]
        ConfigListScreen.__init__(self, self.list, session=session)
        self["key_red"] = Label(_("Cancel"))
        self["key_green"] = Label(_("Save"))
        self["actions"] = ActionMap(
            ["OkCancelActions", "ColorActions"],
            {
                "cancel": self.keyCancel,
                "red": self.keyCancel,
                "green": self.keySave,
            },
            -1,
        )

    def keySave(self):
        config.plugins.dreamhub.authEnabled.save()
        if self.port.value == self.oldPort:
            self.close()
            return
        # Preview and confirm: a new port restarts DreamHub and changes the
        # address everyone uses, so say so and wait for a yes.
        self.session.openWithCallback(
            self.portConfirmed,
            MessageBox,
            _("Change the DreamHub port to %d?\n\nDreamHub restarts for a few seconds. Browsers and bookmarks need the new address (http://<box>:%d).") % (self.port.value, self.port.value),
            MessageBox.TYPE_YESNO,
        )

    def portConfirmed(self, yes):
        if not yes:
            return
        try:
            writePort(self.port.value)
        except (IOError, OSError) as e:
            self.session.open(MessageBox, _("The port could not be saved: %s") % e, MessageBox.TYPE_ERROR)
            return
        self.console = eConsoleAppContainer()
        self.console.execute(RESTART_CMD)
        self.close()

    def keyCancel(self):
        config.plugins.dreamhub.authEnabled.cancel()
        self.close()
