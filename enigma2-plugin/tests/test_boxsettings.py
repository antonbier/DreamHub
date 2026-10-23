# -*- coding: utf-8 -*-
# Off-box tests of boxsettings.py and websettings.py against the box's REAL
# Components/config.py (Python 3 with small stubs for enigma, six, Tools.*, netaddr
# in stubs/). The file is Dream Property's, so it is not in this repository:
#
#   scp -O root@<dreambox>:/usr/lib/enigma2/python/Components/config.py /tmp/e2config.py
#   DREAMHUB_E2_CONFIG_PY=/tmp/e2config.py python3 -m unittest discover -s enigma2-plugin/tests
#
# Without the variable the tests are skipped. They cannot say anything about
# enigma2 itself (Python 2.7, Twisted, the live tree) — see docs/BOXSETTINGS_PLAN.md,
# "S2 design", for what has to be checked on the box.
import builtins
import json
import os
import shutil
import sys
import tempfile
import unittest
import xml.etree.ElementTree as ET

HERE = os.path.dirname(os.path.abspath(__file__))
CONFIG_PY = os.environ.get("DREAMHUB_E2_CONFIG_PY", "")
SKIP = "set DREAMHUB_E2_CONFIG_PY to the box's Components/config.py"

if CONFIG_PY and os.path.exists(CONFIG_PY):
    # The real config.py goes into a scratch copy of the stubs.
    TMP = tempfile.mkdtemp(prefix="dh-e2-")
    shutil.copytree(os.path.join(HERE, "stubs"), os.path.join(TMP, "stubs"))
    shutil.copy(CONFIG_PY, os.path.join(TMP, "stubs", "Components", "config.py"))
    os.environ["STUB_CONFDIR"] = TMP
    sys.path.insert(0, os.path.join(TMP, "stubs"))
    # The plugin package, as enigma2 imports it: Plugins.Extensions.DreamHub
    sys.path.insert(0, os.path.join(HERE, "..", "src", ".."))
    builtins._ = lambda s: s
    from Components.config import (config, ConfigSubsection, ConfigSubList, ConfigSubDict,
                                   ConfigYesNo, ConfigSelection, ConfigSelectionNumber, ConfigInteger,
                                   ConfigNumber, ConfigText, ConfigPassword, ConfigPIN, ConfigIP,
                                   ConfigClock, ConfigSlider, ConfigSet, ConfigDirectory, NoSave)
    import importlib.util

    def _load(name):
        spec = importlib.util.spec_from_file_location("dh_" + name, os.path.join(HERE, "..", "src", name + ".py"),
                                                      submodule_search_locations=None)
        mod = importlib.util.module_from_spec(spec)
        return spec, mod

    # boxsettings.py has no relative imports; websettings.py does (from . import boxsettings)
    spec, bs = _load("boxsettings")
    sys.modules["dh_boxsettings"] = bs
    spec.loader.exec_module(bs)
    import types

    pkg = types.ModuleType("dhpkg")
    pkg.__path__ = [os.path.join(HERE, "..", "src")]
    pkg.PluginVersion = "test"
    sys.modules["dhpkg"] = pkg
    sys.modules["dhpkg.boxsettings"] = bs
    spec2 = importlib.util.spec_from_file_location("dhpkg.websettings", os.path.join(HERE, "..", "src", "websettings.py"))
    ws = importlib.util.module_from_spec(spec2)
    ws.__package__ = "dhpkg"
    spec2.loader.exec_module(ws)

    # A tree with one of every kind.
    config.plugins = ConfigSubsection()
    p = config.plugins.demo = ConfigSubsection()
    p.yes = ConfigYesNo(default=True)
    p.sel = ConfigSelection(default="b", choices=[("a", "Alpha"), ("b", "Beta"), ("c", "Gamma")])
    p.selnum = ConfigSelectionNumber(0, 10, 5, default=5)
    p.int = ConfigInteger(default=3, limits=(1, 9))
    p.num = ConfigNumber(default=7)
    p.txt = ConfigText(default="hello", fixed_size=False)
    p.pw = ConfigPassword(default="")
    p.pin = ConfigPIN(default=0)
    p.user = ConfigText(default="", fixed_size=False)
    p.ip = ConfigIP(default=[192, 168, 1, 1])
    p.clk = ConfigClock(default=3600 * 5)
    p.sld = ConfigSlider(default=10, increment=5, limits=(0, 100))
    p.set = ConfigSet(choices=[2, 4, 8, 16], default=[2, 4])
    p.dir = ConfigDirectory(default="")
    p.auth = ConfigYesNo(default=False)  # named like a secret, but a yes/no
    p.nosave = NoSave(ConfigYesNo(default=False))
    config.plugins.other = ConfigSubsection()
    config.plugins.other.n = ConfigInteger(default=1, limits=(0, 5))
    config.ci = ConfigSubList()
    config.ci.append(ConfigSubsection())
    config.ci[0].mode = ConfigYesNo()
    config.dict_ = ConfigSubDict()
    config.dict_["a-b"] = ConfigSubsection()
    config.dict_["a-b"].v = ConfigInteger(default=1)
    config.top = ConfigSubsection()
    config.top.x = ConfigYesNo(default=False)
    config.usage = ConfigSubsection()
    config.usage.setup_level = ConfigSelection(default="expert", choices=[("simple", "Simple"), ("intermediate", "Advanced"), ("expert", "Expert")])
    config.usage.hidden = ConfigYesNo(default=False)
    config.ParentalControl = ConfigSubsection()
    config.ParentalControl.setuppin = ConfigPIN(default=0)
    ENABLED = True
