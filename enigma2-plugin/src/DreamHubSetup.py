# -*- coding: utf-8 -*-
# The launcher's "a few basic settings" (CONCEPT.md): which port DreamHub's
# backend listens on, and whether it has its own auth switched on. Anything
# beyond this — recordings, timers, everything else — is the web UI's job,
# not this screen's (PRINCIPLES.md, "Settings: one central menu").
from Components.ActionMap import ActionMap
from Components.config import config, getConfigListEntry
from Components.ConfigList import ConfigListScreen
from Components.Label import Label
from Screens.Screen import Screen

from . import _


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
        self.list = [
            getConfigListEntry(_("Backend port"), config.plugins.dreamhub.port),
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
        for entry in self.list:
            entry[1].save()
        self.close()

    def keyCancel(self):
        for entry in self.list:
            entry[1].cancel()
        self.close()
