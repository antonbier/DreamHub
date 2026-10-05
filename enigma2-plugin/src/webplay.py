# -*- coding: utf-8 -*-
# /dreamhub/play on the box's own web server (WebInterface, port 80): plays
# a service reference in Enigma2's movie player, the way a recording plays
# from the movie list — with seeking, pause and the movie infobar.
#
# Needed for Plex films (PlexHub): WebInterface's /web/zap only opens the
# movie player for a reference whose path starts with "/" (a file), so a
# stream URL would play like a channel, with the TV infobar and no seeking
# (SwitchService.py on the box, ARCHITECTURE.md). Registered through
# WebInterface's addExternalChild, like AutoTimer's API, so it sits behind
# the same login as /web/*. Only the backend on the same box calls it.
import json

from enigma import eServiceReference
from twisted.web import resource


class PlayResource(resource.Resource):
    isLeaf = True

    def __init__(self, session):
        resource.Resource.__init__(self)
        self.session = session

    def render_POST(self, request):
        return self.render_GET(request)

    def render_GET(self, request):
        request.setHeader("Content-Type", "application/json; charset=utf-8")
        ref = request.args.get("sRef", [""])[0]
        title = request.args.get("title", [""])[0]
        if not ref:
            request.setResponseCode(400)
            return json.dumps({"ok": False, "error": "sRef missing"})
        from Screens.Standby import inStandby
        if inStandby is not None:
            return json.dumps({"ok": False, "error": "the box is in standby"})
        from Screens.InfoBar import MoviePlayer
        eref = eServiceReference(ref)
        if title:
            eref.setName(title)
        if isinstance(self.session.current_dialog, MoviePlayer):
            # Already playing a film: switch to this one in the same player.
            self.session.nav.playService(eref)
        else:
            # The movie player remembers the channel and goes back to it
            # when it's closed.
            self.session.open(MoviePlayer, eref)
        return json.dumps({"ok": True})


def register(session):
    from Plugins.Extensions.WebInterface.WebChilds.Toplevel import addExternalChild

    root = resource.Resource()
    root.putChild("play", PlayResource(session))
    # (path, resource, name, version, listed in WebInterface's menu)
    addExternalChild(("dreamhub", root, "DreamHub", "1", False))
