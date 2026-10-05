# -*- coding: utf-8 -*-
# Registration only — no logic here (see __init__.py for gettext/config,
# DreamHubScreen.py for the actual launcher screen). Two menu locations,
# same screen: the plugin browser (PluginDescriptor's classic entry) and
# the Extensions ("blue") menu, matching where AutoTimer/AdvancedMovieSelection
# and the other Extensions plugins referenced in ARCHITECTURE.md put
# themselves. A third, at session start, registers /dreamhub/play on the
# box's web server (webplay.py).
from Plugins.Plugin import PluginDescriptor

from . import _


def main(session, **kwargs):
    from .DreamHubScreen import DreamHubScreen

    session.open(DreamHubScreen)


def sessionstart(reason, session=None, **kwargs):
    if reason != 0 or session is None:
        return
    try:
        from .webplay import register
    except ImportError:
        return  # no WebInterface on this box: nothing to register with
    register(session)


def Plugins(**kwargs):
    return [
        PluginDescriptor(
            name="DreamHub",
            description=_("Open DreamHub on your phone"),
            where=[PluginDescriptor.WHERE_PLUGINMENU, PluginDescriptor.WHERE_EXTENSIONSMENU],
            # 100x40, the Plugin Browser's classic size; drawn from
            # ../art/plugin-icon.svg. The Extensions menu shows no icons.
            icon="plugin.png",
            fnc=main,
        ),
        PluginDescriptor(where=PluginDescriptor.WHERE_SESSIONSTART, fnc=sessionstart),
    ]
