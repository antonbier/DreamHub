# -*- coding: utf-8 -*-
# Talks to DreamHub's own backend on 127.0.0.1 — no business logic here,
# just the two calls this launcher needs (CONCEPT.md: "a launcher, not a
# second UI"; PRINCIPLES.md: no re-implementing what already exists
# elsewhere). Both endpoints are deliberately public/unauthenticated on the
# backend side (backend/internal/api's handleHealth, handleQR), so no
# credentials are needed here yet, even with config.plugins.dreamhub.authEnabled
# on — that flag is about calls this plugin doesn't make yet, kept for when
# it does.
#
# Uses Twisted's getPage, the same non-blocking HTTP mechanism the rest of
# Enigma2 uses (e.g. Screens.Ipkg, SoftwareManager) — a slow or unreachable
# backend must never freeze the GUI thread.
from twisted.web.client import getPage

from .portcfg import readPort


def backendURL(path):
    return "http://127.0.0.1:%d%s" % (readPort(), path)


def fetchHealth(onSuccess, onError):
    getPage(backendURL("/api/v1/health")).addCallback(onSuccess).addErrback(onError)


def fetchQR(onSuccess, onError):
    getPage(backendURL("/api/v1/qr.png")).addCallback(onSuccess).addErrback(onError)