else:
    ENABLED = False


class FakeRequest(object):
    def __init__(self, route, **args):
        self.postpath = route.split("/")
        self.args = dict((k, [v]) for k, v in args.items())
        self.code = 200
        self.headers = {}

    def setResponseCode(self, c): self.code = c
    def setHeader(self, k, v): self.headers[k] = v


@unittest.skipUnless(ENABLED, SKIP)
class Describe(unittest.TestCase):
    def d(self, name):
        path = "config.plugins.demo." + name
        return bs.describe(path, bs.resolve(config, path))

    def test_kinds_values_limits_choices(self):
        y = self.d("yes"); self.assertEqual((y["kind"], y["value"], y["default"]), ("bool", True, True))
        s = self.d("sel"); self.assertEqual((s["kind"], s["value"], s["choices"]), ("choice", "b", [["a", "Alpha"], ["b", "Beta"], ["c", "Gamma"]]))
        n = self.d("selnum"); self.assertEqual((n["kind"], n["value"]), ("selectionNumber", 5)); self.assertIn("5", [c[0] for c in n["choices"]])
        i = self.d("int"); self.assertEqual((i["kind"], i["value"], i["limits"]), ("integer", 3, [1, 9]))
        self.assertEqual(self.d("num")["kind"], "number")
        self.assertEqual(self.d("txt")["value"], "hello")
        ip = self.d("ip"); self.assertEqual((ip["kind"], ip["value"], ip["valueText"], ip["separator"]), ("ip", [192, 168, 1, 1], "192.168.1.1", "."))
        c = self.d("clk"); self.assertEqual((c["kind"], len(c["value"]), c["valueText"].count(":")), ("clock", 2, 1))  # the default is a time stamp: hour and minute in the local zone
        sl = self.d("sld"); self.assertEqual((sl["kind"], sl["limits"], sl["step"]), ("slider", [0, 100], 5))
        ms = self.d("set"); self.assertEqual((ms["kind"], ms["value"]), ("multichoice", [2, 4])); self.assertEqual([c[0] for c in ms["choices"]], [2, 4, 8, 16])
        self.assertEqual(self.d("dir")["value"], "")

    def test_secrets_never_carry_a_value(self):
        for name in ("pw", "pin", "user"):
            d = self.d(name)
            self.assertTrue(d["secret"], name); self.assertIsNone(d["value"]); self.assertIsNone(d["default"])
            self.assertNotIn("valueText", d)
        d = bs.describe("config.ParentalControl.setuppin", bs.resolve(config, "config.ParentalControl.setuppin"))
        self.assertTrue(d["secret"] and d["value"] is None)
        self.assertFalse(self.d("auth")["secret"])  # a yes/no is not a secret by its name

    def test_not_persistent_and_notifier_counts(self):
        self.assertEqual(self.d("nosave")["readOnly"], "not_persistent")
        self.assertEqual(self.d("yes")["notifiers"], [0, 0])
        config.plugins.demo.yes.addNotifier(lambda e: None, initial_call=False)
        try:
            self.assertEqual(self.d("yes")["notifiers"][0], 1)
        finally:
            config.plugins.demo.yes.notifiers[:] = []

    def test_rev_changes_with_the_value_and_hides_secrets(self):
        before = self.d("txt")["rev"]
        config.plugins.demo.txt.value = "other"
        try:
            self.assertNotEqual(before, self.d("txt")["rev"])
        finally:
            config.plugins.demo.txt.value = "hello"
        self.assertEqual(before, self.d("txt")["rev"])

    def test_choices_are_capped_in_a_listing(self):
        big = ConfigSelection(default="0", choices=[(str(i), str(i)) for i in range(200)])
        d = bs.describe("config.plugins.demo.big", big)
        self.assertEqual((len(d["choices"]), d["choiceCount"], d["choicesTruncated"]), (64, 200, True))
        self.assertEqual(len(bs.describe_safe("config.plugins.demo.big", big, "settings", 100000)["choices"]), 200)

    def test_one_broken_element_does_not_fail(self):
        n = ConfigNumber(default=7)
        n.text = "not a number"
        d = bs.describe_safe("config.plugins.demo.broken", n)
        self.assertEqual((d["kind"], d["readOnly"]), ("unknown", "unreadable"))


