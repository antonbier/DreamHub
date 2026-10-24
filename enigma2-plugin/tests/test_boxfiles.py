# -*- coding: utf-8 -*-
# Off-box tests of boxfiles.py (pure Python, no enigma2): python3 -m unittest discover -s enigma2-plugin/tests
import importlib.util
import json
import os
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
spec = importlib.util.spec_from_file_location("dh_boxfiles", os.path.join(HERE, "..", "src", "boxfiles.py"))
bf = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bf)

GOOD = b'{"serverlist": [{"id": "x", "name": "X"}]}'


class CheckPath(unittest.TestCase):
    def test_accepts_netcast_lists_and_their_backup(self):
        for n in ("netcastserver_dhfoo.json", "netcastepg_dh-foo_1.json", "netcastserver_gemini.json.dreamhub-backup"):
            self.assertEqual(bf.check_path("/etc/enigma2/" + n), n)

    def test_refuses_everything_else(self):
        for p in ("", "/etc/enigma2/settings", "/etc/enigma2/netcast_dvb2tvgid.json", "/etc/enigma2/netcastserver_x.txt",
                  "/etc/passwd", "/etc/enigma2/../passwd", "/etc/enigma2/sub/netcastserver_x.json",
                  "/etc/enigma2//netcastserver_x.json", "netcastserver_x.json", "/tmp/netcastserver_x.json",
                  "/etc/enigma2/netcastserver_x.json\x00.png", "/etc/enigma2/netcastserver_.json"):
            with self.assertRaises(bf.Refused, msg=p):
                bf.check_path(p)


class Host(unittest.TestCase):
    def test_host(self):
        names = {"one"}
        for h in ("192.168.1.43", "192.168.1.43:80", "localhost", "127.0.0.1:8765", "[::1]:80", "one", "ONE:80"):
            self.assertTrue(bf.host_ok(h, names), h)
        for h in ("", None, "evil.example.com", "evil.example.com:80", ":80"):
            self.assertFalse(bf.host_ok(h, names), h)


class Write(unittest.TestCase):
    def setUp(self):
        self.d = tempfile.mkdtemp()
        self.p = os.path.join(self.d, "netcastserver_dhx.json")

    def test_writes_atomically_and_keeps_the_mode(self):
        open(self.p, "w").write("{}")
        os.chmod(self.p, 0o640)
        self.assertEqual(bf.write(self.p, GOOD, self.d), len(GOOD))
        self.assertEqual(open(self.p, "rb").read(), GOOD)
        self.assertEqual(os.stat(self.p).st_mode & 0o777, 0o640)
        self.assertEqual(os.listdir(self.d), ["netcastserver_dhx.json"])  # no temp file left

    def test_new_file(self):
        bf.write(self.p, GOOD, self.d)
        self.assertEqual(os.stat(self.p).st_mode & 0o777, 0o644)

    def test_refusals_leave_the_file_alone(self):
        open(self.p, "wb").write(GOOD)
        for data, code in ((b"not json", "not_json"), (b"[1]", "not_json"), (b"\xff\xfe", "not_json"),
                           (b'{"a": "' + b"x" * bf.MAX_BYTES + b'"}', "too_large")):
            with self.assertRaises(bf.Refused) as c:
                bf.write(self.p, data, self.d)
            self.assertEqual(c.exception.code, code)
        self.assertEqual(open(self.p, "rb").read(), GOOD)
        with self.assertRaises(bf.Refused):
            bf.write(os.path.join(self.d, "other.json"), GOOD, self.d)
        self.assertEqual(os.listdir(self.d), ["netcastserver_dhx.json"])


if __name__ == "__main__":
    unittest.main()
