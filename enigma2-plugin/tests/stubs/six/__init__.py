def itervalues(d): return iter(d.values())
def iteritems(d): return iter(d.items())
def ensure_str(s, encoding='utf-8', errors='strict'):
    return s.decode(encoding, errors) if isinstance(s, bytes) else s
def ensure_text(s, encoding='utf-8', errors='strict'):
    return s.decode(encoding, errors) if isinstance(s, bytes) else s
unichr = chr