@unittest.skipUnless(ENABLED, SKIP)
class Tree(unittest.TestCase):
    def test_walk_covers_lists_and_dicts(self):
        paths = set(p for p, _ in bs.walk(config))
        for p in ("config.ci.0.mode", "config.dict_.a-b.v", "config.plugins.demo.yes", "config.top.x"):
            self.assertIn(p, paths)

    def test_resolve_and_its_errors(self):
        self.assertIs(bs.resolve(config, "config.ci.0.mode"), config.ci[0].mode)
        self.assertIs(bs.resolve(config, "config.dict_.a-b.v"), config.dict_["a-b"].v)
        for path, code in (("config.ci.9.mode", "not_found"), ("config.nope.x", "not_found"),
                           ("config.plugins.demo", "not_an_item"), ("x.y", "bad_path"),
                           ("config.a;b", "bad_path"), ("config.__class__", "not_found"), (None, "bad_path")):
            with self.assertRaises(bs.PathError) as cm:
                bs.resolve(config, path)
            self.assertEqual(str(cm.exception), code, path)

    def test_roots_split_the_plugins(self):
        roots = dict((r["root"], r["count"]) for r in bs.tree_roots("settings"))
        self.assertEqual(roots["config.plugins.demo"], 16)
        self.assertEqual(roots["config.plugins.other"], 1)
        self.assertIn("config.ci", roots)
        self.assertNotIn("config.plugins", roots)
        self.assertIsNone(bs.tree_roots("nope"))

    def test_items_are_paged_and_searched(self):
        page = bs.tree_items("settings", "config.plugins.demo", limit=5)
        self.assertEqual((page["total"], len(page["items"]), page["offset"]), (16, 5, 0))
        rest = bs.tree_items("settings", "config.plugins.demo", offset=14, limit=5)
        self.assertEqual(len(rest["items"]), 2)
        hit = bs.tree_items("settings", "config", q="demo SEL")
        self.assertEqual(sorted(i["path"] for i in hit["items"]), ["config.plugins.demo.sel", "config.plugins.demo.selnum"])
        self.assertIsNone(bs.tree_items("settings", "config.nope"))
        self.assertIsNone(bs.tree_items("settings", "config.a;b"))
        self.assertIsNone(bs.tree_items("nope", "config"))
        self.assertEqual(len(bs.tree_items("settings", "config", limit=100000)["items"]), bs.tree_items("settings", "config")["total"])

    def test_the_second_store_is_only_there_when_gemini_is_loaded(self):
        self.assertEqual(sorted(bs.stores()), ["settings"])
        mod = types_module = type(sys)("gcm")
        mod.gpconfset = config
        sys.modules[bs.GEMINI_MODULE] = mod
        try:
            self.assertEqual(sorted(bs.stores()), ["gp", "settings"])
        finally:
            del sys.modules[bs.GEMINI_MODULE]

    def test_there_is_no_way_to_write(self):
        for name in ("apply_change", "coerce", "_flush", "set_value", "save"):
            self.assertFalse(hasattr(bs, name), name)


