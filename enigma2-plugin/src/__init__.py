# -*- coding: utf-8 -*-
# Gettext wiring and the plugin's own settings — the two things every other
# module in this package needs, so they live here rather than in plugin.py
# (which only exists to register the PluginDescriptor).
#
# Localization mechanism confirmed against DreamOS/opendreambox's own
# convention (PROJECT_PLAN.md, "Enigma2 plugin localization mechanism
# confirmed"): PluginLanguageDomain/PluginLanguagePath, bindtextdomain(),
# re-run on every language.addCallback() so the plugin follows
# config.osd.language automatically — no separate language setting for the
# user to maintain (CLAUDE.md's multilingual requirement). .po files here
# compile to .mo via msgfmt at package build time (WP9); only the .po
# sources are committed.
import gettext
import os

from Components.Language import language
from Tools.Directories import SCOPE_PLUGINS, resolveFilename

# Kept in sync by hand with backend/internal/api.Version — DreamHubScreen
# shows a mismatch warning rather than silently assuming compatibility.
PluginVersion = "0.9.4"

PluginLanguageDomain = "DreamHub"
PluginLanguagePath = "Extensions/DreamHub/locale"


def localeInit():
    lang = language.getLanguage()  # e.g. "de_DE" (confirmed format, PROJECT_PLAN.md)
    os.environ["LANGUAGE"] = lang[:2]
    gettext.bindtextdomain(PluginLanguageDomain, resolveFilename(SCOPE_PLUGINS, PluginLanguagePath))


def _(txt):
    return gettext.dgettext(PluginLanguageDomain, txt)


localeInit()
language.addCallback(localeInit)

# The plugin's own settings. The port is not stored here: portcfg.py reads
# and writes the one place the backend itself reads it from
# (/etc/default/dreamhub). Everything else stays the web UI's job
# (PRINCIPLES.md, "one central menu"). authEnabled is no longer offered on
# the settings screen (it never did anything); the entry stays so an
# existing enigma2 settings file keeps loading.
from Components.config import ConfigSubsection, ConfigYesNo, config  # noqa: E402

config.plugins.dreamhub = ConfigSubsection()
config.plugins.dreamhub.authEnabled = ConfigYesNo(default=False)
