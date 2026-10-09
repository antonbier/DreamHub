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
#
# Optional "t" parameter (milliseconds): resume where Plex left off (#30).
# There is no API to start playback already seeked, so this opens the file
# from the beginning and then seeks once the service's own seek interface
# is ready — the same two-step dance CutListEditor's own resume-point
# prompt does with a .ts.cuts file, just without a cuts file to read the
# position from. Not yet confirmed on the box (no device access at the
# time this was written) — if the service isn't seekable yet after the
# delay below, the seek is silently skipped and the film just plays from
# the start, same as before this existed.
import json

from enigma import eServiceReference, eTimer
from twisted.web import resource

# How long to wait for the stream to open before seeking. Long enough for
# DreamHub's own proxy and the Plex server to answer the first request;
# too short and the seek lands before there's anything to seek in.
_SEEK_DELAY_MS = 2500


class PlayResource(resource.Resource):
    isLeaf = True

    def __init__(self, session):
        resource.Resource.__init__(self)
        self.session = session

    def render_GET(self, request):
        # POST only: a GET can be triggered by any web page open on the
        # LAN (an <img> pointing here) and would start playback on the TV.
        # DreamHub's backend always posts.
        request.setResponseCode(405)
        request.setHeader("Allow", "POST")
        request.setHeader("Content-Type", "application/json; charset=utf-8")
        return json.dumps({"ok": False, "error": "use POST"})

    def render_POST(self, request):
        request.setHeader("Content-Type", "application/json; charset=utf-8")
        ref = request.args.get("sRef", [""])[0]
        title = request.args.get("title", [""])[0]
        start_ms = request.args.get("t", [""])[0]
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
        if start_ms.isdigit():
            # A malformed value is just ignored: an exception here would,
            # like the eTimer bug below, keep the answer from going out
            # although the film already plays.
            self._seekOnceReady(int(start_ms))
        return json.dumps({"ok": True})

    def _seekOnceReady(self, start_ms):
        timer = eTimer()

        def seek():
            # Keep the timer and its connection alive until they fire —
            # dropping the only reference here would let them be collected
            # first. Confirmed on the reference box (2026-10-06): this
            # image's eTimer takes callbacks via timeout.connect(), not
            # callback.append() (a different Enigma2 fork's API) — the
            # earlier version of this method raised AttributeError here,
            # after the service had already opened, so playback still
            # started but the HTTP response back to DreamHub never did.
            self._seekTimer = None
            self._seekConn = None
            service = self.session.nav.getCurrentService()
            if service is None:
                return
            seekable = service.seek()
            if seekable is None:
                return  # not every service (or not yet) supports seeking
            seekable.seekTo(start_ms * 90)  # pts: a 90kHz clock

        self._seekConn = timer.timeout.connect(seek)
        self._seekTimer = timer
        timer.start(_SEEK_DELAY_MS, True)


def register(session):
    from Plugins.Extensions.WebInterface.WebChilds.Toplevel import addExternalChild

    root = resource.Resource()
    root.putChild("play", PlayResource(session))
    # (path, resource, name, version, listed in WebInterface's menu)
    addExternalChild(("dreamhub", root, "DreamHub", "1", False))
