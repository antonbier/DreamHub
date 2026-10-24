# -*- coding: utf-8 -*-
# The launcher's one setting (CONCEPT.md): which port DreamHub's backend
# listens on. Anything beyond this — recordings, timers, passwords,
# everything else — is the web UI's job, not this screen's (PRINCIPLES.md,
# "Settings: one central menu"). The "backend requires a password" switch
# the first version had here did nothing (the plugin makes no
# authenticated calls), so it is gone rather than left to confuse.
from Components.ActionMap import ActionMap
from Components.config import ConfigNumber, config, configfile, getConfigListEntry
from Components.ConfigList import ConfigListScreen
from Components.Label import Label
from enigma import eConsoleAppContainer
from Screens.MessageBox import MessageBox
from Screens.Screen import Screen

from . import _
from .DreamHubScreen import KEY_GREEN, KEY_RED
from .portcfg import RESTART_CMD, readPort, writePort

MIN_PORT = 1024
MAX_PORT = 65535


class DreamHubSetup(Screen, ConfigListScreen):
    skin = """
    <screen name="DreamHubSetup" position="center,center" size="760,300" title="DreamHub Settings">
        <widget name="config" position="30,30" size="700,110" itemHeight="48" scrollbarMode="showOnDemand" />
        <widget name="hint" position="30,150" size="700,60" font="Regular;22" valign="top" />
        <eLabel position="30,235" size="8,40" backgroundColor="%s" />
        <widget name="key_red" position="48,235" size="320,40" font="Regular;24" halign="left" valign="center" />
        <eLabel position="390,235" size="8,40" backgroundColor="%s" />
        <widget name="key_green" position="408,235" size="320,40" font="Regular;24" halign="left" valign="center" />
    </screen>
    """ % (KEY_RED, KEY_GREEN)

    def __init__(self, session):
        Screen.__init__(self, session)
        self.setTitle(_("DreamHub Settings"))
        self.oldPort = readPort()
        # Not an enigma2 setting: the port is read from and written to
        # /etc/default/dreamhub (portcfg.py), where the backend reads it too.
        # ConfigNumber shows "8765", not the zero-padded "08765" a
        # ConfigInteger of five digits does.
        self.port = ConfigNumber(default=self.oldPort)
        self.list = [
            getConfigListEntry(_("Backend port"), self.port),
            getConfigListEntry(_("Edit Netcast sources"), config.plugins.dreamhub.fileWrite),
        ]
        ConfigListScreen.__init__(self, self.list, session=session)
        self["hint"] = Label(_("The port DreamHub listens on (%d to %d). Changing it restarts DreamHub for a few seconds.") % (MIN_PORT, MAX_PORT))
        self["key_red"] = Label(_("Cancel"))
        self["key_green"] = Label(_("Save"))
        self["actions"] = ActionMap(
            ["OkCancelActions", "ColorActions"],
            {
                "cancel": self.close,
                "red": self.close,
                "green": self.keySave,
            },
            -1,
        )

    def keySave(self):
        # The switch is an ordinary enigma2 setting: saved at once, apart from the port.
        config.plugins.dreamhub.fileWrite.save()
        config.plugins.dreamhub.save()
        configfile.save()
        port = self.port.value
        if not MIN_PORT <= port <= MAX_PORT:
            self.session.open(MessageBox, _("The port must be between %d and %d.") % (MIN_PORT, MAX_PORT), MessageBox.TYPE_ERROR)
            return
        if port == self.oldPort:
            self.close()
            return
        # Preview and confirm: a new port restarts DreamHub and changes the
        # address everyone uses, so say so and wait for a yes.
        self.session.openWithCallback(
            self.portConfirmed,
            MessageBox,
            _("Change the DreamHub port to %d?\n\nDreamHub restarts for a few seconds. Browsers and bookmarks need the new address (http://<box>:%d).") % (port, port),
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