SETUP = """<setupxml>
  <setup key="usage" title="Customize">
    <item level="0" text="Setup Mode">config.usage.setup_level</item>
    <item level="1" text="Heading"></item>
    <item level="2" text="Hidden thing" requires="NoSuchThing">config.usage.hidden</item>
    <item level="1" text="Missing">config.nope.x</item>
  </setup>
  <setup key="rare" title="Rare" requires="Gone"><item level="0" text="x">config.top.x</item></setup>
</setupxml>"""


@unittest.skipUnless(ENABLED, SKIP)
class Sections(unittest.TestCase):
    def setUp(self):
        self.dom = ET.ElementTree(ET.fromstring(SETUP))
        self.info = {"Have": True}

    def test_core_sections(self):
        secs = bs.core_sections(self.dom, self.info)
        self.assertEqual([s["key"] for s in secs], ["usage", "rare"])
        self.assertEqual((secs[0]["available"], secs[1]["available"]), (True, False))
        items = secs[0]["items"]
        self.assertEqual([i["heading"] for i in items], [False, True, False, False])
        self.assertEqual((items[0]["level"], items[0]["path"]), (0, "config.usage.setup_level"))
        self.assertFalse(items[2]["available"])

    def test_section_items_describe_each_and_report_a_missing_one(self):
        secs = bs.core_sections(self.dom, self.info)
        out = bs.section_items(secs, "usage")
        self.assertEqual(out["items"][0]["item"]["value"], "expert")
        self.assertTrue(out["items"][1]["heading"])
        self.assertEqual(out["items"][3]["missing"], "not_found")
        self.assertIsNone(bs.section_items(secs, "nope"))


@unittest.skipUnless(ENABLED, SKIP)
class Route(unittest.TestCase):
    def setUp(self):
        self.res = ws.SettingsResource()
        self.res._core = lambda: bs.core_sections(ET.ElementTree(ET.fromstring(SETUP)), {})

    def get(self, route, **args):
        req = FakeRequest(route, **args)
        body = self.res.render_GET(req)
        return req, json.loads(body)

    def test_info(self):
        req, d = self.get("info")
        self.assertEqual((req.code, d["ok"], d["api"], d["writes"]), (200, True, 1, False))
        self.assertEqual(d["level"], 2)

    def test_sections_roots_items_item(self):
        _, d = self.get("sections"); self.assertEqual(d["sections"][0], {"key": "usage", "title": "Customize", "available": True, "items": 3})
        _, d = self.get("roots", store="settings"); self.assertTrue(any(r["root"] == "config.plugins.demo" for r in d["roots"]))
        _, d = self.get("items", section="usage"); self.assertEqual(d["items"][0]["path"], "config.usage.setup_level")
        _, d = self.get("items", store="settings", root="config.plugins.demo", limit="3"); self.assertEqual((d["total"], len(d["items"])), (16, 3))
        _, d = self.get("item", store="settings", path="config.plugins.demo.sel"); self.assertEqual(d["item"]["value"], "b")

    def test_errors_are_200_with_a_code(self):
        for route, args, code in (("items", {}, "bad_request"), ("items", {"section": "zz"}, "not_found"),
                                  ("items", {"root": "config.nope"}, "not_found"), ("roots", {"store": "x"}, "unknown_store"),
                                  ("item", {"path": "config.plugins.demo"}, "not_an_item"), ("item", {"path": "bad path"}, "bad_path"),
                                  ("items", {"root": "config", "offset": "x"}, "bad_request")):
            req, d = self.get(route, **args)
            self.assertEqual((req.code, d["ok"], d["error"]), (200, False, code), route)

    def test_unknown_route_is_404_and_a_write_is_405(self):
        req, d = self.get("nope"); self.assertEqual(req.code, 404)
        req = FakeRequest("info")
        body = json.loads(self.res.render_POST(req))
        self.assertEqual((req.code, body["error"], req.headers["Allow"]), (405, "read_only", "GET"))

    def test_a_bug_becomes_an_error_answer_not_an_exception(self):
        self.res._core = lambda: 1 / 0
        req, d = self.get("sections")
        self.assertEqual((req.code, d["ok"], d["error"]), (200, False, "internal"))


if __name__ == "__main__":
    unittest.main()
