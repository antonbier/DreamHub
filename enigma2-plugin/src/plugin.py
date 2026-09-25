# -*- coding: utf-8 -*-
# Registration only — no logic here (see __init__.py for gettext/config,
# DreamHubScreen.py for the actual launcher screen). Two menu locations,
# same screen: the plugin browser (PluginDescriptor's classic entry) and
# the Extensions ("blue") menu, matching where AutoTimer/AdvancedMovieSelection
# and the other Extensions plugins referenced in ARCHITECTURE.md put
# themselves.
from Plugins.Plugin import PluginDescriptor

from . import _


def main(session, **kwargs):
    from .DreamHubScreen import DreamHubScreen

    session.open(DreamHubScreen)


def Plugins(**kwargs):
    return [
        PluginDescriptor(
            name="DreamHub",
            description=_("Open DreamHub on your phone"),
            where=[PluginDescriptor.WHERE_PLUGINMENU, PluginDescriptor.WHERE_EXTENSIONSMENU],
            fnc=main,
        ),
    ]
